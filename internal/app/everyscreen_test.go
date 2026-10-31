package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// flintAt is the demo machine at w by h with flint-auth selected, its
// screen the question tool's, as the chat goldens draw it, after keys.
func flintAt(t *testing.T, w, h int, keys ...string) Model {
	t.Helper()
	st := screensState(t)
	m := New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	next, _ = next.Update(stateMsg(st))
	nm := next.(Model)
	nm.splashDone = true
	for _, s := range nm.snap.Sessions {
		if s.Name == "flint-auth" {
			nm.chose, nm.selPID = true, s.PID
		}
	}
	nm.watchCmd()
	next, _ = nm.Update(screenMsg{Pane: paneNamed(st, "flint-auth"), At: demoAt, Text: askScreen})
	return pressKeys(t, next.(Model), keys...)
}

// TestSessionsSplitsFortySixty: where Sessions splits, the list takes 40%
// of the width (38 to 78 columns) and the chat the rest, full height, with
// usage and the machine under the list and no dock at the bottom, upright
// or not. The other tabs keep their splits.
func TestSessionsSplitsFortySixty(t *testing.T) {
	for _, c := range []struct{ w, h, list int }{{95, 65, 38}, {86, 30, 38}, {132, 32, 52}, {200, 50, 78}, {110, 65, 44}} {
		m := fullView(flintAt(t, c.w, c.h))
		sc := m.draw(c.w)
		if list, right := m.panes(c.w); list != c.list || right != c.w-c.list-1 || sc.dockAt >= 0 {
			t.Errorf("%dx%d: list %d, chat %d, dock at %d; want list %d and no dock", c.w, c.h, list, right, sc.dockAt, c.list)
			continue
		}
		for _, word := range []string{"USAGE", "MACHINE"} {
			if y := lineOf(t, m, word); !strings.Contains(ansi.Strip(sc.lines[y])[:c.list], word) {
				t.Errorf("%dx%d: %s is not under the list:\n%s", c.w, c.h, word, screen(m))
			}
		}
	}
	m := fullView(flintAt(t, 86, 30, "2"))
	if list, _ := m.panes(86); list != (86-1)/2 {
		t.Errorf("Ships at 86: list %d, want the middle", list)
	}
}
