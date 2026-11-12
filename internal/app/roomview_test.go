package app

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRoomHasOneAccent(t *testing.T) {
	t.Skip("replaced by TestANarrowedChainIsItsRoom (Task 12 step 6d): roomView goes; one accent in the narrowed board")
}

func TestRoomWithNoNeedAccentsTheSelectedRow(t *testing.T) {
	t.Skip("replaced by TestANarrowedChainIsItsRoom (Task 12 step 6d): roomView goes; the talk line or the chosen card holds the accent")
}

func TestRoomAt40ColumnsTruncatesLongNames(t *testing.T) {
	t.Skip("replaced by TestANarrowedChainIsItsRoom (Task 12 step 6d): roomView goes; the narrowed board fits its screen (R3.12)")
}

func TestRoomShowsInPlaceOfTheList(t *testing.T) {
	t.Skip("replaced by TestANarrowedChainIsItsRoom (Task 12 step 1)")
	st := headState(t, runningPlan(), true)
	twoAsk(&st)
	m, k := roomOf(t, st, "hearth-api")
	m = m.openRoom(k)
	f := ansi.Strip(m.render())
	for _, want := range []string{"‹ ≋ hearth", "hearth-api asks: may I force-push the rebase?", "› tell hearth's stoker…", "SHIPS reading…", "GIT   reading…"} {
		if !strings.Contains(f, want) {
			t.Errorf("no %q in:\n%s", want, f)
		}
	}
	if strings.Contains(f, "WAITS FOR YOU") {
		t.Errorf("the Sessions list shows under the room:\n%s", f)
	}
}

// TestAChainStokerInChargePaintsOnlyItsGlyph: the chain stoker's run is on,
// so its card's ≋ takes the night ink, in the frame of the narrowed board;
// the console wears no night.
func TestAChainStokerInChargePaintsOnlyItsGlyph(t *testing.T) {
	m, k := roomOf(t, headState(t, runningPlan(), true), "hearth-api")
	m = m.openRoom(k)
	if m.night() {
		t.Fatal("a chain stoker's run puts the console in the night")
	}
	ink := strings.TrimSuffix(lipgloss.NewStyle().Foreground(nightInk).Render("x"), "x\x1b[m")
	var lit []string
	for _, l := range strings.Split(m.render(), "\n") {
		if strings.Contains(l, ink) {
			lit = append(lit, ansi.Strip(l))
		}
	}
	if len(lit) != 1 || !strings.Contains(lit[0], "≋ \"ship the chain\"") {
		t.Fatalf("night ink on %q, want the card's ≋ only", lit)
	}
}
