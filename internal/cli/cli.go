// Package cli parses commands and formats their output. It wires the
// internal packages together and holds no analysis logic of its own.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/morethancoder/qtldr/internal/config"
)

// Exit codes.
const (
	ExitOK     = 0
	ExitBreach = 1
	ExitError  = 2
)

// errUsage marks errors that are the caller's mistake; usage is printed.
var errUsage = errors.New("usage")

// globals are flags accepted before or after any command.
type globals struct {
	config  string
	dir     string
	json    bool
	quiet   bool
	verbose bool
}

func (g *globals) register(fs *flag.FlagSet) {
	fs.StringVar(&g.config, "config", g.config, "path to .qtldr.toml (default: <module root>/.qtldr.toml)")
	fs.StringVar(&g.dir, "C", g.dir, "run as if started in `dir`")
	fs.BoolVar(&g.json, "json", g.json, "print JSON")
	fs.BoolVar(&g.quiet, "quiet", g.quiet, "print only results and errors")
	fs.BoolVar(&g.verbose, "v", g.verbose, "print progress details")
}

// env is what every command receives.
type env struct {
	ctx    context.Context
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	g      globals
	sys    system
}

type command struct {
	name    string
	args    string
	summary string
	run     func(e *env, fs *flag.FlagSet, args []string) error
	flags   func(fs *flag.FlagSet)
}

func commands() []command {
	return []command{
		{name: "init", summary: "write .qtldr.toml, add .gitignore rules, run doctor", run: runInit},
		{name: "doctor", summary: "check go, git, gremlins and the editor command", run: runDoctor},
		{name: "analyze", args: "[pkgs...] [--coverage] [--mutate] [--changed]", summary: "scan structure and complexity (and run tests with --coverage); write the snapshot", run: runAnalyze, flags: analyzeFlags},
		{name: "check", args: "[--changed | --all] [--fast] [--files-from-stdin] [ids or files...]", summary: "pass/fail report against the thresholds; exit 1 on a breach", run: runCheck, flags: checkFlags},
		{name: "show", args: "<id>", summary: "everything about one package, function or type", run: runShow},
		{name: "worst", args: "[--metric crap|coverage|mutation|cognitive|cc] [-n 10]", summary: "rank functions by a metric", run: runWorst, flags: worstFlags},
		{name: "mutate", args: "[--func <id>] [--pkg <path>] [--changed] [--force]", summary: "mutation testing; only changed code is re-tested", run: runMutate, flags: mutateFlags},
		{name: "serve", args: "[--open] [--port N] [--watch]", summary: "web UI on 127.0.0.1", run: runServe, flags: serveFlags},
		{name: "note", args: "add <id> [--line N] <text> | list [id] | resolve <note-id>", summary: "code notes for people and agents", run: runNote, flags: noteFlags},
		{name: "explain", args: "[term]", summary: "what a metric means (same text as the UI tooltips)", run: runExplain},
		{name: "hook", args: "install [--claude] [--git]", summary: "run qtldr check from Claude Code or git pre-push", run: runHook, flags: hookFlags},
	}
}

// Run executes qtldr with args (without the program name) and returns the
// exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return run(&env{ctx: ctx, stdin: os.Stdin, stdout: stdout, stderr: stderr, sys: osSystem{}}, args)
}

func run(e *env, args []string) int {
	top := flag.NewFlagSet("qtldr", flag.ContinueOnError)
	top.SetOutput(io.Discard)
	e.g.register(top)
	if err := top.Parse(args); err != nil || top.NArg() == 0 {
		usage(e.stderr)
		return ExitError
	}
	cmd, ok := findCommand(top.Arg(0))
	if !ok {
		fmt.Fprintf(e.stderr, "qtldr: unknown command %q\n\n", top.Arg(0))
		usage(e.stderr)
		return ExitError
	}
	return report(e, cmd, execute(e, cmd, top.Args()[1:]))
}

func execute(e *env, cmd command, args []string) error {
	fs := flag.NewFlagSet("qtldr "+cmd.name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	e.g.register(fs)
	if cmd.flags != nil {
		cmd.flags(fs)
	}
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	return cmd.run(e, fs, pos)
}

// report prints err and returns the exit code for it.
func report(e *env, cmd command, err error) int {
	var code exitCode
	switch {
	case err == nil:
		return ExitOK
	case errors.As(err, &code):
		return int(code)
	case errors.Is(err, errUsage):
		fmt.Fprintf(e.stderr, "qtldr %s: %v\nusage: qtldr %s %s\n", cmd.name, strings.TrimPrefix(err.Error(), "usage: "), cmd.name, cmd.args)
	default:
		fmt.Fprintf(e.stderr, "qtldr %s: %v\n", cmd.name, err)
	}
	return ExitError
}

// exitCode is an error that only sets the exit code; its output was already
// printed.
type exitCode int

func (c exitCode) Error() string { return fmt.Sprintf("exit %d", int(c)) }

// parseInterspersed lets flags follow positional arguments, e.g.
// `qtldr show applyTiered --json`.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

func findCommand(name string) (command, bool) {
	for _, c := range commands() {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "qtldr shows how risky Go code is to change.\n\nusage: qtldr [-C dir] [--config path] [--json] [--quiet] [-v] <command> [args]\n\ncommands:")
	cmds := commands()
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].name < cmds[j].name })
	for _, c := range cmds {
		fmt.Fprintf(w, "  %-8s %s\n", c.name, c.summary)
	}
}

// startDir is -C if given, else the working directory.
func (e *env) startDir() (string, error) {
	if e.g.dir != "" {
		return filepath.Abs(e.g.dir)
	}
	return os.Getwd()
}

// moduleRoot walks up from the start directory to the nearest go.mod.
func (e *env) moduleRoot() (string, error) {
	start, err := e.startDir()
	if err != nil {
		return "", err
	}
	return findModuleRoot(start)
}

// findModuleRoot walks up from dir to the nearest go.mod, resolving
// symlinks so paths compare equal (/tmp vs /private/tmp on macOS).
func findModuleRoot(start string) (string, error) {
	if real, err := filepath.EvalSymlinks(start); err == nil {
		start = real
	}
	for dir := start; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("no go.mod in %s or any parent; run qtldr inside a Go module or pass -C <dir>", start)
		}
	}
}

// setup finds the module root and loads its configuration.
func (e *env) setup() (string, config.Config, error) {
	root, err := e.moduleRoot()
	if err != nil {
		return "", config.Config{}, err
	}
	cfg, err := e.loadConfig(root)
	return root, cfg, err
}

// progressLine prints pipeline progress with -v.
func (e *env) progressLine(msg string) { e.debug("%s", msg) }

// loadConfig reads --config or <root>/.qtldr.toml and prints its warnings.
func (e *env) loadConfig(root string) (config.Config, error) {
	path := e.g.config
	if path == "" {
		path = filepath.Join(root, config.FileName)
	}
	cfg, warnings, err := config.Load(path)
	for _, w := range warnings {
		fmt.Fprintln(e.stderr, "warning:", w)
	}
	return cfg, err
}

func (e *env) printJSON(v any) error {
	enc := json.NewEncoder(e.stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// info prints a progress line unless --quiet.
func (e *env) info(format string, args ...any) {
	if !e.g.quiet {
		fmt.Fprintf(e.stderr, format+"\n", args...)
	}
}

// debug prints a line only with -v.
func (e *env) debug(format string, args ...any) {
	if e.g.verbose {
		fmt.Fprintf(e.stderr, format+"\n", args...)
	}
}
