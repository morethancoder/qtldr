package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/morethancoder/qtldr/internal/analysis"
	"github.com/morethancoder/qtldr/internal/check"
	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/store"
)

func checkFlags(fs *flag.FlagSet) {
	fs.Bool("changed", false, "functions changed since [project].base_ref, plus untracked files (default)")
	fs.Bool("all", false, "every function")
	fs.Bool("fast", false, "no test run: check cognitive, and CRAP where cached coverage is current")
	fs.Bool("files-from-stdin", false, "read a Claude Code hook payload on stdin and check the edited files")
}

// checkRequest is a parsed check invocation.
type checkRequest struct {
	all, fast bool
	targets   []string
}

func runCheck(e *env, fs *flag.FlagSet, args []string) error {
	req := checkRequest{all: boolFlag(fs, "all"), fast: boolFlag(fs, "fast"), targets: args}
	if boolFlag(fs, "files-from-stdin") {
		return runHookCheck(e, req)
	}
	if req.all && (len(args) > 0 || boolFlag(fs, "changed")) {
		return fmt.Errorf("%w: use one of --changed, --all, or IDs/files", errUsage)
	}
	root, cfg, err := e.setup()
	if err != nil {
		return err
	}
	report, err := e.check(root, cfg, req)
	if err != nil {
		return err
	}
	return e.printReport(report, e.stdout)
}

// check runs the analysis for the request's scope and evaluates it.
func (e *env) check(root string, cfg config.Config, req checkRequest) (check.Report, error) {
	scope, name, base := e.scopeFor(root, cfg, req)
	res, err := analysis.Run(e.ctx, analysis.Options{
		Root: root, Config: cfg, Coverage: !req.fast, Scope: scope, Progress: e.progressLine,
	})
	if err != nil {
		return check.Report{}, err
	}
	r := check.Evaluate(res.Snapshot.Graph, res.Scope, cfg.Thresholds, req.fast)
	r.Scope, r.Base = name, base
	return r, nil
}

func (e *env) scopeFor(root string, cfg config.Config, req checkRequest) (func(model.Graph) ([]model.ID, error), string, string) {
	switch {
	case req.all:
		return func(g model.Graph) ([]model.ID, error) { return check.All(g), nil }, "all", ""
	case len(req.targets) > 0:
		return func(g model.Graph) ([]model.ID, error) { return e.resolveTargets(g, root, req.targets) }, "explicit", ""
	}
	return changedScope(e, root, cfg.Project.BaseRef), "changed", cfg.Project.BaseRef
}

// resolveTargets turns IDs and file paths into function IDs.
func (e *env) resolveTargets(g model.Graph, root string, targets []string) ([]model.ID, error) {
	cwd, _ := e.startDir()
	var ids, files []model.ID
	var paths []string
	for _, t := range targets {
		if strings.HasSuffix(t, ".go") || fileExists(filepath.Join(cwd, t)) {
			rel, ok := analysis.RelFile(root, cwd, t)
			if !ok {
				return nil, fmt.Errorf("%s is outside the module at %s", t, root)
			}
			paths = append(paths, rel)
			continue
		}
		id, err := model.Resolve(g.IDs(), t)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	files = check.InFiles(g, paths)
	return append(check.Expand(g, ids), files...), nil
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// printReport prints Markdown or JSON; with --quiet a passing report prints
// nothing. A report with breaches returns exit code 1.
func (e *env) printReport(r check.Report, w io.Writer) error {
	switch {
	case e.g.json:
		if err := json.NewEncoder(w).Encode(r); err != nil {
			return err
		}
	case !(e.g.quiet && r.Pass):
		fmt.Fprint(w, check.Markdown(r))
	}
	if !r.Pass {
		return exitCode(ExitBreach)
	}
	return nil
}

// hookPayload is the part of a Claude Code PostToolUse payload qtldr reads
// (captured in testdata/claude-hook/).
type hookPayload struct {
	Cwd       string `json:"cwd"`
	ToolInput struct {
		FilePath string `json:"file_path"`
		Edits    []struct {
			FilePath string `json:"file_path"`
		} `json:"edits"`
	} `json:"tool_input"`
}

// Files returns the edited file paths.
func (p hookPayload) Files() []string {
	var files []string
	if p.ToolInput.FilePath != "" {
		files = append(files, p.ToolInput.FilePath)
	}
	for _, ed := range p.ToolInput.Edits {
		if ed.FilePath != "" {
			files = append(files, ed.FilePath)
		}
	}
	return files
}

// runHookCheck is `check --files-from-stdin`, run by the Claude Code hook.
// Breaches go to stderr with exit 2, which Claude Code shows to the model
// (exit 1 would only show the user a notice; see docs/decisions.md #10).
// Runtime errors exit 0 so a broken tool never blocks the agent; they are
// logged under .qtldr/logs/.
func runHookCheck(e *env, req checkRequest) error {
	root, r, err := e.hookCheck(req)
	if err != nil {
		logHookError(e, root, err)
		return nil
	}
	if r.Pass && len(r.Functions) == 0 {
		return nil
	}
	if printErr := e.printReport(r, e.stderr); printErr != nil {
		var code exitCode
		if errors.As(printErr, &code) && code == ExitBreach {
			return exitCode(ExitError) // exit 2: Claude Code feeds stderr to the model
		}
		return printErr
	}
	return nil
}

func (e *env) hookCheck(req checkRequest) (string, check.Report, error) {
	var p hookPayload
	if err := json.NewDecoder(e.stdin).Decode(&p); err != nil {
		return "", check.Report{}, fmt.Errorf("read hook payload from stdin: %w", err)
	}
	goFiles := goFilesOf(p.Files())
	if len(goFiles) == 0 {
		return "", check.Report{Pass: true}, nil
	}
	root, err := e.hookRoot(goFiles[0])
	if err != nil {
		return "", check.Report{}, err
	}
	cfg, err := e.loadConfig(root)
	if err != nil {
		return root, check.Report{}, err
	}
	req.targets = goFiles
	e.g.dir = root
	r, err := e.check(root, cfg, req)
	r.Scope = "hook"
	return root, r, err
}

// hookRoot is -C if given, else the module containing the edited file.
func (e *env) hookRoot(file string) (string, error) {
	if e.g.dir != "" {
		return e.moduleRoot()
	}
	return findModuleRoot(filepath.Dir(file))
}

func goFilesOf(files []string) []string {
	var out []string
	for _, f := range files {
		if strings.HasSuffix(f, ".go") && !strings.HasSuffix(f, "_test.go") {
			out = append(out, f)
		}
	}
	return out
}

func logHookError(e *env, root string, err error) {
	fmt.Fprintln(e.stderr, "qtldr hook:", err)
	if root == "" {
		return
	}
	if _, lerr := store.WriteLog(root, "hook", time.Now(), []byte(err.Error()+"\n")); lerr != nil {
		fmt.Fprintln(e.stderr, "qtldr hook: also could not write the log:", lerr)
	}
}
