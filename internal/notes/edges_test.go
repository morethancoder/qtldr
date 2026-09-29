package notes

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/morethancoder/qtldr/internal/model"
)

func TestNewLineBounds(t *testing.T) {
	now := time.Now()
	// fn has 7 lines starting at 20: 20..26
	if n, err := New("n", "m/p.f", 20, 20, fn, "first line", "user", now); err != nil || *n.LineOffset != 0 {
		t.Errorf("first line: %+v %v", n, err)
	}
	if n, err := New("n", "m/p.f", 20, 26, fn, "last line", "user", now); err != nil || *n.LineOffset != 6 {
		t.Errorf("last line: %+v %v", n, err)
	}
	for _, line := range []int{19, 27} {
		_, err := New("n", "m/p.f", 20, line, fn, "x", "user", now)
		if want := fmt.Sprintf("line %d is outside m/p.f (lines 20–26)", line); err == nil || err.Error() != want {
			t.Errorf("line %d: %v, want %q", line, err, want)
		}
	}
}

func TestNearest(t *testing.T) {
	x := func(at ...int) []string {
		lines := make([]string, 8)
		for i := range lines {
			lines[i] = "other"
		}
		for _, i := range at {
			lines[i] = "  x  "
		}
		return lines
	}
	cases := []struct {
		name  string
		lines []string
		off   int
		want  int
	}{
		{"no match", x(), 3, -1},
		{"one match", x(6), 0, 6},
		{"first of two is closer", x(0, 5), 1, 0},
		{"second of two is closer", x(0, 5), 4, 5},
		{"tie keeps the earlier", x(1, 5), 3, 1},
		{"exact line wins over an earlier one", x(0, 2), 2, 2},
		{"earlier exact line is kept", x(2, 5), 2, 2},
	}
	for _, c := range cases {
		if got := nearest(c.lines, "x", c.off); got != c.want {
			t.Errorf("%s: nearest = %d, want %d", c.name, got, c.want)
		}
	}
	if abs(-3) != 3 || abs(3) != 3 || abs(0) != 0 {
		t.Error("abs")
	}
}

// writeFunc writes src to root/p/f.go and returns a node for lines first..last.
func writeFunc(t *testing.T, root, src string, first, last int) model.Node {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "p"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "p", "f.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return model.Node{ID: "m/p.f", Kind: model.KindFunc, File: "p/f.go", Line: first, EndLine: last}
}

func TestFuncLines(t *testing.T) {
	root := t.TempDir()
	src := "func f() {\n\treturn\n}\n" // splits into 4 lines, the last empty
	n := writeFunc(t, root, src, 1, 3)
	if got := FuncLines(root, n); !slices.Equal(got, []string{"func f() {", "\treturn", "}"}) {
		t.Errorf("whole function: %q", got)
	}
	n.Line = 2
	if got := FuncLines(root, n); !slices.Equal(got, []string{"\treturn", "}"}) {
		t.Errorf("from line 2: %q", got)
	}
	n.EndLine = 4 // the empty piece after the last newline still counts
	if got := FuncLines(root, n); len(got) != 3 {
		t.Errorf("end at the last split line: %q", got)
	}
	cases := []struct {
		name string
		edit func(*model.Node)
	}{
		{"not a function", func(n *model.Node) { n.Kind = model.KindType }},
		{"no line", func(n *model.Node) { n.Line = 0 }},
		{"past the end of the file", func(n *model.Node) { n.EndLine = 5 }},
		{"missing file", func(n *model.Node) { n.File = "p/none.go" }},
	}
	for _, c := range cases {
		m := writeFunc(t, root, src, 1, 3)
		c.edit(&m)
		if got := FuncLines(root, m); got != nil {
			t.Errorf("%s: %q", c.name, got)
		}
	}
}

func TestAdd(t *testing.T) {
	root := t.TempDir()
	node := writeFunc(t, root, "package p\n\nfunc f() {\n\treturn\n}\n", 3, 5)
	snap := model.Snapshot{Graph: model.Graph{Nodes: []model.Node{node}}}
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	n, err := Add(root, snap, "p.f", 4, "why return", "user", now)
	if err != nil || n.Target != "m/p.f" || n.LineText != "return" {
		t.Fatalf("%+v %v", n, err)
	}
	if all, _ := Load(root); len(all) != 1 || all[0].ID != n.ID {
		t.Errorf("saved: %+v", all)
	}
	if _, err := Add(root, snap, "nope", 0, "x", "user", now); err == nil {
		t.Error("unknown target")
	}
	if _, err := Add(root, snap, "p.f", 0, " ", "user", now); err == nil {
		t.Error("empty text")
	}
	if all, _ := Load(root); len(all) != 1 {
		t.Errorf("failed adds must not save: %d notes", len(all))
	}
}
