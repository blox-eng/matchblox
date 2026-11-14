package app

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
)

// roomWith is the console on st with the room of name's chain open, and
// its conn.
func roomWith(t *testing.T, st proto.State, name string) (Model, *fakeConn) {
	t.Helper()
	m, f := headModel(t, 160, st)
	_, k := roomOf(t, st, name)
	for _, s := range m.all.sessions {
		if s.Name == name && k.isOne() {
			k = oneKey(s)
		}
	}
	return m.openRoom(k), f
}

func at(t *testing.T, m Model, kind roomKind, has string) Model {
	t.Helper()
	for i, r := range m.roomRows(m.room.k) {
		if r.kind == kind && strings.Contains(r.text, has) {
			m.room.at = i
			return m
		}
	}
	t.Fatalf("no row %d %q", kind, has)
	return m
}

func send(t *testing.T, m tea.Model, keys ...string) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		switch k {
		case "enter":
			m, cmd = press(m, tea.KeyEnter)
		case "esc":
			m, cmd = press(m, tea.KeyEscape)
		case "down":
			m, cmd = press(m, tea.KeyDown)
		default:
			m, cmd = m.Update(tea.KeyPressMsg{Code: []rune(k)[0], Text: k})
		}
		drain(cmd)
	}
	return m.(Model), cmd
}

// aGo makes the unit's step 3 wait for a go.
func aGo(v proto.StokerView, needs, want string) proto.StokerView {
	v.Run = append(v.Run, proto.StokerEntry{ID: "e9", Step: 3, State: "waits", Needs: needs, Want: want, Text: "rebase ember-billing", At: demoAt.Add(-time.Minute)})
	return v
}

func TestYOnAWaitingStepGivesItsGo(t *testing.T) {
	st := headState(t, aGo(runningPlan(), "merge:#4556", ""), true)
	st.Queue = nil
	m, f := roomWith(t, st, "hearth-api")
	m = at(t, m, roomStep, "▸ step · waits for your go")
	if keys := ansi.Strip(m.render()); !strings.Contains(keys, "y give") {
		t.Fatalf("no y on a waiting step:\n%s", keys)
	}
	next, _ := send(t, m, "y")
	if next.pending == nil {
		t.Fatalf("y on a waiting step: flash %q", next.flash)
	}
	send(t, next, "y")
	if a := f.acts(); len(a) != 1 || a[0].Text != "s1 e9" {
		t.Fatalf("acts %+v", a)
	}
}

func TestAGoThatMergesAsksInTheConsentBox(t *testing.T) {
	st := headState(t, aGo(runningPlan(), "merge:#4556", "gh pr merge 4556 --squash"), true)
	st.Queue = nil
	m, f := roomWith(t, st, "hearth-api")
	next, _ := send(t, m, "y")
	if len(f.acts()) != 0 || next.pending == nil || !next.pending.destructive {
		t.Fatalf("a merge go sent %+v, pending %+v", f.acts(), next.pending)
	}
	if got := next.pending.steps; len(got) != 1 || strings.Join(got[0], " ") != "gh pr merge 4556 --squash" {
		t.Fatalf("argv %q", got)
	}
	send(t, next, "y")
	if a := f.acts(); len(a) != 1 || a[0].RecID != "stoker:grant" || a[0].Confirm != "y" || a[0].Text != "s1 e9" {
		t.Fatalf("acts %+v", a)
	}
}

func TestTypingTakesEveryKeyUntilEnterOrEsc(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, f := roomWith(t, st, "hearth-api")
	m, _ = send(t, m, "enter", "y", "]", "q")
	if !m.room.typing || m.room.draft != "y]q" || len(f.acts()) != 0 {
		t.Fatalf("typing %v draft %q acts %+v", m.room.typing, m.room.draft, f.acts())
	}
	if keys := ansi.Strip(m.render()); !strings.Contains(keys, "typing to hearth's stoker · ⏎ sends · esc stops") {
		t.Fatalf("key line:\n%s", keys)
	}
	m, _ = send(t, m, "esc")
	if m.room == nil || m.room.typing || m.room.draft != "y]q" {
		t.Fatalf("esc: %+v", m.room)
	}
}

func TestTalkStartsAStokerWithNoSheet(t *testing.T) {
	m, f := roomWith(t, fuseState(t, mockupChain), "hearth-api")
	m, _ = send(t, at(t, m, roomTalk, "›"), "enter", "s", "h", "i", "p", "enter")
	if a := f.acts(); len(a) != 1 || a[0].RecID != "stoker:new" || a[0].Text != `{"chain":"c1","words":"ship","grants":[]}` {
		t.Fatalf("acts %+v", a)
	}
	if m.sheet != nil {
		t.Fatal("a sheet opened")
	}
}

func TestTalkToARunningStokerSendsAnOrder(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, f := roomWith(t, st, "hearth-api")
	send(t, m, "enter", "g", "o", "enter")
	if a := f.acts(); len(a) != 1 || a[0].RecID != "stoker:order-chain" || a[0].Text != `{"chain":"c1","words":"go","grants":[]}` {
		t.Fatalf("acts %+v", a)
	}
}

func TestTalkSendsOneOrderAtATime(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, f := roomWith(t, st, "hearth-api")
	m, _ = send(t, m, "enter", "g", "o", "enter", "enter", "x", "enter")
	if a := f.acts(); len(a) != 1 {
		t.Fatalf("acts %+v: a second order while the first is in flight", a)
	}
	next, _ := m.Update(resultMsg(proto.Result{ActID: m.room.act}))
	if m = next.(Model); m.room.act != "" || m.room.draft != "x" {
		t.Fatalf("after the result: %+v", m.room)
	}
}

func TestTalkSentClearsAndSays(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, _ := roomWith(t, st, "hearth-api")
	m, _ = send(t, m, "enter", "g", "o", "enter")
	next, _ := m.Update(resultMsg(proto.Result{ActID: m.room.act}))
	if m = next.(Model); m.room.draft != "" || saidNow(m) != "sent to hearth's stoker" {
		t.Fatalf("draft %q toast %q", m.room.draft, saidNow(m))
	}
}

func TestTalkRefusalKeepsTheDraft(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, _ := roomWith(t, st, "hearth-api")
	m, _ = send(t, m, "enter", "g", "o", "enter")
	next, _ := m.Update(resultMsg(proto.Result{ActID: m.room.act, Err: "the night is on"}))
	if m = next.(Model); m.room.draft != "go" || m.flash != "not sent: the night is on" {
		t.Fatalf("draft %q flash %q", m.room.draft, m.flash)
	}
}

func TestBracketsWalkTheChains(t *testing.T) {
	m, _ := roomWith(t, fuseState(t, mockupChain, []string{"flint-auth", "spark-docs"}), "hearth-api")
	first := m.room.k
	m, _ = send(t, m, "]")
	if m.room.k == first || m.room.k.id != "c2" {
		t.Fatalf("] went to %+v", m.room.k)
	}
	m, _ = send(t, m, "[")
	if m.room.k != first {
		t.Fatalf("[ went to %+v", m.room.k)
	}
}

func TestEscStepsBackOneLevel(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, _ := roomWith(t, st, "hearth-api")
	m, _ = send(t, m, "enter", "esc")
	if m.room == nil || m.room.typing {
		t.Fatalf("esc while typing: %+v", m.room)
	}
	if m, _ = send(t, m, "esc"); m.room != nil {
		t.Fatal("esc in the room keeps it open")
	}
}

// TestTalkInAChainOfOneStartsTheHostStoker: with the host stoker off, ⏎
// starts it, and the order goes once it is ready (eng review correction 9).
func TestTalkInAChainOfOneStartsTheHostStoker(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue, st.Stoker = nil, &proto.StokerView{}
	m, f := roomWith(t, st, "spark-docs")
	m, _ = send(t, at(t, m, roomTalk, "›"), "enter", "g", "o", "enter")
	if a := f.acts(); len(a) != 1 || a[0].RecID != "stoker:start" {
		t.Fatalf("acts %+v", a)
	}
	st.Stoker = &proto.StokerView{PaneUp: true, Ready: true}
	next, cmd := m.Update(stateMsg(st))
	drain(cmd)
	if a := f.acts(); len(a) != 2 || a[1].RecID != "stoker:order" || a[1].Text != `{"words":"go","night":false}` {
		t.Fatalf("acts %+v", a)
	}
	_ = next
}

// TestUTakesAMatchOutOfTheChain: out of the chain asks first, with the
// act named, and a typed y takes it out. u is the board's strip: the act
// is the room's card menu's (TestTheMenuTakesAMatchOutOfItsChain).
func TestUTakesAMatchOutOfTheChain(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, f := roomWith(t, st, "hearth-api")
	s := matchOf(t, m, "ember-billing")
	m = outOfChain(m, s, menuLine(m, s, "out of hearth"))
	if len(f.acts()) != 0 || m.pending == nil || !m.pending.destructive || m.pending.label != "take ember-billing out of hearth" {
		t.Fatalf("acts %+v pending %+v", f.acts(), m.pending)
	}
	send(t, m, "y")
	if a := f.acts(); len(a) != 1 || a[0].RecID != "chain:out:%8@hearth" || a[0].Confirm != "y" {
		t.Fatalf("acts %+v", a)
	}
}

// TestUOnAChainOfOneSaysThereIsNoChain: a chain of one's room offers no
// way out of a chain: its card's menu has no out of line.
func TestUOnAChainOfOneSaysThereIsNoChain(t *testing.T) {
	m, f := roomWith(t, headState(t, runningPlan(), true), "spark-docs")
	s := matchOf(t, m, "spark-docs")
	if !m.room.k.isOne() || slices.ContainsFunc(m.menuFor(s), func(it menuItem) bool { return strings.HasPrefix(it.label, "out of") }) {
		t.Fatalf("a chain of one's menu: %q", itemLabels(m.menuFor(s)))
	}
	if len(f.acts()) != 0 {
		t.Fatalf("acts %+v", f.acts())
	}
}

// TestASecondFlipKeepsTheRoom: ⌥s into the room already open keeps the
// draft and the order on its way (spec: "the draft stays").
func TestASecondFlipKeepsTheRoom(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, f := roomWith(t, st, "hearth-api")
	m, _ = send(t, m, "enter", "g", "o", "enter", "x", "esc")
	act := m.room.act
	m = m.flipFrom("/dev/pts/1", paneNamed(st, "hearth-api"))
	if m.room.act != act || act == "" || m.room.draft != "x" {
		t.Fatalf("after the flip: %+v", m.room)
	}
	if _, _ = send(t, m, "enter"); len(f.acts()) != 1 {
		t.Fatalf("a second order went while the first is on its way: %+v", f.acts())
	}
}

// TestTalkToAHostThatIsNotConnectedKeepsTheWords: nothing goes, the line
// says why, and the words stay.
func TestTalkToAHostThatIsNotConnectedKeepsTheWords(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, f := roomWith(t, st, "hearth-api")
	m.lost = true
	m, _ = send(t, m, "enter", "g", "o", "enter")
	if len(f.acts()) != 0 || m.room.act != "" || m.room.draft != "go" || !strings.HasPrefix(m.flash, "not sent: ") {
		t.Fatalf("acts %+v room %+v flash %q", f.acts(), m.room, m.flash)
	}
}

// TestALostConnFreesTheTalkLine: an order whose conn is lost has no
// answer: its words come back, and the line takes the next.
func TestALostConnFreesTheTalkLine(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, _ := roomWith(t, st, "hearth-api")
	m, _ = send(t, m, "enter", "g", "o", "enter")
	next, _ := m.Update(lostMsg{errors.New("eof"), m.conn})
	if m = next.(Model); m.room.act != "" || m.room.draft != "go" {
		t.Fatalf("after the loss: %+v", m.room)
	}
}

// TestAStartThatFailsSaysWhyOnTheCard: a chain of one whose host stoker
// does not start says why, and the words come back.
func TestAStartThatFailsSaysWhyOnTheCard(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue, st.Stoker = nil, &proto.StokerView{}
	m, f := roomWith(t, st, "spark-docs")
	m, _ = send(t, at(t, m, roomTalk, "›"), "enter", "g", "o", "enter")
	if a := f.acts(); len(a) != 1 || a[0].RecID != "stoker:start" {
		t.Fatalf("acts %+v", a)
	}
	next, _ := m.Update(resultMsg(proto.Result{ActID: m.room.act, Err: "no agent on the PATH"}))
	m = next.(Model)
	if m.room.queued || m.room.draft != "go" {
		t.Fatalf("after the refusal: %+v", m.room)
	}
	roomRowOf(t, m.roomRows(m.room.k), roomCard, "no agent on the PATH")
}
