package tmuxtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// noSocket marks a test's tmux that names no server on purpose.
const noSocket = "no socket on purpose:"

// TestEveryTestTmuxNamesItsServer: a tmux with no socket goes to the server
// that $TMUX names, or else to the default one, and a builder runs the tests
// inside tmux. A test cleanup with no socket once could end the builder's
// server with every agent in it (#215). So each tmux a test starts names its
// server (-L or -S), or only asks the version (-V). A call that must run tmux
// with no socket says so with noSocket and its reason. Each package whose
// tests start tmux also has a TestMain with testhome.Run, which keeps every
// tmux of the package and its subprocesses off the builder's servers.
// Production code is not test code: it runs tmux on the builder's server on
// purpose.
func TestEveryTestTmuxNamesItsServer(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	runs := map[string]string{} // package directory → its first test file that starts tmux
	cleared := map[string]bool{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != root {
				return filepath.SkipDir // .git, .claude worktrees
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		test := strings.HasSuffix(path, "_test.go")
		helper := strings.HasSuffix(path, ".go") && filepath.Dir(rel) == filepath.Join("internal", "tmuxtest") // tmuxtest is test code
		if !test && !helper {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		if test && clearsTmux(f) {
			cleared[filepath.Dir(rel)] = true
		}
		for _, bad := range unnamedTmux(fset, f) {
			t.Errorf("%s:%d: the tmux names no server: pass -L or -S, or it reaches the builder's server", rel, bad)
		}
		if _, seen := runs[filepath.Dir(rel)]; test && !seen && len(tmuxCalls(f)) > 0 {
			runs[filepath.Dir(rel)] = rel
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) == 0 {
		t.Fatal("no test starts tmux: the scan reads the wrong folder")
	}
	for dir, file := range runs {
		if !cleared[dir] {
			t.Errorf("%s starts tmux, but its package has no TestMain with testhome.Run: add main_test.go with os.Exit(testhome.Run(m)), so no tmux of the package reaches the builder's servers", file)
		}
	}
}

// TestTheRuleReadsCallsNotLines: the rule finds a tmux call written over
// several lines, and a flag in a comment does not satisfy it.
func TestTheRuleReadsCallsNotLines(t *testing.T) {
	for src, want := range map[string]int{
		"exec.Command(\n\t\"tmux\", \"kill-server\")":                          1,
		`exec.Command("tmux", "kill-server") // "-L"`:                          1,
		`exec.CommandContext(ctx, "tmux", "list-clients")`:                     1,
		`exec.Command("tmux", append([]string{"-L", sock}, args...)...)`:       0,
		`exec.Command("tmux", "-S", sock, "kill-server")`:                      0,
		`exec.Command("tmux", "-V")`:                                           0,
		`exec.Command("tmux", "kill-server") //nolint // ` + noSocket + ` why`: 0,
		`exec.Command("git", "status")`:                                        0,
		`exec.Command("tmux", "capture-pane", "-p", "-S", "-20")`:              1,
		`exec.Command("tmux", "select-pane", "-L")`:                            1,
		`exec.Command("tmux", "-f", "-L", "kill-server")`:                      1,
		`exec.Command("tmux", "-u", "-f", os.DevNull, "-L", sock, "ls")`:       0,
		`exec.Command("tmux", flags, "kill-server")`:                           1,
		`exec.Command("sh", "-c", "tmux kill-server")`:                         1,
		`exec.Command("bash", "-c", "x; tmux -L t ls; tmux kill-server")`:      1,
		`exec.Command("sh", "-c", "exec tmux -L t attach")`:                    0,
	} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "x_test.go", "package x\nfunc f() {\n"+src+"\n}\n", parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(unnamedTmux(fset, f)); got != want {
			t.Errorf("%s: %d findings, want %d", src, got, want)
		}
	}
	for src, want := range map[string]bool{
		"func TestMain(m *testing.M) { os.Exit(testhome.Run(m)) }": true,
		"func TestMain(m *testing.M) { os.Exit(m.Run()) }":         false,
		"// testhome.Run(m) is not called here\nfunc f() {}":       false,
	} {
		f, err := parser.ParseFile(token.NewFileSet(), "x_test.go", "package x\n"+src+"\n", parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		if got := clearsTmux(f); got != want {
			t.Errorf("%s: clears %v, want %v", src, got, want)
		}
	}
}

// tmuxCall is a tmux that a test starts: its exec call, and its arguments
// after the program. A token that is not a string literal is unknown.
type tmuxCall struct {
	call *ast.CallExpr
	args []tmuxArg
}

type tmuxArg struct {
	value string
	known bool
}

// tmuxCalls are the exec.Command and exec.CommandContext calls of f whose
// program is the literal "tmux", or sh or bash -c with a script that runs
// tmux: each tmux of the script is a call of its own.
func tmuxCalls(f *ast.File) []tmuxCall {
	var calls []tmuxCall
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || fmt.Sprint(sel.X) != "exec" {
			return true
		}
		prog := 0
		switch sel.Sel.Name {
		case "Command":
		case "CommandContext":
			prog = 1
		default:
			return true
		}
		if len(call.Args) <= prog {
			return true
		}
		switch literal(call.Args[prog]) {
		case "tmux":
			calls = append(calls, tmuxCall{call, tokens(call.Args[prog+1:])})
		case "sh", "bash":
			if len(call.Args) > prog+2 && literal(call.Args[prog+1]) == "-c" {
				words := strings.Fields(literal(call.Args[prog+2]))
				for i, w := range words {
					if w == "tmux" {
						args := []tmuxArg{}
						for _, a := range words[i+1:] {
							args = append(args, tmuxArg{strings.TrimRight(a, ";"), true})
						}
						calls = append(calls, tmuxCall{call, args})
					}
				}
			}
		}
		return true
	})
	return calls
}

// tokens are the arguments of a call, with append([]string{…}, …) read
// as its literal elements first.
func tokens(args []ast.Expr) []tmuxArg {
	var out []tmuxArg
	for _, a := range args {
		if app, ok := a.(*ast.CallExpr); ok && fmt.Sprint(app.Fun) == "append" && len(app.Args) > 0 {
			if lit, ok := app.Args[0].(*ast.CompositeLit); ok {
				out = append(out, tokens(lit.Elts)...)
			}
			out = append(out, tmuxArg{})
			continue
		}
		v := literal(a)
		out = append(out, tmuxArg{v, v != ""})
	}
	return out
}

// namesServer tells if tmux's global flags, which come before its command,
// hold -L or -S, or -V, which asks no server. A -S after the command is the
// command's own flag, as in capture-pane -S.
func namesServer(args []tmuxArg) bool {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case !a.known:
			return false
		case a.value == "-L" || a.value == "-S" || a.value == "-V":
			return true
		case a.value == "-c" || a.value == "-f" || a.value == "-T":
			i++ // the flag's value
		case !strings.HasPrefix(a.value, "-"):
			return false // the command
		}
	}
	return false
}

// unnamedTmux are the lines of the tmux calls of f that name no server,
// and carry no noSocket comment.
func unnamedTmux(fset *token.FileSet, f *ast.File) []int {
	marked := map[int]bool{}
	for _, g := range f.Comments {
		for _, c := range g.List {
			if strings.Contains(c.Text, noSocket) {
				marked[fset.Position(c.Pos()).Line] = true
			}
		}
	}
	var bad []int
	for _, c := range tmuxCalls(f) {
		from, to := fset.Position(c.call.Pos()).Line, fset.Position(c.call.End()).Line
		if !namesServer(c.args) && !marked[from] && !marked[to] {
			bad = append(bad, from)
		}
	}
	return bad
}

// clearsTmux tells if f has a TestMain that calls testhome.Run, or Run in
// package testhome itself.
func clearsTmux(f *ast.File) bool {
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "TestMain" || fn.Recv != nil || fn.Body == nil {
			continue
		}
		found := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return !found
			}
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				found = found || fun.Sel.Name == "Run" && fmt.Sprint(fun.X) == "testhome"
			case *ast.Ident:
				found = found || fun.Name == "Run" && f.Name.Name == "testhome"
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

// literal is the value of a string literal node, or "".
func literal(n ast.Node) string {
	lit, ok := n.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return s
}
