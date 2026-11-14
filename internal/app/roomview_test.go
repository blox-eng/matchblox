package app

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

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
