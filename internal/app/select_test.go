package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// TestTheSelectionIsUnmistakable: in every layout the selected row is on
// the wash across its whole pane, with ▌ and its name in the accent, and
// every other session's name is muted. On a one-column phone the opened
// block stands between hairlines.
func TestTheSelectionIsUnmistakable(t *testing.T) {
	for _, sz := range [][2]int{{44, 31}, {86, 30}, {107, 60}, {132, 32}} {
		w, h := sz[0], sz[1]
		m := loaded(t, w)
		next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
		m = next.(Model)
		lw, _ := m.panes(w)
		sel, _ := m.selectedSession()
		lines := parsed(t, m.render())
		found := false
		for y, line := range lines {
			x, washed, text := 0, 0, ""
			for _, s := range line {
				if x < lw {
					text += s.Text
					if s.BG == "wash" {
						washed += lipgloss.Width(s.Text)
					}
				}
				x += lipgloss.Width(s.Text)
			}
			for _, s := range line {
				for _, other := range m.all.sessions {
					if other.Name != sel.Name && strings.TrimSpace(s.Text) == other.Name && s.FG != "muted" {
						t.Fatalf("%dx%d: %s is %q, not muted", w, h, other.Name, s.FG)
					}
				}
			}
			if !strings.HasPrefix(text, "▌") || !strings.Contains(text, sel.Name) {
				continue
			}
			found = true
			if washed != lw {
				t.Fatalf("%dx%d: the selected row is washed over %d of %d cells", w, h, washed, lw)
			}
			if line[0].FG != "accent" {
				t.Fatalf("%dx%d: ▌ is %q", w, h, line[0].FG)
			}
			for _, s := range line {
				if strings.TrimSpace(s.Text) == sel.Name && s.FG != "accent" {
					t.Fatalf("%dx%d: the selected name is %q", w, h, s.FG)
				}
			}
			if m.layoutAt(m.width) == Narrow {
				above := lines[y-1]
				if len(above) == 0 || above[0].FG != "hair" || !strings.HasPrefix(above[0].Text, "────") {
					t.Fatalf("%dx%d: no hairline above the opened block", w, h)
				}
				end := y + len(m.sessionRow(sel, true, lw))
				if below := lines[end]; len(below) == 0 || below[0].FG != "hair" || !strings.HasPrefix(below[0].Text, "────") {
					t.Fatalf("%dx%d: no hairline under the opened block", w, h)
				}
			}
		}
		if !found {
			t.Fatalf("%dx%d: no selected row", w, h)
		}
	}
}
