package app

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// TestARowShowsItsNumber: a numbered session draws its number in a faint
// column left of the match; one with no number draws a space there, so the
// names stay aligned. The chosen row keeps its ▌.
func TestARowShowsItsNumber(t *testing.T) {
	s := boardMatch("hearth-api", time.Hour)
	s.Number = 3
	m := boardModel(t, boardState(s))
	numbered := m.all.sessions[0]
	bare := numbered
	bare.Number = 0

	row := m.sideRow(numbered, 40, false, sideLink{})[0]
	if !strings.HasPrefix(row, " "+m.st.faint.Render("3")+" ") {
		t.Fatalf("row %q, want a faint 3 after the mark", row)
	}
	got, want := ansi.Strip(m.sideRow(bare, 40, false, sideLink{})[0]), ansi.Strip(row)
	if i, j := strings.Index(got, "hearth-api"), strings.Index(want, "hearth-api"); i != j || !strings.HasPrefix(got, "   ") {
		t.Fatalf("a row with no number %q, want its name aligned with %q", got, want)
	}
	chosen := plainLines(m.sideRow(numbered, 40, true, sideLink{}))
	if !strings.HasPrefix(chosen[0], "▌3 ") {
		t.Fatalf("chosen row %q, want ▌ then 3", chosen[0])
	}
}
