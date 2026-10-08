package app

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/docscheck"
)

// docKey turns a key as the docs write it into the name the console's key
// handlers compare. Words match in any case.
var docKey = map[string]string{
	"enter": "enter", "⏎": "enter", "esc": "esc", "↑": "up", "↓": "down",
	"tab": "tab", "shift+tab": "shift+tab", "space": "space", "ctrl+c": "ctrl+c",
	"backspace": "backspace",
}

func keyName(doc string) string {
	if k, ok := docKey[strings.ToLower(doc)]; ok && len(doc) > 1 {
		return k
	}
	if k, ok := docKey[doc]; ok {
		return k
	}
	return doc
}

// keyLayer is the keys of one screen: groups are the cases of its key
// function (keys that do the same thing), all is every key its handlers
// compare with (case or ==).
type keyLayer struct {
	groups [][]string
	all    map[string]bool
}

// handledKeys reads the key handlers from source, by receiver: Model is the
// console, Shell is Hosts.
func handledKeys(t *testing.T) map[string]*keyLayer {
	t.Helper()
	fset := token.NewFileSet()
	layers := map[string]*keyLayer{"Model": {all: map[string]bool{}}, "Shell": {all: map[string]bool{}}}
	for _, file := range []string{"model.go", "layers.go", "lists.go", "doors.go"} {
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !slices.Contains([]string{"key", "typing", "scrollPreview", "search"}, fn.Name.Name) {
				continue
			}
			l := layers[recvName(fn)]
			if l == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CaseClause:
					var g []string
					for _, e := range n.List {
						if s, ok := literal(e); ok {
							g = append(g, s)
							l.all[s] = true
						}
					}
					if fn.Name.Name == "key" && len(g) > 0 {
						l.groups = append(l.groups, g)
					}
				case *ast.BinaryExpr:
					if n.Op == token.EQL {
						if s, ok := literal(n.Y); ok {
							l.all[s] = true
							if fn.Name.Name == "key" && isKeyVar(n.X) {
								l.groups = append(l.groups, []string{s})
							}
						}
					}
				}
				return true
			})
		}
	}
	return layers
}

func recvName(fn *ast.FuncDecl) string {
	if id, ok := fn.Recv.List[0].Type.(*ast.Ident); ok {
		return id.Name
	}
	return ""
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

// TestTheDocsNameEveryKeyAndOnlyRealOnes holds the keys of the README and
// the docs to the console: a key the console does not handle fails, in a
// Key table or in prose. A Key table under a Hosts heading is held to Hosts,
// any other to the console. And every key the console or Hosts handles is
// in a Key table of its own screen.
func TestTheDocsNameEveryKeyAndOnlyRealOnes(t *testing.T) {
	files, err := docscheck.Files("../..")
	if err != nil {
		t.Fatal(err)
	}
	refs, err := docscheck.Keys(files)
	if err != nil {
		t.Fatal(err)
	}
	layers := handledKeys(t)
	if len(layers["Model"].groups) < 10 || len(layers["Shell"].groups) < 4 {
		t.Fatalf("read %d console and %d Hosts key groups: the parse is broken", len(layers["Model"].groups), len(layers["Shell"].groups))
	}
	tabs := fmt.Sprintf("1-%d", len(tabNames))
	named := map[string]map[string]bool{"Model": {}, "Shell": {}}
	for _, r := range refs {
		k := keyName(r.Text)
		screens := []string{"Model", "Shell"}
		if r.Table {
			screens = []string{"Model"}
			if strings.Contains(r.Section, "Hosts") {
				screens = []string{"Shell"}
			}
		}
		ok := false
		for _, sc := range screens {
			if (sc == "Model" && r.Text == tabs) || layers[sc].all[k] {
				ok = true
				if r.Table {
					named[sc][k] = true
				}
			}
		}
		if !ok {
			t.Errorf("%s:%d (%s): the docs name the key %q, and %s does not handle it", r.File, r.Line, r.Section, r.Text, strings.Join(screens, " or "))
		}
	}
	if !named["Model"][tabs] {
		t.Errorf("no Key table names %q, the tab keys", tabs)
	}
	for sc, l := range layers {
		seen := map[string]bool{}
		for _, g := range l.groups {
			if seen[strings.Join(g, " ")] {
				continue
			}
			seen[strings.Join(g, " ")] = true
			found := false
			for _, k := range g {
				found = found || named[sc][k]
			}
			if !found {
				t.Errorf("%s handles %s, and no Key table of its screen names it", sc, strings.Join(g, " or "))
			}
		}
	}
}
