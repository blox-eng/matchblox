package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// noWindow is a session whose tokens have no window known.
func noWindow(s *sample.Session) {
	s.Agent, s.Tokens = "claude", 158_000
	s.Context, s.Window, s.ContextPct = "unmeasured", 0, 0
}

// windowFixWords is the fix for noWindow, whole.
const windowFixWords = "158k tokens · no window known: run matchblox setup, or add the model to [sessions.windows]"

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

// factsText is the facts of s at w, plain, each line trimmed and joined by
// a space.
func factsText(m Model, s sample.Session, w int) string {
	var said []string
	for _, l := range m.chatFacts(s, w) {
		said = append(said, strings.TrimSpace(ansi.Strip(l)))
	}
	return strings.Join(said, " ")
}

// TestChatFactsLineNamesTheWindowFix (owner, 2026-11-12): tokens with no
// window known name the fix, in its own words, on the line under the
// chat's head.
func TestChatFactsLineNamesTheWindowFix(t *testing.T) {
	m, s := factsChat(t, noWindow)
	lines := plainLines(m.chatPanel(120, 12))
	if got := strings.TrimSpace(lines[1]); got != windowFixWords {
		t.Fatalf("the line under the head is %q, want %q\n%s", got, windowFixWords, strings.Join(lines, "\n"))
	}
	if got := factsText(m, s, 120); got != windowFixWords {
		t.Errorf("chatFacts = %q", got)
	}
}

// TestChatFactsLineIsAbsentWhenNothingToSay: a session with no fact to
// name has no facts line: the conversation starts under the head.
func TestChatFactsLineIsAbsentWhenNothingToSay(t *testing.T) {
	m, s := factsChat(t, func(s *sample.Session) { s.Agents, s.Shells, s.AccountDir = 0, nil, "" })
	if facts := m.chatFacts(s, 120); len(facts) != 0 {
		t.Fatalf("a session with nothing to say has the facts %q", plainLines(facts))
	}
	for _, l := range plainLines(m.chatPanel(120, 12)) {
		if strings.Contains(l, "tokens ·") || strings.Contains(l, "running") {
			t.Errorf("a facts line draws: %q", l)
		}
	}
}

// TestChatFactsLineFitsItsWidth: every line of the facts is the chat's
// width; where all fit, they are one line, the window fix first.
func TestChatFactsLineFitsItsWidth(t *testing.T) {
	m, s := factsChat(t, func(s *sample.Session) {
		noWindow(s)
		s.Agents = 3
		s.Shells = []sample.Shell{{PID: 4242, Command: "go test ./..."}}
		s.AccountDir = "/home/u/.matchblox/accounts/claude/work"
	})
	for _, w := range []int{20, 44, 80, 120, 300} {
		facts := m.chatFacts(s, w)
		if len(facts) == 0 {
			t.Fatalf("width %d: no facts", w)
		}
		for _, l := range facts {
			if strings.Contains(l, "\n") || ansi.StringWidth(l) != w {
				t.Errorf("width %d: a line is %d wide, or wraps: %q", w, ansi.StringWidth(l), ansi.Strip(l))
			}
		}
		if plain := strings.TrimSpace(ansi.Strip(facts[0])); !strings.HasPrefix(plain, "158k") {
			t.Errorf("width %d: the window fix does not come first: %q", w, plain)
		}
	}
	wide := m.chatFacts(s, 300)
	if len(wide) != 1 {
		t.Fatalf("at 300 the facts take %d lines, want 1", len(wide))
	}
	for _, want := range []string{windowFixWords, "3 subagents running", "1 shell", "work"} {
		if !strings.Contains(ansi.Strip(wide[0]), want) {
			t.Errorf("the wide facts lack %q: %q", want, ansi.Strip(wide[0]))
		}
	}
}

// TestWindowFixKeepsItsActionAt44: on a phone's chat the window fix wraps
// whole: matchblox setup and sessions.windows both show, and the other
// facts follow it.
func TestWindowFixKeepsItsActionAt44(t *testing.T) {
	m, s := factsChat(t, func(s *sample.Session) { noWindow(s); s.Agents = 2 })
	facts := m.chatFacts(s, 44)
	if len(facts) < 2 {
		t.Fatalf("the fix does not wrap at 44: %q", plainLines(facts))
	}
	got := factsText(m, s, 44)
	for _, want := range []string{"run matchblox setup", "add the model to [sessions.windows]", "2 subagents running"} {
		if !strings.Contains(got, want) {
			t.Errorf("at 44 the facts lack %q: %q", want, got)
		}
	}
	if strings.Contains(got, "…") {
		t.Errorf("at 44 the fix is cut: %q", got)
	}
	panel := strings.Join(strings.Fields(strings.Join(plainLines(m.chatPanel(44, 16)), " ")), " ")
	if !strings.Contains(panel, "or add the model to [sessions.windows]") {
		t.Errorf("the chat at 44 does not draw the fix whole:\n%s", panel)
	}
}

// TestChatFactsNameEachFact: each fact, alone, says its words; the PR
// hides when a ship on the head names it, and the subagents hide when the
// state word says them.
func TestChatFactsNameEachFact(t *testing.T) {
	m, s := factsChat(t, nil)
	m.chains = &state.ChainsView{Mates: []state.MateView{{Member: memberOf(s), Inbox: 3, Full: true}}}
	if got := factsText(m, s, 200); got != "✉ 3 in its inbox, the oldest dropped" {
		t.Errorf("the inbox: %q", got)
	}

	m, s = factsChat(t, func(s *sample.Session) { s.Cwd = "/w/api/src" })
	m.git = &proto.GitReport{Repos: []gitscan.Repo{{Path: "/w/api", Worktrees: []gitscan.Worktree{
		{Path: "/w", PR: &gitscan.PR{Number: 1}},
		{Path: "/w/api", PR: &gitscan.PR{Number: 4551}},
	}}}}
	if got := factsText(m, s, 200); got != "#4551" {
		t.Errorf("the PR of the longest worktree: %q", got)
	}
	m.git.Repos[0].Worktrees[1].PR.Draft = true
	if got := factsText(m, s, 200); got != "#4551 draft" {
		t.Errorf("a draft PR: %q", got)
	}
	sh := proto.Ship{Number: 4551, Host: s.Host}
	sh.Session.Pane = s.Pane
	m.allShips = []proto.Ship{sh}
	if got := factsText(m, s, 200); got != "" {
		t.Errorf("a PR the head names shows again: %q", got)
	}

	m, s = factsChat(t, nil)
	top := &sample.ProcRef{PID: s.PID, Start: s.Start}
	m.snap.Children = []sample.Session{
		{PID: 901, Start: 1, Host: s.Host, Name: "codex exec", Parent: top},
		{PID: 903, Start: 3, Host: s.Host, Name: "a1", Parent: &sample.ProcRef{PID: 901, Start: 1}},
	}
	if got := factsText(m, s, 200); got != "2 child runs" {
		t.Errorf("the child runs: %q", got)
	}

	m, s = factsChat(t, func(s *sample.Session) { s.Agents = 2 })
	if got := factsText(m, s, 200); got != "2 subagents running" {
		t.Errorf("the subagents: %q", got)
	}
	s.Status = sample.StatusAgents
	if got := factsText(m, s, 200); got != "" {
		t.Errorf("subagents the state word says show again: %q", got)
	}

	m, s = factsChat(t, func(s *sample.Session) {
		s.AccountDir, s.Provider = "/home/u/.matchblox/accounts/claude/work", "anthropic"
	})
	if got := factsText(m, s, 120); got != "✻ work" {
		t.Errorf("the account: %q", got)
	}
}
