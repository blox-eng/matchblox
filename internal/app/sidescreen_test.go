package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// sideShot is the demo machine at w by h on the Sessions tab, as the
// console opens it: the list and the chat. clipped is what it copies,
// ran what it runs.
func sideShot(t *testing.T, w, h int) (m Model, clipped *[]string, ran *[][]string) {
	t.Helper()
	clipped, ran = &[]string{}, &[][]string{}
	m = New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt },
		Clip: func(s string) { *clipped = append(*clipped, s) },
		Run:  func(argv []string) error { *ran = append(*ran, argv); return nil }})
	m.viewPrefs.CopyCheckedPhone, m.viewPrefs.CopyCheckedDesk = true, true // the copy check is the board's first-run door
	m.viewPrefs.Told = []string{"copy", "chain", "digit"}
	m = upd(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = upd(t, m, stateMsg(screensState(t)))
	m.splashDone = true
	return upd(t, m, noticeMsg{}), clipped, ran
}

// listRow is the cell of name's row in the list: the first cell of its
// name in the list's columns.
func listRow(t *testing.T, m Model, name string) point {
	t.Helper()
	lw := m.sideBoxAt(max(m.width, minWidth)).listW
	frame := plainFrame(m)
	for y, l := range strings.Split(frame, "\n") {
		if y < 3 {
			continue // the header and the tabs
		}
		if i := strings.Index(ansi.Truncate(l, lw, ""), name); i >= 0 {
			return point{ansi.StringWidth(l[:i]), y}
		}
	}
	t.Fatalf("no row of %s in the list:\n%s", name, frame)
	return point{}
}

func tapOn(t *testing.T, m Model, at point) Model {
	t.Helper()
	return mouse(t, m, tea.MouseClickMsg{X: at.x, Y: at.y, Button: tea.MouseLeft})
}

func chosenSideName(m Model) string {
	s, _ := m.sideChosen()
	return s.Name
}

// listOf is the list's columns of the body.
func listOf(m Model) string {
	lw := m.sideBoxAt(max(m.width, minWidth)).listW
	var out []string
	for i := range m.bodyRows() {
		out = append(out, ansi.Truncate(bodyLine(m, i), lw, ""))
	}
	return strings.Join(out, "\n")
}

// bodyLine is line i of the body as the screen draws it.
func bodyLine(m Model, i int) string {
	return strings.Split(plainFrame(m), "\n")[3+i]
}

func TestTapOnARowOpensItsChat(t *testing.T) {
	m, _, _ := sideShot(t, 160, 38)
	m = pressOn(m, "esc") // the keys in the list: a tap gives them to the line
	m = tapOn(t, m, listRow(t, m, "spark-docs"))
	if chosenSideName(m) != "spark-docs" || m.keysIn != focusLine {
		t.Fatalf("desk: chosen %q, keys in %v", chosenSideName(m), m.keysIn)
	}
	if head := ansi.Cut(bodyLine(m, 0), listWide+1, 160); !strings.HasPrefix(head, " spark-docs") {
		t.Fatalf("desk: the chat's head %q", head)
	}
	m, _, _ = sideShot(t, 44, 31)
	if !strings.Contains(plainFrame(m), "NEEDS YOU") || !m.inList() {
		t.Fatalf("a phone opens on the list:\n%s", plainFrame(m))
	}
	m = tapOn(t, m, listRow(t, m, "spark-docs"))
	if chosenSideName(m) != "spark-docs" || m.inList() || !strings.HasPrefix(bodyLine(m, 0), " ◂ spark-docs") {
		t.Fatalf("phone: chosen %q, in the list %v, head %q", chosenSideName(m), m.inList(), bodyLine(m, 0))
	}
}

func TestPhoneBackReturnsToTheList(t *testing.T) {
	m, _, _ := sideShot(t, 44, 31)
	m = tapOn(t, m, listRow(t, m, "flint-auth"))
	m = tapOn(t, m, point{1, 3}) // the ◂ at the head line's left
	if !m.inList() || !strings.Contains(plainFrame(m), "NEEDS YOU") {
		t.Fatalf("a tap on ◂ left the chat:\n%s", plainFrame(m))
	}
	if chosenSideName(m) != "flint-auth" {
		t.Fatalf("back in the list chose %q", chosenSideName(m))
	}
	m = pressOn(m, "enter")
	if m.inList() {
		t.Fatal("⏎ in the list opened no chat")
	}
	if !strings.Contains(keyLineOf(m), "◂ list") {
		t.Fatalf("the chat's key line %q names no way back", keyLineOf(m))
	}
	if m = pressOn(m, "left"); !m.inList() {
		t.Fatal("◂ left the chat on the screen")
	}
}

func TestTapOnAHeaderNameOpensItsChat(t *testing.T) {
	m, _, _ := sideShot(t, 132, 32)
	m = pressOn(m, "ctrl+l")
	if m.listShown() {
		t.Fatal("ctrl+l left the list on the screen")
	}
	names := m.headNames()
	head := strings.Split(plainFrame(m), "\n")[0]
	if len(names) < 2 || !strings.Contains(head, strings.Join(names, " · ")) {
		t.Fatalf("the header %q does not name the waits %q", head, names)
	}
	last := names[len(names)-1]
	m = tapOn(t, m, point{ansi.StringWidth(head[:strings.LastIndex(head, last)]) + 1, 0})
	if chosenSideName(m) != last || m.listShown() {
		t.Fatalf("a tap on %s chose %q, the list shown %v", last, chosenSideName(m), m.listShown())
	}
	if !strings.HasPrefix(bodyLine(m, 0), " "+last) {
		t.Fatalf("the chat's head %q", bodyLine(m, 0))
	}
	if m = pressOn(m, "ctrl+l"); !m.listShown() {
		t.Fatal("a second ctrl+l did not show the list")
	}
}

func TestCtrlOExpandsAndTheKeyLineSaysTheWayBack(t *testing.T) {
	m, _, ran := sideShot(t, 160, 38)
	m = tapOn(t, m, listRow(t, m, "spark-docs"))
	next, cmd := m.Update(keyPress("ctrl+o"))
	if next.(Model).pending != nil {
		t.Fatalf("ctrl+o waits for a confirm: %v", next.(Model).pending)
	}
	m = fed(next, cmd).(Model)
	if len(*ran) != 1 || strings.Join((*ran)[0], " ") != "tmux switch-client -t "+paneNamed(screensState(t), "spark-docs") {
		t.Fatalf("ran %v", *ran)
	}
	keys := keyLineOf(m)
	if !strings.HasPrefix(keys, "went into spark-docs") || !strings.Contains(keys, m.keys.view.Back.Help().Key+" back") {
		t.Fatalf("the key line %q does not say the way back", keys)
	}
	if keys = keyLineOf(pressOn(m, "a")); !strings.HasPrefix(keys, "writing to spark-docs") {
		t.Fatalf("after a key the key line %q", keys)
	}
}

func TestDragInTheChatSelectsChatTextOnly(t *testing.T) {
	m, clipped, _ := sideShot(t, 160, 38)
	m.sawRelease = true
	m = tapOn(t, m, listRow(t, m, "flint-auth"))
	m = letGo(t, m, listRow(t, m, "flint-auth"))
	m.watchCmd()
	m = upd(t, m, screenMsg{Pane: paneNamed(screensState(t), "flint-auth"), At: demoAt, Text: askScreen})
	lines := strings.Split(plainFrame(m), "\n")
	from := -1
	for y, l := range lines {
		if strings.Contains(ansi.Cut(l, listWide+1, 160), "Rotate") && from < 0 {
			from = y
		}
	}
	if from < 0 {
		t.Fatalf("no chat text to drag:\n%s", plainFrame(m))
	}
	at, to := point{listWide + 2, from - 4}, point{150, from + 1}
	m = drag(t, m, at, point{listWide + 8, from - 4}, to)
	m = letGo(t, m, to)
	if len(*clipped) != 1 {
		t.Fatalf("the drag copied %q", *clipped)
	}
	got := (*clipped)[0]
	if !strings.Contains(got, "Rotate") || !strings.Contains(got, "The refresh endpoint is in") {
		t.Fatalf("the drag copied %q, want the rows of the chat it covers", got)
	}
	for _, list := range []string{"NEEDS YOU", "kindle-web", "hearth-api", "│"} {
		if strings.Contains(got, list) {
			t.Errorf("the drag in the chat copied %q of the list: %q", list, got)
		}
	}
}

func TestTapOnAGroupsFoldMarkFoldsIt(t *testing.T) {
	m, _, _ := sideShot(t, 160, 38)
	head := listRow(t, m, "WORKING")
	before := ansi.Truncate(bodyLine(m, head.y-3), listWide, "")
	mark := point{ansi.StringWidth(before[:strings.Index(before, "−")]), head.y}
	m = tapOn(t, m, mark)
	folded := ansi.Truncate(bodyLine(m, head.y-3), listWide, "")
	if folded != strings.Replace(before, "−", "+", 1) {
		t.Fatalf("the folded head %q, was %q", folded, before)
	}
	if list := listOf(m); strings.Contains(list, "hearth-api") {
		t.Fatalf("a folded group draws its rows:\n%s", list)
	}
	for _, k := range m.sideWalk() {
		if k.pane == paneNamed(screensState(t), "hearth-api") {
			t.Fatal("↓ walks to a folded row")
		}
	}
	if m = tapOn(t, m, mark); !strings.Contains(listOf(m), "hearth-api") {
		t.Fatalf("a second tap left the group folded:\n%s", listOf(m))
	}
}

func TestFootItemOpensInTheChatsPlace(t *testing.T) {
	m, _, _ := sideShot(t, 160, 38)
	chat := ansi.Cut(bodyLine(m, 0), listWide+1, 160)
	foot := 3 + m.bodyRows() - 1
	footLine := strings.Split(plainFrame(m), "\n")[foot]
	tips := strings.Index(footLine, "✧")
	if tips < 0 {
		t.Fatalf("the foot %q has no tips", footLine)
	}
	m = tapOn(t, m, point{ansi.StringWidth(footLine[:tips]) + 1, foot})
	if head := ansi.Cut(bodyLine(m, 0), listWide+1, 160); !strings.HasPrefix(strings.TrimSpace(head), "TIPS · ") {
		t.Fatalf("the tips are not in the chat's place: %q", head)
	}
	if !strings.Contains(bodyLine(m, 0), "NEEDS YOU") {
		t.Fatal("the tips cover the list")
	}
	if keys := keyLineOf(m); !strings.HasPrefix(keys, "in the tips") || !strings.Contains(keys, "d dismiss") {
		t.Fatalf("the key line %q", keys)
	}
	n := len(m.recs)
	if m = pressOn(m, "d"); len(m.recs) != n-1 {
		t.Fatalf("d left %d of %d tips", len(m.recs), n)
	}
	if m = pressOn(m, "esc"); m.sidePlace != placeChat || ansi.Cut(bodyLine(m, 0), listWide+1, 160) != chat {
		t.Fatalf("esc did not give the place back to the chat: %q", bodyLine(m, 0))
	}
	if setup := strings.Index(footLine, "! "); setup >= 0 {
		m = tapOn(t, m, point{ansi.StringWidth(footLine[:setup]) + 1, foot})
		if head := ansi.Cut(bodyLine(m, 0), listWide+1, 160); !strings.HasPrefix(strings.TrimSpace(head), "SET UP") {
			t.Fatalf("the setup step is not in the chat's place: %q", head)
		}
		m = pressOn(m, "esc")
	}
	m = tapOn(t, m, point{2, foot})
	if m.form == nil && !strings.HasPrefix(m.flash, "no agent to start here") {
		t.Fatalf("+ new opened nothing: flash %q", m.flash)
	}
}

// TestSmallScreensKeepTheChatLine (Review Focus 3): at 80x24 and 60x20 no
// line is wider than the screen, the chat's line is on the screen, and the
// list does not cover the chat.
func TestSmallScreensKeepTheChatLine(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {60, 20}} {
		w, h := size[0], size[1]
		m, _, _ := sideShot(t, w, h)
		lines := strings.Split(m.render(), "\n")
		if len(lines) > h {
			t.Errorf("%dx%d: %d lines", w, h, len(lines))
		}
		for i, l := range lines {
			if lw := ansi.StringWidth(l); lw > w {
				t.Errorf("%dx%d: line %d is %d wide", w, h, i, lw)
			}
		}
		if !strings.Contains(plainFrame(m), "⏎ send to flint-auth") {
			t.Errorf("%dx%d: no chat line:\n%s", w, h, plainFrame(m))
		}
		box := m.sideBoxAt(w)
		if box.listW > 0 && box.chatW > 0 && (box.chatX < box.listW+1 || box.chatX+box.chatW > w) {
			t.Errorf("%dx%d: the list %d and the chat %d+%d overlap", w, h, box.listW, box.chatX, box.chatW)
		}
	}
}
