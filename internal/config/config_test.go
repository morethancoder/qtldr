package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	c := Default()
	if c.Thresholds.CrapMax != 8 || c.Thresholds.CognitiveMax != 15 || c.Mutation.MaxFunctions != 40 {
		t.Fatalf("thresholds: %+v, mutation: %+v", c.Thresholds, c.Mutation)
	}
	if c.Coverage.Timeout.Duration != 10*time.Minute || c.Editor.Preset != "vscode" {
		t.Fatalf("coverage timeout %v, preset %q", c.Coverage.Timeout, c.Editor.Preset)
	}
	if len(c.Project.Include) != 1 || c.Project.Include[0] != "./..." {
		t.Fatalf("include %v", c.Project.Include)
	}
}

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	c, warns, err := Load(filepath.Join(t.TempDir(), FileName))
	if err != nil || len(warns) != 0 || c.Project.BaseRef != "main" {
		t.Fatalf("got %+v %v %v", c.Project, warns, err)
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		name, text string
		wantErr    string
		wantWarn   string
		check      func(Config) bool
	}{
		{name: "override", text: "[thresholds]\ncrap_max = 12\n", check: func(c Config) bool {
			return c.Thresholds.CrapMax == 12 && c.Thresholds.CognitiveMax == 15
		}},
		{name: "unknown table", text: "[nope]\nx = 1\n", wantErr: `unknown table or key "nope"`},
		{name: "unknown top-level key", text: "x = 1\n", wantErr: `unknown table or key "x"`},
		{name: "unknown key", text: "[project]\nfoo = 1\n", wantWarn: `unknown key "foo" in [project]`},
		{name: "bad enum", text: "[coverage]\ncoverpkg = \"all\"\n", wantErr: "coverage.coverpkg"},
		{name: "bad duration", text: "[mutation]\ntimeout = \"soon\"\n", wantErr: "not a duration"},
		{name: "custom editor without command", text: "[editor]\npreset = \"custom\"\n", wantErr: "needs editor.command"},
		{name: "custom editor", text: "[editor]\npreset = \"custom\"\ncommand = \"vi +{line} {file}\"\n", check: func(c Config) bool {
			return c.Editor.Command != ""
		}},
		{name: "syntax error", text: "[project\n", wantErr: "test.toml"},
		{name: "provider without extensions", text: "[[providers]]\nname = \"py\"\ncommand = \"x\"\n", wantErr: "providers[0] needs name, command and extensions"},
		{name: "provider", text: "[[providers]]\nname = \"py\"\ncommand = \"x\"\nextensions = [\".py\"]\n", check: func(c Config) bool {
			return len(c.Providers) == 1 && c.Providers[0].Extensions[0] == ".py"
		}},
		{name: "bad calls", text: "[project]\ncalls = \"magic\"\n", wantErr: "project.calls"},
	}
	for _, c := range cases {
		cfg, warns, err := Parse("test.toml", c.text)
		switch {
		case c.wantErr != "":
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("%s: err = %v, want %q", c.name, err, c.wantErr)
			}
		case err != nil:
			t.Errorf("%s: unexpected error %v", c.name, err)
		case c.wantWarn != "" && (len(warns) != 1 || !strings.Contains(warns[0], c.wantWarn)):
			t.Errorf("%s: warnings %v, want %q", c.name, warns, c.wantWarn)
		case c.check != nil && !c.check(cfg):
			t.Errorf("%s: check failed on %+v", c.name, cfg)
		}
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := Load(dir); err == nil || !strings.HasPrefix(err.Error(), "read "+dir) {
		t.Errorf("a directory is not a config file: %v", err)
	}
	p := filepath.Join(dir, FileName)
	if err := os.WriteFile(p, []byte("[ui]\ntheme = \"nord\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, warns, err := Load(p)
	if err != nil || len(warns) != 0 || c.UI.Theme != "nord" || c.Thresholds.CrapMax != Default().Thresholds.CrapMax {
		t.Errorf("load: %+v %q %v", c.UI, warns, err)
	}
}

func TestTrailingComment(t *testing.T) {
	cases := []struct{ in, want string }{
		{`theme = "a" # c`, " # c"},
		{`theme = "a#b"`, ""},
		{"# only a comment", "# only a comment"},
		{"  # indented comment", "  # indented comment"},
		{`x = "a"` + "\t\t# tabs", "\t\t# tabs"},
	}
	for _, c := range cases {
		if got := trailingComment(c.in); got != c.want {
			t.Errorf("trailingComment(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSetString(t *testing.T) {
	cases := []struct{ in, want string }{
		{DefaultTOML, strings.Replace(DefaultTOML, `theme = "gruvbox-dark"                     # id`, `theme = "nord"                     # id`, 1)},
		{"[ui]\nport = 0\n", "[ui]\ntheme = \"nord\"\nport = 0\n"},
		{"[project]\nbase_ref = \"main\"", "[project]\nbase_ref = \"main\"\n\n[ui]\ntheme = \"nord\"\n"},
		{"", "\n[ui]\ntheme = \"nord\"\n"},
		{"[ui]\ntheme = \"a#b\" # c\n[editor]\ntheme = \"x\"\n", "[ui]\ntheme = \"nord\" # c\n[editor]\ntheme = \"x\"\n"},
		{"[ui]\ntheme = \"a\"\t# tab\n", "[ui]\ntheme = \"nord\"\t# tab\n"},
		{"[ui]\ntheme = \"a\"\n", "[ui]\ntheme = \"nord\"\n"},
	}
	for _, c := range cases {
		got := SetString(c.in, "ui", "theme", "nord")
		if got != c.want {
			t.Errorf("SetString(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
		if _, _, err := Parse("x", got); err != nil && c.in == DefaultTOML {
			t.Errorf("result does not parse: %v", err)
		}
	}
}
