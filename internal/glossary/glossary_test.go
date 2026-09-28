package glossary

import (
	"strings"
	"testing"

	"github.com/morethancoder/qtldr/internal/config"
)

func TestLoadSubstitutesThresholds(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.CrapMax = 12
	g := Load(cfg)
	crap, err := g.Lookup("crap")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(crap.Good, "qtldr target: 12 or less") {
		t.Errorf("crap.good = %q", crap.Good)
	}
	for _, term := range g {
		for _, s := range []string{term.Title, term.Short, term.Body, term.Good} {
			if strings.Contains(s, "{") {
				t.Errorf("%s: unsubstituted placeholder in %q", term.Key, s)
			}
		}
	}
}

func TestEveryTermIsComplete(t *testing.T) {
	g := Load(config.Default())
	if len(g) < 20 {
		t.Fatalf("only %d terms", len(g))
	}
	for k, term := range g {
		if term.Title == "" || term.Short == "" || term.Body == "" || term.Good == "" {
			t.Errorf("%s: missing a field: %+v", k, term)
		}
	}
}

func TestLookup(t *testing.T) {
	g := Load(config.Default())
	for _, name := range []string{"not covered", "Not-Covered", " CRAP "} {
		if _, err := g.Lookup(name); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
	if _, err := g.Lookup("nope"); err == nil || !strings.Contains(err.Error(), "known terms") {
		t.Errorf("unknown term: %v", err)
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	for _, in := range []string{"schema: 2\nterms: {a: {title: x}}\n", "schema: 1\n", "{{"} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%q: want error", in)
		}
	}
}

func TestSubset(t *testing.T) {
	g := Load(config.Default()).Subset("crap", "nope", "coverage")
	if len(g) != 2 || g["crap"].Title == "" {
		t.Fatalf("got %v", g.Keys())
	}
}
