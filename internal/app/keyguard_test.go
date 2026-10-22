package app

import (
	"strconv"

	bkey "charm.land/bubbles/v2/key"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
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

// TestNoKeyGlyphLiteralsInRender keeps the key a footer or hint shows in
// keys.go: no Render call is handed a string that is exactly a key of a
// binding (its help key or one of its keys), as "y" or "⏎".
func TestNoKeyGlyphLiteralsInRender(t *testing.T) {
	glyphs := map[string]bool{}
	k, s := defaultKeys(), defaultShellKeys()
	for _, bs := range [][]bkey.Binding{k.global.bindings(), k.main.bindings(), k.form.bindings(), k.peek.bindings(),
		k.sheet.bindings(), k.search.bindings(), k.pair.bindings(), k.takeOver.bindings(), k.confirm.bindings(),
		s.shell.bindings(), s.typing.bindings()} {
		for _, b := range bs {
			glyphs[b.Help().Key] = true
			for _, key := range b.Keys() {
				glyphs[key] = true
			}
		}
	}
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
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "Render" {
				return true
			}
			for _, a := range call.Args {
				if lit, ok := a.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if v, err := strconv.Unquote(lit.Value); err == nil && glyphs[v] {
						t.Errorf("%s: Render(%s) is a key of keys.go: draw it from its binding", fset.Position(lit.Pos()), lit.Value)
					}
				}
			}
			return true
		})
	}
}

// specialKey finds a special key's name or glyph as a whole word.
var specialKey = regexp.MustCompile(`(^|[^\w+])(⏎|esc|Enter|␣|⌫|↑↓|←→|\^o|shift\+tab|ctrl\+\w+)($|[^\w+])`)

// TestNoSpecialKeysInStrings keeps the name of a special key out of any
// string of the console: a footer, a hint or a line of prose takes it from
// its binding (the word tab, and a single letter is held to the Render guard above only).
func TestNoSpecialKeysInStrings(t *testing.T) {
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
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if v, err := strconv.Unquote(lit.Value); err == nil {
				if m := specialKey.FindStringSubmatch(v); m != nil {
					t.Errorf("%s: the string %s names the key %q: draw it from its binding", fset.Position(lit.Pos()), lit.Value, m[2])
				}
			}
			return true
		})
	}
}
