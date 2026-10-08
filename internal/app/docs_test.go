package app

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/docscheck"
)

// docKey turns a key as the docs write it into the name the console's key
// handlers compare.
var docKey = map[string]string{
	"Enter": "enter", "⏎": "enter", "Esc": "esc", "↑": "up", "↓": "down",
	"Tab": "tab", "Shift+Tab": "shift+tab", "Space": "space", "Ctrl+C": "ctrl+c",
	"Backspace": "backspace",
}

// handledKeys reads the console's key handlers from source: each case of
// the functions named key is one group of keys that do the same thing, and
// every key a handler compares with (case or ==) is in all.
func handledKeys(t *testing.T) (groups [][]string, all map[string]bool) {
	t.Helper()
	fset := token.NewFileSet()
	all = map[string]bool{}
	for _, file := range []string{"model.go", "layers.go", "lists.go", "doors.go"} {
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || (fn.Name.Name != "key" && fn.Name.Name != "typing" && fn.Name.Name != "scrollPreview" && fn.Name.Name != "search") {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CaseClause:
					var g []string
					for _, e := range n.List {
						if s, ok := literal(e); ok {
							g = append(g, s)
							all[s] = true
						}
					}
					if fn.Name.Name == "key" && len(g) > 0 {
						groups = append(groups, g)
					}
				case *ast.BinaryExpr:
					if n.Op == token.EQL {
						if s, ok := literal(n.Y); ok {
							all[s] = true
							if fn.Name.Name == "key" && isKeyVar(n.X) {
								groups = append(groups, []string{s})
							}
						}
					}
				}
				return true
			})
		}
	}
	return groups, all
}

func isKeyVar(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && (id.Name == "k" || id.Name == "key")
}

func literal(e ast.Expr) (string, bool) {
	b, ok := e.(*ast.BasicLit)
	if !ok || b.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(b.Value)
	return s, err == nil
}

// TestTheDocsNameEveryKeyAndOnlyRealOnes holds the Key tables of the README
// and the docs to the console: a documented key the console does not
// handle fails, and so does a console key that no Key table names.
func TestTheDocsNameEveryKeyAndOnlyRealOnes(t *testing.T) {
	files, err := docscheck.Files("../..")
	if err != nil {
		t.Fatal(err)
	}
	refs, err := docscheck.Keys(files)
	if err != nil {
		t.Fatal(err)
	}
	groups, handled := handledKeys(t)
	if len(groups) < 10 {
		t.Fatalf("read %d key groups from the handlers: the parse is broken", len(groups))
	}
	tabs := fmt.Sprintf("1-%d", len(tabNames))
	named := map[string]bool{}
	for _, r := range refs {
		k := r.Text
		if v, ok := docKey[k]; ok {
			k = v
		}
		switch {
		case r.Text == tabs:
			named[tabs] = true
		case handled[k]:
			named[k] = true
		default:
			t.Errorf("%s:%d: the docs name the key %q, and the console does not handle it", r.File, r.Line, r.Text)
		}
	}
	if !named[tabs] {
		t.Errorf("no Key table names %q, the tab keys", tabs)
	}
	seen := map[string]bool{}
	for _, g := range groups {
		if seen[strings.Join(g, " ")] {
			continue
		}
		seen[strings.Join(g, " ")] = true
		found := false
		for _, k := range g {
			found = found || named[k]
		}
		if !found {
			t.Errorf("the console handles %s, and no Key table names it", strings.Join(g, " or "))
		}
	}
}
