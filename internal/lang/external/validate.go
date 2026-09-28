// Package external runs language providers that speak qtldr's JSON contract
// (docs/provider-contract.md) and validates what they print.
package external

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/morethancoder/qtldr/internal/model"
)

// Problem is one contract violation at a JSON path.
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (p Problem) String() string { return p.Path + ": " + p.Message }

var kinds = []string{"module", "package", "func", "type", "external"}
var edgeKinds = []string{"imports", "calls", "calls_dynamic", "implements"}

// Validate parses provider output and checks it against the contract. The
// graph is returned only when there are no problems (pure).
func Validate(b []byte) (model.Graph, []Problem) {
	var raw struct {
		Nodes   []json.RawMessage          `json:"nodes"`
		Edges   []json.RawMessage          `json:"edges"`
		Metrics map[string]json.RawMessage `json:"metrics"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return model.Graph{}, []Problem{{"$", "not a JSON object with nodes, edges and metrics: " + err.Error()}}
	}
	v := validator{ids: map[model.ID]model.Kind{}}
	g := model.Graph{Nodes: v.nodes(raw.Nodes), Metrics: map[model.ID]model.Metrics{}}
	v.parents(g.Nodes)
	for i, re := range raw.Edges {
		if e, ok := v.edge(i, re); ok {
			g.Edges = append(g.Edges, e)
		}
	}
	for _, id := range sortedKeys(raw.Metrics) {
		if m, ok := v.metrics(id, raw.Metrics[id]); ok {
			g.Metrics[model.ID(id)] = m
		}
	}
	if len(v.problems) > 0 {
		return model.Graph{}, v.problems
	}
	g.Sort()
	return g, nil
}

func (v *validator) nodes(raw []json.RawMessage) []model.Node {
	if raw == nil {
		v.add("$.nodes", "is required (an array)")
	}
	var out []model.Node
	for i, rn := range raw {
		if n, ok := v.node(i, rn); ok {
			out = append(out, n)
		}
	}
	return out
}

type validator struct {
	ids      map[model.ID]model.Kind
	problems []Problem
}

func (v *validator) add(path, format string, args ...any) {
	v.problems = append(v.problems, Problem{path, fmt.Sprintf(format, args...)})
}

func (v *validator) node(i int, raw json.RawMessage) (model.Node, bool) {
	path := fmt.Sprintf("$.nodes[%d]", i)
	var n model.Node
	if err := strictDecode(raw, &n); err != nil {
		v.add(path, "%v", err)
		return n, false
	}
	before := len(v.problems)
	switch {
	case n.ID == "":
		v.add(path+".id", "is required")
	case v.ids[n.ID] != "":
		v.add(path+".id", "duplicate id %q", n.ID)
	}
	if !slices.Contains(kinds, string(n.Kind)) {
		v.add(path+".kind", "must be one of %s, got %q", strings.Join(kinds, ", "), n.Kind)
	}
	if n.Name == "" {
		v.add(path+".name", "is required")
	}
	v.location(path, n)
	v.ids[n.ID] = n.Kind
	return n, len(v.problems) == before
}

// location: functions and types need file and a line range.
func (v *validator) location(path string, n model.Node) {
	if n.Kind != model.KindFunc && n.Kind != model.KindType {
		return
	}
	if !relativePath(n.File) {
		v.add(path+".file", "must be a path relative to root, got %q", n.File)
	}
	if !validLines(n) {
		v.add(path+".line", "needs line ≥ 1 and end_line ≥ line (got %d–%d)", n.Line, n.EndLine)
	}
}

func relativePath(f string) bool {
	return f != "" && !strings.HasPrefix(f, "/") && !strings.Contains(f, "..")
}

// validLines: line ≥ 1, and for functions end_line ≥ line.
func validLines(n model.Node) bool {
	return n.Line >= 1 && (n.Kind != model.KindFunc || n.EndLine >= n.Line)
}

func (v *validator) parents(nodes []model.Node) {
	for i, n := range nodes {
		if n.Parent != "" && v.ids[n.Parent] == "" {
			v.add(fmt.Sprintf("$.nodes[%d].parent", i), "unknown node %q", n.Parent)
		}
	}
}

func (v *validator) edge(i int, raw json.RawMessage) (model.Edge, bool) {
	path := fmt.Sprintf("$.edges[%d]", i)
	var e model.Edge
	if err := strictDecode(raw, &e); err != nil {
		v.add(path, "%v", err)
		return e, false
	}
	before := len(v.problems)
	if !slices.Contains(edgeKinds, string(e.Kind)) {
		v.add(path+".kind", "must be one of %s, got %q", strings.Join(edgeKinds, ", "), e.Kind)
	}
	if v.ids[e.From] == "" {
		v.add(path+".from", "unknown node %q", e.From)
	}
	if v.ids[e.To] == "" {
		v.add(path+".to", "unknown node %q", e.To)
	}
	return e, len(v.problems) == before
}

func (v *validator) metrics(id string, raw json.RawMessage) (model.Metrics, bool) {
	path := fmt.Sprintf("$.metrics[%q]", id)
	var m model.Metrics
	if err := strictDecode(raw, &m); err != nil {
		v.add(path, "%v", err)
		return m, false
	}
	before := len(v.problems)
	if v.ids[model.ID(id)] == "" {
		v.add(path, "unknown node %q", id)
	}
	if m.CC != nil && *m.CC < 1 {
		v.add(path+".cc", "must be ≥ 1, got %d", *m.CC)
	}
	if !validCoverage(m.Coverage) {
		v.add(path+".coverage", "covered must be ≤ stmts and percent within 0–100")
	}
	if mu := m.Mutation; mu != nil && !percentOK(mu.Score) {
		v.add(path+".mutation.score", "must be within 0–100, got %g", *mu.Score)
	}
	return m, len(v.problems) == before
}

func validCoverage(c *model.Coverage) bool {
	return c == nil || (c.Covered <= c.Stmts && percentOK(c.Percent))
}

// percentOK: absent, or within 0–100.
func percentOK(p *float64) bool { return p == nil || (*p >= 0 && *p <= 100) }

// strictDecode rejects unknown fields so typos surface as problems.
func strictDecode(raw json.RawMessage, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
