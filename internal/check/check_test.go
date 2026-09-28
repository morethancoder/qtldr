package check

import (
	"fmt"
	"strings"
	"testing"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/gitx"
	"github.com/morethancoder/qtldr/internal/model"
)

func f(v float64) *float64 { return &v }

func fixture() model.Graph {
	return model.Graph{
		Nodes: []model.Node{
			{ID: "m/p", Kind: model.KindPackage},
			{ID: "m/p.tiered", Kind: model.KindFunc, Parent: "m/p", File: "p/tier.go", Line: 20, EndLine: 48},
			{ID: "m/p.nested", Kind: model.KindFunc, Parent: "m/p", File: "p/a.go", Line: 1, EndLine: 30},
			{ID: "m/p.clean", Kind: model.KindFunc, Parent: "m/p", File: "p/a.go", Line: 32, EndLine: 40},
			{ID: "m/p.fresh", Kind: model.KindFunc, Parent: "m/p", File: "p/new.go", Line: 3, EndLine: 5},
			{ID: "m/q", Kind: model.KindPackage},
			{ID: "m/q.broken", Kind: model.KindFunc, Parent: "m/q", File: "q/b.go", Line: 1, EndLine: 3},
		},
		Metrics: map[model.ID]model.Metrics{
			"m/p.tiered": {Cognitive: model.Ptr(11), CRAP: f(14.08), Coverage: &model.Coverage{Stmts: 17, Covered: 12, Percent: f(70.6),
				Lines: &model.LineStates{Uncovered: []int{25, 37, 41, 42, 43, 44}}},
				Mutation: &model.Mutation{Killed: 9, Survived: 7, Score: f(56.3), Mutants: []model.Mutant{
					{Line: 29, Status: "LIVED", Description: ">= → >"}, {Line: 30, Status: "KILLED"}, {Line: 22, Status: "LIVED", Description: "ErrNoTiers → nil"}}}},
			"m/p.nested": {Cognitive: model.Ptr(22), CRAP: f(3), Coverage: &model.Coverage{Stmts: 4, Covered: 4, Percent: f(100), Stale: true},
				Mutation: &model.Mutation{}},
			"m/p.clean": {Cognitive: model.Ptr(1), CRAP: f(1), Coverage: &model.Coverage{Stmts: 1, Covered: 1, Percent: f(100)},
				Mutation: &model.Mutation{Killed: 3, Score: f(100)}},
			"m/p.fresh":  {Cognitive: model.Ptr(0)},
			"m/q.broken": {Cognitive: model.Ptr(0)},
			"m/q":        {CoverageError: "tests failed; output in .qtldr/logs/x.log"},
		},
	}
}

func kinds(r Report) map[string][]string {
	out := map[string][]string{}
	for _, b := range r.Breaches {
		out[b.Name] = append(out[b.Name], b.Kind)
	}
	return out
}

func TestEvaluateFull(t *testing.T) {
	r := Evaluate(fixture(), All(fixture()), config.Default().Thresholds, false)
	got := kinds(r)
	if strings.Join(got["p.tiered"], ",") != "crap,coverage,mutation" {
		t.Errorf("tiered breaches %v", got["p.tiered"])
	}
	if strings.Join(got["p.nested"], ",") != "cognitive" {
		t.Errorf("nested breaches %v (zero mutation sites must not breach)", got["p.nested"])
	}
	if len(got["p.clean"]) != 0 || r.Pass {
		t.Errorf("clean %v pass %v", got["p.clean"], r.Pass)
	}
	for _, b := range r.Breaches {
		if b.Kind == KindCoverage && b.Hint != "add tests for lines tier.go:25, 37, 41–44" {
			t.Errorf("coverage hint %q", b.Hint)
		}
		if b.Kind == KindMutation && strings.Join(b.Details, "|") != "tier.go:29 >= → >|tier.go:22 ErrNoTiers → nil" {
			t.Errorf("mutation details %q", b.Details)
		}
	}
	reasons := map[string]string{}
	for _, m := range r.NotMeasured {
		reasons[m.Name+"/"+m.Metric] = m.Reason
	}
	if !strings.Contains(reasons["q.broken/crap"], "tests failed") {
		t.Errorf("failed package reason: %v", reasons)
	}
	if !strings.Contains(reasons["p.fresh/mutation"], "qtldr mutate --func p.fresh") {
		t.Errorf("mutation reason: %v", reasons)
	}
}

func TestEvaluateFast(t *testing.T) {
	g := fixture()
	m := g.Metrics["m/p.tiered"]
	m.Coverage.Stale = true
	g.Metrics["m/p.tiered"] = m
	r := Evaluate(g, All(g), config.Default().Thresholds, true)
	got := kinds(r)
	if len(got["p.tiered"]) != 0 {
		t.Errorf("fast mode must skip CRAP on stale coverage, got %v", got["p.tiered"])
	}
	if strings.Join(got["p.nested"], ",") != "cognitive" {
		t.Errorf("nested %v", got["p.nested"])
	}
	for _, m := range r.NotMeasured {
		if m.Metric == KindMutation {
			t.Errorf("fast mode does not report mutation: %+v", m)
		}
	}
}

func TestCriticalPaths(t *testing.T) {
	th := config.Default().Thresholds
	th.CriticalPaths = []string{"p/**"}
	g := fixture()
	g.Metrics["m/p.clean"] = model.Metrics{Cognitive: model.Ptr(1), CRAP: f(1), Coverage: &model.Coverage{Stmts: 10, Covered: 9, Percent: f(90)}, Mutation: &model.Mutation{}}
	r := Evaluate(g, []model.ID{"m/p.clean"}, th, false)
	if len(r.Breaches) != 1 || r.Breaches[0].Limit != 100 {
		t.Fatalf("critical path: %+v", r.Breaches)
	}
}

func TestChangedScope(t *testing.T) {
	ch := gitx.Changes{
		Hunks:     map[string][]gitx.Range{"p/tier.go": {{Start: 29, End: 29}}, "p/a.go": {{Start: 31, End: 31}}},
		Untracked: []string{"p/new.go"},
	}
	got := Changed(fixture(), ch)
	if fmt.Sprint(got) != "[m/p.tiered m/p.fresh]" {
		t.Fatalf("got %v", got)
	}
	if got := Expand(fixture(), []model.ID{"m/q", "m/p.clean", "nope"}); fmt.Sprint(got) != "[m/p.clean m/q.broken]" {
		t.Fatalf("expand %v", got)
	}
}

func TestMarkdown(t *testing.T) {
	r := Evaluate(fixture(), All(fixture()), config.Default().Thresholds, false)
	r.Scope, r.Base = "changed", "main"
	md := Markdown(r)
	for _, want := range []string{
		"## qtldr check · 5 changed functions · base main\n",
		"### Breaches (4)",
		"- [crap] p.tiered — 14.1 (limit 8.0) — add tests for the uncovered branches, or split the function",
		"  - tier.go:29 >= → >   · tier.go:22 ErrNoTiers → nil",
		"| p.nested (stale) | 3.0 | 100% | — | 22 |",
		"### Not measured",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
}

func TestMarkdownNeverExceedsLimit(t *testing.T) {
	var r Report
	for i := range 200 {
		name := fmt.Sprintf("p.f%d", i)
		r.Functions = append(r.Functions, Row{Name: name})
		r.Breaches = append(r.Breaches, Breach{Kind: KindCRAP, Name: name, Value: 9, Limit: 8, Hint: "h", Details: []string{"d"}})
		r.NotMeasured = append(r.NotMeasured, Missing{Name: name, Reason: "r"})
	}
	md := Markdown(r)
	lines := strings.Count(md, "\n")
	if lines > MaxLines {
		t.Fatalf("%d lines", lines)
	}
	if !strings.Contains(md, "… and") || !strings.Contains(md, "### Breaches (200)") {
		t.Errorf("truncation markers missing:\n%s", md)
	}
}

// A large report (a whole real module) must not print a table header with no
// rows, and cut "not measured" lines are summed up per metric.
func TestMarkdownSummarizesCutSections(t *testing.T) {
	var r Report
	for i := range 300 {
		name := fmt.Sprintf("p.f%d", i)
		r.Functions = append(r.Functions, Row{Name: name})
		r.Breaches = append(r.Breaches, Breach{Kind: KindCRAP, Name: name, Value: 9, Limit: 8, Hint: "h"})
		metric := KindMutation
		if i%100 == 0 {
			metric = KindCRAP
		}
		r.NotMeasured = append(r.NotMeasured, Missing{Name: name, Metric: metric, Reason: "r " + name})
	}
	md := Markdown(r)
	if n := strings.Count(md, "\n"); n > MaxLines {
		t.Fatalf("%d lines", n)
	}
	for _, want := range []string{
		"### Functions\n- 300 functions, too many to list here: run qtldr worst, or add --json for all of them\n",
		"### Not measured\n- 3 functions: coverage missing or stale (run: qtldr analyze --coverage)\n- 297 functions: mutation not run, or no mutant could run (run: qtldr mutate)\n",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	if strings.Contains(md, "| function |") {
		t.Errorf("empty table printed:\n%s", md)
	}
}

// A table that only slightly overflows keeps its rows and says how many were
// cut; a short not-measured list is printed as is.
func TestMarkdownCutsTableRows(t *testing.T) {
	var r Report
	for i := range 70 {
		r.Functions = append(r.Functions, Row{Name: fmt.Sprintf("p.f%d", i)})
	}
	r.NotMeasured = []Missing{{Name: "p.f1", Metric: KindMutation, Reason: "mutation not run"}}
	md := Markdown(r)
	for _, want := range []string{"| p.f0 |", "- … and ", "- p.f1 — mutation not run"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	if n := strings.Count(md, "\n"); n > MaxLines {
		t.Fatalf("%d lines", n)
	}
}

func TestNotesAndGenericCoverageHint(t *testing.T) {
	g := fixture()
	m := g.Metrics["m/p.tiered"]
	m.Coverage.Lines = nil
	g.Metrics["m/p.tiered"] = m
	r := Evaluate(g, []model.ID{"m/p.tiered"}, config.Default().Thresholds, false)
	r.Notes = []Note{{Name: "p.tiered", Line: 34, Author: "user", Text: "split by Kind"}, {Name: "p", Author: "agent:claude-code", Text: "pkg note"}}
	md := Markdown(r)
	for _, want := range []string{"add tests for the uncovered lines", "### Notes", "- p.tiered:34 (user): split by Kind", "- p (agent:claude-code): pkg note"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	lines := make([]int, 0, 30)
	for i := 1; i <= 60; i += 2 {
		lines = append(lines, i)
	}
	m.Coverage.Lines = &model.LineStates{Uncovered: lines}
	g.Metrics["m/p.tiered"] = m
	r = Evaluate(g, []model.ID{"m/p.tiered"}, config.Default().Thresholds, true)
	if len(r.Breaches) != 1 || r.Breaches[0].Kind != KindCRAP {
		t.Errorf("fast: %+v", r.Breaches)
	}
	r = Evaluate(g, []model.ID{"m/p.tiered"}, config.Default().Thresholds, false)
	for _, b := range r.Breaches {
		if b.Kind == KindCoverage && strings.Count(b.Hint, ",") != 9 {
			t.Errorf("at most 10 ranges: %q", b.Hint)
		}
	}
}
