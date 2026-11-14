package app

import (
	"slices"
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
	cur := next
	for _, k := range keys {
		var cmd tea.Cmd
		cur, cmd = keyOrOpen(cur, k)
		drain(cmd)
	}
	return cur.(Model)
}

// viewDrawn tells if the frame shows flint-auth's chat, and if so whether
// the agent's menu is whole in it: its question, every option and its hint.
func viewDrawn(m Model) (drawn, whole bool) {
	sc := m.draw(m.width)
	out := ansi.Strip(strings.Join(sc.lines, "\n"))
	drawn = sc.placeFull
	whole = strings.Contains(out, "Should a refresh also rotate") && strings.Contains(out, "❯ 1. Rotate it") &&
		strings.Contains(out, "3. Type something.") && strings.Contains(out, "Enter to select")
	return drawn, whole
}

// TestEveryGoldenShowsTheAgentsWholeMenu: on every golden size, a chat at a
// menu shows the tallest common menu from its question down, line by line,
// as the agent draws it at the chat's size, or is not drawn: in the list's
// view, with the keys, in list only, and with the keys from list only.
func TestEveryGoldenShowsTheAgentsWholeMenu(t *testing.T) {
	text := tallestMenu(t)
	sizes := append(slices.Clone(surveyGrids), [2]int{200, 50}, [2]int{132, 32}, [2]int{86, 30}, [2]int{107, 60}, [2]int{44, 31}, [2]int{44, 16})
	for _, size := range sizes {
		w, h := size[0], size[1]
		for _, keys := range [][]string{nil, {"enter"}, {"v"}, {"v", "enter"}} {
			m := flintAt(t, w, h, keys...)
			if drawn, _ := viewDrawn(m); !drawn {
				continue
			}
			cols, rows := viewRoom(m)
			if cols == 0 {
				continue // in the list: no chat sizes the agent
			}
			at := fitScreen(text, cols, rows)
			next, _ := m.Update(screenMsg{Pane: m.watching, At: demoAt, Text: strings.Join(at, "\n")})
			out := screen(next)
			for _, want := range menuFrom(t, text, cols) {
				if !strings.Contains(out, strings.TrimSpace(want)) {
					t.Errorf("%dx%d after %v: the chat of %dx%d lacks %q:\n%s", w, h, keys, cols, rows, want, out)
				}
			}
		}
	}
}

// TestUsageUnderANarrowListIsNeverCutMidWord: under a list of 38 to 41
// columns a reset time, or a fix, that does not fit goes whole: no usage
// line is cut.
func TestUsageUnderANarrowListIsNeverCutMidWord(t *testing.T) {
	for _, size := range [][2]int{{96, 20}, {104, 38}, {96, 30}} {
		m := fullView(flintAt(t, size[0], size[1]))
		sc := m.draw(size[0])
		left := plainLines(sc.lines)
		in := false
		for _, l := range left {
			switch {
			case strings.HasPrefix(l, " USAGE"):
				in = true
			case strings.HasPrefix(l, "───"):
				in = false
			case in && strings.Contains(l, "…"):
				t.Errorf("%v: a usage line is cut: %q", size, l)
			}
		}
	}
}
