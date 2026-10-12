package app

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/history"
)

// TestRecommendationsSitAboveTheSessions: what to do next sits above who
// waits, on the Sessions tab; ↑ walks from the sessions into them.
func TestRecommendationsSitAboveTheSessions(t *testing.T) {
	st := queueState()
	st.Recommendations = []advice.Rec{{ID: "r1", Level: "warn", Title: "Compact app-feature", Evidence: "context 91%",
		Primary: &advice.Action{Label: "jump", Nav: true, Steps: [][]string{{"tmux", "switch-client", "-t", "%1"}}}}}
	m, _ := loadedWith(t, 100, st)
	out := ansi.Strip(m.render())
	tabs := strings.Split(out, "\n")[1]
	if strings.Contains(tabs, "queue") || strings.Contains(tabs, "procs") || !strings.HasPrefix(tabs, " SESSIONS") {
		t.Fatalf("the tabs: %q", tabs)
	}
	recs, wait := strings.Index(out, "RECOMMENDATIONS · 1"), strings.Index(out, "WAITS FOR YOU")
	if wait < 0 || recs < 0 || recs > wait || !strings.Contains(out, "Compact app-feature") {
		t.Fatalf("the recommendations are not above the sessions:\n%s", out)
	}
	next, _ := key(m, "up")
	got := next.(Model)
	if r, ok := got.selectedRec(); !ok || r.ID != "r1" {
		t.Fatal("↑ from the sessions does not select the recommendation")
	}
	if _, ok := got.selectedSession(); ok {
		t.Fatal("a session is still selected")
	}
	if !strings.Contains(ansi.Strip(got.render()), "context 91%") {
		t.Fatal("the selected recommendation does not show its evidence")
	}
	next, _ = key(next, "enter")
	if p := next.(Model).pending; p == nil || p.String() != "tmux switch-client -t %1" {
		t.Fatalf("enter on the recommendation: %+v", p)
	}
	next, _ = key(next, "esc")
	if next, _ = key(next, "a"); next.(Model).peek != nil {
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

// TestProgressShowsWhereThePersonLooks: the bar an agent reported shows
// under its opened row on a phone, on its desk row, and with its step in
// the detail.
func TestProgressShowsWhereThePersonLooks(t *testing.T) {
	m, _ := loadedWith(t, 50, queueState())
	if out := screen(m); !strings.Contains(out, "▰▰▰▰▱ plan 75%") {
		t.Fatalf("the opened row has no plan:\n%s", out)
	}
	m, _ = loadedWith(t, 132, queueState())
	out := screen(m)
	// On a desk row an action that is due (! compact) holds the plan
	// column; the bar shows there when nothing is due.
	if l := rowOf(m.render(), "app-feature"); !strings.Contains(l, "! compact") || strings.Contains(listOf(l, 132), "75%") {
		t.Fatalf("the due action did not win the plan column: %q", l)
	}
	if !strings.Contains(out, "plan     ▰▰▰▰▰▰▰▰▱▱ 75%  wiring the console") {
		t.Fatalf("the detail has no plan:\n%s", out)
	}
	st := queueState()
	for i := range st.Sessions {
		st.Sessions[i].Do = ""
	}
	idle, _ := loadedWith(t, 132, st)
	if l := rowOf(idle.render(), "app-feature"); !strings.Contains(l, "▰▰▰▰▱ 75%") {
		t.Fatalf("with nothing due the row has no plan: %q", l)
	}
}
