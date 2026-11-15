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
