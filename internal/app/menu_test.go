package app

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/menu"
	"github.com/blox-eng/matchblox/internal/proto"
)

const promptScreen = `● Bash(go test ./...)

╭───────────────────────────────────────────────────╮
│ Bash command                                      │
│   go test ./...                                   │
│ Do you want to proceed?                           │
│ ❯ 1. Yes                                          │
│   2. Yes, and don't ask again for go test commands│
│   3. No, and tell Claude what to do differently   │
╰───────────────────────────────────────────────────╯
`

// promptConsole is kindle-web at a permission prompt, chosen, its screen
// on the chat for peekDead.
func promptConsole(t *testing.T, screen string) (Model, *time.Time) {
	t.Helper()
	s := boardMatch("kindle-web", time.Hour)
	s.Agent, s.Permits, s.LastLine = "claude", true, "Do you want to proceed?"
	m, clock := sideConsole(t, boardState(s))
	return showScreen(t, m, clock, screen), clock
}

// showScreen puts screen on the chat of the chosen session, and lets the
// mark guard's dead time pass.
func showScreen(t *testing.T, m Model, clock *time.Time, screen string) Model {
	t.Helper()
	s, _ := m.sideChosen()
	m.watchHost, m.watching = s.Host, s.Pane
	m.screen = proto.Screen{Pane: s.Pane, At: m.now(), Text: screen}
	m = upd(t, m, noticeMsg{})
	*clock = clock.Add(peekDead)
	return m
}

// TestTwoPressesAnswerAPermissionPrompt: 2 marks, 2 again sends a choice
// with the option's whole text and the screen's signature.
func TestTwoPressesAnswerAPermissionPrompt(t *testing.T) {
	m, _ := promptConsole(t, promptScreen)
	s, _ := m.sideChosen()
	m = pressOn(m, "2")
	if len(sent(m)) != 0 || !strings.Contains(chatText(m, 100, 20), "❯ 2. Yes, and don't ask again") {
		t.Fatalf("the first 2 sent %+v, chat:\n%s", sent(m), chatText(m, 100, 20))
	}
	m = pressOn(m, "2")
	want := "2\x00Yes, and don't ask again for go test commands\x00" + menu.Sign(promptScreen)
	if got := sent(m); len(got) != 1 || got[0].RecID != proto.ChoosePrefix+s.Pane || got[0].Text != want || got[0].Confirm != "y" || got[0].PID != s.PID {
		t.Fatalf("sent %+v", got)
	}
	if m.lines[paneOf(s)] != "" {
		t.Fatal("a digit that answered typed into the line")
	}
}

// TestARefusedPickSaysWhy: the service's refusal shows, and the digits
// take an answer again.
func TestARefusedPickSaysWhy(t *testing.T) {
	m, _ := promptConsole(t, promptScreen)
	m = pressOn(m, "1", "1")
	s, _ := m.sideChosen()
	why := "not sent: kindle-web's menu changed — look again"
	m = upd(t, m, resultMsg{ActID: "a" + strconv.Itoa(m.acts), Err: why})
	if _, n := m.chatMarks(s); n != 0 || m.noticeText != why {
		t.Fatalf("sent %d, notice %q", n, m.noticeText)
	}
}

// TestNoMenuSaysAnswerInItsPane: the session needs you and its screen has
// no menu: the line says where to answer, and a digit is text.
func TestNoMenuSaysAnswerInItsPane(t *testing.T) {
	m, _ := promptConsole(t, "● Working on it.\n\n❯ \n")
	if got := chatLineOf(m, 120); !strings.Contains(got, "answer in its pane · ctrl+o") {
		t.Fatalf("line %q", got)
	}
	m = pressOn(m, "1")
	if s, _ := m.sideChosen(); m.lines[paneOf(s)] != "1" || len(sent(m)) != 0 {
		t.Fatalf("line %q, sent %+v", m.lines[paneOf(s)], sent(m))
	}
}

// TestTenOptionsPickOneToNine: digits reach options 1 to 9.
func TestTenOptionsPickOneToNine(t *testing.T) {
	var b strings.Builder
	b.WriteString("Which file?\n")
	for n := 1; n <= 10; n++ {
		b.WriteString("  " + strconv.Itoa(n) + ". file" + strconv.Itoa(n) + ".go\n")
	}
	m, _ := promptConsole(t, b.String())
	if m = pressOn(m, "0"); m.noticeText != "1-9 only" || len(sent(m)) != 0 {
		t.Fatalf("0: notice %q, sent %+v", m.noticeText, sent(m))
	}
	m = pressOn(m, "9", "9")
	if got := sent(m); len(got) != 1 || !strings.HasPrefix(got[0].Text, "9\x00file9.go\x00") {
		t.Fatalf("sent %+v", got)
	}
}

// TestAnotherPromptWithTheSameOptionsTakesItsOwnPicks: the pick went, and
// the next prompt asks about another command with the same options: the
// "sent" mark of the first does not hold the digits.
func TestAnotherPromptWithTheSameOptionsTakesItsOwnPicks(t *testing.T) {
	m, clock := promptConsole(t, promptScreen)
	m = pressOn(m, "1", "1")
	m = showScreen(t, m, clock, "● Running…\n\n❯ \n") // the agent works
	m = showScreen(t, m, clock, strings.Replace(promptScreen, "go test ./...  ", "rm -rf build/  ", 1))
	m = pressOn(m, "1", "1")
	if got := sent(m); len(got) != 2 || got[1].Text == got[0].Text {
		t.Fatalf("sent %+v", got)
	}
}

// TestACheckboxMenuIsAnsweredInItsPane: a menu that picks many has a
// Submit with no number: the digits leave it to the pane.
func TestACheckboxMenuIsAnsweredInItsPane(t *testing.T) {
	m, _ := promptConsole(t, "Which toppings?\n❯ 1. [ ] Cheese\n  2. [ ] Olives\n     Submit\n")
	if got := chatLineOf(m, 120); !strings.Contains(got, "answer in its pane") {
		t.Fatalf("line %q", got)
	}
}
