package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// A full-screen agent's screen is cut at the view's width, with a › at
// each cut, and never wraps: its box keeps its shape. The blank rows under
// it go, and its last rows show, where its prompt is.
func TestAFullScreenAgentIsCutNotWrapped(t *testing.T) {
	m, _ := viewModel(t)
	sel, _ := m.selected()
	text := "\x1b[44m╭────────────╮\x1b[0m\n│ a  b  c  d │\n╰────────────╯\n> prompt\n\n\n"
	m.screen = proto.Screen{Pane: sel.Pane, Text: text, Alt: true}
	got := m.viewLines(8, 6)
	want := []string{"", "", "╭──────›", "│ a  b ›", "╰──────›", "> prompt"}
	if len(got) != len(want) {
		t.Fatalf("lines %q, want %q", got, want)
	}
	for i := range want {
		if ansi.Strip(got[i]) != want[i] {
			t.Fatalf("line %d %q, want %q (all %q)", i, ansi.Strip(got[i]), want[i], got)
		}
	}
	if got := m.viewLines(8, 2); ansi.Strip(got[1]) != "> prompt" || ansi.Strip(got[0]) != "╰──────›" {
		t.Fatalf("a short view shows the last rows: %q", got)
	}
}

// A text agent's line, which tmux joined where the pane wrapped it, wraps
// again at the view's width: every word shows.
func TestATextAgentWrapsToTheView(t *testing.T) {
	m, _ := viewModel(t)
	sel, _ := m.selected()
	long := "codex says the tests pass and the branch is ready for review now"
	m.screen = proto.Screen{Pane: sel.Pane, Text: long + "\n"}
	lines := m.viewLines(20, 10)
	got := strings.Join(lines, " ")
	for _, word := range strings.Fields(long) {
		if !strings.Contains(got, word) {
			t.Fatalf("%q lost %q", got, word)
		}
	}
	for _, l := range lines {
		if ansi.StringWidth(l) > 20 {
			t.Fatalf("line %q is wider than the view", l)
		}
	}
}

// Browsing Sessions watches each row it rests on once: no second watch of
// the same row comes later with the view's size, as one did when the view
// sized the agent's window (#351).
func TestBrowsingWatchesWithNoSize(t *testing.T) {
	t.Skip("replaced by TestTheLiveViewFollowsTheChosenCard (Task 12 step 1): the board watches only beside its live view")
	st := fixtureState()
	st.Sessions = []sample.Session{{PID: 1, Pane: "%1", Name: "a"}, {PID: 2, Pane: "%2", Name: "b"}, {PID: 3, Pane: "%3", Name: "c"}}
	f := newFake()
	next, _ := New(Options{NoMotion: true, Conn: f, Now: func() time.Time { return demoAt }}).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	next, cmd := next.Update(stateMsg(st))
	drain(cmd)
	for range 2 {
		next, cmd = key(next, "down")
		drain(cmd)
	}
	var watched []string
	for _, a := range f.allActs() {
		if p, ok := strings.CutPrefix(a.RecID, proto.WatchPrefix); ok {
			watched = append(watched, p)
		}
	}
	sel, _ := next.(Model).selected()
	if len(watched) != 3 || watched[2] != sel.Pane || watched[0] == watched[1] || watched[1] == watched[2] {
		t.Fatalf("watched %q, want each of the 3 rows once, %s last", watched, sel.Pane)
	}
}

// viewRoom is the room the drawn view gives the agent's screen: its
// columns and rows, 0, 0 where no view is drawn.
func viewRoom(m Model) (cols, rows int) {
	sc := m.draw(max(m.width, minWidth))
	if sc.viewW == 0 {
		return 0, 0
	}
	return sc.viewW, sc.viewRows
}
