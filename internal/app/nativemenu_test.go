package app

import (
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
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

// TestAQuestionDrawsTheAgentsOwnMenu: at each survey size, a chat at a
// question draws the agent's own menu and no card; where it is too short the
// chat is not drawn and the key line says so.
func TestAQuestionDrawsTheAgentsOwnMenu(t *testing.T) {
	for _, size := range slices.Concat(surveyGrids, [][2]int{{44, 31}, {132, 32}, {200, 50}}) {
		w, h := size[0], size[1]
		m := flintAt(t, w, h, "enter")
		out := screen(m)
		if strings.Contains(out, "flint-auth asks") || strings.Contains(out, "Type an answer") || strings.Contains(out, "╰─ ⏎") {
			t.Errorf("%dx%d: a card is drawn:\n%s", w, h, out)
		}
		if h-3 < menuLeast {
			if m.chatKeys || !strings.Contains(m.flash, "too short to show flint-auth") {
				t.Errorf("%dx%d: too short, yet keys %v flash %q", w, h, m.chatKeys, m.flash)
			}
			continue
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

// TestAQuestionResizesNoAgent: the window is the chat's size, and no card
// takes rows from it: a question that starts or ends sends no new watch.
func TestAQuestionResizesNoAgent(t *testing.T) {
	m, f := sizedChat(t)
	cols, rows := m.chatSize()
	before := len(f.acts())
	st := fixtureState()
	st.Sessions = []sample.Session{{PID: 1, Pane: "%1", Name: "a", Ask: &said.Ask{ID: "toolu_1", Questions: []said.Question{threeWays}}}, {PID: 2, Pane: "%2", Name: "b"}}
	st.Queue = []queue.Item{{Pane: "%1", Name: "a", State: queue.StateQuestion, Since: demoAt}}
	next, cmd := m.Update(stateMsg(st))
	drain(cmd)
	if c, r := next.(Model).chatSize(); c != cols || r != rows {
		t.Fatalf("a question moved the chat's size from %dx%d to %dx%d", cols, rows, c, r)
	}
	st.Queue, st.Sessions[0].Ask = nil, nil
	next, cmd = next.Update(stateMsg(st))
	drain(cmd)
	if c, r := next.(Model).chatSize(); c != cols || r != rows {
		t.Fatalf("an answer moved the chat's size from %dx%d to %dx%d", cols, rows, c, r)
	}
	if acts := f.acts()[before:]; len(acts) != 0 {
		t.Fatalf("a question sent %+v", acts)
	}
}

// TestEveryKeyOfAMenuReachesTheAgent: ↑ ↓ Space Enter, digits and typing go
// to the agent as keys, in order, and nothing goes through the pick: act.
func TestEveryKeyOfAMenuReachesTheAgent(t *testing.T) {
	m, f := askModel(t, 100, threeWays)
	presses := []struct{ key, text string }{
		{"down", "Down"}, {"enter", "Enter"}, {"up", "Up"}, {"space", "lit: "}, {"2", "lit:2"}, {"x", "lit:x"}, {"backspace", "BSpace"},
	}
	for i, p := range presses {
		m = pressKeys(t, m, p.key)
		got := keyActs(f)
		if len(got) != i+1 || got[i].Text != p.text || got[i].RecID != proto.KeysPrefix+"%1" {
			t.Fatalf("%s: keys %+v", p.key, got)
		}
		if got[i].PID != 1 || got[i].Seen.IsZero() {
			t.Fatalf("%s: the key names no agent or no screen seen: %+v", p.key, got[i])
		}
		m = answered(t, m, "")
	}
	if len(pickActs(f)) != 0 {
		t.Fatalf("the chat picked: %+v", pickActs(f))
	}
}

// TestAKeyAtAMenuBeforeItsScreenIsSeen: with no screen of the pane the key
// carries no time seen, so the service refuses it (its
// TestAKeyAtAnAskNeedsAScreen), and the refusal shows.
func TestAKeyAtAMenuBeforeItsScreenIsSeen(t *testing.T) {
	m, f := loadedWith(t, 100, cardState("toolu_1", threeWays))
	m = pastQuiet(pressKeys(t, onPane(t, m, "%1"), "enter"))
	m = pressKeys(t, m, "1")
	got := keyActs(f)
	if len(got) != 1 || got[0].Text != "lit:1" || !got[0].Seen.IsZero() {
		t.Fatalf("keys %+v", got)
	}
	m = answered(t, m, "not sent: the console has not shown a's screen yet — look again")
	if !strings.Contains(m.flash, "look again") {
		t.Fatalf("flash %q", m.flash)
	}
}

// TestAPasteWithALineBreakAtAMenuIsRefused: its Enter would pick an option
// the person never chose. A line with none goes, and so does any paste away
// from a menu.
func TestAPasteWithALineBreakAtAMenuIsRefused(t *testing.T) {
	for _, content := range []string{"first\nsecond", "one line\n", "with\rreturn"} {
		m, f := askModel(t, 100, threeWays)
		next, cmd := m.Update(tea.PasteMsg{Content: content})
		drain(cmd)
		if got := next.(Model); len(keyActs(f)) != 0 || !strings.Contains(got.flash, "line break") {
			t.Fatalf("%q: keys %+v flash %q", content, keyActs(f), got.flash)
		}
	}
	m, f := askModel(t, 100, threeWays)
	next, cmd := m.Update(tea.PasteMsg{Content: "keep the old key"})
	drain(cmd)
	if got := keyActs(f); len(got) != 1 || got[0].Text != "lit:keep the old key" || next.(Model).flash != "" {
		t.Fatalf("one line: keys %+v flash %q", got, next.(Model).flash)
	}
}

// TestAQuietStartsWithANewMenuAndHoldsEveryKeyThatAnswers: a new wait holds Enter, digits, arrows, Esc and text; ^C and
// Backspace answer nothing and go.
func TestAQuietStartsWithANewMenuAndHoldsEveryKeyThatAnswers(t *testing.T) {
	working := cardState("toolu_1", threeWays)
	working.Sessions[0].Ask = nil
	asks := cardState("toolu_1", threeWays)
	for _, k := range []string{"enter", "1", "down", "esc", "x", "space"} {
		m, f := loadedWith(t, 100, working)
		m = pastQuiet(pressKeys(t, onPane(t, m, "%1"), "enter"))
		m, now := clocked(m)
		next, _ := m.Update(stateMsg(asks))
		m = pressKeys(t, next.(Model), k)
		if len(keyActs(f)) != 0 || m.flash != askedAgain("a") {
			t.Fatalf("%s in the quiet: keys %+v flash %q", k, keyActs(f), m.flash)
		}
		*now = now.Add(menuQuiet)
		pressKeys(t, m, k)
		if len(keyActs(f)) != 1 {
			t.Fatalf("%s after the quiet: keys %+v", k, keyActs(f))
		}
	}
	m, f := loadedWith(t, 100, working)
	m = pastQuiet(pressKeys(t, onPane(t, m, "%1"), "enter"))
	next, _ := m.Update(stateMsg(asks))
	pressKeys(t, next.(Model), "backspace")
	if len(keyActs(f)) != 1 {
		t.Fatalf("backspace in the quiet: keys %+v", keyActs(f))
	}
}

// TestTappingTheChatSendsNothing: no line of the agent's screen is a button.
// A tap on it gives the chat the keys beside a list and does nothing in a
// phone's chat; ‹ Sessions and a key of the key line still work.
func TestTappingTheChatSendsNothing(t *testing.T) {
	m, f := askModel(t, 100, threeWays)
	sc := m.draw(100)
	next, cmd := m.tap(tea.Mouse{Button: tea.MouseLeft, X: sc.listW + 4, Y: sc.bodyTop + len(sc.rows) - 2})
	drain(cmd)
	if len(keyActs(f)) != 0 || len(pickActs(f)) != 0 || !next.(Model).chatKeys {
		t.Fatalf("a tap beside the list: acts %+v keys %v", f.acts(), next.(Model).chatKeys)
	}

	m, f = askModel(t, 44, threeWays)
	if !m.draw(44).chatFull {
		t.Fatal("no phone chat")
	}
	sc = m.draw(44)
	next, cmd = m.tap(tea.Mouse{Button: tea.MouseLeft, X: 5, Y: sc.keysAt - 3})
	drain(cmd)
	if len(keyActs(f)) != 0 || len(pickActs(f)) != 0 || !next.(Model).chatKeys {
		t.Fatalf("a tap on a phone's chat: acts %+v keys %v", f.acts(), next.(Model).chatKeys)
	}
	next, cmd = next.(Model).tap(tea.Mouse{Button: tea.MouseLeft, X: 3, Y: 0})
	drain(cmd)
	if next.(Model).chatKeys {
		t.Fatal("‹ Sessions did not give the keys back")
	}
}

// TestAMenuReadsReWrappedWhereTheWindowIsNotSized: a split window keeps its
// wide lines, and fitScreen wraps them: every option and the key hint show,
// and the rule is cut.
func TestAMenuReadsReWrappedWhereTheWindowIsNotSized(t *testing.T) {
	text := tallestMenu(t)
	for _, size := range [][2]int{{44, 31}, {51, 31}, {96, 57}, {145, 40}} {
		m := flintAt(t, size[0], size[1], "enter")
		next, _ := m.Update(screenMsg{Pane: m.watching, At: demoAt, Text: text, Cols: 120, Rows: 50})
		out := screen(next)
		cols, _ := next.(Model).chatSize()
		for _, want := range menuFrom(t, text, cols) {
			if !strings.Contains(out, strings.TrimSpace(want)) {
				t.Errorf("%v: %q is gone:\n%s", size, want, out)
			}
		}
	}
}
