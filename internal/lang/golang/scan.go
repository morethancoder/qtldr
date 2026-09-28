package golang

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/model"
)

// scanner accumulates the graph while walking packages.
type scanner struct {
	root     string
	cfg      config.Project
	module   string
	pkgs     []*packages.Package
	inModule map[string]bool
	nodes    []model.Node
	ids      map[model.ID]bool
	edges    map[model.Edge]bool
	metrics  map[model.ID]model.Metrics
}

func newScanner(root string, cfg config.Project, all []*packages.Package) (*scanner, error) {
	pkgs, mod, err := mainModule(all)
	if err != nil {
		return nil, err
	}
	s := &scanner{
		root: mod.Dir, cfg: cfg, module: mod.Path, pkgs: pkgs,
		inModule: map[string]bool{},
		ids:      map[model.ID]bool{}, edges: map[model.Edge]bool{}, metrics: map[model.ID]model.Metrics{},
	}
	for _, p := range pkgs {
		s.inModule[p.PkgPath] = true
	}
	s.addNode(model.Node{ID: model.ID(mod.Path), Kind: model.KindModule, Name: shortModuleName(mod.Path)})
	return s, nil
}

func (s *scanner) addNode(n model.Node) {
	s.nodes = append(s.nodes, n)
	s.ids[n.ID] = true
}

func (s *scanner) addEdge(from, to model.ID, kind model.EdgeKind) {
	s.edges[model.Edge{From: from, To: to, Kind: kind}] = true
}

func (s *scanner) addPackage(pkg *packages.Package) {
	rel := strings.TrimPrefix(strings.TrimPrefix(pkg.PkgPath, s.module), "/")
	name := rel
	if rel == "" {
		rel, name = ".", shortModuleName(s.module)
	}
	s.addNode(model.Node{
		ID: model.ID(pkg.PkgPath), Kind: model.KindPackage, Name: name,
		Parent: model.ID(s.module), Dir: rel, Errors: packageErrors(pkg),
	})
	for _, f := range pkg.Syntax {
		file, ok := s.relFile(pkg.Fset, f)
		if !ok {
			continue
		}
		for _, d := range f.Decls {
			s.addDecl(pkg, file, d)
		}
	}
}

// relFile returns f's path relative to the module root, or false if the file
// is generated, excluded, or outside the module.
func (s *scanner) relFile(fset *token.FileSet, f *ast.File) (string, bool) {
	abs := fset.Position(f.Package).Filename
	rel, err := filepath.Rel(s.root, abs)
	if err != nil || strings.HasPrefix(rel, "..") || ast.IsGenerated(f) {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	return rel, !excluded(s.cfg.Exclude, rel)
}

func (s *scanner) addDecl(pkg *packages.Package, file string, d ast.Decl) {
	switch d := d.(type) {
	case *ast.FuncDecl:
		s.addFunc(pkg, file, d)
	case *ast.GenDecl:
		if d.Tok != token.TYPE {
			return
		}
		for _, spec := range d.Specs {
			s.addType(pkg, file, spec.(*ast.TypeSpec))
		}
	}
}

func (s *scanner) addFunc(pkg *packages.Package, file string, fn *ast.FuncDecl) {
	if fn.Body == nil {
		return
	}
	start, end := pkg.Fset.Position(fn.Pos()).Line, pkg.Fset.Position(fn.End()).Line
	recv, ptr := receiver(fn)
	id := s.uniqueID(funcID(pkg.PkgPath, recv, fn.Name.Name), file, start)
	sig, _ := Signature(pkg.Fset, fn)
	hash, _ := BodyHash(pkg.Fset, fn)
	s.addNode(model.Node{
		ID: id, Kind: model.KindFunc, Name: qualifiedName(recv, fn.Name.Name), Parent: model.ID(pkg.PkgPath),
		File: file, Line: start, EndLine: end, Exported: model.Ptr(fn.Name.IsExported()),
		Recv: recv, PtrRecv: ptr, Signature: sig, BodyHash: hash,
	})
	s.metrics[id] = model.Metrics{
		CC: model.Ptr(Cyclomatic(fn)), Cognitive: model.Ptr(Cognitive(fn)), LOC: model.Ptr(end - start + 1),
	}
	s.addCalls(pkg.TypesInfo, id, fn.Body)
}

// uniqueID adds a "#file" suffix (then "#file:line") when id is taken, e.g. by
// several init functions.
func (s *scanner) uniqueID(id model.ID, file string, line int) model.ID {
	if !s.ids[id] {
		return id
	}
	withFile := model.ID(fmt.Sprintf("%s#%s", id, path.Base(file)))
	if !s.ids[withFile] {
		return withFile
	}
	return model.ID(fmt.Sprintf("%s:%d", withFile, line))
}

func (s *scanner) addType(pkg *packages.Package, file string, ts *ast.TypeSpec) {
	n := model.Node{
		ID: model.ID(pkg.PkgPath + "." + ts.Name.Name), Kind: model.KindType, TypeKind: typeKind(ts),
		Name: ts.Name.Name, Parent: model.ID(pkg.PkgPath), File: file, Line: pkg.Fset.Position(ts.Pos()).Line,
	}
	if obj := pkg.TypesInfo.Defs[ts.Name]; obj != nil && n.TypeKind == "struct" {
		n.Fields = structFields(obj.Type(), pkg.Types)
	}
	s.addNode(n)
}

func typeKind(ts *ast.TypeSpec) string {
	switch ts.Type.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	}
	return "other"
}

func structFields(t types.Type, pkg *types.Package) []model.Field {
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	qual := func(p *types.Package) string {
		if p == pkg {
			return ""
		}
		return p.Name()
	}
	fields := make([]model.Field, st.NumFields())
	for i := range fields {
		f := st.Field(i)
		fields[i] = model.Field{Name: f.Name(), Type: types.TypeString(f.Type(), qual)}
	}
	return fields
}

func packageErrors(pkg *packages.Package) []string {
	var msgs []string
	for _, e := range pkg.Errors {
		msgs = append(msgs, e.Error())
	}
	return msgs
}

// graph returns the collected graph with dangling edges removed, sorted.
func (s *scanner) graph() model.Graph {
	g := model.Graph{Nodes: s.nodes, Edges: []model.Edge{}, Metrics: s.metrics}
	for e := range s.edges {
		if s.ids[e.From] && s.targetExists(e) {
			g.Edges = append(g.Edges, e)
		}
	}
	g.Sort()
	return g
}

// targetExists: a calls_dynamic edge points at an interface method, which is
// not a node; it is kept when the interface type is.
func (s *scanner) targetExists(e model.Edge) bool {
	if e.Kind != model.EdgeCallsDynamic {
		return s.ids[e.To]
	}
	i := strings.LastIndex(string(e.To), ".")
	return i > 0 && s.ids[e.To[:i]]
}

// shortModuleName is the display name of a module path: the last element
// without a /vN suffix ("github.com/acme/ledger" → "ledger").
func shortModuleName(mod string) string {
	return path.Base(trimMajor(mod))
}

// externalName is the display name of an external module: the path without
// its host and /vN suffix ("github.com/go-chi/chi/v5" → "go-chi/chi").
func externalName(mod string) string {
	mod = trimMajor(mod)
	first, rest, ok := strings.Cut(mod, "/")
	if ok && strings.Contains(first, ".") {
		return rest
	}
	return mod
}

func trimMajor(mod string) string {
	dir, last := path.Split(mod)
	if dir != "" && len(last) > 1 && last[0] == 'v' && strings.Trim(last[1:], "0123456789") == "" {
		return strings.TrimSuffix(dir, "/")
	}
	return mod
}
