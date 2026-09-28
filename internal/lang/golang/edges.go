package golang

import (
	"context"
	"fmt"
	"go/ast"
	"go/types"
	"maps"
	"slices"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/morethancoder/qtldr/internal/model"
)

// receiver returns the receiver type name of a method ("" for functions) and
// whether the receiver is a pointer.
func receiver(fn *ast.FuncDecl) (string, bool) {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return "", false
	}
	t := fn.Recv.List[0].Type
	_, ptr := t.(*ast.StarExpr)
	return recvTypeName(t), ptr
}

func recvTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return recvTypeName(t.X)
	case *ast.ParenExpr:
		return recvTypeName(t.X)
	case *ast.IndexExpr:
		return recvTypeName(t.X)
	case *ast.IndexListExpr:
		return recvTypeName(t.X)
	}
	return ""
}

// funcID is <pkg>.<Func> or <pkg>.<Type>.<Method>.
func funcID(pkg, recv, name string) model.ID {
	return model.ID(pkg + "." + qualifiedName(recv, name))
}

func qualifiedName(recv, name string) string {
	if recv == "" {
		return name
	}
	return recv + "." + name
}

// addCalls adds a calls edge for every static call to a module function in
// body, and a calls_dynamic edge for every call through a module interface.
// Calls inside function literals belong to the enclosing function.
func (s *scanner) addCalls(info *types.Info, from model.ID, body *ast.BlockStmt) {
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			s.addCall(info, from, call)
		}
		return true
	})
}

func (s *scanner) addCall(info *types.Info, from model.ID, call *ast.CallExpr) {
	fn, ok := typeutil.Callee(info, call).(*types.Func)
	if !ok || fn.Pkg() == nil {
		return
	}
	fn = fn.Origin()
	id, dynamic, ok := calleeID(fn)
	if !ok || !s.inModule[fn.Pkg().Path()] {
		return
	}
	kind := model.EdgeCalls
	if dynamic {
		kind = model.EdgeCallsDynamic
	}
	s.addEdge(from, id, kind)
}

// calleeID returns the node ID of fn and whether it is an interface method.
func calleeID(fn *types.Func) (model.ID, bool, bool) {
	recv := fn.Signature().Recv()
	if recv == nil {
		return funcID(fn.Pkg().Path(), "", fn.Name()), false, true
	}
	t := recv.Type()
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := types.Unalias(t).(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return "", false, false
	}
	obj := named.Obj()
	return funcID(obj.Pkg().Path(), obj.Name(), fn.Name()), types.IsInterface(named), true
}

// addImports adds imports edges between module packages, and to external
// modules (resolved with one extra go/packages load of the imported paths).
func (s *scanner) addImports(ctx context.Context) error {
	external := map[string][]string{} // import path → importers
	for _, pkg := range s.pkgs {
		for imp := range pkg.Imports {
			if s.isExternal(imp) {
				external[imp] = append(external[imp], pkg.PkgPath)
			} else if s.inModule[imp] {
				s.addEdge(model.ID(pkg.PkgPath), model.ID(imp), model.EdgeImports)
			}
		}
	}
	if len(external) == 0 {
		return nil
	}
	return s.addExternals(ctx, external)
}

// isExternal: not cgo's "C" and not a module package (analyzed or outside
// the include patterns).
func (s *scanner) isExternal(imp string) bool {
	return imp != "C" && !s.inModule[imp] && !s.inAnyModule(imp)
}

func (s *scanner) addExternals(ctx context.Context, external map[string][]string) error {
	mods, err := s.moduleOf(ctx, slices.Sorted(maps.Keys(external)))
	if err != nil {
		return err
	}
	for imp, importers := range external {
		s.addExternal(mods[imp], imp, importers)
	}
	return nil
}

// addExternal adds the node and edges for one external import. mod is the
// module path, or "" for the standard library.
func (s *scanner) addExternal(mod, imp string, importers []string) {
	id, name := mod, externalName(mod)
	switch {
	case mod == "" && !s.cfg.ShowStdlib:
		return
	case mod == "":
		id, name = imp, imp
	case s.cfg.ExternalModules == "hidden":
		return
	}
	if !s.ids[model.ID(id)] {
		s.addNode(model.Node{ID: model.ID(id), Kind: model.KindExternal, Name: name})
	}
	for _, from := range importers {
		s.addEdge(model.ID(from), model.ID(id), model.EdgeImports)
	}
}

// moduleOf maps each import path to its module path ("" for the standard
// library).
func (s *scanner) moduleOf(ctx context.Context, paths []string) (map[string]string, error) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedModule, Dir: s.root, Context: ctx}
	pkgs, err := packages.Load(cfg, paths...)
	if err != nil {
		return nil, fmt.Errorf("find modules of imported packages in %s: %w", s.root, err)
	}
	mods := map[string]string{}
	for _, p := range pkgs {
		if p.Module != nil {
			mods[p.PkgPath] = p.Module.Path
		}
	}
	return mods, nil
}
