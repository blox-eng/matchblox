package app

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// cardState is the fixture machine where flint-auth asks "Which way do we
// fix the login?" with two options, drives #4551 in CI, and is in the
// chain login with flint-ui.
func cardState(edit func(*sample.Session)) (proto.State, sample.Session) {
	s := boardMatch("flint-auth", time.Hour)
	s.Agent, s.Context, s.ContextPct, s.Tokens = "claude", "known", 62, 124000
	s.LastLine = "I can fix the login in two ways."
	s.Ask = &said.Ask{ID: "toolu_login", Questions: []said.Question{{Question: "Which way do we fix the login?",
		Options: []said.Option{{Label: "Patch the token check"}, {Label: "Move to the new session store"}}}}}
	if edit != nil {
		edit(&s)
	}
	ui := boardMatch("flint-ui", 2*time.Hour)
	st := boardState(s, ui)
	st.Queue = []queue.Item{waitFor(s, queue.StateQuestion, 2*time.Minute)}
	st.Ships.Ships = []proto.Ship{shipFor(s, 4551, shipRunning, "CI · 6 of 9 jobs", 1, 4*time.Minute)}
	st.Chains = &state.ChainsView{Chains: []chains.Chain{{ID: "c1", Name: "login", Repo: "/home/user/src/hearth",
		Members: []chains.Member{memberOf(s), memberOf(ui)}}}}
	return st, s
}

func cardModel(t *testing.T, edit func(*sample.Session)) (Model, sample.Session) {
	t.Helper()
	st, s := cardState(edit)
	m := boardModel(t, st)
	for _, x := range m.all.sessions {
		if x.Name == s.Name {
			return m, x
		}
	}
	t.Fatal("flint-auth is not in the model")
	return m, s
}

func plainLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ansi.Strip(l)
	}
	return out
}

func pressesOn(spans []span) []string {
	var out []string
	for _, s := range spans {
		out = append(out, s.press)
	}
	return out
}

func washOn(m Model) string {
	r, g, b, _ := m.st.theme.AccentWash.RGBA()
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
}

func TestCardLines(t *testing.T) {
	m, s := cardModel(t, nil)
	st := m.st

	lines, taps := m.boardCard(s, 38, cardOpts{need: true})
	want := []string{
		" ╿ flint-auth                   ✻  2m ",
		"  asks Which way do we fix the login? ",
		"  #4551 ●━◉┄○┄○┄○  ctx 62%    ─╴login ",
		"  ⏎ answer                            ",
	}
	if got := plainLines(lines); !slices.Equal(got, want) {
		t.Fatalf("a card that waits, not chosen:\n%q\nwant\n%q", got, want)
	}
	if !strings.Contains(lines[1], st.text.Bold(true).Render("asks")) {
		t.Errorf("the state word of a card that is not chosen is not fg bold: %q", lines[1])
	}
	if !strings.Contains(lines[3], st.muted.Render("⏎")) || strings.Contains(lines[3], st.accent.Render("⏎")) {
		t.Errorf("the keys of a card that is not chosen are not muted: %q", lines[3])
	}
	if !strings.Contains(lines[2], st.faint.Render("─╴login")) {
		t.Errorf("the stub of a card that is not lit is not faint: %q", lines[2])
	}
	if len(taps) != len(lines) || !slices.Equal(pressesOn(taps[2]), []string{"g"}) || !slices.Equal(pressesOn(taps[3]), []string{"enter"}) {
		t.Errorf("taps: %v", taps)
	}

	// Chosen: the peek adds the last message and the options, on the wash.
	lines, taps = m.boardCard(s, 38, cardOpts{need: true, sel: true})
	want = []string{
		"▌╿ flint-auth                   ✻  2m ",
		"  asks Which way do we fix the login? ",
		"  #4551 ●━◉┄○┄○┄○  ctx 62%    ━╸login ",
		"  “I can fix the login in two ways.”  ",
		"  1. Patch the token check            ",
		"  2. Move to the new session store    ",
		"  1-2 mark an answer   ⏎ go in        ",
	}
	if got := plainLines(lines); !slices.Equal(got, want) {
		t.Fatalf("the chosen card:\n%q\nwant\n%q", got, want)
	}
	for _, c := range []struct {
		line int
		ink  string
		what string
	}{
		{0, st.accent.Render("▌"), "the selection mark"},
		{1, st.accent.Render("asks"), "the state word"},
		{2, st.text.Render("━╸login"), "the lit stub"},
		{6, st.accent.Render("1-2"), "the digits"},
		{6, st.accent.Render("⏎"), "the key"},
	} {
		if !strings.Contains(lines[c.line], c.ink) {
			t.Errorf("%s of the chosen card is not in its ink: %q", c.what, lines[c.line])
		}
	}
	for i, l := range lines {
		if !strings.HasPrefix(l, washOn(m)) || lipgloss.Width(l) != 38 {
			t.Errorf("line %d of the chosen card is not on the wash, every cell: %q", i, l)
		}
	}
	if !slices.Equal(pressesOn(taps[4]), []string{"1"}) || !slices.Equal(pressesOn(taps[5]), []string{"2"}) {
		t.Errorf("an option does not press its digit: %v", taps)
	}

	// Option 1 armed.
	lines, taps = m.boardCard(s, 38, cardOpts{need: true, sel: true, armed: 1})
	got := plainLines(lines)
	if got[4] != "❯ 1. Patch the token check            " || !strings.Contains(lines[4], st.accent.Render("❯")) {
		t.Errorf("the armed option: %q", lines[4])
	}
	if got[6] != "  1 again sends · esc clears          " {
		t.Errorf("the armed key line: %q", got[6])
	}
	if !slices.Equal(pressesOn(taps[6]), []string{"1", "esc"}) {
		t.Errorf("the armed key line's taps: %v", taps[6])
	}

	// Sent: no digits.
	lines, _ = m.boardCard(s, 38, cardOpts{need: true, sel: true, sent: 1})
	got = plainLines(lines)
	if !slices.Contains(got, "  sent 1 · waits for flint-auth       ") || !strings.Contains(strings.Join(lines, "\n"), st.muted.Render("sent 1 · waits for flint-auth")) {
		t.Errorf("the sent card:\n%q", got)
	}
	for _, l := range got {
		if strings.Contains(l, "1. ") || strings.Contains(l, "2. ") || strings.Contains(l, "1-2") {
			t.Errorf("the sent card still has digits: %q", l)
		}
	}

	// One line: line 1, and line 2 in NEEDS YOU.
	if lines, _ = m.boardCard(s, 38, cardOpts{need: true, oneLine: true}); len(lines) != 2 || ansi.Strip(lines[1]) != want[1] {
		t.Errorf("one line, in NEEDS YOU:\n%q", plainLines(lines))
	}
	if lines, _ = m.boardCard(s, 38, cardOpts{oneLine: true}); len(lines) != 1 {
		t.Errorf("one line, elsewhere:\n%q", plainLines(lines))
	}

	// A quiet row on a wide screen: one line; its peek adds the last message.
	lines, _ = m.boardCard(s, 40, cardOpts{quietRow: true, sel: true})
	if got := plainLines(lines); len(got) != 2 || got[0] != "▌╿ flint-auth     asks            ✻  2m " || got[1] != "   “I can fix the login in two ways.”   " {
		t.Errorf("a quiet row:\n%q", got)
	}
	if lines, _ = m.boardCard(s, 26, cardOpts{quietRow: true}); strings.Contains(ansi.Strip(lines[0]), "…") {
		t.Errorf("a quiet row cuts its word: %q", ansi.Strip(lines[0]))
	}

	// A narrow card: the stub goes, then a context under 70%.
	lines, _ = m.boardCard(s, 20, cardOpts{need: true})
	if got := ansi.Strip(lines[2]); got != "  #4551 ●━◉┄○┄○┄○   " {
		t.Errorf("w=20, ctx 62%%: %q", got)
	}
	m, s = cardModel(t, func(s *sample.Session) { s.ContextPct = 82 })
	lines, _ = m.boardCard(s, 20, cardOpts{need: true})
	if got := ansi.Strip(lines[2]); got != "  #4551 ●━◉  ctx 82%" || !strings.Contains(lines[2], st.warn.Render("ctx 82%")) {
		t.Errorf("w=20, ctx 82%%: %q", lines[2])
	}
	m, s = cardModel(t, func(s *sample.Session) { s.Name = "flint-auth-refresh" })
	lines, _ = m.boardCard(s, 20, cardOpts{need: true})
	if got := ansi.Strip(lines[0]); got != " ╿ flint-aut… ✻  2m " {
		t.Errorf("w=20, a long name: %q", got)
	}

	// A match the owner parked, that asks.
	st2, s2 := cardState(nil)
	st2.Holds = []proto.Hold{{SessionID: s2.SessionID, Pane: s2.Pane, Name: s2.Name, By: holdByYou}}
	m = boardModel(t, st2)
	lines, _ = m.boardCard(s2, 38, cardOpts{})
	if got := ansi.Strip(lines[1]); !strings.HasPrefix(got, "  ⏸ parked · asks") {
		t.Errorf("a parked match that asks: %q", got)
	}
}

func TestCardIsAlwaysItsWidth(t *testing.T) {
	cases := map[string]func(*sample.Session){
		"a long name": func(s *sample.Session) { s.Name = strings.Repeat("kindle-legacy-", 5)[:60] },
		"a question in CJK": func(s *sample.Session) {
			s.Ask.Questions[0].Question = "ログインをどう直しますか"
			s.Ask.Questions[0].Options[0].Label = "トークンの確認を直す"
			s.LastLine = "ログインを直す道は二つあります。トークンの確認は小さな修正です。"
		},
		"no last message": func(s *sample.Session) { s.LastLine = "" },
		"another host":    func(s *sample.Session) { s.Host = "gpu-1" },
	}
	opts := []cardOpts{
		{}, {need: true}, {oneLine: true}, {oneLine: true, need: true}, {quietRow: true}, {quietRow: true, sel: true},
		{sel: true, need: true}, {sel: true, need: true, armed: 1}, {sel: true, need: true, sent: 1},
		{sel: true, bare: true}, {otherHost: true}, {lit: true},
	}
	for name, edit := range cases {
		m, s := cardModel(t, edit)
		if name == "another host" {
			s.Host = "gpu-1" // a host the console has not reached
		}
		for _, w := range []int{18, 20, 26, 38, 40, 80} {
			for _, o := range opts {
				if name == "another host" {
					o.otherHost = true
				}
				lines, taps := m.boardCard(s, w, o)
				if len(taps) != len(lines) {
					t.Errorf("%s, w=%d, %+v: %d lines, %d tap rows", name, w, o, len(lines), len(taps))
				}
				for i, l := range lines {
					if lipgloss.Width(l) != w {
						t.Errorf("%s, w=%d, %+v: line %d is %d cells: %q", name, w, o, i, lipgloss.Width(l), ansi.Strip(l))
					}
					if p := ansi.Strip(l); !utf8.ValidString(p) || strings.ContainsRune(p, utf8.RuneError) {
						t.Errorf("%s, w=%d: a rune is cut in half: %q", name, w, p)
					}
					for _, sp := range taps[i] {
						if sp.x0 < 0 || sp.x1 > w || sp.x0 >= sp.x1 {
							t.Errorf("%s, w=%d, %+v: a tap off its line: %+v", name, w, o, sp)
						}
					}
				}
				if name == "no last message" && o.sel {
					for _, l := range plainLines(lines) {
						if strings.Contains(l, "“") {
							t.Errorf("w=%d: an empty message draws a quote line: %q", w, l)
						}
					}
				}
			}
		}
	}
	m, s := cardModel(t, cases["another host"])
	s.Host = "gpu-1"
	if lines, _ := m.boardCard(s, 80, cardOpts{otherHost: true}); !strings.Contains(ansi.Strip(lines[0]), "flint-auth @gpu-1") {
		t.Errorf("a card of another host does not name it: %q", ansi.Strip(lines[0]))
	}
	m, s = cardModel(t, nil)
	lines, _ := m.boardCard(s, 80, cardOpts{sel: true, bare: true})
	for _, l := range plainLines(lines) {
		if strings.Contains(l, "“") || strings.Contains(l, "1. ") {
			t.Errorf("a bare chosen card repeats the live view: %q", l)
		}
	}
}

func TestTheCardsMarkFollowsTheAgent(t *testing.T) {
	for _, c := range []struct{ agent, provider, mark string }{
		{"opencode", "anthropic", "▣"},
		{"claude", "bedrock", "✻"},
		{"codex", "openai", "❋"},
		{"aider", "anthropic", "·"},
	} {
		m, s := cardModel(t, func(s *sample.Session) { s.Agent, s.Provider = c.agent, c.provider })
		lines, _ := m.boardCard(s, 38, cardOpts{need: true})
		if got := ansi.Strip(lines[0]); !strings.HasSuffix(got, " "+c.mark+"  2m ") {
			t.Errorf("%s on %s: %q, want the mark %s", c.agent, c.provider, got, c.mark)
		}
	}
	st := limitsState()
	st.Limits = accountBlocks()
	oc := boardMatch("flint-auth", time.Hour)
	oc.Agent, oc.Provider = "opencode", "anthropic"
	st.Sessions = append(st.Sessions, oc)
	m, _ := loadedWith(t, 132, st)
	if out := screen(m); !strings.Contains(out, "✻ Claude Max ") {
		t.Errorf("the dock's account row for the Anthropic plan lost ✻:\n%s", out)
	}
}
