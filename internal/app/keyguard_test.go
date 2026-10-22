package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// isKeyExpr is a key as a handler holds it: k, key, or k.String().
func isKeyExpr(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name == "k" || e.Name == "key"
	case *ast.CallExpr:
		sel, ok := e.Fun.(*ast.SelectorExpr)
		if !ok || len(e.Args) != 0 || sel.Sel.Name != "String" {
			return false
		}
		id, ok := sel.X.(*ast.Ident)
		return ok && id.Name == "k"
	}
	return false
}

func isStringLit(e ast.Expr) bool {
	b, ok := e.(*ast.BasicLit)
	return ok && b.Kind == token.STRING
}

// TestNoKeyStringLiterals keeps the keys in keys.go: a handler matches a
// binding, and never compares a key with a string of its own.
func TestNoKeyStringLiterals(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "keys.go" {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.SwitchStmt:
				if n.Tag == nil || !isKeyExpr(n.Tag) {
					return true
				}
				for _, c := range n.Body.List {
					for _, x := range c.(*ast.CaseClause).List {
						if isStringLit(x) {
							t.Errorf("%s: case %s on a key: match a binding of keys.go", fset.Position(x.Pos()), x.(*ast.BasicLit).Value)
						}
					}
				}
			case *ast.BinaryExpr:
				if n.Op != token.EQL && n.Op != token.NEQ {
					return true
				}
				if isKeyExpr(n.X) && isStringLit(n.Y) || isKeyExpr(n.Y) && isStringLit(n.X) {
					t.Errorf("%s: a key compared with a string: match a binding of keys.go", fset.Position(n.Pos()))
				}
			}
			return true
		})
	}
}
