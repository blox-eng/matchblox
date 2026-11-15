package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// orphanShip is the console with #4570, a pull request that no session
// drives, of kind, checked out at /w/wick-mobile on its branch.
func orphanShip(t *testing.T, kind string) Model {
	t.Helper()
	st := screensState(t)
	st.Agents = []string{"claude"}
	r := *st.Ships
	sh := proto.Ship{Repo: "hearth/hearth-api", Number: 4570, Title: "Cache the session store",
		Kind: kind, Note: "CI · 3 of 9 jobs", Stage: 1, Where: "/w/wick-mobile", Since: demoAt.Add(-time.Hour),
		URL: "https://github.com/hearth/hearth-api/pull/4570"}
	if kind == shipWaits {
		sh.Note, sh.Merge = "waits for your merge", []string{"gh", "pr", "merge", "4570", "--squash"}
	}
	r.Ships = append(slices.Clone(r.Ships), sh)
	st.Ships = &r
	st.Git = &proto.GitReport{Repos: []gitscan.Repo{{Path: "/w/hearth-api", Worktrees: []gitscan.Worktree{
		{Path: "/w/hearth-api", Branch: "main"}, {Path: "/w/wick-mobile", Branch: "cache-store"}}}}}
	m, _, _ := sideShotOf(t, st, 160, 38)
	return m
}

// chooseShip chooses the row of the ship no session drives, the keys in
// the list.
func chooseShip(t *testing.T, m Model) Model {
	t.Helper()
	i := slices.IndexFunc(m.sideSessions(), isShipCard)
	if i < 0 {
		t.Fatalf("no row of the ship nobody drives:\n%s", plainFrame(m))
	}
	m.chooseRow(m.sideSessions()[i])
	m.toList()
	return m
}

// TestAShipWithNoSessionThatWaitsNeedsYou: a pull request that no session
// drives and that waits for your merge is a row in NEEDS YOU, named by its
// branch, with its badge; the header counts it. ⏎ opens New session on its
// branch, N too, and p merges it behind the typed y.
func TestAShipWithNoSessionThatWaitsNeedsYou(t *testing.T) {
	m := orphanShip(t, shipWaits)
	before := orphanShip(t, shipRunning).needCount()
	if got := m.needCount(); got != before+1 {
		t.Errorf("the header counts %d, want %d with the ship that waits", got, before+1)
	}
	m = chooseShip(t, m)
	s, _ := m.sideChosen()
	if m.sideStateOf(s) != sideNeed || s.Name != "cache-store" {
		t.Fatalf("the ship's row is %q in %s", s.Name, m.sideStateOf(s).head())
	}
	if l := lineWith(frameOf(m), "cache-store"); !strings.Contains(l, "#4570") {
		t.Errorf("the ship's row has no badge: %q", l)
	}
	for _, k := range []string{"enter", "N"} {
		nm := pressOn(m, k)
		if nm.form == nil || nm.whereSelectedIn(nm.form).path() != "/w/wick-mobile" || !strings.Contains(nm.form.first, "#4570") {
			t.Errorf("%s on the ship's row: form %v, flash %q", k, nm.form != nil, nm.flash)
		}
	}
	for _, k := range []string{"x", "z", "r", "A", "y"} {
		if nm := pressOn(m, k); nm.pending != nil || nm.form != nil || nm.held != nil {
			t.Errorf("%s on the ship's row opened %+v %+v", k, nm.pending, nm.form)
		}
	}
	nm := pressOn(m, "p")
	if nm.pending == nil || !strings.Contains(strings.Join(nm.pending.steps[0], " "), "gh pr merge 4570") {
		t.Fatalf("p on the ship's row: pending %+v, flash %q", nm.pending, nm.flash)
	}
}

// TestAShipWithNoSessionStaysInTheList: one that runs (CI) is a row in
// IDLE with its badge: no pull request leaves the list.
func TestAShipWithNoSessionStaysInTheList(t *testing.T) {
	m := chooseShip(t, orphanShip(t, shipRunning))
	s, _ := m.sideChosen()
	if m.sideStateOf(s) != sideIdle {
		t.Fatalf("the running ship's row is in %s", m.sideStateOf(s).head())
	}
	if l := lineWith(frameOf(m), "cache-store"); !strings.Contains(l, "#4570") {
		t.Errorf("the running ship's row has no badge: %q", l)
	}
	if got := chatLineOf(m, 100); !strings.Contains(got, "no session drives #4570") {
		t.Errorf("the ship's chat line %q does not say why it takes no text", got)
	}
}

// noPaneConsole is the console with two sessions outside tmux: no pane, each
// its own process.
func noPaneConsole(t *testing.T) Model {
	t.Helper()
	st := hostState(sess(1, "hearth-api", "%1", nil), sess(4242, "flint-cli", "", nil), sess(4243, "spark-cli", "", nil))
	m, _, _ := sideShotOf(t, st, 160, 38)
	return m
}

// TestTwoSessionsOutsideTmuxAreTwoRows: a session with no pane is a row by
// its process: it can be chosen, and ↑↓ step from one to the other.
func TestTwoSessionsOutsideTmuxAreTwoRows(t *testing.T) {
	m := noPaneConsole(t)
	var names []string
	for _, k := range m.sideOrder() {
		i := slices.IndexFunc(m.sideSessions(), func(s sample.Session) bool { return paneOf(s) == k })
		names = append(names, m.sideSessions()[i].Name)
	}
	if !slices.Contains(names, "flint-cli") || !slices.Contains(names, "spark-cli") || len(names) != 3 {
		t.Fatalf("the list holds %v", names)
	}
	m.toList()
	seen := map[string]bool{}
	for range 3 {
		s, ok := m.sideChosen()
		if !ok {
			t.Fatalf("no session is chosen on %v", m.chosenSide())
		}
		seen[s.Name] = true
		m = pressOn(m, "down")
	}
	if !seen["flint-cli"] || !seen["spark-cli"] {
		t.Errorf("↓ chose %v", seen)
	}
}

// TestASessionOutsideTmuxSaysWhy: its line says it takes no text, and a key
// that goes into its pane says it runs outside tmux.
func TestASessionOutsideTmuxSaysWhy(t *testing.T) {
	m := noPaneConsole(t)
	for _, s := range m.sideSessions() {
		if s.Name == "flint-cli" {
			m.chooseRow(s)
		}
	}
	if s, _ := m.sideChosen(); s.Name != "flint-cli" {
		t.Fatalf("chose %q", s.Name)
	}
	if got := chatLineOf(m, 100); !strings.Contains(got, "flint-cli runs outside tmux") {
		t.Errorf("the line %q does not say why it takes no text", got)
	}
	m.toLine()
	if nm := pressOn(m, "h", "i", "enter"); nm.lines[nm.chosenSide()] != "" || len(messageActs(boardFake(nm))) != 0 {
		t.Errorf("the line took %q", nm.lines[nm.chosenSide()])
	}
	if nm := pressOn(m, "ctrl+o"); nm.flash != noPane("flint-cli") {
		t.Errorf("ctrl+o said %q", nm.flash)
	}
}

// TestAShipsRowSaysWhatItsKeysDo: on a ship's row the key line names
// what ⏎ and p do, the chat's place shows the ship and the same two acts,
// and its head says the ship's state.
func TestAShipsRowSaysWhatItsKeysDo(t *testing.T) {
	m := chooseShip(t, orphanShip(t, shipWaits))
	keys := keyLineOf(m)
	if !strings.Contains(keys, "⏎ new session") || !strings.Contains(keys, "p merge") || strings.Contains(keys, "write") {
		t.Errorf("the key line %q", keys)
	}
	chat := strings.Join(plainLines(m.chatPanel(100, 20)), "\n")
	for _, want := range []string{"Cache the session store", "#4570", "waits for your merge", "new session on its branch", "p merge"} {
		if !strings.Contains(chat, want) {
			t.Errorf("the chat's place lacks %q:\n%s", want, chat)
		}
	}
	if head := plainLines(m.chatPanel(100, 20))[0]; strings.Contains(head, "idle") || !strings.Contains(head, "waits for your merge") {
		t.Errorf("the head %q", head)
	}
	m.toLine()
	if keys := keyLineOf(m); strings.Contains(keys, "send") {
		t.Errorf("in the line the key line %q offers a send", keys)
	}
	if nm := pressOn(m, "p"); nm.pending == nil {
		t.Errorf("p, which the key line offers, does not merge from the line: %q", nm.noticeText)
	}
}
