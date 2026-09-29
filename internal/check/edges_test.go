package check

import (
	"fmt"
	"slices"
	"testing"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/model"
)

// one is a graph with a single function m/p.f whose metrics are m.
func one(m model.Metrics) model.Graph {
	return model.Graph{
		Nodes:   []model.Node{{ID: "m/p", Kind: model.KindPackage}, {ID: "m/p.f", Kind: model.KindFunc, Parent: "m/p", File: "p/f.go", Line: 1, EndLine: 9}},
		Metrics: map[model.ID]model.Metrics{"m/p.f": m},
	}
}

func TestThresholdBoundaries(t *testing.T) {
	th := config.Default().Thresholds // cognitive 15, coverage 80
	cases := []struct {
		name string
		m    model.Metrics
		want []string
	}{
		{"cognitive at the limit passes", model.Metrics{Cognitive: model.Ptr(15)}, nil},
		{"cognitive over the limit", model.Metrics{Cognitive: model.Ptr(16)}, []string{KindCognitive}},
		{"coverage at the limit passes", model.Metrics{Coverage: &model.Coverage{Percent: f(80)}}, nil},
		{"coverage under the limit", model.Metrics{Coverage: &model.Coverage{Percent: f(79.9)}}, []string{KindCoverage}},
	}
	for _, c := range cases {
		r := Evaluate(one(c.m), []model.ID{"m/p.f"}, th, false)
		var got []string
		for _, b := range r.Breaches {
			if b.Kind == KindCognitive || b.Kind == KindCoverage {
				got = append(got, b.Kind)
			}
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: breaches %v, want %v", c.name, got, c.want)
		}
	}
}

func TestCoverageHint(t *testing.T) {
	th := config.Default().Thresholds
	hint := func(lines *model.LineStates) string {
		r := Evaluate(one(model.Metrics{Coverage: &model.Coverage{Percent: f(50), Lines: lines}}), []model.ID{"m/p.f"}, th, false)
		for _, b := range r.Breaches {
			if b.Kind == KindCoverage {
				return b.Hint
			}
		}
		return ""
	}
	if got := hint(&model.LineStates{Partial: []int{3}}); got != "add tests for the uncovered lines" {
		t.Errorf("no uncovered lines: %q", got)
	}
	var ten []int
	for i := range 10 {
		ten = append(ten, 2*i+1)
	}
	if got := hint(&model.LineStates{Uncovered: ten}); got != "add tests for lines f.go:1, 3, 5, 7, 9, 11, 13, 15, 17, 19" {
		t.Errorf("ten ranges: %q", got)
	}
	if got := hint(&model.LineStates{Uncovered: append(ten, 30)}); got != "add tests for lines f.go:1, 3, 5, 7, 9, 11, 13, 15, 17, 19" {
		t.Errorf("at most ten ranges: %q", got)
	}
}

func TestInFiles(t *testing.T) {
	g := fixture()
	g.Nodes = append(g.Nodes, model.Node{ID: "m/p.Tier", Kind: model.KindType, Parent: "m/p", File: "p/tier.go", Line: 5, EndLine: 9})
	if got := InFiles(g, []string{"p/tier.go", "q/b.go"}); fmt.Sprint(got) != "[m/p.tiered m/q.broken]" {
		t.Errorf("got %v", got)
	}
	if got := InFiles(g, nil); len(got) != 0 {
		t.Errorf("no files: %v", got)
	}
}

func TestBreachLines(t *testing.T) {
	r := Report{Breaches: []Breach{
		{Kind: KindCRAP, Name: "p.f", Value: 9, Limit: 8, Hint: "h"},
		{Kind: KindMutation, Name: "p.g", Value: 50, Limit: 70, Hint: "m", Details: []string{"a", "b"}},
	}}
	want := []string{
		"### Breaches (2)",
		"- [crap] p.f — 9.0 (limit 8.0) — h",
		"- [mutation] p.g — 50% (limit 70%) — m",
		"  - a   · b",
	}
	if got := breachLines(r); !slices.Equal(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestSectionShrink(t *testing.T) {
	cases := []struct {
		name string
		s    section
		over int
		want []string
	}{
		{"empty body is left alone", section{keep: 1, lines: []string{"h"}}, 3, []string{"h"}},
		{"one row is not swapped for a line saying so", section{keep: 1, lines: []string{"h", "a"}}, 1, []string{"h", "a"}},
		{"whole body cut", section{keep: 1, lines: []string{"h", "a", "b"}}, 10, []string{"h", "- … and 2 more"}},
		{"part cut, header kept", section{keep: 2, lines: []string{"h", "t", "a", "b", "c"}}, 1, []string{"h", "t", "a", "- … and 2 more"}},
		{"summary replaces a cut body", section{keep: 1, lines: []string{"h", "a", "b", "c"}, summary: []string{"s"}}, 1, []string{"h", "s"}},
		{"summary no shorter than the body is not used", section{keep: 1, lines: []string{"h", "a"}, summary: []string{"s"}}, 1, []string{"h", "a"}},
		{"partial section keeps rows while some stay", section{keep: 1, lines: []string{"h", "a", "b", "c"}, summary: []string{"s"}, partial: true}, 1, []string{"h", "a", "- … and 2 more"}},
		{"partial section summarized when every row goes", section{keep: 1, lines: []string{"h", "a", "b", "c"}, summary: []string{"s"}, partial: true}, 5, []string{"h", "s"}},
	}
	for _, c := range cases {
		s := c.s
		s.lines = slices.Clone(c.s.lines)
		s.shrink(c.over)
		if !slices.Equal(s.lines, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, s.lines, c.want)
		}
	}
}

func TestSectionSummarize(t *testing.T) {
	s := section{keep: 1, lines: []string{"h", "a"}, summary: []string{"s"}}
	if s.summarize(true) || !slices.Equal(s.lines, []string{"h", "a"}) {
		t.Errorf("summary as long as the body: %q", s.lines)
	}
	s = section{keep: 1, lines: []string{"h", "a", "b"}, summary: []string{"s"}}
	if !s.summarize(true) || !slices.Equal(s.lines, []string{"h", "s"}) {
		t.Errorf("shorter summary: %q", s.lines)
	}
	s = section{keep: 1, lines: []string{"h", "a", "b"}}
	if s.summarize(true) {
		t.Error("no summary")
	}
}
