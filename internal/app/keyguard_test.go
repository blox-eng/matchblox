package app

import (
	"fmt"
	"sort"
	"strconv"

	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"

	bkey "charm.land/bubbles/v2/key"
)

// isKeyType is tea.KeyPressMsg (or tea.KeyMsg): the type of a key as a
// handler holds it, whatever the variable is called.
func isKeyType(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && (sel.Sel.Name == "KeyPressMsg" || sel.Sel.Name == "KeyMsg")
}

// isKeyExpr is a key of the names given, or what it reads: x, x.String(),
// x.Keystroke(), x.Text, x.Code.
func isKeyExpr(e ast.Expr, names map[string]bool) bool {
	switch e := e.(type) {
	case *ast.ParenExpr:
		return isKeyExpr(e.X, names)
	case *ast.Ident:
		return names[e.Name]
	case *ast.SelectorExpr:
		id, ok := e.X.(*ast.Ident)
		return ok && names[id.Name] && (e.Sel.Name == "Text" || e.Sel.Name == "Code")
	case *ast.CallExpr:
		sel, ok := e.Fun.(*ast.SelectorExpr)
		if !ok || len(e.Args) != 0 || (sel.Sel.Name != "String" && sel.Sel.Name != "Keystroke") {
			return false
		}
		id, ok := sel.X.(*ast.Ident)
		return ok && names[id.Name]
	}
	return false
}

// isKeyLit is a string or a char literal that names a key: "" is not one (it
// asks whether a key prints text).
func isKeyLit(e ast.Expr) bool {
	b, ok := e.(*ast.BasicLit)
	return ok && (b.Kind == token.STRING && b.Value != `""` && b.Value != "``" || b.Kind == token.CHAR)
}

// keyLiteralSites lists where a file compares a key with a string or a char of
// its own. Keys are found by type: a parameter of type tea.KeyPressMsg, and
// the variable a type switch binds in a case tea.KeyPressMsg.
func keyLiteralSites(fset *token.FileSet, f *ast.File) []string {
	seen := map[token.Pos]string{}
	scan := func(body ast.Node, names map[string]bool) {
		if len(names) == 0 || body == nil {
			return
		}
		ast.Inspect(body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.SwitchStmt:
				tag := n.Tag
				if as, ok := n.Init.(*ast.AssignStmt); ok && len(as.Lhs) == 1 && len(as.Rhs) == 1 {
					if id, ok := tag.(*ast.Ident); ok {
						if l, ok := as.Lhs[0].(*ast.Ident); ok && l.Name == id.Name && isKeyExpr(as.Rhs[0], names) {
							tag = as.Rhs[0]
						}
					}
				}
				if tag == nil || !isKeyExpr(tag, names) {
					return true
				}
				for _, c := range n.Body.List {
					for _, x := range c.(*ast.CaseClause).List {
						if isKeyLit(x) {
							seen[x.Pos()] = fmt.Sprintf("%s: case %s on a key: match a binding of keys.go", fset.Position(x.Pos()), x.(*ast.BasicLit).Value)
						}
					}
				}
			case *ast.BinaryExpr:
				if n.Op != token.EQL && n.Op != token.NEQ {
					return true
				}
				if isKeyExpr(n.X, names) && isKeyLit(n.Y) || isKeyExpr(n.Y, names) && isKeyLit(n.X) {
					seen[n.Pos()] = fmt.Sprintf("%s: a key compared with a literal: match a binding of keys.go", fset.Position(n.Pos()))
				}
			}
			return true
		})
	}
	fromFields := func(fl *ast.FieldList) map[string]bool {
		names := map[string]bool{}
		if fl == nil {
			return names
		}
		for _, p := range fl.List {
			if isKeyType(p.Type) {
				for _, id := range p.Names {
					names[id.Name] = true
				}
			}
		}
		return names
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			if n.Body != nil {
				scan(n.Body, fromFields(n.Type.Params))
			}
		case *ast.FuncLit:
			scan(n.Body, fromFields(n.Type.Params))
		case *ast.TypeSwitchStmt:
			as, ok := n.Assign.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != 1 {
				return true
			}
			id, ok := as.Lhs[0].(*ast.Ident)
			if !ok {
				return true
			}
			for _, c := range n.Body.List {
				cc := c.(*ast.CaseClause)
				if len(cc.List) == 1 && isKeyType(cc.List[0]) {
					for _, st := range cc.Body {
						scan(st, map[string]bool{id.Name: true})
					}
				}
			}
		}
		return true
	})
	var out []string
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// TestNoKeyStringLiterals keeps the keys in keys.go: a handler matches a
// binding, and never compares a key with a string or a char of its own.
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
		for _, site := range keyLiteralSites(fset, f) {
			t.Error(site)
		}
	}
}

// TestNoKeyStringLiteralsCatchesEveryForm holds the guard to every form of a
// key compared with a literal, and to none that is not a key.
func TestNoKeyStringLiteralsCatchesEveryForm(t *testing.T) {
	const src = `package p

import tea "charm.land/bubbletea/v2"

type other struct{}

func (other) String() string { return "" }

func a(k tea.KeyPressMsg) bool { return k.String() == "y" } // line 9

func b(m tea.Msg) bool {
	switch msg := m.(type) {
	case tea.KeyPressMsg:
		return msg.String() == "y" // line 14
	}
	return false
}

func c(k tea.KeyPressMsg) bool { return k.Text == "y" } // line 19

func d(k tea.KeyPressMsg) bool { return k.Code == 'y' } // line 21

func e(k tea.KeyPressMsg) {
	switch s := k.String(); s {
	case "n": // line 25
	}
}

func f(a other) bool { return a.String() == "x" }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := keyLiteralSites(fset, file)
	var lines []string
	for _, g := range got {
		lines = append(lines, strings.SplitN(g, ":", 3)[1])
	}
	sort.Strings(lines)
	if want := []string{"14", "19", "21", "25", "9"}; strings.Join(lines, ",") != strings.Join(want, ",") {
		t.Errorf("sites on lines %v, want %v:\n%s", lines, want, strings.Join(got, "\n"))
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

// letterVerb finds a single letter as a whole word, then a verb: "f starts it".
var letterVerb = regexp.MustCompile(`(^|[^\w%])[a-zA-Z] (starts|stops|gives|ends|again|turns|shows|hides|rescans|tries|copies|on)($|[^\w])`)

// letterSetOff finds a literal that is a single letter set off by spaces, as "box+" m "+label": a key glued to the text after it.
var letterSetOff = regexp.MustCompile(`^\s[a-zA-Z]\s$`)

// TestNoKeyLettersInProse keeps a plain letter key out of the prose of the
// console: "f starts it" takes its f from the binding, as the special keys do.
func TestNoKeyLettersInProse(t *testing.T) {
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
			v, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if m := letterVerb.FindString(v); m != "" {
				t.Errorf("%s: the string %s has a letter key in its prose (%q): draw it from its binding", fset.Position(lit.Pos()), lit.Value, strings.TrimSpace(m))
			} else if letterSetOff.MatchString(v) {
				t.Errorf("%s: the string %s ends in a letter key set off by spaces: draw it from its binding", fset.Position(lit.Pos()), lit.Value)
			}
			return true
		})
	}
}

// TestKeyPressRoundTrips holds keyPress to the keys of the bindings: the tap
// of a binding's first key matches the binding, and a key that prints no
// text carries none.
func TestKeyPressRoundTrips(t *testing.T) {
	k, s := defaultKeys(), defaultShellKeys()
	for _, bs := range [][]bkey.Binding{k.global.bindings(), k.main.bindings(), k.form.bindings(), k.peek.bindings(),
		k.sheet.bindings(), k.search.bindings(), k.pair.bindings(), k.takeOver.bindings(), k.confirm.bindings(),
		s.shell.bindings(), s.typing.bindings()} {
		for _, b := range bs {
			for _, key := range b.Keys() {
				msg := keyPress(key)
				if !bkey.Matches(msg, b) {
					t.Errorf("keyPress(%q) does not match its binding %q", key, b.Help().Key)
				}
				if r := []rune(key); len(r) > 1 && key != "space" && msg.Text != "" {
					t.Errorf("keyPress(%q) carries the text %q: it prints none", key, msg.Text)
				}
			}
		}
	}
}
