package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/doors"
)

// menuBoard is the board at w by h on the fixture, with its copies kept in
// clipped and its jumps run by nothing, on a clock the test moves.
func menuBoard(t *testing.T, w, h int, edit func(*Model)) (Model, *[]string, *time.Time) {
	t.Helper()
	var clipped []string
	clock := demoAt
	m := boardShot(t, boardFixture(t), w, h, func(m *Model) {
		m.opt.Now = func() time.Time { return clock }
		m.opt.Clip = func(s string) { clipped = append(clipped, s) }
		m.opt.Run = func([]string) error { return nil }
		m.viewPrefs.Told = []string{"copy", "chain", "digit"}
		if edit != nil {
			edit(m)
		}
	})
	return upd(t, m, noticeMsg{}), &clipped, &clock
}

// cardOn is the cell of name's card on the frame: the first cell of its name.
func cardOn(t *testing.T, m Model, name string) (int, int) {
	t.Helper()
	for y, l := range strings.Split(plainFrame(m), "\n") {
		if i := strings.Index(l, name); i >= 0 {
			return lipgloss.Width(l[:i]), y
		}
	}
	t.Fatalf("no %s on the frame:\n%s", name, plainFrame(m))
	return 0, 0
}

func menuLabels(m Model) []string {
	if m.menu == nil {
		return nil
	}
	var out []string
	for _, it := range m.menu.items {
		out = append(out, it.label)
	}
	return out
}

func mouse(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, cmd := m.Update(msg)
	drain(cmd)
	return next.(Model)
}

func rightClick(t *testing.T, m Model, x, y int) Model {
	return mouse(t, m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseRight})
}

func pressAt(t *testing.T, m Model, x, y int) Model {
	t.Helper()
	next, _ := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	return next.(Model) // the long-press tick is not waited for: the test sends it
}

func TestRightClickOpensTheMenuOnThatCard(t *testing.T) {
	m, _, _ := menuBoard(t, 80, 31, nil)
	x, y := cardOn(t, m, "kindle-web")
	m = rightClick(t, m, x, y)
	if m.menu == nil || chosenName(t, m) != "kindle-web" {
		t.Fatalf("a right-click on kindle-web: menu %v, chosen %s", m.menu, chosenName(t, m))
	}
	if l := menuLabels(m); l[0] != "copy last message" {
		t.Errorf("the first line is %q, want copy last message", l[0])
	}
	if !strings.Contains(plainFrame(m), "copy last message") {
		t.Errorf("the menu is not on the frame:\n%s", plainFrame(m))
	}

	off, _, _ := menuBoard(t, 80, 31, nil)
	off.tab = tabMachine
	for _, msg := range []tea.Msg{tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseRight},
		tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft}, tea.MouseMotionMsg{X: x + 3, Y: y, Button: tea.MouseLeft}} {
		was := plainFrame(off)
		if off = mouse(t, off, msg); off.menu != nil || plainFrame(off) != was {
			t.Errorf("with the board off, %T changed the screen", msg)
		}
	}
}

func TestALongPressOpensTheMenu(t *testing.T) {
	if longPressFor != 480*time.Millisecond {
		t.Errorf("a long press is %v, want 480ms", longPressFor)
	}
	open := func() (Model, int, int) {
		m, _, _ := menuBoard(t, 80, 31, nil)
		m = mouse(t, m, tea.MouseReleaseMsg{X: 1, Y: 1, Button: tea.MouseLeft}) // the terminal reports releases
		x, y := cardOn(t, m, "kindle-web")
		m = pressAt(t, m, x, y)
		if m.tapLater == nil || m.menu != nil {
			t.Fatalf("a press on kindle-web waits for no release: menu %v", m.menu)
		}
		return m, x, y
	}

	m, _, _ := open()
	if m = mouse(t, m, longPressMsg{m.longPress.id}); m.menu == nil || chosenName(t, m) != "kindle-web" {
		t.Errorf("no release by 480 ms: menu %v", m.menu)
	}

	m, x, y := open()
	m = mouse(t, m, tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	if m = mouse(t, m, longPressMsg{m.longPress.id}); m.menu != nil {
		t.Error("a release at 200 ms opened the menu")
	}

	m, x, y = open()
	m = mouse(t, m, tea.MouseMotionMsg{X: x + 2, Y: y, Button: tea.MouseLeft})
	if m = mouse(t, m, longPressMsg{m.longPress.id}); m.menu != nil {
		t.Error("a move of two cells is a long press")
	}

	m, x, y = open()
	m = mouse(t, m, tea.MouseMotionMsg{X: x + 1, Y: y + 1, Button: tea.MouseLeft})
	if m = mouse(t, m, longPressMsg{m.longPress.id}); m.menu == nil {
		t.Error("a move of one cell undid the long press")
	}

	m, _, _ = open()
	if m = mouse(t, m, longPressMsg{m.longPress.id - 1}); m.menu != nil {
		t.Error("the tick of an older press opened the menu")
	}
}

func TestNoReleaseEverMeansNoLongPress(t *testing.T) {
	m, _, _ := menuBoard(t, 80, 31, nil)
	x, y := cardOn(t, m, "kindle-web")
	m = pressAt(t, m, x, y)
	if chosenName(t, m) != "kindle-web" {
		t.Fatalf("a press chose %s", chosenName(t, m))
	}
	for _, id := range []int{m.longPress.id, m.longPress.id + 1} {
		if m = mouse(t, m, longPressMsg{id}); m.menu != nil {
			t.Fatal("a terminal that never reported a release has a long press")
		}
	}
	if m = pressOn(m, "m"); m.menu == nil {
		t.Error("m opened no menu")
	}
	m = pressOn(m, "esc")
	if m = rightClick(t, m, x, y); m.menu == nil {
		t.Error("a right-click opened no menu")
	}
}

func TestTheMenuHasAKey(t *testing.T) {
	m, _, _ := menuBoard(t, 80, 31, chooseCard("flint-auth"))
	m = pressOn(m, "m")
	s, _ := m.boardSelected()
	if m.menu == nil || m.menu.pane != paneOf(s) || s.Name != "flint-auth" {
		t.Fatalf("m opened %v on %s", m.menu, s.Name)
	}
	var want []string
	for _, it := range m.menuFor(s) {
		want = append(want, it.label)
	}
	if !slices.Equal(menuLabels(m), want) {
		t.Errorf("m's menu %q, a right-click's %q", menuLabels(m), want)
	}
	if !strings.Contains(plainFrame(m), "show only the chain login") {
		t.Errorf("the menu of m is not drawn:\n%s", plainFrame(m))
	}
}

func TestTheMenuOnlyOffersWhatApplies(t *testing.T) {
	withLink := func(m *Model) {
		m.allShips = slices.Clone(m.allShips)
		for i, sh := range m.allShips {
			if sh.Number == 4556 {
				m.allShips[i].URL = "https://github.com/hearth/kindle-web/pull/4556"
			}
		}
	}
	m, _, _ := menuBoard(t, 80, 31, withLink)
	for name, want := range map[string][]string{
		"flint-auth": {"copy last message", "copy the question", "copy its name", "show only the chain login", "go in", "hide needs you"},
		"kindle-web": {"copy last message", "copy the link of #4556", "copy its name", "go in", "hide needs you"},
		"spark-docs": {"copy last message", "copy its name", "go in", "hide working"},
		"flint-api":  {"copy last message", "copy its name", "show only the chain login", "go in", "hide shipping"},
	} {
		var got []string
		for _, it := range m.menuFor(matchNamed(t, m, name)) {
			got = append(got, it.label)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s's menu is %q, want %q", name, got, want)
		}
	}
}

func TestTheMenuHasNoDestructiveLine(t *testing.T) {
	m, _, _ := menuBoard(t, 160, 38, nil)
	b, _ := m.boardDrawOf()
	for _, ss := range b.idx {
		for _, s := range ss {
			items := m.menuFor(s)
			for i, it := range items {
				for _, word := range []string{"kill", "merge", "give", "park", "remove", "delete", "stop", "quit", "send", "answer"} {
					if strings.Contains(it.label, word) {
						t.Errorf("%s's menu has %q", s.Name, it.label)
					}
				}
				run := m
				run.boardChooseCard(m.columnOf(s), s)
				run = run.openMenu(s, 0, 5)
				for range i {
					run = pressOn(run, "down")
				}
				// go in to another host asks for its y, as ⏎ does: a jump, never
				// a destructive step.
				if run = pressOn(run, "enter"); run.pending != nil && run.pending.destructive || run.held != nil || run.menu != nil {
					t.Errorf("%s's %q opened a destructive step or kept the menu: %v", s.Name, it.label, run.pending)
				}
			}
		}
	}
}

func TestTheMenuMovesAndCloses(t *testing.T) {
	m, clipped, _ := menuBoard(t, 80, 31, chooseCard("flint-auth"))
	m = pressOn(m, "m")
	if m = pressOn(m, "down", "down"); m.menu.at != 2 {
		t.Fatalf("two ↓ are on line %d", m.menu.at)
	}
	if m = pressOn(m, "up"); m.menu.at != 1 || menuLabels(m)[1] != "copy the question" {
		t.Fatalf("↑ is on line %d", m.menu.at)
	}
	if m = pressOn(m, "up", "up"); m.menu.at != 0 {
		t.Errorf("↑ past the first line is on line %d", m.menu.at)
	}
	m = pressOn(m, "down", "enter")
	if m.menu != nil || len(*clipped) != 1 || !strings.HasPrefix((*clipped)[0], "Which way do we fix the login?") {
		t.Fatalf("⏎ on copy the question: menu %v, copied %q", m.menu, *clipped)
	}
	if noticeRow(m) != "copied: the question of flint-auth" {
		t.Errorf("the copy says %q", noticeRow(m))
	}

	m.filter = "flint"
	m = pressOn(m, "m", "esc")
	if m.menu != nil || m.filter != "flint" {
		t.Errorf("esc: menu %v, search %q; it closes the menu first", m.menu, m.filter)
	}
	m.filter = ""

	m = pressOn(m, "m", "g")
	if m.menu != nil || m.boardChain != "login" {
		t.Errorf("g in the menu: menu %v, chain %q", m.menu, m.boardChain)
	}
	m.boardChain = ""

	m = pressOn(m, "m")
	if m = mouse(t, m, tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft}); m.menu != nil {
		t.Error("a tap outside the box left it open")
	}
	if m.boardChain != "" || m.flash != "" {
		t.Errorf("a tap outside the box did more than close it: chain %q", m.boardChain)
	}

	m = pressOn(m, "m")
	lines, x, y := m.menuBox(80, 31)
	name := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(ansi.Strip(l), "copy its name") })
	m = mouse(t, m, tea.MouseClickMsg{X: x + 4, Y: y + name, Button: tea.MouseLeft})
	if m.menu != nil || (*clipped)[len(*clipped)-1] != "flint-auth" {
		t.Errorf("a tap on copy its name: menu %v, copied %q", m.menu, *clipped)
	}

	m = pressOn(m, "m")
	m.tab = tabMachine
	if m = upd(t, m, noticeMsg{}); m.menu != nil {
		t.Error("the menu stays with the board off")
	}
}

func TestADigitInTheMenuArmsNothing(t *testing.T) {
	m, clock := flintBoard(t, 80, 31)
	m = pressOn(m, "m", "1", "1")
	if m.peekArmed != 0 || len(sent(m)) != 0 || m.menu == nil {
		t.Fatalf("a digit in the menu: marked %d, sent %+v, menu %v", m.peekArmed, sent(m), m.menu)
	}
	*clock = clock.Add(time.Second)
	if m = pressOn(m, "esc", "1"); armedOn(m, "1") || m.peekArmed != 0 {
		t.Error("a digit right after the menu closed marks an answer")
	}
	*clock = clock.Add(peekDead)
	if m = pressOn(m, "1"); !armedOn(m, "1") {
		t.Errorf("a digit %v after the menu closed marks nothing", peekDead)
	}
}

func TestTheMenuFitsEveryScreen(t *testing.T) {
	for _, size := range [][2]int{{40, 34}, {80, 31}, {160, 38}} {
		w, h := size[0], size[1]
		m, _, _ := menuBoard(t, w, h, chooseCard("flint-auth"))
		s, _ := m.boardSelected()
		m = m.openMenu(s, w-1, h-1)
		box, x, y := m.menuBox(w, h)
		bw := lipgloss.Width(box[0])
		if x < 0 || x+bw > w || y < 1 || y+len(box) > h-2 {
			t.Errorf("%dx%d: the box at %d,%d, %d by %d, is not on the screen above the key line", w, h, x, y, bw, len(box))
		}
		if l := m.layoutAt(w); l == Narrow && (bw != w || y+len(box) != h-2) {
			t.Errorf("%dx%d: a phone's box is %d wide and ends on row %d", w, h, bw, y+len(box))
		}
		frame := strings.Split(plainFrame(m), "\n")
		if len(frame) != h {
			t.Errorf("%dx%d: the frame has %d rows", w, h, len(frame))
		}
		for i, l := range frame {
			if lipgloss.Width(l) > w {
				t.Errorf("%dx%d: row %d is %d wide", w, h, i, lipgloss.Width(l))
			}
		}
		for j, b := range box {
			if row := frame[y+j]; !strings.Contains(row, ansi.Strip(b)) {
				t.Errorf("%dx%d: the box's row %d is not whole:\n%s", w, h, j, strings.Join(frame, "\n"))
				break
			}
		}
		for _, label := range menuLabels(m) {
			if !strings.Contains(plainFrame(m), label) {
				t.Errorf("%dx%d: no %q on the box", w, h, label)
			}
		}
	}
}

// TestAStaleLineAboveIsCleared: a chosen line above the columns that is
// gone is no choice to come back to when the lines come back.
func TestAStaleLineAboveIsCleared(t *testing.T) {
	m, _, _ := menuBoard(t, 80, 31, func(m *Model) {
		m.doors = []doors.Door{{ID: doors.Tmux, Title: "Install tmux", Term: []string{"sh", "-c", "true"}}}
	})
	if m = pressOn(m, "up"); m.boardAbove != 1 {
		t.Fatalf("↑ from the first card chose line %d", m.boardAbove)
	}
	m.doors = nil
	m = upd(t, m, noticeMsg{})
	m.doors = []doors.Door{{ID: doors.Tmux, Title: "Install tmux", Term: []string{"sh", "-c", "true"}}}
	if _, above := m.aboveChosen(); above || m.boardAbove != 0 {
		t.Errorf("the doors line came back chosen: %d", m.boardAbove)
	}
}

// TestATapOnTheQuietDrawerHidesIt: on Medium the drawer's name hides QUIET
// as a column's name does, closed and open, and its + quiet brings it back.
func TestATapOnTheQuietDrawerHidesIt(t *testing.T) {
	for name, edit := range map[string]func(*Model){"closed": nil, "open": chooseCard("ember-reports")} {
		m, _, _ := menuBoard(t, 80, 31, edit)
		_, y := cardOn(t, m, " QUIET · 5")
		if m = mouse(t, m, tea.MouseClickMsg{X: 3, Y: y, Button: tea.MouseLeft}); !m.colHidden(colQuiet) {
			t.Fatalf("%s: a tap on QUIET did not hide it:\n%s", name, plainFrame(m))
		}
		x, y := cardOn(t, m, "+ quiet 5")
		if m = mouse(t, m, tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseLeft}); m.colHidden(colQuiet) {
			t.Errorf("%s: + quiet did not show QUIET again", name)
		}
	}
	m, _, _ := menuBoard(t, 80, 31, nil)
	_, y := cardOn(t, m, " QUIET · 5")
	if m = mouse(t, m, tea.MouseClickMsg{X: 40, Y: y, Button: tea.MouseLeft}); m.colHidden(colQuiet) {
		t.Error("a tap on the drawer's names hid QUIET")
	}
}
