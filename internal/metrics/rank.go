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
	Cognitive = "cognitive"
	CC        = "cc"
)

// Ranked is one entry of a ranking.
type Ranked struct {
	ID    model.ID `json:"id"`
	Value float64  `json:"value"`
}

// Ranking is the result of Rank: the top n functions, worst first, and the
// number of functions that have no value for the metric.
type Ranking struct {
	Metric      string   `json:"metric"`
	Items       []Ranked `json:"items"`
	NotMeasured int      `json:"not_measured"`
}

var extractors = map[string]func(model.Metrics) *int{
	Cognitive: func(m model.Metrics) *int { return m.Cognitive },
	CC:        func(m model.Metrics) *int { return m.CC },
}

// Rank lists functions by metric, worst (highest) first, ties by ID.
// n ≤ 0 means all.
func Rank(g model.Graph, metric string, n int) (Ranking, error) {
	get, ok := extractors[metric]
	if !ok {
		return Ranking{}, fmt.Errorf("unknown metric %q; use one of: cognitive, cc", metric)
	}
	r := Ranking{Metric: metric, Items: []Ranked{}}
	for _, node := range g.Nodes {
		if node.Kind != model.KindFunc {
			continue
		}
		v := get(g.Metrics[node.ID])
		if v == nil {
			r.NotMeasured++
			continue
		}
		r.Items = append(r.Items, Ranked{ID: node.ID, Value: float64(*v)})
	}
	slices.SortFunc(r.Items, func(a, b Ranked) int {
		return cmp.Or(cmp.Compare(b.Value, a.Value), cmp.Compare(a.ID, b.ID))
	})
	if n > 0 && len(r.Items) > n {
		r.Items = r.Items[:n]
	}
	return r, nil
}
