package mutate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/lang/golang"
	"github.com/morethancoder/qtldr/internal/model"
)

var fixture = filepath.Join("..", "..", "testdata", "ledger")

const pricing = model.ID("github.com/acme/ledger/internal/pricing")

// Real Gremlins v0.6.0 output captured on the fixture (docs/decisions.md #5).
func readCapture(t *testing.T, name, dir string) []FileMutant {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "gremlins", name))
	if err != nil {
		t.Fatal(err)
	}
	ms, err := ParseReport(b, dir)
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

func TestParseReport(t *testing.T) {
	pkg := readCapture(t, "ledger-pricing.json", "internal/pricing")
	if len(pkg) != 31 || pkg[0].File != "internal/pricing/apply.go" {
		t.Fatalf("%d mutants, first %+v", len(pkg), pkg[0])
	}
	all := readCapture(t, "ledger-all.json", ".")
	if len(all) != 84 || !strings.HasPrefix(all[0].File, "cmd/") && !strings.HasPrefix(all[0].File, "internal/") {
		t.Fatalf("%d mutants, first %+v", len(all), all[0])
	}
	if _, err := ParseReport([]byte("{"), "."); err == nil {
		t.Error("bad JSON")
	}
}

func scanFixture(t *testing.T) model.Graph {
	t.Helper()
	root, _ := filepath.Abs(fixture)
	g, err := golang.New().Scan(context.Background(), root, config.Default().Project)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestAttributeAndDescribe(t *testing.T) {
	g := scanFixture(t)
	root, _ := filepath.Abs(fixture)
	got := Attribute(g, pricing, readCapture(t, "ledger-pricing.json", "internal/pricing"), sources(root, g, pricing))
	at := got[pricing+".applyTiered"]
	if at.Killed != 7 || at.Survived != 4 || at.NotCovered != 2 || at.Score == nil || *at.Score != 63.6 {
		t.Fatalf("applyTiered %+v score %v", at, at.Score)
	}
	var lived []string
	for _, m := range at.Mutants {
		if m.Status == Lived {
			lived = append(lived, strconv.Itoa(m.Line)+" "+m.Description)
		}
	}
	if strings.Join(lived, ", ") != "24 <= → <, 29 >= → >, 29 > → >=, 36 > → >=" {
		t.Errorf("survivors: %v", lived)
	}
	if bf := got[pricing+".BestRule"]; bf.Killed+bf.Survived != 0 || bf.NotCovered == 0 {
		t.Errorf("BestRule is untested: %+v", bf)
	}
	if _, ok := got[pricing+".applyFlat"]; !ok {
		t.Error("every function of the package gets an entry")
	}
	src, _ := os.ReadFile(filepath.Join(fixture, "internal", "pricing", "tier.go"))
	for _, c := range []struct {
		typ       string
		line, col int
		want      string
	}{
		{"INVERT_NEGATIVES", 39, 35, "- → +"},
		{"ARITHMETIC_BASE", 39, 35, "- → +"},
		{"CONDITIONALS_NEGATION", 21, 16, "== → !="},
		{"NEW_TYPE", 21, 16, "NEW_TYPE"},
		{"CONDITIONALS_BOUNDARY", 21, 1, "CONDITIONALS_BOUNDARY"},
	} {
		if got := Describe(c.typ, src, c.line, c.col); got != c.want {
			t.Errorf("%s %d:%d = %q, want %q", c.typ, c.line, c.col, got, c.want)
		}
	}
}

func TestTimeoutCoefficientAndVersion(t *testing.T) {
	cases := map[time.Duration]int{0: 16, time.Second: 16, 1400 * time.Millisecond: 12, 10 * time.Second: 3, time.Minute: 3}
	for base, want := range cases {
		if got := TimeoutCoefficient(base); got != want {
			t.Errorf("TimeoutCoefficient(%v) = %d, want %d", base, got, want)
		}
	}
	out := "/Users/x/go/bin/gremlins: go1.27.0\n\tpath\tgithub.com/go-gremlins/gremlins/cmd/gremlins\n\tmod\tgithub.com/go-gremlins/gremlins\tv0.6.0\th1:x\n"
	if ModuleVersion(out, "github.com/go-gremlins/gremlins") != "v0.6.0" || ModuleVersion("", "x") != "unknown" {
		t.Error("ModuleVersion")
	}
}

func TestCacheRoundTrip(t *testing.T) {
	g := scanFixture(t)
	root, _ := filepath.Abs(fixture)
	results := Attribute(g, pricing, readCapture(t, "ledger-pricing.json", "internal/pricing"), sources(root, g, pricing))
	at := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	c := NewCache(pricing)
	c.Record(g, results, "gremlins", "v0.6.0", at)
	if !c.Current(g, pricing) {
		t.Fatal("fresh results are current")
	}
	dir := t.TempDir()
	if err := Save(dir, c); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".qtldr", "mutation", "github.com__acme__ledger__internal__pricing.json")); err != nil {
		t.Fatal(err)
	}
	all, err := LoadAll(dir)
	if err != nil || len(all) != 1 {
		t.Fatalf("%v %v", all, err)
	}
	// applyTiered's body changed and it moved down 3 lines.
	for i, n := range g.Nodes {
		if n.ID == pricing+".applyTiered" {
			g.Nodes[i].BodyHash += "x"
			g.Nodes[i].Line += 3
		}
	}
	if all[0].Current(g, pricing) {
		t.Error("a changed body is not current")
	}
	mu, errs, latest := Resolve(all, g)
	tiered := mu[pricing+".applyTiered"]
	if !tiered.Stale || tiered.Mutants[0].Line != 24 || len(errs) != 0 || !latest.Equal(at) {
		t.Errorf("resolve: stale %v first line %d errs %v latest %v", tiered.Stale, tiered.Mutants[0].Line, errs, latest)
	}
	all[0].Fail(PreflightFailed, at)
	if _, errs, _ := Resolve(all, g); errs[pricing] != PreflightFailed {
		t.Errorf("errors %v", errs)
	}
	if c, err := Load(dir, "nope"); err != nil || len(c.Functions) != 0 {
		t.Errorf("missing cache: %+v %v", c, err)
	}
}

func TestMakePlan(t *testing.T) {
	g := model.Graph{
		Nodes: []model.Node{
			{ID: "m/a", Kind: model.KindPackage, Dir: "a"}, {ID: "m/a.f", Kind: model.KindFunc, Parent: "m/a", BodyHash: "1"},
			{ID: "m/b", Kind: model.KindPackage, Dir: "b"}, {ID: "m/b.g", Kind: model.KindFunc, Parent: "m/b", BodyHash: "2"},
			{ID: "m/b.h", Kind: model.KindFunc, Parent: "m/b", BodyHash: "3"},
			{ID: "m/c", Kind: model.KindPackage, Dir: "c"},
		},
		Metrics: map[model.ID]model.Metrics{"m/a.f": {CRAP: model.Ptr(3.0)}, "m/b.g": {CRAP: model.Ptr(30.0)}},
	}
	at := time.Now()
	current := NewCache("m/a")
	current.RanAt = &at
	current.Functions["m/a.f"] = Entry{BodyHash: "1"}
	caches := map[model.ID]Cache{"m/a": current}
	p := MakePlan(g, caches, nil, false, 40)
	if len(p.Run) != 1 || p.Run[0].Pkg != "m/b" || len(p.UpToDate) != 1 {
		t.Fatalf("incremental: %+v", p)
	}
	p = MakePlan(g, caches, nil, true, 2)
	if len(p.Run) != 1 || p.Run[0].Pkg != "m/b" || len(p.Skipped) != 1 || p.Skipped[0].Pkg != "m/a" {
		t.Fatalf("force with max 2: highest CRAP first, rest skipped: %+v", p)
	}
	if p := MakePlan(g, caches, []model.ID{"m/a"}, true, 0); len(p.Run) != 1 || p.Run[0].Pkg != "m/a" {
		t.Fatalf("scope: %+v", p)
	}
}

// fakeEngine returns canned mutants and counts its runs.
type fakeEngine struct {
	runs    []string
	mutants map[string][]FileMutant
}

func (f *fakeEngine) Name() string                   { return "fake" }
func (f *fakeEngine) Version(context.Context) string { return "v1" }
func (f *fakeEngine) Run(_ context.Context, _ string, t Target, _ config.Mutation) ([]FileMutant, []byte, error) {
	f.runs = append(f.runs, t.Dir)
	if t.Baseline <= 0 {
		return nil, nil, os.ErrInvalid
	}
	return f.mutants[t.Dir], []byte("ok"), nil
}

func writeModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":      "module example.com/m\n\ngo 1.26.0\n",
		"a/a.go":      "package a\n\nfunc Pos(x int) bool {\n\treturn x > 0\n}\n",
		"a/a_test.go": "package a\n\nimport \"testing\"\n\nfunc TestPos(t *testing.T) {\n\tif !Pos(2) {\n\t\tt.Fatal(\"Pos\")\n\t}\n}\n",
		"b/b.go":      "package b\n\nfunc Neg(x int) bool { return x < 0 }\n",
		"b/b_test.go": "package b\n\nimport \"testing\"\n\nfunc TestNeg(t *testing.T) { t.Fatal(\"broken\") }\n",
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRunIncrementalAndPreflight(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	root := writeModule(t)
	g, err := golang.New().Scan(context.Background(), root, config.Default().Project)
	if err != nil {
		t.Fatal(err)
	}
	eng := &fakeEngine{mutants: map[string][]FileMutant{"a": {
		{File: "a/a.go", Line: 4, Col: 11, Type: "CONDITIONALS_BOUNDARY", Status: Lived},
		{File: "a/a.go", Line: 4, Col: 11, Type: "CONDITIONALS_NEGATION", Status: Killed},
		{File: "a/a.go", Line: 4, Col: 11, Type: "CONDITIONALS_BOUNDARY", Status: NotCovered},
	}}}
	var progress []string
	opt := Options{Root: root, Graph: g, Config: config.Default().Mutation, Engine: eng, Progress: func(m string) { progress = append(progress, m) }}
	rep, err := Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]PackageReport{}
	for _, p := range rep.Packages {
		status[string(p.Pkg)] = p
	}
	if a := status["example.com/m/a"]; a.Status != "ran" || a.Killed != 1 || a.Lived != 1 || a.NotCov != 1 {
		t.Errorf("a: %+v", a)
	}
	if secs := strconv.FormatFloat(status["example.com/m/a"].Seconds, 'f', -1, 64); strings.Contains(secs, ".") && len(secs)-strings.Index(secs, ".") > 2 {
		t.Errorf("seconds are rounded to 0.1: %s", secs)
	}
	if len(progress) != 2 || progress[0] != "Running mutation… 1/2 packages (a)" || progress[1] != "Running mutation… 2/2 packages (b)" {
		t.Errorf("progress %q", progress)
	}
	if b := status["example.com/m/b"]; b.Status != "failed" || !strings.Contains(b.Message, PreflightFailed) || !strings.Contains(b.Message, ".qtldr/logs/") {
		t.Errorf("b must be blocked by the pre-flight: %+v", b)
	}
	if strings.Join(eng.runs, ",") != "a" {
		t.Errorf("engine runs %v", eng.runs)
	}
	all, _ := LoadAll(root)
	mu, _, _ := Resolve(all, g)
	if pos := mu["example.com/m/a.Pos"]; pos.Mutants[0].Description != "> → >=" || *pos.Score != 50 {
		t.Errorf("Pos: %+v", pos)
	}

	// Second run with no change and no timeout (0 = no limit): a is up to
	// date; b (failed) is retried.
	eng.runs = nil
	opt.Config.Timeout = config.Duration{}
	rep, err = Run(context.Background(), opt)
	if err != nil || strings.Join(eng.runs, ",") != "" || rep.Packages[len(rep.Packages)-1].Status != "up to date" {
		t.Errorf("second run: %v engine %v report %+v", err, eng.runs, rep.Packages)
	}
}

// fakeGremlins writes a script that acts like `gremlins unleash`: it copies a
// captured report to the -o path (or fails).
func fakeGremlins(t *testing.T, report string, fail bool) Gremlins {
	t.Helper()
	dir := t.TempDir()
	body := "#!/bin/sh\nwhile [ $# -gt 0 ]; do if [ \"$1\" = -o ]; then out=$2; fi; shift; done\n"
	if fail {
		body += "echo 'ERROR: failed to gather coverage' >&2; exit 1\n"
	} else {
		body += "cp '" + report + "' \"$out\"\necho 'Killed: 19, Lived: 6'\n"
	}
	bin := filepath.Join(dir, "gremlins")
	if err := os.WriteFile(bin, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return Gremlins{Bin: bin}
}

func TestGremlinsRun(t *testing.T) {
	report, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "gremlins", "ledger-pricing.json"))
	ms, out, err := fakeGremlins(t, report, false).Run(context.Background(), t.TempDir(), Target{Dir: "internal/pricing", Baseline: time.Second}, config.Mutation{})
	if err != nil || len(ms) != 31 || ms[0].File != "internal/pricing/apply.go" || !strings.Contains(string(out), "Killed: 19") {
		t.Fatalf("%d mutants, %v, %s", len(ms), err, out)
	}
	_, out, err = fakeGremlins(t, report, true).Run(context.Background(), t.TempDir(), Target{Dir: "a"}, config.Mutation{})
	if err == nil || !strings.Contains(err.Error(), "unleash ./a -o") || !strings.Contains(string(out), "failed to gather coverage") {
		t.Fatalf("failure: %v %s", err, out)
	}
	args := Gremlins{}.Args(Target{Dir: "x", Baseline: 10 * time.Second}, "/r.json", []string{"--invert-logical"})
	if strings.Join(args, " ") != "unleash ./x -o /r.json --timeout-coefficient 3 --invert-logical" {
		t.Errorf("args %v", args)
	}
	if (Gremlins{Bin: "/nope/gremlins"}).Version(context.Background()) != "unknown" || (Gremlins{}).Name() != "gremlins" {
		t.Error("Version/Name")
	}
}

func TestTallyStatuses(t *testing.T) {
	mu := Tally([]model.Mutant{{Line: 2, Status: TimedOut}, {Line: 1, Col: 5, Status: Killed}, {Line: 1, Col: 2, Status: NotCovered}, {Line: 3, Status: NotViable}})
	if mu.Killed != 1 || mu.TimedOut != 1 || mu.NotCovered != 1 || mu.Survived != 0 || *mu.Score != 100 || mu.Mutants[0].Col != 2 {
		t.Fatalf("%+v", mu)
	}
}

// fakeGremlinsBinary builds a Go program whose main module path is
// Gremlins', so `go version -m` reports it as "(devel)".
func fakeGremlinsBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{"go.mod": "module github.com/go-gremlins/gremlins\n\ngo 1.26.0\n", "main.go": "package main\n\nfunc main() {}\n"}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("go", "build", "-o", "gremlins", ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	return filepath.Join(dir, "gremlins")
}

func TestGremlinsVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary")
	}
	if got := (Gremlins{Bin: fakeGremlinsBinary(t)}).Version(context.Background()); got != "(devel)" {
		t.Errorf("Version = %q, want the module version from go version -m", got)
	}
	// A binary without Go build info (a script) has no version.
	script := filepath.Join(t.TempDir(), "gremlins")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := (Gremlins{Bin: script}).Version(context.Background()); got != "unknown" {
		t.Errorf("script: %q", got)
	}
}

func TestModuleVersionFields(t *testing.T) {
	cases := map[string]string{
		"\tmod\tgithub.com/go-gremlins/gremlins\t(devel)\t\n":         "(devel)", // main module: no sum
		"\tmod\tgithub.com/go-gremlins/gremlins\tv0.6.0\th1:x\n":      "v0.6.0",
		"\tdep\tgithub.com/go-gremlins/gremlins\tv0.6.0\th1:x\n":      "unknown",
		"\tmod\tgithub.com/go-gremlins/gremlins-fork\tv1.0.0\th1:x\n": "unknown",
		"\tmod\tgithub.com/go-gremlins/gremlins\n":                    "unknown",
	}
	for out, want := range cases {
		if got := ModuleVersion(out, "github.com/go-gremlins/gremlins"); got != want {
			t.Errorf("ModuleVersion(%q) = %q, want %q", out, got, want)
		}
	}
}

func TestFill(t *testing.T) {
	items := func(funcs ...int) []Item {
		var out []Item
		for i, n := range funcs {
			out = append(out, Item{Pkg: model.ID(strconv.Itoa(i)), Funcs: n})
		}
		return out
	}
	cases := []struct {
		name         string
		funcs        []int
		max          int
		run, skipped int
	}{
		{"the first package always runs", []int{50, 1}, 40, 1, 1},
		{"up to max exactly", []int{20, 20, 1}, 40, 2, 1},
		{"max 0 means no limit", []int{50, 50}, 0, 2, 0},
		{"later packages fill the gap", []int{30, 20, 10}, 40, 2, 1},
	}
	for _, c := range cases {
		var p Plan
		p.fill(items(c.funcs...), c.max)
		if len(p.Run) != c.run || len(p.Skipped) != c.skipped || p.Run[0].Pkg != "0" {
			t.Errorf("%s: run %+v skipped %d, want %d (first is 0) and %d", c.name, p.Run, len(p.Skipped), c.run, c.skipped)
		}
	}
}

// Mutants at the same position keep their report order.
func TestTallyKeepsSamePositionOrder(t *testing.T) {
	mu := Tally([]model.Mutant{{Line: 2, Col: 3, Type: "B"}, {Line: 2, Col: 3, Type: "A"}, {Line: 1, Col: 9, Type: "C"}})
	if got := mu.Mutants[0].Type + mu.Mutants[1].Type + mu.Mutants[2].Type; got != "CBA" {
		t.Errorf("order %s", got)
	}
}

// A package whose functions have no CRAP ranks below one whose highest CRAP
// is the minimum (1); ties go by package path.
func TestPlanOrderMissingCRAP(t *testing.T) {
	g := model.Graph{
		Nodes: []model.Node{
			{ID: "m/a", Kind: model.KindPackage}, {ID: "m/a.f", Kind: model.KindFunc, Parent: "m/a"},
			{ID: "m/b", Kind: model.KindPackage}, {ID: "m/b.g", Kind: model.KindFunc, Parent: "m/b"},
		},
		Metrics: map[model.ID]model.Metrics{"m/b.g": {CRAP: model.Ptr(1.0)}},
	}
	p := MakePlan(g, nil, nil, true, 0)
	if len(p.Run) != 2 || p.Run[0].Pkg != "m/b" {
		t.Fatalf("order %+v", p.Run)
	}
}

func TestEnclosingAndCounted(t *testing.T) {
	fns := []model.Node{{ID: "f", Line: 3, EndLine: 5}, {ID: "g", Line: 7, EndLine: 7}}
	for line, want := range map[int]model.ID{3: "f", 5: "f", 7: "g", 6: "", 8: ""} {
		if got, ok := enclosing(fns, line); got != want || ok != (want != "") {
			t.Errorf("enclosing(%d) = %q %v", line, got, ok)
		}
	}
	for status, want := range map[string]bool{Killed: true, Lived: true, NotCovered: true, TimedOut: true, NotViable: false, Skipped: false, Runnable: false} {
		if counted(status) != want {
			t.Errorf("counted(%s) != %v", status, want)
		}
	}
}

func TestLater(t *testing.T) {
	t1, t2 := time.Unix(100, 0), time.Unix(200, 0)
	cases := []struct{ a, b, want *time.Time }{{nil, &t1, &t1}, {&t1, nil, &t1}, {&t1, &t2, &t2}, {&t2, &t1, &t2}, {nil, nil, nil}}
	for _, c := range cases {
		if got := later(c.a, c.b); got != c.want {
			t.Errorf("later(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
