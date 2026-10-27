package app

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
)

func shipsAt(t *testing.T, w int, st proto.State) (Model, *fakeConn) {
	t.Helper()
	m, f := loadedWith(t, w, st)
	next, _ := key(m, "p")
	return next.(Model), f
}

// TestShipsShowTheirTrack: each ship's number, title and age, its track
// and its note; the tab counts the ships in flight.
func TestShipsShowTheirTrack(t *testing.T) {
	for _, w := range []int{44, 86} {
		m, _ := shipsAt(t, w, screensState(t))
		out := ansi.Strip(m.render())
		for _, want := range []string{"SHIPS 3", "#4551 Rate limits", "●━◉┄○┄○┄○  CI · 6 of 9 jobs", "●━●━◉┄○┄○  green · waits for your merge",
			"●━✕┄○┄○┄○  conflicts with main", "●━●━●━●━●  live", "by hearth-api"} {
			if !strings.Contains(out, want) {
				t.Fatalf("width %d: Ships lacks %q:\n%s", w, want, out)
			}
		}
	}
}

// TestShipKeys: Enter selects the session that drives the ship in
// Sessions; x on one that waits for your merge shows its merge and runs it
// only after a typed y; a on a stuck one opens the peek on its session.
func TestShipKeys(t *testing.T) {
	st := screensState(t)
	m, f := shipsAt(t, 86, st)
	next, _ := key(m, "enter")
	if s, ok := next.(Model).selectedSession(); next.(Model).tab != tabSessions || !ok || s.Name != "hearth-api" {
		t.Fatalf("enter went to tab %d, session %q", next.(Model).tab, s.Name)
	}

	next, _ = key(m, "down") // #4552 waits for your merge
	next, _ = key(next, "x")
	if out := actionLine(next); !strings.Contains(out, "gh pr merge 4552 --squash -R hearth/ember-billing") {
		t.Fatalf("x does not show the merge:\n%s", out)
	}
	if _, cmd := key(next, "enter"); cmd != nil || len(f.acts()) != 0 {
		t.Fatal("enter alone merged")
	}
	_, cmd := key(next, "y")
	drain(cmd)
	if acts := f.acts(); len(acts) != 1 || acts[0] != (proto.Act{RecID: "ship:hearth/ember-billing#4552", Which: "secondary", Confirm: "y"}) {
		t.Fatalf("acts %+v", acts)
	}

	next, _ = key(m, "down")
	next, _ = key(next, "down") // #4545 is stuck; flint-auth asks
	next, _ = key(next, "a")
	if p := next.(Model).peek; p == nil || p.pane != "%2" {
		t.Fatalf("a on a stuck ship: peek %+v flash %q", p, next.(Model).flash)
	}
	next, _ = key(m, "a") // #4551 runs
	if next.(Model).peek != nil || next.(Model).flash == "" {
		t.Fatal("a on a ship that runs opened the peek")
	}
}

// TestShipsSayWhyTheyAreBehind: the tab says plainly when GitHub's rate
// limit pauses the reading, when gh cannot read, and which repository
// failed.
func TestShipsSayWhyTheyAreBehind(t *testing.T) {
	st := screensState(t)
	r := *st.Ships
	r.Paused, r.Unknown, r.Errors = time.Date(2026, 10, 1, 23, 33, 0, 0, time.Local), "gh not signed in", []string{"hearth/hearth-api: HTTP 502"}
	st.Ships = &r
	m, _ := shipsAt(t, 132, st)
	out := ansi.Strip(m.render())
	for _, want := range []string{"ships read again at 23:33", "GitHub cannot be read: gh not signed in", "not read: hearth/hearth-api: HTTP 502"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Ships lacks %q:\n%s", want, out)
		}
	}
	st.Ships = nil
	m, _ = shipsAt(t, 50, st)
	if out := ansi.Strip(m.render()); !strings.Contains(out, "No ships yet.") || strings.Contains(out, "SHIPS 0") {
		t.Fatalf("no ships:\n%s", out)
	}
}

// A session's row and detail show the ship it drives.
func TestASessionShowsItsShip(t *testing.T) {
	m, _ := loadedWith(t, 132, screensState(t))
	out := ansi.Strip(m.render())
	if !strings.Contains(rowOf(out, "flint-auth"), "#4545 ●━✕┄○┄○┄○") {
		t.Fatalf("the row lacks the ship:\n%s", out)
	}
}

// A tap selects a ship; a tap on the selected one goes to its session.
func TestATapOnAShipGoesToItsSession(t *testing.T) {
	m, _ := shipsAt(t, 44, screensState(t))
	y := lineOf(t, m, "#4552")
	next, _ := tap(m, 5, y)
	if sh, _ := next.(Model).selectedShip(); sh.Number != 4552 || next.(Model).tab != tabShips {
		t.Fatalf("the first tap: ship %d tab %d", sh.Number, next.(Model).tab)
	}
	next, _ = tap(next, 5, lineOf(t, next, "#4552"))
	if s, ok := next.(Model).selectedSession(); next.(Model).tab != tabSessions || !ok || s.Name != "ember-billing" {
		t.Fatalf("the second tap: tab %d session %q", next.(Model).tab, s.Name)
	}
}

// withShips is st with more ships for the session named name, after the
// ones its report holds.
func withShips(st proto.State, name string, more ...proto.Ship) proto.State {
	pane := ""
	for _, s := range st.Sessions {
		if s.Name == name {
			pane = s.Pane
		}
	}
	r := *st.Ships
	r.Ships = slices.Clone(r.Ships)
	for _, sh := range more {
		sh.Repo, sh.Session = "hearth/"+name, proto.ShipSession{Pane: pane, Name: name}
		sh.URL, sh.Since = "https://github.com/"+sh.Repo+"/pull/"+strconv.Itoa(sh.Number), demoAt.Add(-time.Hour)
		r.Ships = append(r.Ships, sh)
		if sh.Kind != shipDone {
			r.InFlight++
		}
	}
	st.Ships = &r
	return st
}

// twoShipsState is the demo machine where spark-docs drives a second ship
// in flight beside the one that went live, and hearth-api one that merged
// beside the one at CI.
func twoShipsState(t *testing.T) proto.State {
	t.Helper()
	st := withShips(screensState(t), "spark-docs",
		proto.Ship{Number: 4547, Title: "Search in the docs", Kind: shipRunning, Note: "CI · 2 of 5 jobs", Stage: 1})
	return withShips(st, "hearth-api",
		proto.Ship{Number: 219, Title: "Fix the flaky CI job", Kind: shipRunning, Note: "merged · deploying · 2h", Stage: 3})
}

// A session that drives two ships leads its row with the one in flight
// and counts the other by its stage.
func TestARowCountsTheOtherShipsOfItsSession(t *testing.T) {
	m, _ := loadedWith(t, 132, twoShipsState(t))
	out := ansi.Strip(m.render())
	if row := rowOf(out, "hearth-api"); !strings.Contains(row, "#4551 ●━◉┄○┄○┄○ +1 merged") {
		t.Fatalf("hearth-api's row: %q\n%s", row, out)
	}
	if row := rowOf(out, "spark-docs"); !strings.Contains(row, "#4547 ●━◉┄○┄○┄○ +1 live") {
		t.Fatalf("spark-docs's row: %q\n%s", row, out)
	}
}

// A stuck ship leads its session's row over one in flight, wherever the
// report puts it.
func TestAStuckShipLeadsTheRow(t *testing.T) {
	st := withShips(screensState(t), "hearth-api",
		proto.Ship{Number: 4561, Title: "Retry the webhook", Kind: shipStuck, Note: "CI failed · lint", Stage: 1})
	m, _ := loadedWith(t, 132, st)
	out := ansi.Strip(m.render())
	if row := rowOf(out, "hearth-api"); !strings.Contains(row, "#4561 ●━✕┄○┄○┄○ +1 ci") {
		t.Fatalf("hearth-api's row: %q\n%s", row, out)
	}
}

// When the row is narrow the count folds away first: with a long number
// read the narrowest list that has the track has no room for a count, so
// the track and the number stay and the count goes; as the list widens the
// count comes back, short and then with its stage.
func TestTheCountFoldsBeforeTheTrack(t *testing.T) {
	long := func(st proto.State) proto.State {
		return withShips(st, "kindle-web", proto.Ship{Number: 1234567, Title: "Dark mode", Kind: shipRunning, Note: "open", Stage: 0})
	}
	row := func(st proto.State, w int) string {
		m, _ := loadedWith(t, 132, st)
		for _, s := range m.snap.Sessions {
			if s.Name == "hearth-api" {
				return ansi.Strip(m.sessionRow(s, false, w)[0])
			}
		}
		t.Fatal("no hearth-api")
		return ""
	}
	has := func(r string) bool {
		return strings.Contains(r, "#4551") && strings.Contains(r, "●━◉┄○┄○┄○")
	}
	narrowest := 0
	for w := 70; w <= 120 && narrowest == 0; w++ {
		if has(row(long(screensState(t)), w)) {
			narrowest = w
		}
	}
	if narrowest == 0 {
		t.Fatal("no list between 70 and 120 wide shows the track")
	}
	two := long(twoShipsState(t))
	if r := row(two, narrowest); !has(r) || strings.Contains(r, "+1") {
		t.Fatalf("%d wide: %q", narrowest, r)
	}
	short := false
	for w := narrowest; ; w++ {
		r := row(two, w)
		if !has(r) {
			t.Fatalf("%d wide lost the track: %q", w, r)
		}
		if strings.Contains(r, "+1 merged") {
			break
		}
		short = short || strings.Contains(r, "+1")
		if w > narrowest+30 {
			t.Fatalf("the count never shows: %q", r)
		}
	}
	if !short {
		t.Fatal("the count never showed short")
	}
}

// The detail and a phone's opened row list every ship of the session in
// the row's order: stuck, waits for you, in flight, merged, live.
func TestTheDetailListsEveryShipOfTheSession(t *testing.T) {
	st := withShips(screensState(t), "spark-docs",
		proto.Ship{Number: 4549, Title: "Changelog", Kind: shipRunning, Note: "merged · deploying", Stage: 3},
		proto.Ship{Number: 4548, Title: "Search", Kind: shipRunning, Note: "CI · 2 of 5 jobs", Stage: 1},
		proto.Ship{Number: 4547, Title: "Typos", Kind: shipWaits, Note: "green · waits for your merge", Stage: 2},
		proto.Ship{Number: 4546, Title: "Links", Kind: shipStuck, Note: "conflicts with main", Stage: 1})
	want := []string{"#4546 ●━✕┄○┄○┄○", "#4547 ●━●━◉┄○┄○", "#4548 ●━◉┄○┄○┄○", "#4549 ●━●━●━◉┄○", "#4540 ●━●━●━●━●"}
	for _, w := range []int{44, 132} {
		f := newFake()
		m := New(Options{NoMotion: true, Conn: f, Now: func() time.Time { return demoAt }})
		next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 60})
		next, _ = next.Update(stateMsg(st))
		out := ansi.Strip(next.(Model).render())
		at := -1
		for _, s := range want {
			i := strings.Index(out, s)
			if i < 0 || i < at {
				t.Fatalf("width %d: %q is missing or out of order:\n%s", w, s, out)
			}
			at = i
		}
	}
}
