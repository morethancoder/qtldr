package mutate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/morethancoder/qtldr/internal/metrics"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/store"
)

// CacheSchema is the version of .qtldr/mutation/*.json.
const CacheSchema = 1

// Cache is one package's committed mutation results (PLAN.md §5.3).
type Cache struct {
	Schema        int                `json:"schema"`
	Package       model.ID           `json:"package"`
	Engine        string             `json:"engine"`
	EngineVersion string             `json:"engine_version"`
	RanAt         *time.Time         `json:"ran_at"`
	Error         string             `json:"error,omitempty"`
	Functions     map[model.ID]Entry `json:"functions"`
}

// Entry is one function's results. Mutant lines are offsets from the
// function's first line, so a function that only moved keeps them.
type Entry struct {
	BodyHash      string         `json:"body_hash"`
	Engine        string         `json:"engine"`
	EngineVersion string         `json:"engine_version"`
	RanAt         time.Time      `json:"ran_at"`
	Counts        Counts         `json:"counts"`
	Mutants       []CachedMutant `json:"mutants"`
}

// Counts are the per-status totals.
type Counts struct {
	Killed     int `json:"killed"`
	Survived   int `json:"survived"`
	NotCovered int `json:"not_covered"`
	TimedOut   int `json:"timed_out"`
}

// CachedMutant is a mutant with a line offset.
type CachedMutant struct {
	LineOffset  int    `json:"line_offset"`
	Col         int    `json:"col"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Description string `json:"description"`
}

// NewCache is an empty cache for pkg.
func NewCache(pkg model.ID) Cache {
	return Cache{Schema: CacheSchema, Package: pkg, Functions: map[model.ID]Entry{}}
}

// Record replaces the package's results with a fresh run.
func (c *Cache) Record(g model.Graph, results map[model.ID]model.Mutation, engine, version string, at time.Time) {
	c.Engine, c.EngineVersion, c.RanAt, c.Error = engine, version, &at, ""
	c.Functions = map[model.ID]Entry{}
	for _, n := range g.Nodes {
		mu, ok := results[n.ID]
		if !ok || n.Kind != model.KindFunc {
			continue
		}
		e := Entry{BodyHash: n.BodyHash, Engine: engine, EngineVersion: version, RanAt: at,
			Counts: Counts{mu.Killed, mu.Survived, mu.NotCovered, mu.TimedOut}, Mutants: []CachedMutant{}}
		for _, m := range mu.Mutants {
			e.Mutants = append(e.Mutants, CachedMutant{LineOffset: m.Line - n.Line, Col: m.Col, Type: m.Type, Status: m.Status, Description: m.Description})
		}
		c.Functions[n.ID] = e
	}
}

// Fail records why the package was not mutated; earlier results are kept
// (and show as stale once the code changes).
func (c *Cache) Fail(msg string, at time.Time) {
	c.Error, c.RanAt = msg, &at
}

// Current reports whether every function of pkg in g has a result for its
// current body and the last attempt did not fail.
func (c Cache) Current(g model.Graph, pkg model.ID) bool {
	if c.Error != "" || c.RanAt == nil {
		return false
	}
	for _, n := range g.Nodes {
		if n.Kind != model.KindFunc || n.Parent != pkg {
			continue
		}
		if e, ok := c.Functions[n.ID]; !ok || e.BodyHash != n.BodyHash {
			return false
		}
	}
	return true
}

// Resolve turns caches into per-function mutation results (absolute lines,
// Stale when the body changed), per-package errors, and the latest run time.
func Resolve(caches []Cache, g model.Graph) (map[model.ID]model.Mutation, map[model.ID]string, *time.Time) {
	entries, errs, latest := merge(caches)
	out := map[model.ID]model.Mutation{}
	for _, n := range g.Nodes {
		if e, ok := entries[n.ID]; ok && n.Kind == model.KindFunc {
			out[n.ID] = e.mutation(n)
		}
	}
	return out, errs, latest
}

// merge collects every function entry, every package error, and the latest
// run time.
func merge(caches []Cache) (map[model.ID]Entry, map[model.ID]string, *time.Time) {
	entries := map[model.ID]Entry{}
	errs := map[model.ID]string{}
	var latest *time.Time
	for _, c := range caches {
		maps.Copy(entries, c.Functions)
		if c.Error != "" {
			errs[c.Package] = c.Error
		}
		latest = later(latest, c.RanAt)
	}
	return entries, errs, latest
}

func later(a, b *time.Time) *time.Time {
	if a == nil || (b != nil && b.After(*a)) {
		return b
	}
	return a
}

func (e Entry) mutation(n model.Node) model.Mutation {
	mu := model.Mutation{
		Killed: e.Counts.Killed, Survived: e.Counts.Survived, NotCovered: e.Counts.NotCovered, TimedOut: e.Counts.TimedOut,
		Score: metrics.Score(e.Counts.Killed, e.Counts.Survived), Stale: e.BodyHash != n.BodyHash,
	}
	for _, m := range e.Mutants {
		mu.Mutants = append(mu.Mutants, model.Mutant{Line: n.Line + m.LineOffset, Col: m.Col, Type: m.Type, Status: m.Status, Description: m.Description})
	}
	return mu
}

// Dir is .qtldr/mutation under root.
func Dir(root string) string { return filepath.Join(root, store.Dir, "mutation") }

// Path is the cache file of pkg: the import path with "/" replaced by "__".
func Path(root string, pkg model.ID) string {
	return filepath.Join(Dir(root), strings.ReplaceAll(string(pkg), "/", "__")+".json")
}

// Load reads pkg's cache; a missing file is an empty cache.
func Load(root string, pkg model.ID) (Cache, error) {
	b, err := os.ReadFile(Path(root, pkg))
	if errors.Is(err, fs.ErrNotExist) {
		return NewCache(pkg), nil
	}
	if err != nil {
		return Cache{}, err
	}
	var c Cache
	if err := json.Unmarshal(b, &c); err != nil || c.Schema != CacheSchema {
		return NewCache(pkg), nil // rebuilt by the next run
	}
	if c.Functions == nil {
		c.Functions = map[model.ID]Entry{}
	}
	return c, nil
}

// LoadAll reads every cache file under .qtldr/mutation.
func LoadAll(root string) ([]Cache, error) {
	files, err := filepath.Glob(filepath.Join(Dir(root), "*.json"))
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	var out []Cache
	for _, f := range files {
		var c Cache
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, fmt.Errorf("%s is not valid JSON (%v); delete it and run qtldr mutate", f, err)
		}
		if c.Schema == CacheSchema {
			out = append(out, c)
		}
	}
	return out, nil
}

// Save writes pkg's cache.
func Save(root string, c Cache) error { return store.WriteJSON(Path(root, c.Package), c) }
