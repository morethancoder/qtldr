// Package check turns thresholds and a snapshot into the pass/fail report of
// `qtldr check` (PLAN.md §8). It is pure: scope, evaluation and rendering take
// values and return values.
package check

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/coverage"
	"github.com/morethancoder/qtldr/internal/gitx"
	"github.com/morethancoder/qtldr/internal/lang/golang"
	"github.com/morethancoder/qtldr/internal/metrics"
	"github.com/morethancoder/qtldr/internal/model"
)

// Breach kinds.
const (
	KindCRAP      = "crap"
	KindCognitive = "cognitive"
	KindCoverage  = "coverage"
	KindMutation  = "mutation"
)

// Breach is one function over one limit.
type Breach struct {
	Kind    string   `json:"kind"`
	ID      model.ID `json:"id"`
	Name    string   `json:"name"`
	Value   float64  `json:"value"`
	Limit   float64  `json:"limit"`
	Hint    string   `json:"hint"`
	Details []string `json:"details,omitempty"`
}

// Row is one line of the functions table.
type Row struct {
	ID        model.ID `json:"id"`
	Name      string   `json:"name"`
	CRAP      *float64 `json:"crap"`
	Coverage  *float64 `json:"coverage"`
	Mutation  *float64 `json:"mutation"`
	Cognitive *int     `json:"cognitive"`
	Stale     bool     `json:"stale"`
}

// Missing is a value that could not be checked.
type Missing struct {
	ID     model.ID `json:"id"`
	Name   string   `json:"name"`
	Metric string   `json:"metric"`
	Reason string   `json:"reason"`
}

// Note is an open note on an in-scope function.
type Note struct {
	ID     model.ID `json:"id"`
	Name   string   `json:"name"`
	Line   int      `json:"line,omitempty"`
	Author string   `json:"author"`
	Text   string   `json:"text"`
}

// Report is the result of a check.
type Report struct {
	Scope       string    `json:"scope"`
	Base        string    `json:"base,omitempty"`
	Fast        bool      `json:"fast"`
	Pass        bool      `json:"pass"`
	Breaches    []Breach  `json:"breaches"`
	Functions   []Row     `json:"functions"`
	NotMeasured []Missing `json:"not_measured"`
	Notes       []Note    `json:"notes,omitempty"`
}

// Changed returns the functions whose line range a hunk touches, plus every
// function in an untracked file.
func Changed(g model.Graph, ch gitx.Changes) []model.ID {
	var ids []model.ID
	for _, n := range g.Nodes {
		if n.Kind == model.KindFunc && touched(n, ch) {
			ids = append(ids, n.ID)
		}
	}
	return ids
}

func touched(n model.Node, ch gitx.Changes) bool {
	if slices.Contains(ch.Untracked, n.File) {
		return true
	}
	for _, r := range ch.Hunks[n.File] {
		if r.Touches(n.Line, n.EndLine) {
			return true
		}
	}
	return false
}

// InFiles returns the functions declared in the given module-relative files.
func InFiles(g model.Graph, files []string) []model.ID {
	var ids []model.ID
	for _, n := range g.Nodes {
		if n.Kind == model.KindFunc && slices.Contains(files, n.File) {
			ids = append(ids, n.ID)
		}
	}
	return ids
}

// All returns every function.
func All(g model.Graph) []model.ID {
	var ids []model.ID
	for _, n := range g.Nodes {
		if n.Kind == model.KindFunc {
			ids = append(ids, n.ID)
		}
	}
	return ids
}

// Expand turns package IDs into their functions and keeps function IDs.
func Expand(g model.Graph, ids []model.ID) []model.ID {
	var out []model.ID
	for _, id := range ids {
		n, ok := g.Node(id)
		switch {
		case !ok:
		case n.Kind == model.KindFunc:
			out = append(out, id)
		case n.Kind == model.KindPackage:
			out = append(out, childFuncs(g, id)...)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func childFuncs(g model.Graph, pkg model.ID) []model.ID {
	var ids []model.ID
	for _, n := range g.Nodes {
		if n.Kind == model.KindFunc && n.Parent == pkg {
			ids = append(ids, n.ID)
		}
	}
	return ids
}

// Evaluate checks the functions in ids. In fast mode only cognitive
// complexity, and CRAP for functions whose coverage is current, are checked.
// Missing data never breaches; it is listed in NotMeasured.
func Evaluate(g model.Graph, ids []model.ID, th config.Thresholds, fast bool) Report {
	r := Report{Fast: fast, Breaches: []Breach{}, Functions: []Row{}, NotMeasured: []Missing{}}
	for _, id := range ids {
		n, ok := g.Node(id)
		if !ok || n.Kind != model.KindFunc {
			continue
		}
		e := evaluator{g: g, n: n, m: g.Metrics[id], th: th, fast: fast, name: Short(id)}
		e.run(&r)
	}
	r.Pass = len(r.Breaches) == 0
	return r
}

type evaluator struct {
	g    model.Graph
	n    model.Node
	m    model.Metrics
	th   config.Thresholds
	fast bool
	name string
}

func (e evaluator) run(r *Report) {
	r.Functions = append(r.Functions, e.row())
	e.cognitive(r)
	e.crap(r)
	if !e.fast {
		e.coverage(r)
		e.mutation(r)
	}
}

func (e evaluator) row() Row {
	row := Row{ID: e.n.ID, Name: e.name, CRAP: e.m.CRAP, Cognitive: e.m.Cognitive}
	if c := e.m.Coverage; c != nil {
		row.Coverage, row.Stale = c.Percent, c.Stale
	}
	if mu := e.m.Mutation; mu != nil {
		row.Mutation, row.Stale = mu.Score, row.Stale || mu.Stale
	}
	return row
}

func (e evaluator) breach(r *Report, kind string, value, limit float64, hint string, details ...string) {
	r.Breaches = append(r.Breaches, Breach{Kind: kind, ID: e.n.ID, Name: e.name, Value: value, Limit: limit, Hint: hint, Details: details})
}

func (e evaluator) missing(r *Report, metric, reason string) {
	r.NotMeasured = append(r.NotMeasured, Missing{ID: e.n.ID, Name: e.name, Metric: metric, Reason: reason})
}

func (e evaluator) cognitive(r *Report) {
	if c := e.m.Cognitive; c != nil && *c > e.th.CognitiveMax {
		e.breach(r, KindCognitive, float64(*c), float64(e.th.CognitiveMax), "extract the nested block or invert conditions to flatten")
	}
}

func (e evaluator) crap(r *Report) {
	cov := e.m.Coverage
	switch {
	case e.m.CRAP == nil:
		e.missing(r, KindCRAP, e.coverageMissingReason())
	case e.fast && cov.Stale:
		e.missing(r, KindCRAP, "coverage is stale since the last change (run: qtldr check "+e.name+")")
	case *e.m.CRAP > e.th.CrapMax:
		e.breach(r, KindCRAP, *e.m.CRAP, e.th.CrapMax, "add tests for the uncovered branches, or split the function")
	}
}

func (e evaluator) coverageMissingReason() string {
	if msg := e.g.Metrics[e.n.Parent].CoverageError; msg != "" {
		return "coverage not measured: " + msg
	}
	return "coverage not measured yet (run: qtldr check " + e.name + ")"
}

func (e evaluator) coverage(r *Report) {
	c := e.m.Coverage
	if c == nil || c.Percent == nil {
		return // reported under crap, or no statements
	}
	limit := e.th.CoverageMin
	if e.critical() {
		limit = 100
	}
	if *c.Percent >= limit {
		return
	}
	hint := "add tests for the uncovered lines"
	var details []string
	if c.Lines != nil && len(c.Lines.Uncovered) > 0 {
		ranges := coverage.Ranges(c.Lines.Uncovered)
		if len(ranges) > 10 {
			ranges = ranges[:10]
		}
		hint = "add tests for lines " + path.Base(e.n.File) + ":" + strings.Join(ranges, ", ")
	}
	e.breach(r, KindCoverage, *c.Percent, limit, hint, details...)
}

func (e evaluator) critical() bool {
	for _, p := range e.th.CriticalPaths {
		if golang.MatchGlob(p, e.n.File) {
			return true
		}
	}
	return false
}

func (e evaluator) mutation(r *Report) {
	mu := e.m.Mutation
	switch {
	case mu == nil || mu.Stale:
		e.missing(r, KindMutation, "mutation not run since last change (run: qtldr mutate --func "+e.name+")")
	case !metrics.MutationGraded(e.m):
	case mu.Score == nil:
		e.missing(r, KindMutation, fmt.Sprintf("no mutant could run: %d not covered by tests", metrics.TestableNotCovered(e.m)))
	case *mu.Score < e.th.MutationMin:
		e.breach(r, KindMutation, *mu.Score, e.th.MutationMin, "each survivor is one missing assertion", survivors(e.n.File, mu.Mutants, 5)...)
	}
}

func survivors(file string, ms []model.Mutant, limit int) []string {
	var out []string
	for _, m := range ms {
		if m.Status != "LIVED" {
			continue
		}
		if len(out) == limit {
			break
		}
		out = append(out, fmt.Sprintf("%s:%d %s", path.Base(file), m.Line, m.Description))
	}
	return out
}

// Short is the display name of an ID: "…/internal/pricing.applyTiered" →
// "pricing.applyTiered".
func Short(id model.ID) string { return path.Base(string(id)) }
