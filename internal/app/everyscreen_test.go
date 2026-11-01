package app

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
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

// TestEveryGoldenShowsTheWholeCard: on every golden size, the chat shows
// its whole question card or is not drawn: in the list's view, with the
// keys, and in the full view.
func TestEveryGoldenShowsTheWholeCard(t *testing.T) {
	sizes := append(slices.Clone(surveyGrids), [2]int{200, 50}, [2]int{132, 32}, [2]int{86, 30}, [2]int{107, 60}, [2]int{92, 40})
	for _, size := range sizes {
		w, h := size[0], size[1]
		for _, keys := range [][]string{nil, {"enter"}, {"v"}, {"v", "enter"}} {
			m := flintAt(t, w, h, keys...)
			if drawn, whole := chatDrawn(m); drawn && !whole {
				t.Errorf("%dx%d after %v: the chat shows without its whole question:\n%s", w, h, keys, screen(m))
			}
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
	night := sized(t, 132, 20, stokerState(runningNight(fixtureState().At)))
	if out := screen(night); !strings.Contains(out, "STOKER") || strings.Contains(out, "MACHINE") {
		t.Fatalf("132x20 at night: want the stoker in charge over the cut list, the machine folded:\n%s", out)
	}
}

// TestTheChatKeyLineKeepsTheWayBack: the chat's key line drops a hint
// whole when it does not fit, by worth: the agent's name first, then the
// way into tmux; ctrl+a m back stays.
func TestTheChatKeyLineKeepsTheWayBack(t *testing.T) {
	for w := 40; w <= 70; w++ {
		m := flintAt(t, w, 31, "enter")
		line, spans := m.chatKeyLine(w)
		got := strings.TrimSpace(ansi.Strip(line))
		if strings.Contains(got, "…") || !strings.Contains(got, "ctrl+a m back") {
			t.Fatalf("%d columns: the key line is %q", w, got)
		}
		parts := strings.Split(got, " · ")
		for _, p := range parts {
			if !slices.Contains([]string{"keys go to flint-auth", "ctrl+a m back", "ctrl+o into tmux"}, p) {
				t.Fatalf("%d columns: %q is cut in %q", w, p, got)
			}
		}
		if strings.Contains(got, "keys go to") && !strings.Contains(got, "ctrl+o") {
			t.Fatalf("%d columns: the name stayed over the way into tmux: %q", w, got)
		}
		if len(spans) != strings.Count(got, "ctrl+") {
			t.Fatalf("%d columns: %d tap targets for %q", w, len(spans), got)
		}
	}
}

// TestTheQuestionShowsOnceUpTheHistory: up the agent's history, with the
// card up, the agent's own menu is cut from the lines above the card, as
// on the live screen.
func TestTheQuestionShowsOnceUpTheHistory(t *testing.T) {
	for _, size := range [][2]int{{44, 31}, {200, 50}} {
		w, h := size[0], size[1]
		var next tea.Model = flintAt(t, w, h, "enter")
		for range 2 {
			next, _ = next.Update(tea.MouseWheelMsg{X: w - 2, Y: h / 2, Button: tea.MouseWheelUp})
		}
		next, _ = next.Update(screenMsg{Pane: next.(Model).watching, At: demoAt, Text: historyScreen, Up: 6})
		out := screen(next)
		if !strings.Contains(out, "6 lines up") {
			t.Fatalf("%dx%d: not up the history:\n%s", w, h, out)
		}
		if n := strings.Count(out, "Should a refresh also rotate the token?"); n != 1 || strings.Contains(out, "☐ Rotate") {
			t.Fatalf("%dx%d: the question shows %d times up the history:\n%s", w, h, n, out)
		}
	}
}

// TestListOnlyFillsItsFreeRowsWithUsage: in list only the list comes first
// and is never cut for the stats; the rows it leaves free take usage, then
// the machine, under a hairline, each whole at its fewest lines.
func TestListOnlyFillsItsFreeRowsWithUsage(t *testing.T) {
	at := func(st proto.State) Model {
		t.Helper()
		m := New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
		next, _ := m.Update(tea.WindowSizeMsg{Width: 107, Height: 60})
		next, _ = next.Update(stateMsg(st))
		nm := next.(Model)
		nm.splashDone = true
		if !nm.listOnly() {
			t.Fatal("107x60 does not start list only")
		}
		return nm
	}
	short := at(screensState(t))
	if out := screen(short); !strings.Contains(out, "USAGE") || strings.Contains(out, " more") {
		t.Fatalf("a short list leaves usage behind its blank rows:\n%s", out)
	}
	var many []func(sample.Session) sample.Session
	for i := range 60 {
		many = append(many, sess(100+i, fmt.Sprintf("agent-%02d", i), fmt.Sprintf("%%%d", 100+i), nil))
	}
	st := hostState(many...)
	st.At = demoAt // the age on the key line does not move with the wall clock
	full := at(st)
	if out := screen(full); strings.Contains(out, "USAGE") || strings.Contains(out, "MACHINE") {
		t.Fatalf("a list that fills the screen gave way to the stats:\n%s", out)
	}
	golden(t, full, "listonlyfull", 107, 60)
}

// TestTheCardsTopShowsWhereverTheChatIsBeside: the split counts the chat's
// title and its strip as they are drawn (the strip takes two lines on a
// chat of 44 to 59 columns), so the card's top line, with its "1 of 2",
// is never cut.
func TestTheCardsTopShowsWhereverTheChatIsBeside(t *testing.T) {
	for _, w := range []int{83, 84, 90, 95, 96, 100, 104, 110} {
		for h := 8; h <= 24; h++ { // above 24 rows every card fits
			m := fullView(flintAt(t, w, h))
			if _, right := m.panes(w); right == 0 {
				continue
			}
			if out := screen(m); !strings.Contains(out, "╭─ flint-auth asks · 1 of 2") {
				t.Fatalf("%dx%d: the card's top is cut:\n%s", w, h, out)
			}
		}
	}
}

// TestTheSplitHoldsAsTheCursorMoves: Sessions splits by the tallest card of
// the sessions that ask, not by the selected one's: moving the cursor never
// flips the screen between one column and the split.
func TestTheSplitHoldsAsTheCursorMoves(t *testing.T) {
	for _, size := range [][2]int{{96, 13}, {96, 14}, {100, 16}, {132, 15}} {
		m := fullView(flintAt(t, size[0], size[1]))
		_, want := m.panes(size[0])
		for range 8 {
			m = pressKeys(t, m, "up")
			if _, right := m.panes(size[0]); right != want {
				s, _ := m.selectedSession()
				t.Fatalf("%v: on %s the chat is %d wide, on flint-auth %d", size, s.Name, right, want)
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
		left, _ := sides(sc, size[0])
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

// TestUsageOfThreeHostsIsNeverCutSideways: under a narrow list each
// window's hosts that do not fit after it go whole onto the next lines:
// every host's percent shows, and no usage line ends cut.
func TestUsageOfThreeHostsIsNeverCutSideways(t *testing.T) {
	for _, c := range []struct {
		w, h int
		gpu  string
	}{{132, 32, "fine"}, {86, 30, "blocked"}, {86, 40, "fine"}} {
		m := threeHosts(t, screensState(t), c.w, c.h, c.gpu)
		sc := m.draw(c.w)
		left, _ := sides(sc, c.w)
		in, usage := false, ""
		for _, l := range left {
			switch {
			case strings.HasPrefix(l, " USAGE"):
				in = true
			case strings.HasPrefix(l, "───"):
				in = false
			case in:
				usage += l + "\n"
				if strings.Contains(l, "…") {
					t.Errorf("%dx%d: a usage line is cut: %q", c.w, c.h, l)
				}
			}
		}
		if !strings.Contains(usage, "this machine 84%") {
			t.Errorf("%dx%d: this machine's percent is gone:\n%s", c.w, c.h, usage)
		}
	}
}
