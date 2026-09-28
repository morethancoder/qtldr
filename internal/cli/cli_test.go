package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/mutate"
)

// fakeSystem finds only the tools in found.
type fakeSystem struct{ found map[string]string }

func (f fakeSystem) LookPath(name string) (string, error) {
	if _, ok := f.found[name]; ok {
		return "/bin/" + name, nil
	}
	return "", errors.New("not found")
}

func (f fakeSystem) Start(args ...string) error {
	if _, ok := f.found[args[0]]; ok {
		return nil
	}
	return errors.New("not found")
}

func (f fakeSystem) Output(_ context.Context, name string, _ ...string) (string, error) {
	return f.found[name], nil
}

var allTools = fakeSystem{found: map[string]string{"go": "go version go1.27.0", "git": "git version 2.43.0", "code": "1.0"}}

type result struct {
	code           int
	stdout, stderr string
}

func runCLI(t *testing.T, sys system, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	e := &env{ctx: context.Background(), stdout: &out, stderr: &errb, sys: sys}
	code := run(e, args)
	return result{code, out.String(), errb.String()}
}

// tempModule writes a small module and returns its directory.
func tempModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/m\n\ngo 1.26.0\n",
		"a/a.go": "package a\n\nimport \"example.com/m/b\"\n\n// Big branches.\nfunc Big(x int) int {\n\tif x > 0 && x < 10 {\n\t\treturn b.Close(x)\n\t}\n\treturn 0\n}\n\nfunc Close() {}\n",
		"b/b.go": "package b\n\n// Close is not ambiguous with a.Close only by package.\nfunc Close(x int) int { return x }\n",
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestAnalyzeShowWorst(t *testing.T) {
	dir := tempModule(t)
	if r := runCLI(t, allTools, "-C", dir, "analyze"); r.code != 0 || !strings.Contains(r.stdout, "2 packages, 3 functions") {
		t.Fatalf("analyze: %+v", r)
	}
	cases := []struct {
		args     []string
		code     int
		contains string
	}{
		{[]string{"show", "a.Big"}, 0, "Cyclomatic (CC)  3"},
		{[]string{"show", "Big"}, 0, "Callees (1): b.Close"},
		{[]string{"show", "a.Big"}, 0, "Coverage         not measured"},
		{[]string{"show", "Close"}, 2, "matches 2 nodes"},
		{[]string{"show", "Nope"}, 2, "no node matches"},
		{[]string{"show"}, 2, "usage: qtldr show <id>"},
		{[]string{"worst", "--metric", "cognitive", "-n", "1"}, 0, "1. a.Big"},
		{[]string{"worst"}, 0, "Run: qtldr analyze --coverage"},
		{[]string{"worst", "--metric", "loc"}, 2, "unknown metric"},
	}
	for _, c := range cases {
		r := runCLI(t, allTools, append([]string{"-C", dir}, c.args...)...)
		if r.code != c.code || !strings.Contains(r.stdout+r.stderr, c.contains) {
			t.Errorf("%v: code %d, want %d containing %q\nstdout: %s\nstderr: %s", c.args, r.code, c.code, c.contains, r.stdout, r.stderr)
		}
	}
}

func TestFlagsAfterPositionalArgs(t *testing.T) {
	dir := tempModule(t)
	runCLI(t, allTools, "analyze", "-C", dir, "--quiet")
	r := runCLI(t, allTools, "show", "a.Big", "-C", dir, "--json")
	var d struct {
		Node    struct{ Name string }
		Metrics struct{ CC int }
	}
	if err := json.Unmarshal([]byte(r.stdout), &d); err != nil || d.Node.Name != "Big" || d.Metrics.CC != 3 {
		t.Fatalf("show --json: %v %+v %+v", err, d, r)
	}
}

func TestShowWithoutSnapshot(t *testing.T) {
	r := runCLI(t, allTools, "-C", tempModule(t), "show", "Big")
	if r.code != 2 || !strings.Contains(r.stderr, "run `qtldr analyze` first") {
		t.Fatalf("%+v", r)
	}
}

func TestNoModule(t *testing.T) {
	r := runCLI(t, allTools, "-C", t.TempDir(), "analyze")
	if r.code != 2 || !strings.Contains(r.stderr, "no go.mod") {
		t.Fatalf("%+v", r)
	}
}

func TestUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"nope"}, {"--bogus", "analyze"}} {
		if r := runCLI(t, allTools, args...); r.code != 2 || !strings.Contains(r.stderr, "commands:") {
			t.Errorf("%v: %+v", args, r)
		}
	}
}

func TestInit(t *testing.T) {
	dir := tempModule(t)
	gitignore := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("bin/\n.qtldr/logs/"), 0o644); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if r := runCLI(t, allTools, "-C", dir, "init"); r.code != 0 {
			t.Fatalf("init: %+v", r)
		}
	}
	cfg, err := os.ReadFile(filepath.Join(dir, config.FileName))
	if err != nil || string(cfg) != config.DefaultTOML {
		t.Fatalf("config: %v", err)
	}
	b, _ := os.ReadFile(gitignore)
	want := "bin/\n.qtldr/logs/\n# qtldr\n.qtldr/snapshot.json\n.qtldr/focus.json\n.qtldr/cache/\n"
	if string(b) != want {
		t.Fatalf(".gitignore:\n%s\nwant:\n%s", b, want)
	}
}

func TestDoctor(t *testing.T) {
	cases := []struct {
		name     string
		found    map[string]string
		code     int
		contains string
	}{
		{"all present", allTools.found, 0, "✓ go"},
		{"gremlins optional", allTools.found, 0, "– gremlins"},
		{"editor missing", map[string]string{"go": "", "git": ""}, 0, `"code" is not on PATH`},
		{"go missing", map[string]string{"git": ""}, 2, "✗ go"},
	}
	for _, c := range cases {
		r := runCLI(t, fakeSystem{found: c.found}, "-C", t.TempDir(), "doctor")
		if r.code != c.code || !strings.Contains(r.stdout, c.contains) {
			t.Errorf("%s: code %d, want %d containing %q\n%s", c.name, r.code, c.code, c.contains, r.stdout)
		}
	}
}

func TestMissingLines(t *testing.T) {
	got := missingLines("a\n  b  \n", []string{"a", "b", "c"})
	if len(got) != 1 || got[0] != "c" {
		t.Fatalf("got %v", got)
	}
}

// A module with one passing and one failing test package, for coverage runs.
func coverageModule(t *testing.T) string {
	t.Helper()
	dir := tempModule(t)
	files := map[string]string{
		"a/a_test.go": "package a\n\nimport \"testing\"\n\nfunc TestBig(t *testing.T) {\n\tif Big(5) != 5 {\n\t\tt.Fatal(\"Big\")\n\t}\n}\n",
		"b/b_test.go": "package b\n\nimport \"testing\"\n\nfunc TestClose(t *testing.T) { t.Fatal(\"always fails\") }\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestAnalyzeCoverage(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	dir := coverageModule(t)
	r := runCLI(t, allTools, "-C", dir, "analyze", "--coverage", "-v")
	if r.code != 0 || !strings.Contains(r.stderr, "tests failed in 1 packages") || !strings.Contains(r.stderr, "Running coverage… 2 packages") {
		t.Fatalf("%+v", r)
	}
	if r := runCLI(t, allTools, "-C", dir, "show", "a.Big"); !strings.Contains(r.stdout, "Coverage         66.7% (2 of 3 statements)") {
		t.Errorf("a.Big:\n%s", r.stdout)
	}
	if r := runCLI(t, allTools, "-C", dir, "show", "example.com/m/b"); !strings.Contains(r.stdout, "Coverage error   tests failed; output in .qtldr/logs/coverage-") {
		t.Errorf("package b:\n%s", r.stdout)
	}
	if r := runCLI(t, allTools, "-C", dir, "worst", "--metric", "coverage", "--json"); !strings.Contains(r.stdout, `"not_measured": 2`) {
		t.Errorf("worst coverage: %s", r.stdout)
	}
	if r := runCLI(t, allTools, "-C", dir, "check", "--all", "a.Big"); r.code != 2 {
		t.Errorf("--all with IDs: %+v", r)
	}
	if r := runCLI(t, allTools, "-C", dir, "analyze", "--coverage", "--changed"); r.code != 2 || !strings.Contains(r.stderr, "not a git repository") {
		t.Errorf("--changed outside git: %+v", r)
	}
}

func TestExplainListAndJSON(t *testing.T) {
	if r := runCLI(t, allTools, "-C", t.TempDir(), "explain"); r.code != 0 || !strings.Contains(r.stdout, "Terms: cc, churn,") {
		t.Errorf("%+v", r)
	}
	if r := runCLI(t, allTools, "-C", t.TempDir(), "--json", "explain", "not covered"); r.code != 0 || !strings.Contains(r.stdout, `"key": "not_covered"`) {
		t.Errorf("%+v", r)
	}
	if r := runCLI(t, allTools, "-C", t.TempDir(), "--json", "explain"); r.code != 0 || !strings.Contains(r.stdout, `"crap_avg"`) {
		t.Errorf("%+v", r)
	}
}

func TestCheckExplicitTargets(t *testing.T) {
	dir := tempModule(t)
	if r := runCLI(t, allTools, "-C", dir, "check", "--fast", "a/a.go"); r.code != 0 || !strings.Contains(r.stdout, "2 functions") {
		t.Errorf("file target: %+v", r)
	}
	if r := runCLI(t, allTools, "-C", dir, "check", "--fast", "example.com/m/a"); r.code != 0 || !strings.Contains(r.stdout, "| a.Close |") {
		t.Errorf("package target: %+v", r)
	}
	if r := runCLI(t, allTools, "-C", dir, "check", "--fast", "Nope"); r.code != 2 {
		t.Errorf("unknown target: %+v", r)
	}
	if r := runCLI(t, allTools, "-C", dir, "check", "--fast", "/elsewhere/x.go"); r.code != 2 || !strings.Contains(r.stderr, "outside the module") {
		t.Errorf("outside file: %+v", r)
	}
	if r := runCLI(t, allTools, "-C", dir, "hook", "uninstall"); r.code != 2 {
		t.Errorf("hook usage: %+v", r)
	}
}

func TestMutateCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	dir := coverageModule(t) // package b's tests fail
	r := runCLI(t, allTools, "-C", dir, "mutate", "--pkg", "b")
	if r.code != 2 || !strings.Contains(r.stdout, "b  ") || !strings.Contains(r.stdout, "tests fail before mutation") {
		t.Fatalf("pre-flight must block b: %+v", r)
	}
	if r := runCLI(t, allTools, "-C", dir, "show", "example.com/m/b"); !strings.Contains(r.stdout, "Mutation error   tests fail before mutation") {
		t.Errorf("show b: %s", r.stdout)
	}
	if r := runCLI(t, allTools, "-C", dir, "mutate", "--func", "Nope"); r.code != 2 {
		t.Errorf("unknown func: %+v", r)
	}
	if r := runCLI(t, allTools, "-C", dir, "mutate", "extra"); r.code != 2 {
		t.Errorf("args: %+v", r)
	}
	if r := runCLI(t, allTools, "-C", dir, "--json", "mutate", "--pkg", "./b"); !strings.Contains(r.stdout, `"status": "failed"`) {
		t.Errorf("json: %+v", r)
	}
}

func TestPackageLine(t *testing.T) {
	cases := []struct {
		p    mutate.PackageReport
		want string
	}{
		{mutate.PackageReport{Status: "ran", Killed: 3, Lived: 1, NotCov: 2, Seconds: 1.5}, "75% score · 3 killed, 1 survived, 2 not covered · 1.5s"},
		{mutate.PackageReport{Status: "ran"}, "— score · 0 killed, 0 survived, 0 not covered · 0.0s"},
		{mutate.PackageReport{Status: "skipped", Message: "over max"}, "skipped: over max"},
		{mutate.PackageReport{Status: "up to date"}, "up to date"},
	}
	for _, c := range cases {
		if got := packageLine(c.p); got != c.want {
			t.Errorf("%+v: %q", c.p, got)
		}
	}
}

func TestMCPUsage(t *testing.T) {
	if r := runCLI(t, allTools, "-C", tempModule(t), "mcp", "extra"); r.code != 2 || !strings.Contains(r.stderr, "mcp takes no arguments") {
		t.Fatalf("%+v", r)
	}
}
