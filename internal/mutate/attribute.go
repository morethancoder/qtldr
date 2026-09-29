package mutate

import (
	"go/scanner"
	"go/token"
	"sort"

	"github.com/morethancoder/qtldr/internal/metrics"
	"github.com/morethancoder/qtldr/internal/model"
)

// Attribute assigns each mutant to the function of pkg whose [Line, EndLine]
// contains it and describes it from the source. Every function of the
// package gets an entry; one with no mutants has no mutation sites. Mutants
// outside functions (package-level initializers) are dropped, as are NOT
// VIABLE, SKIPPED and RUNNABLE ones (PLAN.md §7.3).
func Attribute(g model.Graph, pkg model.ID, mutants []FileMutant, sources map[string][]byte) map[model.ID]model.Mutation {
	byFunc := map[model.ID][]model.Mutant{}
	funcs := map[string][]model.Node{}
	for _, n := range g.Nodes {
		if n.Kind == model.KindFunc && n.Parent == pkg {
			byFunc[n.ID] = nil
			funcs[n.File] = append(funcs[n.File], n)
		}
	}
	for _, m := range mutants {
		if fn, ok := owner(funcs, m); ok {
			byFunc[fn] = append(byFunc[fn], model.Mutant{
				Line: m.Line, Col: m.Col, Type: m.Type, Status: m.Status, Description: Describe(m.Type, sources[m.File], m.Line, m.Col),
			})
		}
	}
	out := map[model.ID]model.Mutation{}
	for id, ms := range byFunc {
		out[id] = Tally(ms)
	}
	return out
}

// owner is the function a counted mutant belongs to.
func owner(funcs map[string][]model.Node, m FileMutant) (model.ID, bool) {
	if !counted(m.Status) {
		return "", false
	}
	return enclosing(funcs[m.File], m.Line)
}

func counted(status string) bool {
	return status == Killed || status == Lived || status == NotCovered || status == TimedOut
}

func enclosing(fns []model.Node, line int) (model.ID, bool) {
	for _, f := range fns {
		if line >= f.Line && line <= f.EndLine {
			return f.ID, true
		}
	}
	return "", false
}

// Tally counts mutants by status and computes the score (metrics.Score;
// timed-out mutants count as caught). Mutants are sorted by line and column.
func Tally(ms []model.Mutant) model.Mutation {
	mu := model.Mutation{Mutants: ms}
	sort.SliceStable(mu.Mutants, func(i, j int) bool { return before(mu.Mutants[i], mu.Mutants[j]) })
	counts := map[string]*int{Killed: &mu.Killed, Lived: &mu.Survived, NotCovered: &mu.NotCovered, TimedOut: &mu.TimedOut}
	for _, m := range ms {
		if c, ok := counts[m.Status]; ok {
			*c++
		}
	}
	mu.Score = metrics.Score(mu.Killed, mu.TimedOut, mu.Survived)
	return mu
}

func before(a, b model.Mutant) bool {
	return a.Line < b.Line || (a.Line == b.Line && a.Col < b.Col)
}

// replacements mirrors Gremlins' tokenMutations (internal/engine/mappings.go,
// v0.6.0): the token each mutation type turns another into.
var replacements = map[string]map[token.Token]token.Token{
	"ARITHMETIC_BASE":       {token.ADD: token.SUB, token.MUL: token.QUO, token.QUO: token.MUL, token.REM: token.MUL, token.SUB: token.ADD},
	"CONDITIONALS_BOUNDARY": {token.GEQ: token.GTR, token.GTR: token.GEQ, token.LEQ: token.LSS, token.LSS: token.LEQ},
	"CONDITIONALS_NEGATION": {token.EQL: token.NEQ, token.GEQ: token.LSS, token.GTR: token.LEQ, token.LEQ: token.GTR, token.LSS: token.GEQ, token.NEQ: token.EQL},
	"INCREMENT_DECREMENT":   {token.DEC: token.INC, token.INC: token.DEC},
	"INVERT_ASSIGNMENTS":    {token.ADD_ASSIGN: token.SUB_ASSIGN, token.MUL_ASSIGN: token.QUO_ASSIGN, token.QUO_ASSIGN: token.MUL_ASSIGN, token.REM_ASSIGN: token.REM_ASSIGN, token.SUB_ASSIGN: token.ADD_ASSIGN},
	"INVERT_BITWISE":        {token.AND: token.OR, token.OR: token.AND, token.XOR: token.AND, token.AND_NOT: token.AND, token.SHL: token.SHR, token.SHR: token.SHL},
	"INVERT_BWASSIGN": {token.AND_ASSIGN: token.OR_ASSIGN, token.OR_ASSIGN: token.AND_ASSIGN, token.XOR_ASSIGN: token.AND_ASSIGN,
		token.AND_NOT_ASSIGN: token.AND_ASSIGN, token.SHL_ASSIGN: token.SHR_ASSIGN, token.SHR_ASSIGN: token.SHL_ASSIGN},
	"INVERT_LOGICAL":   {token.LAND: token.LOR, token.LOR: token.LAND},
	"INVERT_LOOPCTRL":  {token.BREAK: token.CONTINUE, token.CONTINUE: token.BREAK},
	"INVERT_NEGATIVES": {token.SUB: token.ADD},
	"REMOVE_SELF_ASSIGNMENTS": {token.ADD_ASSIGN: token.ASSIGN, token.AND_ASSIGN: token.ASSIGN, token.AND_NOT_ASSIGN: token.ASSIGN,
		token.MUL_ASSIGN: token.ASSIGN, token.OR_ASSIGN: token.ASSIGN, token.QUO_ASSIGN: token.ASSIGN, token.REM_ASSIGN: token.ASSIGN,
		token.SHL_ASSIGN: token.ASSIGN, token.SHR_ASSIGN: token.ASSIGN, token.SUB_ASSIGN: token.ASSIGN, token.XOR_ASSIGN: token.ASSIGN},
}

// Describe returns "orig → repl" for the token at line:col, or the raw type
// name when the type or token is unknown.
func Describe(mutationType string, src []byte, line, col int) string {
	table, ok := replacements[mutationType]
	if !ok || src == nil {
		return mutationType
	}
	tok := tokenAt(src, line, col)
	repl, ok := table[tok]
	if !ok {
		return mutationType
	}
	return tok.String() + " → " + repl.String()
}

func tokenAt(src []byte, line, col int) token.Token {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	var s scanner.Scanner
	s.Init(file, src, nil, 0)
	for {
		pos, tok, _ := s.Scan()
		if tok == token.EOF {
			return token.ILLEGAL
		}
		p := fset.Position(pos)
		if p.Line == line && p.Column == col {
			return tok
		}
		if p.Line > line {
			return token.ILLEGAL
		}
	}
}
