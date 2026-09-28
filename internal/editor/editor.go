// Package editor turns the [editor] config into an argv for "open in
// editor" (flags verified per preset, docs/decisions.md #9) and runs it
// without a shell.
package editor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/morethancoder/qtldr/internal/argv"
	"github.com/morethancoder/qtldr/internal/config"
)

// Preset is one known editor.
type Preset struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Bin   string   `json:"bin"`
	Args  []string `json:"-"`
}

// Presets are the built-in editors, in the order the UI offers them.
var Presets = []Preset{
	{ID: "vscode", Label: "VS Code", Bin: "code", Args: []string{"code", "-g", "{file}:{line}:{col}"}},
	{ID: "cursor", Label: "Cursor", Bin: "cursor", Args: []string{"cursor", "-g", "{file}:{line}:{col}"}},
	{ID: "zed", Label: "Zed", Bin: "zed", Args: []string{"zed", "{file}:{line}:{col}"}},
	{ID: "goland", Label: "GoLand", Bin: "goland", Args: []string{"goland", "--line", "{line}", "--column", "{col}", "{file}"}},
	{ID: "nvim-remote", Label: "Neovim", Bin: "nvim", Args: []string{"nvim", "--server", "{socket}", "--remote-send", `<C-\><C-N>:e +{line} {vimfile}<CR>`}},
	{ID: "vim-tmux", Label: "Vim (tmux)", Bin: "vim", Args: []string{"{terminal}", "vim", "+{line}", "{file}"}},
}

// Target is where to open: an absolute file path, 1-based line and column.
type Target struct {
	File string
	Line int
	Col  int
}

// Command builds the argv for the configured editor (pure).
func Command(ed config.Editor, t Target) ([]string, error) {
	if t.Line < 1 {
		t.Line = 1
	}
	if t.Col < 1 {
		t.Col = 1
	}
	tmpl, err := template(ed)
	if err != nil {
		return nil, err
	}
	vars := map[string]string{
		"file": t.File, "line": strconv.Itoa(t.Line), "col": strconv.Itoa(t.Col),
		"socket": ed.NvimSocket, "vimfile": vimEscape(t.File),
	}
	return expandTerminal(argv.Fill(tmpl, vars), ed.Terminal)
}

func template(ed config.Editor) ([]string, error) {
	if ed.Preset == "custom" {
		return argv.Split(ed.Command)
	}
	for _, p := range Presets {
		if p.ID == ed.Preset {
			if p.ID == "nvim-remote" && ed.NvimSocket == "" {
				return nil, errors.New(`[editor] preset "nvim-remote" needs nvim_socket (or $NVIM, set inside a Neovim terminal)`)
			}
			return p.Args, nil
		}
	}
	return nil, fmt.Errorf("unknown [editor].preset %q", ed.Preset)
}

// expandTerminal replaces the {terminal} word with the words of
// [editor].terminal (vim-tmux).
func expandTerminal(args []string, terminal string) ([]string, error) {
	if len(args) == 0 || args[0] != "{terminal}" {
		return args, nil
	}
	words, err := argv.Split(terminal)
	if err != nil {
		return nil, errors.New(`[editor] preset "vim-tmux" needs terminal, e.g. "tmux new-window"`)
	}
	return append(words, args[1:]...), nil
}

// vimEscape escapes characters special in a Vim :e argument.
func vimEscape(p string) string {
	return strings.NewReplacer(`\`, `\\`, " ", `\ `, "%", `\%`, "#", `\#`, "|", `\|`).Replace(p)
}

// Resolve fills defaults from the environment: nvim_socket from $NVIM.
func Resolve(ed config.Editor) config.Editor {
	if ed.NvimSocket == "" {
		ed.NvimSocket = os.Getenv("NVIM")
	}
	return ed
}

// Available lists the configured editor first, then up to two other
// presets whose binary is on PATH.
func Available(ed config.Editor, lookPath func(string) (string, error)) []Preset {
	out := configured(ed)
	for _, p := range Presets {
		if len(out) >= 3 {
			break
		}
		if detectable(p, ed) && onPath(lookPath, p.Bin) {
			out = append(out, p)
		}
	}
	return out
}

func configured(ed config.Editor) []Preset {
	if ed.Preset == "custom" {
		return []Preset{{ID: "custom", Label: "Editor"}}
	}
	for _, p := range Presets {
		if p.ID == ed.Preset {
			return []Preset{p}
		}
	}
	return nil
}

// detectable: another GUI preset (terminal presets need extra config).
func detectable(p Preset, ed config.Editor) bool {
	return p.ID != ed.Preset && p.ID != "vim-tmux" && p.ID != "nvim-remote"
}

func onPath(lookPath func(string) (string, error), bin string) bool {
	_, err := lookPath(bin)
	return err == nil
}

// Open runs the editor from root. The error names the exact command.
func Open(ctx context.Context, root string, ed config.Editor, t Target) ([]string, error) {
	if !filepath.IsAbs(t.File) {
		t.File = filepath.Join(root, t.File)
	}
	args, err := Command(Resolve(ed), t)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return args, fmt.Errorf("`%s` failed: %v %s; check [editor] in .qtldr.toml", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return args, nil
}
