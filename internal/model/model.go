// Package model holds the language-neutral snapshot types: nodes, edges and
// metrics. It is pure: no I/O, no clock, no globals.
package model

import (
	"cmp"
	"slices"
	"time"
)

// SchemaVersion is the snapshot.json schema written by this build.
const SchemaVersion = 1

// ID identifies a node: an import path, a function, method or type ID, or an
// external module path.
type ID string

// Kind is what a node is.
type Kind string

// Node kinds.
const (
	KindModule   Kind = "module"
	KindPackage  Kind = "package"
	KindFunc     Kind = "func"
	KindType     Kind = "type"
	KindExternal Kind = "external"
)

// EdgeKind is how two nodes relate.
type EdgeKind string

// Edge kinds.
const (
	EdgeImports      EdgeKind = "imports"
	EdgeCalls        EdgeKind = "calls"
	EdgeCallsDynamic EdgeKind = "calls_dynamic"
	EdgeImplements   EdgeKind = "implements"
)

// Field is one field of a struct type.
type Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Node is one box on the map. Fields that do not apply to a kind are omitted.
type Node struct {
	ID        ID       `json:"id"`
	Kind      Kind     `json:"kind"`
	Name      string   `json:"name"`
	Parent    ID       `json:"parent,omitempty"`
	Dir       string   `json:"dir,omitempty"`
	File      string   `json:"file,omitempty"`
	Line      int      `json:"line,omitempty"`
	EndLine   int      `json:"end_line,omitempty"`
	DocLine   int      `json:"doc_line,omitempty"`
	Exported  *bool    `json:"exported,omitempty"`
	Recv      string   `json:"recv,omitempty"`
	PtrRecv   bool     `json:"ptr_recv,omitempty"`
	Signature string   `json:"signature,omitempty"`
	BodyHash  string   `json:"body_hash,omitempty"`
	TypeKind  string   `json:"type_kind,omitempty"`
	Fields    []Field  `json:"fields,omitempty"`
	Pure      *bool    `json:"pure,omitempty"`
	Effects   []string `json:"effects,omitempty"`
	Errors    []string `json:"errors,omitempty"`
}

// Edge is a directed relation between two nodes.
type Edge struct {
	From ID       `json:"from"`
	To   ID       `json:"to"`
	Kind EdgeKind `json:"kind"`
}

// Metrics are the measured values of one node. A nil field means "not
// measured", never zero. Functions use the first group; packages and the
// module use the roll-up group.
type Metrics struct {
	CC         *int      `json:"cc,omitempty"`
	Cognitive  *int      `json:"cognitive,omitempty"`
	LOC        *int      `json:"loc,omitempty"`
	Churn      *int      `json:"churn,omitempty"`
	ChurnScope string    `json:"churn_scope,omitempty"`
	Coverage   *Coverage `json:"coverage,omitempty"`
	CRAP       *float64  `json:"crap,omitempty"`
	Mutation   *Mutation `json:"mutation,omitempty"`
	Grades     *Grades   `json:"grades,omitempty"`

	// Roll-up (packages, module).
	CrapMax       *float64 `json:"crap_max,omitempty"`
	CrapAvg       *float64 `json:"crap_avg,omitempty"`
	Worst         ID       `json:"worst,omitempty"`
	CoverageError string   `json:"coverage_error,omitempty"`
	MutationError string   `json:"mutation_error,omitempty"`
}

// Coverage is statement coverage from go test. Percent is nil when the code
// has no statements.
type Coverage struct {
	Stmts   int         `json:"stmts"`
	Covered int         `json:"covered"`
	Percent *float64    `json:"percent"`
	Stale   bool        `json:"stale"`
	Lines   *LineStates `json:"lines,omitempty"`
}

// LineStates lists source lines by coverage state (absolute line numbers).
type LineStates struct {
	Covered   []int `json:"covered"`
	Uncovered []int `json:"uncovered"`
	Partial   []int `json:"partial"`
}

// Mutation is the result of mutation testing. Score is nil when killed +
// survived is 0.
type Mutation struct {
	Killed     int      `json:"killed"`
	Survived   int      `json:"survived"`
	NotCovered int      `json:"not_covered"`
	TimedOut   int      `json:"timed_out"`
	Score      *float64 `json:"score"`
	Stale      bool     `json:"stale"`
	Mutants    []Mutant `json:"mutants,omitempty"`
}

// Sites is the number of mutation sites found (every status except the
// ignored ones).
func (m Mutation) Sites() int { return m.Killed + m.Survived + m.NotCovered + m.TimedOut }

// Mutant is one code change made by the mutation engine.
type Mutant struct {
	Line        int    `json:"line"`
	Col         int    `json:"col"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Description string `json:"description"`
}

// Grades are 1 (worst) to 10 (best). Mutation is nil when the code has no
// mutation sites, so it is left out of Combined.
type Grades struct {
	CRAP     int  `json:"crap"`
	Mutation *int `json:"mutation"`
	Coverage int  `json:"coverage"`
	Combined int  `json:"combined"`
}

// Graph is what a language provider's scan returns.
type Graph struct {
	Nodes   []Node         `json:"nodes"`
	Edges   []Edge         `json:"edges"`
	Metrics map[ID]Metrics `json:"metrics"`
}

// GitInfo is the repository state at scan time.
type GitInfo struct {
	Head  string `json:"head"`
	Dirty bool   `json:"dirty"`
}

// Runs records when each kind of analysis last ran; nil means never.
type Runs struct {
	Structure *time.Time `json:"structure"`
	Coverage  *time.Time `json:"coverage"`
	Mutation  *time.Time `json:"mutation"`
}

// Snapshot is the content of .qtldr/snapshot.json.
type Snapshot struct {
	Schema      int       `json:"schema"`
	ToolVersion string    `json:"tool_version"`
	Module      string    `json:"module"`
	Generated   time.Time `json:"generated"`
	Git         *GitInfo  `json:"git"`
	Runs        Runs      `json:"runs"`
	Graph
}

// Sort orders nodes by ID and edges by (from, to, kind) so output is stable.
func (g *Graph) Sort() {
	slices.SortFunc(g.Nodes, func(a, b Node) int { return cmp.Compare(a.ID, b.ID) })
	slices.SortFunc(g.Edges, func(a, b Edge) int {
		return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To), cmp.Compare(a.Kind, b.Kind))
	})
}

// Node returns the node with the given ID.
func (g *Graph) Node(id ID) (Node, bool) {
	i := slices.IndexFunc(g.Nodes, func(n Node) bool { return n.ID == id })
	if i < 0 {
		return Node{}, false
	}
	return g.Nodes[i], true
}

// Neighbors returns the nodes with an edge of one of the given kinds into id
// (in) and out of id (out), sorted by ID.
func (g *Graph) Neighbors(id ID, kinds ...EdgeKind) (in, out []ID) {
	for _, e := range g.Edges {
		if !slices.Contains(kinds, e.Kind) {
			continue
		}
		if e.To == id {
			in = append(in, e.From)
		}
		if e.From == id {
			out = append(out, e.To)
		}
	}
	slices.Sort(in)
	slices.Sort(out)
	return in, out
}

// Ptr returns a pointer to v, for optional fields.
func Ptr[T any](v T) *T { return &v }
