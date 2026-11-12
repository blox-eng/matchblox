package app

import (
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

// TestBackStepsOneLevelPerPress: the home key (◂, ⌘M, ctrl+M, prefix m)
// at the console steps back one level: typing → the room → the list.
func TestBackStepsOneLevelPerPress(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, _ := roomWith(t, st, "hearth-api")
	m, _ = send(t, m, "enter") // the talk line types
	back := func(m Model) Model {
		next, cmd := m.Update(uv.UnknownApcEvent(homeKey))
		drain(cmd)
		return next.(Model)
	}
	if m = back(m); m.room == nil || m.room.typing {
		t.Fatalf("one press: %+v", m.room)
	}
	if m = back(m); m.room != nil {
		t.Fatalf("two presses: %+v", m.room)
	}
}
