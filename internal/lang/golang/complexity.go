package golang

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/printer"
	"go/token"
	"strings"

	"github.com/uudashr/gocognit"
)

// Cyclomatic returns the cyclomatic complexity of fn, matching crap4go: 1 plus
// one for each if, for, range, case clause (including default), select comm
// clause (including default), && and ||. Function literals count toward fn.
func Cyclomatic(fn *ast.FuncDecl) int {
	cc := 1
	if fn.Body == nil {
		return cc
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		cc += decisionPoints(n)
		return true
	})
	return cc
}

func decisionPoints(n ast.Node) int {
	switch n := n.(type) {
	case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
		return 1
	case *ast.BinaryExpr:
		if n.Op == token.LAND || n.Op == token.LOR {
			return 1
		}
	}
	return 0
}

// Cognitive returns the cognitive complexity of fn as computed by gocognit.
func Cognitive(fn *ast.FuncDecl) int { return gocognit.Complexity(fn) }

// BodyHash is the SHA-256 of fn printed without comments and blank lines, so
// edits to comments, doc comments or blank lines keep the same hash.
func BodyHash(fset *token.FileSet, fn *ast.FuncDecl) (string, error) {
	decl := *fn
	decl.Doc = nil
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, &decl); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(dropBlankLines(buf.String())))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// Signature prints fn's declaration line without its body or doc comment.
func Signature(fset *token.FileSet, fn *ast.FuncDecl) (string, error) {
	decl := *fn
	decl.Doc = nil
	decl.Body = nil
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, &decl); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func dropBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	kept := lines[:0]
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			kept = append(kept, l)
		}
	}
	return strings.Join(kept, "\n")
}
