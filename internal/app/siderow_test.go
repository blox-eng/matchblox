package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
)

func needsYou(t *testing.T) (Model, sample.Session) {
	t.Helper()
	s := boardMatch("flint-auth", time.Hour)
	st := boardState(s)
	st.Queue = append(st.Queue, waitFor(s, queue.StatePermission, 4*time.Minute))
	m := boardModel(t, st)
	return m, m.all.sessions[0]
}

func TestRowIsOneLineUnlessItNeedsYou(t *testing.T) {
	s := boardMatch("hearth-api", time.Hour)
	s.Idle = 30 * time.Hour
	m := boardModel(t, boardState(s))
	if got := m.sideRow(m.all.sessions[0], 40, false, sideLink{}); len(got) != 1 {
		t.Fatalf("an idle row has %d lines, want 1: %q", len(got), got)
	}
	m, a := needsYou(t)
	if got := m.sideRow(a, 40, false, sideLink{}); len(got) != 2 {
		t.Fatalf("a row that needs you has %d lines, want 2: %q", len(got), got)
	}
}

func TestChosenRowHasASecondLine(t *testing.T) {
	s := boardMatch("hearth-api", time.Hour)
	s.LastLine = "Reading the router."
	m := boardModel(t, boardState(s))
	got := plainLines(m.sideRow(m.all.sessions[0], 40, true, sideLink{}))
	if len(got) != 2 || !strings.HasPrefix(got[0], "▌") || !strings.Contains(got[1], "Reading the router.") {
		t.Fatalf("chosen row %q, want the mark and its doing line", got)
	}
	unchosen := boardMatch("hearth-api", time.Hour)
	unchosen.Idle = 30 * time.Hour
	um := boardModel(t, boardState(unchosen))
	if got := plainLines(um.sideRow(um.all.sessions[0], 40, false, sideLink{})); len(got) != 1 || strings.HasPrefix(got[0], "▌") {
		t.Fatalf("an unchosen row %q has the mark or a second line", got)
	}
}

func TestHeldRowSaysWhyWhenChosen(t *testing.T) {
	s := boardMatch("hearth-api", time.Hour)
	st := boardState(s)
	st.Holds = append(st.Holds, proto.Hold{SessionID: s.SessionID, Pane: s.Pane, Name: s.Name, By: holdByYou})
	m := boardModel(t, st)
	s = m.all.sessions[0]
	got := plainLines(m.sideRow(s, 50, true, sideLink{}))
	if len(got) != 2 || !strings.Contains(got[1], "parked by you") {
		t.Fatalf("held row %q, want its reason", got)
	}
}

func TestChosenRowWithNothingSaidStillHasASecondLine(t *testing.T) {
	m := boardModel(t, boardState(boardMatch("hearth-api", time.Hour)))
	s := m.all.sessions[0]
	s.LastLine = ""
	got := plainLines(m.sideRow(s, 40, true, sideLink{}))
	if len(got) != 2 || strings.TrimSpace(got[1]) == "" {
		t.Fatalf("chosen row %q, want a second line with the state word", got)
	}
}

func TestHeldSessionThatAsksShowsItsQuestion(t *testing.T) {
	s := boardMatch("flint-auth", time.Hour)
	s.Ask = &said.Ask{ID: "toolu_a", Questions: []said.Question{{Question: "Do I skip the lint?"}}}
	s.Asks, s.Reported, s.Status = true, true, "waiting"
	st := boardState(s)
	st.Holds = append(st.Holds, proto.Hold{SessionID: s.SessionID, Pane: s.Pane, Name: s.Name, By: "stoker", Why: "not the priority"})
	m := boardModel(t, st)
	s = m.all.sessions[0]
	if m.sideStateOf(s) != sideNeed {
		t.Fatalf("fixture state is %v, not NEEDS YOU", m.sideStateOf(s))
	}
	got := plainLines(m.sideRow(s, 50, false, sideLink{}))
	if len(got) != 2 || !strings.Contains(got[1], "Do I skip the lint?") {
		t.Fatalf("row %q, want the question", got)
	}
}

func TestRowEndsWithTheAgentsMark(t *testing.T) {
	for agent, mark := range map[string]string{"claude": "✻", "codex": "❋", "cursor": "⬢"} {
		s := boardMatch("hearth-api", time.Hour)
		s.Agent = agent
		m := boardModel(t, boardState(s))
		l := strings.TrimRight(plainLines(m.sideRow(m.all.sessions[0], 40, false, sideLink{}))[0], " ")
		if !strings.HasSuffix(l, mark) {
			t.Errorf("%s: row %q does not end with %s", agent, l, mark)
		}
	}
}

func TestRowShowsAnotherHost(t *testing.T) {
	m, _ := twoHostBoard(t, 120, "flint-auth")
	var other, home sample.Session
	for _, s := range m.all.sessions {
		if s.Host != m.opt.Host {
			other = s
		} else {
			home = s
		}
	}
	if l := plainLines(m.sideRow(other, 50, false, sideLink{}))[0]; !strings.Contains(l, "flint-auth@ws-1") {
		t.Errorf("another host's row %q, want flint-auth@ws-1", l)
	}
	if l := plainLines(m.sideRow(home, 50, false, sideLink{}))[0]; strings.Contains(l, "@") {
		t.Errorf("the home row %q has a host", l)
	}
	one := boardModel(t, boardState(boardMatch("flint-auth", time.Hour)))
	if l := plainLines(one.sideRow(one.all.sessions[0], 50, false, sideLink{}))[0]; strings.Contains(l, "@") {
		t.Errorf("one host: row %q has a host", l)
	}
}

func TestBadgeIsNumberAndStage(t *testing.T) {
	s := boardMatch("hearth-api", time.Hour)
	st := boardState(s)
	st.Ships.Ships = append(st.Ships.Ships, shipFor(s, 4551, shipRunning, "CI runs", 1, time.Hour))
	m := boardModel(t, st)
	l := plainLines(m.sideRow(m.all.sessions[0], 50, false, sideLink{}))[0]
	if !strings.Contains(l, "#4551 ci") {
		t.Fatalf("row %q, want the badge #4551 ci", l)
	}
}

func TestLinksLightOnlyWhenTheChosenIsInTheChain(t *testing.T) {
	m := loginChain(t, nil)
	s := m.all.sessions[slices.IndexFunc(m.all.sessions, func(s sample.Session) bool { return s.Name == "flint-auth" })]
	lit := m.sideRow(s, 40, false, sideLink{i: 0, n: 3, lit: true})
	dim := m.sideRow(s, 40, false, sideLink{i: 0, n: 3, lit: false})
	for i := range lit {
		if lit[i] == dim[i] {
			t.Errorf("line %d: a lit link draws as an unlit one: %q", i, lit[i])
		}
	}
	got := plainLines(lit)
	if !strings.Contains(got[0], "├") || !strings.HasPrefix(got[1], "   │") {
		t.Errorf("links %q, want ├ then │ under a two-line row", got)
	}
	last := plainLines(m.sideRow(s, 40, false, sideLink{i: 2, n: 3}))
	if !strings.Contains(last[0], "└") || strings.Contains(last[1], "│") {
		t.Errorf("last link %q, want └ and no │", last)
	}
	f, r := linkOf(1, 3, true)
	if f != "├" || r != "│" {
		t.Errorf("linkOf(1,3,true) = %q %q", f, r)
	}
	if f, r := linkOf(0, 1, true); f != "└" || r != " " {
		t.Errorf("linkOf(0,1,true) = %q %q", f, r)
	}
}

func TestChainHeadNamesTheStep(t *testing.T) {
	m := loginChain(t, nil)
	u := m.sideGroups()[0].units[0]
	got := ansi.Strip(m.chainHead(u, 40, false))
	if !strings.HasPrefix(got, " ≋ login") || !strings.HasSuffix(strings.TrimRight(got, " "), "step 2 of 3 · 3") {
		t.Fatalf("head %q", got)
	}
	if m.chainHead(u, 40, true) == m.chainHead(u, 40, false) {
		t.Fatal("a lit head draws as an unlit one")
	}
}

func TestSideMatchHasAGlyphForEachState(t *testing.T) {
	m := loginChain(t, nil)
	want := map[sideState]string{sideNeed: "╿", sideDone: "✓", sideWork: "✦", sideHeld: "╽", sideIdle: "╵"}
	for h, g := range want {
		if got := ansi.Strip(m.sideMatch(sample.Session{}, h)); got != g {
			t.Errorf("%s: %q, want %q", h.head(), got, g)
		}
	}
}

func TestRowFitsItsWidth(t *testing.T) {
	m, a := needsYou(t)
	a.Agent = "cursor"
	rows := []Model{m}
	subjects := []sample.Session{a}
	ship := boardMatch("hearth-api", time.Hour)
	sst := boardState(ship)
	sst.Ships.Ships = append(sst.Ships.Ships, shipFor(ship, 4551, shipRunning, "CI runs", 1, time.Hour))
	sm := boardModel(t, sst)
	rows, subjects = append(rows, sm), append(subjects, sm.all.sessions[0])
	hm, _ := twoHostBoard(t, 120, "flint-auth")
	for _, s := range hm.all.sessions {
		if s.Host != hm.opt.Host {
			rows, subjects = append(rows, hm), append(subjects, s)
		}
	}
	for w := 24; w <= 60; w++ {
		for i := range rows {
			for _, l := range rows[i].sideRow(subjects[i], w, true, sideLink{i: 1, n: 3, lit: true}) {
				if lw := ansi.StringWidth(l); lw > w {
					t.Fatalf("width %d, row %d: line is %d wide: %q", w, i, lw, ansi.Strip(l))
				}
			}
		}
		for _, chosen := range []bool{false, true} {
			for _, link := range []sideLink{{}, {i: 0, n: 2, lit: true}} {
				for _, l := range m.sideRow(a, w, chosen, link) {
					if lw := ansi.StringWidth(l); lw > w {
						t.Fatalf("width %d: line is %d wide: %q", w, lw, ansi.Strip(l))
					}
				}
			}
		}
		u := m.sideGroups()[0].units[0]
		if lw := ansi.StringWidth(m.chainHead(u, w, true)); lw > w {
			t.Fatalf("width %d: head is %d wide", w, lw)
		}
	}
}
