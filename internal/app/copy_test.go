package app

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/doors"
)

// lastMessage is a message of five lines: the peek shows less of it.
const lastMessage = "I can fix it in two ways.\nThe token check is a small patch.\nThe new session store is the long way.\nIt moves every login to a new table.\nWhich way do we take?"

// saysAtLength makes name's last message text, in its match and its wait.
func saysAtLength(name, text string) func(*Model) {
	return func(m *Model) {
		for i, s := range m.snap.Sessions {
			if s.Name != name {
				continue
			}
			m.snap.Sessions[i].LastLine = text
			for j, it := range m.queue {
				if it.Host == s.Host && it.Pane == s.Pane {
					m.queue[j].LastLine = text
				}
			}
		}
	}
}

// uncheckedBoard is menuBoard on a screen no paste has checked, its view
// kept in dir.
func uncheckedBoard(t *testing.T, w, h int, dir string, edit func(*Model)) (Model, *[]string, *time.Time) {
	t.Helper()
	return menuBoard(t, w, h, func(m *Model) {
		told := m.viewPrefs.Told
		m.opt.StateDir, m.viewPrefs = dir, loadViewPrefs(dir)
		m.viewPrefs.Told = told
		if edit != nil {
			edit(m)
		}
	})
}

// tmuxSays is a tmux whose set-clipboard is value, and that keeps every
// command it gets in ran.
func tmuxSays(value string, ran *[][]string) func(args ...string) (string, error) {
	return func(args ...string) (string, error) {
		*ran = append(*ran, args)
		if slices.Equal(args, []string{"show", "-sv", "set-clipboard"}) {
			return value + "\n", nil
		}
		return "", errors.New("unknown command")
	}
}

// toDoors moves up from the chosen card to the doors line.
func toDoors(t *testing.T, m Model) Model {
	t.Helper()
	for range 12 {
		if line, above := m.aboveChosen(); above && line == "" {
			return m
		}
		m = pressOn(m, "up")
	}
	t.Fatalf("↑ never reached the doors line:\n%s", plainFrame(m))
	return m
}

// menuDo opens the chosen card's menu and does its line label.
func menuDo(t *testing.T, m Model, label string) Model {
	t.Helper()
	m = pressOn(m, "m")
	i := slices.Index(menuLabels(m), label)
	if i < 0 {
		t.Fatalf("the menu has no %q: %q", label, menuLabels(m))
	}
	for range i {
		m = pressOn(m, "down")
	}
	return pressOn(m, "enter")
}

func boardDoorIDs(m Model) []string {
	var out []string
	for _, d := range m.boardDoors() {
		out = append(out, d.ID)
	}
	return out
}

func TestCCopiesTheLastMessage(t *testing.T) {
	m, clipped, _ := menuBoard(t, 80, 31, func(m *Model) {
		chooseCard("flint-auth")(m)
		saysAtLength("flint-auth", lastMessage)(m)
	})
	if peek := plainFrame(m); strings.Contains(peek, "Which way do we take?") {
		t.Fatalf("the peek shows the whole message: the test proves nothing\n%s", peek)
	}
	m = pressOn(m, "c")
	if len(*clipped) != 1 || (*clipped)[0] != lastMessage {
		t.Errorf("c copied %q, want the whole last message", *clipped)
	}
	if got := noticeRow(m); got != "copied: last message of flint-auth" {
		t.Errorf("c says %q", got)
	}

	// With no Clip, the program writes the text as OSC 52.
	m, _, _ = menuBoard(t, 80, 31, func(m *Model) {
		chooseCard("flint-auth")(m)
		saysAtLength("flint-auth", lastMessage)(m)
		m.opt.Clip = nil
	})
	_, cmd := key(m, "c")
	if !slices.Contains(cmdMsgs(cmd), tea.SetClipboard(lastMessage)()) {
		t.Errorf("c without a Clip gave %v, want OSC 52 of the last message", cmdMsgs(cmd))
	}
}

func TestEachMenuLineCopiesItsText(t *testing.T) {
	const link = "https://github.com/hearth/kindle-web/pull/4556"
	withLink := func(m *Model) { // the link proto.Ship carries
		m.allShips = slices.Clone(m.allShips)
		for i, sh := range m.allShips {
			if sh.Number == 4556 {
				m.allShips[i].URL = link
			}
		}
	}
	for _, c := range []struct{ card, line, text, said string }{
		{"flint-auth", "copy the question", "Which way do we fix the login?\n1. Patch the token check\n2. Move to the new session store", "copied: the question of flint-auth"},
		{"kindle-web", "copy the link of #4556", link, "copied: the link of #4556"},
		{"kindle-web", "copy its name", "kindle-web", "copied: kindle-web"},
	} {
		m, clipped, _ := menuBoard(t, 80, 31, func(m *Model) { withLink(m); chooseCard(c.card)(m) })
		m = menuDo(t, m, c.line)
		if c.text == "" || len(*clipped) != 1 || (*clipped)[0] != c.text {
			t.Errorf("%s on %s copied %q, want %q", c.line, c.card, *clipped, c.text)
		}
		if got := noticeRow(m); got != c.said {
			t.Errorf("%s on %s says %q, want %q", c.line, c.card, got, c.said)
		}
	}
}

// drag presses at from, moves through the cells of path, and gives the
// board at each step: pressed, then after each move.
func drag(t *testing.T, m Model, from point, path ...point) Model {
	t.Helper()
	m = pressAt(t, m, from.x, from.y)
	for _, c := range path {
		m = mouse(t, m, tea.MouseMotionMsg{X: c.x, Y: c.y, Button: tea.MouseLeft})
	}
	return m
}

func letGo(t *testing.T, m Model, at point) Model {
	t.Helper()
	return mouse(t, m, tea.MouseReleaseMsg{X: at.x, Y: at.y, Button: tea.MouseLeft})
}

// cellsOf is the plain text of frame from a to b, row by row: the first
// row from a, the last to b, each right-trimmed.
func cellsOf(frame string, a, b point) string {
	lines := strings.Split(frame, "\n")
	var out []string
	for y := a.y; y <= b.y; y++ {
		x0, x1 := 0, ansi.StringWidth(lines[y])
		if y == a.y {
			x0 = a.x
		}
		if y == b.y {
			x1 = b.x + 1
		}
		out = append(out, strings.TrimRight(ansi.Cut(lines[y], x0, x1), " "))
	}
	return strings.Join(out, "\n")
}

func TestADragCopiesWhatItCovers(t *testing.T) {
	for _, size := range [][2]int{{80, 31}, {160, 38}} {
		name := fmt.Sprintf("%dx%d", size[0], size[1])
		for _, rows := range []int{0, 2} {
			m, clipped, _ := menuBoard(t, size[0], size[1], nil)
			x, y := cardOn(t, m, "kindle-web")
			from, to := point{x, y}, point{x + 20, y + rows}
			m = drag(t, m, from, point{x + 5, y}, to)
			frame := plainFrame(m)
			want := cellsOf(frame, from, to)
			if want == "" || rows > 0 && strings.Count(want, "\n") != rows {
				t.Fatalf("%s: the drag covers %q", name, want)
			}
			raw := strings.Split(m.render(), "\n")
			end := ansi.StringWidth(strings.Split(frame, "\n")[from.y])
			if rows == 0 {
				end = to.x + 1
			}
			first := ansi.Cut(strings.Split(frame, "\n")[from.y], from.x, end) // the cells, spaces and all
			if !strings.Contains(raw[from.y], m.st.accent.Render(first)) {
				t.Errorf("%s: the cells under the drag are not in the accent: %q", name, raw[from.y])
			}
			m = letGo(t, m, to)
			if len(*clipped) != 1 || (*clipped)[0] != want {
				t.Errorf("%s, %d rows down: the drag copied %q, want %q", name, rows, *clipped, want)
			}
			if got, said := noticeRow(m), fmt.Sprintf("copied: %d characters", utf8.RuneCountInString(want)); got != said {
				t.Errorf("%s: the drag says %q, want %q", name, got, said)
			}
			if m.dragFrom != nil || m.dragTo != nil || strings.Contains(m.render(), m.st.accent.Render(first)) {
				t.Errorf("%s: the selection stays after the release", name)
			}
		}

		// A drag of one cell is a tap.
		m, clipped, _ := menuBoard(t, size[0], size[1], nil)
		x, y := cardOn(t, m, "kindle-web")
		m = letGo(t, drag(t, m, point{x, y}, point{x + 1, y + 1}), point{x + 1, y + 1})
		if len(*clipped) != 0 || chosenName(t, m) != "kindle-web" || strings.HasPrefix(noticeRow(m), "copied") {
			t.Errorf("%s: a drag of one cell copied %q", name, *clipped)
		}
	}

	m, clipped, _ := menuBoard(t, 40, 34, nil)
	x, y := cardOn(t, m, "hearth-api")
	m = drag(t, m, point{x, y}, point{x + 10, y + 2})
	if m.dragTo != nil || len(*clipped) != 0 {
		t.Fatalf("a drag on a phone selects: %v %q", m.dragTo, *clipped)
	}
	if m = letGo(t, m, point{x + 10, y + 2}); len(*clipped) != 0 {
		t.Errorf("a drag on a phone copied %q", *clipped)
	}
}

func TestADragIsNoLongPress(t *testing.T) {
	press := func() (Model, *[]string, point) {
		m, clipped, _ := menuBoard(t, 80, 31, nil)
		m = mouse(t, m, tea.MouseReleaseMsg{X: 1, Y: 1, Button: tea.MouseLeft}) // the terminal reports releases
		x, y := cardOn(t, m, "kindle-web")
		return m, clipped, point{x, y}
	}

	m, clipped, at := press()
	m = drag(t, m, at, point{at.x + 2, at.y})
	if m.longPress.on || m.dragTo == nil {
		t.Fatalf("a move of two cells: long press %v, drag %v", m.longPress.on, m.dragTo)
	}
	if m = mouse(t, m, longPressMsg{m.longPress.id}); m.menu != nil {
		t.Error("a drag opened the menu")
	}
	if m = letGo(t, m, point{at.x + 2, at.y}); len(*clipped) != 1 {
		t.Errorf("the drag copied %q", *clipped)
	}

	m, clipped, at = press()
	m = drag(t, m, at, point{at.x + 1, at.y})
	if !m.longPress.on || m.dragTo != nil {
		t.Fatalf("a move of one cell: long press %v, drag %v", m.longPress.on, m.dragTo)
	}
	if m = mouse(t, m, longPressMsg{m.longPress.id}); m.menu == nil {
		t.Fatal("the long press opened no menu")
	}
	m = mouse(t, m, tea.MouseMotionMsg{X: at.x + 12, Y: at.y + 2, Button: tea.MouseLeft})
	if m = letGo(t, m, point{at.x + 12, at.y + 2}); m.dragTo != nil || len(*clipped) != 0 || m.menu == nil {
		t.Errorf("a drag after the long press: drag %v, copied %q, menu %v", m.dragTo, *clipped, m.menu)
	}

	// While the menu is open a drag does nothing: here it starts on the
	// menu's frame, where a press closes nothing.
	m, clipped, _ = press()
	m = pressOn(m, "m")
	_, bx, by := m.menuBox(80, len(frameLines(m)))
	m = letGo(t, drag(t, m, point{bx + 1, by}, point{bx + 6, by + 3}, point{0, 0}), point{0, 0})
	if m.dragTo != nil || len(*clipped) != 0 || m.menu == nil {
		t.Errorf("a drag with the menu open: drag %v, copied %q, menu %v", m.dragTo, *clipped, m.menu)
	}
}

func TestTmuxPassesClipboard(t *testing.T) {
	for _, c := range []struct {
		value     string
		err       error
		ok, known bool
	}{
		{"on", nil, true, true},
		{"external", nil, true, true},
		{"off", nil, false, true},
		{"", errors.New("no server running on /tmp/tmux-1000/default"), false, false},
	} {
		var ran [][]string
		run := tmuxSays(c.value, &ran)
		if c.err != nil {
			run = func(args ...string) (string, error) { ran = append(ran, args); return "", c.err }
		}
		if ok, known := tmuxPassesClipboard(run); ok != c.ok || known != c.known {
			t.Errorf("set-clipboard %q, %v: ok %v known %v, want %v %v", c.value, c.err, ok, known, c.ok, c.known)
		}
		if want := [][]string{{"show", "-sv", "set-clipboard"}}; !slices.EqualFunc(ran, want, slices.Equal) {
			t.Errorf("it ran %q, want only the read %q", ran, want)
		}
	}
}

func TestCopyNeverClaimsWhatTmuxDrops(t *testing.T) {
	const fix = "not copied: tmux drops it · tmux set -s set-clipboard on"
	var ran [][]string
	var execd [][]string
	m, clipped, _ := menuBoard(t, 80, 31, func(m *Model) {
		chooseCard("flint-auth")(m)
		m.opt.Pane, m.opt.Tmux = "%1", tmuxSays("off", &ran)
		m.opt.Exec = func(argv []string, done func(error) tea.Msg) tea.Cmd {
			execd = append(execd, argv)
			return func() tea.Msg { return done(nil) }
		}
	})
	m = pressOn(m, "c")
	if len(*clipped) != 0 {
		t.Errorf("c copied %q through a tmux that drops it", *clipped)
	}
	if got := noticeRow(m); got != fix {
		t.Errorf("c says %q, want %q", got, fix)
	}
	if !slices.Contains(boardDoorIDs(m), doors.CopyThrough) {
		t.Errorf("the board's doors are %q, want copy-through", boardDoorIDs(m))
	}
	first := m
	first.viewPrefs.Told = nil
	if got := noticeRow(pressOn(first, "c")); got != fix {
		t.Errorf("the first c ever, through a tmux that drops it, says %q", got)
	}
	m = menuDo(t, m, "copy its name")
	if len(*clipped) != 0 || noticeRow(m) != fix {
		t.Errorf("the menu copied %q and said %q", *clipped, noticeRow(m))
	}

	// ⏎ on the doors line asks for its y, and only the y runs the setting.
	m = pressOn(toDoors(t, m), "enter")
	if m.pending == nil || !m.consenting() || len(execd) != 0 {
		t.Fatalf("⏎ on copy-through: pending %v, ran %q", m.pending, execd)
	}
	next, cmd := key(m, "y")
	fed(next, cmd)
	if want := [][]string{{"tmux", "set", "-s", "set-clipboard", "on"}}; !slices.EqualFunc(execd, want, slices.Equal) {
		t.Errorf("y ran %q, want %q", execd, want)
	}
	for _, args := range ran {
		if !slices.Equal(args, []string{"show", "-sv", "set-clipboard"}) {
			t.Errorf("the console ran tmux %q by itself", args)
		}
	}
}

func TestOutsideTmuxCopyGoesOn(t *testing.T) {
	var ran [][]string
	m, clipped, _ := menuBoard(t, 80, 31, func(m *Model) {
		chooseCard("flint-auth")(m)
		m.opt.OutsideTmux, m.opt.Tmux = true, tmuxSays("off", &ran)
	})
	m = pressOn(m, "c")
	if len(*clipped) != 1 || noticeRow(m) != "copied: last message of flint-auth" || len(ran) != 0 {
		t.Errorf("outside tmux: copied %q, said %q, tmux ran %q", *clipped, noticeRow(m), ran)
	}
	if slices.Contains(boardDoorIDs(m), doors.CopyThrough) {
		t.Error("outside tmux the board offers copy-through")
	}

	// A tmux that answers no value it knows: the copy goes on too.
	m, clipped, _ = menuBoard(t, 80, 31, func(m *Model) {
		chooseCard("flint-auth")(m)
		m.opt.Pane, m.opt.Tmux = "%1", func(...string) (string, error) { return "", errors.New("no server running") }
	})
	if m = pressOn(m, "c"); len(*clipped) != 1 || noticeRow(m) != "copied: last message of flint-auth" {
		t.Errorf("a tmux with no answer: copied %q, said %q", *clipped, noticeRow(m))
	}
}

func TestCopySaysUncheckedUntilAPasteProvesIt(t *testing.T) {
	dir := t.TempDir()
	var ran []string
	m, clipped, _ := uncheckedBoard(t, 132, 32, dir, func(m *Model) {
		chooseCard("flint-auth")(m)
		m.opt.Run = func(argv []string) error { ran = argv; return nil }
	})
	m = pressOn(m, "c")
	if got := noticeRow(m); got != "copied: last message of flint-auth · not checked here" {
		t.Errorf("c on a screen not checked says %q", got)
	}
	next, cmd := key(m, "enter")
	fed(next, cmd)
	if want := "tmux switch-client -t " + paneNamed(boardFixture(t), "flint-auth"); strings.Join(ran, " ") != want {
		t.Errorf("⏎ after the toast ran %q, want %q", ran, want)
	}

	if ids := boardDoorIDs(m); len(ids) == 0 || ids[0] != doors.CopyCheck {
		t.Fatalf("the doors line is %q, want copy-check first", ids)
	}
	if !strings.Contains(plainFrame(m), "SET UP · 1 · Check that copy reaches you") {
		t.Errorf("the doors line does not name the check:\n%s", plainFrame(m))
	}
	m = pressOn(toDoors(t, m), "enter")
	if len(*clipped) != 2 {
		t.Fatalf("⏎ on the doors line copied %q", *clipped)
	}
	code := (*clipped)[1]
	if utf8.RuneCountInString(code) != 6 || strings.Trim(code, "abcdefghijklmnopqrstuvwxyz") != "" {
		t.Errorf("the code is %q, want 6 letters", code)
	}
	if got := noticeRow(m); got != "Now paste here." {
		t.Errorf("the check says %q", got)
	}
	m = pressOn(m, "esc", "enter")
	if again := (*clipped)[len(*clipped)-1]; again == code {
		t.Errorf("a second check copied the same code %q", code)
	}
	code = (*clipped)[len(*clipped)-1]

	m = mouse(t, m, tea.PasteMsg{Content: code})
	if !m.viewPrefs.CopyCheckedDesk || m.viewPrefs.CopyCheckedPhone || !m.copyChecked() {
		t.Errorf("the paste checked desk %v, phone %v", m.viewPrefs.CopyCheckedDesk, m.viewPrefs.CopyCheckedPhone)
	}
	if kept := loadViewPrefs(dir); !kept.CopyCheckedDesk || kept.CopyCheckedPhone {
		t.Errorf("view.json keeps desk %v, phone %v", kept.CopyCheckedDesk, kept.CopyCheckedPhone)
	}
	if slices.Contains(boardDoorIDs(m), doors.CopyCheck) {
		t.Error("the check stays a door on a checked screen")
	}
	chooseCard("flint-auth")(&m)
	m = pressOn(m, "c")
	if got := noticeRow(m); got != "copied: last message of flint-auth" {
		t.Errorf("c on a checked screen says %q", got)
	}
}

func TestAWrongPasteKeepsTheCheckOpen(t *testing.T) {
	dir := t.TempDir()
	m, clipped, _ := uncheckedBoard(t, 132, 32, dir, nil)
	m = pressOn(toDoors(t, m), "enter")
	if len(*clipped) != 1 {
		t.Fatalf("the check copied %q", *clipped)
	}
	sent := len(boardFake(m).sent)
	m = mouse(t, m, tea.PasteMsg{Content: "hello flint"})
	if got := noticeRow(m); got != `That is not what the console copied. A hop drops it: see docs/console.md, "Copy".` {
		t.Errorf("a wrong paste says %q", got)
	}
	if m.copyChecked() || loadViewPrefs(dir).CopyCheckedDesk || m.copyCode != (*clipped)[0] {
		t.Errorf("a wrong paste: checked %v, code %q", m.copyChecked(), m.copyCode)
	}
	if got := len(boardFake(m).sent); got != sent {
		t.Errorf("the paste went to the service: %d sends, want %d", got, sent)
	}
	if m = mouse(t, m, tea.PasteMsg{Content: (*clipped)[0] + "\n"}); !m.copyChecked() {
		t.Error("the right paste after a wrong one checks nothing")
	}
}

func TestNoPasteLeavesItUnchecked(t *testing.T) {
	m, _, _ := uncheckedBoard(t, 132, 32, t.TempDir(), chooseCard("flint-auth"))
	m = pressOn(toDoors(t, m), "enter", "esc")
	if m.copyCode != "" || m.copyChecked() {
		t.Fatalf("esc: code %q, checked %v", m.copyCode, m.copyChecked())
	}
	if strings.Contains(plainFrame(m), "Now paste here.") {
		t.Error("esc leaves the check's line")
	}
	m = mouse(t, m, tea.PasteMsg{Content: "abcdef"})
	if m.copyChecked() {
		t.Error("a paste after esc checks the screen")
	}
	m = pressOn(m, "down")
	chooseCard("flint-auth")(&m)
	m = pressOn(m, "c")
	if got := noticeRow(m); got != "copied: last message of flint-auth · not checked here" {
		t.Errorf("c says %q", got)
	}
}

func TestTheCopyDoorsStayOffTheOldConsole(t *testing.T) {
	var ran [][]string
	m, clipped, _ := uncheckedBoard(t, 80, 31, t.TempDir(), func(m *Model) {
		m.opt.Pane, m.opt.Tmux = "%1", tmuxSays("off", &ran)
	})
	m.board = false
	was := plainFrame(m)
	if strings.Contains(was, "copy") || len(m.doors) != 0 {
		t.Errorf("the old console shows a copy door: %v\n%s", m.doors, was)
	}
	m = mouse(t, m, tea.PasteMsg{Content: "abcdef"})
	x, y := 10, 6
	m = letGo(t, drag(t, m, point{x, y}, point{x + 20, y + 2}), point{x + 20, y + 2})
	if len(*clipped) != 0 || m.dragFrom != nil || m.copyChecked() {
		t.Errorf("the old console: copied %q, drag %v", *clipped, m.dragFrom)
	}
}

func TestTheMenuShowsItsOwnKeys(t *testing.T) {
	for _, w := range []int{40, 80} {
		m, _, _ := menuBoard(t, w, 31, chooseCard("flint-auth"))
		m = pressOn(m, "m")
		lines := strings.Split(plainFrame(m), "\n")
		if got := strings.TrimSpace(lines[len(lines)-1]); got != "↑↓ move   ⏎ do   esc close" {
			t.Errorf("%d columns: the key line with the menu open is %q", w, got)
		}
	}
}

func TestTheWheelLeavesTheMenuAlone(t *testing.T) {
	m, _, _ := menuBoard(t, 80, 31, chooseCard("flint-auth"))
	m = pressOn(m, "m")
	at := m.menu.at
	for _, b := range []tea.MouseButton{tea.MouseWheelDown, tea.MouseWheelUp, tea.MouseWheelDown} {
		m = mouse(t, m, tea.MouseWheelMsg{X: 20, Y: 10, Button: b})
	}
	if chosenName(t, m) != "flint-auth" || m.menu == nil || m.menu.at != at {
		t.Errorf("the wheel with the menu open: chosen %s, menu %v", chosenName(t, m), m.menu)
	}
}

func TestTheCheckLeadsTheDoorsLine(t *testing.T) {
	for _, size := range [][2]int{{40, 34}, {80, 31}} {
		m := boardShot(t, boardFixture(t), size[0], size[1], func(m *Model) {
			m.viewPrefs.CopyCheckedPhone, m.viewPrefs.CopyCheckedDesk = false, false
		})
		m = toDoors(t, m)
		golden(t, m, "boardcopycheck", size[0], size[1])
	}
}
