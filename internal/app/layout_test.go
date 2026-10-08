package app

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

func TestLayoutBreakpoints(t *testing.T) {
	for w, want := range map[int]Layout{40: Narrow, 59: Narrow, 60: Medium, 99: Medium, 100: Wide, 200: Wide} {
		if got := layout(w); got != want {
			t.Errorf("layout(%d) = %v, want %v", w, got, want)
		}
	}
}

// lineOf is the screen row of the first line that holds s.
func lineOf(t *testing.T, m tea.Model, s string) int {
	t.Helper()
	for i, line := range strings.Split(ansi.Strip(m.(Model).render()), "\n") {
		if strings.Contains(line, s) {
			return i
		}
	}
	t.Fatalf("no line holds %q:\n%s", s, ansi.Strip(m.(Model).render()))
	return -1
}

func fitsWidth(t *testing.T, m Model, w int) {
	t.Helper()
	for i, line := range strings.Split(m.render(), "\n") {
		if lw := lipgloss.Width(line); lw > w {
			t.Fatalf("width %d: line %d is %d wide: %q", w, i, lw, ansi.Strip(line))
		}
	}
}

func TestNarrowLayoutQueueFirst(t *testing.T) {
	m, _ := loadedWith(t, 50, queueState())
	fitsWidth(t, m, 50)
	out := strings.Split(ansi.Strip(m.render()), "\n")
	queueAt, rowAt := -1, -1
	for i, line := range out {
		if queueAt < 0 && strings.Contains(line, "2 WAITING FOR YOU") {
			queueAt = i
		}
		if rowAt < 0 && strings.Contains(line, "app-feature") {
			rowAt = i
		}
	}
	if queueAt < 0 || queueAt > 4 {
		t.Fatalf("the queue is not the first region:\n%s", strings.Join(out, "\n"))
	}
	// Two lines for each row: what and who on the first, where and the
	// last line of the session on the second.
	if rowAt < 0 || !strings.Contains(out[rowAt+1], "work:1.1") || !strings.Contains(out[rowAt+1], "Claude needs") {
		t.Fatalf("the row is not on two lines:\n%s", strings.Join(out, "\n"))
	}
	if strings.Contains(out[rowAt], "work:1.1") {
		t.Fatalf("the place belongs on the second line: %q", out[rowAt])
	}

	m.tab = tabSessions
	fitsWidth(t, m, 50)
	if i := lineOf(t, m, "app-feature"); !strings.Contains(ansi.Strip(strings.Split(m.render(), "\n")[i+1]), "work:1.1") {
		t.Fatalf("a session row is not on two lines:\n%s", ansi.Strip(m.render()))
	}
	for _, tab := range []int{tabMachine, tabProcs, tabGit, tabRecs, tabHistory, tabPanes} {
		m.tab = tab
		fitsWidth(t, m, 50)
	}
}

func tap(m tea.Model, x, y int) (tea.Model, tea.Cmd) {
	return m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
}

func TestTapSelectsThenActs(t *testing.T) {
	for _, w := range []int{50, 100} {
		m, f := loadedWith(t, w, queueState())
		var ran []string
		m.opt.Run = func(argv []string) error { ran = argv; return nil }

		// The first tap selects and shows what a second tap runs.
		y := lineOf(t, m, "app-review")
		if w < 60 {
			y++ // the second line of a row is the same row
		}
		next, cmd := tap(m, 5, y)
		if cmd != nil || ran != nil {
			t.Fatalf("width %d: the first tap ran something", w)
		}
		got := next.(Model)
		if it, _ := got.selectedQueue(); it.Pane != "%2" {
			t.Fatalf("width %d: selected %q, want %%2", w, it.Pane)
		}
		if !strings.Contains(ansi.Strip(got.render()), "tmux switch-client -t %2") {
			t.Fatalf("width %d: the step is not shown before it runs:\n%s", w, ansi.Strip(got.render()))
		}

		// The second tap on the same row runs the safe action.
		next, cmd = tap(next, 5, y)
		if cmd == nil {
			t.Fatalf("width %d: the second tap ran nothing", w)
		}
		next.Update(cmd())
		if strings.Join(ran, " ") != "tmux switch-client -t %2" {
			t.Fatalf("width %d: ran %v", w, ran)
		}
		if len(f.acts()) != 0 {
			t.Fatalf("width %d: a tap sent %v to the service", w, f.acts())
		}
	}
}

func TestTapOnATabOpensIt(t *testing.T) {
	for _, w := range []int{50, 100} {
		m, _ := loadedWith(t, w, queueState())
		line := ansi.Strip(strings.Split(m.render(), "\n")[1])
		x := strings.Index(line, "8 ")
		if x < 0 {
			t.Fatalf("width %d: no tab 8 in %q", w, line)
		}
		next, _ := tap(m, len([]rune(line[:x])), 1)
		if got := next.(Model).tab; got != tabPanes {
			t.Fatalf("width %d: tab %d, want the panes tab", w, got)
		}
	}
}

// TestTapNeverRunsADestructiveStep: a tap is never consent to kill or
// remove; those stay behind x and a typed y.
func TestTapNeverRunsADestructiveStep(t *testing.T) {
	st := fixtureState()
	st.Orphans = []sample.Orphan{{PID: 4242, Start: 7, Comm: "bash", Pane: "%1", PaneAlive: true, Kill: []string{"kill", "4242"}}}
	m, f := loadedWith(t, 100, st)
	var ran [][]string
	m.opt.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	m.tab = tabProcs
	o, ok := m.selectedOrphan()
	if !ok {
		t.Fatal("the fixture has no orphan")
	}
	y := lineOf(t, m, strconv.Itoa(o.PID))
	next, _ := tap(m, 5, y)
	next, cmd := tap(next, 5, y)
	if cmd != nil {
		next.Update(cmd())
	}
	for _, step := range ran {
		if step[0] == "kill" {
			t.Fatalf("a tap ran %v", step)
		}
	}
	if len(f.acts()) != 0 {
		t.Fatalf("a tap sent %v", f.acts())
	}
}

func TestTapWhilePendingCancels(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	next, _ := key(m, "enter")
	if next.(Model).pending == nil {
		t.Fatal("enter did not ask to confirm")
	}
	next, cmd := tap(next, 5, lineOf(t, next, "app-feature"))
	if cmd != nil || next.(Model).pending != nil {
		t.Fatal("a tap did not cancel the pending step")
	}
}

// TestNoCtrlKeysNeeded: every key is on a phone SSH key bar (DESIGN.md §6).
func TestNoCtrlKeysNeeded(t *testing.T) {
	for _, w := range []int{50, 100} {
		m, _ := loadedWith(t, w, queueState())
		for tab := range tabNames {
			m.tab = tab
			footer := ansi.Strip(m.footer(w))
			if strings.Contains(strings.ToLower(footer), "ctrl") || strings.Contains(footer, "^") {
				t.Fatalf("width %d tab %d: footer names a Ctrl key: %q", w, tab, footer)
			}
		}
		for tab := range tabNames {
			next, _ := key(m, string(rune('1'+tab)))
			if next.(Model).tab != tab {
				t.Fatalf("digit %d does not open its tab", tab+1)
			}
		}
		m.tab = tabQueue
		next, _ := key(m, "down") // the second row waits for an answer
		next, _ = key(next, "a")
		if next.(Model).input == nil {
			t.Fatal("a does not start an answer")
		}
		next, _ = key(next, "esc")
		if next.(Model).input != nil {
			t.Fatal("esc does not leave the answer")
		}
		if next, cmd := key(m, "q"); cmd == nil || !next.(Model).quitting {
			t.Fatal("q does not quit")
		}
	}
}

func TestMouseIsOn(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("the console does not take taps")
	}
}

func manySessions(n int) proto.State {
	st := fixtureState()
	base := st.Sessions[0]
	st.Sessions = nil
	for i := range n {
		s := base
		s.PID, s.Name, s.Pane, s.Target = 9000+i, fmt.Sprintf("agent-%02d", i), fmt.Sprintf("%%%d", 100+i), fmt.Sprintf("work:%d.1", i)
		st.Sessions = append(st.Sessions, s)
	}
	st.Queue = nil
	return st
}

// TestNarrowScrollsToTheSelection: on a phone, two lines a row fill the
// screen fast; the selected row stays on it.
func TestNarrowScrollsToTheSelection(t *testing.T) {
	m, _ := loadedWith(t, 50, manySessions(30))
	var next tea.Model = m
	next, _ = key(next, "2")
	for range 25 {
		next, _ = key(next, "down")
	}
	got := next.(Model)
	if !strings.Contains(ansi.Strip(got.render()), "agent-25") {
		t.Fatalf("the selected agent-25 is off the screen:\n%s", ansi.Strip(got.render()))
	}
	if got := len(strings.Split(got.render(), "\n")); got != 30 {
		t.Fatalf("%d lines on a 30-line screen", got)
	}
	// A tap on the row it shows selects that row.
	y := lineOf(t, next, "agent-24")
	next, _ = tap(next, 5, y)
	if s, _ := next.(Model).selected(); s.Name != "agent-24" {
		t.Fatalf("the tap selected %s", s.Name)
	}
}

// TestFirstTapOnTheSelectedRowOnlyShows: the default selection is not a
// tap; the step shows before a tap runs it, and a key in between disarms.
func TestFirstTapOnTheSelectedRowOnlyShows(t *testing.T) {
	m, _ := loadedWith(t, 50, queueState())
	var ran []string
	m.opt.Run = func(argv []string) error { ran = argv; return nil }
	y := lineOf(t, m, "app-feature")
	next, cmd := tap(m, 5, y)
	if cmd != nil || ran != nil {
		t.Fatal("one tap on the preselected row ran a step")
	}
	if !strings.Contains(next.(Model).flash, "tap again: tmux switch-client -t %1") {
		t.Fatalf("flash %q", next.(Model).flash)
	}
	next, _ = key(next, "down")
	next, _ = key(next, "up")
	if _, cmd = tap(next, 5, y); cmd != nil {
		t.Fatal("a tap after a key ran the step")
	}
}

func TestWheelMovesTheSelection(t *testing.T) {
	m, _ := loadedWith(t, 50, queueState())
	next, _ := m.Update(tea.MouseWheelMsg{X: 5, Y: 6, Button: tea.MouseWheelDown})
	if it, _ := next.(Model).selectedQueue(); it.Pane != "%2" {
		t.Fatalf("wheel down selected %s", it.Pane)
	}
	next, _ = next.Update(tea.MouseWheelMsg{X: 5, Y: 6, Button: tea.MouseWheelUp})
	if it, _ := next.(Model).selectedQueue(); it.Pane != "%1" {
		t.Fatalf("wheel up selected %s", it.Pane)
	}
}
