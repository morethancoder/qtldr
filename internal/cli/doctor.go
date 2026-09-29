package cli

import (
	"flag"
	"fmt"
	"runtime"
	"strings"

	"github.com/morethancoder/qtldr/internal/config"
)

// toolCheck is one line of the doctor report.
type toolCheck struct {
	Name     string `json:"name"`
	Found    bool   `json:"found"`
	Required bool   `json:"required"`
	Version  string `json:"version,omitempty"`
	Hint     string `json:"hint,omitempty"`
}

// editorBinary is the executable each editor preset runs.
var editorBinary = map[string]string{
	"vscode": "code", "cursor": "cursor", "zed": "zed", "goland": "goland",
	"nvim-remote": "nvim", "vim-tmux": "vim",
}

func runDoctor(e *env, _ *flag.FlagSet, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("%w: doctor takes no arguments", errUsage)
	}
	return doctor(e, e.editorConfig())
}

// editorConfig reads the editor settings, falling back to defaults when
// there is no module or config yet.
func (e *env) editorConfig() config.Editor {
	root, err := e.moduleRoot()
	if err != nil {
		return config.Default().Editor
	}
	cfg, err := e.loadConfig(root)
	if err != nil {
		fmt.Fprintln(e.stderr, "warning:", err)
		return config.Default().Editor
	}
	return cfg.Editor
}

func doctor(e *env, ed config.Editor) error {
	checks := []toolCheck{
		e.checkTool("go", true, "install Go from https://go.dev/dl", "version"),
		e.checkTool("git", true, "install git; qtldr needs it for --changed and churn", "--version"),
		e.checkTool("gremlins", false, "optional, needed for qtldr mutate: go install github.com/go-gremlins/gremlins/cmd/gremlins@latest", "--version"),
		e.checkEditor(ed),
	}
	if e.g.json {
		if err := e.printJSON(checks); err != nil {
			return err
		}
	} else {
		printChecks(e, checks)
	}
	for _, c := range checks {
		if c.Required && !c.Found {
			return exitCode(ExitError)
		}
	}
	return nil
}

func (e *env) checkTool(name string, required bool, hint string, versionArgs ...string) toolCheck {
	c := toolCheck{Name: name, Required: required}
	if _, err := e.sys.LookPath(name); err != nil {
		c.Hint = hint
		return c
	}
	c.Found = true
	out, err := e.sys.Output(e.ctx, name, versionArgs...)
	if err == nil {
		c.Version, _, _ = strings.Cut(out, "\n")
	}
	return c
}

func (e *env) checkEditor(ed config.Editor) toolCheck {
	bin := editorBinary[ed.Preset]
	if ed.Preset == "custom" {
		bin, _, _ = strings.Cut(strings.TrimSpace(ed.Command), " ")
	}
	c := toolCheck{Name: "editor (" + ed.Preset + ": " + bin + ")"}
	if _, err := e.sys.LookPath(bin); err != nil {
		c.Hint = fmt.Sprintf("%q is not on PATH; set [editor] preset or command in .qtldr.toml", bin)
		return c
	}
	c.Found, c.Hint = true, editorWarning(runtime.GOOS, ed.Preset)
	return c
}

// editorWarning says when an editor preset may not work on goos.
func editorWarning(goos, preset string) string {
	if goos == "windows" && (preset == "vim-tmux" || preset == "nvim-remote") {
		return preset + " may not work on Windows"
	}
	return ""
}

func printChecks(e *env, checks []toolCheck) {
	for _, c := range checks {
		mark, detail := "✓", c.Version
		if !c.Found {
			mark, detail = "✗", "not found"
			if !c.Required {
				mark = "–"
			}
		}
		fmt.Fprintf(e.stdout, "%s %-28s %s\n", mark, c.Name, detail)
		if c.Hint != "" {
			fmt.Fprintf(e.stdout, "  %s\n", c.Hint)
		}
	}
}
