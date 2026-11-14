package app

import (
	"io"
	"slices"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
)

// The board never acts on a row of Sessions it does not show (fix round 1).

// TestNoBoardKeyActsOnAHiddenRow: with the stoker running the night and
// Sessions' own row on another match, a key on the board that reaches no
// card of a session (every column hidden, a search that matches nothing,
// the copy check's door, a host line) never asks to take over that row:
// no question, or its n arms nothing and runs nothing.
func TestNoBoardKeyActsOnAHiddenRow(t *testing.T) {
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

	hidden := night(func(m *Model) {
		for _, c := range boardCols {
			m.flipHidden(c)
		}
	})
	check(t, "every column hidden", hidden, "x")
	check(t, "every column hidden", hidden, "enter")

	none := pressOn(night(nil), "/")
	none = pressOn(typeText(none, "zzzq").(Model), "enter")
	if _, ok := none.boardSelected(); ok {
		t.Fatal("a search that matches nothing chose a card")
	}
	check(t, "a search with no match", none, "x")
	check(t, "a search with no match", none, "enter")

	door := firstCard(night(func(m *Model) { m.viewPrefs.CopyCheckedDesk, m.viewPrefs.CopyCheckedPhone = false, false }))
	if door = pressOn(door, "up"); func() bool { line, ok := door.aboveChosen(); return !ok || line != "" }() {
		t.Fatal("↑ from the first card did not choose the copy check's door")
	}
	check(t, "the copy check's door", door, "enter")
	check(t, "the copy check's door", door, "x")

	now := lostAt.Add(-time.Second)
	m, _, conns := hostsAt(t, &now, "", 120, 30, "ws-1")
	home := hostState(sess(1, "hearth-api", "%1", busy), sess(3, "flint-auth", "%3", busy))
	v := runningNight(lostAt)
	home.Stoker = &v
	m = upd(t, m, stateMsg(stateAt(home, now)))
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(hostState(sess(2, "spark-docs", "%2", busy)))})
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: lostMsg{err: io.EOF, conn: conns["ws-1"]}})
	now = lostAt.Add(40 * time.Second)
	m = boardOn(upd(t, m, stateMsg(stateAt(home, now))))
	m.selPID, m.selHost, m.chose = 3, "", true
	m.boardChooseCard(colWork, matchOf(t, m, "hearth-api"))
	if m = pressOn(m, "up"); func() bool { line, ok := m.aboveChosen(); return !ok || line != "ws-1" }() {
		t.Fatal("↑ from the first card did not choose ws-1's host line")
	}
	m.opt.Run = func(argv []string) error { t.Errorf("the host line ran %q", argv); return nil }
	for _, k := range []string{"enter", "x"} {
		next, cmd := key(m, k)
		drain(cmd)
		if nm := next.(Model); nm.held != nil {
			if nm = pressOn(nm, "n"); nm.pending != nil && nm.pending.rec != "" {
				t.Errorf("a host line: %s then n armed %+v", k, nm.pending)
			}
		}
	}
}

// TestAKeyTouchesTheRestOfTheChosenCardsHost: while gpu-1's stoker rests,
// a key on gpu-1's card keeps gpu-1's rest, wherever Sessions' own row is.
func TestAKeyTouchesTheRestOfTheChosenCardsHost(t *testing.T) {
	rests := nightView()
	rests.Rests = lostAt
	m, home, conns := stokedHosts(t, nil, nil, rests)
	m = boardOn(m)
	m.selPID, m.selHost, m.chose = 1, "", true // Sessions' row: home-1
	gpu := matchOf(t, m, "gpu-run")
	m.boardChooseCard(m.columnOf(gpu), gpu)
	pressAll(m, "v")
	if got := stokerActs(conns["gpu-1"]); !slices.Equal(got, []string{"stoker:touch"}) {
		t.Errorf("gpu-1 got %v, want its rest touched", got)
	}
	if got := stokerActs(home); len(got) != 0 {
		t.Errorf("home got %v", got)
	}
}

// TestAShipCardChoosesNoRow: a ship card's match has no PID; choosing it
// never puts Sessions' selection on a session that has none either.
func TestAShipCardChoosesNoRow(t *testing.T) {
	st := screensState(t)
	for i := range st.Sessions {
		if st.Sessions[i].Name == "wick-mobile" {
			st.Sessions[i].PID = 0 // a session the sampler read no process of
		}
	}
	r := *st.Ships
	r.Ships = append(slices.Clone(r.Ships), proto.Ship{Repo: "hearth/hearth-api", Number: 4570, Kind: shipRunning, Stage: 1,
		Where: "/w/hearth-api", Since: demoAt.Add(-time.Hour)})
	st.Ships = &r
	m := boardShot(t, st, 120, 40, nil)
	card := matchOf(t, m, "hearth-api #4570")
	m.boardChooseCard(colShip, card)
	if s, ok := m.onChoice().selectedSession(); ok && s.Name == "wick-mobile" {
		t.Error("the ship card put Sessions' selection on wick-mobile, which has no PID")
	}
}
