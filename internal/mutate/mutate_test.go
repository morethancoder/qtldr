package mutate

import (
	"context"
	"os"
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
	}}}
	opt := Options{Root: root, Graph: g, Config: config.Default().Mutation, Engine: eng}
	rep, err := Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]PackageReport{}
	for _, p := range rep.Packages {
		status[string(p.Pkg)] = p
	}
	if a := status["example.com/m/a"]; a.Status != "ran" || a.Killed != 1 || a.Lived != 1 {
		t.Errorf("a: %+v", a)
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

	// Second run with no change: a is up to date; b (failed) is retried.
	eng.runs = nil
	rep, _ = Run(context.Background(), opt)
	if strings.Join(eng.runs, ",") != "" || rep.Packages[len(rep.Packages)-1].Status != "up to date" {
		t.Errorf("second run: engine %v report %+v", eng.runs, rep.Packages)
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
