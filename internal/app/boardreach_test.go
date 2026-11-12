package app

import (
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
)

// reachBoard is the board fixture with a setup door, a ghost row on a
// folder and two recommendations, on its first card.
func reachBoard(t *testing.T) (Model, *fakeConn) {
	t.Helper()
	st := boardFixture(t)
	st.Agents = []string{"claude"}
	jump := &advice.Action{Label: "jump", Nav: true, Steps: [][]string{{"tmux", "switch-client", "-t", "%1"}}}
	st.Recommendations = []advice.Rec{
		{ID: "r1", Level: "warn", Title: "Compact flint-auth", Evidence: "context 91%",
			Primary: &advice.Action{Label: "jump", Nav: true, Steps: [][]string{{"rm", "-rf", "/tmp/x"}}}},
		{ID: "r2", Level: "info", Title: "Clear tinder-scratch", Evidence: "idle 2 days", Primary: jump,
			Second: &advice.Action{Label: "kill", Destructive: true, Steps: [][]string{{"kill", "42"}}}},
	}
	m := boardShot(t, st, 120, 40, func(m *Model) {
		m.doors = []doors.Door{{ID: doors.Tmux, Title: "Install tmux", Why: "tmux keeps your sessions when the terminal closes.",
			Term: []string{"sh", "-c", "true"}}}
		m.wghost = []whereRepo{{path: "/home/user/src/hearth", name: "hearth", branch: "main"}}
		m.opt.Run = func(argv []string) error { t.Fatalf("ran %v in the console", argv); return nil }
	})
	return firstCard(m), boardFake(m)
}

// reachOf says what the board has chosen: a card, the doors line, the
// ghost row or a recommendation.
func reachOf(m Model) string {
	if line, ok := m.aboveChosen(); ok {
		return "above " + line
	}
	if line, ok := m.belowChosen(); ok {
		if line == "" {
			return "ghost"
		}
		return "rec " + line
	}
	if s, ok := m.boardSelected(); ok {
		return "card " + s.Name
	}
	return "nothing"
}

// TestTheBoardReachesDoorsGhostAndRecommendations (R3.5): ↑ from the
// first card of a column chooses the doors line, which opens in place and
// takes ⏎ and x as its row in Sessions did; ↓ from the last card chooses
// the ghost row, whose ⏎ opens New session on its folder, and ↓ again the
// recommendations, which open in place and take ⏎, x, d and h as their
// rows did. ↑ and ↓ come back to the cards.
func TestTheBoardReachesDoorsGhostAndRecommendations(t *testing.T) {
	m, _ := reachBoard(t)
	first := reachOf(m)

	// The doors line.
	if strings.Contains(plainFrame(m), "tmux keeps your sessions") {
		t.Fatal("the doors line is open while a card is chosen")
	}
	if m = pressOn(m, "up"); reachOf(m) != "above " {
		t.Fatalf("↑ from the first card chose %s, want the doors line", reachOf(m))
	}
	if !strings.Contains(plainFrame(m), "tmux keeps your sessions") {
		t.Fatalf("the chosen doors line does not open in place:\n%s", plainFrame(m))
	}
	next, _ := key(m, "enter")
	if p := next.(Model).pending; p == nil || p.door != doors.Tmux || p.which == "secondary" {
		t.Errorf("⏎ on the doors line: %+v, want the door's step", p)
	}
	next, _ = key(m, "x")
	if p := next.(Model).pending; p == nil || p.door != doors.Tmux || p.which != "secondary" || !p.destructive {
		t.Errorf("x on the doors line: %+v, want the door's close behind a y", p)
	}
	if m = pressOn(m, "down"); reachOf(m) != first {
		t.Fatalf("↓ from the doors line chose %s, want %s", reachOf(m), first)
	}

	// The ghost row, from the last card of the open column.
	b, _ := m.boardDrawOf()
	for range len(b.idx[b.open]) - 1 {
		m = pressOn(m, "down")
	}
	last := reachOf(m)
	if want := "card " + b.idx[b.open][len(b.idx[b.open])-1].Name; last != want {
		t.Fatalf("↓ down the column chose %s, want %s", last, want)
	}
	if m = pressOn(m, "down"); reachOf(m) != "ghost" {
		t.Fatalf("↓ from the last card chose %s, want the ghost row", reachOf(m))
	}
	if _, ok := m.boardSelected(); ok {
		t.Error("a card is chosen with the ghost row")
	}
	if l := lineWith(strings.Split(plainFrame(m), "\n"), "start a new session here"); !strings.HasPrefix(l, "▌") {
		t.Errorf("the chosen ghost row is not drawn chosen: %q", l)
	}
	next, _ = key(m, "enter")
	if f := next.(Model).form; f == nil || f.where != "/home/user/src/hearth" {
		t.Errorf("⏎ on the ghost row opened %+v (%q), want New session on its folder", f, next.(Model).flash)
	}
	if up := pressOn(m, "up"); reachOf(up) != last {
		t.Errorf("↑ from the ghost row chose %s, want %s", reachOf(up), last)
	}

	// The recommendations.
	if m = pressOn(m, "down"); reachOf(m) != "rec r1" {
		t.Fatalf("↓ from the ghost row chose %s, want the first recommendation", reachOf(m))
	}
	out := plainFrame(m)
	if !strings.Contains(out, "Compact flint-auth") || !strings.Contains(out, "context 91%") {
		t.Fatalf("the chosen recommendation does not open in place:\n%s", out)
	}
	if strings.Contains(plainFrame(pressOn(m, "up")), "context 91%") {
		t.Error("the recommendation stays open once the choice leaves it")
	}
	// ⏎ shows a Nav step, ⏎ again runs only tmux moves (TestNavRunsOnlyTmuxMoves).
	nav := pressOn(m, "enter")
	if p := nav.pending; p == nil || p.String() != "rm -rf /tmp/x" {
		t.Fatalf("⏎ on the recommendation: %+v", p)
	}
	ran, cmd := key(nav, "enter")
	if nav = fed(ran, cmd).(Model); !strings.Contains(plainFrame(nav), "refused") {
		t.Errorf("a Nav step that is no tmux move is not refused:\n%s", plainFrame(nav))
	}
	if reachOf(nav) != "rec r1" {
		t.Errorf("after the act the board chose %s, want the recommendation still", reachOf(nav))
	}

	// x on the second: its destructive step is sent on a typed y, never run
	// here (TestConfirmedRecActIsSentNotRun), and the row says what came of it.
	m = pressOn(m, "down")
	if reachOf(m) != "rec r2" {
		t.Fatalf("↓ on the first recommendation chose %s", reachOf(m))
	}
	m = pressOn(m, "x")
	if p := m.pending; p == nil || !p.destructive || p.String() != "kill 42" {
		t.Fatalf("x on the recommendation: %+v", p)
	}
	f := boardFake(m)
	m = pressOn(m, "y")
	if acts := f.acts(); len(acts) == 0 || acts[len(acts)-1] != (proto.Act{RecID: "r2", Which: "secondary", Confirm: "y"}) {
		t.Fatalf("y sent %+v", acts)
	}
	m = upd(t, m, resultMsg(proto.Result{ActID: f.lastAct(t), Err: "no such process"}))
	if !strings.Contains(plainFrame(m), "✗ failed: no such process") || reachOf(m) != "rec r2" {
		t.Errorf("the recommendation does not say what came of x (%s):\n%s", reachOf(m), plainFrame(m))
	}

	// d dismisses it, and the one before takes the choice; h folds them and
	// the choice goes to the ghost row; h again shows them.
	m = pressOn(m, "d")
	if out := plainFrame(m); strings.Contains(out, "○ Clear tinder-scratch") || !strings.Contains(out, "RECOMMENDATIONS · 1") {
		t.Fatalf("d did not dismiss the recommendation:\n%s", out)
	}
	if reachOf(m) != "rec r1" {
		t.Errorf("after d the board chose %s, want the other recommendation", reachOf(m))
	}
	m = pressOn(m, "h")
	if out := plainFrame(m); !strings.Contains(out, "RECOMMENDATIONS · 1 hidden") || reachOf(m) != "ghost" {
		t.Fatalf("h did not fold the recommendations (%s):\n%s", reachOf(m), out)
	}
	if m = pressOn(m, "down"); reachOf(m) != "ghost" {
		t.Errorf("↓ reached a folded recommendation: %s", reachOf(m))
	}
	if m = pressOn(m, "h"); !strings.Contains(plainFrame(m), "RECOMMENDATIONS · 1") || strings.Contains(plainFrame(m), "hidden") {
		t.Errorf("h again did not show the recommendations:\n%s", plainFrame(m))
	}

	// Taps, as on their rows: the label folds the recommendations, the
	// chosen one's dismiss mark dismisses it, and the ghost row opens New
	// session on its folder.
	m = pressOn(m, "down")
	if tapped, _ := tap(m, m.width-3, lineOf(t, m, "Compact flint-auth")); strings.Contains(screen(tapped), "▲ Compact flint-auth") {
		t.Errorf("a tap on the dismiss mark did not dismiss:\n%s", screen(tapped))
	}
	if folded, _ := tap(m, 3, lineOf(t, m, "RECOMMENDATIONS")); !strings.Contains(screen(folded), "RECOMMENDATIONS · 1 hidden") {
		t.Errorf("a tap on the label did not fold:\n%s", screen(folded))
	}
	if opened, _ := tap(m, 3, lineOf(t, m, "start a new session here")); opened.(Model).form == nil {
		t.Error("a tap on the ghost row did not open New session")
	}
	m = pressOn(m, "up")
	if m = pressOn(m, "esc"); reachOf(m) != last {
		t.Errorf("esc on the ghost row chose %s, want %s", reachOf(m), last)
	}
}
