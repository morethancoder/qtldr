package metrics

import (
	"math"
	"testing"

	"github.com/morethancoder/qtldr/internal/model"
)

func f(v float64) *float64 { return &v }

func TestCRAP(t *testing.T) {
	cases := []struct {
		name string
		cc   int
		cov  *model.Coverage
		want *float64
	}{
		{"never measured", 5, nil, nil},
		{"untested", 5, &model.Coverage{Stmts: 6, Percent: f(0)}, f(30)},
		{"fully covered", 11, &model.Coverage{Stmts: 17, Covered: 17, Percent: f(100)}, f(11)},
		{"applyTiered 12/17", 11, &model.Coverage{Stmts: 17, Covered: 12, Percent: f(70.588)}, f(121*math.Pow(5.0/17, 3) + 11)},
		{"no statements counts as covered", 1, &model.Coverage{Stmts: 0}, f(1)},
	}
	for _, c := range cases {
		got := CRAP(c.cc, c.cov)
		switch {
		case c.want == nil && got != nil:
			t.Errorf("%s: got %v, want nil", c.name, *got)
		case c.want != nil && (got == nil || math.Abs(*got-*c.want) > 1e-9):
			t.Errorf("%s: got %v, want %v", c.name, got, *c.want)
		}
	}
}

func TestGradeBoundaries(t *testing.T) {
	crap := []struct {
		v    *float64
		want int
	}{{nil, 1}, {f(1), 10}, {f(5), 10}, {f(5.01), 8}, {f(8), 8}, {f(8.01), 6}, {f(12), 6}, {f(12.01), 4},
		{f(20), 4}, {f(20.01), 2}, {f(30), 2}, {f(30.01), 1}}
	for _, c := range crap {
		if got := GradeCRAP(c.v); got != c.want {
			t.Errorf("GradeCRAP(%v) = %d, want %d", deref(c.v), got, c.want)
		}
	}
	mut := []struct {
		v    *float64
		want int
	}{{nil, 1}, {f(100), 10}, {f(90), 10}, {f(89.9), 8}, {f(80), 8}, {f(79.9), 6}, {f(70), 6}, {f(69.9), 4},
		{f(55), 4}, {f(54.9), 2}, {f(40), 2}, {f(39.9), 1}, {f(0), 1}}
	for _, c := range mut {
		if got := GradeMutation(c.v); got != c.want {
			t.Errorf("GradeMutation(%v) = %d, want %d", deref(c.v), got, c.want)
		}
	}
	cov := []struct {
		v    *float64
		want int
	}{{nil, 1}, {f(100), 10}, {f(90), 10}, {f(89.9), 8}, {f(80), 8}, {f(79.9), 6}, {f(70), 6}, {f(69.9), 4},
		{f(50), 4}, {f(49.9), 2}, {f(30), 2}, {f(29.9), 1}}
	for _, c := range cov {
		if got := GradeCoverage(c.v); got != c.want {
			t.Errorf("GradeCoverage(%v) = %d, want %d", deref(c.v), got, c.want)
		}
	}
}

func deref(v *float64) any {
	if v == nil {
		return "nil"
	}
	return *v
}

func TestFunctionGrades(t *testing.T) {
	sites := &model.Mutation{Killed: 9, Survived: 7, NotCovered: 3, Score: f(56.25)}
	cases := []struct {
		name         string
		m            model.Metrics
		wantCombined int
		wantMut      *int
	}{
		// round((4+4)/2) = 4
		{"applyTiered", model.Metrics{CRAP: f(14.08), Mutation: sites, Coverage: &model.Coverage{Percent: f(70.6)}}, 4, model.Ptr(4)},
		// half rounds up: round((10+1)/2) = round(5.5) = 6
		{"mutation missing counts as worst", model.Metrics{CRAP: f(3)}, 6, model.Ptr(1)},
		// sites but killed+lived = 0 grades 1
		{"only not-covered mutants", model.Metrics{CRAP: f(3), Mutation: &model.Mutation{NotCovered: 6}}, 6, model.Ptr(1)},
		// zero sites: excluded, combined = CRAP grade
		{"zero mutation sites", model.Metrics{CRAP: f(3), Mutation: &model.Mutation{}}, 10, nil},
		{"nothing measured", model.Metrics{}, 1, model.Ptr(1)},
	}
	for _, c := range cases {
		g := FunctionGrades(c.m)
		if g.Combined != c.wantCombined {
			t.Errorf("%s: combined = %d, want %d", c.name, g.Combined, c.wantCombined)
		}
		if (g.Mutation == nil) != (c.wantMut == nil) || (g.Mutation != nil && *g.Mutation != *c.wantMut) {
			t.Errorf("%s: mutation grade = %v, want %v", c.name, g.Mutation, c.wantMut)
		}
	}
}
