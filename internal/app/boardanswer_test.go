package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
)

// askBoard is the board at w by h on st with the card of name chosen and
// its peek landed peekDead ago, on a clock the test moves. The once-notice
// of a digit is already told.
func askBoard(t *testing.T, st proto.State, w, h int, name string) (Model, *time.Time) {
	t.Helper()
	clock := demoAt
	m := boardShot(t, st, w, h, func(m *Model) {
		m.opt.Now = func() time.Time { return clock }
		chooseCard(name)(m)
		m.tell("digit")
	})
	next, _ := m.Update(noticeMsg{}) // the first frame: the peek lands
	clock = clock.Add(peekDead)
	return next.(Model), &clock
}

// flintBoard is askBoard on the fixture with flint-auth chosen: it asks
// "Which way do we fix the login?" with two options.
func flintBoard(t *testing.T, w, h int) (Model, *time.Time) {
	t.Helper()
	return askBoard(t, boardFixture(t), w, h, "flint-auth")
}

func matchNamed(t *testing.T, m Model, name string) sample.Session {
	t.Helper()
	i := slices.IndexFunc(m.snap.Sessions, func(s sample.Session) bool { return s.Name == name })
	if i < 0 {
		t.Fatalf("no match %s", name)
	}
	return m.snap.Sessions[i]
}

// sent are the keys the board sent.
func sent(m Model) []proto.Act { return keyActs(boardFake(m)) }

func armedOn(m Model, n string) bool {
	return strings.Contains(plainFrame(m), "❯ "+n+".")
}

// askNew is st with flint-auth asking a new question.
func askNew(st proto.State) proto.State {
	st.Sessions = slices.Clone(st.Sessions)
	for i, s := range st.Sessions {
		if s.Name == "flint-auth" {
			s.Ask = &said.Ask{ID: "toolu_next", Questions: []said.Question{{Question: "Which test goes first?",
				Options: []said.Option{{Label: "The expiry test"}, {Label: "The refresh test"}}}}}
			st.Sessions[i] = s
		}
	}
	return st
}

func TestTheFirstDigitArmsAndTheSecondSends(t *testing.T) {
	m, _ := flintBoard(t, 80, 31)
	m = pressOn(m, "1")
	if len(sent(m)) != 0 || !armedOn(m, "1") {
		t.Fatalf("the first 1 sent %+v or marks nothing:\n%s", sent(m), plainFrame(m))
	}
	if !strings.Contains(plainFrame(m), "1 again sends · esc clears") {
		t.Errorf("the armed card does not say how to send:\n%s", plainFrame(m))
	}
	m = pressOn(m, "2")
	if len(sent(m)) != 0 || !armedOn(m, "2") || armedOn(m, "1") {
		t.Fatalf("2 after 1 sent %+v or did not move the mark:\n%s", sent(m), plainFrame(m))
	}
	m = pressOn(m, "2")
	acts := sent(m)
	if len(acts) != 1 || acts[0].Text != proto.LitPrefix+"2" || acts[0].RecID != proto.KeysPrefix+matchNamed(t, m, "flint-auth").Pane {
		t.Fatalf("2 again sent %+v, want one keys act with 2", acts)
	}

	m, _ = flintBoard(t, 80, 31)
	m = pressOn(m, "1", "esc")
	if armedOn(m, "1") {
		t.Errorf("esc did not clear the mark")
	}
	m = pressOn(m, "1")
	if len(sent(m)) != 0 || !armedOn(m, "1") {
		t.Errorf("1 after esc sent %+v: it must only mark", sent(m))
	}

	m, clock := flintBoard(t, 80, 31)
	m = pressOn(m, "1", "up", "down")
	*clock = clock.Add(peekDead)
	if armedOn(m, "1") {
		t.Errorf("a move of the peek did not clear the mark")
	}
	if m = pressOn(m, "1"); len(sent(m)) != 0 {
		t.Errorf("1 after a move sent %+v: it must only mark", sent(m))
	}

	m, _ = flintBoard(t, 80, 31)
	m = tapText(t, m, "Patch the token check")
	if len(sent(m)) != 0 || !armedOn(m, "1") {
		t.Fatalf("a tap on option 1 sent %+v or marks nothing", sent(m))
	}
	m = tapText(t, m, "Patch the token check")
	if acts := sent(m); len(acts) != 1 || acts[0].Text != proto.LitPrefix+"1" {
		t.Errorf("a second tap on option 1 sent %+v", acts)
	}
}

func TestTheDeadTimeStartsOnOpenAndOnAFilterChange(t *testing.T) {
	clock := demoAt
	m := boardShot(t, boardFixture(t), 80, 31, func(m *Model) {
		m.opt.Now = func() time.Time { return clock }
		chooseCard("flint-auth")(m)
		m.tell("digit")
	})
	if m = pressOn(m, "1"); armedOn(m, "1") {
		t.Errorf("a digit on the first frame marks an answer")
	}
	clock = clock.Add(peekDead - time.Millisecond)
	if m = pressOn(m, "1"); armedOn(m, "1") {
		t.Errorf("a digit within %v of the first frame marks an answer", peekDead)
	}
	clock = clock.Add(time.Millisecond)
	if m = pressOn(m, "1", "esc"); len(sent(m)) != 0 {
		t.Fatalf("sent %+v", sent(m))
	}

	for _, keys := range [][]string{{"tab", "shift+tab"}, {"s"}, {"g"}} {
		clock = clock.Add(time.Second)
		m = pressOn(m, keys...)
		if !m.boardShown() || chosenName(t, m) != "flint-auth" {
			t.Fatalf("after %v the board shows %v, chosen %s", keys, m.boardShown(), chosenName(t, m))
		}
		clock = clock.Add(peekDead - time.Millisecond)
		if m = pressOn(m, "1"); armedOn(m, "1") {
			t.Errorf("a digit within %v of %v marks an answer", peekDead, keys)
		}
		clock = clock.Add(time.Millisecond)
		if m = pressOn(m, "1"); !armedOn(m, "1") {
			t.Errorf("a digit %v after %v marks nothing", peekDead, keys)
		}
		m = pressOn(m, "esc")
	}
	if len(sent(m)) != 0 {
		t.Errorf("sent %+v", sent(m))
	}
}

func TestTheSentState(t *testing.T) {
	m, clock := flintBoard(t, 80, 31)
	m = pressOn(m, "2", "2")
	if len(sent(m)) != 1 {
		t.Fatalf("sent %+v", sent(m))
	}
	if got := noticeRow(m); got != "sent 2 to flint-auth · now in WORKING" {
		t.Errorf("the notice says %q", got)
	}
	if chosenName(t, m) == "flint-auth" {
		t.Errorf("after the answer flint-auth is still chosen")
	}

	m = pressOn(m, "down", "down", "down")
	if chosenName(t, m) != "flint-auth" {
		t.Fatalf("chose %s", chosenName(t, m))
	}
	*clock = clock.Add(peekDead)
	m = pressOn(m, "2", "2", "2")
	if len(sent(m)) != 1 {
		t.Errorf("a digit after the send sent %+v", sent(m))
	}
	f := plainFrame(m)
	if !strings.Contains(f, "sent 2 · waits for flint-auth") || strings.Contains(f, "1. Patch the token check") {
		t.Errorf("the sent card draws its options again:\n%s", f)
	}

	done, _ := m.Update(resultMsg{ActID: keyActID + strings.TrimPrefix(sent(m)[0].RecID, proto.KeysPrefix)})
	m = newBoardState(t, done.(Model), askNew(boardFixture(t)))
	if f := plainFrame(m); strings.Contains(f, "sent 2 ·") || !strings.Contains(f, "1. The expiry test") {
		t.Errorf("a new question does not bring the options back:\n%s", f)
	}
	*clock = clock.Add(peekDead)
	m = pressOn(m, "1", "1")
	if acts := sent(m); len(acts) != 2 || acts[1].Text != proto.LitPrefix+"1" {
		t.Errorf("the new question's answer sent %+v", acts)
	}
}

func TestADigitAnswersTheChosenQuestion(t *testing.T) {
	for _, size := range [][2]int{{40, 34}, {80, 31}, {160, 38}} {
		m, _ := flintBoard(t, size[0], size[1])
		if bare := size[0] >= liveFrom; bare && strings.Contains(plainFrame(m), "1. Patch the token check") {
			t.Errorf("%v: the card beside the live view draws its options before a digit", size)
		}
		m = pressOn(m, "1")
		if f := plainFrame(m); len(sent(m)) != 0 || !armedOn(m, "1") || !strings.Contains(f, "1 again sends · esc clears") {
			t.Errorf("%v: the first 1 sent %+v, or the card does not show the mark:\n%s", size, sent(m), f)
		}
		if f := plainFrame(m); size[0] >= liveFrom && strings.Contains(f, "2. Move to the new session store") {
			t.Errorf("%v: the bare card draws an option that is not marked:\n%s", size, f)
		}
		m = pressOn(m, "1")
		acts := sent(m)
		flint := matchNamed(t, m, "flint-auth")
		if len(acts) != 1 || acts[0].Text != proto.LitPrefix+"1" || acts[0].RecID != proto.KeysPrefix+flint.Pane || acts[0].PID != flint.PID {
			t.Errorf("%v: sent %+v, want one key 1 to %s", size, acts, flint.Pane)
			continue
		}
		if at := m.stateAt(flint.Host); !acts[0].Seen.Equal(at) || at.IsZero() {
			t.Errorf("%v: Seen is %v, want the state's time %v", size, acts[0].Seen, at)
		}
	}
}

func TestTheDigitIsTiedToTheQuestionTheCardDrew(t *testing.T) {
	m, clock := flintBoard(t, 80, 31)
	flint := matchNamed(t, m, "flint-auth")
	sampled := m.stateAt(flint.Host)
	m.watching = flint.Pane
	next, _ := m.Update(screenMsg{Pane: flint.Pane, At: clock.Add(time.Second)})
	m = pressOn(next.(Model), "1", "1")
	if acts := sent(m); len(acts) != 1 || !acts[0].Seen.Equal(sampled) {
		t.Fatalf("sent %+v, want Seen = the state's %v, never the screen's", acts, sampled)
	}

	m, clock = flintBoard(t, 80, 31)
	m = pressOn(m, "1")
	*clock = clock.Add(time.Second)
	m = newBoardState(t, m, askNew(boardFixture(t)))
	m = pressOn(m, "1")
	if len(sent(m)) != 0 || armedOn(m, "1") {
		t.Errorf("a digit right after a new question sent %+v or marks it", sent(m))
	}
	if got := noticeRow(m); got != "not sent: flint-auth asks something new — look again" {
		t.Errorf("the notice says %q", got)
	}
	if m = pressOn(m, "1"); len(sent(m)) != 0 {
		t.Errorf("a second digit sent %+v", sent(m))
	}
	*clock = clock.Add(peekDead)
	if m = pressOn(m, "1"); len(sent(m)) != 0 || !armedOn(m, "1") {
		t.Errorf("the mark made for the old question sent %+v to the new one", sent(m))
	}
}

func TestADigitOnAnotherHostIsNotSent(t *testing.T) {
	st := boardFixture(t)
	for i := range st.Sessions {
		if st.Sessions[i].Name == "flint-auth" {
			st.Sessions[i].Host = "gpu-1"
		}
	}
	st.Queue[0].Host = "gpu-1"
	m, _ := askBoard(t, st, 80, 31, "flint-auth")
	m = pressOn(m, "1", "1")
	peer := m.peers[m.peerAt("gpu-1")].conn.(*fakeConn)
	if len(sent(m)) != 0 || len(keyActs(peer)) != 0 {
		t.Errorf("a digit on gpu-1's card sent %+v and %+v", sent(m), keyActs(peer))
	}
	if got := noticeRow(m); got != "not sent: flint-auth is on gpu-1 · ⏎ goes in" {
		t.Errorf("the notice says %q", got)
	}
}

func TestADigitIsDeadRightAfterThePeekMoves(t *testing.T) {
	m, clock := askBoard(t, boardFixture(t), 80, 31, "hearth-api")
	m = pressOn(m, "down")
	if chosenName(t, m) != "flint-auth" {
		t.Fatalf("↓ chose %s", chosenName(t, m))
	}
	*clock = clock.Add(peekDead - time.Millisecond)
	if m = pressOn(m, "1"); armedOn(m, "1") {
		t.Errorf("1 at %v marks an answer", peekDead-time.Millisecond)
	}
	if got := noticeRow(m); got != "look at flint-auth's question first" {
		t.Errorf("the notice says %q", got)
	}
	*clock = clock.Add(time.Millisecond)
	if m = pressOn(m, "1"); !armedOn(m, "1") {
		t.Errorf("1 at %v marks nothing", peekDead)
	}
	if len(sent(m)) != 0 {
		t.Errorf("sent %+v", sent(m))
	}
}

func TestADigitWithNoSuchOptionDoesNothing(t *testing.T) {
	m, _ := flintBoard(t, 80, 31)
	for _, k := range []string{"3", "9", "0"} {
		m = pressOn(m, k, k)
		if len(sent(m)) != 0 || armedOn(m, k) {
			t.Errorf("%s on two options sent %+v or marks it", k, sent(m))
		}
	}
	if m.tab != tabSessions {
		t.Errorf("a digit switched the tab to %d", m.tab)
	}
}

func TestANewQuestionRefusesTheDigit(t *testing.T) {
	m, _ := flintBoard(t, 80, 31)
	m = pressOn(m, "1", "1")
	acts := sent(m)
	if len(acts) != 1 {
		t.Fatalf("sent %+v", acts)
	}
	refusal := "not sent: flint-auth asks something new — look again"
	next, _ := m.Update(resultMsg{ActID: keyActID + strings.TrimPrefix(acts[0].RecID, proto.KeysPrefix), Err: refusal})
	m = newBoardState(t, next.(Model), askNew(boardFixture(t)))
	m = pressOn(m, "down", "down", "down") // flint-auth, at the bottom of NEEDS YOU
	if got := noticeRow(m); got != refusal {
		t.Errorf("the notice says %q, want %q", got, refusal)
	}
	if head := strings.Split(plainFrame(m), "\n")[0]; strings.Contains(head, "not sent") {
		t.Errorf("the header shows the refusal: %q", head)
	}
	if f := plainFrame(m); !strings.Contains(f, "Which test goes first?") || strings.Contains(f, "sent 1 ·") {
		t.Errorf("the board does not show the new question:\n%s", f)
	}
}

func TestADigitOnACardThatDoesNotAskDoesNothing(t *testing.T) {
	for _, name := range []string{"kindle-web", "flint-ui", "ember-reports"} {
		m, _ := askBoard(t, boardFixture(t), 80, 31, name)
		m = pressOn(m, "1", "1")
		if len(sent(m)) != 0 || armedOn(m, "1") || m.tab != tabSessions {
			t.Errorf("%s: 1 1 sent %+v, marks %v, tab %d", name, sent(m), armedOn(m, "1"), m.tab)
		}
	}
}

func TestOnlyAOneChoiceQuestionTakesDigits(t *testing.T) {
	two := []said.Option{{Label: "Patch the token check"}, {Label: "Move to the new session store"}}
	for _, c := range []struct {
		name string
		edit func(*sample.Session, *queue.Item)
	}{
		{"a permission prompt", func(s *sample.Session, it *queue.Item) {
			s.Ask, s.Permits, it.State = nil, true, queue.StatePermission
		}},
		{"a question of Codex", func(s *sample.Session, it *queue.Item) {
			s.Ask, s.Asks, s.Agent = nil, true, "codex"
		}},
		{"a call with two questions", func(s *sample.Session, it *queue.Item) {
			s.Ask.Questions = append(s.Ask.Questions, said.Question{Question: "And the tests?", Options: two})
		}},
		{"a multi-select", func(s *sample.Session, it *queue.Item) {
			s.Ask.Questions = []said.Question{{Question: "Which ways?", Options: two, Multi: true}}
		}},
	} {
		st := boardFixture(t)
		for i := range st.Sessions {
			if st.Sessions[i].Name == "flint-auth" {
				a := *st.Sessions[i].Ask
				st.Sessions[i].Ask = &a
				c.edit(&st.Sessions[i], &st.Queue[0])
			}
		}
		m, _ := askBoard(t, st, 80, 31, "flint-auth")
		f := plainFrame(m)
		if strings.Contains(f, "1. Patch") || strings.Contains(f, "1-2") || !strings.Contains(f, "⏎ go in") {
			t.Errorf("%s: the card draws options or no ⏎ go in:\n%s", c.name, f)
		}
		if m = pressOn(m, "1", "1"); len(sent(m)) != 0 || armedOn(m, "1") {
			t.Errorf("%s: 1 1 sent %+v", c.name, sent(m))
		}
	}
}

func TestThePeekSendsNoWatch(t *testing.T) {
	next, _ := boardShot(t, boardFixture(t), 104, 38, nil).Update(noticeMsg{})
	m := next.(Model) // the old console's watch of its list ends here
	before := len(boardFake(m).allActs())
	for _, k := range []string{"down", "down", "down", "right", "down", "right", "down", "right", "down", "down"} {
		m = pressOn(m, k)
	}
	if acts := boardFake(m).allActs(); len(acts) != before {
		t.Errorf("moving the peek sent %+v", acts[before:])
	}
}

func TestATapOnAnOptionArmsIt(t *testing.T) {
	m, clock := askBoard(t, boardFixture(t), 80, 31, "hearth-api")
	m = pressOn(m, "down")
	m = tapText(t, m, "Patch the token check")
	if armedOn(m, "1") {
		t.Errorf("a tap inside the dead time marks an answer")
	}
	*clock = clock.Add(peekDead)
	m = tapText(t, m, "Move to the new session store")
	if !armedOn(m, "2") || len(sent(m)) != 0 {
		t.Fatalf("a tap on option 2 marks %v, sent %+v", armedOn(m, "2"), sent(m))
	}
	m = tapText(t, m, "Move to the new session store")
	if acts := sent(m); len(acts) != 1 || acts[0].Text != proto.LitPrefix+"2" {
		t.Errorf("a second tap on option 2 sent %+v", acts)
	}
}

func TestADigitWhileTheServiceIsLostIsNotSent(t *testing.T) {
	m, _ := flintBoard(t, 80, 31)
	m = pressOn(m, "1")
	m.lost = true
	m = pressOn(m, "1")
	if len(sent(m)) != 0 {
		t.Errorf("a digit with the service lost sent %+v", sent(m))
	}
	if got := noticeRow(m); got != "not sent: no service · it redials" {
		t.Errorf("the notice says %q", got)
	}
	if head := strings.Split(plainFrame(m), "\n")[0]; strings.Contains(head, "not sent") {
		t.Errorf("the header shows the refusal: %q", head)
	}
}

func TestAnAnswerThatCannotWaitInLineIsNotSent(t *testing.T) {
	m, _ := flintBoard(t, 80, 31)
	m.keysOut = keysOut{conn: m.conn, busy: true, queue: make([]proto.Act, maxKeysAhead)}
	m = pressOn(m, "1", "1")
	if got := noticeRow(m); got != "not sent: 64 keys still wait for flint-auth" {
		t.Errorf("the notice says %q", got)
	}
	if head := strings.Split(plainFrame(m), "\n")[0]; strings.Contains(head, "not sent") || m.flash != "" {
		t.Errorf("the header shows %q, the flash %q", head, m.flash)
	}
	if _, sentN := m.peekMarks(matchNamed(t, m, "flint-auth")); sentN != 0 || chosenName(t, m) != "flint-auth" || !armedOn(m, "1") {
		t.Errorf("a key that did not go counts as sent (%d), or the choice moved to %s", sentN, chosenName(t, m))
	}
	m.keysOut.queue = nil
	if m = pressOn(m, "1"); len(m.keysOut.queue) != 1 || m.keysOut.queue[0].Text != proto.LitPrefix+"1" {
		t.Errorf("the digits are not live after the line had room: %+v", m.keysOut.queue)
	}
}

func TestOnlyTheAnswersRefusalTakesItsDigitsBack(t *testing.T) {
	m, _ := flintBoard(t, 80, 31)
	m = pressOn(m, "1", "1")
	flint := matchNamed(t, m, "flint-auth")
	next, _ := m.Update(resultMsg{ActID: keyActID + "%another", Err: "not sent: kindle-web asks something new — look again"})
	m = next.(Model)
	if _, n := m.peekMarks(flint); n != 1 {
		t.Errorf("a refused key of another pane cleared the answer: sent %d", n)
	}
	next, _ = m.Update(resultMsg{ActID: keyActID + flint.Pane, Err: "not sent: flint-auth asks something new — look again"})
	if _, n := next.(Model).peekMarks(flint); n != 0 {
		t.Errorf("the answer's refusal kept it sent: %d", n)
	}
}

func TestADigitIsDeadWhenTheChoiceMovesWithNoKey(t *testing.T) {
	st := boardFixture(t)
	for i, s := range st.Sessions {
		if s.Name == "spark-search" {
			s.Ask = &said.Ask{ID: "toolu_lint", Questions: []said.Question{{Question: "Do I skip the lint?",
				Options: []said.Option{{Label: "Skip it"}, {Label: "Fix the file"}}}}}
			st.Sessions[i] = s
			st.Queue = append(st.Queue, waitFor(s, queue.StateQuestion, 31*time.Minute))
		}
	}
	m, clock := askBoard(t, st, 80, 31, "flint-auth")
	gone := st
	gone.Sessions = slices.Clone(st.Sessions)
	for i, s := range gone.Sessions {
		if s.Name == "flint-auth" {
			s.Ask, s.Busy, s.Status = nil, true, "busy"
			gone.Sessions[i] = s
		}
	}
	gone.Queue = st.Queue[1:]
	*clock = clock.Add(time.Second)
	m = newBoardState(t, m, gone)
	if s, ok := m.boardSelected(); !ok || s.Name == "flint-auth" || !m.peekAnswers(s) {
		t.Fatalf("the choice is %s (%v), want a card that asks", s.Name, ok)
	}
	if m = pressOn(m, "1"); armedOn(m, "1") {
		t.Errorf("a digit right after the choice moved with no key marks an answer")
	}
	*clock = clock.Add(peekDead)
	if m = pressOn(m, "1"); !armedOn(m, "1") || len(sent(m)) != 0 {
		t.Errorf("a digit %v later marks %v, sent %+v", peekDead, armedOn(m, "1"), sent(m))
	}
}

func TestADigitIsDeadWhenTheBoardComesBackFromTheConsentBox(t *testing.T) {
	m, clock := flintBoard(t, 80, 31)
	m = pressOn(m, "x")
	if m.pending == nil {
		t.Fatal("x opened no consent box")
	}
	*clock = clock.Add(time.Second)
	m = pressOn(m, "esc")
	if m.pending != nil || chosenName(t, m) != "flint-auth" {
		t.Fatalf("esc left the box open, or chose %s", chosenName(t, m))
	}
	if m = pressOn(m, "1"); armedOn(m, "1") {
		t.Errorf("a digit right after the consent box closed marks an answer")
	}
	*clock = clock.Add(peekDead)
	if m = pressOn(m, "1"); !armedOn(m, "1") {
		t.Errorf("a digit %v after the box closed marks nothing", peekDead)
	}
}
