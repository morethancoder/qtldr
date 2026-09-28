package notes

import (
	"strings"
	"testing"
	"time"
)

var fn = []string{
	"func applyTiered(tiers []Tier) error {",
	"\tif len(tiers) == 0 {",
	"\t\treturn ErrNoTiers",
	"\t}",
	"\tswitch best.Kind {",
	"\t}",
	"}",
}

func TestNewAndPlace(t *testing.T) {
	now := time.Date(2026, 9, 26, 9, 12, 0, 0, time.UTC)
	n, err := New("n_1", "m/p.f", 20, 24, fn, " Split by Kind. ", "user", now)
	if err != nil || *n.LineOffset != 4 || n.LineText != "switch best.Kind {" || n.Text != "Split by Kind." {
		t.Fatalf("%+v %v", n, err)
	}
	if p := Place(n, 20, fn); p.Line != 24 || p.Outdated {
		t.Errorf("unchanged: %+v", p)
	}
	moved := append([]string{fn[0], "\t// new comment", "\t// another"}, fn[1:]...)
	if p := Place(n, 30, moved); p.Line != 36 || p.Outdated {
		t.Errorf("shifted inside the function: %+v", p)
	}
	changed := append([]string{}, fn...)
	changed[4] = "\tswitch best.Type {"
	if p := Place(n, 20, changed); !p.Outdated || p.Line != 24 {
		t.Errorf("text gone: %+v", p)
	}
	if p := Place(n, 20, fn[:2]); !p.Outdated || p.Line != 21 {
		t.Errorf("function shrank: %+v", p)
	}
	pkg, err := New("n_2", "m/p", 0, 0, nil, "package note", "agent:claude-code", now)
	if err != nil || pkg.LineOffset != nil || Place(pkg, 0, nil).Line != 0 {
		t.Errorf("package note: %+v %v", pkg, err)
	}
	if _, err := New("n_3", "m/p.f", 20, 99, fn, "x", "user", now); err == nil {
		t.Error("line outside the function")
	}
	if _, err := New("n_3", "m/p.f", 20, 0, fn, "  ", "user", now); err == nil {
		t.Error("empty text")
	}
}

func TestULID(t *testing.T) {
	a := ulid(time.UnixMilli(1469918176385), [10]byte{})
	if a != "01ARYZ6S410000000000000000" {
		t.Errorf("got %s", a)
	}
	id1, id2 := NewID(time.Now()), NewID(time.Now().Add(time.Millisecond))
	if len(id1) != 28 || !strings.HasPrefix(id1, "n_") || id1 >= id2 {
		t.Errorf("ids %s %s", id1, id2)
	}
}

func TestStore(t *testing.T) {
	root := t.TempDir()
	if all, err := Load(root); err != nil || len(all) != 0 {
		t.Fatalf("%v %v", all, err)
	}
	now := time.Now().UTC()
	for i, text := range []string{"b", "a"} {
		if err := Append(root, Note{ID: "n_" + text, Target: "m/p.f", Text: text, Created: now.Add(time.Duration(-i) * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Update(root, "n_a", func(n *Note) { n.Resolved = true }); err != nil {
		t.Fatal(err)
	}
	all, _ := Load(root)
	if len(all) != 2 || all[0].ID != "n_a" || !all[0].Resolved {
		t.Fatalf("%+v", all)
	}
	if got := ForTarget(all, "m/p.f"); len(got) != 1 || got[0].ID != "n_b" {
		t.Errorf("open notes %+v", got)
	}
	if _, err := Update(root, "nope", func(*Note) {}); err == nil {
		t.Error("unknown id")
	}
}
