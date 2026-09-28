// Package glossary serves terms.yaml: the single source for UI tooltips,
// `qtldr explain` and the glossary map attached to MCP results.
package glossary

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/morethancoder/qtldr/internal/config"
)

//go:embed terms.yaml
var termsYAML []byte

// Link is an external reference shown as "text ↗".
type Link struct {
	Text string `yaml:"text" json:"text"`
	URL  string `yaml:"url" json:"url"`
}

// Term is one glossary entry.
type Term struct {
	Key   string `yaml:"-" json:"key"`
	Title string `yaml:"title" json:"title"`
	Short string `yaml:"short" json:"short"`
	Body  string `yaml:"body" json:"body"`
	Good  string `yaml:"good" json:"good"`
	Links []Link `yaml:"links" json:"links,omitempty"`
}

// Glossary is every term by key, with thresholds substituted.
type Glossary map[string]Term

type file struct {
	Schema int             `yaml:"schema"`
	Terms  map[string]Term `yaml:"terms"`
}

// Parse decodes terms.yaml content.
func Parse(data []byte) (Glossary, error) {
	var f file
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("terms.yaml: %w", err)
	}
	if f.Schema != 1 || len(f.Terms) == 0 {
		return nil, fmt.Errorf("terms.yaml: want schema 1 with terms, got schema %d and %d terms", f.Schema, len(f.Terms))
	}
	g := Glossary{}
	for k, t := range f.Terms {
		t.Key = k
		g[k] = t
	}
	return g, nil
}

// Load returns the embedded glossary with the configured thresholds
// substituted for {crap_max}, {coverage_min}, {mutation_min},
// {cognitive_max} and {churn_months}.
func Load(cfg config.Config) Glossary {
	g, err := Parse(termsYAML)
	if err != nil {
		panic("glossary: embedded terms.yaml is invalid: " + err.Error())
	}
	return g.Substitute(Vars(cfg))
}

// Vars are the placeholder values taken from the configuration.
func Vars(cfg config.Config) map[string]string {
	t := cfg.Thresholds
	return map[string]string{
		"crap_max":      trimFloat(t.CrapMax),
		"coverage_min":  trimFloat(t.CoverageMin),
		"mutation_min":  trimFloat(t.MutationMin),
		"cognitive_max": fmt.Sprint(t.CognitiveMax),
		"churn_months":  fmt.Sprint(cfg.Churn.WindowMonths),
	}
}

func trimFloat(f float64) string { return strings.TrimSuffix(fmt.Sprintf("%g", f), ".0") }

// Substitute returns a copy with every {name} replaced from vars.
func (g Glossary) Substitute(vars map[string]string) Glossary {
	pairs := make([]string, 0, 2*len(vars))
	for k, v := range vars {
		pairs = append(pairs, "{"+k+"}", v)
	}
	r := strings.NewReplacer(pairs...)
	out := Glossary{}
	for k, t := range g {
		t.Title, t.Short, t.Body, t.Good = r.Replace(t.Title), r.Replace(t.Short), r.Replace(t.Body), r.Replace(t.Good)
		out[k] = t
	}
	return out
}

// Keys returns the term keys, sorted.
func (g Glossary) Keys() []string {
	keys := make([]string, 0, len(g))
	for k := range g {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// Lookup finds a term by key, ignoring case, spaces and hyphens
// ("not covered" → not_covered).
func (g Glossary) Lookup(name string) (Term, error) {
	key := strings.NewReplacer(" ", "_", "-", "_").Replace(strings.ToLower(strings.TrimSpace(name)))
	if t, ok := g[key]; ok {
		return t, nil
	}
	return Term{}, fmt.Errorf("no glossary term %q; known terms: %s", name, strings.Join(g.Keys(), ", "))
}

// Subset returns the named terms that exist, for attaching to results.
func (g Glossary) Subset(keys ...string) Glossary {
	out := Glossary{}
	for _, k := range keys {
		if t, ok := g[k]; ok {
			out[k] = t
		}
	}
	return out
}
