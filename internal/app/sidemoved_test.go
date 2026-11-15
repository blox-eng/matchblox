package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
)

// The guards of the board's tests, on the console: a digit that comes
// before the person saw the question marks nothing, no act runs on a row
// that is not chosen, and a tap is never consent.

func TestADigitIsDeadWhenTheChatComesBackFromTheConsentBox(t *testing.T) {
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
	m.toLine() // a digit answers from the chat's line
	if m = pressOn(m, "1"); armedOn(m, "1") {
		t.Errorf("a digit right after the consent box closed marks an answer")
	}
	m = pressOn(m, "backspace") // a dead digit types
	*clock = clock.Add(peekDead)
	if m = pressOn(m, "1"); !armedOn(m, "1") {
		t.Errorf("a digit %v after the box closed marks nothing", peekDead)
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
	chooseCard("spark-search")(&m) // flint-auth works now: the person looks at the next that asks
	if s, ok := m.sideChosen(); !ok || !m.chatAnswers(s) {
		t.Fatalf("the choice is %s (%v), want a session that asks", s.Name, ok)
	}
	m.toLine()
	if m = pressOn(m, "1"); armedOn(m, "1") {
		t.Errorf("a digit right after the choice moved marks an answer")
	}
	m = pressOn(m, "backspace")
	*clock = clock.Add(peekDead)
	if m = pressOn(m, "1"); !armedOn(m, "1") || len(sent(m)) != 0 {
		t.Errorf("a digit %v later marks %v, sent %+v", peekDead, armedOn(m, "1"), sent(m))
	}
}

func TestTheChatsDeadTimeStartsOnOpenAndOnAFilterChange(t *testing.T) {
	clock := demoAt
	m := boardShot(t, boardFixture(t), 80, 31, func(m *Model) {
		m.opt.Now = func() time.Time { return clock }
		chooseCard("flint-auth")(m)
		m.tell("digit")
	})
	m.toLine() // a digit answers from the chat's line; a dead one types
	if m = pressOn(m, "1", "backspace"); armedOn(m, "1") {
		t.Errorf("a digit on the first frame marks an answer")
	}
	clock = clock.Add(peekDead - time.Millisecond)
	if m = pressOn(m, "1", "backspace"); armedOn(m, "1") {
		t.Errorf("a digit within %v of the first frame marks an answer", peekDead)
	}
	clock = clock.Add(time.Millisecond)
	if m = pressOn(m, "1", "esc"); len(sent(m)) != 0 {
		t.Fatalf("sent %+v", sent(m))
	}

	for _, keys := range [][]string{{"tab", "shift+tab"}, {"s"}, {"g", "esc"}} {
		clock = clock.Add(time.Second)
		m.toList()
		m = pressOn(m, keys...)
		if !m.boardShown() || chosenName(t, m) != "flint-auth" {
			t.Fatalf("after %v the console shows %v, chosen %s", keys, m.boardShown(), chosenName(t, m))
		}
		m.toLine()
		clock = clock.Add(peekDead - time.Millisecond)
		if m = pressOn(m, "1"); armedOn(m, "1") {
			t.Errorf("a digit within %v of %v marks an answer", peekDead, keys)
		}
		m = pressOn(m, "backspace")
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

// TestNoRowChosenActsOnNothing: with no row chosen, every act key of the
// list sends nothing and says why.
func TestNoRowChosenActsOnNothing(t *testing.T) {
	st := boardFixture(t)
	st.Agents = []string{"claude"}
	m := boardShot(t, st, 80, 31, func(m *Model) {
		m.filter = "zzzz"
		m.refresh()
		m.opt.Run = func(argv []string) error { t.Errorf("ran %v", argv); return nil }
	})
	if _, ok := m.sideChosen(); ok {
		t.Fatal("a search that matches nothing chose a row")
	}
	for _, k := range []string{"x", "r", "A", "p", "y", "f", "F", "o", "n", "z", "c", "m"} {
		next, cmd := key(m, k)
		drain(cmd)
		nm := next.(Model)
		if nm.pending != nil || nm.form != nil || nm.menu != nil || nm.tab != tabSessions || len(boardFake(nm).acts()) != 0 {
			t.Errorf("%s acted with no row: pending %v, form %v, menu %v, acts %+v", k, nm.pending, nm.form != nil, nm.menu != nil, boardFake(nm).acts())
		}
		if got := nm.noticeText; got != `nothing matches "zzzz" · esc clears` {
			t.Errorf("%s says %q", k, got)
		}
	}
	next, cmd := key(m, "ctrl+o")
	drain(cmd)
	if nm := next.(Model); nm.pending != nil || !strings.Contains(nm.noticeText, "no session to expand") {
		t.Errorf("ctrl+o with no row: pending %v, notice %q", nm.pending, nm.noticeText)
	}
	if nm := pressOn(m, "N"); nm.form == nil {
		t.Errorf("N opens no form with no row: %q", nm.flash)
	}
}

// TestATapNeverRunsADestructiveAct: a tap on a row only chooses it, and a
// tap on the consent box cancels it; only a typed y runs.
func TestATapNeverRunsADestructiveAct(t *testing.T) {
	m, _, _ := sideShot(t, 132, 32)
	m = tapOn(t, m, listRow(t, m, "ember-billing"))
	if m.consenting() || chosenName(t, m) != "ember-billing" {
		t.Fatalf("a tap on a row: consent %v, chosen %s", m.consenting(), chosenName(t, m))
	}
	m.toList()
	if m = pressOn(m, "p"); !m.consenting() || !strings.Contains(consentText(m), "gh pr merge 4552") {
		t.Fatalf("p on ember-billing: consent %v", m.consenting())
	}
	next, cmd := tap(m, 40, 15)
	drain(cmd)
	m = next.(Model)
	if m.pending != nil || len(boardFake(m).acts()) != 0 {
		t.Errorf("a tap on the box did not cancel it: %v %+v", m.pending, boardFake(m).acts())
	}
}

// TestDigitInTheListMarksWhenTheLineIsEmpty: a digit pressed in the list
// goes to the line and is read there: with the line empty and a question
// on the screen it marks, and the same digit sends; else it types.
func TestDigitInTheListMarksWhenTheLineIsEmpty(t *testing.T) {
	m, clock := flintBoard(t, 80, 31)
	*clock = clock.Add(peekDead)
	m.toList()
	if m = pressOn(m, "1"); !armedOn(m, "1") || m.lines[m.chosenSide()] != "" || m.inList() {
		t.Fatalf("a digit in the list: armed %v, line %q, in the list %v", armedOn(m, "1"), m.lines[m.chosenSide()], m.inList())
	}
	m.toList()
	if m = pressOn(m, "1"); len(sent(m)) != 1 {
		t.Errorf("the same digit from the list again sent %+v", sent(m))
	}
	n, _ := flintBoard(t, 80, 31) // the question just showed: the digit types
	n.peekAt = n.now()
	n.toList()
	if n = pressOn(n, "1"); armedOn(n, "1") || n.lines[n.chosenSide()] != "1" {
		t.Errorf("a dead digit in the list: armed %v, line %q", armedOn(n, "1"), n.lines[n.chosenSide()])
	}
}
