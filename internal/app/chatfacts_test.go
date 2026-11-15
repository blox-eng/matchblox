package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
)

// noWindow is a session whose tokens have no window known.
func noWindow(s *sample.Session) {
	s.Agent, s.Tokens = "claude", 158_000
	s.Context, s.Window, s.ContextPct = "unmeasured", 0, 0
}

// factsChat is the console at 160 columns with the first session of the
// queue's state, edited by edit, chosen.
func factsChat(t *testing.T, edit func(*sample.Session)) (Model, sample.Session) {
	t.Helper()
	st := queueState()
	st.Queue = nil
	if edit != nil {
		edit(&st.Sessions[0])
	}
	m, _ := sideConsole(t, st)
	m = pickSide(t, m, st.Sessions[0].Name)
	s, ok := m.sideChosen()
	if !ok {
		t.Fatal("no chosen session")
	}
	return m, s
}

// TestChatFactsLineNamesTheWindowFix (owner, 2026-11-12): tokens with no
// window known name the fix, in its own words, on the line under the
// chat's head.
func TestChatFactsLineNamesTheWindowFix(t *testing.T) {
	m, s := factsChat(t, noWindow)
	lines := plainLines(m.chatPanel(120, 12))
	want := "158k tokens · no window known: run matchblox setup, or add the model to [sessions.windows]"
	if got := strings.TrimSpace(lines[1]); got != want {
		t.Fatalf("the line under the head is %q, want %q\n%s", got, want, strings.Join(lines, "\n"))
	}
	if facts, ok := m.chatFacts(s, 120); !ok || strings.TrimSpace(ansi.Strip(facts)) != want {
		t.Errorf("chatFacts = %q, %v", ansi.Strip(facts), ok)
	}
}

// TestChatFactsLineIsAbsentWhenNothingToSay: a session with no fact to
// name has no facts line: the conversation starts under the head.
func TestChatFactsLineIsAbsentWhenNothingToSay(t *testing.T) {
	m, s := factsChat(t, func(s *sample.Session) { s.Agents, s.Shells, s.AccountDir = 0, nil, "" })
	if facts, ok := m.chatFacts(s, 120); ok {
		t.Fatalf("a session with nothing to say has the facts %q", ansi.Strip(facts))
	}
	with, ws := factsChat(t, noWindow)
	facts, _ := with.chatFacts(ws, 120)
	for _, l := range plainLines(m.chatPanel(120, 12)) {
		if strings.Contains(l, "tokens ·") || strings.Contains(l, strings.TrimSpace(ansi.Strip(facts))) {
			t.Errorf("a facts line draws: %q", l)
		}
	}
}

// TestChatFactsLineFitsItsWidth: every fact at once is one line of the
// chat's width, cut and never wrapped; the window fix comes first.
func TestChatFactsLineFitsItsWidth(t *testing.T) {
	m, s := factsChat(t, func(s *sample.Session) {
		noWindow(s)
		s.Agents = 3
		s.Shells = []sample.Shell{{PID: 4242, Command: "go test ./..."}}
		s.AccountDir = "/home/u/.matchblox/accounts/claude/work"
	})
	for _, w := range []int{20, 44, 80, 120, 300} {
		facts, ok := m.chatFacts(s, w)
		if !ok {
			t.Fatalf("width %d: no facts", w)
		}
		if strings.Contains(facts, "\n") || ansi.StringWidth(facts) != w {
			t.Errorf("width %d: the facts are %d wide, or wrap: %q", w, ansi.StringWidth(facts), ansi.Strip(facts))
		}
		if plain := strings.TrimSpace(ansi.Strip(facts)); !strings.HasPrefix(plain, "158k") {
			t.Errorf("width %d: the window fix does not come first: %q", w, plain)
		}
	}
	wide, _ := m.chatFacts(s, 300)
	for _, want := range []string{"3 subagents running", "1 shell", "work"} {
		if !strings.Contains(ansi.Strip(wide), want) {
			t.Errorf("the wide facts lack %q: %q", want, ansi.Strip(wide))
		}
	}
}
