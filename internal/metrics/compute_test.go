package metrics

import (
	"testing"

	"github.com/morethancoder/qtldr/internal/model"
)

// graph: module m; package p with funcs a (calls b), b (local effect), c
// (calls d through interface p.I), d = p.T.M implementing p.I; package q with
// pure func e; package r with only a type.
func testGraph(dEffects []string) model.Graph {
	return model.Graph{
		Nodes: []model.Node{
			{ID: "m", Kind: model.KindModule},
			{ID: "m/p", Kind: model.KindPackage, Parent: "m", Dir: "p"},
			{ID: "m/p.a", Kind: model.KindFunc, Parent: "m/p", File: "p/a.go"},
			{ID: "m/p.b", Kind: model.KindFunc, Parent: "m/p", File: "p/a.go", Effects: []string{"calls os.Getenv"}},
			{ID: "m/p.c", Kind: model.KindFunc, Parent: "m/p", File: "p/c.go"},
			{ID: "m/p.T", Kind: model.KindType, Parent: "m/p"},
			{ID: "m/p.I", Kind: model.KindType, Parent: "m/p"},
			{ID: "m/p.T.M", Kind: model.KindFunc, Parent: "m/p", File: "p/c.go", Effects: dEffects},
			{ID: "m/q", Kind: model.KindPackage, Parent: "m", Dir: "q"},
			{ID: "m/q.e", Kind: model.KindFunc, Parent: "m/q", File: "q/e.go"},
			{ID: "m/r", Kind: model.KindPackage, Parent: "m", Dir: "r"},
		},
		Edges: []model.Edge{
			{From: "m/p.a", To: "m/p.b", Kind: model.EdgeCalls},
			{From: "m/p.c", To: "m/p.I.M", Kind: model.EdgeCallsDynamic},
			{From: "m/p.T", To: "m/p.I", Kind: model.EdgeImplements},
		},
		Metrics: map[model.ID]model.Metrics{
			"m/p.a": {CC: model.Ptr(5)}, "m/p.b": {CC: model.Ptr(11)}, "m/p.c": {CC: model.Ptr(1)},
			"m/p.T.M": {CC: model.Ptr(1)}, "m/q.e": {CC: model.Ptr(2)},
		},
	}
}

func TestPurityPropagation(t *testing.T) {
	cases := []struct {
		name     string
		dEffects []string
		allow    []model.ID
		pure     map[model.ID]bool
	}{
		{"implementation pure", nil, nil, map[model.ID]bool{"m/p.a": false, "m/p.b": false, "m/p.c": true, "m/p.T.M": true, "m/q.e": true}},
		{"implementation effectful", []string{"go statement"}, nil, map[model.ID]bool{"m/p.c": false, "m/p.T.M": false}},
		{"allow overrides and stops propagation", nil, []model.ID{"m/p.b"}, map[model.ID]bool{"m/p.a": true, "m/p.b": true}},
	}
	for _, c := range cases {
		res := Purity(testGraph(c.dEffects), c.allow)
		for id, want := range c.pure {
			if res[id].Pure != want {
				t.Errorf("%s: %s pure = %v, want %v (effects %v)", c.name, id, res[id].Pure, want, res[id].Effects)
			}
		}
	}
	res := Purity(testGraph(nil), nil)
	if got := res["m/p.a"].Effects; len(got) != 1 || got[0] != "calls p.b (effectful)" {
		t.Errorf("a effects = %v", got)
	}
}

func TestDynamicCallWithoutImplementationIsEffectful(t *testing.T) {
	g := testGraph(nil)
	g.Edges = g.Edges[:2] // drop implements
	if Purity(g, nil)["m/p.c"].Pure {
		t.Error("an interface with no known implementation must be effectful")
	}
}

func TestComputeRollUp(t *testing.T) {
	in := Inputs{
		Coverage: map[model.ID]model.Coverage{
			"m/p.a":   {Stmts: 6, Covered: 0, Percent: f(0)},
			"m/p.b":   {Stmts: 17, Covered: 12, Percent: f(70.6), Stale: true},
			"m/p.T.M": {Stmts: 0},
			"m/q.e":   {Stmts: 4, Covered: 4, Percent: f(100)},
		},
		CoverageErrors: map[model.ID]string{"m/q": "tests failed"},
		FileChurn:      map[string]int{"p/a.go": 7},
		PackageChurn:   map[string]int{"p": 9},
	}
	g := Compute(testGraph(nil), in)
	a := g.Metrics["m/p.a"]
	if a.CRAP == nil || *a.CRAP != 30 || a.Grades.CRAP != 2 || *a.Churn != 7 || a.ChurnScope != "file" {
		t.Fatalf("a: %+v grades %+v", a, a.Grades)
	}
	if c := g.Metrics["m/p.c"]; c.CRAP != nil || c.Grades.CRAP != 1 {
		t.Errorf("c has no coverage: CRAP must be nil and graded 1, got %+v", c)
	}
	p := g.Metrics["m/p"]
	if *p.CrapMax != 30 || p.Worst != "m/p.a" || p.Grades.CRAP != 1 || *p.Churn != 9 {
		t.Errorf("p roll-up: max %v worst %v grades %+v churn %v", *p.CrapMax, p.Worst, p.Grades, *p.Churn)
	}
	// statement-weighted: (0 + 12 + 0) / (6 + 17 + 0) = 52.2%
	if *p.Coverage.Percent != 52.2 || !p.Coverage.Stale {
		t.Errorf("p coverage %+v", p.Coverage)
	}
	if q := g.Metrics["m/q"]; q.CoverageError != "tests failed" {
		t.Errorf("q error %q", q.CoverageError)
	}
	if r := g.Metrics["m/r"]; r.Grades != nil {
		t.Errorf("package without functions has no grades, got %+v", r.Grades)
	}
	if mod := g.Metrics["m"]; mod.Grades.CRAP != 1 || mod.Worst != "m/p.a" {
		t.Errorf("module %+v", mod)
	}
	pure := map[model.ID]bool{}
	for _, n := range g.Nodes {
		if n.Pure != nil {
			pure[n.ID] = *n.Pure
		}
	}
	if pure["m/p"] || !pure["m/q"] || !pure["m/r"] {
		t.Errorf("package purity %v", pure)
	}
}
