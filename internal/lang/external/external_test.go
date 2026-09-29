package external

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/model"
)

// Hand-written contract inputs (pure validator tests).
const valid = `{"nodes": [
  {"id": "py:app", "kind": "package", "name": "app"},
  {"id": "py:app.run", "kind": "func", "name": "run", "parent": "py:app", "file": "app/run.py", "line": 3, "end_line": 9}
], "edges": [{"from": "py:app.run", "to": "py:app.run", "kind": "calls"}],
"metrics": {"py:app.run": {"cc": 3, "coverage": {"stmts": 4, "covered": 3, "percent": 75, "stale": false}}}}`

func TestValidate(t *testing.T) {
	g, problems := Validate([]byte(valid))
	if len(problems) != 0 || len(g.Nodes) != 2 || *g.Metrics["py:app.run"].CC != 3 {
		t.Fatalf("%v %+v", problems, g)
	}
	cases := []struct {
		name, json string
		want       []string
	}{
		{"not json", `[`, []string{"$: not a JSON object"}},
		{"no nodes", `{"edges": []}`, []string{"$.nodes: is required"}},
		{"bad node", `{"nodes": [{"id": "a", "kind": "class", "name": ""}, {"id": "a", "kind": "func", "name": "f", "file": "/abs.py", "line": 0}]}`,
			[]string{`$.nodes[0].kind: must be one of`, `$.nodes[0].name: is required`, `$.nodes[1].id: duplicate id "a"`, `$.nodes[1].file: must be a path relative to root`, `$.nodes[1].line: needs line ≥ 1`}},
		{"unknown field", `{"nodes": [{"id": "a", "kind": "package", "name": "a", "colour": "red"}]}`, []string{`$.nodes[0]: json: unknown field "colour"`}},
		{"bad parent and edge", `{"nodes": [{"id": "a", "kind": "package", "name": "a", "parent": "zz"}], "edges": [{"from": "a", "to": "b", "kind": "uses"}]}`,
			[]string{`$.nodes[0].parent: unknown node "zz"`, `$.edges[0].kind: must be one of`, `$.edges[0].to: unknown node "b"`}},
		{"malformed metrics", `{"nodes": [{"id": "a", "kind": "package", "name": "a"}], "metrics": {"a": {"cc": "high"}}}`, []string{`$.metrics["a"]: json: cannot unmarshal`}},
		{"malformed edge", `{"nodes": [{"id": "a", "kind": "package", "name": "a"}], "edges": [{"from": 1}]}`, []string{`$.edges[0]: json: cannot unmarshal`}},
		{"bad metrics", `{"nodes": [{"id": "a", "kind": "package", "name": "a"}], "metrics": {"a": {"cc": 0, "coverage": {"stmts": 1, "covered": 2, "percent": 200, "stale": false}, "mutation": {"killed": 0, "survived": 0, "not_covered": 0, "timed_out": 0, "score": 120, "stale": false}}, "b": {}}}`,
			[]string{`$.metrics["a"].cc: must be ≥ 1`, `$.metrics["a"].coverage:`, `$.metrics["a"].mutation.score:`, `$.metrics["b"]: unknown node "b"`}},
	}
	for _, c := range cases {
		_, problems := Validate([]byte(c.json))
		var got []string
		for _, p := range problems {
			got = append(got, p.String())
		}
		for _, w := range c.want {
			if !strings.Contains(strings.Join(got, "\n"), w) {
				t.Errorf("%s: missing %q in\n%s", c.name, w, strings.Join(got, "\n"))
			}
		}
	}
}

// Valid edges reach the graph; an unknown source is a problem of its own.
func TestValidateEdges(t *testing.T) {
	g, problems := Validate([]byte(valid))
	if len(problems) != 0 || len(g.Edges) != 1 || g.Edges[0].Kind != model.EdgeCalls {
		t.Errorf("edges %+v problems %v", g.Edges, problems)
	}
	_, problems = Validate([]byte(`{"nodes": [{"id": "a", "kind": "package", "name": "a"}],
"edges": [{"from": "a", "to": "a", "kind": "imports"}, {"from": "zz", "to": "a", "kind": "imports"}]}`))
	if len(problems) != 1 || problems[0].String() != `$.edges[1].from: unknown node "zz"` {
		t.Errorf("problems %v", problems)
	}
}

// The range checks include their bounds.
func TestValidationBounds(t *testing.T) {
	if _, problems := Validate([]byte(strings.Replace(valid, `"cc": 3`, `"cc": 1`, 1))); len(problems) != 0 {
		t.Errorf("cc 1 is the minimum: %v", problems)
	}
	pct := func(v float64) *float64 { return &v }
	for _, c := range []struct {
		p    *float64
		want bool
	}{{nil, true}, {pct(0), true}, {pct(100), true}, {pct(-0.1), false}, {pct(100.1), false}} {
		if got := percentOK(c.p); got != c.want {
			t.Errorf("percentOK(%v) = %v", c.p, got)
		}
	}
	if !validCoverage(&model.Coverage{Stmts: 3, Covered: 3}) || validCoverage(&model.Coverage{Stmts: 3, Covered: 4}) || !validCoverage(nil) {
		t.Error("validCoverage: covered may equal stmts, not exceed it")
	}
	lines := []struct {
		n    model.Node
		want bool
	}{
		{model.Node{Kind: model.KindFunc, Line: 4, EndLine: 4}, true}, // one-line function
		{model.Node{Kind: model.KindFunc, Line: 4, EndLine: 3}, false},
		{model.Node{Kind: model.KindType, Line: 4}, true}, // types need no end line
		{model.Node{Kind: model.KindFunc, Line: 0, EndLine: 3}, false},
		{model.Node{Kind: model.KindType, Line: 1}, true},
	}
	for _, c := range lines {
		if got := validLines(c.n); got != c.want {
			t.Errorf("validLines(%+v) = %v", c.n, got)
		}
	}
}

func TestMerge(t *testing.T) {
	g := model.Graph{Nodes: []model.Node{{ID: "m", Kind: model.KindModule}, {ID: "py:app", Kind: model.KindPackage}}, Metrics: map[model.ID]model.Metrics{}}
	add, _ := Validate([]byte(valid))
	problems := Merge(&g, add, "py", "m")
	if len(problems) != 1 || !strings.Contains(problems[0].Message, "collides") {
		t.Fatalf("problems %v", problems)
	}
	fn, ok := g.Node("py:app.run")
	if !ok || len(fn.Effects) != 1 || !strings.Contains(fn.Effects[0], "purity not known") {
		t.Fatalf("run %+v", fn)
	}
	g2 := model.Graph{Nodes: []model.Node{{ID: "m", Kind: model.KindModule}}, Metrics: map[model.ID]model.Metrics{}}
	Merge(&g2, add, "py", "m")
	if pkg, _ := g2.Node("py:app"); pkg.Parent != "m" {
		t.Errorf("parent %q", pkg.Parent)
	}
	if nm := NotMeasured("py", "m", []string{"x"}); nm.ID != "provider:py" || nm.Errors[0] != "x" {
		t.Errorf("%+v", nm)
	}
}

func buildExample(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "example-provider")
	src, _ := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "provider", "example"))
	if out, err := exec.Command("go", "build", "-o", bin, src).CombinedOutput(); err != nil {
		t.Fatalf("build example provider: %v %s", err, out)
	}
	return bin
}

func TestScanWithExampleProvider(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"a/one.txt", "a/two.txt", "b/three.txt", ".hidden/x.txt", "node_modules/y.txt", "a/skip.md"} {
		writeFile(t, filepath.Join(root, f))
	}
	files, err := Files(root, []string{".txt"}, []string{"**/two.txt"}, func(p []string, f string) bool { return len(p) > 0 && strings.HasSuffix(f, "two.txt") })
	if err != nil || strings.Join(files, ",") != "a/one.txt,b/three.txt" {
		t.Fatalf("files %v %v", files, err)
	}
	p := config.Provider{Name: "txt", Command: buildExample(t), Extensions: []string{".txt"}}
	g, problems, err := Scan(context.Background(), root, p, nil, func([]string, string) bool { return false })
	if err != nil || len(problems) != 0 || len(g.Nodes) != 5 {
		t.Fatalf("%v %v %+v", err, problems, g.Nodes)
	}
	broken := config.Provider{Name: "bad", Command: `sh -c 'echo "{\"nodes\": [{\"id\": \"x\", \"kind\": \"class\", \"name\": \"x\"}]}"'`, Extensions: []string{".txt"}}
	if _, problems, err := Scan(context.Background(), root, broken, nil, func([]string, string) bool { return false }); err != nil || len(problems) != 1 {
		t.Fatalf("broken: %v %v", err, problems)
	}
	failing := config.Provider{Name: "fail", Command: `sh -c 'echo boom >&2; exit 3'`, Extensions: []string{".txt"}}
	if _, _, err := Scan(context.Background(), root, failing, nil, func([]string, string) bool { return false }); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("failing: %v", err)
	}
}

func writeFile(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}
