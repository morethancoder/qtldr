package source

import (
	"strings"
	"testing"
	"time"

	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/notes"
)

const tier = `// applyTiered prices a line
func applyTiered(tiers []Tier, qty int) error {
	if qty <= 0 {
		return ErrBadQty
	}
	switch best.Kind {
	case Fixed:
		price = money.Sub(price, off)
	case Capped:
		if price > best.Cap {
			price = best.Cap
		}
	}
	return nil
}`

func TestAnnotate(t *testing.T) {
	n := model.Node{ID: "m/p.applyTiered", File: "p/tier.go", Line: 11}
	m := model.Metrics{
		Coverage: &model.Coverage{Stale: true, Lines: &model.LineStates{
			Covered: []int{11, 12, 15}, Uncovered: []int{13, 14, 17, 19, 20, 21}, Partial: []int{}}},
		Mutation: &model.Mutation{Mutants: []model.Mutant{
			{Line: 12, Status: "LIVED", Type: "CONDITIONALS_BOUNDARY", Description: "<= → <"},
			{Line: 12, Status: "LIVED", Type: "CONDITIONALS_NEGATION", Description: "<= → >"},
			{Line: 12, Status: "KILLED", Description: "x"},
			{Line: 24, Status: "LIVED", Description: "outside"},
		}},
	}
	created := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	placed := []notes.Placed{{Note: notes.Note{ID: "n_1", Author: "user", Text: "Split by Kind.", Created: created}, Line: 15}}
	s := Annotate(n, m, placed, tier, 10)
	if s.From != 10 || s.To != 24 || len(s.Lines) != 15 || !s.CoverageStale {
		t.Fatalf("range %d–%d, %d lines", s.From, s.To, len(s.Lines))
	}
	at := func(line int) Line { return s.Lines[line-10] }
	if l := at(12); l.State != "covered" || l.Survived != 2 || len(l.Annotations) != 1 ||
		l.Annotations[0].Title != "2 mutants survived" || l.Annotations[0].Detail != "<= → <   ·   <= → >" ||
		l.Annotations[0].Why != "No test puts the value exactly on the boundary." {
		t.Errorf("line 12: %+v", l)
	}
	if a := at(13).Annotations; len(a) != 1 || a[0].Kind != NotCovered || a[0].Detail != "if qty <= 0" || a[0].Why != "lines 13–14" {
		t.Errorf("line 13: %+v", a)
	}
	// 17 and 19–21 are one run: 18 ("case Capped:") has no state.
	if a := at(17).Annotations; len(a) != 1 || a[0].Detail != "case Fixed:" || a[0].Why != "lines 17–21" {
		t.Errorf("line 17: %+v", a)
	}
	if len(at(19).Annotations) != 0 {
		t.Errorf("line 19 continues the run: %+v", at(19).Annotations)
	}
	if a := at(15).Annotations; len(a) != 1 || a[0].Kind != Note || a[0].Title != "Note from you" || a[0].Why != "Split by Kind." {
		t.Errorf("line 15: %+v", a)
	}
}

func TestSurvivorWhyPrefersNote(t *testing.T) {
	n := model.Node{Line: 1}
	m := model.Metrics{Mutation: &model.Mutation{Mutants: []model.Mutant{{Line: 1, Status: "LIVED", Type: "NEW_TYPE", Description: "a → b"}}}}
	s := Annotate(n, m, []notes.Placed{{Note: notes.Note{Author: "agent:claude-code", Text: "equivalent: tiers[0] is re-checked"}, Line: 1}}, "x := a", 1)
	a := s.Lines[0].Annotations
	if len(a) != 2 || a[0].Why != "equivalent: tiers[0] is re-checked" || a[1].Title != "Note from agent:claude-code" {
		t.Fatalf("%+v", a)
	}
	if !strings.HasPrefix(Hint("NEW_TYPE"), "Add an assertion") {
		t.Error("fallback hint")
	}
}
