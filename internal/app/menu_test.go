package app

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/menu"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
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

// unknownWording is a Claude Code approval box whose choices no rule
// knows: no numbers, and a keys line in other words.
func unknownWording(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../testdata/menus/unknown-wording.txt")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// unknownConsole is kindle-web, Claude Code 2.3.1, at a permission wait
// the service marked an unknown screen, chosen, screen on the chat for
// peekDead. edit changes the session first.
func unknownConsole(t *testing.T, screen string, edit ...func(*sample.Session)) (Model, sample.Session) {
	t.Helper()
	if menu.Read(screen) != nil {
		t.Fatalf("menu.Read reads %v", menu.Read(screen))
	}
	if q, ok := said.Menu(screen); ok {
		t.Fatalf("said.Menu reads %q", q)
	}
	s := boardMatch("kindle-web", time.Hour)
	s.Agent, s.Version, s.UnknownScreen = "claude", "2.3.1", true
	for _, e := range edit {
		e(&s)
	}
	st := boardState(s)
	st.Queue = []queue.Item{waitFor(s, queue.StatePermission, time.Minute)}
	m, clock := sideConsole(t, st)
	m = showScreen(t, m, clock, screen)
	s, _ = m.sideChosen()
	return m, s
}

// noDigits: the chat offers no digits for s, and 1 pressed twice sends
// nothing to it.
func noDigits(t *testing.T, m Model, s sample.Session) {
	t.Helper()
	if q, _, ok := m.digitMenu(s); ok {
		t.Fatalf("digits offered for %+v", q)
	}
	before := len(boardFake(m).acts())
	m = pressOn(m, "1", "1")
	if acts := boardFake(m).acts()[before:]; len(acts) != 0 {
		t.Fatalf("a digit sent %+v", acts)
	}
}

func TestNoDigitsOnAnUnknownWording(t *testing.T) {
	m, s := unknownConsole(t, unknownWording(t))
	noDigits(t, m, s)
}

// The agent's reply quotes an old menu with numbers above a live menu
// in words no rule knows: the quote is not the menu.
func TestAQuotedOldMenuGivesNoDigits(t *testing.T) {
	quote := "● The prompt used to read:\n\n  Do you want to proceed?\n  ❯ 1. Yes\n    2. No\n\n"
	screen := strings.Replace(unknownWording(t), "● Bash(rm -rf build/)", quote+"● Bash(rm -rf build/)", 1)
	if !strings.Contains(screen, "1. Yes") {
		t.Fatal("no quoted menu")
	}
	m, s := unknownConsole(t, screen)
	noDigits(t, m, s)
}

func TestTheRowSaysUnknownScreen(t *testing.T) {
	m, s := unknownConsole(t, unknownWording(t))
	if w, _ := m.rowWord(s); w != m.st.warn.Render("unknown screen") {
		t.Fatalf("state word %q, want %q", w, m.st.warn.Render("unknown screen"))
	}
}

const unknownNote = "unknown screen · Claude Code 2.3.1 — answer it in its pane: ctrl+o opens it"

func TestTheChatNamesTheVersionAndTheFix(t *testing.T) {
	m, _ := unknownConsole(t, unknownWording(t))
	if got := chatLineOf(m, 120); !strings.HasSuffix(strings.TrimRight(got, " "), unknownNote) {
		t.Fatalf("line %q", got)
	}
	m.keys.main.Jump = bind(ks("ctrl+g"), "ctrl+g", "go in")
	if got := chatLineOf(m, 120); !strings.Contains(got, "answer it in its pane: ctrl+g opens it") {
		t.Fatalf("a rebound jump: line %q", got)
	}
}

func TestTheChatSaysVersionNotRead(t *testing.T) {
	m, _ := unknownConsole(t, unknownWording(t), func(s *sample.Session) {
		s.Version, s.VersionWhy = "", "the install path names no version"
	})
	want := "unknown screen · Claude Code, version not read — answer it in its pane: ctrl+o opens it"
	if got := chatLineOf(m, 120); !strings.HasSuffix(strings.TrimRight(got, " "), want) {
		t.Fatalf("line %q", got)
	}
}

func TestTheNoteGoesWithTheFlag(t *testing.T) {
	m, s := unknownConsole(t, unknownWording(t), func(s *sample.Session) { s.UnknownScreen = false })
	if got := chatLineOf(m, 120); !strings.HasSuffix(strings.TrimRight(got, " "), m.inPane()) {
		t.Fatalf("line %q", got)
	}
	if got := strings.Join(plainLines(m.chatPanel(120, 30)), "\n"); strings.Contains(got, "unknown screen") {
		t.Fatalf("the chat says unknown screen:\n%s", got)
	}
	if w, _ := m.rowWord(s); w != m.st.accent.Render("asks") {
		t.Fatalf("state word %q", w)
	}
}

// At a phone's 44 columns the note wraps above the line, its key whole.
func TestTheNoteWrapsOnAPhone(t *testing.T) {
	m, _ := unknownConsole(t, unknownWording(t))
	lines := plainLines(m.chatPanel(44, 30))
	var note []string
	for _, l := range lines {
		if lipgloss.Width(l) > 44 {
			t.Fatalf("line %q is wider than 44", l)
		}
		if l := strings.TrimSpace(l); strings.HasPrefix(l, "unknown screen") || len(note) > 0 && !strings.HasPrefix(l, "›") {
			note = append(note, l)
		}
	}
	if len(note) < 2 || strings.Join(note, " ") != unknownNote {
		t.Fatalf("note lines %q", note)
	}
	if !slices.ContainsFunc(note, func(l string) bool { return strings.HasSuffix(l, "ctrl+o opens it") }) {
		t.Fatalf("ctrl+o opens it is cut: %q", note)
	}
	if last := strings.TrimSpace(lines[len(lines)-1]); !strings.HasPrefix(last, "›") {
		t.Fatalf("the last line %q is not the line", last)
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[0]), "kindle-web") {
		t.Fatalf("the note pushed the head out: %q", lines[0])
	}
}
