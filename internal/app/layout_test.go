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
	for _, c := range []struct {
		cols, rows int
		want       Layout
	}{
		{107, 60, Medium},  // iPhone Pro Max upright, keyboard up: the drawer
		{107, 101, Medium}, // keyboard down
		{107, 96, Medium},  // mid-animation
		{205, 21, Wide},    // landscape, keyboard down
		{205, 5, Wide},     // landscape, keyboard up
		{192, 60, Wide},    // a desk
		{44, 31, Narrow},
		{86, 30, Medium},
		{132, 32, Wide},
		{100, 30, Wide},
		{120, 60, Medium}, // rows*2 == cols is portrait
		{89, 60, Narrow},  // upright under 90: one column
		{90, 45, Medium},
		{70, 120, Narrow},
		{121, 60, Wide},
		{59, 10, Narrow},
		{60, 10, Medium},
		{99, 10, Medium},
		{0, 0, Narrow},
		{1, 1, Narrow},
		{80, 0, Medium}, // no height yet: by width
		{200, 0, Wide},
	} {
		if got := layout(c.cols, c.rows); got != c.want {
			t.Errorf("layout(%d, %d) = %v, want %v", c.cols, c.rows, got, c.want)
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

// sessLine is the screen row of a session's row: its name between
// spaces, never a recommendation that names it ("Compact x (%1)").
func sessLine(t *testing.T, m tea.Model, name string) int {
	t.Helper()
	lw, _ := m.(Model).panes(max(m.(Model).width, minWidth))
	for i, line := range strings.Split(ansi.Strip(m.(Model).render()), "\n") {
		if r := []rune(line); lw > 0 && len(r) > lw {
			line = string(r[:lw]) // the list, not the detail beside it
		}
		if strings.Contains(line+" ", " "+name+" ") && !strings.Contains(line, "(%") {
			return i
		}
	}
	t.Fatalf("no row of %q:\n%s", name, ansi.Strip(m.(Model).render()))
	return -1
}

func tap(m tea.Model, x, y int) (tea.Model, tea.Cmd) {
	return m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
}

func TestTapOnATabOpensIt(t *testing.T) {
	for _, w := range []int{58, 100} {
		m, _ := loadedWith(t, w, queueState())
		line := ansi.Strip(strings.Split(m.render(), "\n")[1])
		x := strings.Index(line, "machine")
		if x < 0 {
			t.Fatalf("width %d: no machine tab in %q", w, line)
		}
		next, _ := tap(m, len([]rune(line[:x])), 1)
		if got := next.(Model).tab; got != tabMachine {
			t.Fatalf("width %d: tab %d, want the machine tab", w, got)
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
	m.tab = tabMachine
	m.selectRow(len(machineViews)) // the orphan, under the views
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

// TestNoCtrlKeysNeeded: every key is on a phone SSH key bar (DESIGN.md §6).
func TestNoCtrlKeysNeeded(t *testing.T) {
	for _, w := range []int{50, 100} {
		m, _ := loadedWith(t, w, queueState())
		for tab := range screenNames() { // the tabs and Machine's views
			m.tab = tab
			line, _ := m.keyLine(w)
			footer := ansi.Strip(line)
			if strings.Contains(strings.ToLower(footer), "ctrl") || strings.Contains(footer, "^") {
				t.Fatalf("width %d tab %d: footer names a Ctrl key: %q", w, tab, footer)
			}
		}
		for tab := range tabNames { // Tab, not a digit: a digit answers a card
			m.tab = tab
			if next, _ := key(m, "tab"); next.(Model).tab != (tab+1)%len(tabNames) {
				t.Fatalf("tab on tab %d does not open the next one", tab)
			}
		}
		m.tab = tabSessions
		ran := recordRuns(&m)
		next, _ := key(m, "down") // the second row waits for an answer
		_, cmd := key(next, "a")
		drain(cmd)
		if len(*ran) != 1 {
			t.Fatal("a does not go into the pane")
		}
		// The way back is a tap on the list or the app's home gesture:
		// TestClicksMoveTheKeys and TestHomeKeyGivesKeysBack.
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

// TestNarrowScrollsToTheSelection: on a phone the list scrolls, and the
// selected row stays on the screen, opened.
func TestNarrowScrollsToTheSelection(t *testing.T) {
	m, _ := loadedWith(t, 50, manySessions(30))
	var next tea.Model = m
	next, _ = key(next, "1")
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

// head is the header line, without its tap target.
func (m Model) head(w int) string {
	h, _, _ := m.header(w)
	return h
}

// selectPane chooses the card of the session in pane.
func selectPane(t *testing.T, m tea.Model, pane string) tea.Model {
	t.Helper()
	nm := m.(Model)
	for _, s := range nm.snap.Sessions {
		if s.Pane == pane {
			choose(&nm, s.PID)
			return nm
		}
	}
	t.Fatalf("no session in %s to select", pane)
	return m
}

// listOf is the list column of a screen line: its first w cells, the list
// width of Sessions on a console w wide.
func listOf(line string, w int) string {
	lw := w
	if byWidth(w) != Narrow {
		lw = splitAt(w, byWidth(w))
	}
	if byWidth(w) == Wide {
		lw = min(lw, w*2/5)
	}
	r := []rune(line)
	return string(r[:min(lw, len(r))])
}

// TestAToastSaysWhatRan: after an action the header's right says what ran,
// cut before the name is, then goes; a failure stays on the key line.
func TestAToastSaysWhatRan(t *testing.T) {
	m, _ := loadedWith(t, 44, queueState())
	m.tab = tabMachine // the board says what ran in its strip's row (TestEveryNoticeKeepsItsFix)
	next, cmd := m.Update(resultMsg(proto.Result{Ran: [][]string{{"tmux", "send-keys", "-t", "%2", "-l", "--", "a long answer for a narrow screen"}}}))
	head := strings.Split(screen(next), "\n")[0]
	if !strings.HasPrefix(head, " ▰ matchblox") || !strings.Contains(head, "ran: tmux") || !strings.Contains(head, "…") {
		t.Fatalf("header %q", head)
	}
	if cmd == nil {
		t.Fatal("the toast never goes")
	}
	gone, _ := next.Update(toastMsg{n: next.(Model).toasts})
	if head := strings.Split(screen(gone), "\n")[0]; strings.Contains(head, "ran:") || !strings.Contains(head, "● 2") {
		t.Fatalf("after the toast: %q", head)
	}
	failed, _ := m.Update(resultMsg(proto.Result{Err: "the pane is gone"}))
	if !strings.Contains(actionLine(failed), "failed: the pane is gone") || failed.(Model).toast != "" {
		t.Fatalf("a failure: key line %q, toast %q", actionLine(failed), failed.(Model).toast)
	}
}

// TestTheDockSaysTheDisk: the home's file system as a bar, its percent and
// its size, and / beside it when it is another.
func TestTheDockSaysTheDisk(t *testing.T) {
	st := queueState()
	st.Machine.Disks = []sample.Disk{{Path: "/home/user", Total: 1 << 40, Used: 710 << 30, Avail: 290 << 30}, {Path: "/", Total: 100 << 30, Used: 90 << 30, Avail: 10 << 30}}
	m, _ := loadedWith(t, 100, st)
	out := ansi.Strip(strings.Join(m.machineLines(60, false), "\n"))
	if !strings.Contains(out, "disk ▰▰▰▰▰▰▱▱ 71% of 1 TB · / 90%") {
		t.Fatalf("the machine lacks the disk:\n%s", out)
	}
	if got := bytesWord(931 << 30); got != "931 GB" {
		t.Fatalf("bytesWord %q", got)
	}
}

// TestRowColumnsLineUp: every session row spans its pane, and its word
// starts at the same x on every row, at every width and shape.
func TestRowColumnsLineUp(t *testing.T) {
	for _, size := range [][2]int{{44, 31}, {86, 30}, {107, 60}, {132, 32}, {192, 60}} {
		w, h := size[0], size[1]
		m := loaded(t, w)
		next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
		m = next.(Model)
		lw, _ := m.panes(w)
		if lw == 0 {
			lw = w
		}
		x := -1
		for _, s := range m.all.sessions {
			line := ansi.Strip(m.sessionRow(s, false, lw)[0])
			if got := ansi.StringWidth(line); got != lw {
				t.Fatalf("%dx%d: %s's row is %d wide, want %d: %q", w, h, s.Name, got, lw, line)
			}
			word := m.wordOf(s)
			at := strings.LastIndex(line, " "+word+" ")
			if at < 0 {
				t.Fatalf("%dx%d: no word %q in %q", w, h, word, line)
			}
			if wx := ansi.StringWidth(line[:at]); x < 0 {
				x = wx
			} else if wx != x {
				t.Fatalf("%dx%d: %s's word is at %d, the others at %d: %q", w, h, s.Name, wx, x, line)
			}
		}
	}
}

// TestAKeyboardKeepsTheShape: a 60-column screen at 34 rows is upright; a
// keyboard that takes it to 21 rows keeps it one column, upright. A new
// width decides afresh (design/0006 §4).
func TestAKeyboardKeepsTheShape(t *testing.T) {
	m, _ := loadedWith(t, 60, fixtureState())
	next, _ := fullView(m).Update(tea.WindowSizeMsg{Width: 60, Height: 34})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 60, Height: 21})
	nm := next.(Model)
	if _, right := nm.panes(60); nm.layoutAt(60) != Narrow || !nm.upright() || right != 0 {
		t.Fatalf("60x21 after 60x34: layout %v, upright %v, right side %d:\n%s", nm.layoutAt(60), nm.upright(), right, screen(nm))
	}
	next, _ = next.Update(tea.WindowSizeMsg{Width: 100, Height: 21})
	if l := next.(Model).layoutAt(100); l != Wide {
		t.Fatalf("100x21: layout %v, want Wide", l)
	}
}
