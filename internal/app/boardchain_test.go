package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// The board's tests of the chains and their rooms (Task 12 step 0c).

// chainBoard is the board of st at w columns, on the demo's clock, with
// the card of the match named name chosen ("" for the board's choice).
func chainBoard(t *testing.T, w int, st proto.State, name string) (Model, *fakeConn) {
	t.Helper()
	m, f := headModel(t, w, st)
	m = boardOn(m)
	if name != "" {
		s := matchOf(t, m, name)
		m.boardChooseCard(m.columnOf(s), s)
	}
	return m, f
}

// stubOf is the chain's stub on the card of the match named name, ""
// for none.
func stubOf(t *testing.T, m Model, name string) string {
	t.Helper()
	lines, _ := m.boardCard(matchOf(t, m, name), 40, cardOpts{})
	for _, l := range plainLines(lines) {
		if i := strings.IndexAny(l, "─━"); i >= 0 && strings.ContainsAny(l, "╴╸") {
			return strings.TrimSpace(l[i:])
		}
	}
	return ""
}

// TestACardOfAChainOfOneHasNoStub: a chain with one match here draws no
// chain mark; with two, each card has its stub.
func TestACardOfAChainOfOneHasNoStub(t *testing.T) {
	st := fuseState(t, mockupChain)
	left := func(gone ...string) proto.State {
		out := st
		out.Sessions = slices.DeleteFunc(slices.Clone(st.Sessions), func(s sample.Session) bool { return slices.Contains(gone, s.Name) })
		return out
	}
	m, _ := chainBoard(t, 160, left("hearth-api", "ember-billing"), "")
	if stub := stubOf(t, m, "kindle-web"); stub != "" {
		t.Errorf("a chain of one has the stub %q", stub)
	}
	if names := m.boardChains(); len(names) != 0 {
		t.Errorf("a chain of one is a chain of the board: %v", names)
	}
	m, _ = chainBoard(t, 160, left("ember-billing"), "")
	for _, name := range []string{"kindle-web", "hearth-api"} {
		if stub := stubOf(t, m, name); stub == "" {
			t.Errorf("%s's card in a chain of two has no stub", name)
		}
	}
}

// TestFOnACardOfABareChainOpensTheSheet: F on a card of a chain with no
// stoker opens its start sheet; on a card in no chain it opens nothing;
// at night it says the night is sealed and sends nothing.
func TestFOnACardOfABareChainOpensTheSheet(t *testing.T) {
	m, _ := chainBoard(t, 160, fuseState(t, mockupChain), "hearth-api")
	if s := plainFrame(sendKey(m, "F").(Model)); !strings.Contains(s, "Stoke the chain kindle-web (3 matches)?") {
		t.Fatalf("F on a card of a bare chain opened no sheet:\n%s", s)
	}
	m, _ = chainBoard(t, 160, fuseState(t, mockupChain), "flint-auth")
	if next := sendKey(m, "F").(Model); next.sheet != nil {
		t.Error("F opened a sheet on a card in no chain")
	}

	st := fuseState(t, mockupChain)
	host := proto.StokerView{}
	host.On, host.Nightly = true, true
	st.Stoker = &host
	m, f := chainBoard(t, 160, st, "hearth-api")
	if next := sendKey(m, "F").(Model); next.sheet != nil || next.flash != sealedFamily || len(f.acts()) != 0 {
		t.Errorf("F at night: sheet %v, flash %q, acts %+v", next.sheet, next.flash, f.acts())
	}
}

// TestTheRoomCardCountsTheOrdersSteps: the room's stoker card counts the
// steps of the order in flight from its go: a step done before the go is
// no step of it.
func TestTheRoomCardCountsTheOrdersSteps(t *testing.T) {
	marks := func(v proto.StokerView) string {
		m, k := roomOf(t, headState(t, v, true), "kindle-web")
		var out []string
		for _, r := range m.cardRows(k) {
			if r.kind == roomStep {
				out = append(out, r.id+r.text[:strings.Index(r.text, " ")])
			}
		}
		return strings.Join(out, " ")
	}
	v := runningPlan() // steps 1 and 2 done, an hour ago
	if got := marks(v); got != "2✓ 3▸ 4" {
		t.Errorf("the card's steps: %q, want 2✓ 3▸ 4", got)
	}
	v = runningPlan()
	v.Order.Went = demoAt.Add(-30 * time.Minute) // the order went again after them
	if got := marks(v); got != "1▸ 2" {
		t.Errorf("after a new go the card's steps: %q, want the first step active", got)
	}
}

// The tests the map names that need a later step of Task 12.

func TestTheConsoleOpensOnTheBoard(t *testing.T) {
	t.Skip("needs Task 12 step 1: the board is the home")
}

func TestTheTabRowIsBoardAndMachine(t *testing.T) {
	t.Skip("needs Task 12 step 6c: the tab row is board · machine")
}

func TestARoomOpensOnTheNeedElseTheTalkLine(t *testing.T) {
	t.Skip("needs Task 12 step 1: the room is the narrowed board, with the talk line")
}

func TestTheRoomsCursorWalksCardColumnsAndTalk(t *testing.T) {
	t.Skip("needs Task 12 step 1: the room is the narrowed board, with the stoker's card and the talk line")
}

func TestACardSaysItsChainMates(t *testing.T) {
	t.Skip("needs Task 12 step 6d: the room's match rows (inbox, repo, also in) leave for the cards")
}

func TestTheMenuTakesAMatchOutOfItsChain(t *testing.T) {
	t.Skip("needs Task 12 step 6d: u leaves the room for the card's menu, which has no destructive line today")
}
