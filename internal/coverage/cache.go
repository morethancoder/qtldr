package coverage

import (
	"slices"
	"time"

	"github.com/morethancoder/qtldr/internal/model"
)

// CacheSchema is the version of .qtldr/cache/coverage.json.
const CacheSchema = 1

// Cache keeps the last coverage result per function so a structure-only
// analyze still shows coverage, marked stale when the body changed. Lines
// are stored as offsets from the function's first line, so a function that
// only moved keeps correct line numbers.
type Cache struct {
	Schema        int                 `json:"schema"`
	RanAt         *time.Time          `json:"ran_at"`
	Functions     map[model.ID]Entry  `json:"functions"`
	PackageErrors map[model.ID]string `json:"package_errors,omitempty"`
}

// Entry is one function's cached coverage.
type Entry struct {
	BodyHash string    `json:"body_hash"`
	RanAt    time.Time `json:"ran_at"`
	Stmts    int       `json:"stmts"`
	Covered  int       `json:"covered"`
	// Line offsets from the function's first line, by state.
	CoveredLines   []int `json:"covered_offsets"`
	UncoveredLines []int `json:"uncovered_offsets"`
	PartialLines   []int `json:"partial_offsets"`
}

// NewCache returns an empty cache.
func NewCache() Cache {
	return Cache{Schema: CacheSchema, Functions: map[model.ID]Entry{}, PackageErrors: map[model.ID]string{}}
}

// Update records a run over the packages in ran. measured is Map's result;
// failed maps a package to why its coverage is not measured. Functions of
// packages that ran without failing and have no measured entry had no
// statements in the profile.
func (c *Cache) Update(g model.Graph, measured map[model.ID]model.Coverage, ran []model.ID, failed map[model.ID]string, at time.Time) {
	c.init()
	c.RanAt = &at
	c.forget(ran)
	for _, n := range g.Nodes {
		if n.Kind == model.KindFunc && slices.Contains(ran, n.Parent) && failed[n.Parent] == "" {
			c.Functions[n.ID] = entryFor(n, measured[n.ID], at)
		}
	}
	c.recordErrors(ran, failed)
}

func (c *Cache) init() {
	if c.Functions == nil {
		c.Functions = map[model.ID]Entry{}
	}
	if c.PackageErrors == nil {
		c.PackageErrors = map[model.ID]string{}
	}
}

// recordErrors sets the error of each failed package and clears it for the
// packages that ran cleanly.
func (c *Cache) recordErrors(ran []model.ID, failed map[model.ID]string) {
	for _, pkg := range ran {
		if msg := failed[pkg]; msg != "" {
			c.PackageErrors[pkg] = msg
		} else {
			delete(c.PackageErrors, pkg)
		}
	}
}

// forget drops entries whose ID belongs to a package in pkgs (so deleted
// functions do not linger).
func (c *Cache) forget(pkgs []model.ID) {
	for id := range c.Functions {
		for _, p := range pkgs {
			if len(id) > len(p) && id[:len(p)] == p && id[len(p)] == '.' {
				delete(c.Functions, id)
				break
			}
		}
	}
}

func entryFor(n model.Node, cov model.Coverage, at time.Time) Entry {
	e := Entry{BodyHash: n.BodyHash, RanAt: at, Stmts: cov.Stmts, Covered: cov.Covered}
	if cov.Lines != nil {
		e.CoveredLines = offsets(cov.Lines.Covered, n.Line)
		e.UncoveredLines = offsets(cov.Lines.Uncovered, n.Line)
		e.PartialLines = offsets(cov.Lines.Partial, n.Line)
	}
	return e
}

func offsets(lines []int, base int) []int {
	out := make([]int, len(lines))
	for i, l := range lines {
		out[i] = l - base
	}
	return out
}

// Resolve returns coverage for the functions of g that have a cache entry,
// with absolute line numbers and Stale set when the body hash changed.
func (c Cache) Resolve(g model.Graph) map[model.ID]model.Coverage {
	out := map[model.ID]model.Coverage{}
	for _, n := range g.Nodes {
		e, ok := c.Functions[n.ID]
		if !ok || n.Kind != model.KindFunc {
			continue
		}
		out[n.ID] = model.Coverage{
			Stmts: e.Stmts, Covered: e.Covered, Percent: Percent(e.Covered, e.Stmts), Stale: e.BodyHash != n.BodyHash,
			Lines: &model.LineStates{
				Covered:   absolute(e.CoveredLines, n.Line),
				Uncovered: absolute(e.UncoveredLines, n.Line),
				Partial:   absolute(e.PartialLines, n.Line),
			},
		}
	}
	return out
}

func absolute(offs []int, base int) []int {
	out := make([]int, len(offs))
	for i, o := range offs {
		out[i] = o + base
	}
	return out
}
