package app

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
)

// sideConsole is the console at 160 columns on st, with the keys of the
// list and the chat, on a clock the test moves: the first frame is
// peekDead ago. The once-notice of a digit is already told.
func sideConsole(t *testing.T, st proto.State) (Model, *time.Time) {
	t.Helper()
	clock := boardAt
	m, _ := loadedWith(t, 160, st)
	m.opt.Now = func() time.Time { return clock }
	m.tab, m.sideOn = tabSessions, true
	m.tell("digit")
	m.peekReached = false // the guard starts again on the test's clock
	next, _ := m.Update(noticeMsg{})
	clock = clock.Add(peekDead)
	return next.(Model), &clock
}

// askingMatch is a match that asks one question with the options given.
func askingMatch(name string, options ...string) sample.Session {
	s := boardMatch(name, time.Hour)
	q := said.Question{Question: "Which way do we fix the login?"}
	for _, o := range options {
		q.Options = append(q.Options, said.Option{Label: o})
	}
	s.Asks, s.Ask = true, &said.Ask{ID: "toolu_" + name, Questions: []said.Question{q}}
	return s
}

// pickSide makes the session named name the chosen one.
func pickSide(t *testing.T, m Model, name string) Model {
	t.Helper()
	i := slices.IndexFunc(m.all.sessions, func(s sample.Session) bool { return s.Name == name })
	if i < 0 {
		t.Fatalf("no session %s", name)
	}
	m.sideAt = paneOf(m.all.sessions[i])
	return m
}

// typeLine presses each character of text.
func typeLine(m Model, text string) Model {
	for _, r := range text {
		k := string(r)
		if r == ' ' {
			k = "space"
		}
		m = pressOn(m, k)
	}
	return m
}

func messageActs(f *fakeConn) []proto.Act {
	var out []proto.Act
	for _, a := range f.acts() {
		if strings.HasPrefix(a.RecID, "message:") {
			out = append(out, a)
		}
	}
	return out
}

func chatLineOf(m Model, w int) string {
	lines := plainLines(m.chatPanel(w, 12))
	return lines[len(lines)-1]
}

func TestLettersTypeIntoTheLine(t *testing.T) {
	m, _ := sideConsole(t, boardState(boardMatch("hearth-api", time.Hour)))
	m = typeLine(m, "hi jkqn")
	if got := m.lines[m.chosenSide()]; got != "hi jkqn" {
		t.Fatalf("the line holds %q", got)
	}
	if m.quitting || len(boardFake(m).acts()) != 0 {
		t.Fatalf("a letter acted: quitting %v, acts %+v", m.quitting, boardFake(m).acts())
	}
	if got := chatLineOf(m, 100); !strings.Contains(got, "› hi jkqn") || !strings.HasSuffix(strings.TrimRight(got, " "), "send to hearth-api") {
		t.Fatalf("line %q", got)
	}
	m = pressOn(m, "backspace")
	if got := m.lines[m.chosenSide()]; got != "hi jkq" {
		t.Fatalf("⌫ left %q", got)
	}
}

// TestEnterSendsTheLineToItsSession: the message goes on the conn of the
// session's own host, and the line empties.
func TestEnterSendsTheLineToItsSession(t *testing.T) {
	m, _ := sideConsole(t, boardState(boardMatch("hearth-api", time.Hour), boardMatch("flint-auth", time.Hour)))
	m = pickSide(t, m, "flint-auth")
	m = pressOn(typeLine(m, "fix it"), "enter")
	got := messageActs(boardFake(m))
	if len(got) != 1 || got[0].RecID != "message:%flint-auth" || got[0].Text != "fix it" || got[0].Confirm != "y" {
		t.Fatalf("sent %+v", got)
	}
	if m.lines[m.chosenSide()] != "" {
		t.Fatalf("the line still holds %q", m.lines[m.chosenSide()])
	}

	home, ws := newFake(), newFake()
	var next tea.Model = New(Options{NoMotion: true, Conn: home, Peers: []Peer{{Host: "ws-2", Conn: ws}}})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 160, Height: 38})
	next, _ = next.Update(stateMsg(hostState()))
	next, _ = next.Update(peerMsg{host: "ws-2", conn: ws, msg: stateMsg(hostState(sess(4242, "flint-auth", "%7", nil)))})
	p := next.(Model)
	p.tab, p.sideOn = tabSessions, true
	p = pressOn(typeLine(p, "go on"), "enter")
	if got := messageActs(ws); len(got) != 1 || got[0].RecID != "message:%7" || got[0].Text != "go on" {
		t.Fatalf("ws-2 got %+v", ws.acts())
	}
	if got := messageActs(home); len(got) != 0 {
		t.Fatalf("the home conn got %+v", got)
	}
}

// TestPasteArrivesAsOnePiece: a paste with a newline stays one piece in
// the line and goes as one message; the service's refusal shows in the
// line, and the text comes back.
func TestPasteArrivesAsOnePiece(t *testing.T) {
	m, _ := sideConsole(t, boardState(boardMatch("flint-auth", time.Hour)))
	paste := "the first line\nthe second line"
	next, _ := m.Update(tea.PasteMsg{Content: paste})
	m = next.(Model)
	if got := m.lines[m.chosenSide()]; got != paste {
		t.Fatalf("the line holds %q", got)
	}
	m = pressOn(m, "enter")
	acts := messageActs(boardFake(m))
	if len(acts) != 1 || acts[0].Text != paste {
		t.Fatalf("sent %+v", acts)
	}
	id := "a" + strconv.Itoa(m.acts)
	next, _ = m.Update(resultMsg{ActID: id, Err: "the message has a control character"})
	m = next.(Model)
	if got := m.lines[m.chosenSide()]; got != paste {
		t.Fatalf("after the refusal the line holds %q", got)
	}
	if got := chatLineOf(m, 120); !strings.Contains(got, "not sent: the message has a control character") {
		t.Fatalf("line %q", got)
	}
}

// TestDigitMarksThenSends is done-when line 1: at 160 columns the owner
// chooses flint-auth, presses 2 twice, and the answer reaches it.
func TestDigitMarksThenSends(t *testing.T) {
	m, clock := sideConsole(t, boardState(boardMatch("hearth-api", time.Hour), askingMatch("flint-auth", "Keep the token", "Refresh the token")))
	m = pickSide(t, m, "flint-auth")
	next, _ := m.Update(noticeMsg{})
	m = next.(Model)
	*clock = clock.Add(peekDead)
	m = pressOn(m, "2")
	if armed, _ := m.chatMarks(matchNamed(t, m, "flint-auth")); armed != 2 || len(sent(m)) != 0 {
		t.Fatalf("the first 2 marked %d, sent %+v", armed, sent(m))
	}
	if got := chatText(m, 100, 14); !strings.Contains(got, "❯ 2. Refresh the token") {
		t.Fatalf("the chat shows no mark:\n%s", got)
	}
	m = pressOn(m, "2")
	got := sent(m)
	if len(got) != 1 || got[0].RecID != proto.KeysPrefix+"%flint-auth" || got[0].Text != proto.LitPrefix+"2" {
		t.Fatalf("sent %+v", got)
	}
	if m.lines[paneOf(matchNamed(t, m, "flint-auth"))] != "" {
		t.Fatal("a digit that answered typed into the line")
	}
}

func TestDigitWithNoQuestionTypes(t *testing.T) {
	m, _ := sideConsole(t, boardState(boardMatch("hearth-api", time.Hour)))
	m = pressOn(m, "2")
	if got := m.lines[m.chosenSide()]; got != "2" || len(sent(m)) != 0 {
		t.Fatalf("the line holds %q, sent %+v", got, sent(m))
	}
}

// TestDigitJustBeforeAQuestionTypes (Review Focus 5): a question that
// opens while the owner types takes no digit for peekDead: it types.
func TestDigitJustBeforeAQuestionTypes(t *testing.T) {
	m, _ := sideConsole(t, boardState(boardMatch("flint-auth", time.Hour)))
	next, _ := m.Update(stateMsg(boardState(askingMatch("flint-auth", "Keep the token", "Refresh the token"))))
	m = pressOn(next.(Model), "1")
	if armed, _ := m.chatMarks(matchNamed(t, m, "flint-auth")); armed != 0 || len(sent(m)) != 0 {
		t.Fatalf("a digit on a question that just came marked %d, sent %+v", armed, sent(m))
	}
	if got := m.lines[m.chosenSide()]; got != "1" {
		t.Fatalf("the line holds %q", got)
	}
}

// TestArrowsChooseInDrawnOrder: ↑↓ walk the list as drawn, only with the
// line empty; j and k type.
func TestArrowsChooseInDrawnOrder(t *testing.T) {
	m := loginChain(t, nil)
	m.opt.Now = func() time.Time { return boardAt }
	m.tab, m.sideOn = tabSessions, true
	order := m.sideOrder()
	if len(order) != 3 {
		t.Fatalf("order %v", order)
	}
	for i := 1; i < len(order); i++ {
		if m = pressOn(m, "down"); m.chosenSide() != order[i] {
			t.Fatalf("↓ %d chose %v, want %v", i, m.chosenSide(), order[i])
		}
	}
	if m = pressOn(m, "down"); m.chosenSide() != order[2] {
		t.Fatalf("↓ past the last chose %v", m.chosenSide())
	}
	if m = pressOn(m, "up"); m.chosenSide() != order[1] {
		t.Fatalf("↑ chose %v, want %v", m.chosenSide(), order[1])
	}
	m = pressOn(m, "j", "k")
	if m.chosenSide() != order[1] || m.lines[order[1]] != "jk" {
		t.Fatalf("j k moved to %v, the line holds %q", m.chosenSide(), m.lines[order[1]])
	}
	if m = pressOn(m, "down"); m.chosenSide() != order[1] {
		t.Fatalf("↓ with text in the line moved to %v", m.chosenSide())
	}
}

func TestEscClearsAMarkThenMovesToTheList(t *testing.T) {
	m, _ := sideConsole(t, boardState(askingMatch("flint-auth", "Keep the token", "Refresh the token")))
	if got := m.sideKeyWords(); !strings.HasPrefix(got, "writing to flint-auth") {
		t.Fatalf("key words %q", got)
	}
	m = pressOn(m, "1")
	if armed, _ := m.chatMarks(matchNamed(t, m, "flint-auth")); armed != 1 {
		t.Fatalf("1 marked %d", armed)
	}
	m = pressOn(m, "esc")
	if armed, _ := m.chatMarks(matchNamed(t, m, "flint-auth")); armed != 0 || m.keysIn != focusLine {
		t.Fatalf("esc left the mark %d, keys in %v", armed, m.keysIn)
	}
	m = pressOn(m, "esc")
	if m.keysIn != focusList {
		t.Fatal("the second esc left the keys in the line")
	}
	got := m.sideKeyWords()
	if !strings.HasPrefix(got, "in the list") || strings.Count(got, " · ") > 5 {
		t.Fatalf("key words %q: the place, then at most five keys", got)
	}
	m = pressOn(m, "enter")
	if m.keysIn != focusLine || len(sent(m)) != 0 {
		t.Fatalf("⏎ in the list: keys in %v, sent %+v", m.keysIn, sent(m))
	}
}

// TestListKeys: in the list each key does its act and the keys stay
// there; another letter goes back to the line and is typed there.
func TestListKeys(t *testing.T) {
	for _, c := range []struct {
		key  string
		done func(Model) bool
	}{
		{"s", func(m Model) bool { return m.keysIn == focusList }}, // one host: Task 9 says so
		{"g", func(m Model) bool { return m.room != nil }},
		{"/", func(m Model) bool { return m.searching }},
		{"m", func(m Model) bool { return m.menu != nil }},
		{"c", func(m Model) bool { return strings.Contains(m.noticeText, "nothing to copy") }},
		{"z", func(m Model) bool { return hasAct(boardFake(m), "park:%flint-auth") }},
		{"N", func(m Model) bool { return m.form != nil || strings.HasPrefix(m.flash, "no agent to start here") }},
		{"tab", func(m Model) bool { return m.tab != tabSessions }},
	} {
		t.Run(c.key, func(t *testing.T) {
			m := loginChain(t, nil)
			m.tab, m.sideOn = tabSessions, true
			m = pickSide(t, m, "flint-auth")
			m.tell("copy")
			m = pressOn(m, "esc", c.key)
			if !c.done(m) {
				t.Fatalf("%s did nothing: notice %q flash %q", c.key, m.noticeText, m.flash)
			}
			if len(m.lines) != 0 && m.lines[m.chosenSide()] != "" {
				t.Fatalf("%s typed %q", c.key, m.lines[m.chosenSide()])
			}
		})
	}
	m := loginChain(t, nil)
	m.tab, m.sideOn = tabSessions, true
	m = pressOn(m, "esc", "x")
	if m.keysIn != focusLine || m.lines[m.chosenSide()] != "x" {
		t.Fatalf("x in the list: keys in %v, the line holds %q", m.keysIn, m.lines[m.chosenSide()])
	}
}

// TestLineKeepsItsSessionWhenTheListMoves (Review Focus 1): a row that
// moves under a half-typed line does not take the message.
func TestLineKeepsItsSessionWhenTheListMoves(t *testing.T) {
	a, b := boardMatch("flint-auth", time.Hour), boardMatch("hearth-api", 2*time.Hour)
	m, _ := sideConsole(t, boardState(a, b))
	first := m.chosenSide()
	m = typeLine(m, "half")
	other := a
	if paneOf(a) == first {
		other = b
	}
	st := boardState(a, b)
	for i := range st.Sessions {
		if st.Sessions[i].Name == other.Name {
			st.Sessions[i] = askingMatch(other.Name, "Yes", "No")
			st.Sessions[i].PID, st.Sessions[i].Started = other.PID, other.Started
		}
	}
	next, _ := m.Update(stateMsg(st))
	m = next.(Model)
	if m.sideOrder()[0] == first {
		t.Fatalf("the list did not move: %v", m.sideOrder())
	}
	m = pressOn(m, "enter")
	if got := messageActs(boardFake(m)); len(got) != 1 || got[0].RecID != "message:"+first.pane {
		t.Fatalf("sent %+v, want one message to %s", got, first.pane)
	}
}

// TestChosenSessionEndsKeepsTheDraft (Review Focus 4): the choice moves
// to the next session as drawn, and the line says the text is not sent.
func TestChosenSessionEndsKeepsTheDraft(t *testing.T) {
	ss := []sample.Session{boardMatch("hearth-api", time.Hour), boardMatch("flint-auth", 2*time.Hour), boardMatch("spark-docs", 3*time.Hour)}
	m, _ := sideConsole(t, boardState(ss...))
	order := m.sideOrder()
	gone := order[1]
	m.sideAt = gone
	m = typeLine(m, "half")
	var left []sample.Session
	for _, s := range ss {
		if paneOf(s) != gone {
			left = append(left, s)
		}
	}
	next, _ := m.Update(stateMsg(boardState(left...)))
	m = next.(Model)
	if m.chosenSide() != order[2] {
		t.Fatalf("chose %v, want the next as drawn %v", m.chosenSide(), order[2])
	}
	name := strings.TrimPrefix(gone.pane, "%")
	if got := chatLineOf(m, 120); !strings.Contains(got, name+" ended · the text is not sent") {
		t.Fatalf("line %q", got)
	}
	if m.lines[gone] != "half" || len(messageActs(boardFake(m))) != 0 {
		t.Fatalf("the draft %q, sent %+v", m.lines[gone], messageActs(boardFake(m)))
	}
}

func TestParkedLineIsOff(t *testing.T) {
	s := boardMatch("flint-auth", time.Hour)
	st := boardState(s)
	st.Holds = append(st.Holds, proto.Hold{SessionID: s.SessionID, Pane: s.Pane, Name: s.Name, By: holdByYou})
	m, _ := sideConsole(t, st)
	m = pressOn(m, "a", "enter")
	if m.lines[paneOf(s)] != "" || len(messageActs(boardFake(m))) != 0 {
		t.Fatalf("a parked line took %q, sent %+v", m.lines[paneOf(s)], messageActs(boardFake(m)))
	}
	if got := chatLineOf(m, 100); !strings.Contains(got, "parked: z lets it go") {
		t.Fatalf("line %q", got)
	}
	m = pressOn(m, "z")
	if !hasAct(boardFake(m), "park:%flint-auth") {
		t.Fatalf("z on the parked line sent %+v", boardFake(m).acts())
	}
}

// TestHeldByStokerOneReleases: the chat asks whether to let the session
// go, and 1, then 1 again, releases it.
func TestHeldByStokerOneReleases(t *testing.T) {
	s := boardMatch("flint-auth", time.Hour)
	st := boardState(s)
	st.Holds = append(st.Holds, proto.Hold{SessionID: s.SessionID, Pane: s.Pane, Name: s.Name, By: "stoker", Why: "not the priority"})
	m, _ := sideConsole(t, st)
	if got := chatText(m, 100, 12); !strings.Contains(got, "held by the stoker · not the priority") || !strings.Contains(got, "1. Let it go") {
		t.Fatalf("chat:\n%s", got)
	}
	m = pressOn(m, "1")
	if hasAct(boardFake(m), "release:%flint-auth") || !strings.Contains(chatText(m, 100, 12), "❯ 1. Let it go") {
		t.Fatalf("the first 1 sent %+v or marks nothing:\n%s", boardFake(m).acts(), chatText(m, 100, 12))
	}
	m = pressOn(m, "1")
	if !hasAct(boardFake(m), "release:%flint-auth") {
		t.Fatalf("1 1 sent %+v", boardFake(m).acts())
	}
}

// TestUnreachedHostKeepsTheText: the text stays, the line says why, and
// nothing retries.
func TestUnreachedHostKeepsTheText(t *testing.T) {
	home, ws := newFake(), newFake()
	var next tea.Model = New(Options{NoMotion: true, Conn: home, Peers: []Peer{{Host: "ws-2", Conn: ws}}})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 160, Height: 38})
	next, _ = next.Update(stateMsg(hostState()))
	next, _ = next.Update(peerMsg{host: "ws-2", conn: ws, msg: stateMsg(hostState(sess(4242, "flint-auth", "%7", nil)))})
	m := next.(Model)
	m.tab, m.sideOn = tabSessions, true
	m.peers[m.peerAt("ws-2")].lost = true
	m = pressOn(typeLine(m, "go on"), "enter")
	if got := m.lines[m.chosenSide()]; got != "go on" {
		t.Fatalf("the line holds %q", got)
	}
	if got := chatLineOf(m, 120); !strings.Contains(got, "not sent: ws-2 is not reached") {
		t.Fatalf("line %q", got)
	}
	next, cmd := m.Update(noticeMsg{})
	drain(cmd)
	_, cmd = next.Update(stateMsg(hostState()))
	drain(cmd)
	if got := append(messageActs(ws), messageActs(home)...); len(got) != 0 {
		t.Fatalf("sent %+v", got)
	}
}

func TestNoServiceSendsNothing(t *testing.T) {
	m, _ := sideConsole(t, boardState(boardMatch("flint-auth", time.Hour)))
	m.lost = true
	m = pressOn(typeLine(m, "go on"), "enter")
	if got := boardFake(m).acts(); len(got) != 0 {
		t.Fatalf("sent %+v", got)
	}
	if got := m.lines[m.chosenSide()]; got != "go on" {
		t.Fatalf("the line holds %q", got)
	}
	if got := chatLineOf(m, 120); !strings.Contains(got, "not sent: no service · it redials") {
		t.Fatalf("line %q", got)
	}
}

// TestAQuestionInThePaneIsNotDrawnTwice: the agent's own menu in the cut
// pane is the question; the chat draws only the mark under it. A pane
// that does not show the question gets the chat's own.
func TestAQuestionInThePaneIsNotDrawnTwice(t *testing.T) {
	m, _ := sideConsole(t, boardState(askingMatch("flint-auth", "Keep the token", "Refresh the token")))
	s := matchNamed(t, m, "flint-auth")
	if got := chatText(m, 100, 14); strings.Count(got, "Refresh the token") != 1 {
		t.Fatalf("no screen yet: the chat draws the question once:\n%s", got)
	}
	m.watchHost, m.watching = s.Host, s.Pane
	m.screen = proto.Screen{Pane: s.Pane, Text: "● Which way do we fix the login?\n❯ 1. Keep the token\n  2. Refresh the token"}
	if got := chatText(m, 100, 14); strings.Count(got, "Refresh the token") != 1 || strings.Count(got, "Which way do we fix the login?") != 1 {
		t.Fatalf("the pane's menu and the chat's question both show:\n%s", got)
	}
	m = pressOn(m, "2")
	if got := chatText(m, 100, 14); !strings.Contains(got, "❯ 2. Refresh the token · 2 again sends") {
		t.Fatalf("no mark line:\n%s", got)
	}
}
