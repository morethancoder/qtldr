package mutate

import (
	"cmp"
	"slices"

	"github.com/morethancoder/qtldr/internal/model"
)

// Plan says which packages a mutate run tests.
type Plan struct {
	Run      []Item     `json:"run"`
	UpToDate []model.ID `json:"up_to_date"`
	// Skipped are packages over [mutation].max_functions for this run.
	Skipped []Item `json:"skipped"`
}

// Item is one package to test.
type Item struct {
	Pkg   model.ID `json:"package"`
	Dir   string   `json:"dir"`
	Funcs int      `json:"functions"`
	risk  float64
}

// MakePlan picks the packages in scope (nil = all) whose cached results are
// missing, failed or stale (all of them with force), ordered by their
// highest CRAP, and stops adding once max functions are reached (the first
// package always runs).
func MakePlan(g model.Graph, caches map[model.ID]Cache, scope []model.ID, force bool, max int) Plan {
	p := Plan{Run: []Item{}, UpToDate: []model.ID{}, Skipped: []Item{}}
	due := p.sortOut(g, caches, scope, force)
	slices.SortFunc(due, func(a, b Item) int { return cmp.Or(cmp.Compare(b.risk, a.risk), cmp.Compare(a.Pkg, b.Pkg)) })
	p.fill(due, max)
	return p
}

// sortOut returns the packages in scope that need a run; current ones go to
// UpToDate. Packages without functions are left out.
func (p *Plan) sortOut(g model.Graph, caches map[model.ID]Cache, scope []model.ID, force bool) []Item {
	var due []Item
	for _, n := range g.Nodes {
		if !inScope(n, scope) {
			continue
		}
		it := item(g, n)
		switch {
		case it.Funcs == 0:
		case !force && caches[n.ID].Current(g, n.ID):
			p.UpToDate = append(p.UpToDate, n.ID)
		default:
			due = append(due, it)
		}
	}
	return due
}

// inScope: a package, and in scope when a scope is given.
func inScope(n model.Node, scope []model.ID) bool {
	return n.Kind == model.KindPackage && (scope == nil || slices.Contains(scope, n.ID))
}

// fill runs packages in order until max functions; the first always runs.
func (p *Plan) fill(due []Item, max int) {
	total := 0
	for _, it := range due {
		if len(p.Run) > 0 && max > 0 && total+it.Funcs > max {
			p.Skipped = append(p.Skipped, it)
			continue
		}
		total += it.Funcs
		p.Run = append(p.Run, it)
	}
}

// item counts a package's functions and its highest CRAP (missing CRAP
// ranks below any measured one).
func item(g model.Graph, pkg model.Node) Item {
	it := Item{Pkg: pkg.ID, Dir: pkg.Dir, risk: -1}
	for _, n := range g.Nodes {
		if n.Kind == model.KindFunc && n.Parent == pkg.ID {
			it.Funcs++
			if c := g.Metrics[n.ID].CRAP; c != nil && *c > it.risk {
				it.risk = *c
			}
		}
	}
	return it
}
