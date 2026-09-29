package analysis

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/coverage"
	"github.com/morethancoder/qtldr/internal/lang/external"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/mutate"
	"github.com/morethancoder/qtldr/internal/store"
)

const (
	mod  = model.ID("example.com/m")
	pkgA = model.ID("example.com/m/a")
	pkgB = model.ID("example.com/m/b")
	fnF  = model.ID("example.com/m/a.F")
	fnG  = model.ID("example.com/m/b.G")
)

var now = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

// aGo is a/a.go in the test roots: F spans lines 4–6, its doc comment line 3.
const aGo = "package a\n\n// F doubles x.\nfunc F(x int) int {\n\treturn x * 2\n}\n"

// graph is what the fake provider scans: one module, packages a and b.
func graph() model.Graph {
	return model.Graph{
		Nodes: []model.Node{
			{ID: mod, Kind: model.KindModule, Name: "example.com/m", Dir: "."},
			{ID: pkgA, Kind: model.KindPackage, Name: "a", Parent: mod, Dir: "a"},
			{ID: fnF, Kind: model.KindFunc, Name: "F", Parent: pkgA, File: "a/a.go", Line: 4, EndLine: 6, DocLine: 3, BodyHash: "hF"},
			{ID: pkgB, Kind: model.KindPackage, Name: "b", Parent: mod, Dir: "b"},
			{ID: fnG, Kind: model.KindFunc, Name: "G", Parent: pkgB, File: "b/b.go", Line: 3, EndLine: 3, BodyHash: "hG"},
		},
		Metrics: map[model.ID]model.Metrics{
			fnF: {CC: model.Ptr(1), Cognitive: model.Ptr(0), LOC: model.Ptr(3)},
			fnG: {CC: model.Ptr(1), Cognitive: model.Ptr(0), LOC: model.Ptr(1)},
		},
	}
}

// fakeProvider scans graph() and returns canned coverage.
type fakeProvider struct {
	scanErr error
	run     coverage.RunResult
	runErr  error
	cov     map[model.ID]model.Coverage
	calls   *calls
}

type calls struct {
	project  []config.Project
	coverage [][]string
}

func (fakeProvider) Name() string       { return "fake" }
func (fakeProvider) Detect(string) bool { return true }

func (p fakeProvider) Scan(_ context.Context, _ string, cfg config.Project) (model.Graph, error) {
	p.calls.project = append(p.calls.project, cfg)
	if p.scanErr != nil {
		return model.Graph{}, p.scanErr
	}
	return graph(), nil
}

func (p fakeProvider) CoverageRun(_ context.Context, _ string, pkgs []string, _ config.Coverage, _ string) (coverage.RunResult, error) {
	p.calls.coverage = append(p.calls.coverage, pkgs)
	return p.run, p.runErr
}

func (p fakeProvider) MapCoverage(model.Graph, coverage.Profile) map[model.ID]model.Coverage {
	return p.cov
}

func (fakeProvider) Source(context.Context, string, string, int, int) (string, error) { return "", nil }

func newFake() fakeProvider {
	full := model.Coverage{Stmts: 1, Covered: 1, Percent: model.Ptr(100.0)}
	return fakeProvider{cov: map[model.ID]model.Coverage{fnF: full, fnG: full}, calls: &calls{}}
}

// root is a temp directory holding a/a.go.
func root(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "a/a.go", aGo)
	return dir
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func options(dir string, p fakeProvider) Options {
	return Options{Root: dir, Config: config.Default(), Provider: p, Now: now}
}

const notRepo = "not a git repository: churn, --changed and base-ref scope are not available"

func TestRunWritesSnapshot(t *testing.T) {
	dir, p := root(t), newFake()
	var progress []string
	opt := options(dir, p)
	opt.Coverage = true
	opt.Progress = func(msg string) { progress = append(progress, msg) }
	res, err := Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	s := res.Snapshot
	if s.Module != string(mod) || s.ToolVersion != ToolVersion || !s.Generated.Equal(now) || s.Runs.Structure == nil || !s.Runs.Structure.Equal(now) {
		t.Errorf("snapshot header: %+v", s)
	}
	if s.Runs.Coverage == nil || !s.Runs.Coverage.Equal(now) || s.Runs.Mutation != nil {
		t.Errorf("runs: %+v", s.Runs)
	}
	if m := s.Metrics[fnF]; m.Coverage == nil || *m.Coverage.Percent != 100 || m.CRAP == nil || *m.CRAP != 1 {
		t.Errorf("F metrics: %+v", m)
	}
	if want := []string{"Scanning packages…", "Running coverage… 2 packages"}; !reflect.DeepEqual(progress, want) {
		t.Errorf("progress %q, want %q", progress, want)
	}
	if want := [][]string{{string(pkgA), string(pkgB)}}; !reflect.DeepEqual(p.calls.coverage, want) {
		t.Errorf("coverage ran for %v, want %v", p.calls.coverage, want)
	}
	if want := []string{"./..."}; len(p.calls.project) != 1 || !reflect.DeepEqual(p.calls.project[0].Include, want) {
		t.Errorf("scan project: %+v", p.calls.project)
	}
	if !reflect.DeepEqual(res.Warnings, []string{notRepo}) || res.LogPath != "" || res.Failed != nil || res.Scope != nil {
		t.Errorf("result: warnings %q, log %q, failed %v, scope %v", res.Warnings, res.LogPath, res.Failed, res.Scope)
	}
	read, err := store.ReadSnapshot(dir)
	if err != nil || read.Module != string(mod) {
		t.Errorf("written snapshot: %v %q", err, read.Module)
	}
}

// Without --coverage the tests do not run; the cached coverage is used.
func TestRunWithoutCoverage(t *testing.T) {
	dir, p := root(t), newFake()
	opt := options(dir, p)
	opt.Patterns = []string{"./a"}
	res, err := Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.calls.coverage) != 0 || res.Snapshot.Runs.Coverage != nil || res.Snapshot.Metrics[fnF].Coverage != nil {
		t.Errorf("coverage ran: %v %+v", p.calls.coverage, res.Snapshot.Runs)
	}
	if got := p.calls.project[0].Include; !reflect.DeepEqual(got, []string{"./a"}) {
		t.Errorf("patterns override include: %q", got)
	}
}

func TestRunScope(t *testing.T) {
	cases := []struct {
		name  string
		scope []model.ID
		ran   [][]string
	}{
		{"scope narrows the coverage run", []model.ID{fnF}, [][]string{{string(pkgA)}}},
		{"empty scope runs nothing", []model.ID{}, nil},
	}
	for _, c := range cases {
		dir, p := root(t), newFake()
		opt := options(dir, p)
		opt.Coverage = true
		opt.Scope = func(model.Graph) ([]model.ID, error) { return c.scope, nil }
		res, err := Run(context.Background(), opt)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !reflect.DeepEqual(p.calls.coverage, c.ran) || !reflect.DeepEqual(res.Scope, c.scope) {
			t.Errorf("%s: ran %v scope %v", c.name, p.calls.coverage, res.Scope)
		}
	}
}

func TestRunErrors(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name  string
		setup func(t *testing.T, dir string, opt *Options)
		want  string
	}{
		{"scan fails", func(_ *testing.T, _ string, opt *Options) {
			p := newFake()
			p.scanErr = boom
			opt.Provider = p
		}, "boom"},
		{"scope fails", func(_ *testing.T, _ string, opt *Options) {
			opt.Scope = func(model.Graph) ([]model.ID, error) { return nil, boom }
		}, "boom"},
		{"coverage cache unreadable", func(t *testing.T, dir string, _ *Options) {
			mkdir(t, filepath.Join(dir, ".qtldr", "cache", "coverage.json"))
		}, "read "},
		{"coverage run fails", func(_ *testing.T, _ string, opt *Options) {
			p := newFake()
			p.run, p.runErr = coverage.RunResult{Output: []byte("go: build failed")}, boom
			opt.Provider, opt.Coverage = p, true
		}, "boom (full output: .qtldr/logs/coverage-20260929-120000.log)"},
		{"mutation cache is not JSON", func(t *testing.T, dir string, _ *Options) {
			write(t, dir, ".qtldr/mutation/x.json", "{")
		}, "is not valid JSON"},
	}
	for _, c := range cases {
		dir := root(t)
		opt := options(dir, newFake())
		c.setup(t, dir, &opt)
		_, err := Run(context.Background(), opt)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v, want it to contain %q", c.name, err, c.want)
		}
	}
}

func mkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

// A failed coverage run keeps its output in a log: withLog names it, or
// returns the run's error alone when the log cannot be written.
func TestCoverageRunLog(t *testing.T) {
	boom := errors.New("boom")
	dir := root(t)
	if err := withLog(options(dir, newFake()), []byte("output"), boom); !errors.Is(err, boom) ||
		err.Error() != "boom (full output: .qtldr/logs/coverage-20260929-120000.log)" {
		t.Errorf("withLog: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, ".qtldr", "logs", "coverage-20260929-120000.log")); err != nil || string(b) != "output" {
		t.Errorf("log: %q %v", b, err)
	}
	blocked := root(t)
	write(t, blocked, ".qtldr/logs", "a file where the logs directory goes")
	if err := withLog(options(blocked, newFake()), []byte("output"), boom); err != boom {
		t.Errorf("withLog without a log: %v", err)
	}
}

func TestRunFailedPackages(t *testing.T) {
	dir, p := root(t), newFake()
	p.run = coverage.RunResult{Failed: []string{string(pkgB)}, Output: []byte("--- FAIL: TestG")}
	opt := options(dir, p)
	opt.Coverage = true
	res, err := Run(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	log := ".qtldr/logs/coverage-20260929-120000.log"
	if res.LogPath != log || !reflect.DeepEqual(res.Failed, []model.ID{pkgB}) {
		t.Errorf("log %q failed %v", res.LogPath, res.Failed)
	}
	if got := res.Snapshot.Metrics[pkgB].CoverageError; got != "tests failed; output in "+log {
		t.Errorf("package error %q", got)
	}
	if res.Snapshot.Metrics[fnG].Coverage != nil || res.Snapshot.Metrics[fnF].Coverage == nil {
		t.Errorf("coverage of the failed package must be not measured: %+v %+v", res.Snapshot.Metrics[fnG].Coverage, res.Snapshot.Metrics[fnF].Coverage)
	}

	blocked := root(t)
	write(t, blocked, ".qtldr/logs", "a file where the logs directory goes")
	if _, err := Run(context.Background(), Options{Root: blocked, Config: config.Default(), Provider: p, Now: now, Coverage: true}); err == nil {
		t.Error("an unwritable log must fail the run")
	}
}

// fakeMutator must never run: every package in these tests is up to date.
type fakeMutator struct{ t *testing.T }

func (fakeMutator) Name() string                   { return "gremlins" }
func (fakeMutator) Version(context.Context) string { return "test" }
func (m fakeMutator) Run(context.Context, string, mutate.Target, config.Mutation) ([]mutate.FileMutant, []byte, error) {
	m.t.Error("mutator ran for an up-to-date package")
	return nil, nil, nil
}

// saveMutation writes current mutation results for F and G.
func saveMutation(t *testing.T, dir string) {
	t.Helper()
	g := graph()
	for pkg, fn := range map[model.ID]model.ID{pkgA: fnF, pkgB: fnG} {
		c := mutate.NewCache(pkg)
		c.Record(g, map[model.ID]model.Mutation{fn: {Killed: 1, Mutants: []model.Mutant{{Line: 5, Col: 11, Type: "ARITHMETIC_BASE", Status: "KILLED"}}}}, "gremlins", "test", now)
		if err := mutate.Save(dir, c); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunMutationScope(t *testing.T) {
	cases := []struct {
		name  string
		pkgs  []model.ID
		scope []model.ID
		want  []model.ID
	}{
		{"everything by default", nil, nil, []model.ID{pkgA, pkgB}},
		{"the scope's packages", nil, []model.ID{fnF}, []model.ID{pkgA}},
		{"explicit packages win over the scope", []model.ID{pkgB}, []model.ID{fnF}, []model.ID{pkgB}},
	}
	for _, c := range cases {
		dir := root(t)
		saveMutation(t, dir)
		opt := options(dir, newFake())
		opt.Mutate, opt.MutatePackages, opt.Mutator = true, c.pkgs, fakeMutator{t}
		if c.scope != nil {
			opt.Scope = func(model.Graph) ([]model.ID, error) { return c.scope, nil }
		}
		res, err := Run(context.Background(), opt)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var got []model.ID
		for _, p := range res.Mutation.Packages {
			got = append(got, p.Pkg)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: mutation looked at %v, want %v", c.name, got, c.want)
		}
		if at := res.Snapshot.Runs.Mutation; at == nil || !at.Equal(now) {
			t.Errorf("%s: mutation run time %v", c.name, at)
		}
		if mu := res.Snapshot.Metrics[fnF].Mutation; mu == nil || mu.Killed != 1 {
			t.Errorf("%s: F mutation %+v", c.name, mu)
		}
	}
}

func TestRunMutationError(t *testing.T) {
	dir := root(t)
	mkdir(t, mutate.Path(dir, pkgA)) // a directory where the cache file goes
	opt := options(dir, newFake())
	opt.Mutate, opt.Mutator = true, fakeMutator{t}
	if _, err := Run(context.Background(), opt); err == nil {
		t.Error("an unreadable mutation cache must fail the run")
	}
}

// providerOutput is what the example provider (testdata/provider/example)
// prints for one file dir/x.txt: a package and a function.
const providerOutput = `{"nodes":[{"id":"example:dir","kind":"package","name":"dir (txt)","dir":"dir"},` +
	`{"id":"example:dir.x","kind":"func","name":"x.txt","parent":"example:dir","file":"dir/x.txt","line":1,"end_line":3,"pure":true}],` +
	`"edges":[],"metrics":{"example:dir.x":{"cc":2,"coverage":{"stmts":4,"covered":3,"percent":75,"stale":false}}}}`

func TestRunProviders(t *testing.T) {
	cases := []struct {
		name, command, output string
		node                  model.ID
		warning               string
	}{
		{"merged", "cat out.json", providerOutput, "example:dir", ""},
		{"command fails", "qtldr-test-no-such-provider", providerOutput, "provider:ex", "provider ex: "},
		{"contract broken", "cat out.json", `{"nodes":[{"id":"x","kind":"bogus"}],"edges":[],"metrics":{}}`, "provider:ex", "provider ex broke the contract ("},
	}
	for _, c := range cases {
		dir := root(t)
		write(t, dir, "dir/x.txt", "text\n")
		write(t, dir, "out.json", c.output)
		opt := options(dir, newFake())
		opt.Config.Providers = []config.Provider{{Name: "ex", Command: c.command, Extensions: []string{".txt"}}}
		var progress []string
		opt.Progress = func(msg string) { progress = append(progress, msg) }
		res, err := Run(context.Background(), opt)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		n, ok := res.Snapshot.Graph.Node(c.node)
		if !ok || n.Parent != mod {
			t.Errorf("%s: node %s: %v %+v", c.name, c.node, ok, n)
		}
		warned := len(res.Warnings) == 2 && strings.HasPrefix(res.Warnings[0], c.warning)
		if (c.warning == "") != (len(res.Warnings) == 1) || (c.warning != "" && !warned) {
			t.Errorf("%s: warnings %q", c.name, res.Warnings)
		}
		if !reflect.DeepEqual(progress, []string{"Scanning packages…", "Running provider ex…"}) {
			t.Errorf("%s: progress %q", c.name, progress)
		}
	}
}

func TestProviderFailure(t *testing.T) {
	if got := providerFailure("ex", nil, nil); got != nil {
		t.Errorf("no failure: %q", got)
	}
	if got := providerFailure("ex", errors.New("exit 1"), nil); !reflect.DeepEqual(got, []string{"provider ex: exit 1; its language is not measured"}) {
		t.Errorf("error: %q", got)
	}
	got := providerFailure("ex", nil, []external.Problem{{Path: "$.nodes[0].kind", Message: "unknown kind"}})
	if len(got) != 2 || got[0] != "provider ex broke the contract (1 problems; run: qtldr provider test ex); its language is not measured" || !strings.Contains(got[1], "$.nodes[0].kind") {
		t.Errorf("problems: %q", got)
	}
}

func TestFirstModuleAndPackageIDs(t *testing.T) {
	if got := firstModule(model.Graph{}); got != "" {
		t.Errorf("no module: %q", got)
	}
	g := graph()
	if got := firstModule(g); got != mod {
		t.Errorf("module: %q", got)
	}
	g.Nodes = []model.Node{g.Nodes[4], g.Nodes[3], g.Nodes[1]} // G, b, a: only packages, sorted
	if got := packageIDs(g); !reflect.DeepEqual(got, []model.ID{pkgA, pkgB}) {
		t.Errorf("packages: %v", got)
	}
}

func TestSnapshotModuleLabel(t *testing.T) {
	two := graph()
	two.Nodes = append(two.Nodes, model.Node{ID: "example.com/n", Kind: model.KindModule})
	cases := []struct {
		g    model.Graph
		want string
	}{
		{model.Graph{}, ""},
		{graph(), "example.com/m"},
		{two, "go.work: example.com/m, example.com/n"},
	}
	for _, c := range cases {
		if got := snapshot(c.g, nil, now, nil).Module; got != c.want {
			t.Errorf("module label %q, want %q", got, c.want)
		}
	}
}

func TestAllowIDs(t *testing.T) {
	var res Result
	got := allowIDs(graph(), []string{"a.F", "nope"}, &res)
	if !reflect.DeepEqual(got, []model.ID{fnF}) {
		t.Errorf("ids %v", got)
	}
	if len(res.Warnings) != 1 || !strings.HasPrefix(res.Warnings[0], "[purity].allow: ") {
		t.Errorf("warnings %q", res.Warnings)
	}
}

func TestWithDefaults(t *testing.T) {
	o := Options{}.withDefaults()
	if o.Provider == nil || o.Provider.Name() != "go" || o.Now.IsZero() || o.Now.Nanosecond() != 0 {
		t.Errorf("defaults: %+v", o)
	}
	p := newFake()
	if o := (Options{Provider: p, Now: now}).withDefaults(); o.Provider.Name() != "fake" || !o.Now.Equal(now) {
		t.Errorf("kept: %+v", o)
	}
	Options{}.progress("no Progress func: nothing happens")
}

// gitRepo makes dir a git repository with one commit of everything in it.
func gitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "."},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
}

func TestRunChurn(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	cases := []struct{ scope, want string }{{"function", "function"}, {"file", "file"}}
	for _, c := range cases {
		dir := root(t)
		gitRepo(t, dir)
		opt := options(dir, newFake())
		opt.Config.Churn.Scope = c.scope
		res, err := Run(context.Background(), opt)
		if err != nil {
			t.Fatal(err)
		}
		m := res.Snapshot.Metrics[fnF]
		if m.ChurnScope != c.want || m.Churn == nil || *m.Churn != 1 || len(res.Warnings) != 0 {
			t.Errorf("scope %s: churn %v %q, warnings %q", c.scope, m.Churn, m.ChurnScope, res.Warnings)
		}
		if res.Snapshot.Git == nil || res.Snapshot.Git.Head == "" {
			t.Errorf("git info: %+v", res.Snapshot.Git)
		}
	}
}

// In a repository without commits git log fails: churn is "not measured",
// with git's reason, and the run goes on.
func TestRunChurnNotMeasured(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := root(t)
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	res, err := Run(context.Background(), options(dir, newFake()))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || !strings.HasPrefix(res.Warnings[0], "churn not measured: git log") {
		t.Errorf("warnings %q", res.Warnings)
	}
	if m := res.Snapshot.Metrics[fnF]; m.Churn != nil {
		t.Errorf("churn must be not measured, got %d", *m.Churn)
	}
}

func TestRelFile(t *testing.T) {
	real, err := filepath.EvalSymlinks(t.TempDir()) // macOS: /var is a symlink
	if err != nil {
		t.Fatal(err)
	}
	write(t, real, "a/a.go", aGo)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}
	cases := []struct {
		root, cwd, p string
		want         string
		ok           bool
	}{
		{real, real, "a/a.go", "a/a.go", true},
		{link, "/", filepath.Join(real, "a", "a.go"), "a/a.go", true}, // root through a symlink
		{real, "/", filepath.Join(link, "a", "a.go"), "a/a.go", true}, // file through a symlink
		{real, real, "../outside.go", "", false},
		{real, real, "a/missing/new.go", "a/missing/new.go", true}, // parent does not exist yet
	}
	for _, c := range cases {
		got, ok := RelFile(c.root, c.cwd, c.p)
		if got != c.want || ok != c.ok {
			t.Errorf("RelFile(%q, %q, %q) = %q, %v; want %q, %v", c.root, c.cwd, c.p, got, ok, c.want, c.ok)
		}
	}
}

func TestSource(t *testing.T) {
	dir := root(t)
	g := graph()
	f := g.Nodes[2]
	cases := []struct {
		name  string
		n     model.Node
		from  int
		first string
		err   string
	}{
		{"from the doc comment", f, 3, "// F doubles x.", ""},
		{"no doc comment", withDoc(f, 0), 4, "func F(x int) int {", ""},
		{"types too", model.Node{ID: "example.com/m/a.T", Kind: model.KindType, File: "a/a.go", Line: 1, EndLine: 1}, 1, "package a", ""},
		{"not a function", g.Nodes[1], 0, "", "example.com/m/a is a package; source is shown for functions and types"},
		{"file gone", model.Node{ID: fnG, Kind: model.KindFunc, File: "b/b.go", Line: 3, EndLine: 3}, 0, "", "re-run qtldr analyze"},
	}
	for _, c := range cases {
		src, err := Source(dir, model.Snapshot{Graph: g}, c.n)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%s: error %v, want %q", c.name, err, c.err)
			}
			continue
		}
		if err != nil || src.From != c.from || len(src.Lines) == 0 || src.Lines[0].Text != c.first {
			t.Errorf("%s: %v from %d, lines %+v", c.name, err, src.From, src.Lines)
		}
	}
	write(t, dir, ".qtldr/notes.json", "{")
	if _, err := Source(dir, model.Snapshot{Graph: g}, f); err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("bad notes: %v", err)
	}
}

func withDoc(n model.Node, line int) model.Node {
	n.DocLine = line
	return n
}

func ExampleRelFile() {
	rel, ok := RelFile("/repo", "/repo/sub", "x.go")
	fmt.Println(rel, ok)
	// Output: sub/x.go true
}
