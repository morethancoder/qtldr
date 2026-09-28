package editor

import (
	"errors"
	"strings"
	"testing"

	"github.com/morethancoder/qtldr/internal/config"
)

func TestCommand(t *testing.T) {
	target := Target{File: "/src/my app/tier.go", Line: 29, Col: 5}
	cases := []struct {
		ed   config.Editor
		want string
		err  bool
	}{
		{config.Editor{Preset: "vscode"}, "code|-g|/src/my app/tier.go:29:5", false},
		{config.Editor{Preset: "cursor"}, "cursor|-g|/src/my app/tier.go:29:5", false},
		{config.Editor{Preset: "zed"}, "zed|/src/my app/tier.go:29:5", false},
		{config.Editor{Preset: "goland"}, "goland|--line|29|--column|5|/src/my app/tier.go", false},
		{config.Editor{Preset: "nvim-remote", NvimSocket: "/tmp/nvim.sock"}, `nvim|--server|/tmp/nvim.sock|--remote-send|<C-\><C-N>:e +29 /src/my\ app/tier.go<CR>`, false},
		{config.Editor{Preset: "nvim-remote"}, "", true},
		{config.Editor{Preset: "vim-tmux", Terminal: "tmux new-window"}, "tmux|new-window|vim|+29|/src/my app/tier.go", false},
		{config.Editor{Preset: "vim-tmux"}, "", true},
		{config.Editor{Preset: "custom", Command: `nvim "+{line}" {file}`}, "nvim|+29|/src/my app/tier.go", false},
		{config.Editor{Preset: "custom", Command: `nvim "+{line}`}, "", true},
		{config.Editor{Preset: "emacs"}, "", true},
	}
	for _, c := range cases {
		got, err := Command(c.ed, target)
		if (err != nil) != c.err || (!c.err && strings.Join(got, "|") != c.want) {
			t.Errorf("%s: got %q %v, want %q", c.ed.Preset, strings.Join(got, "|"), err, c.want)
		}
	}
	got, _ := Command(config.Editor{Preset: "zed"}, Target{File: "/a.go"})
	if got[1] != "/a.go:1:1" {
		t.Errorf("line and column default to 1: %v", got)
	}
}

func TestAvailable(t *testing.T) {
	onPath := map[string]bool{"zed": true, "nvim": true, "goland": true, "cursor": true}
	look := func(b string) (string, error) {
		if onPath[b] {
			return "/bin/" + b, nil
		}
		return "", errors.New("no")
	}
	got := Available(config.Editor{Preset: "vscode"}, look)
	ids := []string{}
	for _, p := range got {
		ids = append(ids, p.ID)
	}
	if strings.Join(ids, ",") != "vscode,cursor,zed" {
		t.Errorf("got %v", ids)
	}
	if got := Available(config.Editor{Preset: "custom", Command: "x"}, look); got[0].ID != "custom" || len(got) != 3 {
		t.Errorf("custom first: %v", got)
	}
}

func TestResolve(t *testing.T) {
	t.Setenv("NVIM", "/run/nvim.sock")
	if Resolve(config.Editor{}).NvimSocket != "/run/nvim.sock" || Resolve(config.Editor{NvimSocket: "x"}).NvimSocket != "x" {
		t.Error("NVIM fallback")
	}
}
