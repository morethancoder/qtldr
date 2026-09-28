package golang

import (
	"context"
	"fmt"
	"go/types"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/morethancoder/qtldr/internal/model"
)

// addVTAEdges resolves dynamic calls (interface methods and function values)
// with Variable Type Analysis and adds calls_dynamic edges to the concrete
// module functions they can reach. It loads every package with full syntax,
// so it is opt-in ([project].calls = "vta"; cost in docs/decisions.md).
func (s *scanner) addVTAEdges(ctx context.Context, patterns []string) error {
	cfg := &packages.Config{Mode: packages.LoadAllSyntax, Dir: s.root, Context: ctx}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return fmt.Errorf("vta: load %v: %w", patterns, err)
	}
	prog, _ := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()
	cg := vta.CallGraph(ssautil.AllFunctions(prog), cha.CallGraph(prog))
	return callgraph.GraphVisitEdges(cg, func(e *callgraph.Edge) error {
		s.addDynamicEdge(e)
		return nil
	})
}

// addDynamicEdge records a caller → callee edge for a dynamic call site
// between two module functions.
func (s *scanner) addDynamicEdge(e *callgraph.Edge) {
	if e.Site == nil || e.Site.Common().StaticCallee() != nil {
		return
	}
	from, ok1 := s.ssaID(e.Caller.Func)
	to, ok2 := s.ssaID(e.Callee.Func)
	if ok1 && ok2 && from != to {
		s.addEdge(from, to, model.EdgeCallsDynamic)
	}
}

// ssaID maps an SSA function (closures map to their enclosing function) to
// the node ID of a module function.
func (s *scanner) ssaID(fn *ssa.Function) (model.ID, bool) {
	fn = topLevel(fn)
	if fn == nil {
		return "", false
	}
	obj, ok := fn.Object().(*types.Func)
	if !ok || obj.Pkg() == nil || !s.inModule[obj.Pkg().Path()] {
		return "", false
	}
	id, _, ok := calleeID(obj)
	return id, ok
}

// topLevel is the declared function a closure or instantiation comes from.
func topLevel(fn *ssa.Function) *ssa.Function {
	for fn != nil && fn.Parent() != nil {
		fn = fn.Parent()
	}
	if fn != nil && fn.Origin() != nil {
		fn = fn.Origin()
	}
	return fn
}
