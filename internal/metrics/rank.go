// Package metrics computes derived values (rankings now; CRAP, grades and
// roll-ups from M1) from a graph. It is pure: no I/O, no clock, no globals.
package metrics

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/morethancoder/qtldr/internal/model"
)

// Metric names accepted by Rank.
const (
	CRAPMetric = "crap"
	Cognitive  = "cognitive"
	CC         = "cc"
	Coverage   = "coverage"
	Mutation   = "mutation"
)

// Metrics lists the names Rank accepts.
var Metrics = []string{CRAPMetric, Cognitive, CC, Coverage, Mutation}

// Ranked is one entry of a ranking.
type Ranked struct {
	ID    model.ID `json:"id"`
	Value float64  `json:"value"`
	Stale bool     `json:"stale,omitempty"`
}

// Ranking is the result of Rank: the top n functions, worst first, and the
// number of functions that have no value for the metric.
type Ranking struct {
	Metric      string   `json:"metric"`
	Items       []Ranked `json:"items"`
	NotMeasured int      `json:"not_measured"`
}

type metricDef struct {
	get func(model.Metrics) (*float64, bool) // value, stale
	low bool                                 // lower is worse
}

func intMetric(get func(model.Metrics) *int) func(model.Metrics) (*float64, bool) {
	return func(m model.Metrics) (*float64, bool) {
		if v := get(m); v != nil {
			return model.Ptr(float64(*v)), false
		}
		return nil, false
	}
}

var defs = map[string]metricDef{
	Cognitive:  {get: intMetric(func(m model.Metrics) *int { return m.Cognitive })},
	CC:         {get: intMetric(func(m model.Metrics) *int { return m.CC })},
	CRAPMetric: {get: func(m model.Metrics) (*float64, bool) { return m.CRAP, m.Coverage != nil && m.Coverage.Stale }},
	Coverage: {low: true, get: func(m model.Metrics) (*float64, bool) {
		if m.Coverage == nil {
			return nil, false
		}
		return m.Coverage.Percent, m.Coverage.Stale
	}},
	Mutation: {low: true, get: func(m model.Metrics) (*float64, bool) {
		if m.Mutation == nil {
			return nil, false
		}
		return m.Mutation.Score, m.Mutation.Stale
	}},
}

// Rank lists functions by metric, worst first (highest CRAP, cognitive and
// CC; lowest coverage and mutation score), ties by ID. n ≤ 0 means all.
// scope, when non-nil, limits the ranking to those IDs.
func Rank(g model.Graph, metric string, n int, scope []model.ID) (Ranking, error) {
	def, ok := defs[metric]
	if !ok {
		return Ranking{}, fmt.Errorf("unknown metric %q; use one of: crap, cognitive, cc, coverage, mutation", metric)
	}
	r := collect(g, def, scope)
	r.Metric = metric
	sortRanked(r.Items, def.low)
	if n > 0 && len(r.Items) > n {
		r.Items = r.Items[:n]
	}
	return r, nil
}

// collect gathers the metric of every function in scope (nil = all).
func collect(g model.Graph, def metricDef, scope []model.ID) Ranking {
	r := Ranking{Items: []Ranked{}}
	for _, node := range g.Nodes {
		if node.Kind != model.KindFunc || (scope != nil && !slices.Contains(scope, node.ID)) {
			continue
		}
		v, stale := def.get(g.Metrics[node.ID])
		if v == nil {
			r.NotMeasured++
			continue
		}
		r.Items = append(r.Items, Ranked{ID: node.ID, Value: *v, Stale: stale})
	}
	return r
}

// sortRanked puts the worst first: highest values, or lowest when low.
func sortRanked(items []Ranked, low bool) {
	sign := -1
	if low {
		sign = 1
	}
	slices.SortFunc(items, func(a, b Ranked) int {
		return cmp.Or(sign*cmp.Compare(a.Value, b.Value), cmp.Compare(a.ID, b.ID))
	})
}
