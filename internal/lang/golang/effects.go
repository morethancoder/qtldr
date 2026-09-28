package golang

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

// effectfulTypePkgs: a parameter or result whose type comes from one of these
// packages makes a function effectful (PLAN.md §6.2 rule 1). log counts only
// for log.Logger.
var effectfulTypePkgs = []string{"context", "io", "os", "net", "net/http", "database/sql"}

// effectfulCallPkgs: calling any function or method of these packages is an
// effect (rule 4). Any package under net/ is included too.
var effectfulCallPkgs = []string{
	"os", "os/exec", "os/signal", "os/user", "io", "io/ioutil", "io/fs",
	"math/rand", "math/rand/v2", "crypto/rand", "log", "log/slog", "log/syslog", "syscall", "net",
}

// effectfulCalls are single effectful functions in otherwise pure packages.
var effectfulCalls = []string{
	"time.Now", "time.Sleep", "time.Since", "time.Until", "time.After", "time.AfterFunc",
	"time.Tick", "time.NewTimer", "time.NewTicker",
	"fmt.Print", "fmt.Printf", "fmt.Println", "fmt.Fprint", "fmt.Fprintf", "fmt.Fprintln",
	"fmt.Scan", "fmt.Scanf", "fmt.Scanln", "fmt.Fscan", "fmt.Fscanf", "fmt.Fscanln",
}

// varFacts says which package-level variables are written anywhere in the
// module, and which are sentinel errors (PLAN.md §6.2 rule 2 exception, see
// docs/decisions.md).
type varFacts struct {
	written  map[*types.Var]bool
	sentinel map[*types.Var]bool // module vars initialized with errors.New / fmt.Errorf
	module   map[string]bool
}

func newVarFacts(pkgs []*packages.Package, module map[string]bool) *varFacts {
	vf := &varFacts{written: map[*types.Var]bool{}, sentinel: map[*types.Var]bool{}, module: module}
	for _, p := range pkgs {
		for _, f := range p.Syntax {
			vf.scanFile(p.TypesInfo, f)
		}
	}
	return vf
}

func (vf *varFacts) scanFile(info *types.Info, f *ast.File) {
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.VAR {
			vf.addSentinels(info, gd)
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		for _, target := range writeTargets(n) {
			if v := packageVar(info, target); v != nil {
				vf.written[v] = true
			}
		}
		return true
	})
}

func (vf *varFacts) addSentinels(info *types.Info, gd *ast.GenDecl) {
	for _, spec := range gd.Specs {
		vs := spec.(*ast.ValueSpec)
		if len(vs.Values) != len(vs.Names) {
			continue
		}
		for i, name := range vs.Names {
			v, ok := info.Defs[name].(*types.Var)
			if ok && isError(v.Type()) && isErrorConstructor(info, vs.Values[i]) {
				vf.sentinel[v] = true
			}
		}
	}
}

// writeTargets returns the expressions a statement assigns to or takes the
// address of.
func writeTargets(n ast.Node) []ast.Expr {
	switch n := n.(type) {
	case *ast.AssignStmt:
		if n.Tok != token.DEFINE {
			return n.Lhs
		}
	case *ast.IncDecStmt:
		return []ast.Expr{n.X}
	case *ast.UnaryExpr:
		if n.Op == token.AND {
			return []ast.Expr{n.X}
		}
	case *ast.RangeStmt:
		if n.Tok == token.ASSIGN {
			return []ast.Expr{n.Key, n.Value}
		}
	}
	return nil
}

// packageVar resolves x (ident or pkg.Ident) to a package-level variable.
func packageVar(info *types.Info, x ast.Expr) *types.Var {
	id := targetIdent(x)
	if id == nil {
		return nil
	}
	v, ok := info.Uses[id].(*types.Var)
	if !ok || !isPackageLevel(v) {
		return nil
	}
	return v
}

func isPackageLevel(v *types.Var) bool {
	return v != nil && !v.IsField() && v.Pkg() != nil && v.Parent() == v.Pkg().Scope()
}

func isError(t types.Type) bool { return types.Identical(t, types.Universe.Lookup("error").Type()) }

func isErrorConstructor(info *types.Info, e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	fn, ok := typeutil.Callee(info, call).(*types.Func)
	return ok && fn.Pkg() != nil && slices.Contains([]string{"errors.New", "fmt.Errorf"}, fn.Pkg().Path()+"."+fn.Name())
}

// isSentinel: an error variable nothing in the module writes, created by
// errors.New or fmt.Errorf (module vars), or any error variable of another
// module or the standard library (io.EOF, sql.ErrNoRows).
func (vf *varFacts) isSentinel(v *types.Var) bool {
	if vf.written[v] || !isError(v.Type()) {
		return false
	}
	return vf.sentinel[v] || !vf.module[v.Pkg().Path()]
}

// localEffects lists why fn is effectful by itself (rules 1–4), deduplicated,
// in source order. Calls to module functions are rule 5 and are handled by
// propagation over calls edges.
func (s *scanner) localEffects(info *types.Info, fn *ast.FuncDecl) []string {
	var reasons []string
	add := func(r string) {
		if r != "" && !slices.Contains(reasons, r) {
			reasons = append(reasons, r)
		}
	}
	if obj, ok := info.Defs[fn.Name].(*types.Func); ok {
		for _, r := range signatureEffects(obj.Signature()) {
			add(r)
		}
	}
	writes := map[*ast.Ident]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		for _, t := range writeTargets(n) {
			if id := targetIdent(t); id != nil {
				writes[id] = true
			}
		}
		add(s.nodeEffect(info, n, writes))
		return true
	})
	return reasons
}

func (s *scanner) nodeEffect(info *types.Info, n ast.Node, writes map[*ast.Ident]bool) string {
	switch n := n.(type) {
	case *ast.CallExpr:
		return callEffect(info, n)
	case *ast.Ident:
		return s.varEffect(info, n, writes)
	}
	return concurrencyEffect(info, n)
}

// concurrencyEffect is rule 3: go statements, channel operations, select.
func concurrencyEffect(info *types.Info, n ast.Node) string {
	switch n.(type) {
	case *ast.GoStmt:
		return "go statement"
	case *ast.SendStmt:
		return "channel send"
	case *ast.SelectStmt:
		return "select"
	}
	if receives(info, n) {
		return "channel receive"
	}
	return ""
}

// receives: <-ch, or range over a channel.
func receives(info *types.Info, n ast.Node) bool {
	switch n := n.(type) {
	case *ast.UnaryExpr:
		return n.Op == token.ARROW
	case *ast.RangeStmt:
		_, ok := info.TypeOf(n.X).Underlying().(*types.Chan)
		return ok
	}
	return false
}

func (s *scanner) varEffect(info *types.Info, id *ast.Ident, writes map[*ast.Ident]bool) string {
	v := packageVar(info, id)
	switch {
	case v == nil:
		return ""
	case writes[id]:
		return "writes var " + varName(v)
	case s.vars.isSentinel(v):
		return ""
	}
	return "reads var " + varName(v)
}

// targetIdent is the identifier an assignment target names: x or pkg.x.
func targetIdent(x ast.Expr) *ast.Ident {
	switch x := x.(type) {
	case *ast.Ident:
		return x
	case *ast.SelectorExpr:
		return x.Sel
	}
	return nil
}

func varName(v *types.Var) string {
	return v.Pkg().Name() + "." + v.Name()
}

func callEffect(info *types.Info, call *ast.CallExpr) string {
	fn, ok := typeutil.Callee(info, call).(*types.Func)
	if !ok || fn.Pkg() == nil {
		return ""
	}
	pkg := fn.Pkg().Path()
	if slices.Contains(effectfulCallPkgs, pkg) || strings.HasPrefix(pkg, "net/") ||
		(fn.Signature().Recv() == nil && slices.Contains(effectfulCalls, pkg+"."+fn.Name())) {
		return "calls " + fn.FullName()
	}
	return ""
}

// signatureEffects applies rule 1 to parameters and results.
func signatureEffects(sig *types.Signature) []string {
	var out []string
	for _, tuple := range []*types.Tuple{sig.Params(), sig.Results()} {
		for i := range tuple.Len() {
			if r := typeEffect(tuple.At(i).Type(), 0); r != "" {
				out = append(out, "uses "+r)
			}
		}
	}
	return out
}

// typeEffect returns a description if t is or contains an effectful type.
// Named types are checked by package and not expanded.
func typeEffect(t types.Type, depth int) string {
	switch t := types.Unalias(t).(type) {
	case *types.Chan:
		return "a channel"
	case *types.Named:
		return namedEffect(t)
	}
	if depth > 8 {
		return ""
	}
	for _, inner := range innerTypes(t) {
		if r := typeEffect(inner, depth+1); r != "" {
			return r
		}
	}
	return ""
}

// innerTypes lists the types an unnamed composite type is built from.
func innerTypes(t types.Type) []types.Type {
	switch t := t.(type) {
	case *types.Pointer:
		return []types.Type{t.Elem()}
	case *types.Slice:
		return []types.Type{t.Elem()}
	case *types.Array:
		return []types.Type{t.Elem()}
	case *types.Map:
		return []types.Type{t.Key(), t.Elem()}
	case *types.Signature:
		return append(tupleTypes(t.Params()), tupleTypes(t.Results())...)
	}
	return nil
}

func tupleTypes(tu *types.Tuple) []types.Type {
	out := make([]types.Type, tu.Len())
	for i := range out {
		out[i] = tu.At(i).Type()
	}
	return out
}

func namedEffect(t *types.Named) string {
	obj := t.Obj()
	if obj.Pkg() == nil {
		return ""
	}
	path := obj.Pkg().Path()
	if slices.Contains(effectfulTypePkgs, path) || (path == "log" && obj.Name() == "Logger") {
		return path + "." + obj.Name()
	}
	return ""
}
