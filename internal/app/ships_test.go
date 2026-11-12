package app

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// A session's row and detail show the ship it drives.
func TestASessionShowsItsShip(t *testing.T) {
	m, _ := loadedWith(t, 200, screensState(t))
	out := ansi.Strip(m.render())
	if !strings.Contains(rowOf(out, "flint-auth"), "#4545 ●━✕┄○┄○┄○") {
		t.Fatalf("the row lacks the ship:\n%s", out)
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
