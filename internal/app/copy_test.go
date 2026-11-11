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

// cellsIn is cellsOf with each row cut to the cells x0 to x1 of a column.
func cellsIn(frame string, a, b point, x0, x1 int) string {
	lines := strings.Split(frame, "\n")
	var out []string
	for y := a.y; y <= b.y; y++ {
		from, to := x0, x1
		if y == a.y {
			from = max(a.x, x0)
		}
		if y == b.y {
			to = min(b.x+1, x1)
		}
		out = append(out, strings.TrimRight(ansi.Cut(lines[y], from, max(from, to)), " "))
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

// columnOn is the cells of the board's column head over x.
func columnOn(t *testing.T, m Model, x int) (x0, x1 int) {
	t.Helper()
	for _, h := range m.boardPanel(max(m.width, minWidth), m.bodyRows()).heads {
		if x >= h.x0 && x < h.x1 {
			return h.x0, h.x1
		}
	}
	t.Fatalf("no column at %d", x)
	return 0, 0
}

func TestADragCopiesOnlyWhatWasUnderThePointer(t *testing.T) {
	// A press on a card that is not chosen, then a drag: the press never
	// chooses the card first, and no row reaches into the next column.
	for _, released := range []bool{true, false} {
		m, clipped, _ := menuBoard(t, 80, 31, func(m *Model) { m.sawRelease = released })
		before := plainFrame(m)
		x, y := cardOn(t, m, "green #4556")
		from, to := point{x, y}, point{x + 22, y + 1}
		m = drag(t, m, from, point{x + 5, y}, to)
		x0, x1 := columnOn(t, m, x)
		want := cellsIn(before, from, to, x0, x1)
		if !strings.HasPrefix(want, "green #4556 waits for your merge\n") {
			t.Fatalf("the press is not where the test thinks: %q", want)
		}
		if during := plainFrame(m); cellsIn(during, from, to, x0, x1) != want {
			t.Errorf("released %v: the frame moved under the drag:\n%s", released, during)
		}
		m = letGo(t, m, to)
		if len(*clipped) != 1 || (*clipped)[0] != want {
			t.Errorf("released %v: the drag copied %q, want %q", released, *clipped, want)
		}
		for _, other := range []string{"spark-docs", "hearth-li", "Running", "“Every"} {
			if len(*clipped) == 1 && strings.Contains((*clipped)[0], other) {
				t.Errorf("released %v: the drag copied %q of another column or card: %q", released, other, (*clipped)[0])
			}
		}
		if released && chosenName(t, m) == "kindle-web" {
			t.Errorf("a drag chose the card it started on")
		}
	}
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
			x0, x1 := columnOn(t, m, x)
			want := cellsIn(frame, from, to, x0, x1)
			if want == "" || rows > 0 && strings.Count(want, "\n") != rows {
				t.Fatalf("%s: the drag covers %q", name, want)
			}
			raw := strings.Split(m.render(), "\n")
			end := x1 // the rows of a drag stay in the column of its press
			if rows == 0 {
				end = to.x + 1
			}
			first := ansi.Cut(strings.Split(frame, "\n")[from.y], from.x, end) // the cells, spaces and all
			if !strings.Contains(raw[from.y], m.st.selected.Render(first)) || strings.Contains(raw[from.y], m.st.accent.Render(first)) {
				t.Errorf("%s: the cells under the drag are not on the wash: %q", name, raw[from.y])
			}
			m = letGo(t, m, to)
			if len(*clipped) != 1 || (*clipped)[0] != want {
				t.Errorf("%s, %d rows down: the drag copied %q, want %q", name, rows, *clipped, want)
			}
			if got, said := noticeRow(m), fmt.Sprintf("copied: %d characters", utf8.RuneCountInString(want)); got != said {
				t.Errorf("%s: the drag says %q, want %q", name, got, said)
			}
			if m.dragFrom != nil || m.dragTo != nil || strings.Contains(m.render(), m.st.selected.Render(first)) {
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

		// A drag that comes back to within a cell of its start is a tap too.
		m, clipped, _ = menuBoard(t, size[0], size[1], nil)
		x, y = cardOn(t, m, "kindle-web")
		m = letGo(t, drag(t, m, point{x, y}, point{x + 8, y + 2}, point{x + 1, y}), point{x + 1, y})
		if len(*clipped) != 0 || strings.HasPrefix(noticeRow(m), "copied") {
			t.Errorf("%s: a drag back to its start copied %q", name, *clipped)
		}
	}

	// One character is a character.
	if said := characters(1); said != "1 character" {
		t.Errorf("one character is %q", said)
	}
	if said := characters(42); said != "42 characters" {
		t.Errorf("42 characters are %q", said)
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
		{"external", nil, false, true}, // tmux's default sets the terminal's clipboard, never from an application
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
	for _, value := range []string{"off", "external"} {
		t.Run(value, func(t *testing.T) { copyThroughTmux(t, value) })
	}
}

func copyThroughTmux(t *testing.T, value string) {
	const fix = "not copied: tmux drops it · tmux set -s set-clipboard on"
	var ran [][]string
	var execd [][]string
	m, clipped, _ := menuBoard(t, 80, 31, func(m *Model) {
		chooseCard("flint-auth")(m)
		m.opt.Pane, m.opt.Tmux = "%1", tmuxSays(value, &ran)
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
	if got := noticeRow(m); got != m.pasteHere() {
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

func TestTheCopyCheckSaysHowToLeave(t *testing.T) {
	for _, size := range [][2]int{{40, 34}, {80, 31}} {
		name := fmt.Sprintf("%dx%d", size[0], size[1])
		m, _, clock := uncheckedBoard(t, size[0], size[1], t.TempDir(), nil)
		m = pressOn(toDoors(t, m), "enter")
		if m.copyCode == "" {
			t.Fatalf("%s: ⏎ on the doors line started no check", name)
		}
		*clock = clock.Add(time.Hour)
		m = upd(t, m, noticeMsg{})
		if row := stripRow(m); !strings.Contains(row, "Now paste here · esc cancels") {
			t.Errorf("%s: an hour on, the check's row says %q", name, row)
		}
		if keys := keyLineOf(m); !strings.Contains(keys, "esc cancel") || strings.Contains(keys, "set up") {
			t.Errorf("%s: while the check waits the key line says %q", name, keys)
		}
		m = pressOn(m, "down") // off the doors line
		if m.copyCode != "" || strings.Contains(stripRow(m), "paste") || strings.Contains(keyLineOf(m), "cancel") {
			t.Errorf("%s: a move off the doors line keeps the check: code %q, row %q, keys %q", name, m.copyCode, stripRow(m), keyLineOf(m))
		}
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
	if got := noticeRow(m); got != pasteWrong {
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
	if strings.Contains(plainFrame(m), m.pasteHere()) {
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

// releasing is menuBoard on a terminal that has reported a release, with
// its jumps kept in ran.
func releasing(t *testing.T, sawRelease bool, ran *[]string) (Model, *[]string) {
	t.Helper()
	return releasingAt(t, 80, 31, sawRelease, ran)
}

// releasingAt is releasing at w by h, kindle-web chosen by hand.
func releasingAt(t *testing.T, w, h int, sawRelease bool, ran *[]string) (Model, *[]string) {
	t.Helper()
	m, clipped, _ := menuBoard(t, w, h, func(m *Model) {
		chooseCard("kindle-web")(m)
		m.chooseByHand()
		m.opt.Run = func(argv []string) error { *ran = argv; return nil }
	})
	m.sawRelease = sawRelease
	return m, clipped
}

// headOn is the cell of the column name word on the heads line.
func headOn(t *testing.T, m Model, word string) point {
	t.Helper()
	for y, l := range strings.Split(plainFrame(m), "\n") {
		if i := strings.Index(l, word+" ·"); i >= 0 && y > 2 {
			return point{ansi.StringWidth(l[:i]) + 1, y}
		}
	}
	t.Fatalf("no head %s:\n%s", word, plainFrame(m))
	return point{}
}

func TestADragIsNeverATap(t *testing.T) {
	goIn := "tmux switch-client -t " + paneNamed(boardFixture(t), "kindle-web")

	var ran []string
	m, clipped := releasing(t, true, &ran)
	x, y := cardOn(t, m, "kindle-web")
	m = mouse(t, m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}) // its commands run
	m = mouse(t, m, tea.MouseMotionMsg{X: x + 12, Y: y + 2, Button: tea.MouseLeft})
	letGo(t, m, point{x + 12, y + 2})
	if len(ran) != 0 || len(*clipped) != 1 {
		t.Errorf("a drag from the chosen card: ran %q, copied %q", ran, *clipped)
	}

	m, _ = releasing(t, true, &ran)
	m = mouse(t, m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if len(ran) != 0 {
		t.Fatalf("a press on the chosen card went in before the release: %q", ran)
	}
	if letGo(t, m, point{x, y}); strings.Join(ran, " ") != goIn {
		t.Errorf("a click on the chosen card ran %q, want %q", ran, goIn)
	}

	ran = nil
	m, clipped = releasing(t, true, &ran)
	h := headOn(t, m, "WORKING")
	m = letGo(t, drag(t, m, h, point{h.x + 10, h.y + 2}), point{h.x + 10, h.y + 2})
	if m.colHidden(colWork) || len(*clipped) != 1 {
		t.Errorf("a drag from a column's name: hidden %v, copied %q", m.colHidden(colWork), *clipped)
	}
	m, _ = releasing(t, true, &ran)
	if m = pressAt(t, m, h.x, h.y); m.colHidden(colWork) {
		t.Fatal("a press on a column's name hid it before the release")
	}
	if m = letGo(t, m, h); !m.colHidden(colWork) {
		t.Error("a click on a column's name did not hide it")
	}

	// A terminal that never reported a release: both act on the press.
	ran = nil
	m, _ = releasing(t, false, &ran)
	if mouse(t, m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}); strings.Join(ran, " ") != goIn {
		t.Errorf("with no release seen, a press on the chosen card ran %q", ran)
	}
	if m = pressAt(t, m, h.x, h.y); !m.colHidden(colWork) {
		t.Error("with no release seen, a press on a column's name did not hide it")
	}

	// A right press opens the menu and starts no drag.
	m, clipped = releasing(t, true, &ran)
	m = rightClick(t, m, x, y)
	m = mouse(t, m, tea.MouseMotionMsg{X: x + 12, Y: y + 2, Button: tea.MouseRight})
	if m = letGo(t, m, point{x + 12, y + 2}); m.dragTo != nil || len(*clipped) != 0 || m.menu == nil {
		t.Errorf("a right press: drag %v, copied %q, menu %v", m.dragTo, *clipped, m.menu)
	}
}

func TestTheCheckBlocksNoOtherDoor(t *testing.T) {
	open := func() Model {
		m, _, _ := uncheckedBoard(t, 80, 31, t.TempDir(), func(m *Model) {
			m.doors = []doors.Door{{ID: doors.Tmux, Title: "Install tmux", Term: []string{"sh", "-c", "true"}}}
		})
		return toDoors(t, m)
	}
	m := pressOn(open(), "enter")
	if m.copyCode == "" || m.pending != nil {
		t.Errorf("⏎ alone: code %q, pending %v; want the check", m.copyCode, m.pending)
	}
	m = pressOn(open(), "right")
	if !strings.Contains(plainFrame(m), "SET UP · 2 of 2 · Install tmux") {
		t.Errorf("→ does not name the next door:\n%s", plainFrame(m))
	}
	if m = pressOn(m, "enter"); m.pending == nil || m.pending.door != doors.Tmux || m.copyCode != "" {
		t.Errorf("→ then ⏎ opened %v, code %q; want the tmux door", m.pending, m.copyCode)
	}
	if m = pressOn(open(), "right", "left"); !strings.Contains(plainFrame(m), "SET UP · 1 of 2 · Check that copy reaches you") {
		t.Errorf("← does not step back:\n%s", plainFrame(m))
	}

	m = open()
	for y, l := range strings.Split(plainFrame(m), "\n") {
		if strings.Contains(l, "SET UP") {
			m = mouse(t, m, tea.MouseClickMsg{X: 3, Y: y, Button: tea.MouseLeft})
		}
	}
	if m.copyCode == "" {
		t.Error("a tap on the doors line opened no check")
	}
}

func TestTheFirstCSaysWhatItCopied(t *testing.T) {
	m, clipped, clock := uncheckedBoard(t, 80, 31, t.TempDir(), chooseCard("flint-auth"))
	m.viewPrefs.Told = nil
	m = pressOn(m, "c")
	if got := noticeRow(m); got != "c copies now. The stoker is on its card." || len(*clipped) != 1 {
		t.Fatalf("the first c says %q, copied %q", got, *clipped)
	}
	*clock = clock.Add(noticeFor)
	if m = mouse(t, m, noticeMsg{}); noticeRow(m) != "copied: last message of flint-auth · not checked here" {
		t.Errorf("after the key's news: %q", noticeRow(m))
	}
}

// wentIn: msgs hold the jump into the pane of name.
func wentIn(msgs []tea.Msg, pane string) bool {
	return slices.ContainsFunc(msgs, func(msg tea.Msg) bool {
		r, ok := msg.(ranMsg)
		return ok && r.cmd == "tmux switch-client -t "+pane
	})
}

func TestOnlyACardYouChoseGoesIn(t *testing.T) {
	m, _, _ := menuBoard(t, 80, 31, nil)
	m.sawRelease = true
	first := chosenName(t, m) // the board's own first choice
	pane := paneNamed(boardFixture(t), first)
	x, y := cardOn(t, m, first)
	click := func(m Model) (Model, []tea.Msg) {
		next, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
		msgs := cmdMsgs(cmd)
		next, cmd = next.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
		return next.(Model), append(msgs, cmdMsgs(cmd)...)
	}
	m, msgs := click(m)
	if wentIn(msgs, pane) || chosenName(t, m) != first {
		t.Fatalf("the first tap on the card the board chose went in: %v", msgs)
	}
	if _, msgs = click(m); !wentIn(msgs, pane) {
		t.Errorf("the second tap did not go in: %v", msgs)
	}

	// A key move chooses by hand too; a choice that moves by itself does not.
	m, _, _ = menuBoard(t, 80, 31, nil)
	m = pressOn(m, "down", "up")
	if !m.chosenByHand(matchNamed(t, m, first)) {
		t.Error("↓ ↑ is no choice by hand")
	}
	m = pressOn(m, "down")
	m.boardPick = map[boardCol]paneKey{m.boardAt: paneOf(matchNamed(t, m, first))}
	if m = upd(t, m, noticeMsg{}); m.chosenByHand(matchNamed(t, m, first)) {
		t.Error("a choice that moved by itself counts as the person's")
	}
}

func TestALongPressOnTheChosenCardOnAPhone(t *testing.T) {
	goIn := "tmux switch-client -t " + paneNamed(boardFixture(t), "kindle-web")
	var ran []string
	open := func(sawRelease bool) (Model, int, int) {
		ran = nil
		m, _ := releasingAt(t, 40, 34, sawRelease, &ran)
		x, y := cardOn(t, m, "kindle-web")
		return m, x, y
	}

	m, x, y := open(true)
	m = mouse(t, m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if m = mouse(t, m, longPressMsg{m.longPress.id}); m.menu == nil || len(ran) != 0 {
		t.Errorf("a long press on the chosen card: menu %v, ran %q", m.menu, ran)
	}
	if m = letGo(t, m, point{x, y}); len(ran) != 0 {
		t.Errorf("the release after the menu ran %q", ran)
	}

	m, x, y = open(true)
	m = mouse(t, m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if letGo(t, m, point{x, y}); strings.Join(ran, " ") != goIn { // at 100 ms: before the tick
		t.Errorf("a tap with a release ran %q, want %q", ran, goIn)
	}

	m, x, y = open(false)
	if mouse(t, m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}); strings.Join(ran, " ") != goIn {
		t.Errorf("with no release seen, a press ran %q", ran)
	}

	// A right release alone is no release of a tap.
	m, x, y = open(false)
	m = mouse(t, m, tea.MouseReleaseMsg{X: 1, Y: 1, Button: tea.MouseRight})
	if m.sawRelease {
		t.Fatal("a right release counts as a left one")
	}
	if mouse(t, m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}); strings.Join(ran, " ") != goIn {
		t.Errorf("after a right release, a press ran %q", ran)
	}
}

func TestALostReleaseRunsNoOldTap(t *testing.T) {
	var ran []string
	m, _ := releasing(t, true, &ran)
	x, y := cardOn(t, m, "kindle-web")
	m = mouse(t, m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}) // its release is lost
	if m.tapLater == nil {
		t.Fatal("the press on the chosen card waits for no release")
	}
	o, oy := cardOn(t, m, "hearth-api")
	m = mouse(t, m, tea.MouseClickMsg{X: o, Y: oy, Button: tea.MouseLeft})
	if m.tapLater != nil && *m.tapLater == (point{x, y}) {
		t.Error("a new press keeps the old tap")
	}
	if m = letGo(t, m, point{x, y}); len(ran) != 0 {
		t.Errorf("a later release ran the old tap: %q", ran)
	}
}

func TestATapOnTheDoorsLineUnderAHost(t *testing.T) {
	m := lostPeer(t, 40*time.Second)
	m.splashDone, m.board = true, true
	m = upd(t, m, tea.WindowSizeMsg{Width: 80, Height: 31})
	if len(m.unreached()) != 1 {
		t.Fatalf("unreached hosts: %q", m.unreached())
	}
	for y, l := range strings.Split(plainFrame(m), "\n") {
		if strings.Contains(l, "SET UP") {
			m = mouse(t, m, tea.MouseClickMsg{X: 3, Y: y, Button: tea.MouseLeft})
		}
	}
	if line, above := m.aboveChosen(); !above || line != "" || m.copyCode == "" {
		t.Errorf("a tap on the doors line chose %q, %v, code %q", line, above, m.copyCode)
	}
}

func TestTheNamedDoorResetsWhenTheDoorsChange(t *testing.T) {
	m, _, _ := uncheckedBoard(t, 80, 31, t.TempDir(), func(m *Model) {
		m.doors = []doors.Door{{ID: doors.Tmux, Title: "Install tmux", Term: []string{"sh", "-c", "true"}}}
	})
	m = pressOn(toDoors(t, m), "right")
	if !strings.Contains(plainFrame(m), "SET UP · 2 of 2 · Install tmux") {
		t.Fatalf("→ named no second door:\n%s", plainFrame(m))
	}
	m.doors = append(m.doors, doors.Door{ID: doors.Guide, Title: "Open a guide session"})
	if !strings.Contains(plainFrame(m), "SET UP · 1 of 3 · Check that copy reaches you") {
		t.Errorf("a new list of doors keeps the old door:\n%s", plainFrame(m))
	}
}
