package golang

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func parseFunc(t *testing.T, src string) (*token.FileSet, *ast.FuncDecl) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", "package x\n"+src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok {
			return fset, fn
		}
	}
	t.Fatal("no func in source")
	return nil, nil
}

func TestCyclomatic(t *testing.T) {
	cases := []struct {
		name, src string
		want      int
	}{
		{"empty", "func f() {}", 1},
		{"if else", "func f(a bool) { if a { } else { } }", 2},
		{"else if", "func f(a, b bool) { if a { } else if b { } }", 3},
		{"for and range", "func f(s []int) { for i := 0; i < 1; i++ {}; for range s {} }", 3},
		{"and or", "func f(a, b, c bool) bool { return a && b || c }", 3},
		{"switch counts default", "func f(x int) { switch x { case 1: case 2, 3: default: } }", 4},
		{"type switch", "func f(x any) { switch x.(type) { case int: case string: } }", 3},
		{"select counts default", "func f(c chan int) { select { case <-c: default: } }", 3},
		{"closure counts in enclosing", "func f() { g := func(a bool) { if a {} }; g(true) }", 2},
		{"method", "func (T) f(a bool) { if a {} }", 2},
		{"no body", "func f()", 1},
	}
	for _, c := range cases {
		_, fn := parseFunc(t, c.src)
		if got := Cyclomatic(fn); got != c.want {
			t.Errorf("%s: CC = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestBodyHash(t *testing.T) {
	base := "func f(a int) int {\n\tif a > 0 {\n\t\treturn a\n\t}\n\treturn -a\n}"
	same := []struct{ name, src string }{
		{"doc comment", "// f is f.\n" + base},
		{"inner comment", "func f(a int) int {\n\t// positive\n\tif a > 0 {\n\t\treturn a // yes\n\t}\n\treturn -a\n}"},
		{"blank line", "func f(a int) int {\n\n\tif a > 0 {\n\t\treturn a\n\t}\n\n\treturn -a\n}"},
		{"spacing", "func f(a int) int {\n\tif a>0 {\n\t\treturn a\n\t}\n\treturn -a\n}"},
	}
	different := []struct{ name, src string }{
		{"operator", "func f(a int) int {\n\tif a >= 0 {\n\t\treturn a\n\t}\n\treturn -a\n}"},
		{"signature", "func f(a int64) int64 {\n\tif a > 0 {\n\t\treturn a\n\t}\n\treturn -a\n}"},
	}
	want := hash(t, base)
	for _, c := range same {
		if got := hash(t, c.src); got != want {
			t.Errorf("%s: hash changed", c.name)
		}
	}
	for _, c := range different {
		if got := hash(t, c.src); got == want {
			t.Errorf("%s: hash did not change", c.name)
		}
	}
}

func hash(t *testing.T, src string) string {
	t.Helper()
	fset, fn := parseFunc(t, src)
	h, err := BodyHash(fset, fn)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestSignature(t *testing.T) {
	fset, fn := parseFunc(t, "// doc\nfunc (s *S) Get(k string) (int, error) { return 0, nil }")
	got, err := Signature(fset, fn)
	if err != nil || got != "func (s *S) Get(k string) (int, error)" {
		t.Fatalf("got %q %v", got, err)
	}
}
