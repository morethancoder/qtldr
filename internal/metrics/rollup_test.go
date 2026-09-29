package metrics

import (
	"fmt"
	"testing"

	"github.com/morethancoder/qtldr/internal/model"
)

// A package's worst function: highest CRAP; on a CRAP tie the lower
// combined grade; on a full tie the first one added.
func TestRollUpWorst(t *testing.T) {
	graded := func(crap *float64, combined int) model.Metrics {
		return model.Metrics{CRAP: crap, Grades: &model.Grades{Combined: combined}}
	}
	type fn struct {
		id model.ID
		m  model.Metrics
	}
	cases := []struct {
		name string
		fns  []fn
		want model.ID
	}{
		{"highest CRAP", []fn{{"a", graded(f(3), 8)}, {"b", graded(f(9), 8)}, {"c", graded(f(5), 1)}}, "b"},
		{"full tie keeps the first", []fn{{"a", graded(f(4), 6)}, {"b", graded(f(4), 6)}}, "a"},
		{"CRAP tie: lower combined grade", []fn{{"a", graded(f(4), 8)}, {"b", graded(f(4), 5)}}, "b"},
		{"CRAP tie: higher combined grade loses", []fn{{"a", graded(f(4), 5)}, {"b", graded(f(4), 8)}}, "a"},
		{"CRAP tie: ungraded counts as best", []fn{{"a", model.Metrics{CRAP: f(4)}}, {"b", graded(f(4), 10)}}, "b"},
		{"missing CRAP ranks below any measured", []fn{{"a", graded(nil, 1)}, {"b", graded(f(1), 10)}}, "b"},
	}
	for _, c := range cases {
		var a agg
		for _, x := range c.fns {
			a.addCRAP(x.id, x.m)
		}
		if a.worst != c.want {
			t.Errorf("%s: worst = %s, want %s", c.name, a.worst, c.want)
		}
	}
}

func TestRollUpCRAP(t *testing.T) {
	var none agg
	none.addCRAP("a", model.Metrics{})
	if m := none.metrics(); m.CrapMax != nil || m.CrapAvg != nil {
		t.Errorf("no CRAP measured: max %v avg %v", deref(m.CrapMax), deref(m.CrapAvg))
	}
	var two agg
	two.addCRAP("a", model.Metrics{CRAP: f(2)})
	two.addCRAP("b", model.Metrics{CRAP: f(4)})
	two.addCRAP("c", model.Metrics{})
	if m := two.metrics(); deref(m.CrapMax) != 4.0 || deref(m.CrapAvg) != 3.0 {
		t.Errorf("max %v avg %v, want 4 and 3", deref(m.CrapMax), deref(m.CrapAvg))
	}
}

// A function counts as reached when any line ran fully or partly.
func TestReached(t *testing.T) {
	cases := []struct {
		l    model.LineStates
		want bool
	}{
		{model.LineStates{}, false},
		{model.LineStates{Uncovered: []int{3}}, false},
		{model.LineStates{Partial: []int{3}}, true},
		{model.LineStates{Covered: []int{2}, Partial: []int{3}}, true},
		{model.LineStates{Covered: []int{2}}, true},
	}
	for _, c := range cases {
		if got := reached(&c.l); got != c.want {
			t.Errorf("%+v: %v", c.l, got)
		}
	}
}

func TestSortByRisk(t *testing.T) {
	g := model.Graph{Metrics: map[model.ID]model.Metrics{
		"a": {CRAP: f(5)}, "b": {CRAP: f(9)}, "d": {CRAP: f(5)}, "e": {CRAP: f(0.5)},
	}}
	ids := []model.ID{"c", "e", "d", "a", "b"}
	SortByRisk(g, ids)
	// c has no CRAP: last, below even 0.5
	if fmt.Sprint(ids) != "[b a d e c]" {
		t.Errorf("got %v", ids)
	}
}
