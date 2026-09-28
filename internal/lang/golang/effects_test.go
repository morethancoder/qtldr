package golang

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

func TestWriteTargets(t *testing.T) {
	x := ast.NewIdent("x")
	cases := []struct {
		name string
		n    ast.Node
		want int
	}{
		{"assign", &ast.AssignStmt{Lhs: []ast.Expr{x}, Tok: token.ASSIGN}, 1},
		{"define is not a write to an existing var", &ast.AssignStmt{Lhs: []ast.Expr{x}, Tok: token.DEFINE}, 0},
		{"inc", &ast.IncDecStmt{X: x}, 1},
		{"address", &ast.UnaryExpr{Op: token.AND, X: x}, 1},
		{"negation", &ast.UnaryExpr{Op: token.SUB, X: x}, 0},
		{"range assign", &ast.RangeStmt{Key: x, Value: x, Tok: token.ASSIGN}, 2},
		{"range define", &ast.RangeStmt{Key: x, Tok: token.DEFINE}, 0},
		{"other", &ast.ReturnStmt{}, 0},
	}
	for _, c := range cases {
		if got := len(writeTargets(c.n)); got != c.want {
			t.Errorf("%s: %d targets, want %d", c.name, got, c.want)
		}
	}
	if targetIdent(&ast.SelectorExpr{X: ast.NewIdent("p"), Sel: x}) != x || targetIdent(&ast.StarExpr{X: x}) != nil {
		t.Error("targetIdent")
	}
}

// named makes a named type in a fake package (pure; no loading).
func named(path, name string) *types.Named {
	pkg := types.NewPackage(path, path)
	return types.NewNamed(types.NewTypeName(token.NoPos, pkg, name, nil), types.NewStruct(nil, nil), nil)
}

func TestTypeEffect(t *testing.T) {
	ctx, reader, logger := named("context", "Context"), named("io", "Reader"), named("log", "Logger")
	money, logBuf := named("example.com/money", "Amount"), named("log", "Buffer")
	sig := types.NewSignatureType(nil, nil, nil, types.NewTuple(types.NewVar(token.NoPos, nil, "", ctx)), nil, false)
	cases := []struct {
		name string
		t    types.Type
		want string
	}{
		{"context", ctx, "context.Context"},
		{"pointer to slice of readers", types.NewPointer(types.NewSlice(reader)), "io.Reader"},
		{"array", types.NewArray(reader, 2), "io.Reader"},
		{"map value", types.NewMap(types.Typ[types.String], types.NewPointer(logger)), "log.Logger"},
		{"func param", sig, "context.Context"},
		{"channel", types.NewChan(types.SendRecv, types.Typ[types.Int]), "a channel"},
		{"domain type", types.NewSlice(money), ""},
		{"other log type", logBuf, ""},
		{"basic", types.Typ[types.Int], ""},
	}
	for _, c := range cases {
		if got := typeEffect(c.t, 0); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestRecvTypeName(t *testing.T) {
	cases := map[string]string{
		"func (T) f() {}":          "T",
		"func (t *T) f() {}":       "T",
		"func (t *(T)) f() {}":     "T",
		"func (s S[K]) f() {}":     "S",
		"func (s *M[K, V]) f() {}": "M",
		"func f() {}":              "",
	}
	for src, want := range cases {
		_, fn := parseFunc(t, src)
		if got, _ := receiver(fn); got != want {
			t.Errorf("%s: %q, want %q", src, got, want)
		}
	}
	if recvTypeName(&ast.ArrayType{}) != "" {
		t.Error("unknown receiver expression")
	}
}

func TestSource(t *testing.T) {
	p := New()
	got, err := p.Source(context.Background(), fixtureDir, "internal/pricing/tier.go", 20, 21)
	if err != nil || got != "func applyTiered(tiers []Tier, qty int, unit money.Amount) (money.Amount, error) {\n\tif len(tiers) == 0 {" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := p.Source(context.Background(), fixtureDir, "internal/pricing/tier.go", 40, 1000); err == nil {
		t.Error("out of range")
	}
	if _, err := p.Source(context.Background(), fixtureDir, "nope.go", 1, 1); err == nil {
		t.Error("missing file")
	}
	if !p.Detect(fixtureDir) || p.Detect(t.TempDir()) || p.Name() != "go" {
		t.Error("Detect/Name")
	}
}

func TestConcurrencyEffects(t *testing.T) {
	src := `package x

func f(c chan int, done chan struct{}) {
	go f(c, done)
	c <- 1
	<-c
	select {
	case <-done:
	default:
	}
	for range c {
	}
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	if _, err := (&types.Config{}).Check("x", fset, []*ast.File{file}, info); err != nil {
		t.Fatal(err)
	}
	found := map[string]int{}
	ast.Inspect(file, func(n ast.Node) bool {
		if r := concurrencyEffect(info, n); r != "" {
			found[r]++
		}
		return true
	})
	if found["go statement"] != 1 || found["channel send"] != 1 || found["select"] != 1 || found["channel receive"] != 3 {
		t.Fatalf("found %v", found)
	}
}
