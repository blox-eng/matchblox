package app

import (
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// newerConsole is sideConsole with a newer release installed: R starts
// the console again from the list.
func newerConsole(t *testing.T) Model {
	t.Helper()
	m, _ := sideConsole(t, boardState(boardMatch("hearth-api", time.Hour)))
	m.newer, m.startsAgain = "R start again · service v0.46.0, console v0.45.0", true
	m.opt.Restart = func() error { return nil }
	return m
}

// TestRInAnEmptyLineTypes: with a newer release installed, R in the empty
// line types. A message that starts with R never starts the console again.
func TestRInAnEmptyLineTypes(t *testing.T) {
	m := typeLine(newerConsole(t), "Refactor")
	if m.restarting || m.quitting {
		t.Fatal("R in the line started the console again")
	}
	if got := m.lines[m.chosenSide()]; got != "Refactor" {
		t.Fatalf("the line holds %q", got)
	}
	m.startsAgain = false
	m.setLine(m.chosenSide(), "")
	if m = pressOn(m, "R"); m.lines[m.chosenSide()] != "R" {
		t.Fatalf("R with no start again: the line holds %q", m.lines[m.chosenSide()])
	}
}

// TestRInTheListStartsAgain: in the list R starts the console again; a
// release that cannot start again says how to reopen.
func TestRInTheListStartsAgain(t *testing.T) {
	m := newerConsole(t)
	m.toList()
	if m = pressOn(m, "R"); !m.restarting {
		t.Fatal("R in the list did not start the console again")
	}
	m = newerConsole(t)
	m.startsAgain = false
	m.newer = "quit and reopen · service v0.46.0, console v0.45.0"
	m.toList()
	if m = pressOn(m, "R"); !strings.Contains(m.noticeText, "quit and reopen") {
		t.Fatalf("R in the list with no start again: notice %q", m.noticeText)
	}
}

// TestNoMainLetterActsFromTheLine: every letter of the main keys types in
// the empty line. None quits, starts again, sends or changes the tab.
func TestNoMainLetterActsFromTheLine(t *testing.T) {
	for _, b := range defaultKeys().main.bindings() {
		for _, k := range b.Keys() {
			r := []rune(k)
			if len(r) != 1 || !unicode.IsLetter(r[0]) {
				continue
			}
			m := newerConsole(t)
			tab := m.tab
			m = pressOn(m, k)
			if m.quitting || m.restarting || m.tab != tab || len(boardFake(m).acts()) != 0 {
				t.Errorf("%s (%s) acted from the line: quitting %v, tab %v, acts %+v", k, b.Help().Desc, m.quitting, m.tab, boardFake(m).acts())
			}
			if got := m.lines[m.chosenSide()]; got != k {
				t.Errorf("%s (%s): the line holds %q", k, b.Help().Desc, got)
			}
		}
	}
}

// TestATabGoesAsAPaste: message: refuses a tab, and paste: keeps it, so a
// one-line text with a tab goes as a paste.
func TestATabGoesAsAPaste(t *testing.T) {
	m, _ := sideConsole(t, boardState(boardMatch("flint-auth", time.Hour)))
	next, _ := m.Update(tea.PasteMsg{Content: "a\tb"})
	m = pressOn(next.(Model), "enter")
	if acts := messageActs(boardFake(m)); len(acts) != 1 || acts[0].RecID != proto.PastePrefix+"%flint-auth" || acts[0].Text != "a\tb" {
		t.Fatalf("sent %+v", acts)
	}
}

// TestLostMessagesComeBackInSendOrder: two messages with no answer come
// back to the line in the order they went.
func TestLostMessagesComeBackInSendOrder(t *testing.T) {
	for range 20 {
		m, _ := sideConsole(t, boardState(boardMatch("flint-auth", time.Hour)))
		for _, w := range []string{"one", "two", "three", "four"} {
			m = pressOn(typeLine(m, w), "enter")
		}
		next, _ := m.Update(lostMsg{err: errors.New("broken pipe"), conn: m.conn})
		m = next.(Model)
		if got := m.lines[m.chosenSide()]; got != "one two three four" {
			t.Fatalf("the line holds %q", got)
		}
	}
}

// TestASkippedAnswerIsNotSent: a guard that skips the answer's act is a
// refusal. The digits come back, and the notice says why.
func TestASkippedAnswerIsNotSent(t *testing.T) {
	m, _, ws := peerSide(t, hostState(), askingPeer())
	m = pressOn(m, "1", "1")
	s := m.all.sessions[slices.IndexFunc(m.all.sessions, func(s sample.Session) bool { return s.Host == "ws-2" })]
	next, cmd := m.Update(peerMsg{host: "ws-2", conn: ws, msg: resultMsg{ActID: "a" + strconv.Itoa(m.acts), Skipped: []string{"the pane shows another question"}}})
	drain(cmd)
	m = next.(Model)
	if _, sent := m.chatMarks(s); sent != 0 || !strings.Contains(m.noticeText, "the pane shows another question") {
		t.Fatalf("after the skip: sent %d, notice %q", sent, m.noticeText)
	}

	h, _ := sideConsole(t, boardState(askingMatch("flint-auth", "Keep the token", "Refresh the token")))
	h = pickSide(t, h, "flint-auth")
	h.peekAt = h.now().Add(-peekDead)
	h = pressOn(h, "1", "1")
	hs := matchNamed(t, h, "flint-auth")
	if _, sent := h.chatMarks(hs); sent != 1 {
		t.Fatalf("home: sent %d, want 1", sent)
	}
	next, cmd = h.Update(resultMsg{ActID: keyActID + hs.Pane, Skipped: []string{"the pane shows another question"}})
	drain(cmd)
	h = next.(Model)
	if _, sent := h.chatMarks(hs); sent != 0 || !strings.Contains(h.noticeText, "the pane shows another question") {
		t.Fatalf("home after the skip: sent %d, notice %q", sent, h.noticeText)
	}
}

// TestALostPeerForgetsTheAnswerInFlight: the answer's act has no answer
// when the peer's conn goes. The digits come back, and the notice says so.
func TestALostPeerForgetsTheAnswerInFlight(t *testing.T) {
	m, _, ws := peerSide(t, hostState(), askingPeer())
	m = pressOn(m, "1", "1")
	s := m.all.sessions[slices.IndexFunc(m.all.sessions, func(s sample.Session) bool { return s.Host == "ws-2" })]
	next, cmd := m.Update(peerMsg{host: "ws-2", conn: ws, msg: lostMsg{err: io.EOF, conn: ws}})
	drain(cmd)
	m = next.(Model)
	if _, sent := m.chatMarks(s); sent != 0 || m.peekSentAct != "" || !strings.Contains(m.noticeText, "ws-2") {
		t.Fatalf("after the loss: sent %d, act %q, notice %q", sent, m.peekSentAct, m.noticeText)
	}
}

// TestEscThenALetterFromPocketTypesWithADesk: with a desk attached too, an
// Esc and a quick letter from the app (its key carries the app's mark)
// type the letter.
func TestEscThenALetterFromPocketTypesWithADesk(t *testing.T) {
	m, _, _ := sideShot(t, 160, 38)
	pocketOnly(&m)
	m.viewers += ",/dev/pts/3:200@desk"
	if m.onPocket() {
		t.Fatal("a desk is attached: not on Pocket")
	}
	m.toLine()
	m = upd(t, m, uv.UnknownApcEvent(keycmd.MarkedPrefix+"/dev/pts/7\x1b\\"))
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'z', Mod: tea.ModAlt})
	nm := fed(next, cmd).(Model)
	if got := nm.lines[nm.chosenSide()]; got != "z" || len(boardFake(nm).acts()) != 0 {
		t.Fatalf("ESC z from the app: the line holds %q, acts %+v", got, boardFake(nm).acts())
	}
}

// TestADigitAfterTheAnswerSaysWhatItWaitsFor: a digit after the answer
// went says so, and what the digits wait for.
func TestADigitAfterTheAnswerSaysWhatItWaitsFor(t *testing.T) {
	m, _, _ := peerSide(t, hostState(), askingPeer())
	m = pressOn(m, "1", "1")
	m.noticeText = ""
	m = pressOn(m, "2")
	if !strings.Contains(m.noticeText, "1 went to flint-auth") || !strings.Contains(m.noticeText, "next question") {
		t.Fatalf("a digit after the answer: notice %q", m.noticeText)
	}
}

// wheelOn sends one wheel step at x, y.
func wheelOn(t *testing.T, m Model, x, y int, up bool) Model {
	t.Helper()
	b := tea.MouseWheelDown
	if up {
		b = tea.MouseWheelUp
	}
	next, cmd := m.Update(tea.MouseWheelMsg{X: x, Y: y, Button: b})
	return fed(next, cmd).(Model)
}

// wheelConsole is two sessions, flint-auth chosen with its screen watched
// and its second option marked, at w by h.
func wheelConsole(t *testing.T, w, h int) Model {
	t.Helper()
	m, _ := sideConsole(t, boardState(boardMatch("hearth-api", time.Hour), askingMatch("flint-auth", "Keep the token", "Refresh the token")))
	m = upd(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m = pickSide(t, m, "flint-auth")
	s := matchNamed(t, m, "flint-auth")
	m.watchHost, m.watching = s.Host, s.Pane
	m.screen = proto.Screen{Pane: s.Pane, At: m.now(), Text: "● The login uses a token."}
	m.startChatMark(s)
	m.peekAt = m.now().Add(-peekDead)
	m.toLine()
	return pressOn(m, "2")
}

// TestWheelOverTheChatKeepsTheSession: at 160 columns and on a phone, a
// wheel step over the chat keeps the chosen session and its mark, and
// scrolls the chat back through the pane's history.
func TestWheelOverTheChatKeepsTheSession(t *testing.T) {
	for _, size := range [][2]int{{160, 30}, {40, 34}} {
		m := wheelConsole(t, size[0], size[1])
		x := size[0] - 5
		m = wheelOn(t, m, x, 10, false)
		if got := chosenName(t, m); got != "flint-auth" {
			t.Errorf("%d: the wheel over the chat chose %s", size[0], got)
		}
		if armed, _ := m.chatMarks(matchNamed(t, m, "flint-auth")); armed != 2 {
			t.Errorf("%d: the wheel over the chat dropped the mark: %d", size[0], armed)
		}
		if got := sent(m); len(got) != 1 || got[0].RecID != proto.KeysPrefix+"%flint-auth" || got[0].Text != proto.WheelDown {
			t.Errorf("%d: the wheel over the chat sent %+v", size[0], got)
		}
	}
}

// TestWheelOverTheChatWithNoScreenSaysSo: a chat with no screen to scroll
// says so, and chooses nothing.
func TestWheelOverTheChatWithNoScreenSaysSo(t *testing.T) {
	m := wheelConsole(t, 160, 30)
	m.screen = proto.Screen{}
	m.noticeText = ""
	m = wheelOn(t, m, 150, 10, false)
	if got := chosenName(t, m); got != "flint-auth" || len(sent(m)) != 0 || m.noticeText == "" {
		t.Fatalf("chose %s, sent %+v, notice %q", got, sent(m), m.noticeText)
	}
}

// TestWheelOverTheListChooses: a wheel step over the list's columns
// chooses the session above or below, as ↑ and ↓ do.
func TestWheelOverTheListChooses(t *testing.T) {
	for _, size := range [][2]int{{160, 30}, {40, 34}} {
		m := wheelConsole(t, size[0], size[1])
		m.toList()
		before := chosenName(t, m)
		m = wheelOn(t, m, 3, 6, false)
		if got := chosenName(t, m); got == before {
			t.Errorf("%d: the wheel over the list chose nothing new: %s", size[0], got)
		}
		if len(sent(m)) != 0 {
			t.Errorf("%d: the wheel over the list sent %+v", size[0], sent(m))
		}
	}
}

// TestPasteInTheRoomGoesToItsTalkLine: a paste while the room's talk line
// types goes to that line, as one line, and no session's line takes it.
func TestPasteInTheRoomGoesToItsTalkLine(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, _ := roomWith(t, st, "hearth-api")
	m.tab = tabSessions
	m, _ = send(t, m, "enter")
	if m.sidePlace != placeRoom || !m.room.typing {
		t.Fatalf("the room does not type: place %v, room %+v", m.sidePlace, m.room)
	}
	next, cmd := m.Update(tea.PasteMsg{Content: "ship the\nlogin fix"})
	drain(cmd)
	m = next.(Model)
	if m.room == nil || m.room.draft != "ship the login fix" {
		t.Fatalf("the talk line holds %+v", m.room)
	}
	if len(m.lines) != 0 {
		t.Fatalf("a session's line took the paste: %v", m.lines)
	}
}

// TestPasteInTipsSaysSo: the tips take no text. A paste there says so, and
// no line keeps it.
func TestPasteInTipsSaysSo(t *testing.T) {
	m, _ := sideConsole(t, boardState(boardMatch("flint-auth", time.Hour)))
	for _, p := range []sidePlace{placeTips, placeSetup, placeHost} {
		f := m
		f.openPlace(p)
		f.noticeText = ""
		next, cmd := f.Update(tea.PasteMsg{Content: "words"})
		drain(cmd)
		f = next.(Model)
		if len(f.lines) != 0 || !strings.Contains(f.noticeText, "no text · the paste is not kept") {
			t.Errorf("%v: lines %v, notice %q", p, f.lines, f.noticeText)
		}
	}
}

// TestAQuestionThatDoesNotFitIsCut: on a short chat the question keeps its
// first line and the options that fit, and says how many more it has.
func TestAQuestionThatDoesNotFitIsCut(t *testing.T) {
	m, _ := sideConsole(t, boardState(askingMatch("flint-auth", "Keep the token", "Refresh the token", "Drop the token", "Ask the owner")))
	m = pickSide(t, m, "flint-auth")
	full := plainLines(m.chatPanel(100, 30))
	top := slices.IndexFunc(full, func(l string) bool { return strings.TrimSpace(l) == "" }) // the head's lines end at the first blank
	if top < 1 || !strings.Contains(strings.Join(full, "\n"), "Which way do we fix the login?") {
		t.Fatalf("no head or no question on a tall chat:\n%s", strings.Join(full, "\n"))
	}
	h := top + 4 // the head's lines, the question, two lines for options, the line
	got := strings.Join(plainLines(m.chatPanel(100, h)), "\n")
	for _, want := range []string{"Which way do we fix the login?", "1. Keep the token", "+3 more"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in a chat %d high:\n%s", want, h, got)
		}
	}
	if lines := plainLines(m.chatPanel(100, h)); !strings.Contains(lines[len(lines)-1], "send to flint-auth") {
		t.Errorf("the line is not last:\n%s", got)
	}
}

// TestAPlaceKeepsTheChosenRowInView: tips taller than the chat's place
// keep the chosen tip in view, and say how many lines are cut.
func TestAPlaceKeepsTheChosenRowInView(t *testing.T) {
	m, _ := sideConsole(t, boardState(boardMatch("flint-auth", time.Hour)))
	for i := range 12 {
		m.recs = append(m.recs, advice.Rec{ID: "tip-" + strconv.Itoa(i), Level: "info", Title: "Tip number " + strconv.Itoa(i)})
	}
	m.openPlace(placeTips)
	m.sideTip = 10
	got := strings.Join(plainLines(m.chatPlace(80, 8)), "\n")
	for _, want := range []string{"TIPS · 12", "Tip number 10", "more"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in tips 8 high:\n%s", want, got)
		}
	}
	if lines := m.chatPlace(80, 8); len(lines) != 8 {
		t.Errorf("%d lines, want 8", len(lines))
	}
}

// TestTheRoomKeepsItsTalkLineInView: a room taller than the chat's place
// keeps the talk line in view while it types.
func TestTheRoomKeepsItsTalkLineInView(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, _ := roomWith(t, st, "hearth-api")
	m.tab = tabSessions
	m, _ = send(t, m, "enter")
	full := plainLines(m.roomPanel(100))
	h := 5
	if len(full) <= h {
		t.Fatalf("the room is %d lines: the test proves nothing", len(full))
	}
	got := strings.Join(plainLines(m.chatPlace(100, h)), "\n")
	if !strings.Contains(got, strings.TrimSpace(full[len(full)-1])) || !strings.Contains(got, "more") {
		t.Fatalf("the talk line %q is not in view:\n%s", strings.TrimSpace(full[len(full)-1]), got)
	}
}
