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

// toDoors opens the setup steps in the chat's place, as the foot's ! setup
// does.
func toDoors(t *testing.T, m Model) Model {
	t.Helper()
	m.openPlace(placeSetup)
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

func TestADragCopiesWhatItCovers(t *testing.T) {
	for _, size := range [][2]int{{100, 31}, {160, 38}} {
		name := fmt.Sprintf("%dx%d", size[0], size[1])
		for _, rows := range []int{0, 2} {
			m, clipped, _ := menuBoard(t, size[0], size[1], nil)
			x, y := cardOn(t, m, "kindle-web")
			from, to := point{x, y}, point{x + 20, y + rows}
			m = drag(t, m, from, point{x + 5, y}, to)
			frame := plainFrame(m)
			cols := m.dragColumn(x, y-m.drawScreen(size[0]).bodyTop, size[0])
			x0, x1 := cols[0], cols[1]
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
		m, clipped, _ := menuBoard(t, 100, 31, nil)
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

// TestCNeverClaimsACopyThatFailed: when no screen of the person's had
// input lately, c copies nothing and says why, never copied.
func TestCNeverClaimsACopyThatFailed(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tm := &screenTmux{clients: []string{"/dev/pts/4\t" + activeAt(now.Add(-time.Minute)) + "\t/dev/pts/4\tmatchblox"}}
	m, _, _ := menuBoard(t, 80, 31, func(m *Model) {
		chooseCard("flint-auth")(m)
		m.opt.Clip, m.opt.Pane, m.opt.Tmux = nil, "%1", tm.run
		m.opt.Now = func() time.Time { return now }
	})
	m = pressOn(m, "c")
	if len(tm.loads) != 0 || m.noticeText != "not copied: "+errNoScreen.Error() {
		t.Errorf("c loaded %q and said %q", tm.loads, m.noticeText)
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
	next, cmd := key(m, "ctrl+o")
	fed(next, cmd)
	if want := "tmux switch-client -t " + paneNamed(boardFixture(t), "flint-auth"); strings.Join(ran, " ") != want {
		t.Errorf("ctrl+o after the toast ran %q, want %q", ran, want)
	}

	if ids := boardDoorIDs(m); len(ids) == 0 || ids[0] != doors.CopyCheck {
		t.Fatalf("the doors line is %q, want copy-check first", ids)
	}
	m.noticeText, m.saying = "", nil // on the desk the notice sits on the foot until it ends
	if !strings.Contains(plainFrame(m), "! 1 setup") {
		t.Errorf("the list's foot does not name the check:\n%s", plainFrame(m))
	}
	m = pressOn(toDoors(t, m), "enter")
	if len(*clipped) != 2 {
		t.Fatalf("⏎ on the setup step copied %q", *clipped)
	}
	code := (*clipped)[1]
	if utf8.RuneCountInString(code) != 6 || strings.Trim(code, "abcdefghijklmnopqrstuvwxyz") != "" {
		t.Errorf("the code is %q, want 6 letters", code)
	}
	if got := noticeRow(m); got != m.pasteHere() {
		t.Errorf("the check says %q", got)
	}
	m = pressOn(toDoors(t, pressOn(m, "esc")), "enter")
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
	m = pressOn(m, "esc") // back to the chat from the setup step
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
		if row := m.footSays(); !strings.Contains(row, "Now paste here · esc cancels") { // a question: it waits on the foot
			t.Errorf("%s: an hour on, the check's row says %q", name, row)
		}
		if keys := keyLineOf(m); !strings.Contains(keys, "esc cancel") || strings.Contains(keys, "set up") {
			t.Errorf("%s: while the check waits the key line says %q", name, keys)
		}
		m = pressOn(m, "esc") // the chat takes its place back
		if m.copyCode != "" || strings.Contains(boardNoticeRow(m), "paste") || strings.Contains(keyLineOf(m), "cancel") {
			t.Errorf("%s: esc off the setup step keeps the check: code %q, row %q, keys %q", name, m.copyCode, boardNoticeRow(m), keyLineOf(m))
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
	m.tab = tabMachine
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
		got := strings.TrimSpace(lines[len(lines)-1])
		if m.desk() { // the bar: the mode word, then the menu's keys
			_, got, _ = strings.Cut(got, "  ")
			got = strings.TrimSpace(got)
		}
		if !strings.HasPrefix("in the menu · ↑↓ move · ⏎ do · esc close", got) || !strings.HasPrefix(got, "in the menu · ↑↓ move · ⏎ do") {
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
		m.opt.Run = func(argv []string) error { *ran = argv; return nil }
	})
	m.sawRelease = sawRelease
	return m, clipped
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
	m = onDoor(open(), 1).(Model)
	if f := plainFrame(m); !strings.Contains(f, "SET UP · 2 of 2") || !strings.Contains(f, "Install tmux") {
		t.Errorf("the next door is not named:\n%s", plainFrame(m))
	}
	if m = pressOn(m, "enter"); m.pending == nil || m.pending.door != doors.Tmux || m.copyCode != "" {
		t.Errorf("the next door then ⏎ opened %v, code %q; want the tmux door", m.pending, m.copyCode)
	}

	// Naming another door during the check ends the check.
	if m = onDoor(pressOn(open(), "enter"), 1).(Model).copyCheckOn(); m.copyCode != "" || strings.Contains(boardNoticeRow(m), "paste") || strings.Contains(keyLineOf(m), "cancel") {
		t.Errorf("another door keeps the check: code %q, row %q, keys %q", m.copyCode, boardNoticeRow(m), keyLineOf(m))
	}
}

func TestTheFirstCSaysWhatItCopied(t *testing.T) {
	m, clipped, clock := uncheckedBoard(t, 80, 31, t.TempDir(), chooseCard("flint-auth"))
	m.viewPrefs.Told = nil
	m = pressOn(m, "c")
	if got := noticeRow(m); got != "c copies now: the last message, whole." || len(*clipped) != 1 {
		t.Fatalf("the first c says %q, copied %q", got, *clipped)
	}
	*clock = clock.Add(noticeFor)
	if m = mouse(t, m, noticeMsg{}); noticeRow(m) != "copied: last message of flint-auth · not checked here" {
		t.Errorf("after the key's news: %q", noticeRow(m))
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

func TestTheNamedDoorResetsWhenTheDoorsChange(t *testing.T) {
	m, _, _ := uncheckedBoard(t, 80, 31, t.TempDir(), func(m *Model) {
		m.doors = []doors.Door{{ID: doors.Tmux, Title: "Install tmux", Term: []string{"sh", "-c", "true"}}}
	})
	m = onDoor(toDoors(t, m), 1).(Model)
	if f := plainFrame(m); !strings.Contains(f, "SET UP · 2 of 2") || !strings.Contains(f, "Install tmux") {
		t.Fatalf("no second door is named:\n%s", plainFrame(m))
	}
	m.doors = append(m.doors, doors.Door{ID: doors.Guide, Title: "Open a guide session"})
	if f := plainFrame(m); !strings.Contains(f, "SET UP · 1 of 3") || !strings.Contains(f, "Check that copy reaches you") {
		t.Errorf("a new list of doors keeps the old door:\n%s", plainFrame(m))
	}
}

// rowOn is the cell of name's row in the list: only a swipe there chooses.
func rowOn(t *testing.T, m Model, name string) (int, int) {
	t.Helper()
	listW := m.sideBoxAt(max(m.width, minWidth)).listW
	for y, l := range strings.Split(plainFrame(m), "\n") {
		if i := strings.Index(l, name); i >= 0 && ansi.StringWidth(l[:i]) < listW {
			return ansi.StringWidth(l[:i]), y
		}
	}
	t.Fatalf("no row of %s in the list:\n%s", name, plainFrame(m))
	return 0, 0
}

func TestASwipeMovesTheCardAsTheArrowsDo(t *testing.T) {
	for _, size := range [][2]int{{40, 34}, {100, 31}} {
		name := fmt.Sprintf("%dx%d", size[0], size[1])
		keys, _, _ := menuBoard(t, size[0], size[1], chooseCard("spark-search"))
		swipe, _, _ := menuBoard(t, size[0], size[1], chooseCard("spark-search"))
		for _, step := range []struct {
			key string
			b   tea.MouseButton
		}{{"down", tea.MouseWheelDown}, {"down", tea.MouseWheelDown}, {"up", tea.MouseWheelUp}} {
			keys = pressOn(keys, step.key)
			x, y := rowOn(t, swipe, chosenName(t, swipe))
			swipe = mouse(t, swipe, tea.MouseWheelMsg{X: x, Y: y, Button: step.b})
			if got, want := chosenName(t, swipe), chosenName(t, keys); got != want {
				t.Errorf("%s: a swipe %s chose %s, the arrow %s", name, step.key, got, want)
			}
		}
		if chosenName(t, keys) == "spark-search" {
			t.Fatalf("%s: the arrows never moved: the test proves nothing", name)
		}
	}

	// A step drops a marked option, as an arrow does.
	m, _, clock := menuBoard(t, 80, 31, chooseCard("flint-auth"))
	*clock = clock.Add(time.Second)
	m = pressOn(m, "1")
	if armed, _ := m.chatMarks(matchNamed(t, m, "flint-auth")); armed != 1 {
		t.Fatalf("1 marked %d", armed)
	}
	x, y := rowOn(t, m, "flint-auth")
	m = mouse(t, m, tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelUp}) // flint-auth is the column's last
	if armed, _ := m.chatMarks(matchNamed(t, m, "flint-auth")); armed != 0 {
		t.Errorf("a swipe kept the mark %d", armed)
	}
}
