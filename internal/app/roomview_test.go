package app

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// accentSGR is how the dark theme's accent starts in a frame.
func accentLines(m Model, lines []string) []string {
	sgr := strings.TrimSuffix(m.st.accent.Render("x"), "x\x1b[m")
	var out []string
	for _, l := range lines {
		if strings.Contains(l, sgr) {
			out = append(out, ansi.Strip(l))
		}
	}
	return out
}

func TestRoomHasOneAccent(t *testing.T) {
	st := headState(t, runningPlan(), true)
	twoAsk(&st)
	m, k := roomOf(t, st, "hearth-api")
	m = m.openRoom(k)
	m.room.at = len(m.roomRows(k)) - 1 // the cursor off the need line
	got := accentLines(m, m.roomView(60, 30))
	if len(got) != 1 || !strings.Contains(got[0], "hearth-api asks") {
		t.Fatalf("accent lines %q, want the need line only", got)
	}
}

func TestRoomWithNoNeedAccentsTheSelectedRow(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, k := roomOf(t, st, "hearth-api")
	m = m.openRoom(k)
	got := accentLines(m, m.roomView(60, 30))
	if len(got) != 1 || !strings.HasPrefix(got[0], "▌› tell hearth's stoker…") {
		t.Fatalf("accent lines %q, want the selected talk line", got)
	}
}

func TestRoomAt40ColumnsTruncatesLongNames(t *testing.T) {
	st := headState(t, runningPlan(), true)
	twoAsk(&st)
	long := strings.Repeat("hearth", 7) + "-tool" // 47 characters
	st.Chains.Chains[0].Name = long
	for i := range st.Queue {
		st.Queue[i].LastLine = strings.Repeat("may I force-push the rebase onto main now? ", 4)
	}
	m, k := roomOf(t, st, "hearth-api")
	m = m.openRoom(k)
	lines := m.roomView(40, 30)
	need := 0
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > 40 {
			t.Errorf("%d columns: %q", w, ansi.Strip(l))
		}
		if strings.Contains(ansi.Strip(l), "force-push") {
			need++
		}
	}
	if need == 0 || need > 2 {
		t.Fatalf("the need takes %d lines, want 1 or 2:\n%s", need, ansi.Strip(strings.Join(lines, "\n")))
	}
	title := ansi.Strip(m.roomTitle(40))
	if ansi.StringWidth(title) > 40 || !strings.HasSuffix(strings.TrimRight(title, " "), "…") {
		t.Fatalf("title %q", title)
	}
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
// so its card's ≋ takes the night ink; the console wears no night.
func TestAChainStokerInChargePaintsOnlyItsGlyph(t *testing.T) {
	m, k := roomOf(t, headState(t, runningPlan(), true), "hearth-api")
	m = m.openRoom(k)
	if m.night() {
		t.Fatal("a chain stoker's run puts the console in the night")
	}
	ink := strings.TrimSuffix(lipgloss.NewStyle().Foreground(nightInk).Render("x"), "x\x1b[m")
	var lit []string
	for _, l := range m.roomView(60, 30) {
		if strings.Contains(l, ink) {
			lit = append(lit, ansi.Strip(l))
		}
	}
	if len(lit) != 1 || !strings.Contains(lit[0], "≋ \"ship the chain\"") {
		t.Fatalf("night ink on %q, want the card's ≋ only", lit)
	}
}
