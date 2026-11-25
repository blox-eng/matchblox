package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// sideShot is the demo machine at w by h on the Sessions tab, as the
// console opens it: the list and the chat. clipped is what it copies,
// ran what it runs.
func sideShot(t *testing.T, w, h int) (m Model, clipped *[]string, ran *[][]string) {
	t.Helper()
	return sideShotOf(t, screensState(t), w, h)
}

// sideShotOf is sideShot on st.
func sideShotOf(t *testing.T, st proto.State, w, h int) (m Model, clipped *[]string, ran *[][]string) {
	t.Helper()
	clipped, ran = &[]string{}, &[][]string{}
	m = New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt },
		Clip: func(s string) { *clipped = append(*clipped, s) },
		Run:  func(argv []string) error { *ran = append(*ran, argv); return nil }})
	m.viewPrefs.CopyCheckedPhone, m.viewPrefs.CopyCheckedDesk = true, true // the copy check is the board's first-run door
	m.viewPrefs.Told = []string{"copy", "chain", "digit"}
	m = upd(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = upd(t, m, stateMsg(st))
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
		if y < bodyTop(m) {
			continue // the header and the tabs, or the desk's head row and its rule
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
	return strings.Split(plainFrame(m), "\n")[bodyTop(m)+i]
}

// bodyTop is the frame's first line of the body: under the header, the
// tabs and a hairline; on the desk under the head row and its rule.
func bodyTop(m Model) int { return m.draw(max(m.width, minWidth)).bodyTop }

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
	if keys = keyLineOf(pressOn(m, "a")); keys != "AGENT  spark-docs" { // the way back ends: the agent's line has the keys
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
	before := ansi.Truncate(bodyLine(m, head.y-bodyTop(m)), listWide, "")
	mark := point{ansi.StringWidth(before[:strings.Index(before, "−")]), head.y}
	m = tapOn(t, m, mark)
	folded := ansi.Truncate(bodyLine(m, head.y-bodyTop(m)), listWide, "")
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
	foot := bodyTop(m) + m.bodyRows() - 1
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
	m = pressOn(m, "esc")
	bare := m
	bare.saying = nil // the toast of the dismiss is over the chat's top
	if m.sidePlace != placeChat || ansi.Cut(bodyLine(bare, 0), listWide+1, 160) != chat {
		t.Fatalf("esc did not give the place back to the chat: %q", bodyLine(bare, 0))
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

// footLine is the list's foot, and its line on the frame.
func footLine(m Model) (string, int) {
	y := bodyTop(m) + m.bodyRows() - 1
	return strings.Split(plainFrame(m), "\n")[y], y
}

func TestNewSessionOpensInTheChatsPlace(t *testing.T) {
	st := screensState(t)
	st.Agents = []string{"claude", "codex", "opencode"}
	m, _, _ := sideShotOf(t, st, 160, 38)
	chat := ansi.Cut(bodyLine(m, 0), listWide+1, 160)
	m = tapOn(t, m, point{2, bodyTop(m) + m.bodyRows() - 1}) // + new
	if m.form == nil {
		t.Fatalf("+ new opened no form: flash %q", m.flash)
	}
	frame := plainFrame(m)
	if !strings.Contains(frame, "New session") {
		t.Fatalf("no form:\n%s", frame)
	}
	if top := bodyLine(m, 0); !strings.HasPrefix(ansi.Cut(top, listWide+1, 160), "╭") || !strings.Contains(top, "NEEDS YOU") {
		t.Fatalf("the form is not in the chat's place, beside the list: %q", top)
	}
	for i := range m.bodyRows() {
		if l := bodyLine(m, i); strings.Contains(ansi.Truncate(l, listWide+1, ""), "New session") {
			t.Fatalf("the form covers the list on line %d: %q", i, l)
		}
	}
	for range 2 { // the first esc closes where's list of repositories
		m = pressOn(m, "esc")
	}
	if m.form != nil || ansi.Cut(bodyLine(m, 0), listWide+1, 160) != chat {
		t.Fatalf("esc did not give the place back to the chat: %q", bodyLine(m, 0))
	}
}

func TestArrowsReachTheFootAndEnterOpensIt(t *testing.T) {
	st := screensState(t)
	st.Doors = []doors.Door{{ID: doors.Update, Title: "Update to v0.43.0", Why: "matchblox v0.43.0 is out. This machine runs v0.42.0.", Sum: "v0.43.0"}}
	st.Update = &state.UpdateView{Mode: "offer", Current: "v0.42.0", Latest: "v0.43.0"}
	m, _, _ := sideShotOf(t, st, 160, 38)
	m = pressOn(m, "esc") // the keys to the list
	for range len(m.sideWalk()) {
		m = pressOn(m, "down")
	}
	if keys := keyLineOf(m); !strings.Contains(keys, "+ new") {
		t.Fatalf("↓ past the last row: the key line %q names no foot part", keys)
	}
	if foot, _ := footLine(m); !strings.HasPrefix(foot, "▌+ new") {
		t.Fatalf("the chosen foot part is not marked: %q", foot)
	}
	if m = pressOn(m, "up"); strings.Contains(keyLineOf(m), "+ new") {
		t.Fatal("↑ from the foot stayed on it")
	}
	m = pressOn(m, "down", "down")
	if keys := keyLineOf(m); !strings.Contains(keys, "tips") {
		t.Fatalf("the second foot part: %q", keys)
	}
	m = pressOn(m, "enter")
	if head := ansi.Cut(bodyLine(m, 0), listWide+1, 160); !strings.HasPrefix(strings.TrimSpace(head), "TIPS · ") {
		t.Fatalf("⏎ on the tips: %q", head)
	}
	m = pressOn(m, "esc", "esc")
	for range len(m.sideWalk()) + 3 {
		m = pressOn(m, "down")
	}
	if keys := keyLineOf(m); !strings.Contains(keys, "setup") {
		t.Fatalf("the last foot part: %q", keys)
	}
	m = pressOn(m, "enter")
	chat := ansi.Strip(strings.Join(strings.Split(plainFrame(m), "\n")[bodyTop(m):bodyTop(m)+3], "\n"))
	if !strings.Contains(chat, "SET UP") || !strings.Contains(chat, "Update to v0.43.0") {
		t.Fatalf("⏎ on the setup step:\n%s", chat)
	}
}

func TestExitedAgentsShowWithE(t *testing.T) {
	st := screensState(t)
	st.Exited = []sample.Exited{{PID: 4242, Parent: 1, ParentComm: "systemd"}}
	m, _, _ := sideShotOf(t, st, 160, 38)
	if foot, _ := footLine(m); !strings.Contains(foot, "1 exited") {
		t.Fatalf("the foot %q does not count the exited agent", foot)
	}
	idle := func(m Model) string {
		for _, l := range strings.Split(listOf(m), "\n") {
			if strings.Contains(l, "IDLE") || strings.Contains(l, "WORKING") {
				return strings.TrimSpace(l)
			}
		}
		return ""
	}
	head := idle(m)
	m = pressOn(m, "esc", "e")
	if line := m.lines[m.chosenSide()]; line != "" {
		t.Fatalf("e typed %q into the line", line)
	}
	list := listOf(m)
	if !strings.Contains(list, "4242") || !strings.Contains(list, "exited") {
		t.Fatalf("e did not show the exited agent:\n%s", list)
	}
	if foot, _ := footLine(m); strings.Contains(foot, "exited") {
		t.Fatalf("the foot %q counts what the list shows", foot)
	}
	if idle(m) != head && !strings.HasPrefix(idle(m), "IDLE · 0") {
		t.Fatalf("the head %q counts the exited agent (was %q)", idle(m), head)
	}
	if m = pressOn(m, "e"); strings.Contains(listOf(m), "4242") {
		t.Fatal("a second e left the exited agent shown")
	}
}

func TestListChoiceSurvivesAResize(t *testing.T) {
	m, _, _ := sideShot(t, 132, 32)
	m = pressOn(m, "ctrl+l")
	m = upd(t, m, tea.WindowSizeMsg{Width: 160, Height: 38})
	if m.listShown() {
		t.Fatal("a wider desk showed the list the owner hid")
	}
	m = upd(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
	if m.listShown() {
		t.Fatal("a short screen showed the list the owner hid")
	}
	m = upd(t, m, tea.WindowSizeMsg{Width: 132, Height: 32})
	if m.listShown() {
		t.Fatal("back on the desk the list the owner hid shows")
	}
}

func TestANoticeLeavesTheChatLineAndTheFoot(t *testing.T) {
	m, _, _ := sideShot(t, 160, 38)
	chosen := chosenSideName(m)
	m = pressOn(m, "right") // no job in the line: a notice
	if m.noticeText == "" {
		t.Fatal("no notice")
	}
	foot, y := footLine(m)
	// On the desk the notice sits on the list's foot, as the mockup draws
	// it, and the chat's line stays (owner, 2026-11-24).
	if strings.Contains(foot, "+ new") || !strings.Contains(foot, "⏎ send to") {
		t.Fatalf("the notice is not on the list's foot, or covers the chat line: %q", foot)
	}
	at, x := -1, -1
	for i, l := range strings.Split(plainFrame(m), "\n") {
		if c := strings.Index(l, "the line takes text"); c >= 0 {
			at, x = i, ansi.StringWidth(l[:c])
		}
	}
	if at < 0 || at > y {
		t.Fatalf("the notice is at line %d, the foot at %d", at, y)
	}
	m = tapOn(t, m, point{x + 3, at})
	if chosenSideName(m) != chosen || len(m.sideFold) != 0 {
		t.Fatalf("a tap on the notice chose %q, folds %v", chosenSideName(m), m.sideFold)
	}
}

func TestTwoWaitsOfOneNameEachTakeTheirTap(t *testing.T) {
	st := screensState(t)
	for i, s := range st.Sessions {
		if s.Name == "spark-docs" {
			st.Sessions[i].Name = "flint-auth"
		}
	}
	m, _, _ := sideShotOf(t, st, 160, 38)
	m = pressOn(m, "ctrl+l")
	sc := m.draw(160)
	if len(sc.names) < 2 || sc.namePanes[0] == sc.namePanes[1] {
		t.Fatalf("names %v panes %v", sc.names, sc.namePanes)
	}
	m = tapOn(t, m, point{sc.names[1].x0 + 1, 0})
	if m.chosenSide() != sc.namePanes[1] {
		t.Fatalf("a tap on the second name chose %v, want %v", m.chosenSide(), sc.namePanes[1])
	}
}

func TestCtrlOWithNoPaneSaysSoAndNoWayBack(t *testing.T) {
	st := screensState(t)
	for i, s := range st.Sessions {
		if s.Name == "spark-docs" {
			st.Sessions[i].Pane = ""
		}
	}
	m, _, ran := sideShotOf(t, st, 160, 38)
	m = tapOn(t, m, listRow(t, m, "spark-docs"))
	m = pressOn(m, "ctrl+o")
	if len(*ran) != 0 || strings.HasPrefix(keyLineOf(m), "went into") {
		t.Fatalf("ran %v, key line %q", *ran, keyLineOf(m))
	}
}

// formShot is the demo machine with its agents at w by h, New session
// open in the chat's place from the foot.
func formShot(t *testing.T, w, h int) Model {
	t.Helper()
	st := screensState(t)
	st.Agents = []string{"claude", "codex", "opencode"}
	m, _, _ := sideShotOf(t, st, w, h)
	if !m.listShown() { // a short screen starts on the chat
		m = pressOn(m, "ctrl+l")
	}
	_, y := footLine(m)
	if m = tapOn(t, m, point{2, y}); m.form == nil {
		t.Fatalf("+ new opened no form: flash %q", m.flash)
	}
	return m
}

func TestRightClickOnTheListWhileAFormIsOpenOpensNoMenu(t *testing.T) {
	m := formShot(t, 160, 38)
	chosen := m.chosenSide()
	at := listRow(t, m, "spark-docs")
	m = mouse(t, m, tea.MouseClickMsg{X: at.x, Y: at.y, Button: tea.MouseRight})
	if m.menu != nil || m.chosenSide() != chosen || m.form == nil {
		t.Fatalf("a right-click beside the form: menu %v, chosen %v (was %v), form %v", m.menu != nil, m.chosenSide(), chosen, m.form != nil)
	}
	for range 2 {
		m = pressOn(m, "esc")
	}
	if m.form != nil || m.menu != nil {
		t.Fatalf("after esc: form %v, menu %v", m.form != nil, m.menu != nil)
	}
}

func TestSessionKeysOnTheFootSaySo(t *testing.T) {
	m, _, _ := sideShot(t, 160, 38)
	m = pressOn(m, "esc")
	for range len(m.sideWalk()) {
		m = pressOn(m, "down")
	}
	if m.footAt == 0 {
		t.Fatal("no foot part chosen")
	}
	m.tell("copy")
	for _, k := range []string{"z", "c", "g", "m"} {
		m.noticeText = ""
		m = pressOn(m, k)
		if m.menu != nil || m.room != nil || m.boardChain != "" || hasAct(boardFake(m), "park:"+paneNamed(screensState(t), chosenSideName(m))) {
			t.Fatalf("%s on the foot acted on the hidden session", k)
		}
		if !strings.Contains(m.noticeText, m.keys.side.Up.Help().Key) {
			t.Fatalf("%s on the foot: notice %q", k, m.noticeText)
		}
	}
}

func TestFormInTheChatsPlaceFitsAShortScreen(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {60, 20}} {
		m := formShot(t, size[0], size[1])
		sc := m.draw(size[0])
		last := sc.boxAt + len(sc.boxRows) - 1
		if last >= 3+m.bodyRows() {
			t.Fatalf("%dx%d: the form's %d rows run past the body's %d", size[0], size[1], len(sc.boxRows), m.bodyRows())
		}
		foot := ansi.Strip(sc.lines[last])
		if len(sc.boxKeys) == 0 || !strings.Contains(foot, "esc") {
			t.Fatalf("%dx%d: the form's key row %q is not on the screen", size[0], size[1], foot)
		}
		for tap := range 2 { // the first esc closes where's list
			sc := m.draw(size[0])
			i := slices.IndexFunc(sc.boxKeys, func(s span) bool { return s.press == m.keys.form.Close.Keys()[0] })
			if i < 0 {
				t.Fatalf("%dx%d: tap %d: no esc on the form's key row: %+v", size[0], size[1], tap, sc.boxKeys)
			}
			m = tapOn(t, m, point{sc.boxX + sc.boxKeys[i].x0, sc.boxAt + len(sc.boxRows) - 1})
		}
		if m.form != nil {
			t.Fatalf("%dx%d: two taps on the form's esc left it open", size[0], size[1])
		}
	}
}
