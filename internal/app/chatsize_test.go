package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// sizedChat is chatModel with its first watch sent.
func sizedChat(t *testing.T) (Model, *fakeConn) {
	t.Helper()
	st := fixtureState()
	st.Sessions = []sample.Session{{PID: 1, Pane: "%1", Name: "a"}, {PID: 2, Pane: "%2", Name: "b"}}
	f := newFake()
	next, _ := New(Options{NoMotion: true, Conn: f}).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	next, cmd := next.Update(stateMsg(st))
	drain(cmd)
	return next.(Model), f
}

// numbered is a screen of n lines, each cols wide: "L<i>" then dots.
func numbered(n, cols int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		l := fmt.Sprintf("L%d ", i)
		b.WriteString(l + strings.Repeat(".", max(cols-len(l), 0)) + "\n")
	}
	return b.String()
}

// The watch says the room the chat draws the agent's screen in: a screen
// of that size fills it, its widest line whole and its last rows shown.
func TestTheWatchSaysTheChatsSize(t *testing.T) {
	m, f := sizedChat(t)
	a := lastWatch(t, f)
	if a.Cols <= 0 || a.Rows <= 0 {
		t.Fatalf("watch %+v says no size", a)
	}
	sel, _ := m.selected()
	text := numbered(a.Rows, a.Cols)
	next, _ := m.Update(screenMsg{Pane: sel.Pane, Text: text, Cols: a.Cols, Rows: a.Rows})
	out := ansi.Strip(next.(Model).render())
	for _, want := range []string{"L1 ", fmt.Sprintf("L%d ", a.Rows)} {
		row := want + strings.Repeat(".", a.Cols-len(want))
		if !strings.Contains(out, row) {
			t.Fatalf("the chat does not show %q whole in %dx%d:\n%s", want, a.Cols, a.Rows, out)
		}
	}
	next, _ = m.Update(screenMsg{Pane: sel.Pane, Text: numbered(a.Rows+1, a.Cols), Cols: a.Cols, Rows: a.Rows + 1})
	if out := ansi.Strip(next.(Model).render()); strings.Contains(out, "L1 .") || !strings.Contains(out, "L2 .") {
		t.Fatalf("one row more than the chat has: the first goes\n%s", out)
	}
}

// A new size of the console sends the watch again with the chat's new
// size; the same size sends nothing.
func TestTheWatchFollowsTheChatsSize(t *testing.T) {
	m, f := sizedChat(t)
	first := lastWatch(t, f)
	n := len(f.allActs())
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	drain(cmd)
	if len(f.allActs()) != n {
		t.Fatalf("the same size sent %+v", f.allActs()[n:])
	}
	next, cmd = next.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	drain(cmd)
	a := lastWatch(t, f)
	if len(f.allActs()) != n+1 || a.RecID != first.RecID || a.Cols <= first.Cols || a.Rows <= first.Rows {
		t.Fatalf("watch %+v after a bigger console, was %+v", a, first)
	}
	if cols, rows := next.(Model).chatSize(); a.Cols != cols || a.Rows != rows {
		t.Fatalf("watch %dx%d, the chat %dx%d", a.Cols, a.Rows, cols, rows)
	}
}

// A console with no chat on screen says no size: the agent's window
// keeps its own. A chat that goes off screen keeps its size, so the agent
// does not redraw at each tab.
func TestNoChatNoSize(t *testing.T) {
	st := fixtureState()
	st.Sessions = []sample.Session{{PID: 1, Pane: "%1", Name: "a"}}
	f := newFake()
	next, _ := New(Options{NoMotion: true, Conn: f}).Update(tea.WindowSizeMsg{Width: 60, Height: 30}) // one column: the list, no chat
	next, cmd := next.Update(stateMsg(st))
	drain(cmd)
	if a := lastWatch(t, f); a.RecID != proto.WatchPrefix+"%1" || a.Cols != 0 || a.Rows != 0 {
		t.Fatalf("watch %+v with no chat drawn", a)
	}
	if cols, rows := next.(Model).chatSize(); cols != 0 || rows != 0 {
		t.Fatalf("chat size %dx%d with no chat drawn", cols, rows)
	}

	m, f := sizedChat(t)
	n := len(f.allActs())
	for _, msg := range []tea.Msg{tea.WindowSizeMsg{Width: 60, Height: 30}, keyPress("tab")} {
		next, cmd := m.Update(msg)
		drain(cmd)
		m = next.(Model)
	}
	if got := f.allActs()[n:]; len(got) != 0 {
		t.Fatalf("a chat off screen sent %+v", got)
	}
}

// A screen the agent drew as wide as the chat is drawn as it is: its
// trailing spaces and the blank rows under it stay, top first, as in the
// agent's own terminal. One of another width wraps as before.
func TestAScreenAsWideAsTheChatIsDrawnAsItIs(t *testing.T) {
	m, _ := chatModel(t)
	sel, _ := m.selected()
	text := "\x1b[44m╭──────╮   \x1b[0m\n│ a  b │\n╰──────╯\n\n\n"
	m.screen = proto.Screen{Pane: sel.Pane, Text: text, Cols: 11, Rows: 5}
	got := m.chatLines(11, 6)
	if len(got) != 6 || got[0] != "\x1b[44m╭──────╮   \x1b[0m" || got[2] != "╰──────╯" || got[3] != "" || got[5] != "" {
		t.Fatalf("as drawn: %q", got)
	}
	m.screen.Cols = 12
	if got := m.chatLines(11, 6); ansi.Strip(got[5]) != "╰──────╯" {
		t.Fatalf("another width fits: %q", got)
	}
	m.screen = proto.Screen{Pane: sel.Pane, Text: text, Cols: 11, Rows: 5, Up: 3}
	if got := m.chatLines(11, 6); ansi.Strip(got[5]) != "╰──────╯" {
		t.Fatalf("the history fits: %q", got)
	}
}

// sizedScreen is an agent's screen drawn at cols by rows: a table as wide
// as the screen at the top, the rows under it blank, as an agent that
// draws in place leaves them.
func sizedScreen(cols, rows int) string {
	inner := cols - 2
	name := max(inner/3, 6)
	rule := func(l, mid, r string) string {
		return l + strings.Repeat("─", name) + mid + strings.Repeat("─", inner-name-1) + r
	}
	row := func(a, b string) string {
		return "│" + a + strings.Repeat(" ", name-ansi.StringWidth(a)) + "│" + b + strings.Repeat(" ", inner-name-1-ansi.StringWidth(b)) + "│"
	}
	lines := []string{
		"⏺ The endpoints the refresh touches:",
		rule("┌", "┬", "┐"),
		row(" path", " what it does"),
		rule("├", "┼", "┤"),
		row(" /refresh", " rotates the token"),
		row(" /revoke", " ends all its tokens"),
		rule("└", "┴", "┘"),
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n") + "\n"
}

// TestScreensSizedChat: the chat of an agent whose window took the chat's
// size draws its table whole, at the top. The same table at the agent's
// own width of 120 columns would wrap every row.
func TestScreensSizedChat(t *testing.T) {
	st := fixtureState()
	for _, size := range [][2]int{{132, 32}, {44, 31}} {
		w, h := size[0], size[1]
		m := New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
		next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
		next, _ = next.Update(stateMsg(st))
		nm := next.(Model)
		nm.splashDone = true
		next, _ = key(nm, "enter")
		nm = next.(Model)
		cols, rows := nm.chatSize()
		if cols <= 0 || rows <= 0 {
			t.Fatalf("%dx%d: no chat size", w, h)
		}
		sel, _ := nm.selected()
		if wide := fitScreen(sizedScreen(120, rows), cols, rows); len(wide) <= 7 {
			t.Fatalf("%dx%d: the table at 120 columns does not wrap at %d", w, h, cols)
		}
		next, _ = nm.Update(screenMsg{Pane: sel.Pane, At: demoAt, Text: sizedScreen(cols, rows), Cols: cols, Rows: rows})
		golden(t, next.(Model), "chatsized", w, h)
	}
}
