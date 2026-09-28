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
// measured", never zero.
type Metrics struct {
	CC        *int `json:"cc,omitempty"`
	Cognitive *int `json:"cognitive,omitempty"`
	LOC       *int `json:"loc,omitempty"`
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
