package app

import (
	"maps"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// inked is one cell of a frame, its colours named by their tokens.
type inked struct {
	ch, fg, bg string
}

// inkedCells reads a frame cell by cell, the way the replay reads it.
func inkedCells(t *testing.T, view string) [][]inked {
	t.Helper()
	var out [][]inked
	for _, line := range parsed(t, view) {
		var row []inked
		for _, sp := range line {
			for _, r := range sp.Text {
				switch ansi.StringWidth(string(r)) {
				case 0:
					if len(row) > 0 {
						row[len(row)-1].ch += string(r)
					}
				case 2:
					row = append(row, inked{string(r), sp.FG, sp.BG}, inked{"", sp.FG, sp.BG})
				default:
					row = append(row, inked{string(r), sp.FG, sp.BG})
				}
			}
		}
		out = append(out, row)
	}
	return out
}

func textOf(row []inked, from, to int) string {
	var b strings.Builder
	for x := max(from, 0); x < min(to, len(row)); x++ {
		b.WriteString(row[x].ch)
	}
	return b.String()
}

// headOf finds the head of name's card in a frame: the line and the cell
// where the name starts, after its match and a space.
func headOf(cells [][]inked, name string) (y, x int, ok bool) {
	for y, row := range cells {
		for x := 2; x+len(name) <= len(row); x++ {
			if textOf(row, x, x+len(name)) == name && strings.Contains("✦✧╿│·", row[x-2].ch) && row[x-1].ch == " " {
				return y, x, true
			}
		}
	}
	return 0, 0, false
}

// motionBoard is the board of st at 120 by 34 with motion on, on the
// clock *now.
func motionBoard(t *testing.T, st proto.State, now *time.Time, noMotion bool) Model {
	t.Helper()
	m := (New(Options{Conn: newFake(), NoMotion: noMotion, Now: func() time.Time { return *now }}))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	next, _ = next.Update(stateMsg(st))
	m = next.(Model)
	m.splashDone = true
	m.viewPrefs.CopyCheckedPhone, m.viewPrefs.CopyCheckedDesk = true, true
	return m
}

// TestOnlyWaitingAndFireMove: with no change in play, the only cells that
// differ between two frames 200 ms apart are the flames in WORKING and the
// heads that ask in NEEDS YOU. A parked match that asks is in HELD, and
// HELD does not move.
func TestOnlyWaitingAndFireMove(t *testing.T) {
	st := boardFixture(t)
	for _, s := range st.Sessions {
		if s.Name == "kindle-legacy" {
			st.Queue = append(st.Queue, waitFor(s, queue.StatePermission, time.Hour))
		}
	}
	now := demoAt
	m := boardShot(t, st, 120, 34, func(m *Model) {
		m.opt.NoMotion, m.opt.Now = false, func() time.Time { return now }
	})
	byName := map[string]sample.Session{}
	for _, s := range m.all.sessions {
		byName[s.Name] = s
	}
	moved := 0
	for step := range 12 {
		now = demoAt.Add(time.Duration(step) * 200 * time.Millisecond)
		a := inkedCells(t, m.render())
		now = now.Add(200 * time.Millisecond)
		b := inkedCells(t, m.render())
		for y := range min(len(a), len(b)) {
			for x := range min(len(a[y]), len(b[y])) {
				if a[y][x] == b[y][x] {
					continue
				}
				moved++
				name, _, _ := strings.Cut(strings.TrimSpace(textOf(b[y], x+2, len(b[y]))), " ")
				name, _, _ = strings.Cut(name, "@") // another host's row names it
				s, ok := byName[strings.TrimSuffix(name, "…")]
				for full, x := range byName {
					if !ok && strings.HasSuffix(name, "…") && strings.HasPrefix(full, strings.TrimSuffix(name, "…")) {
						s, ok = x, true
					}
				}
				if !ok || !strings.Contains("✦✧╿", a[y][x].ch) {
					t.Fatalf("+%dms line %d cell %d moves: %+v → %+v, not a match's head (%q)", step*200, y, x, a[y][x], b[y][x], textOf(b[y], 0, len(b[y])))
				}
				h := m.sideStateOf(s)
				fire := h == sideWork && m.matchOf(s) == matchLit
				asks := h == sideNeed && m.asks(s)
				if !fire && !asks {
					t.Fatalf("+%dms: the match of %s moves in %s: %+v → %+v", step*200, name, h.head(), a[y][x], b[y][x])
				}
			}
		}
	}
	if moved == 0 {
		t.Fatal("nothing moves: the flames in WORKING should flicker")
	}
}

// TestNoMotionShowsTheEndState: with --no-motion a change and a ship that
// passes a stage show their end state at once, and no frame differs.
func TestNoMotionShowsTheEndState(t *testing.T) {
	st, s := cardState(nil)
	other := boardMatch("hearth-api", 2*time.Hour)
	st.Sessions = append(st.Sessions, other)
	st.Queue = append(st.Queue, waitFor(other, queue.StatePermission, 10*time.Minute))
	now := boardAt
	m := motionBoard(t, st, &now, true)

	next := st
	next.Sessions = append([]sample.Session(nil), st.Sessions...)
	next.Sessions[0].Busy, next.Sessions[0].Status, next.Sessions[0].Ask = true, "busy", nil
	next.Queue = []queue.Item{waitFor(other, queue.StatePermission, 10*time.Minute)}
	next.Ships = &proto.Ships{Ships: []proto.Ship{shipFor(s, 4551, shipRunning, "green", 2, time.Minute)}}
	upd, _ := m.Update(stateMsg(next))
	m = upd.(Model)
	if m.animating || len(m.changes) > 0 {
		t.Fatalf("no motion, yet a change plays: ticks %v, changes %v", m.animating, m.changes)
	}
	first := m.render()
	cells := inkedCells(t, first)
	y, x, ok := headOf(cells, s.Name)
	if !ok {
		t.Fatalf("no card of %s:\n%s", s.Name, ansi.Strip(first))
	}
	if got := cells[y][x-2]; got.ch != "✦" || got.fg != "flame" {
		t.Errorf("no motion shows the burning match at once: %+v", got)
	}
	if got := cells[y][x]; got.fg != "fg" {
		t.Errorf("no motion: the name glows: %+v", got)
	}
	for _, e := range []time.Duration{100, 200, 500, 800, 1100, 1300, 2400} {
		now = boardAt.Add(e * time.Millisecond)
		if got := m.render(); got != first {
			t.Fatalf("no motion, yet the frame at +%dms differs:\n%s\nwas\n%s", e, ansi.Strip(got), ansi.Strip(first))
		}
	}
}

// sixStates is a session in each state a list row draws (DESIGN.md §4):
// works, asks, done, idle, held and spent.
func sixStates() proto.State {
	work, ask, idle := boardMatch("hearth-api", time.Hour), boardMatch("flint-auth", time.Hour), boardMatch("cinder-ci", time.Hour)
	held, spent := boardMatch("ember-billing", time.Hour), boardMatch("wick-vat", time.Hour)
	work.Busy, work.Status, work.Idle = true, "busy", 0
	st := boardState(work, ask, doneMatch("kindle-web", time.Minute), idle, held, spent)
	for i := range st.Sessions[:5] {
		st.Sessions[i] = unspent(st.Sessions[i])
	}
	st.Queue = []queue.Item{waitFor(ask, queue.StatePermission, time.Minute)}
	st.Holds = []proto.Hold{{SessionID: held.SessionID, Pane: held.Pane, Name: held.Name, By: holdByYou}}
	return st
}

// unspent is s with room left in its context: the fixture's sessions are
// at the compact limit.
func unspent(s sample.Session) sample.Session {
	s.ContextPct = 30
	return s
}

// rowMatch is the match before name's row in a frame: at the list's
// left edge, not in the agent's title.
func rowMatch(cells [][]inked, name string) (inked, bool) {
	for _, row := range cells {
		for x := 2; x <= 6 && x+len(name) <= len(row); x++ {
			if textOf(row, x, x+len(name)) == name && row[x-1].ch == " " {
				return row[x-2], true
			}
		}
	}
	return inked{}, false
}

func TestDoneIsAWarmMatch(t *testing.T) {
	if w := ansi.StringWidth("✓"); w != 1 {
		t.Fatalf("✓ takes %d cells, want 1", w)
	}
	for _, noMotion := range []bool{false, true} {
		now := boardAt
		m := boardModel(t, boardState(unspent(doneMatch("kindle-web", time.Minute))))
		m.opt.NoMotion, m.opt.Now, m.splashDone = noMotion, func() time.Time { return now }, true
		for step := range 13 {
			now = boardAt.Add(time.Duration(step) * 200 * time.Millisecond)
			if c, ok := rowMatch(inkedCells(t, m.render()), "kindle-web"); !ok || c.ch != "✓" || c.fg != "match-ember" {
				t.Fatalf("no motion %v, +%dms: the done match is %+v (found %v), want ✓ in match-ember:\n%s", noMotion, step*200, c, ok, plainFrame(m))
			}
		}
	}
}

func TestEachStateHasItsOwnGlyph(t *testing.T) {
	want := map[string]string{"hearth-api": "✦", "flint-auth": "╿", "kindle-web": "✓", "cinder-ci": "╵", "ember-billing": "╽", "wick-vat": "│"}
	chained := sixStates()
	chained.Chains = &state.ChainsView{Chains: []chains.Chain{{ID: "c1", Name: "login", Repo: "/home/user/src/hearth"}}}
	for _, s := range chained.Sessions {
		chained.Chains.Chains[0].Members = append(chained.Chains.Chains[0].Members, memberOf(s))
	}
	for what, st := range map[string]proto.State{"a row": sixStates(), "a chain member": chained} {
		lines, _, _ := boardModel(t, st).sideList(80, 40)
		got := map[string]string{}
		for _, l := range plainLines(lines) {
			r := []rune(l)
			for name := range want {
				if strings.Contains(l, " "+name) && len(r) > 3 {
					at := 1
					if strings.ContainsRune("├└", r[1]) {
						at = 3
					}
					got[name] = string(r[at])
				}
			}
		}
		if !maps.Equal(got, want) {
			t.Errorf("%s: glyphs %q, want %q:\n%s", what, got, want, strings.Join(plainLines(lines), "\n"))
		}
	}
}

// TestTheStateWinsOverSpent: at the compact limit a session keeps its
// state's glyph; only one at rest draws spent (owner, 2026-11-23).
func TestTheStateWinsOverSpent(t *testing.T) {
	st := sixStates()
	for i := range st.Sessions {
		st.Sessions[i].ContextPct = 90
	}
	want := map[string]string{"hearth-api": "✦", "flint-auth": "╿", "kindle-web": "✓", "cinder-ci": "│", "ember-billing": "╽", "wick-vat": "│"}
	for _, noMotion := range []bool{true, false} {
		m := boardModel(t, st)
		m.opt.NoMotion, m.splashDone = noMotion, true
		cells := inkedCells(t, m.render())
		for name, g := range want {
			if c, ok := rowMatch(cells, name); !ok || c.ch != g {
				t.Errorf("no motion %v: %s at the limit draws %+v (found %v), want %s", noMotion, name, c, ok, g)
			}
		}
	}
	now := boardAt
	m := boardModel(t, st)
	m.opt.NoMotion, m.opt.Now, m.splashDone = false, func() time.Time { return now }, true
	first := inkedCells(t, m.render())
	now = boardAt.Add(1200 * time.Millisecond)
	later := inkedCells(t, m.render())
	for _, name := range []string{"flint-auth", "hearth-api"} {
		a, _ := rowMatch(first, name)
		b, _ := rowMatch(later, name)
		if a != b {
			t.Errorf("%s at the limit moves: %+v at +0ms, %+v at +1200ms; want it still", name, a, b)
		}
	}
}
