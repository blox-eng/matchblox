package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// toastLines are the lines of m's frame at w that hold text, by index.
func toastLines(m Model, w int, text string) (drawn, []int) {
	sc := m.draw(w)
	var at []int
	for i, l := range sc.lines {
		if strings.Contains(ansi.Strip(l), text) {
			at = append(at, i)
		}
	}
	return sc, at
}

func TestATellShowsAToastAndTheKeysStay(t *testing.T) {
	m := listShot(t, boardFixture(t), 160, 38, "kindle-web")
	keysBefore := ansi.Strip(m.draw(160).lines[m.draw(160).keysAt])
	m.say("sent: kill api")
	sc, at := toastLines(m, 160, "sent: kill api")
	if len(at) != 1 {
		t.Fatalf("the toast is on lines %v, want one", at)
	}
	if at[0] < sc.bodyTop || at[0] >= sc.bodyTop+3 {
		t.Fatalf("the toast is on line %d, want the top of the body (%d)", at[0], sc.bodyTop)
	}
	if x := strings.Index(ansi.Strip(sc.lines[at[0]]), "sent: kill api"); x < 160/2 {
		t.Fatalf("the toast starts at column %d, want the right half", x)
	}
	if keys := ansi.Strip(sc.lines[sc.keysAt]); keys != keysBefore {
		t.Fatalf("the key line changed:\n%q\nwant\n%q", keys, keysBefore)
	}
}

func TestAnErrorToastStaysLonger(t *testing.T) {
	m := listShot(t, boardFixture(t), 160, 38, "kindle-web")
	m.say("sent: kill api")
	m.sayError("kill failed: no such pane")
	ev, er := m.saying[1], m.saying[0]
	if got, want := er.until.Sub(m.now()), 2*ev.until.Sub(m.now()); got != want {
		t.Fatalf("an error lives %v, want %v", got, want)
	}
}

func TestAToastKeepsOffTheChosenRow(t *testing.T) {
	m := listShot(t, boardFixture(t), 60, 30, "spark-search") // one pane: the list is the whole width
	m.say("sent: kill api")
	sc, at := toastLines(m, 60, "sent: kill api")
	for _, i := range at {
		if strings.Contains(ansi.Strip(sc.lines[i]), "spark-search") || strings.Contains(sc.lines[i], "▌") {
			t.Fatalf("the toast covers the chosen row on line %d: %q", i, ansi.Strip(sc.lines[i]))
		}
	}
	if len(at) == 0 {
		t.Fatal("no toast drawn")
	}
}

func TestAToldTextIsNotSaidTwice(t *testing.T) {
	m := listShot(t, boardFixture(t), 160, 38, "kindle-web")
	m.say("sent: kill api")
	if _, at := toastLines(m, 160, "sent: kill api"); len(at) != 1 {
		t.Fatalf("the text shows on lines %v, want only the toast", at)
	}
}

// shownSaid is what the console says now, its lines joined, once the
// frame draws every one of them; "" when a line is not drawn.
func shownSaid(m Model) string {
	said := saidRows(m, max(m.width, minWidth))
	frame := ansi.Strip(m.render())
	for _, l := range said {
		if !strings.Contains(frame, l) {
			return ""
		}
	}
	return strings.Join(said, " ")
}

// stackSays is every toast the stack shows, newest first, joined by " | ".
func stackSays(m Model) string {
	var said []string
	for _, t := range m.sayingNow() {
		said = append(said, t.text)
	}
	return strings.Join(said, " | ")
}

// toastNow is the newest toast the stack shows, "" when none.
func toastNow(m Model) string {
	if now := m.sayingNow(); len(now) > 0 {
		return now[0].text
	}
	return ""
}

// saidRows are the lines the console says now, plain: the newest
// toast's, else the list foot's.
func saidRows(m Model, w int) []string {
	var rows []string
	if now := m.sayingNow(); len(now) > 0 {
		m.saying = now[:1]
		rows = m.sayRows(w)
	} else {
		rows, _, _ = m.noticeAt(w)
	}
	var out []string
	for _, r := range plainLines(rows) {
		out = append(out, strings.TrimPrefix(strings.TrimSpace(r), "⚠ "))
	}
	return out
}

// askingFlash are the writes of m.flash that ask a question and wait for a
// key: they keep the key line, so they are not toasts. By file and function.
var askingFlash = map[string]string{
	"mouse.go tap":             "tap again: the second tap runs what the first one showed",
	"stoker.go answerTakeOver": "Take over? why the answer did not go, while the question waits",
}

func TestOneFunctionTellsThePerson(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var bad []string
	for _, e := range entries {
		path := e.Name()
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || path == "say.go" {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			where := path + " " + fn.Name.Name
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				as, ok := n.(*ast.AssignStmt)
				if !ok {
					return true
				}
				for i, lhs := range as.Lhs {
					sel, ok := lhs.(*ast.SelectorExpr)
					if !ok || (sel.Sel.Name != "flash" && sel.Sel.Name != "toast") {
						continue
					}
					if len(as.Rhs) == len(as.Lhs) {
						if lit, ok := as.Rhs[i].(*ast.BasicLit); ok && lit.Value == `""` {
							continue
						}
					}
					if askingFlash[where] != "" || shellsOwn(fn, sel) {
						continue
					}
					bad = append(bad, fset.Position(as.Pos()).String()+" ("+fn.Name.Name+")")
				}
				return true
			})
		}
	}
	if len(bad) > 0 {
		slices.Sort(bad)
		t.Fatalf("%d places tell the person around say: call m.say or m.sayError (plan 2026-11-20-acts-show-at-once, Task 9)\n%s", len(bad), strings.Join(bad, "\n"))
	}
}

// shellsOwn: the write is the Hosts layer's own flash (Shell.flash), which
// its action line draws: that screen has no toasts.
func shellsOwn(fn *ast.FuncDecl, sel *ast.SelectorExpr) bool {
	if fn.Recv == nil || len(fn.Recv.List[0].Names) == 0 {
		return false
	}
	recv, ok := fn.Recv.List[0].Type.(*ast.Ident)
	x, isIdent := sel.X.(*ast.Ident)
	return ok && recv.Name == "Shell" && isIdent && x.Name == fn.Recv.List[0].Names[0].Name
}

// TestAToastShowsOverTheWholeChat: the chat that takes the whole screen
// says things too; its key line keeps its keys.
func TestAToastShowsOverTheWholeChat(t *testing.T) {
	m := standIn(t, 132)
	if !m.viewFull() {
		t.Fatal("the stand-in is not the whole screen: the test shows nothing")
	}
	m.sayError(demoLine)
	if said := shownSaid(m); said != demoLine {
		t.Errorf("the chat says %q, want %q", said, demoLine)
	}
	if keys := ansi.Strip(m.draw(132).lines[m.draw(132).keysAt]); strings.Contains(keys, "This is a demo") {
		t.Errorf("the key line says it: %q", keys)
	}
}

func TestAKeyEndsWhatWasSaidBeforeIt(t *testing.T) {
	m := listShot(t, boardFixture(t), 160, 38, "kindle-web")
	m.sayError("This is a demo. Install matchblox to do that.")
	m = pressOn(m, "esc")
	if m.sayingIt("This is a demo. Install matchblox to do that.") {
		t.Fatalf("a key left the toast said before it: %+v", m.sayingNow())
	}
}

func TestAShortToastIsOneLine(t *testing.T) {
	m := listShot(t, boardFixture(t), 132, 32, "kindle-web")
	m.sayError("This is a demo. Install matchblox to do that.")
	if _, at := toastLines(m, 132, "This is a demo. Install matchblox to do that."); len(at) != 1 {
		t.Fatalf("a short toast is cut: on lines %v", at)
	}
}
