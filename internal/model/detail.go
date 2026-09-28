package model

import "slices"

// Detail is everything the graph says about one node; `qtldr show`, the
// /api/node endpoint and MCP get_node all return it.
type Detail struct {
	Node       Node     `json:"node"`
	Metrics    *Metrics `json:"metrics,omitempty"`
	Children   []ID     `json:"children,omitempty"`
	Callers    []ID     `json:"callers,omitempty"`
	Callees    []ID     `json:"callees,omitempty"`
	Imports    []ID     `json:"imports,omitempty"`
	ImportedBy []ID     `json:"imported_by,omitempty"`
	// Types: their methods, the interfaces they implement, and (for
	// interfaces) the types implementing them.
	Methods       []ID `json:"methods,omitempty"`
	Implements    []ID `json:"implements,omitempty"`
	ImplementedBy []ID `json:"implemented_by,omitempty"`
}

// Detail returns the node with its metrics and neighbors. ok is false if id
// is not in the graph.
func (g *Graph) Detail(id ID) (d Detail, ok bool) {
	d.Node, ok = g.Node(id)
	if !ok {
		return d, false
	}
	if m, has := g.Metrics[id]; has {
		d.Metrics = &m
	}
	d.Callers, d.Callees = g.Neighbors(id, EdgeCalls, EdgeCallsDynamic)
	d.ImportedBy, d.Imports = g.Neighbors(id, EdgeImports)
	d.ImplementedBy, d.Implements = g.Neighbors(id, EdgeImplements)
	for _, n := range g.Nodes {
		if n.Parent == id {
			d.Children = append(d.Children, n.ID)
		}
		if isMethodOf(n, d.Node) {
			d.Methods = append(d.Methods, n.ID)
		}
	}
	slices.Sort(d.Children)
	slices.Sort(d.Methods)
	return d, true
}

// isMethodOf: fn is a method declared on type t.
func isMethodOf(fn, t Node) bool {
	return t.Kind == KindType && fn.Kind == KindFunc && fn.Parent == t.Parent && fn.Recv == t.Name
}

// IDs returns every node ID.
func (g *Graph) IDs() []ID {
	ids := make([]ID, len(g.Nodes))
	for i, n := range g.Nodes {
		ids[i] = n.ID
	}
	return ids
}

// Counts returns the number of nodes, edges and metrics entries.
func (g Graph) Counts() (int, int, int) { return len(g.Nodes), len(g.Edges), len(g.Metrics) }
