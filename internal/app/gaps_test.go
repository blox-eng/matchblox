package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestNoBlankRowsWhileThereIsMore: on the demo machine, every tab at every
// size leaves no run of three blank rows in the body while the dock or the
// right pane has more to say: they grow into free rows first.
func TestNoBlankRowsWhileThereIsMore(t *testing.T) {
	st := screensState(t)
	for _, size := range [][2]int{{44, 31}, {107, 60}, {107, 101}, {132, 32}} {
		w, h := size[0], size[1]
		for tab := range tabNames {
			m := New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
			next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			next, _ = next.Update(stateMsg(st))
			nm := next.(Model)
			nm.setTab(tab)
			sc := nm.draw(w)
			end := len(sc.lines) - 2 // the key line's hairline
			if sc.dockAt >= 0 {
				end = sc.dockAt - 1
			}
			run := 0
			for i := sc.bodyTop; i < end; i++ {
				if strings.Trim(ansi.Strip(sc.lines[i]), " │") == "" {
					run++
				} else {
					run = 0
				}
				if run >= 3 && (!sc.full || !sc.paneFull) {
					t.Fatalf("%dx%d %s: three blank rows from line %d while the dock (all %v) or the pane (all %v) has more:\n%s",
						w, h, tabNames[tab], i-2, sc.full, sc.paneFull, ansi.Strip(strings.Join(sc.lines, "\n")))
				}
			}
		}
	}
}
