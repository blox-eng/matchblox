package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
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
	if _, sentN := m.chatMarks(matchNamed(t, m, "flint-auth")); sentN != 0 || chosenName(t, m) != "flint-auth" || !armedOn(m, "1") {
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
	if _, n := m.chatMarks(flint); n != 1 {
		t.Errorf("a refused key of another pane cleared the answer: sent %d", n)
	}
	next, _ = m.Update(resultMsg{ActID: keyActID + flint.Pane, Err: "not sent: flint-auth asks something new — look again"})
	if _, n := next.(Model).chatMarks(flint); n != 0 {
		t.Errorf("the answer's refusal kept it sent: %d", n)
	}
}
