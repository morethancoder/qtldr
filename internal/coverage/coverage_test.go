package coverage

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/morethancoder/qtldr/internal/model"
)

// Hand-written profiles (pure parser tests; allowed by CLAUDE.md rule 3).
const profileText = `mode: count
example.com/m/p/a.go:3.20,4.10 1 1
example.com/m/p/a.go:4.10,6.3 1 0
example.com/m/p/a.go:7.2,7.12 1 2
example.com/m/p/a.go:3.20,4.10 1 3
example.com/m/p/b.go:1.1,1.5 2 0
other.org/x/c.go:1.1,2.2 1 1
`

func parse(t *testing.T, s string) Profile {
	t.Helper()
	p, err := Parse(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseMergesDuplicates(t *testing.T) {
	p := parse(t, profileText)
	if p.Mode != "count" || len(p.Blocks) != 5 {
		t.Fatalf("mode %q, %d blocks", p.Mode, len(p.Blocks))
	}
	if p.Blocks[0].Count != 4 {
		t.Errorf("duplicate block count = %d, want 1+3", p.Blocks[0].Count)
	}
	want := Block{File: "example.com/m/p/a.go", StartLine: 4, StartCol: 10, EndLine: 6, EndCol: 3, NumStmt: 1}
	if p.Blocks[1] != want {
		t.Errorf("block = %+v", p.Blocks[1])
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{"a.go:1.1,2.2 1 1\n", "mode: set\na.go 1 1\n", "mode: set\na.go:1.x,2.2 1 1\n"} {
		if _, err := Parse(strings.NewReader(in)); err == nil {
			t.Errorf("%q: want error", in)
		}
	}
}

func TestMap(t *testing.T) {
	p := parse(t, profileText).Relativize("example.com/m")
	if files := p.Files(); !slices.Equal(files, []string{"p/a.go", "p/b.go"}) {
		t.Fatalf("files %v", files)
	}
	got := Map(p, []Func{
		{ID: "f", File: "p/a.go", Line: 3, EndLine: 8},
		{ID: "empty", File: "p/b.go", Line: 3, EndLine: 3},
		{ID: "unknown", File: "p/z.go", Line: 1, EndLine: 9},
	})
	f := got["f"]
	if f.Stmts != 3 || f.Covered != 2 || *f.Percent != 66.7 {
		t.Errorf("f = %+v", f)
	}
	// line 4 is shared by a run block and a missed block → partial
	if !slices.Equal(f.Lines.Covered, []int{3, 7}) || !slices.Equal(f.Lines.Partial, []int{4}) || !slices.Equal(f.Lines.Uncovered, []int{5, 6}) {
		t.Errorf("lines %+v", f.Lines)
	}
	if e := got["empty"]; e.Stmts != 0 || e.Percent != nil {
		t.Errorf("function with no blocks inside = %+v, want zero statements", e)
	}
	if _, ok := got["unknown"]; ok {
		t.Error("a file missing from the profile is not measured")
	}
}

func TestWithout(t *testing.T) {
	p := parse(t, profileText).Relativize("example.com/m").Without([]string{"p"})
	if len(p.Blocks) != 0 {
		t.Fatalf("blocks left: %v", p.Blocks)
	}
}

func TestRanges(t *testing.T) {
	got := Ranges([]int{25, 37, 41, 42, 43, 44})
	if !slices.Equal(got, []string{"25", "37", "41–44"}) {
		t.Fatalf("got %v", got)
	}
}

func TestCacheUpdateResolve(t *testing.T) {
	g := model.Graph{Nodes: []model.Node{
		{ID: "m/p", Kind: model.KindPackage},
		{ID: "m/p.f", Kind: model.KindFunc, Parent: "m/p", Line: 10, BodyHash: "h1"},
		{ID: "m/p.g", Kind: model.KindFunc, Parent: "m/p", Line: 20, BodyHash: "h2"},
		{ID: "m/q.h", Kind: model.KindFunc, Parent: "m/q", Line: 1, BodyHash: "h3"},
	}}
	at := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	measured := map[model.ID]model.Coverage{
		"m/p.f": {Stmts: 4, Covered: 3, Lines: &model.LineStates{Covered: []int{11, 12}, Uncovered: []int{13}}},
	}
	c := NewCache()
	c.Functions["m/p.gone"] = Entry{}
	c.Update(g, measured, []model.ID{"m/p", "m/q"}, map[model.ID]string{"m/q": "tests failed"}, at)
	if _, ok := c.Functions["m/p.gone"]; ok {
		t.Error("entries of re-run packages are replaced")
	}
	if c.PackageErrors["m/q"] != "tests failed" {
		t.Errorf("errors %v", c.PackageErrors)
	}
	// f moved down 5 lines and g changed.
	g.Nodes[1].Line = 15
	g.Nodes[2].BodyHash = "h2-changed"
	res := c.Resolve(g)
	f := res["m/p.f"]
	if f.Stale || *f.Percent != 75 || !slices.Equal(f.Lines.Covered, []int{16, 17}) || !slices.Equal(f.Lines.Uncovered, []int{18}) {
		t.Errorf("f = %+v lines %+v", f, f.Lines)
	}
	if gc := res["m/p.g"]; !gc.Stale || gc.Stmts != 0 || gc.Percent != nil {
		t.Errorf("g = %+v, want stale with no statements", gc)
	}
	if _, ok := res["m/q.h"]; ok {
		t.Error("functions of a failed package are not measured")
	}
}

func TestBuildArgs(t *testing.T) {
	cases := []struct {
		tmpl, coverpkg string
		want           string
		err            bool
	}{
		{"go test -covermode=count -coverprofile={profile} {packages}", "own", "go test -covermode=count -coverprofile=/p.out ./a ./b", false},
		{"go test -coverprofile={profile} {packages}", "module", "go test -coverprofile=/p.out -coverpkg=./... ./a ./b", false},
		{`go test -tags "a b" -coverprofile={profile} {packages}`, "own", "go test -tags a b -coverprofile=/p.out ./a ./b", false},
		{"go test -coverprofile={profile}", "own", "", true},
		{`go test "unclosed {packages}`, "own", "", true},
	}
	for _, c := range cases {
		got, err := BuildArgs(c.tmpl, c.coverpkg, "/p.out", []string{"./a", "./b"})
		if (err != nil) != c.err {
			t.Errorf("%q: err %v", c.tmpl, err)
			continue
		}
		if !c.err && strings.Join(got, " ") != c.want {
			t.Errorf("%q: got %q", c.tmpl, strings.Join(got, " "))
		}
	}
	if got, _ := BuildArgs(`go test -tags "a b" {packages}`, "own", "", nil); got[3] != "a b" {
		t.Errorf("quoted word split: %q", got)
	}
}

func TestFailedPackages(t *testing.T) {
	out := "ok  \texample.com/m/a\t0.1s\tcoverage: 80.0% of statements\n--- FAIL: TestX (0.00s)\nFAIL\nFAIL\texample.com/m/b\t0.2s\nFAIL\texample.com/m/c [build failed]\n?   \texample.com/m/d\t[no test files]\nFAIL\n"
	got := FailedPackages(out)
	if !slices.Equal(got, []string{"example.com/m/b", "example.com/m/c"}) {
		t.Fatalf("got %v", got)
	}
}
