package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/history"
	"github.com/blox-eng/matchblox/internal/proto"
)

// TestRecommendationsSitUnderTheSessions: what to do next sits under the
// sessions, on the Sessions tab (owner 2026-10-12); ↓ walks from the
// sessions into them.
func TestRecommendationsSitUnderTheSessions(t *testing.T) {
	st := queueState()
	st.Recommendations = []advice.Rec{{ID: "r1", Level: "warn", Title: "Compact app-feature", Evidence: "context 91%",
		Primary: &advice.Action{Label: "jump", Nav: true, Steps: [][]string{{"tmux", "switch-client", "-t", "%1"}}}}}
	m, _ := loadedWith(t, 100, st)
	out := ansi.Strip(m.render())
	tabs := strings.Split(out, "\n")[1]
	if strings.Contains(tabs, "queue") || strings.Contains(tabs, "procs") || !strings.HasPrefix(tabs, " SESSIONS") {
		t.Fatalf("the tabs: %q", tabs)
	}
	recs, quiet := strings.Index(out, "RECOMMENDATIONS · 1"), strings.Index(out, "QUIET")
	if quiet < 0 || recs < 0 || recs < quiet || !strings.Contains(out, "Compact app-feature") {
		t.Fatalf("the recommendations are not under the sessions:\n%s", out)
	}
	var next tea.Model = m
	for range len(m.snap.Sessions) + len(m.ghosts()) {
		next, _ = key(next, "down")
	}
	got := next.(Model)
	if r, ok := got.selectedRec(); !ok || r.ID != "r1" {
		t.Fatal("↓ from the sessions and the ghost row does not select the recommendation")
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
// under its opened row on a phone and on its desk row.
func TestProgressShowsWhereThePersonLooks(t *testing.T) {
	m, _ := loadedWith(t, 50, queueState())
	if out := screen(m); !strings.Contains(out, "▰▰▰▰▱ plan 75%") {
		t.Fatalf("the opened row has no plan:\n%s", out)
	}
	m, _ = loadedWith(t, 200, queueState())
	// On a desk row an action that is due (! compact) holds the plan
	// column; the bar shows there when nothing is due.
	if l := rowOf(m.render(), "app-feature"); !strings.Contains(l, "! compact") || strings.Contains(listOf(l, 200), "75%") {
		t.Fatalf("the due action did not win the plan column: %q", l)
	}
	st := queueState()
	for i := range st.Sessions {
		st.Sessions[i].Do = ""
	}
	idle, _ := loadedWith(t, 200, st)
	if l := rowOf(idle.render(), "app-feature"); !strings.Contains(l, "▰▰▰▰▱ 75%") {
		t.Fatalf("with nothing due the row has no plan: %q", l)
	}
}

func twoRecs() proto.State {
	st := queueState()
	jump := &advice.Action{Label: "jump", Nav: true, Steps: [][]string{{"tmux", "switch-client", "-t", "%1"}}}
	st.Recommendations = []advice.Rec{
		{ID: "r1", Level: "warn", Title: "Compact app-feature", Evidence: "context 91%", Primary: jump},
		{ID: "r2", Level: "info", Title: "Clear tools", Evidence: "idle 21d", Primary: jump},
	}
	return st
}

func loadedIn(t *testing.T, dir string, st proto.State) Model {
	t.Helper()
	m := New(Options{NoMotion: true, Conn: newFake(), StateDir: dir})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	next, _ = next.Update(stateMsg(st))
	return next.(Model)
}

// TestDismissARecommendation: d hides the selected one, a tap on its
// dismiss mark too; it stays hidden in the next console until its
// evidence changes.
func TestDismissARecommendation(t *testing.T) {
	dir := t.TempDir()
	m := loadedIn(t, dir, twoRecs())
	next, _ := key(selectRec(t, m, "r1"), "d")
	if out := screen(next); strings.Contains(out, "▲ Compact app-feature") || !strings.Contains(out, "RECOMMENDATIONS · 1") {
		t.Fatalf("d did not dismiss it:\n%s", out)
	}
	if r, ok := next.(Model).selectedRec(); !ok || r.ID != "r2" {
		t.Fatal("the next recommendation does not take the selection")
	}
	again := loadedIn(t, dir, twoRecs())
	if strings.Contains(screen(again), "▲ Compact app-feature") {
		t.Fatal("a dismissed recommendation came back in the next console")
	}
	changed := twoRecs()
	changed.Recommendations[0].Evidence = "context 96%"
	if !strings.Contains(screen(loadedIn(t, dir, changed)), "▲ Compact app-feature") {
		t.Fatal("new evidence does not bring it back")
	}

	// A tap on the dismiss mark of the selected one.
	sel := selectRec(t, loadedIn(t, t.TempDir(), twoRecs()), "r2")
	y := lineOf(t, sel, "d dismiss")
	line := strings.Split(screen(sel), "\n")[y]
	x := ansi.StringWidth(line[:strings.Index(line, "d dismiss")])
	tapped, _ := tap(sel, x, y)
	if strings.Contains(screen(tapped), "○ Clear tools") {
		t.Fatalf("a tap on dismiss did not dismiss:\n%s", screen(tapped))
	}
}

// TestFoldTheRecommendations: h folds the section to one line, its rows
// leave the selection, h unfolds it, a tap on the label does the same,
// and the next console remembers.
func TestFoldTheRecommendations(t *testing.T) {
	dir := t.TempDir()
	m := loadedIn(t, dir, twoRecs())
	sel := selectRec(t, m, "r2")
	folded, _ := key(sel, "h")
	out := screen(folded)
	if !strings.Contains(out, "RECOMMENDATIONS · 2 hidden · h shows") || strings.Contains(out, "▲ Compact app-feature") {
		t.Fatalf("h did not fold:\n%s", out)
	}
	if _, ok := folded.(Model).selectedRec(); ok || folded.(Model).rowCount() != len(m.snap.Sessions)+len(m.ghosts()) {
		t.Fatal("a folded recommendation can still be selected")
	}
	if !strings.Contains(screen(loadedIn(t, dir, twoRecs())), "2 hidden") {
		t.Fatal("the fold is not remembered")
	}
	open, _ := tap(folded, 3, lineOf(t, folded, "RECOMMENDATIONS"))
	if !strings.Contains(screen(open), "▲ Compact app-feature") {
		t.Fatalf("a tap on the label did not unfold:\n%s", screen(open))
	}
	if again, _ := key(open, "h"); !strings.Contains(screen(again), "2 hidden") {
		t.Fatal("h anywhere in Sessions does not fold")
	}
}
