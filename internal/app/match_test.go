package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
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
	m := New(Options{Conn: newFake(), NoMotion: noMotion, Now: func() time.Time { return *now }})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	next, _ = next.Update(stateMsg(st))
	m = next.(Model)
	m.splashDone = true
	m.viewPrefs.CopyCheckedPhone, m.viewPrefs.CopyCheckedDesk = true, true
	return m
}

// TestOnlyWaitingAndFireMove: with no change in play, the only cells that
// differ between two frames 200 ms apart are the flames in WORKING and the
// heads that ask in NEEDS YOU. A parked match that asks is in QUIET, and
// QUIET does not move.
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
				s, ok := byName[strings.TrimSuffix(name, "…")]
				for full, x := range byName {
					if !ok && strings.HasSuffix(name, "…") && strings.HasPrefix(full, strings.TrimSuffix(name, "…")) {
						s, ok = x, true
					}
				}
				if !ok || !strings.Contains("✦✧╿", a[y][x].ch) {
					t.Fatalf("+%dms line %d cell %d moves: %+v → %+v, not a match's head (%q)", step*200, y, x, a[y][x], b[y][x], textOf(b[y], 0, len(b[y])))
				}
				c := m.columnOf(s)
				fire := c == colWork && m.matchOf(s) == matchLit
				asks := c == colNeed && m.asks(s)
				if !fire && !asks {
					t.Fatalf("+%dms: the match of %s moves in %s: %+v → %+v", step*200, name, c.name(), a[y][x], b[y][x])
				}
			}
		}
	}
	if moved == 0 {
		t.Fatal("nothing moves: the flames in WORKING should flicker")
	}
}

// TestQuietCoolsWithAge: the head of a QUIET match is match-head while
// idle, match-ember when cold, and faint when stale, parked or held.
func TestQuietCoolsWithAge(t *testing.T) {
	s := boardMatch("ember-reports", time.Hour)
	for _, c := range []struct {
		what string
		idle time.Duration
		held bool
		want string
	}{
		{"idle", 3 * time.Hour, false, "match-head"},
		{"cold", 2 * 24 * time.Hour, false, "match-ember"},
		{"stale", 9 * 24 * time.Hour, false, "faint"},
		{"parked or held", 5 * time.Hour, true, "faint"},
	} {
		s.Idle = c.idle
		if got := quietTone(s, c.held); got != c.want {
			t.Errorf("%s: %s, want %s", c.what, got, c.want)
		}
	}

	// On the board, with motion on: the heads in QUIET are still and cool.
	now := demoAt
	m := boardShot(t, boardFixture(t), 120, 34, func(m *Model) {
		m.opt.NoMotion, m.opt.Now = false, func() time.Time { return now }
	})
	cells := inkedCells(t, m.render())
	// QUIET is narrow at 120 columns: the names are cut.
	for name, want := range map[string]string{"ember-rep": "match-head", "tinder-sc": "match-ember", "kindle-le": "faint"} {
		y, x, ok := headOf(cells, name)
		if !ok {
			t.Fatalf("no card of %s", name)
		}
		if got := cells[y][x-2]; got.ch != "╿" || got.fg != want {
			t.Errorf("the head of %s is %+v, want ╿ in %s", name, got, want)
		}
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
