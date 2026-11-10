package app

import (
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
)

// tallestMenu is a real capture of the tallest common menu: the question
// tool's multi-select at four options with descriptions, cut after its last
// line.
func tallestMenu(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../testdata/menus/toppings.txt")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimRight(string(b), "\n")
}

// menuFrom is the menu in text from its question down, as the agent wraps
// it at w columns.
func menuFrom(t *testing.T, text string, w int) []string {
	t.Helper()
	at := strings.Index(text, "Which toppings do you want?\n")
	if at < 0 {
		t.Fatalf("no question in %q", text)
	}
	return fitScreen(text[at:], w, math.MaxInt)
}

// TestAQuestionDrawsTheAgentsOwnMenu: at each survey size, a live view at a
// question draws the agent's own menu and no card. One column has no view:
// the list's opened row shows the question, once, and the menu is answered
// in the agent's own pane (#351).
func TestAQuestionDrawsTheAgentsOwnMenu(t *testing.T) {
	for _, size := range slices.Concat(surveyGrids, [][2]int{{44, 31}, {132, 32}, {200, 50}}) {
		w, h := size[0], size[1]
		m := flintAt(t, w, h, "enter")
		out := screen(m)
		if m.viewFull() {
			t.Errorf("%dx%d: a view takes the screen outside the demo", w, h)
		}
		if _, right := m.panes(w); right == 0 {
			if n := strings.Count(out, "? Should a refresh"); n != 1 {
				t.Errorf("%dx%d: the opened row shows the question %d times:\n%s", w, h, n, out)
			}
			continue
		}
		if h-3 < menuLeast {
			continue
		}
		if strings.Contains(out, "flint-auth asks") || strings.Contains(out, "Type an answer") || strings.Contains(out, "╰─ ⏎") {
			t.Errorf("%dx%d: a card is drawn:\n%s", w, h, out)
		}
		for _, want := range []string{"Should a refresh also rotate the token?", "❯ 1. Rotate it", "2. Rotate, keep the old one a minute", "3. Type something.", "Enter to select"} {
			if !strings.Contains(out, want) {
				t.Errorf("%dx%d: the agent's menu lacks %q:\n%s", w, h, want, out)
			}
		}
		if n := strings.Count(out, "Should a refresh also rotate the token?"); n != 1 {
			t.Errorf("%dx%d: the question shows %d times:\n%s", w, h, n, out)
		}
	}
}

// TestAQuestionResizesNoAgent: no card takes rows from the view, and a
// question that starts or ends sends no new watch.
func TestAQuestionResizesNoAgent(t *testing.T) {
	m, f := viewModel(t)
	cols, rows := viewRoom(m)
	before := len(f.acts())
	st := fixtureState()
	st.Sessions = []sample.Session{{PID: 1, Pane: "%1", Name: "a", Ask: &said.Ask{ID: "toolu_1", Questions: []said.Question{threeWays}}}, {PID: 2, Pane: "%2", Name: "b"}}
	st.Queue = []queue.Item{{Pane: "%1", Name: "a", State: queue.StateQuestion, Since: demoAt}}
	next, cmd := m.Update(stateMsg(st))
	drain(cmd)
	if c, r := viewRoom(next.(Model)); c != cols || r != rows {
		t.Fatalf("a question moved the chat's size from %dx%d to %dx%d", cols, rows, c, r)
	}
	st.Queue, st.Sessions[0].Ask = nil, nil
	next, cmd = next.Update(stateMsg(st))
	drain(cmd)
	if c, r := viewRoom(next.(Model)); c != cols || r != rows {
		t.Fatalf("an answer moved the chat's size from %dx%d to %dx%d", cols, rows, c, r)
	}
	if acts := f.acts()[before:]; len(acts) != 0 {
		t.Fatalf("a question sent %+v", acts)
	}
}

// TestAMenuReadsReWrappedWhereTheWindowIsNotSized: a split window keeps its
// wide lines, and fitScreen wraps them: every option and the key hint show,
// and the rule is cut.
func TestAMenuReadsReWrappedWhereTheWindowIsNotSized(t *testing.T) {
	text := tallestMenu(t)
	for _, size := range [][2]int{{44, 31}, {51, 31}, {96, 57}, {145, 40}} {
		m := flintAt(t, size[0], size[1], "enter")
		next, _ := m.Update(screenMsg{Pane: m.watching, At: demoAt, Text: text})
		out := screen(next)
		cols, _ := viewRoom(next.(Model))
		for _, want := range menuFrom(t, text, cols) {
			if !strings.Contains(out, strings.TrimSpace(want)) {
				t.Errorf("%v: %q is gone:\n%s", size, want, out)
			}
		}
	}
}
