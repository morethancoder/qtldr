package mcpserver

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/glossary"
	"github.com/morethancoder/qtldr/internal/model"
)

func TestLineText(t *testing.T) {
	lines := []string{"a", "\tb ", "c"}
	cases := []struct {
		line int
		want string
	}{{9, ""}, {10, "a"}, {11, "b"}, {12, "c"}, {13, ""}}
	for _, c := range cases {
		if got := lineText(lines, 10, c.line); got != c.want {
			t.Errorf("lineText(line %d) = %q, want %q", c.line, got, c.want)
		}
	}
}

func TestSelectedText(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("func A() {\n\treturn\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	n := model.Node{Kind: model.KindFunc, File: "a.go", Line: 1, EndLine: 3}
	cases := []struct {
		line int
		want string
	}{{0, ""}, {1, "func A() {"}, {2, "return"}, {4, ""}}
	for _, c := range cases {
		if got := selectedText(root, n, c.line); got != c.want {
			t.Errorf("selectedText(line %d) = %q, want %q", c.line, got, c.want)
		}
	}
}

func TestNodeViewLinesAndWorst(t *testing.T) {
	g := glossary.Load(config.Default())
	fn := model.Detail{Node: model.Node{ID: "m/p.f", Kind: model.KindFunc, Name: "f", Line: 5, EndLine: 9}, Metrics: &model.Metrics{}}
	if v := nodeView(fn, config.Default().Thresholds, g, nil); v.Lines != "5-9" || v.Worst != "" {
		t.Errorf("function: lines %q worst %q", v.Lines, v.Worst)
	}
	pkg := model.Detail{Node: model.Node{ID: "github.com/acme/ledger/internal/pricing", Kind: model.KindPackage, Name: "pricing"},
		Metrics: &model.Metrics{Worst: "github.com/acme/ledger/internal/pricing.applyTiered"}}
	if v := nodeView(pkg, config.Default().Thresholds, g, nil); v.Lines != "" || v.Worst != "pricing.applyTiered" {
		t.Errorf("package: lines %q worst %q", v.Lines, v.Worst)
	}
}

func TestPackageRow(t *testing.T) {
	id := model.ID("github.com/acme/ledger/internal/pricing")
	snap := model.Snapshot{Graph: model.Graph{Metrics: map[model.ID]model.Metrics{id: {
		Coverage: &model.Coverage{Percent: model.Ptr(50.0)}, Mutation: &model.Mutation{Score: model.Ptr(75.0)},
		Worst: id + ".applyTiered", CoverageError: "tests failed in pricing",
	}}}}
	r := packageRow(snap, model.Node{ID: id, Name: "pricing", Pure: model.Ptr(true)})
	if !r.Pure || *r.Coverage != 50 || *r.Mutation != 75 || r.Riskiest != "pricing.applyTiered" || fmt.Sprint(r.Problems) != "[tests failed in pricing]" {
		t.Errorf("measured package: %+v", r)
	}
	r = packageRow(snap, model.Node{ID: "m/q", Name: "q"})
	if r.Pure || r.Coverage != nil || r.Mutation != nil || r.Riskiest != "" || r.Problems != nil {
		t.Errorf("unmeasured package: %+v", r)
	}
}

func TestScopeIDs(t *testing.T) {
	g := model.Graph{Nodes: []model.Node{
		{ID: "m", Kind: model.KindModule},
		{ID: "m/p", Kind: model.KindPackage, Parent: "m"},
		{ID: "m/p.a", Kind: model.KindFunc, Parent: "m/p"},
		{ID: "m/p.b", Kind: model.KindFunc, Parent: "m/p"},
	}}
	cases := []struct{ id, want string }{{"m", "[m/p.a m/p.b]"}, {"m/p.a", "[m/p.a]"}, {"m/p", "[m/p.a m/p.b]"}}
	for _, c := range cases {
		if got, err := scopeIDs(g, c.id); err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("scopeIDs(%s) = %v %v, want %s", c.id, got, err, c.want)
		}
	}
	if _, err := scopeIDs(g, "nope"); err == nil {
		t.Error("unknown id")
	}
}
