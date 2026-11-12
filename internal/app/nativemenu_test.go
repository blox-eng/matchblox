package app

import (
	"math"
	"os"
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
