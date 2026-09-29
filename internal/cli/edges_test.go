package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/morethancoder/qtldr/internal/metrics"
	"github.com/morethancoder/qtldr/internal/model"
)

func TestInitMessages(t *testing.T) {
	dir := tempModule(t)
	r := runCLI(t, allTools, "-C", dir, "init")
	root, _ := filepath.EvalSymlinks(dir)
	for _, want := range []string{"wrote " + filepath.Join(root, ".qtldr.toml"), "added 4 rules to " + filepath.Join(root, ".gitignore")} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("missing %q in stderr:\n%s", want, r.stderr)
		}
	}
	if !strings.Contains(r.stdout, "✓ go") {
		t.Errorf("init runs doctor last:\n%s", r.stdout)
	}
	r = runCLI(t, allTools, "-C", dir, "init")
	if !strings.Contains(r.stderr, "kept existing") || strings.Contains(r.stderr, "added") {
		t.Errorf("second init:\n%s", r.stderr)
	}
}

func TestChurnLabel(t *testing.T) {
	if churnLabel("function") != "Churn (approx.)" || churnLabel("file") != "Churn (file)" {
		t.Error(churnLabel("function"), churnLabel("file"))
	}
}

func TestDoctorJSON(t *testing.T) {
	r := runCLI(t, allTools, "-C", t.TempDir(), "--json", "doctor")
	if r.code != 0 || !strings.Contains(r.stdout, `"version": "go version go1.27.0"`) {
		t.Errorf("versions: %+v", r)
	}
	r = runCLI(t, fakeSystem{found: map[string]string{"git": ""}}, "-C", t.TempDir(), "--json", "doctor")
	if r.code != 2 || !strings.Contains(r.stdout, `"name": "go"`) {
		t.Errorf("go missing: %+v", r)
	}
}

func TestEditorWarning(t *testing.T) {
	cases := []struct{ goos, preset, want string }{
		{"windows", "vim-tmux", "vim-tmux may not work on Windows"},
		{"windows", "nvim-remote", "nvim-remote may not work on Windows"},
		{"windows", "vscode", ""},
		{"darwin", "vim-tmux", ""},
		{"linux", "nvim-remote", ""},
	}
	for _, c := range cases {
		if got := editorWarning(c.goos, c.preset); got != c.want {
			t.Errorf("editorWarning(%q, %q) = %q, want %q", c.goos, c.preset, got, c.want)
		}
	}
}

func TestEditorConfigFromFile(t *testing.T) {
	dir := tempModule(t)
	if err := os.WriteFile(filepath.Join(dir, ".qtldr.toml"), []byte("[editor]\npreset = \"zed\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sys := fakeSystem{found: map[string]string{"go": "", "git": "", "zed": ""}}
	if r := runCLI(t, sys, "-C", dir, "doctor"); !strings.Contains(r.stdout, "editor (zed: zed)") || r.stderr != "" {
		t.Errorf("configured editor: %+v", r)
	}
	if err := os.WriteFile(filepath.Join(dir, ".qtldr.toml"), []byte("[editor]\npreset = 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := runCLI(t, sys, "-C", dir, "doctor"); !strings.Contains(r.stderr, "warning:") || !strings.Contains(r.stdout, "editor (vscode: code)") {
		t.Errorf("broken config falls back to defaults: %+v", r)
	}
}

type fakeWatcher struct{ err error }

func (w fakeWatcher) Watch(context.Context) error { return w.err }

func TestStartExtrasAndWatch(t *testing.T) {
	var errb bytes.Buffer
	e := &env{ctx: context.Background(), stdout: &bytes.Buffer{}, stderr: &errb, sys: fakeSystem{found: map[string]string{"open": "", "xdg-open": ""}}}
	e.startExtras(nil, "http://127.0.0.1:1", false, true)
	e.watch(fakeWatcher{})
	if errb.String() != "" {
		t.Errorf("browser opened and watcher stopped cleanly, yet: %q", errb.String())
	}
	e.sys = fakeSystem{}
	e.startExtras(nil, "http://127.0.0.1:1", false, true)
	e.watch(fakeWatcher{errors.New("too many files")})
	for _, want := range []string{"could not open a browser", "open http://127.0.0.1:1 yourself", "warning: --watch stopped: too many files"} {
		if !strings.Contains(errb.String(), want) {
			t.Errorf("missing %q in %q", want, errb.String())
		}
	}
}

func TestFindModuleRootResolvesSymlinks(t *testing.T) {
	real := tempModule(t)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}
	want, _ := filepath.EvalSymlinks(real)
	if got, err := findModuleRoot(filepath.Join(link, "a")); err != nil || got != want {
		t.Errorf("got %q %v, want %q", got, err, want)
	}
}

func TestLogHookError(t *testing.T) {
	var errb bytes.Buffer
	e := &env{stderr: &errb}
	cwd := t.TempDir()
	t.Chdir(cwd)
	logHookError(e, "", errors.New("boom"))
	if errb.String() != "qtldr hook: boom\n" {
		t.Errorf("no root: %q", errb.String())
	}
	if _, err := os.Stat(filepath.Join(cwd, ".qtldr")); err == nil {
		t.Error("no root must not write a log in the working directory")
	}

	root := t.TempDir()
	errb.Reset()
	logHookError(e, root, errors.New("boom"))
	logs, _ := filepath.Glob(filepath.Join(root, ".qtldr", "logs", "hook-*.log"))
	if len(logs) != 1 || errb.String() != "qtldr hook: boom\n" {
		t.Fatalf("logs %v, stderr %q", logs, errb.String())
	}
	if b, _ := os.ReadFile(logs[0]); string(b) != "boom\n" {
		t.Errorf("log content %q", b)
	}

	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	errb.Reset()
	logHookError(e, file, errors.New("boom"))
	if !strings.Contains(errb.String(), "also could not write the log") {
		t.Errorf("unwritable log: %q", errb.String())
	}
}

func TestNoteTarget(t *testing.T) {
	snap := model.Snapshot{Graph: model.Graph{Nodes: []model.Node{{ID: "m/p.Big", Kind: model.KindFunc}}}}
	if id, err := noteTarget(snap, nil, []string{"Big"}); err != nil || id != "m/p.Big" {
		t.Errorf("resolved: %q %v", id, err)
	}
	if id, err := noteTarget(snap, nil, nil); err != nil || id != "" {
		t.Errorf("no argument: %q %v", id, err)
	}
	if _, err := noteTarget(snap, errors.New("no snapshot"), []string{"Big"}); err == nil || err.Error() != "no snapshot" {
		t.Errorf("snapshot error: %v", err)
	}
}

func TestParseInterspersedDoubleDash(t *testing.T) {
	var g globals
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	g.register(fs)
	pos, err := parseInterspersed(fs, []string{"--", "a", "--json"})
	if err != nil || !slices.Equal(pos, []string{"a", "--json"}) || g.json {
		t.Errorf("everything after -- is positional: %q %v json=%v", pos, err, g.json)
	}
}

func TestPrintCoverageLines(t *testing.T) {
	n := model.Node{Kind: model.KindFunc, File: "p/f.go"}
	cases := []struct {
		lines *model.LineStates
		want  string
	}{
		{&model.LineStates{Uncovered: []int{4, 5}}, "\nNot covered: f.go:4–5\n"},
		{&model.LineStates{Partial: []int{3}}, "Partly covered: f.go:3\n"},
		{&model.LineStates{}, ""},
	}
	for _, c := range cases {
		var b bytes.Buffer
		printCoverageLines(&b, n, &model.Coverage{Lines: c.lines})
		if b.String() != c.want {
			t.Errorf("%+v: got %q, want %q", c.lines, b.String(), c.want)
		}
	}
}

func TestPrintDetailOmitsEmptyParts(t *testing.T) {
	var b bytes.Buffer
	printDetail(&b, model.Detail{Node: model.Node{ID: "m/p", Name: "p", Kind: model.KindPackage}})
	out := b.String()
	if strings.Contains(out, "effectful because") || strings.Contains(out, ":0") {
		t.Errorf("no file and no effects:\n%s", out)
	}
	b.Reset()
	printDetail(&b, model.Detail{Node: model.Node{ID: "m/p.f", Name: "f", Kind: model.KindFunc, File: "p/f.go", Line: 3, EndLine: 9, Effects: []string{"calls os.Getenv"}}})
	if out := b.String(); !strings.Contains(out, "p/f.go:3–9\n") || !strings.Contains(out, "effectful because: calls os.Getenv\n") {
		t.Errorf("file and effects:\n%s", out)
	}
}

func TestPrintRanking(t *testing.T) {
	var out bytes.Buffer
	e := &env{stdout: &out}
	printRanking(e, metrics.Ranking{Metric: metrics.CRAPMetric, Items: []metrics.Ranked{{ID: "m/p.f", Value: 9}, {ID: "m/p.g", Value: 3, Stale: true}}})
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[1], "  1. p.f") || !strings.HasPrefix(lines[2], "  2. p.g") || !strings.HasSuffix(lines[2], "(stale)") {
		t.Errorf("ranking:\n%s", out.String())
	}
	out.Reset()
	printRanking(e, metrics.Ranking{Metric: metrics.CRAPMetric, Items: []metrics.Ranked{{ID: "m/p.f", Value: 9}}, NotMeasured: 2})
	if !strings.Contains(out.String(), "2 functions not measured.") {
		t.Errorf("not measured:\n%s", out.String())
	}
}

func TestPurityLabel(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		n    model.Node
		want string
	}{
		{model.Node{Kind: model.KindFunc}, ""},
		{model.Node{Kind: model.KindPackage, Pure: &yes}, " · λ pure core"},
		{model.Node{Kind: model.KindFunc, Pure: &yes}, " · λ pure"},
		{model.Node{Kind: model.KindPackage, Pure: &no}, " · effectful shell"},
		{model.Node{Kind: model.KindFunc, Pure: &no}, " · effectful"},
	}
	for _, c := range cases {
		if got := purityLabel(c.n); got != c.want {
			t.Errorf("%+v: %q, want %q", c.n, got, c.want)
		}
	}
}

func TestExplainUsesModuleConfig(t *testing.T) {
	dir := tempModule(t)
	if err := os.WriteFile(filepath.Join(dir, ".qtldr.toml"), []byte("[thresholds]\ncrap_max = 6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := runCLI(t, allTools, "-C", dir, "explain", "crap"); !strings.Contains(r.stdout, "qtldr target: 6 or less") {
		t.Errorf("configured threshold: %+v", r)
	}
}

func TestMCPOutsideModule(t *testing.T) {
	if r := runCLI(t, allTools, "-C", t.TempDir(), "mcp"); r.code != 2 || !strings.Contains(r.stderr, "no go.mod") {
		t.Errorf("%+v", r)
	}
}

func TestMCPStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errb bytes.Buffer
	e := &env{ctx: ctx, stdin: strings.NewReader(""), stdout: &out, stderr: &errb, sys: allTools}
	run(e, []string{"-C", tempModule(t), "mcp"})
	if strings.Contains(errb.String(), "takes no arguments") {
		t.Errorf("no arguments given: %q", errb.String())
	}
}

func TestServePort(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	serveFlags(fs)
	if got := servePort(fs, 7777); got != 7777 {
		t.Errorf("default: %d, want the configured port", got)
	}
	if err := fs.Parse([]string{"--port", "0"}); err != nil {
		t.Fatal(err)
	}
	if got := servePort(fs, 7777); got != 0 {
		t.Errorf("--port 0: %d", got)
	}
}

func TestUsageIsSorted(t *testing.T) {
	var b bytes.Buffer
	usage(&b)
	var names []string
	for _, l := range strings.Split(b.String(), "\n") {
		if strings.HasPrefix(l, "  ") {
			names = append(names, strings.Fields(l)[0])
		}
	}
	if len(names) != len(commands()) || !slices.IsSorted(names) {
		t.Errorf("commands %v", names)
	}
}
