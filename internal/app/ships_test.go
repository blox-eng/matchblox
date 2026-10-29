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
	"github.com/blox-eng/matchblox/internal/sample"
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
	m, _ := loadedWith(t, 200, screensState(t))
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
	m, _ := loadedWith(t, 200, twoShipsState(t))
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
	m, _ := loadedWith(t, 200, st)
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

// A phone's opened row lists the session's ships in the row's order
// (stuck, waits for you, in flight, merged, live): the first three, then
// how many more. A desk shows the chat beside the list, and the Ships tab.
func TestTheOpenedRowListsEveryShipOfTheSession(t *testing.T) {
	st := withShips(screensState(t), "spark-docs",
		proto.Ship{Number: 4549, Title: "Changelog", Kind: shipRunning, Note: "merged · deploying", Stage: 3},
		proto.Ship{Number: 4548, Title: "Search", Kind: shipRunning, Note: "CI · 2 of 5 jobs", Stage: 1},
		proto.Ship{Number: 4547, Title: "Typos", Kind: shipWaits, Note: "green · waits for your merge", Stage: 2},
		proto.Ship{Number: 4546, Title: "Links", Kind: shipStuck, Note: "conflicts with main", Stage: 1})
	want := []string{"#4546 ●━✕┄○┄○┄○", "#4547 ●━●━◉┄○┄○", "#4548 ●━◉┄○┄○┄○", "+2 more · p ships"}
	const w = 44
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
	nm := next.(Model)
	for _, s := range nm.snap.Sessions {
		if s.Name != "spark-docs" {
			continue
		}
		var got []int
		for _, sh := range nm.shipsOf(s) {
			got = append(got, sh.Number)
		}
		if !slices.Equal(got, []int{4546, 4547, 4548, 4549, 4540}) {
			t.Fatalf("spark-docs's ships in order: %v", got)
		}
	}
}

// A ship whose stage the console does not know is counted, not a crash.
func TestACountWithAnUnknownStageDraws(t *testing.T) {
	st := withShips(screensState(t), "hearth-api", proto.Ship{Number: 219, Title: "Future", Kind: shipRunning, Stage: 7})
	m, _ := loadedWith(t, 200, st)
	if row := rowOf(ansi.Strip(m.render()), "hearth-api"); !strings.Contains(row, "#4551 ●━◉┄○┄○┄○ +1") {
		t.Fatalf("hearth-api's row: %q", row)
	}
}

// The count names a need before a stage: another ship that waits for you
// says so.
func TestTheCountNamesWhatTheOthersNeed(t *testing.T) {
	st := withShips(screensState(t), "hearth-api",
		proto.Ship{Number: 4561, Title: "Retry the webhook", Kind: shipStuck, Note: "CI failed · lint", Stage: 1},
		proto.Ship{Number: 4562, Title: "Docs", Kind: shipWaits, Note: "green · waits for your merge", Stage: 2})
	m, _ := loadedWith(t, 200, st)
	out := ansi.Strip(m.render())
	if row := rowOf(out, "hearth-api"); !strings.Contains(row, "#4561 ●━✕┄○┄○┄○ +2 waits") {
		t.Fatalf("hearth-api's row: %q\n%s", row, out)
	}
}

// A ship an earlier agent in the same pane claimed is not this session's:
// their session ids tell them apart, on the row and for Enter on the ship.
func TestAShipOfAnEarlierAgentInThePaneIsNotTheSessions(t *testing.T) {
	st := screensState(t)
	var docs sample.Session
	for _, s := range st.Sessions {
		if s.Name == "spark-docs" {
			docs = s
		}
	}
	if docs.SessionID == "" {
		t.Fatal("spark-docs has no session id")
	}
	old := proto.Ship{Repo: "hearth/spark-docs", Number: 4400, Title: "Old", Kind: shipDone, Note: "live", Stage: 4,
		Session: proto.ShipSession{Pane: docs.Pane, ID: "an-earlier-agent"}}
	r := *st.Ships
	r.Ships = append(slices.Clone(r.Ships), old)
	st.Ships = &r
	m, _ := loadedWith(t, 132, st)
	if row := rowOf(ansi.Strip(m.render()), "spark-docs"); strings.Contains(row, "+1") {
		t.Fatalf("spark-docs counts the earlier agent's ship: %q", row)
	}
	if s, ok := m.driver(old); ok {
		t.Fatalf("the earlier agent's ship is driven by %q", s.Name)
	}
}

// drawnAt is the console on st at w×h, drawn: its lines without styles.
func drawnAt(t *testing.T, st proto.State, w, h int) (drawn, []string) {
	t.Helper()
	m := New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	next, _ = next.Update(stateMsg(st))
	nm := next.(Model)
	nm.splashDone = true
	sc := nm.draw(w)
	lines := make([]string, len(sc.lines))
	for i, l := range sc.lines {
		lines[i] = ansi.Strip(l)
	}
	return sc, lines
}

// Many ships: the opened row on a short phone lists three and how many
// more, and the session's keys stay where a tap finds them, on the line
// that shows them.
func TestManyShipsKeepTheSessionsKeys(t *testing.T) {
	var more []proto.Ship
	for i := range 5 {
		more = append(more, proto.Ship{Number: 4600 + i, Title: "Page " + strconv.Itoa(i), Kind: shipRunning, Note: "CI", Stage: 1})
	}
	st := withShips(screensState(t), "spark-docs", more...)
	size := [2]int{44, 16}
	sc, lines := drawnAt(t, st, size[0], size[1])
	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "+3 more") || strings.Contains(out, "#4603") {
		t.Fatalf("%v: not three ships and +3 more:\n%s", size, out)
	}
	if sc.actKeysAt <= 0 || !strings.Contains(lines[sc.actKeysAt], "⏎ chat") {
		t.Fatalf("%v: the keys are marked on line %d:\n%s", size, sc.actKeysAt, out)
	}
}

// Ships of one rank keep the report's order: two in flight list as the
// report has them.
func TestShipsOfARankKeepTheReportsOrder(t *testing.T) {
	st := withShips(screensState(t), "spark-docs",
		proto.Ship{Number: 4550, Title: "First", Kind: shipRunning, Note: "CI", Stage: 1},
		proto.Ship{Number: 4549, Title: "Second", Kind: shipRunning, Note: "green", Stage: 2})
	m, _ := loadedWith(t, 44, st)
	out := ansi.Strip(m.render())
	if a, b := strings.Index(out, "#4550 "), strings.Index(out, "#4549 "); a < 0 || b < 0 || a > b {
		t.Fatalf("#4550 at %d, #4549 at %d:\n%s", a, b, out)
	}
}

// TestEnterOnAShipNobodyOwnsStartsASessionThere: #80, a ship whose session
// is gone is no dead end. Enter opens New session in the checkout the ship
// names, its first message about the PR, and sends nothing on its own.
func TestEnterOnAShipNobodyOwnsStartsASessionThere(t *testing.T) {
	st := screensState(t)
	r := *st.Ships
	r.Ships = slices.Clone(r.Ships)
	r.Ships[0].Session, r.Ships[0].Where = proto.ShipSession{}, "/w/hearth-api"
	st.Ships = &r
	st.Agents = []string{"claude"}
	m, f := shipsAt(t, 86, st)
	if out := ansi.Strip(m.render()); !strings.Contains(out, "by no session") {
		t.Fatalf("the ship does not say no session owns it:\n%s", out)
	}
	next, _ := key(m, "enter")
	nm := next.(Model)
	n := strconv.Itoa(r.Ships[0].Number)
	switch {
	case nm.form == nil:
		t.Fatalf("enter did not open New session: %q", nm.flash)
	case nm.whereSelectedIn(nm.form).path() != "/w/hearth-api":
		t.Fatalf("New session opens in %q", nm.whereSelectedIn(nm.form).path())
	case nm.form.at != fieldAgent || !nm.form.picked:
		t.Fatalf("the folder is not picked with the cursor on agent, as a ghost row does: at %v", nm.form.at)
	case !strings.Contains(nm.form.first, "#"+n):
		t.Fatalf("the first message %q does not name #%s", nm.form.first, n)
	case len(f.acts()) != 0:
		t.Fatalf("enter sent %+v", f.acts())
	}

	// No checkout of it here: the flash says so.
	r.Ships[0].Where = ""
	m, _ = shipsAt(t, 86, st)
	next, _ = key(m, "enter")
	if nm := next.(Model); nm.form != nil || !strings.Contains(nm.flash, "no checkout of") {
		t.Fatalf("form %v, flash %q", nm.form != nil, nm.flash)
	}
}
