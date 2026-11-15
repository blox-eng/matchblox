package app

import (
	"errors"
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
	m.tab = tabSessions
	m.toLine()
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
		if strings.HasPrefix(a.RecID, "message:") || strings.HasPrefix(a.RecID, proto.PastePrefix) {
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
	p.tab = tabSessions
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
	if len(acts) != 1 || acts[0].RecID != proto.PastePrefix+"%flint-auth" || acts[0].Text != paste || acts[0].Confirm != "y" {
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

// TestALongOrManyLineTextGoesAsAPaste: a text with a newline, or over
// 500 characters, goes as paste:; a short line goes as message:.
func TestALongOrManyLineTextGoesAsAPaste(t *testing.T) {
	for text, want := range map[string]string{
		"fix it":                   "message:",
		strings.Repeat("ü", 500):   "message:",
		strings.Repeat("ü", 501):   proto.PastePrefix,
		"the first line\nthe next": proto.PastePrefix,
	} {
		m, _ := sideConsole(t, boardState(boardMatch("flint-auth", time.Hour)))
		next, _ := m.Update(tea.PasteMsg{Content: text})
		m = pressOn(next.(Model), "enter")
		if acts := messageActs(boardFake(m)); len(acts) != 1 || acts[0].RecID != want+"%flint-auth" || acts[0].Text != text {
			t.Errorf("%.20q: sent %+v, want %s", text, acts, want)
		}
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
	m.tab = tabSessions
	order := m.sideOrder()
	if len(order) != 3 {
		t.Fatalf("order %v", order)
	}
	for i := 1; i < len(order); i++ {
		if m = pressOn(m, "down"); m.chosenSide() != order[i] {
			t.Fatalf("↓ %d chose %v, want %v", i, m.chosenSide(), order[i])
		}
	}
	if m = pressOn(m, "down"); m.chosenSide() != order[2] || m.footAt != 1 {
		t.Fatalf("↓ past the last chose %v, the foot's part %d", m.chosenSide(), m.footAt)
	}
	if m = pressOn(m, "up"); m.chosenSide() != order[2] || m.footAt != 0 {
		t.Fatalf("↑ from the foot chose %v, the foot's part %d", m.chosenSide(), m.footAt)
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
		{"s", func(m Model) bool {
			return strings.HasSuffix(m.noticeText, "is the only host · Machine › Hosts adds one")
		}},
		{"h", func(m Model) bool { return m.noticeText == "no host is hidden" }},
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
			m.tab = tabSessions
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
	m.tab = tabSessions
	m = pressOn(m, "esc", "w")
	if m.keysIn != focusLine || m.lines[m.chosenSide()] != "w" {
		t.Fatalf("w in the list: keys in %v, the line holds %q", m.keysIn, m.lines[m.chosenSide()])
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
	m.tab = tabSessions
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

// peerSide is the console at 160 columns with an empty home and the peer
// ws-2 that sent peer, with the keys of the list and the chat, on a clock
// the test moves: the guard started peekDead ago.
func peerSide(t *testing.T, home, peer proto.State) (Model, *fakeConn, *fakeConn) {
	t.Helper()
	clock := boardAt
	hc, ws := newFake(), newFake()
	m := New(Options{NoMotion: true, Conn: hc, Peers: []Peer{{Host: "ws-2", Conn: ws}}})
	m.opt.Now = func() time.Time { return clock }
	m.tab = tabSessions
	m.tell("digit")
	var next tea.Model = m
	next, _ = next.Update(tea.WindowSizeMsg{Width: 160, Height: 38})
	next, _ = next.Update(stateMsg(home))
	next, _ = next.Update(peerMsg{host: "ws-2", conn: ws, msg: stateMsg(peer)})
	m = next.(Model)
	for _, s := range m.all.sessions {
		if s.Host == "ws-2" {
			m.sideAt = paneOf(s)
		}
	}
	next, _ = m.Update(noticeMsg{})
	clock = clock.Add(peekDead)
	return next.(Model), hc, ws
}

func askingPeer() proto.State {
	return hostState(sess(4242, "flint-auth", "%7", func(s *sample.Session) {
		a := askingMatch("flint-auth", "Keep the token", "Refresh the token")
		s.Asks, s.Ask = true, a.Ask
	}))
}

// TestShortOptionsInProseStillDrawTheQuestion: options that are plain
// words in the pane's prose are no menu: the chat draws the question.
func TestShortOptionsInProseStillDrawTheQuestion(t *testing.T) {
	m, _ := sideConsole(t, boardState(askingMatch("flint-auth", "Yes", "No")))
	s := matchNamed(t, m, "flint-auth")
	m.watchHost, m.watching = s.Host, s.Pane
	m.screen = proto.Screen{Pane: s.Pane, Text: "● Yes, the token is stored. No change is needed."}
	if got := chatText(m, 100, 14); !strings.Contains(got, "Which way do we fix the login?") || !strings.Contains(got, "1. Yes") {
		t.Fatalf("the question is off the screen:\n%s", got)
	}
}

func TestListKeyWithNoJobSaysSo(t *testing.T) {
	m, _ := sideConsole(t, boardState(boardMatch("flint-auth", time.Hour)))
	m = pressOn(m, "esc")
	for _, k := range []tea.KeyPressMsg{keyPress("backspace"), keyPress("left"), keyPress("right"), keyPress("esc")} {
		m.noticeText = ""
		next, cmd := m.Update(k)
		drain(cmd)
		m = next.(Model)
		if !strings.Contains(m.noticeText, "the list takes") || m.keysIn != focusList {
			t.Fatalf("%s in the list: notice %q, keys in %v", k.String(), m.noticeText, m.keysIn)
		}
	}
	e, _ := sideConsole(t, boardState())
	e = pressOn(e, "esc")
	if e = pressOn(e, "down"); e.footAt != 1 || !strings.Contains(e.sideKeyWords(), "+ new") {
		t.Fatalf("↓ on an empty list: foot %d, key words %q, want + new chosen", e.footAt, e.sideKeyWords())
	}
	e.noticeText = ""
	if e = pressOn(e, "up", "up"); e.noticeText == "" {
		t.Fatal("↑ on an empty list said nothing")
	}
}

func TestLineKeyWithNoJobSaysSo(t *testing.T) {
	m, _ := sideConsole(t, boardState(boardMatch("flint-auth", time.Hour)))
	for _, k := range []tea.KeyPressMsg{keyPress("right"), keyPress("pgup"), {Code: tea.KeyHome}, {Code: tea.KeyDelete}, keyPress("ctrl+x")} {
		m.noticeText = ""
		next, cmd := m.Update(k)
		drain(cmd)
		m = next.(Model)
		if !strings.Contains(m.noticeText, "the line takes text") || m.keysIn != focusLine || len(boardFake(m).acts()) != 0 {
			t.Fatalf("%s in the line: notice %q, keys in %v, acts %+v", k.String(), m.noticeText, m.keysIn, boardFake(m).acts())
		}
	}
}

func TestTypingWithNoSessionSaysSo(t *testing.T) {
	m, _ := sideConsole(t, boardState())
	if m = pressOn(m, "a"); !strings.Contains(m.noticeText, "no session") || len(m.lines) != 0 {
		t.Fatalf("a with no session: notice %q, lines %v", m.noticeText, m.lines)
	}
	m.noticeText = ""
	next, cmd := m.Update(tea.PasteMsg{Content: "words"})
	drain(cmd)
	if m = next.(Model); !strings.Contains(m.noticeText, "no session") || len(m.lines) != 0 {
		t.Fatalf("a paste with no session: notice %q, lines %v", m.noticeText, m.lines)
	}
}

// TestRefusedPeerAnswerClearsAndSays: a peer that refuses the answer
// gives the digits back, and the notice says why.
func TestRefusedPeerAnswerClearsAndSays(t *testing.T) {
	m, _, ws := peerSide(t, hostState(), askingPeer())
	m = pressOn(m, "1", "1")
	if !hasAct(ws, proto.KeysPrefix+"%7") {
		t.Fatalf("ws-2 got %+v", ws.acts())
	}
	s := m.all.sessions[slices.IndexFunc(m.all.sessions, func(s sample.Session) bool { return s.Host == "ws-2" })]
	if _, sent := m.chatMarks(s); sent != 1 {
		t.Fatalf("sent %d, want 1", sent)
	}
	next, cmd := m.Update(peerMsg{host: "ws-2", conn: ws, msg: resultMsg{ActID: "a" + strconv.Itoa(m.acts), Err: "the pane moved"}})
	drain(cmd)
	m = next.(Model)
	if _, sent := m.chatMarks(s); sent != 0 || !strings.Contains(m.noticeText, "the pane moved") {
		t.Fatalf("after the refusal: sent %d, notice %q", sent, m.noticeText)
	}
}

func TestPeerAnswerGoesToThePeerOnly(t *testing.T) {
	m, home, ws := peerSide(t, hostState(sess(1, "hearth-api", "%7", nil)), askingPeer())
	pressOn(m, "1", "1")
	if got := keyActs(ws); len(got) != 1 || got[0].Text != proto.LitPrefix+"1" {
		t.Fatalf("ws-2 got %+v", got)
	}
	if got := keyActs(home); len(got) != 0 {
		t.Fatalf("home got %+v", got)
	}
}

func TestAnswerToAPeerWhileHomeIsDown(t *testing.T) {
	m, home, ws := peerSide(t, hostState(), askingPeer())
	m.lost = true
	m = pressOn(m, "1", "1")
	if got := keyActs(ws); len(got) != 1 || len(keyActs(home)) != 0 {
		t.Fatalf("ws-2 got %+v, home %+v", got, keyActs(home))
	}
}

// TestLostConnPutsTheTextBack: a message the service did not answer
// before its conn went comes back to its line, which says why.
func TestLostConnPutsTheTextBack(t *testing.T) {
	m, _, ws := peerSide(t, hostState(), hostState(sess(4242, "flint-auth", "%7", nil)))
	m = pressOn(typeLine(m, "go on"), "enter")
	if len(messageActs(ws)) != 1 || m.lines[m.chosenSide()] != "" {
		t.Fatalf("sent %+v, the line holds %q", messageActs(ws), m.lines[m.chosenSide()])
	}
	next, cmd := m.Update(peerMsg{host: "ws-2", conn: ws, msg: lostMsg{errors.New("eof"), ws}})
	drain(cmd)
	m = next.(Model)
	if got := m.lines[m.chosenSide()]; got != "go on" {
		t.Fatalf("the line holds %q", got)
	}
	if got := chatLineOf(m, 120); !strings.Contains(got, "not sent: ws-2 is not reached") {
		t.Fatalf("line %q", got)
	}

	h, _ := sideConsole(t, boardState(boardMatch("flint-auth", time.Hour)))
	h = pressOn(typeLine(h, "go on"), "enter")
	next, cmd = h.Update(lostMsg{errors.New("eof"), h.conn})
	drain(cmd)
	h = next.(Model)
	if got := h.lines[h.chosenSide()]; got != "go on" || !strings.Contains(chatLineOf(h, 120), "not sent: no service") {
		t.Fatalf("home lost: the line holds %q, says %q", got, chatLineOf(h, 120))
	}
}

func TestRefusalKeepsBothTexts(t *testing.T) {
	m, _ := sideConsole(t, boardState(boardMatch("flint-auth", time.Hour)))
	m = pressOn(typeLine(m, "first"), "enter")
	id := "a" + strconv.Itoa(m.acts)
	m = typeLine(m, "second")
	next, _ := m.Update(resultMsg{ActID: id, Err: "flint-auth asks a question now"})
	m = next.(Model)
	if got := m.lines[m.chosenSide()]; got != "first second" {
		t.Fatalf("the line holds %q", got)
	}
	if got := chatLineOf(m, 120); !strings.Contains(got, "not sent: flint-auth asks a question now") {
		t.Fatalf("line %q", got)
	}
}

// TestRefusalReturnsToTheOldSessionsLine: the refused text goes back to
// the line it was typed in, not to the line now chosen.
func TestRefusalReturnsToTheOldSessionsLine(t *testing.T) {
	m, _ := sideConsole(t, boardState(boardMatch("flint-auth", time.Hour), boardMatch("hearth-api", 2*time.Hour)))
	m = pickSide(t, m, "flint-auth")
	m = pressOn(typeLine(m, "first"), "enter")
	id := "a" + strconv.Itoa(m.acts)
	m = pickSide(t, m, "hearth-api")
	next, _ := m.Update(resultMsg{ActID: id, Err: "no agent session in %flint-auth any more"})
	m = next.(Model)
	flint, hearth := paneOf(matchNamed(t, m, "flint-auth")), paneOf(matchNamed(t, m, "hearth-api"))
	if m.lines[flint] != "first" || m.lines[hearth] != "" || m.lineNote.k != flint {
		t.Fatalf("lines %v, note %+v", m.lines, m.lineNote)
	}
}

// workingAs is a match of the agent that works now.
func workingAs(name, agent string) sample.Session {
	s := boardMatch(name, time.Hour)
	s.Agent, s.Busy, s.Status, s.Idle = agent, true, "busy", 0
	return s
}

// TestBusyAgentGetsTheMessageWhenItsTurnEnds: OpenCode does not take typed
// input while it works, so the console holds the message and sends it when
// the session leaves "works". A second message waits for the first.
func TestBusyAgentGetsTheMessageWhenItsTurnEnds(t *testing.T) {
	m, _ := sideConsole(t, boardState(workingAs("hearth-api", "opencode")))
	if m.sideStateOf(m.all.sessions[0]) != sideWork {
		t.Fatalf("the session does not work: %v", m.sideStateOf(m.all.sessions[0]))
	}
	k := m.chosenSide()
	m = pressOn(typeLine(m, "next please"), "enter")
	if got := messageActs(boardFake(m)); len(got) != 0 {
		t.Fatalf("sent to a busy agent: %+v", got)
	}
	if got := chatLineOf(m, 120); !strings.Contains(got, "queued for its next turn") {
		t.Fatalf("line %q", got)
	}
	if m.lines[k] != "" || m.lineHeld[k].text != "next please" {
		t.Fatalf("line %q, held %q", m.lines[k], m.lineHeld[k])
	}

	m = pressOn(typeLine(m, "and more"), "enter")
	if got := chatLineOf(m, 120); !strings.Contains(got, "one message already waits for its next turn") || m.lines[k] != "and more" {
		t.Fatalf("line %q, text %q", got, m.lines[k])
	}

	next, cmd := m.Update(stateMsg(boardState(boardMatch("hearth-api", time.Hour))))
	drain(cmd)
	m = next.(Model)
	got := messageActs(boardFake(m))
	if len(got) != 1 || got[0].RecID != "message:"+k.pane || got[0].Text != "next please" || got[0].Confirm != "y" {
		t.Fatalf("sent %+v", got)
	}
	if len(m.lineHeld) != 0 {
		t.Fatalf("still held: %v", m.lineHeld)
	}
}

// TestBusyAgentThatQueuesGetsTheMessageAtOnce: Claude Code queues what is
// typed while it works, and the line says so.
func TestBusyAgentThatQueuesGetsTheMessageAtOnce(t *testing.T) {
	m, _ := sideConsole(t, boardState(workingAs("hearth-api", "claude")))
	m = pressOn(typeLine(m, "now"), "enter")
	if got := messageActs(boardFake(m)); len(got) != 1 || got[0].Text != "now" {
		t.Fatalf("sent %+v", got)
	}
	if got := chatLineOf(m, 120); !strings.Contains(got, "queued for its next turn") {
		t.Fatalf("line %q", got)
	}
}

// TestHeldMessageOfAnEndedSessionComesBack: the session ends while the
// message waits, and the text returns to its line, unsent.
func TestHeldMessageOfAnEndedSessionComesBack(t *testing.T) {
	m, _ := sideConsole(t, boardState(workingAs("hearth-api", "opencode"), boardMatch("flint-auth", time.Hour)))
	m = pickSide(t, m, "hearth-api")
	k := m.chosenSide()
	m = pressOn(typeLine(m, "later"), "enter")
	next, _ := m.Update(stateMsg(boardState(boardMatch("flint-auth", time.Hour))))
	m = next.(Model)
	if len(m.lineHeld) != 0 || m.lines[k] != "later" || len(messageActs(boardFake(m))) != 0 {
		t.Fatalf("held %v, line %q, sent %+v", m.lineHeld, m.lines[k], messageActs(boardFake(m)))
	}
	if m.lineNote.text != "hearth-api ended · the text is not sent" {
		t.Fatalf("note %+v", m.lineNote)
	}
}

// heldOn is the console with one message held for hearth-api, an OpenCode
// session that works.
func heldOn(t *testing.T, others ...sample.Session) Model {
	t.Helper()
	m, _ := sideConsole(t, boardState(append([]sample.Session{workingAs("hearth-api", "opencode")}, others...)...))
	m = pickSide(t, m, "hearth-api")
	m = pressOn(typeLine(m, "later"), "enter")
	if m.lineHeld[paneOf(matchNamed(t, m, "hearth-api"))].text != "later" {
		t.Fatalf("nothing held: %v", m.lineHeld)
	}
	return m
}

// TestParkWhileHeldGivesTheTextBack: a parked session takes no message.
func TestParkWhileHeldGivesTheTextBack(t *testing.T) {
	m := heldOn(t)
	s := boardMatch("hearth-api", time.Hour)
	st := boardState(s)
	st.Holds = append(st.Holds, proto.Hold{SessionID: s.SessionID, Pane: s.Pane, Name: s.Name, By: holdByYou})
	next, cmd := m.Update(stateMsg(st))
	drain(cmd)
	m = next.(Model)
	if got := messageActs(boardFake(m)); len(got) != 0 {
		t.Fatalf("sent to a parked session: %+v", got)
	}
	if len(m.lineHeld) != 0 || m.lines[paneOf(s)] != "later" {
		t.Fatalf("held %v, line %q", m.lineHeld, m.lines[paneOf(s)])
	}
	if got := chatLineOf(m, 120); !strings.Contains(got, "parked: z lets it go") {
		t.Fatalf("line %q", got)
	}
}

// TestStokerHoldKeepsTheMessageWaiting: the stoker's hold is not the end of
// the turn.
func TestStokerHoldKeepsTheMessageWaiting(t *testing.T) {
	m := heldOn(t)
	s := boardMatch("hearth-api", time.Hour)
	st := boardState(s)
	st.Holds = append(st.Holds, proto.Hold{SessionID: s.SessionID, Pane: s.Pane, Name: s.Name, By: "stoker", Why: "not the priority"})
	next, cmd := m.Update(stateMsg(st))
	drain(cmd)
	m = next.(Model)
	if got := messageActs(boardFake(m)); len(got) != 0 {
		t.Fatalf("sent to a held session: %+v", got)
	}
	if m.lineHeld[paneOf(s)].text != "later" {
		t.Fatalf("held %v", m.lineHeld)
	}
	if got := chatLineOf(m, 120); !strings.Contains(got, "queued for its next turn") {
		t.Fatalf("line %q", got)
	}
}

// TestReleaseIntoAQuestionComesBackWithWords: the session asks, the
// message goes, the service refuses, and the text returns with its words.
func TestReleaseIntoAQuestionComesBackWithWords(t *testing.T) {
	m := heldOn(t)
	next, cmd := m.Update(stateMsg(boardState(askingMatch("hearth-api", "Yes", "No"))))
	drain(cmd)
	m = next.(Model)
	got := messageActs(boardFake(m))
	if len(got) != 1 || got[0].Text != "later" {
		t.Fatalf("sent %+v", got)
	}
	next, _ = m.Update(resultMsg{ActID: "a" + strconv.Itoa(m.acts), Err: "hearth-api asks a question now"})
	m = next.(Model)
	if m.lines[m.chosenSide()] != "later" || !strings.Contains(chatLineOf(m, 120), "not sent: hearth-api asks a question now") {
		t.Fatalf("line %q, says %q", m.lines[m.chosenSide()], chatLineOf(m, 120))
	}
}

// TestLostConnWhileHeldKeepsTheText: the service is gone when the turn
// ends, so the text goes back to the line and says why.
func TestLostConnWhileHeldKeepsTheText(t *testing.T) {
	m := heldOn(t)
	fake := boardFake(m)
	m.conn = nil // the turn ends while the service conn is down
	next, cmd := m.Update(stateMsg(boardState(boardMatch("hearth-api", time.Hour))))
	drain(cmd)
	m = next.(Model)
	if got := messageActs(fake); len(got) != 0 {
		t.Fatalf("sent with no service: %+v", got)
	}
	if m.lines[m.chosenSide()] != "later" || !strings.Contains(chatLineOf(m, 120), "not sent: no service") {
		t.Fatalf("line %q, says %q", m.lines[m.chosenSide()], chatLineOf(m, 120))
	}
}

// TestHeldNoteShowsOnlyForItsSession: another session's line says nothing
// of the message that waits.
func TestHeldNoteShowsOnlyForItsSession(t *testing.T) {
	m := heldOn(t, boardMatch("flint-auth", 2*time.Hour))
	if got := chatLineOf(m, 120); !strings.Contains(got, "queued for its next turn") {
		t.Fatalf("its own line %q", got)
	}
	m = pickSide(t, m, "flint-auth")
	if got := chatLineOf(m, 120); strings.Contains(got, "queued") {
		t.Fatalf("flint-auth's line %q", got)
	}
}

// TestAInTheListTypes: a goes into no pane: in the list it goes back to the
// line and types there, as any letter with no job does. Ctrl+O goes in.
func TestAInTheListTypes(t *testing.T) {
	m, _, ran := sideShot(t, 160, 38)
	m.toList()
	if m = pressOn(m, "a"); len(*ran) != 0 || m.inList() || m.lines[m.chosenSide()] != "a" {
		t.Fatalf("a in the list: ran %v, in the list %v, line %q", *ran, m.inList(), m.lines[m.chosenSide()])
	}
}

// TestTheMenuGoesInByCtrlO: the menu's go in names ctrl+o, and goes in.
func TestTheMenuGoesInByCtrlO(t *testing.T) {
	m, _, ran := sideShot(t, 160, 38)
	m.toList()
	m = pressOn(m, "m")
	i := slices.IndexFunc(m.menu.items, func(it menuItem) bool { return it.label == "go in" })
	if i < 0 || m.menu.items[i].key != "ctrl+o" {
		t.Fatalf("the menu's go in: %+v", m.menu.items)
	}
	m.menu.at = i
	next, cmd := m.Update(keyPress("enter"))
	fed(next, cmd)
	if len(*ran) != 1 {
		t.Fatalf("go in ran %v", *ran)
	}
}

// beltTap is a tap on an act key of Pocket's belt: its letter with the
// super modifier (CSI u), which no soft keyboard sends.
func beltTap(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModSuper} }

// TestBeltActKeysActWhileTheLineHasTheKeys: the belt's act keys (copy,
// menu, kill…) act on the chosen session even while the line has the keys,
// and the same letter typed on the keyboard still types.
func TestBeltActKeysActWhileTheLineHasTheKeys(t *testing.T) {
	m, clipped, _ := sideShot(t, 160, 38)
	m.toLine()
	next, cmd := m.Update(beltTap('c'))
	nm := fed(next, cmd).(Model)
	if len(*clipped) != 1 || nm.lines[nm.chosenSide()] != "" || nm.inList() {
		t.Errorf("the belt's c: clipped %q, line %q, in the list %v", *clipped, nm.lines[nm.chosenSide()], nm.inList())
	}
	next, _ = m.Update(beltTap('x'))
	if next.(Model).pending == nil {
		t.Errorf("the belt's x opened no kill box: %q", next.(Model).noticeText)
	}
	next, _ = m.Update(beltTap('m'))
	if next.(Model).menu == nil {
		t.Error("the belt's m opened no menu")
	}
	if nm := pressOn(m, "c"); nm.lines[nm.chosenSide()] != "c" {
		t.Errorf("a typed c: the line holds %q", nm.lines[nm.chosenSide()])
	}
}
