package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/morethancoder/qtldr/internal/model"
)

func pf(v float64) *float64 { return &v }

func TestPrintDetail(t *testing.T) {
	cases := []struct {
		name string
		d    model.Detail
		want []string
	}{
		{"pure function with coverage and survivors", model.Detail{
			Node: model.Node{ID: "m/p.f", Kind: model.KindFunc, Name: "f", File: "p/f.go", Line: 3, EndLine: 9,
				Exported: model.Ptr(false), Pure: model.Ptr(true), Signature: "func f()"},
			Metrics: &model.Metrics{CC: model.Ptr(3), CRAP: pf(4.5), Churn: model.Ptr(2),
				Coverage: &model.Coverage{Stmts: 4, Covered: 3, Percent: pf(75), Stale: true, Lines: &model.LineStates{Uncovered: []int{5, 6}, Partial: []int{4}}},
				Mutation: &model.Mutation{Killed: 1, Survived: 1, Score: pf(50), Mutants: []model.Mutant{{Line: 4, Status: "LIVED", Description: "> → >="}}},
				Grades:   &model.Grades{CRAP: 10, Mutation: model.Ptr(2), Coverage: 6, Combined: 6}},
			Callers: []model.ID{"m/p.g"}},
			[]string{"f  (function · unexported · λ pure)", "p/f.go:3–9", "CRAP             4.5  (grade 10)", "75% (3 of 4 statements)  (grade 6)  stale",
				"50% (1 killed, 1 survived, 0 not covered)  (grade 2)", "Not covered: f.go:5–6", "Partly covered: f.go:4",
				"Surviving mutants (1)", "f.go:4  > → >=", "Callers (1): p.g", "6 of 10 (CRAP 10, mutation 2, coverage 6)"}},
		{"effectful exported function, nothing measured", model.Detail{
			Node:    model.Node{ID: "m/p.F", Kind: model.KindFunc, Name: "F", File: "p/f.go", Line: 3, EndLine: 3, Exported: model.Ptr(true), Pure: model.Ptr(false), Effects: []string{"calls os.Getenv"}},
			Metrics: &model.Metrics{Mutation: &model.Mutation{NotCovered: 2}}},
			[]string{"(function · exported · effectful)", "p/f.go:3\n", "effectful because: calls os.Getenv", "Coverage         not measured", "— (2 not covered)"}},
		{"no statements, no mutation sites", model.Detail{
			Node:    model.Node{ID: "m/p.e", Kind: model.KindFunc, Name: "e"},
			Metrics: &model.Metrics{Coverage: &model.Coverage{}, Mutation: &model.Mutation{}}},
			[]string{"no statements", "no mutation sites", "Grade            —"}},
		{"package", model.Detail{
			Node: model.Node{ID: "m/p", Kind: model.KindPackage, Name: "p", Pure: model.Ptr(true), Errors: []string{"p/x.go:1: syntax"}},
			Metrics: &model.Metrics{CrapMax: pf(30), CrapAvg: pf(9.9), Worst: "m/p.f", CoverageError: "tests failed",
				Coverage: &model.Coverage{Stmts: 10, Covered: 7, Percent: pf(70)}, Mutation: &model.Mutation{Killed: 3, Survived: 1, Score: pf(75)}},
			Children: []model.ID{"m/p.f"}},
			[]string{"p  (package · λ pure core)", "error: p/x.go:1: syntax", "CRAP max         30.0", "Riskiest         p.f", "Coverage error   tests failed", "Contains (1): p.f"}},
		{"effectful package", model.Detail{Node: model.Node{ID: "m/s", Kind: model.KindPackage, Name: "s", Pure: model.Ptr(false)}},
			[]string{"(package · effectful shell)"}},
		{"struct", model.Detail{Node: model.Node{ID: "m/p.T", Kind: model.KindType, TypeKind: "struct", Name: "T", Fields: []model.Field{{Name: "A", Type: "int"}}}},
			[]string{"(type · struct)", "Fields\n  A                int"}},
		{"external", model.Detail{Node: model.Node{ID: "github.com/x/y", Kind: model.KindExternal, Name: "x/y"}, ImportedBy: []model.ID{"m/p"}},
			[]string{"(external module)", "Imported by (1): p"}},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		printDetail(&buf, c.d)
		for _, w := range c.want {
			if !strings.Contains(buf.String(), w) {
				t.Errorf("%s: missing %q in:\n%s", c.name, w, buf.String())
			}
		}
	}
}

func TestPrintSummaryWarnings(t *testing.T) {
	var out, errb bytes.Buffer
	e := &env{stdout: &out, stderr: &errb}
	err := printSummary(e, "/r", analyzeSummary{Module: "m", Snapshot: "/r/.qtldr/snapshot.json", Errors: []string{"p: broken"},
		Warnings: []string{"not a git repository"}, Failed: []model.ID{"m/p"}, Log: ".qtldr/logs/x.log"})
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"does not type-check", "not a git repository", "tests failed in 1 packages", ".qtldr/logs/x.log"} {
		if !strings.Contains(errb.String(), w) {
			t.Errorf("missing %q in %s", w, errb.String())
		}
	}
	if !strings.Contains(out.String(), "Wrote .qtldr/snapshot.json") {
		t.Errorf("stdout %s", out.String())
	}
}

func TestFormatHelpers(t *testing.T) {
	if formatMetric("coverage", 70.5) != "70.5%" || formatMetric("crap", 14.08) != "14.1" || formatMetric("cc", 3) != "3" {
		t.Error("formatMetric")
	}
	if measureHint("mutation") != "Run: qtldr mutate" || measureHint("cc") != "" {
		t.Error("measureHint")
	}
	if exitCode(2).Error() != "exit 2" {
		t.Error("exitCode")
	}
	p := hookPayload{}
	p.ToolInput.FilePath = "a.go"
	p.ToolInput.Edits = append(p.ToolInput.Edits, struct {
		FilePath string `json:"file_path"`
	}{"b.go"})
	if got := p.Files(); strings.Join(got, ",") != "a.go,b.go" {
		t.Errorf("Files %v", got)
	}
}
