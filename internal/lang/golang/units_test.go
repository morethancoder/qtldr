package golang

import (
	"context"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/coverage"
	"github.com/morethancoder/qtldr/internal/model"
)

// checkSource type-checks one file of package x (standard library imports
// are read from source; no network).
func checkSource(t *testing.T, src string) (*ast.File, *types.Info) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{},
		Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	if _, err := conf.Check("x", fset, []*ast.File{file}, info); err != nil {
		t.Fatal(err)
	}
	return file, info
}

func funcDecl(t *testing.T, f *ast.File, name string) *ast.FuncDecl {
	t.Helper()
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == name {
			return fn
		}
	}
	t.Fatalf("no func %s", name)
	return nil
}

// Only package-level functions are matched against effectfulCalls: the
// method time.Time.After is pure, the function time.After is not.
func TestCallEffectFunctionsNotMethods(t *testing.T) {
	file, info := checkSource(t, `package x

import "time"

func f(t, u time.Time) bool {
	_ = time.Now()
	return t.After(u)
}
`)
	var got []string
	ast.Inspect(file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if r := callEffect(info, call); r != "" {
				got = append(got, r)
			}
		}
		return true
	})
	if !slices.Equal(got, []string{"calls time.Now"}) {
		t.Fatalf("got %q", got)
	}
}

// A write marks a package variable as written: the writer "writes" it, and an
// error variable written anywhere is no longer a sentinel for its readers.
func TestLocalEffectsWritesAndSentinels(t *testing.T) {
	file, info := checkSource(t, `package x

import "errors"

var ErrGone = errors.New("gone")
var ErrMoved = errors.New("moved")
var counter int

func write() {
	counter = 1
	ErrMoved = nil
}

func read() error {
	if counter > 0 && ErrGone != nil {
		return ErrMoved
	}
	return nil
}
`)
	vf := newVarFacts([]*packages.Package{{Syntax: []*ast.File{file}, TypesInfo: info}}, map[string]bool{"x": true})
	s := &scanner{vars: vf}
	cases := map[string][]string{
		"write": {"writes var x.counter", "writes var x.ErrMoved"},
		"read":  {"reads var x.counter", "reads var x.ErrMoved"},
	}
	for name, want := range cases {
		if got := s.localEffects(info, funcDecl(t, file, name)); !slices.Equal(got, want) {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
}

// typeEffect looks 9 levels into unnamed composite types, no further.
func TestTypeEffectDepth(t *testing.T) {
	nest := func(n int) types.Type {
		var t types.Type = types.NewChan(types.SendRecv, types.Typ[types.Int])
		for range n {
			t = types.NewSlice(t)
		}
		return t
	}
	if got := typeEffect(nest(9), 0); got != "a channel" {
		t.Errorf("9 levels: %q", got)
	}
	if got := typeEffect(nest(10), 0); got != "" {
		t.Errorf("10 levels: %q", got)
	}
}

func TestMapCoverage(t *testing.T) {
	g := model.Graph{Nodes: []model.Node{
		{ID: "example.com/m", Kind: model.KindModule, Dir: "."},
		{ID: "example.com/m/p", Kind: model.KindPackage, Dir: "p"},
		{ID: "example.com/m/p.f", Kind: model.KindFunc, Parent: "example.com/m/p", File: "p/a.go", Line: 3, EndLine: 6},
	}}
	// Hand-written profile (pure mapping test).
	p, err := coverage.Parse(strings.NewReader("mode: set\nexample.com/m/p/a.go:3.20,4.10 2 1\nexample.com/m/p/a.go:4.10,6.2 1 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := New().MapCoverage(g, p)["example.com/m/p.f"]
	if got.Stmts != 3 || got.Covered != 2 {
		t.Fatalf("coverage %+v", got)
	}
	// Only module nodes map profile paths: blocks of an external module are
	// dropped, not attributed to a module file with the same relative name.
	g.Nodes = append(g.Nodes,
		model.Node{ID: "other.org/x", Kind: model.KindExternal},
		model.Node{ID: "example.com/m.g", Kind: model.KindFunc, Parent: "example.com/m", File: "c.go", Line: 1, EndLine: 3})
	p, err = coverage.Parse(strings.NewReader("mode: set\nother.org/x/c.go:1.1,2.2 1 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := New().MapCoverage(g, p)["example.com/m.g"]; ok {
		t.Errorf("another module's block was attributed: %+v", c)
	}
}

// Source accepts the whole range of a file, including the first line, the
// last line and a single line.
func TestSourceBounds(t *testing.T) {
	file := "internal/pricing/tier.go"
	b, err := os.ReadFile(filepath.Join(fixtureDir, file))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	p := New()
	for _, r := range [][2]int{{1, 1}, {20, 20}, {1, len(lines)}} {
		got, err := p.Source(context.Background(), fixtureDir, file, r[0], r[1])
		if err != nil || got != strings.Join(lines[r[0]-1:r[1]], "\n") {
			t.Errorf("lines %v: %q %v", r, got, err)
		}
	}
	for _, r := range [][2]int{{0, 1}, {3, 2}, {1, len(lines) + 1}} {
		if _, err := p.Source(context.Background(), fixtureDir, file, r[0], r[1]); err == nil {
			t.Errorf("lines %v: want out of range", r)
		}
	}
}

func TestRootDir(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	link := filepath.Join(dir, "link")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(real)
	if got := rootDir(link, nil); got != want {
		t.Errorf("workspace root through a symlink: %q, want %q", got, want)
	}
	if got := rootDir("/no/such/dir", nil); got != "/no/such/dir" {
		t.Errorf("missing directory: %q", got)
	}
	if got := rootDir(link, []*packages.Module{{Dir: "/mod"}}); got != "/mod" {
		t.Errorf("single module: %q", got)
	}
}

func TestUniqueID(t *testing.T) {
	s := &scanner{ids: map[model.ID]bool{"p.init": true}}
	if got := s.uniqueID("p.f", "p/a.go", 3); got != "p.f" {
		t.Errorf("free id: %q", got)
	}
	if got := s.uniqueID("p.init", "p/a.go", 3); got != "p.init#a.go" {
		t.Errorf("taken id: %q", got)
	}
	s.ids["p.init#a.go"] = true
	if got := s.uniqueID("p.init", "p/a.go", 9); got != "p.init#a.go:9" {
		t.Errorf("taken in the same file: %q", got)
	}
}

func TestTopLevelNil(t *testing.T) {
	if topLevel(nil) != nil {
		t.Error("nil function")
	}
}

func TestMethodOwner(t *testing.T) {
	cases := []struct {
		id   model.ID
		want model.ID
		ok   bool
	}{{"example.com/p.T.M", "example.com/p.T", true}, {"p.M", "p", true}, {".M", "", false}, {"M", "", false}}
	for _, c := range cases {
		if got, ok := methodOwner(c.id); got != c.want || ok != c.ok {
			t.Errorf("methodOwner(%q) = %q %v", c.id, got, ok)
		}
	}
}

func TestExternalNodes(t *testing.T) {
	shown := &scanner{cfg: config.Project{ShowStdlib: true, ExternalModules: "collapsed"}, ids: map[model.ID]bool{}, edges: map[model.Edge]bool{}}
	shown.addExternal("", "fmt", []string{"m/p"})
	shown.addExternal("github.com/acme/kit", "github.com/acme/kit/log", []string{"m/p"})
	shown.addExternal("github.com/acme/kit", "github.com/acme/kit/db", []string{"m/q"})
	var ids []string
	for _, n := range shown.nodes {
		ids = append(ids, string(n.ID)+"="+n.Name)
	}
	if strings.Join(ids, " ") != "fmt=fmt github.com/acme/kit="+externalName("github.com/acme/kit") {
		t.Errorf("nodes %v", ids)
	}
	for _, e := range []model.Edge{{From: "m/p", To: "fmt", Kind: model.EdgeImports}, {From: "m/p", To: "github.com/acme/kit", Kind: model.EdgeImports}, {From: "m/q", To: "github.com/acme/kit", Kind: model.EdgeImports}} {
		if !shown.edges[e] {
			t.Errorf("missing edge %+v", e)
		}
	}
	hidden := &scanner{cfg: config.Project{ExternalModules: "hidden"}, ids: map[model.ID]bool{}, edges: map[model.Edge]bool{}}
	hidden.addExternal("", "fmt", []string{"m/p"})
	hidden.addExternal("github.com/acme/kit", "github.com/acme/kit/log", []string{"m/p"})
	if len(hidden.nodes) != 0 || len(hidden.edges) != 0 {
		t.Errorf("hidden: %v %v", hidden.nodes, hidden.edges)
	}
}

func TestTrimMajor(t *testing.T) {
	cases := map[string]string{"example.com/kit/v2": "example.com/kit", "example.com/kit/v": "example.com/kit/v", "example.com/kit/v2x": "example.com/kit/v2x", "v2": "v2"}
	for in, want := range cases {
		if got := trimMajor(in); got != want {
			t.Errorf("trimMajor(%q) = %q, want %q", in, got, want)
		}
	}
}
