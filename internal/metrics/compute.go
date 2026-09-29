package metrics

import (
	"cmp"
	"maps"
	"slices"

	"github.com/morethancoder/qtldr/internal/model"
)

// Inputs are the measured data Compute combines with a scanned graph.
type Inputs struct {
	// Coverage per function; absent means never measured.
	Coverage map[model.ID]model.Coverage
	// CoverageErrors per package, e.g. "tests failed; log: …".
	CoverageErrors map[model.ID]string
	// Mutation per function; absent means never measured. An entry with no
	// sites and no mutants means the function has no mutation sites.
	Mutation map[model.ID]model.Mutation
	// MutationErrors per package.
	MutationErrors map[model.ID]string
	// FileChurn by file and PackageChurn by package dir; nil when not a git repo.
	FileChurn    map[string]int
	PackageChurn map[string]int
	// FunctionChurn per function (approximate); when set it replaces file churn.
	FunctionChurn map[model.ID]int
	// PurityAllow lists function IDs treated as pure.
	PurityAllow []model.ID
}

// Compute returns a copy of g with function metrics (coverage, CRAP,
// mutation, churn, grades), purity, and package and module roll-ups.
func Compute(g model.Graph, in Inputs) model.Graph {
	out := model.Graph{Nodes: slices.Clone(g.Nodes), Edges: g.Edges, Metrics: maps.Clone(g.Metrics)}
	if out.Metrics == nil {
		out.Metrics = map[model.ID]model.Metrics{}
	}
	for _, n := range out.Nodes {
		if n.Kind == model.KindFunc {
			out.Metrics[n.ID] = functionMetrics(n, out.Metrics[n.ID], in)
		}
	}
	applyPurity(&out, Purity(out, in.PurityAllow))
	rollUp(&out, in)
	return out
}

func functionMetrics(n model.Node, m model.Metrics, in Inputs) model.Metrics {
	if c, ok := in.Coverage[n.ID]; ok {
		m.Coverage = &c
		if m.CC != nil {
			m.CRAP = CRAP(*m.CC, &c)
		}
	}
	if mut, ok := in.Mutation[n.ID]; ok {
		m.Mutation = &mut
	}
	m.Churn, m.ChurnScope = churnOf(n, in)
	m.Grades = model.Ptr(FunctionGrades(m))
	return m
}

// churnOf is the function's churn: per function when measured, else its
// file's.
func churnOf(n model.Node, in Inputs) (*int, string) {
	if c, ok := in.FunctionChurn[n.ID]; ok {
		return model.Ptr(c), "function"
	}
	if in.FileChurn != nil {
		return model.Ptr(in.FileChurn[n.File]), "file"
	}
	return nil, ""
}

// agg accumulates a roll-up over functions.
type agg struct {
	grades             *model.Grades
	crapMax, crapSum   float64
	crapN              int
	stmts, covered     int
	covMeasured, stale bool
	mut                model.Mutation
	mutMeasured        bool
	worst              model.ID
	worstCRAP          float64
	worstCombined      int
	hasWorst           bool
}

func (a *agg) add(id model.ID, m model.Metrics) {
	if m.Grades != nil {
		if a.grades == nil {
			a.grades = model.Ptr(*m.Grades)
		} else {
			a.grades = model.Ptr(Worst(*a.grades, *m.Grades))
		}
	}
	a.addCRAP(id, m)
	a.addCoverage(m.Coverage)
	a.addMutation(m.Mutation)
}

func (a *agg) addCRAP(id model.ID, m model.Metrics) {
	combined := 11
	if m.Grades != nil {
		combined = m.Grades.Combined
	}
	crap := -1.0
	if m.CRAP != nil {
		crap = *m.CRAP
		a.crapMax = max(a.crapMax, crap)
		a.crapSum += crap
		a.crapN++
	}
	if !a.hasWorst || crap > a.worstCRAP || (crap == a.worstCRAP && combined < a.worstCombined) {
		a.worst, a.worstCRAP, a.worstCombined, a.hasWorst = id, crap, combined, true
	}
}

func (a *agg) addCoverage(c *model.Coverage) {
	if c == nil {
		return
	}
	a.covMeasured = true
	a.stmts += c.Stmts
	a.covered += c.Covered
	a.stale = a.stale || c.Stale
}

func (a *agg) addMutation(mu *model.Mutation) {
	if mu == nil {
		return
	}
	a.mutMeasured = true
	a.mut.Killed += mu.Killed
	a.mut.Survived += mu.Survived
	a.mut.NotCovered += mu.NotCovered
	a.mut.TimedOut += mu.TimedOut
	a.mut.Stale = a.mut.Stale || mu.Stale
}

// metrics turns the accumulator into roll-up metrics.
func (a *agg) metrics() model.Metrics {
	m := model.Metrics{Grades: a.grades, Worst: a.worst}
	if a.crapN > 0 {
		m.CrapMax = model.Ptr(Round1(a.crapMax))
		m.CrapAvg = model.Ptr(Round1(a.crapSum / float64(a.crapN)))
	}
	if a.covMeasured {
		m.Coverage = &model.Coverage{Stmts: a.stmts, Covered: a.covered, Percent: percent(a.covered, a.stmts), Stale: a.stale}
	}
	if a.mutMeasured {
		mu := a.mut
		mu.Score = Score(mu.Killed, mu.TimedOut, mu.Survived)
		m.Mutation = &mu
	}
	return m
}

// Score = caught / (caught + survived) × 100, nil when the denominator is 0.
// A mutant that made the tests time out was caught, like a killed one.
func Score(killed, timedOut, survived int) *float64 {
	return percent(killed+timedOut, killed+timedOut+survived)
}

func percent(part, total int) *float64 {
	if total == 0 {
		return nil
	}
	return model.Ptr(Round1(float64(part) / float64(total) * 100))
}

// rollUp fills package metrics from their functions and module metrics from
// every function (the worst package is the worst function).
func rollUp(g *model.Graph, in Inputs) {
	pkgs, module := aggregate(*g)
	for _, n := range g.Nodes {
		switch {
		case pkgs[n.ID] != nil:
			g.Metrics[n.ID] = packageMetrics(n, pkgs[n.ID], in)
		case n.Kind == model.KindModule:
			g.Metrics[n.ID] = module.metrics()
		}
	}
}

// aggregate accumulates function metrics per package and for the module.
func aggregate(g model.Graph) (map[model.ID]*agg, *agg) {
	pkgs := map[model.ID]*agg{}
	for _, n := range g.Nodes {
		if n.Kind == model.KindPackage {
			pkgs[n.ID] = &agg{}
		}
	}
	module := &agg{}
	for _, n := range g.Nodes {
		if a, ok := pkgs[n.Parent]; ok && n.Kind == model.KindFunc {
			a.add(n.ID, g.Metrics[n.ID])
			module.add(n.ID, g.Metrics[n.ID])
		}
	}
	return pkgs, module
}

func packageMetrics(n model.Node, a *agg, in Inputs) model.Metrics {
	m := a.metrics()
	if in.PackageChurn != nil {
		m.Churn = model.Ptr(in.PackageChurn[n.Dir])
	}
	m.CoverageError = in.CoverageErrors[n.ID]
	m.MutationError = in.MutationErrors[n.ID]
	return m
}

// SortByRisk orders IDs by CRAP (highest first, missing last), then ID.
func SortByRisk(g model.Graph, ids []model.ID) {
	slices.SortFunc(ids, func(a, b model.ID) int {
		ca, cb := crapOr(g, a), crapOr(g, b)
		return cmp.Or(cmp.Compare(cb, ca), cmp.Compare(a, b))
	})
}

func crapOr(g model.Graph, id model.ID) float64 {
	if c := g.Metrics[id].CRAP; c != nil {
		return *c
	}
	return -1
}
