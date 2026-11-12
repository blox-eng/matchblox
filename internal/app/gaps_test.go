package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestNoBlankRowsWhileThereIsMore: on the demo machine, every tab at every
// size leaves no run of three blank rows in the body while the right pane
// has more to say: it grows into free rows first.
func TestNoBlankRowsWhileThereIsMore(t *testing.T) {
	st := screensState(t)
	for _, size := range [][2]int{{44, 31}, {107, 60}, {107, 101}, {132, 32}} {
		w, h := size[0], size[1]
		for tab := range screenNames() { // the tabs and Machine's views
			if tab == tabSessions {
				continue // the board's columns end where their cards do (TestBoardKeepsTheChosenCardWhole)
			}
			m := New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
			next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			next, _ = next.Update(stateMsg(st))
			nm := next.(Model)
			nm.setTab(tab)
			sc := nm.draw(w)
			end := len(sc.lines) - 2 // the key line's hairline
			run := 0
			for i := sc.bodyTop; i < end; i++ {
				if strings.Trim(ansi.Strip(sc.lines[i]), " │") == "" {
					run++
				} else {
					run = 0
				}
				if run >= 3 && !sc.paneFull {
					t.Fatalf("%dx%d %s: three blank rows from line %d while the pane has more:\n%s",
						w, h, screenNames()[tab], i-2, ansi.Strip(strings.Join(sc.lines, "\n")))
				}
			}
		}
	}
}
