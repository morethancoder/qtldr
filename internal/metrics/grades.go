package metrics

import (
	"math"
	"slices"

	"github.com/morethancoder/qtldr/internal/model"
)

// CRAP = CC² × (1 − cov)³ + CC (PLAN.md §7.2). Coverage never run → nil.
// A function with no statements counts as fully covered.
func CRAP(cc int, cov *model.Coverage) *float64 {
	if cov == nil {
		return nil
	}
	c := 1.0 // no statements
	if cov.Stmts > 0 {
		c = float64(cov.Covered) / float64(cov.Stmts)
	}
	v := float64(cc*cc)*math.Pow(1-c, 3) + float64(cc)
	return &v
}

// step is one row of a grade table: values on the good side of limit get grade.
type step struct {
	limit float64
	grade int
}

var (
	crapSteps     = []step{{5, 10}, {8, 8}, {12, 6}, {20, 4}, {30, 2}}
	mutationSteps = []step{{90, 10}, {80, 8}, {70, 6}, {55, 4}, {40, 2}}
	coverageSteps = []step{{90, 10}, {80, 8}, {70, 6}, {50, 4}, {30, 2}}
)

// GradeCRAP: nil→1; ≤5→10; ≤8→8; ≤12→6; ≤20→4; ≤30→2; else 1.
func GradeCRAP(v *float64) int { return grade(v, crapSteps, func(v, l float64) bool { return v <= l }) }

// GradeMutation: nil→1; ≥90→10; ≥80→8; ≥70→6; ≥55→4; ≥40→2; else 1.
func GradeMutation(v *float64) int {
	return grade(v, mutationSteps, func(v, l float64) bool { return v >= l })
}

// GradeCoverage: nil→1; ≥90→10; ≥80→8; ≥70→6; ≥50→4; ≥30→2; else 1.
func GradeCoverage(v *float64) int {
	return grade(v, coverageSteps, func(v, l float64) bool { return v >= l })
}

func grade(v *float64, steps []step, ok func(v, limit float64) bool) int {
	if v == nil {
		return 1
	}
	for _, s := range steps {
		if ok(*v, s.limit) {
			return s.grade
		}
	}
	return 1
}

// FunctionGrades grades one function (PLAN.md §7.4). Missing data counts as
// worst; a function with zero mutation sites is left out of mutation grading.
func FunctionGrades(m model.Metrics) model.Grades {
	g := model.Grades{CRAP: GradeCRAP(m.CRAP), Combined: GradeCRAP(m.CRAP)}
	if m.Coverage != nil {
		g.Coverage = GradeCoverage(m.Coverage.Percent)
		if m.Coverage.Percent == nil {
			g.Coverage = 10 // no statements: nothing left untested
		}
	} else {
		g.Coverage = 1
	}
	if !MutationGraded(m) {
		return g
	}
	var score *float64
	if m.Mutation != nil {
		score = m.Mutation.Score
	}
	mut := GradeMutation(score)
	g.Mutation = &mut
	g.Combined = (g.CRAP + mut + 1) / 2 // round half up
	return g
}

// notCovered is mutate.NotCovered (mutate imports metrics, not the reverse).
const notCovered = "NOT COVERED"

// MutationGraded reports whether m's mutation result counts for grading. A
// function with no mutation sites, or whose only mutants are ones Gremlins
// cannot test (see TestableNotCovered), is left out: combined = CRAP grade.
// Missing mutation data is graded (as worst).
func MutationGraded(m model.Metrics) bool {
	mu := m.Mutation
	return mu == nil || mu.Score != nil || TestableNotCovered(m) > 0
}

// TestableNotCovered counts the NOT COVERED mutants that a test could reach.
// A mutant on a line that has no coverage block (a `case <expr>:` line: Go
// starts the block after the colon) in a function that ran is never tested
// by Gremlins, so it is not counted. Without line data every one counts.
func TestableNotCovered(m model.Metrics) int {
	mu := m.Mutation
	if m.Coverage == nil || m.Coverage.Lines == nil || !reached(m.Coverage.Lines) {
		return mu.NotCovered
	}
	n := 0
	for _, mt := range mu.Mutants {
		if mt.Status == notCovered && hasState(m.Coverage.Lines, mt.Line) {
			n++
		}
	}
	return n
}

// reached reports whether any line of the function ran.
func reached(l *model.LineStates) bool { return len(l.Covered)+len(l.Partial) > 0 }

// hasState reports whether a coverage block covers line.
func hasState(l *model.LineStates, line int) bool {
	return slices.Contains(l.Covered, line) || slices.Contains(l.Uncovered, line) || slices.Contains(l.Partial, line)
}

// Worst returns the lowest grade of a and b per metric; a nil mutation grade
// is "does not apply" and loses to any real grade.
func Worst(a, b model.Grades) model.Grades {
	out := model.Grades{CRAP: min(a.CRAP, b.CRAP), Coverage: min(a.Coverage, b.Coverage), Combined: min(a.Combined, b.Combined)}
	switch {
	case a.Mutation == nil:
		out.Mutation = b.Mutation
	case b.Mutation == nil:
		out.Mutation = a.Mutation
	default:
		out.Mutation = model.Ptr(min(*a.Mutation, *b.Mutation))
	}
	return out
}

// Round1 rounds to one decimal place.
func Round1(v float64) float64 { return math.Round(v*10) / 10 }
