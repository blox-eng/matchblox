package app

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/history"
)

// TestRecommendationsAreASectionOfTheQueue: what to do next sits under who
// waits, on one tab; ↓ walks from the queue into the recommendations.
func TestRecommendationsAreASectionOfTheQueue(t *testing.T) {
	st := queueState()
	st.Recommendations = []advice.Rec{{ID: "r1", Level: "warn", Title: "Compact app-feature", Evidence: "context 91%",
		Primary: &advice.Action{Label: "jump", Nav: true, Steps: [][]string{{"tmux", "switch-client", "-t", "%1"}}}}}
	m, _ := loadedWith(t, 100, st)
	out := ansi.Strip(m.render())
	if strings.Contains(out, "RECS") || !strings.Contains(out, "7 PANES") {
		t.Fatalf("the tabs still have recs, or panes is not 7:\n%s", out)
	}
	wait, recs := strings.Index(out, "WAITING FOR YOU"), strings.Index(out, "1 RECOMMENDATIONS")
	if wait < 0 || recs < wait || !strings.Contains(out, "Compact app-feature") {
		t.Fatalf("the recommendations are not under the queue:\n%s", out)
	}
	next, _ := key(m, "down")
	next, _ = key(next, "down")
	got := next.(Model)
	if r, ok := got.selectedRec(); !ok || r.ID != "r1" {
		t.Fatal("↓ past the queue does not select the recommendation")
	}
	if _, ok := got.selectedQueue(); ok {
		t.Fatal("a queue item is still selected")
	}
	if !strings.Contains(ansi.Strip(got.render()), "context 91%") {
		t.Fatal("the selected recommendation does not show its evidence")
	}
	next, _ = key(next, "enter")
	if p := next.(Model).pending; p == nil || p.String() != "tmux switch-client -t %1" {
		t.Fatalf("enter on the recommendation: %+v", p)
	}
	next, _ = key(next, "esc")
	if next, _ = key(next, "a"); next.(Model).input != nil {
		t.Fatal("a on a recommendation started an answer")
	}
}

// TestHistoryWithAFlatSeries: a value that never changes (swap at 0 GB)
// draws a flat chart; a zero range must not crash the console.
func TestHistoryWithAFlatSeries(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	at := time.Unix(1_800_000_000, 0)
	for i := range 10 {
		m.hist.Points = append(m.hist.Points, history.Point{At: at.Add(time.Duration(i) * 30 * time.Second), CPU: float64(i)})
	}
	m.tab = tabHistory
	if out := ansi.Strip(m.render()); !strings.Contains(out, "SWAP") {
		t.Fatalf("no swap chart:\n%s", out)
	}
}

// TestProgressShowsWhereThePersonLooks: the bar an agent reported shows on
// its row in the sessions and the queue, and its step in the detail.
func TestProgressShowsWhereThePersonLooks(t *testing.T) {
	for _, w := range []int{50, 100} {
		m, _ := loadedWith(t, w, queueState())
		if out := screen(m); !strings.Contains(out, "▰▰▰▰▱ 75%") {
			t.Fatalf("width %d: the queue row has no progress:\n%s", w, out)
		}
		next, _ := key(m, "2")
		out := screen(next)
		// On a wide row an action that is due (! compact) holds the DO
		// column; the bar shows there when nothing is due.
		if w >= 60 && (!strings.Contains(out, "! compact") || strings.Contains(out, "▰▰▰▰▱ 75%")) {
			t.Fatalf("width %d: the due action did not win the DO column:\n%s", w, out)
		}
		if w < 60 && !strings.Contains(out, "▰▰▰▰▱ 75%") {
			t.Fatalf("width %d: the session row has no progress:\n%s", w, out)
		}
		if w >= 60 {
			for s, ok := next.(Model).selected(); ok && s.Pane != "%1"; s, ok = next.(Model).selected() {
				next, _ = key(next, "down")
			}
			if !strings.Contains(screen(next), "PROGRESS ▰▰▰▰▰▰▰▰▱▱ 75%  wiring the console") {
				t.Fatalf("the detail has no progress:\n%s", screen(next))
			}
			st := queueState()
			for i := range st.Sessions {
				st.Sessions[i].Do = ""
			}
			idle, _ := loadedWith(t, w, st)
			again, _ := key(idle, "2")
			if !strings.Contains(screen(again), "▰▰▰▰▱ 75%") {
				t.Fatalf("width %d: with nothing due the row has no progress:\n%s", w, screen(again))
			}
		}
	}
}
