package metrics

import (
	"path"
	"slices"
	"strings"

	"github.com/morethancoder/qtldr/internal/model"
)

// PurityResult is the purity of one function after propagation.
type PurityResult struct {
	Pure    bool
	Effects []string
}

// Purity propagates effects over calls edges to a fixed point (PLAN.md §6.2
// rule 5). A function starts effectful when the provider recorded local
// effects on its node. It becomes effectful when it calls an effectful
// function, or calls through an interface method whose implementations in
// the module are not all known and pure. IDs in allow are always pure.
func Purity(g model.Graph, allow []model.ID) map[model.ID]PurityResult {
	res := map[model.ID]PurityResult{}
	for _, n := range g.Nodes {
		switch {
		case n.Kind != model.KindFunc:
		case slices.Contains(allow, n.ID):
			res[n.ID] = PurityResult{Pure: true}
		default:
			res[n.ID] = PurityResult{Pure: len(n.Effects) == 0, Effects: slices.Clone(n.Effects)}
		}
	}
	impls := implementations(g, res)
	for changed := true; changed; {
		changed = false
		for _, e := range g.Edges {
			if propagate(res, impls, allow, e) {
				changed = true
			}
		}
	}
	return res
}

// propagate marks e.From effectful if e makes it so; it reports a change.
func propagate(res map[model.ID]PurityResult, impls map[model.ID][]model.ID, allow []model.ID, e model.Edge) bool {
	from, ok := res[e.From]
	if !ok || !from.Pure || slices.Contains(allow, e.From) {
		return false
	}
	reason := effectOf(res, impls, e)
	if reason == "" {
		return false
	}
	res[e.From] = PurityResult{Pure: false, Effects: append(from.Effects, reason)}
	return true
}

// effectOf is the reason edge e makes its caller effectful, or "".
func effectOf(res map[model.ID]PurityResult, impls map[model.ID][]model.ID, e model.Edge) string {
	switch e.Kind {
	case model.EdgeCalls:
		if to, ok := res[e.To]; ok && !to.Pure {
			return "calls " + short(e.To) + " (effectful)"
		}
	case model.EdgeCallsDynamic:
		return dynamicEffect(res, impls, e.To)
	}
	return ""
}

// dynamicEffect: a call resolved to a concrete function (vta) is effectful
// when that function is; a call to an interface method is pure only when
// every module implementation is.
func dynamicEffect(res map[model.ID]PurityResult, impls map[model.ID][]model.ID, to model.ID) string {
	if r, ok := res[to]; ok {
		if !r.Pure {
			return "may call " + short(to) + " (effectful)"
		}
		return ""
	}
	if !allPure(res, impls[to]) {
		return "calls " + short(to) + " through an interface"
	}
	return ""
}

// allPure: at least one implementation, and every one known and pure.
func allPure(res map[model.ID]PurityResult, ids []model.ID) bool {
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if r, ok := res[id]; !ok || !r.Pure {
			return false
		}
	}
	return true
}

// implementations maps each interface method ID (<pkg>.<Iface>.<Method>)
// called dynamically to the method IDs of the module types that implement
// the interface. A method promoted from an embedded field has no node of its
// own; its ID is still listed so allPure treats it as unknown.
func implementations(g model.Graph, res map[model.ID]PurityResult) map[model.ID][]model.ID {
	implementers := map[model.ID][]model.ID{} // interface → types
	for _, e := range g.Edges {
		if e.Kind == model.EdgeImplements {
			implementers[e.To] = append(implementers[e.To], e.From)
		}
	}
	out := map[model.ID][]model.ID{}
	for _, e := range g.Edges {
		if e.Kind != model.EdgeCallsDynamic {
			continue
		}
		i := strings.LastIndex(string(e.To), ".")
		iface, method := e.To[:i], string(e.To[i+1:])
		for _, t := range implementers[iface] {
			out[e.To] = append(out[e.To], t+model.ID("."+method))
		}
	}
	return out
}

// applyPurity sets Pure and Effects on functions, and Pure on packages
// (pure iff every function is).
func applyPurity(g *model.Graph, res map[model.ID]PurityResult) {
	pkgPure := map[model.ID]bool{}
	for i, n := range g.Nodes {
		if n.Kind == model.KindPackage {
			if _, seen := pkgPure[n.ID]; !seen {
				pkgPure[n.ID] = true
			}
		}
		r, ok := res[n.ID]
		if !ok {
			continue
		}
		g.Nodes[i].Pure = model.Ptr(r.Pure)
		g.Nodes[i].Effects = r.Effects
		if !r.Pure {
			pkgPure[n.Parent] = false
		}
	}
	for i, n := range g.Nodes {
		if n.Kind == model.KindPackage {
			g.Nodes[i].Pure = model.Ptr(pkgPure[n.ID])
		}
	}
}

// short is the last path element of an ID: "…/internal/pricing.validate" →
// "pricing.validate".
func short(id model.ID) string { return path.Base(string(id)) }
