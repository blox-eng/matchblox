package app

import (
	"slices"
	"testing"

	"github.com/blox-eng/matchblox/internal/proto"
)

// The list never acts on a row of Sessions it does not show (fix round 1).

// TestNoListKeyActsOnAHiddenRow: with the stoker running the night and
// Sessions' own row on another match, a key in the list that reaches no
// session (a search that matches nothing) never asks to take over that
// row: no question, or its n arms nothing and runs nothing.
func TestNoListKeyActsOnAHiddenRow(t *testing.T) {
	night := func(edit func(*Model)) Model {
		v := runningNight(demoAt)
		m := portShot(t, 120, 40, "", func(st *proto.State) { st.Stoker = &v })
		m.selPID, m.selHost, m.chose = matchOf(t, m, "flint-auth").PID, "", true
		if edit != nil {
			edit(&m)
		}
		return m
	}
	check := func(t *testing.T, name string, m Model, k string) {
		t.Helper()
		ran := recordRuns(&m)
		next, cmd := key(m, k)
		drain(cmd)
		if nm := next.(Model); nm.held != nil {
			next, cmd = key(nm, "n")
			drain(cmd)
		}
		if nm := next.(Model); nm.pending != nil || len(*ran) != 0 || len(boardFake(nm).acts()) != 0 {
			t.Errorf("%s: %s armed %+v, ran %q, sent %+v", name, k, nm.pending, *ran, boardFake(nm).acts())
		}
	}

	none := pressOn(firstCard(night(nil)), "/")
	none = pressOn(typeText(none, "zzzq").(Model), "enter")
	if _, ok := none.sideChosen(); ok {
		t.Fatal("a search that matches nothing chose a session")
	}
	check(t, "a search with no match", none, "x")
	check(t, "a search with no match", none, "enter")
}

// TestAKeyTouchesTheRestOfTheChosenCardsHost: while gpu-1's stoker rests,
// a key on gpu-1's card keeps gpu-1's rest, wherever Sessions' own row is.
func TestAKeyTouchesTheRestOfTheChosenCardsHost(t *testing.T) {
	rests := nightView()
	rests.Rests = lostAt
	m, home, conns := stokedHosts(t, nil, nil, rests)
	m = consoleOn(m)
	m.selPID, m.selHost, m.chose = 1, "", true // Sessions' row: home-1
	m.chooseRow(matchOf(t, m, "gpu-run"))
	m.toList()
	pressAll(m, "c")
	if got := stokerActs(conns["gpu-1"]); !slices.Equal(got, []string{"stoker:touch"}) {
		t.Errorf("gpu-1 got %v, want its rest touched", got)
	}
	if got := stokerActs(home); len(got) != 0 {
		t.Errorf("home got %v", got)
	}
}
