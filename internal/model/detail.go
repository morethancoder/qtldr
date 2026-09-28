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
	for _, n := range g.Nodes {
		if n.Parent == id {
			d.Children = append(d.Children, n.ID)
		}
	}
	slices.Sort(d.Children)
	return d, true
}

// IDs returns every node ID.
func (g *Graph) IDs() []ID {
	ids := make([]ID, len(g.Nodes))
	for i, n := range g.Nodes {
		ids[i] = n.ID
	}
	return ids
}
