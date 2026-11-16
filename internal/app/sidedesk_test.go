package app

import (
	"strings"
	"testing"
	"time"
)

// TestAnUprightDeskPaneShowsTheListBesideTheChat: half a 16-inch laptop
// screen is 107x60, as tall as an iPhone Pro Max held upright. On a desk
// the list and the chat show side by side; Pocket on the phone shows one
// pane at a time.
func TestAnUprightDeskPaneShowsTheListBesideTheChat(t *testing.T) {
	st := boardState(boardMatch("flint-auth", time.Hour), boardMatch("hearth-api", time.Hour))
	desk := boardShot(t, st, 107, 60, nil)
	if !desk.upright() || desk.onePane(107) {
		t.Fatalf("desk 107x60: upright %v, one pane %v", desk.upright(), desk.onePane(107))
	}
	if b := desk.sideBoxAt(107); b.listW != listMedium || b.chatW != 107-listMedium-1 {
		t.Fatalf("desk 107x60: box %+v, want the list %d wide beside the chat", b, listMedium)
	}
	if got := screen(desk); !strings.Contains(got, "│") {
		t.Fatalf("desk 107x60 draws no rule between the list and the chat:\n%s", got)
	}

	pocket := boardShot(t, st, 107, 60, pocketOnly)
	if !pocket.onePane(107) {
		t.Fatalf("Pocket 107x60 shows two panes:\n%s", screen(pocket))
	}
}
