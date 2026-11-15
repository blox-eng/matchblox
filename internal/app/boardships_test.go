package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
)

// TestARowLeadsWithTheShipThatMostNeedsYou: a session's ships lead with
// the one that most needs you (stuck, then waits, then in flight), the
// badge's; ships of one rank keep the report's order, and a ship of an
// earlier agent in the same pane is not the session's.
func TestARowLeadsWithTheShipThatMostNeedsYou(t *testing.T) {
	lead := func(m Model, name string) (int, int) {
		ss := m.shipsOf(matchOf(t, m, name))
		if len(ss) == 0 {
			return 0, 0
		}
		return ss[0].Number, len(ss)
	}
	two := boardShot(t, twoShipsState(t), 200, 40, nil)
	if n, of := lead(two, "hearth-api"); n != 4551 || of != 2 {
		t.Errorf("hearth-api leads with #%d of %d", n, of)
	}
	for _, c := range []struct {
		name string
		more []proto.Ship
		want int
	}{
		{"stuck leads", []proto.Ship{{Number: 4561, Title: "Retry the webhook", Kind: shipStuck, Note: "CI failed · lint", Stage: 1}}, 4561},
		{"an unknown stage", []proto.Ship{{Number: 219, Title: "Future", Kind: shipRunning, Stage: 7}}, 4551},
		{"the need first", []proto.Ship{{Number: 4561, Title: "Retry the webhook", Kind: shipStuck, Note: "CI failed · lint", Stage: 1},
			{Number: 4562, Title: "Docs", Kind: shipWaits, Note: "green · waits for your merge", Stage: 2}}, 4561},
	} {
		m := boardShot(t, withShips(screensState(t), "hearth-api", c.more...), 200, 40, nil)
		if n, _ := lead(m, "hearth-api"); n != c.want {
			t.Errorf("%s: hearth-api leads with #%d, want #%d", c.name, n, c.want)
		}
	}

	m := boardShot(t, withShips(screensState(t), "spark-docs",
		proto.Ship{Number: 4550, Title: "First", Kind: shipRunning, Note: "CI", Stage: 1},
		proto.Ship{Number: 4549, Title: "Second", Kind: shipRunning, Note: "green", Stage: 2}), 200, 40, nil)
	if ss := m.shipsOf(matchOf(t, m, "spark-docs")); len(ss) < 2 || ss[0].Number != 4550 {
		t.Errorf("spark-docs's ships of one rank: %+v, want #4550 first, as the report", ss)
	}

	st := screensState(t)
	docs := matchOf(t, boardShot(t, st, 120, 40, nil), "spark-docs")
	_, before := lead(boardShot(t, st, 200, 40, nil), "spark-docs")
	r := *st.Ships
	r.Ships = append(slices.Clone(r.Ships), proto.Ship{Repo: "hearth/spark-docs", Number: 4400, Title: "Old", Kind: shipDone, Note: "live", Stage: 4,
		Session: proto.ShipSession{Pane: docs.Pane, ID: "an-earlier-agent"}})
	st.Ships = &r
	if _, after := lead(boardShot(t, st, 200, 40, nil), "spark-docs"); after != before {
		t.Errorf("spark-docs counts the earlier agent's ship: %d ships, was %d", after, before)
	}
}

// demoBoard is the console on st at w by 40, on the demo's clock.
func demoBoard(t *testing.T, w int, st proto.State) Model {
	t.Helper()
	var next tea.Model = New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
	next, _ = next.Update(tea.WindowSizeMsg{Width: w, Height: 40})
	next, _ = next.Update(stateMsg(st))
	return consoleOn(next.(Model))
}

// TestTheNoticeSaysWhyShipsAreBehind: the notice row says plainly why the
// ships may be behind: GitHub's rate limit, gh that cannot read, and a
// repository whose poll failed.
func TestTheNoticeSaysWhyShipsAreBehind(t *testing.T) {
	st := screensState(t)
	r := *st.Ships
	r.Paused, r.Unknown, r.Errors = time.Date(2026, 10, 1, 23, 33, 0, 0, time.Local), "gh not signed in", []string{"hearth/hearth-api: HTTP 502"}
	st.Ships = &r
	for _, w := range []int{80, 132} { // a notice keeps three lines: at 44 the last reason goes
		m := demoBoard(t, w, st)
		out := strings.Join(strings.Fields(plainFrame(m)), " ")
		for _, want := range []string{"ships read again at 23:33", "GitHub cannot be read: gh not signed in", "not read: hearth/hearth-api: HTTP 502"} {
			if !strings.Contains(out, want) {
				t.Errorf("width %d: the notice row does not say %q:\n%s", w, want, plainFrame(m))
			}
		}
	}
	if out := plainFrame(demoBoard(t, 132, screensState(t))); strings.Contains(out, "read again") || strings.Contains(out, "cannot be read") {
		t.Errorf("the notice row says the ships are behind when they are not:\n%s", out)
	}
}
