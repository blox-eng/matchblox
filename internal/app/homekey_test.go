package app

import (
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

// The home key (Ctrl+M, prefix m, the phone's back button) in the console
// goes back as Esc does: a filter clears first, then the hosts list shows
// (#360).
func TestTheHomeKeyGoesBackAsEscDoes(t *testing.T) {
	t.Skip("replaced by TestTheHomeKeyActsAsTheBoardsEsc (Task 12 step 1): on the board the home key clears the search, then the chain, and stays")
	m, _ := loadedWith(t, 100, fixtureState())
	m.opt.Layered = true
	m.filter = "app"
	next, cmd := m.Update(uv.UnknownApcEvent(homeKey))
	if got := next.(Model).filter; got != "" {
		t.Fatalf("the home key left the filter %q", got)
	}
	if cmd != nil {
		if _, toHosts := cmd().(layerMsg); toHosts {
			t.Fatal("the home key went to the hosts with a filter on")
		}
	}
	_, cmd = next.Update(uv.UnknownApcEvent(homeKey))
	if cmd == nil {
		t.Fatal("the home key did nothing on the main screen")
	}
	if _, toHosts := cmd().(layerMsg); !toHosts {
		t.Fatal("the home key did not go back to the hosts")
	}
}
