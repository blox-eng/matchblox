package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestEnterOnAChainHeadOpensItsRoom(t *testing.T) {
	t.Skip("replaced by TestGNarrowsToTheChain (Task 12 step 1)")
	m, _ := headModel(t, 160, headState(t, runningPlan(), true))
	m = pick(t, m, stokerPane)
	next, _ := press(m, tea.KeyEnter)
	if r := next.(Model).room; r == nil || r.k.id != "c1" {
		t.Fatalf("room %+v", r)
	}
}

func TestFlipOnALooseSessionOpensItsChainOfOne(t *testing.T) {
	m, _ := headModel(t, 160, headState(t, runningPlan(), true))
	m = pick(t, m, "%2") // flint-auth, in no chain
	next, _ := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModAlt})
	if r := next.(Model).room; r == nil || !r.k.isOne() || r.k.id != "=%2" {
		t.Fatalf("room %+v", r)
	}
}

func TestFlipOnAChainedSessionOpensItsRoom(t *testing.T) {
	m, _ := headModel(t, 160, headState(t, runningPlan(), true))
	m = pick(t, m, "%1") // hearth-api
	next, _ := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModAlt})
	if r := next.(Model).room; r == nil || r.k.id != "c1" {
		t.Fatalf("room %+v", r)
	}
}

// TestRoomScreens: the room on a desk, a medium screen and a phone, with a
// need, the card and a go that waits: ⌥s on a match of the chain opens it.
func TestRoomScreens(t *testing.T) {
	st := headState(t, aGo(runningPlan(), "rebase", ""), true)
	twoAsk(&st)
	for _, size := range [][2]int{{110, 30}, {80, 30}, {40, 24}} {
		check(t, st, "room", size[0], size[1], []string{"alt+s"})
	}
}
