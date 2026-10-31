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

// chatDrawn tells if the frame shows flint-auth's chat, and if so whether
// its question card is whole: its title, every option and its foot.
func chatDrawn(m Model) (drawn, whole bool) {
	sc := m.draw(m.width)
	out := ansi.Strip(strings.Join(sc.lines, "\n"))
	_, right := m.panes(m.width)
	drawn = sc.chatFull || right > 0 && strings.Contains(out, "flint-auth · ")
	whole = strings.Contains(out, "flint-auth asks") && strings.Contains(out, "Should a refresh also rotate") &&
		strings.Contains(out, "Type an answer") && strings.Contains(out, "╰─ ⏎")
	return drawn, whole
}

// TestNoChatWithoutItsQuestion: a screen too short or too narrow for the
// chat's question draws Sessions in one column, and the chat, when it
// opens, is whole or not drawn.
func TestNoChatWithoutItsQuestion(t *testing.T) {
	for _, size := range [][2]int{{96, 8}, {106, 10}, {60, 21}, {96, 20}} {
		w, h := size[0], size[1]
		for _, keys := range [][]string{nil, {"enter"}} {
			m := flintAt(t, w, h, keys...)
			if drawn, whole := chatDrawn(m); drawn && !whole {
				t.Errorf("%dx%d after %v: the chat shows without its whole question:\n%s", w, h, keys, screen(m))
			}
		}
	}
	for _, size := range [][2]int{{96, 8}, {106, 10}, {60, 21}} {
		if _, right := flintAt(t, size[0], size[1]).panes(size[0]); right != 0 {
			t.Errorf("%dx%d: Sessions splits with no room for the question", size[0], size[1])
		}
	}
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
			if word == "MACHINE" && !strings.Contains(screen(m), word) {
				continue // folded for the list
			}
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

// TestTheListOutranksTheMachine: under the list, the machine folds before
// the list is cut; usage is never cut, and the list is cut only when
// usage alone does not leave it room.
func TestTheListOutranksTheMachine(t *testing.T) {
	m := threeHosts(t, screensState(t), 86, 40, "fine")
	out := strings.Join(frameOf(m), "\n")
	if !strings.Contains(out, "USAGE") || strings.Contains(out, "MACHINE") || strings.Contains(out, " more") {
		t.Fatalf("86x40: want the whole list and usage, the machine folded:\n%s", out)
	}
	short := threeHosts(t, screensState(t), 86, 30, "fine")
	out = strings.Join(frameOf(short), "\n")
	if !strings.Contains(out, "USAGE") || !strings.Contains(out, "OpenAI Plus") || strings.Contains(out, "MACHINE") || !strings.Contains(out, " more") {
		t.Fatalf("86x30: want usage whole over the cut list:\n%s", out)
	}
}
