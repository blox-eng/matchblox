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
		{3, st.muted.Render("“I can fix the login in two ways.”"), "the quote"},
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
	long := s
	long.Name = "flint-auth-refresh-2"
	if lines, _ = m.boardCard(long, 80, cardOpts{quietRow: true}); !strings.Contains(ansi.Strip(lines[0]), "flint-auth-refresh-2 asks") {
		t.Errorf("a quiet row with room cuts a long name: %q", ansi.Strip(lines[0]))
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

// landState is flint-auth, which asks or works, beside hearth-api, which
// waits longer for a permission and so is the chosen card.
func landState(busy bool) (proto.State, sample.Session) {
	st, s := cardState(func(s *sample.Session) {
		if busy {
			s.Busy, s.Status, s.Ask = true, "busy", nil
		}
	})
	other := boardMatch("hearth-api", 2*time.Hour)
	st.Sessions = append(st.Sessions, other)
	st.Queue = []queue.Item{waitFor(other, queue.StatePermission, 10*time.Minute)}
	if !busy {
		st.Queue = append(st.Queue, waitFor(s, queue.StateQuestion, 2*time.Minute))
	}
	return st, s
}

// TestACardLandsWithATrueSign (design decision 7): a match that starts to
// work lands in WORKING with the strike and its name in flame-core for
// 1.2 s; one that stops lands elsewhere with the going out, on the wash,
// with no fire.
func TestACardLandsWithATrueSign(t *testing.T) {
	t.Run("into WORKING", func(t *testing.T) {
		before, s := landState(false)
		at := boardAt
		now := at
		m := motionBoard(t, before, &now, false)
		after, _ := landState(true)
		next, cmd := m.Update(stateMsg(after))
		m = next.(Model)
		if cmd == nil || !m.animating {
			t.Fatal("no tick for the landing")
		}
		if c := m.columnOf(m.all.sessions[slices.IndexFunc(m.all.sessions, func(x sample.Session) bool { return x.Name == s.Name })]); c != colWork {
			t.Fatalf("%s is in %s, want WORKING", s.Name, c.name())
		}
		for _, f := range []struct {
			e     time.Duration
			glyph string
			ink   []string
			name  string
		}{
			{0, "╿", []string{"flame-core"}, "flame-core"},
			{200 * time.Millisecond, "·", []string{"flame-core"}, "flame-core"},
			{500 * time.Millisecond, "✧", []string{"flame-amber"}, "flame-core"},
			{800 * time.Millisecond, "✦✧", []string{"flame", "flame-amber", "flame-core"}, "flame-core"},
			{1100 * time.Millisecond, "✦✧", []string{"flame", "flame-amber", "flame-core"}, "flame-core"},
			{landGlow, "✦✧", []string{"flame", "flame-amber", "flame-core"}, "fg"},
		} {
			now = at.Add(f.e)
			d := m.boardPanel(120, 30)
			inWork := false
			for spot, k := range d.cards {
				inWork = inWork || spot.col == colWork && k.pane == s.Pane
			}
			if !inWork {
				t.Fatalf("+%v: the card of %s is not in WORKING", f.e, s.Name)
			}
			cells := inkedCells(t, m.render())
			y, x, ok := headOf(cells, s.Name)
			if !ok {
				t.Fatalf("+%v: no card of %s", f.e, s.Name)
			}
			g := cells[y][x-2]
			if !strings.Contains(f.glyph, g.ch) || !slices.Contains(f.ink, g.fg) {
				t.Errorf("+%v: the match is %+v, want %s in %v", f.e, g, f.glyph, f.ink)
			}
			for i := range len(s.Name) {
				if c := cells[y][x+i]; c.fg != f.name || c.bg == "wash" {
					t.Fatalf("+%v: the name's cell %d is %+v, want %s off the wash", f.e, i, c, f.name)
				}
			}
		}
	})

	t.Run("into NEEDS YOU", func(t *testing.T) {
		before, s := landState(true)
		at := boardAt
		now := at
		m := motionBoard(t, before, &now, false)
		after, _ := landState(false)
		next, _ := m.Update(stateMsg(after))
		m = next.(Model)
		for _, f := range []struct {
			e     time.Duration
			glyph string
			ink   string
			wash  bool
		}{
			{0, "✧", "flame-amber", true},
			{250 * time.Millisecond, "✧", "match-ember", true},
			{500 * time.Millisecond, "╿", "", true},
			{1100 * time.Millisecond, "╿", "", true},
			{landGlow, "╿", "", false},
		} {
			now = at.Add(f.e)
			cells := inkedCells(t, m.render())
			y, x, ok := headOf(cells, s.Name)
			if !ok {
				t.Fatalf("+%v: no card of %s", f.e, s.Name)
			}
			if x > 40 {
				t.Fatalf("+%v: the card of %s is not in NEEDS YOU (cell %d)", f.e, s.Name, x)
			}
			if g := cells[y][x-2]; g.ch != f.glyph || f.ink != "" && g.fg != f.ink {
				t.Errorf("+%v: the match is %+v, want %s %s", f.e, g, f.glyph, f.ink)
			}
			for dy := range 2 {
				for cx := x - 3; cx < x+20; cx++ {
					c := cells[y+dy][cx]
					if c.fg == "flame-core" {
						t.Fatalf("+%v: line %d cell %d is flame-core: %+v", f.e, dy, cx, c)
					}
					if (c.bg == "wash") != f.wash {
						t.Fatalf("+%v: line %d cell %d is %+v, want on the wash: %v", f.e, dy, cx, c, f.wash)
					}
				}
			}
		}
	})

	t.Run("the ticks stop after the glow", func(t *testing.T) {
		before, _ := cardState(func(s *sample.Session) { s.Busy, s.Status, s.Ask = true, "busy", nil })
		before.Queue = nil
		at := boardAt
		now := at
		m := motionBoard(t, before, &now, false)
		after, _ := cardState(func(s *sample.Session) { s.Ask = nil })
		after.Queue = nil
		next, _ := m.Update(stateMsg(after))
		m = next.(Model)
		now = at.Add(landGlow - 100*time.Millisecond)
		if !m.moving() {
			t.Fatal("the glow still plays, yet the console stops drawing")
		}
		now = at.Add(landGlow + 100*time.Millisecond)
		next, cmd := m.Update(animMsg{})
		if cmd != nil || next.(Model).animating {
			t.Fatal("the glow is over and nothing moves, yet the console keeps ticking")
		}
	})
}

// asksState is landState's match asking its question, busy or idle: in
// NEEDS YOU either way.
func asksState(busy bool) (proto.State, sample.Session) {
	st, s := landState(false)
	for i := range st.Sessions {
		if st.Sessions[i].Name == s.Name {
			st.Sessions[i].Busy = busy
		}
	}
	return st, s
}

// cardCells are the first two lines of the card of name, from its
// selection mark on, at now.
func cardCells(t *testing.T, m Model, name string) [][]inked {
	t.Helper()
	cells := inkedCells(t, m.render())
	y, x, ok := headOf(cells, name)
	if !ok {
		t.Fatalf("no card of %s", name)
	}
	if x > 40 {
		t.Fatalf("the card of %s is not in NEEDS YOU (cell %d)", name, x)
	}
	return [][]inked{cells[y][x-3 : x+20], cells[y+1][x-3 : x+20]}
}

// TestFireLandsOnlyInWorking (design decision 7): a match that strikes
// and lands in NEEDS YOU sits on the wash with its still flame, never
// the strike's flame-core; one that goes out and stays in its column
// does not land.
func TestFireLandsOnlyInWorking(t *testing.T) {
	t.Run("a strike into NEEDS YOU", func(t *testing.T) {
		before, s := landState(false)
		before.Queue = before.Queue[:1] // hearth-api's wait only
		for i := range before.Sessions {
			if before.Sessions[i].Name == s.Name {
				before.Sessions[i].Ask = nil
			}
		}
		at := boardAt
		now := at
		m := motionBoard(t, before, &now, false)
		if c := m.columnOf(m.all.sessions[slices.IndexFunc(m.all.sessions, func(x sample.Session) bool { return x.Name == s.Name })]); c != colShip {
			t.Fatalf("%s starts in %s, want SHIPPING", s.Name, c.name())
		}
		after, _ := asksState(true)
		next, _ := m.Update(stateMsg(after))
		m = next.(Model)
		for _, e := range []time.Duration{0, 200 * time.Millisecond, 500 * time.Millisecond, 800 * time.Millisecond, 1100 * time.Millisecond} {
			now = at.Add(e)
			for dy, row := range cardCells(t, m, s.Name) {
				for cx, c := range row {
					if c.fg == "flame-core" {
						t.Fatalf("+%v: line %d cell %d is flame-core: %+v", e, dy, cx, c)
					}
					if c.bg != "wash" {
						t.Fatalf("+%v: line %d cell %d is %+v, want on the wash", e, dy, cx, c)
					}
				}
			}
		}
	})

	t.Run("no move, no landing", func(t *testing.T) {
		before, s := asksState(true)
		at := boardAt
		now := at
		m := motionBoard(t, before, &now, false)
		after, _ := asksState(false)
		next, _ := m.Update(stateMsg(after))
		m = next.(Model)
		for _, e := range []time.Duration{0, 500 * time.Millisecond, 1100 * time.Millisecond} {
			now = at.Add(e)
			for dy, row := range cardCells(t, m, s.Name) {
				for cx, c := range row {
					if c.bg == "wash" {
						t.Fatalf("+%v: line %d cell %d is %+v on the wash: the card did not move", e, dy, cx, c)
					}
				}
			}
		}
	})
}

// TestAShipThatPassesAStageSparksOnce: a ship that goes from ci to green
// runs one bright cell along its track once, on the board only.
func TestAShipThatPassesAStageSparksOnce(t *testing.T) {
	before, s := landState(false)
	at := boardAt
	now := at
	m := motionBoard(t, before, &now, false)
	after, _ := landState(false)
	after.Ships = &proto.Ships{Ships: []proto.Ship{shipFor(s, 4551, shipRunning, "green", 2, 0)}}
	next, _ := m.Update(stateMsg(after))
	m = next.(Model)
	if c, ok := m.changes[shipKey(m.ships()[0])]; !ok || c.kind != changeSpark {
		t.Fatalf("no spark for the ship: %v", m.changes)
	}
	trackAt := func(cells [][]inked) []inked {
		for _, row := range cells {
			for x := 0; x+6 <= len(row); x++ {
				if textOf(row, x, x+6) == "#4551 " {
					return row[x+6 : x+6+trackW]
				}
			}
		}
		t.Fatal("no track of #4551")
		return nil
	}
	var visited []int
	for e := time.Duration(0); e < 2*sparkFor; e += 20 * time.Millisecond {
		now = at.Add(e)
		var bright []int
		for i, c := range trackAt(inkedCells(t, m.render())) {
			if c.fg == "flame-core" {
				bright = append(bright, i)
			}
		}
		switch {
		case e < sparkFor && len(bright) != 1:
			t.Fatalf("+%v: %d bright cells on the track, want 1", e, len(bright))
		case e >= sparkFor && len(bright) != 0:
			t.Fatalf("+%v: the spark runs again: %v", e, bright)
		case len(bright) == 1:
			if n := len(visited); n > 0 && bright[0] < visited[n-1] {
				t.Fatalf("+%v: the spark runs back: %v then %d", e, visited, bright[0])
			}
			visited = append(visited, bright[0])
		}
	}
	if visited[0] != 0 || visited[len(visited)-1] != trackW-1 {
		t.Errorf("the spark does not run the whole track: %v", visited)
	}

	// The Ships tab keeps its look: no spark there.
	now = at.Add(sparkFor / 2)
	sh := m.ships()[0]
	if got, want := m.track(sh), m.trackAt(sh, -1); got != want {
		t.Errorf("the Ships tab's track sparks: %q", got)
	}
}
