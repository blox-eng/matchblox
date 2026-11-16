package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

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

// TestAReusedPidStartsWithAnEmptyLine: outside tmux a session keys on its
// process and the process's start, so a new process that reuses the pid
// of one that ended starts with an empty line and no note of the old one.
func TestAReusedPidStartsWithAnEmptyLine(t *testing.T) {
	started := func(at uint64) func(*sample.Session) { return func(s *sample.Session) { s.Start = at } }
	st := hostState(sess(1, "hearth-api", "%1", nil), sess(4242, "flint-cli", "", started(100)))
	m, _, _ := sideShotOf(t, st, 160, 38)
	old := m.sideSessions()[slices.IndexFunc(m.sideSessions(), func(s sample.Session) bool { return s.Name == "flint-cli" })]
	m.setLine(paneOf(old), "half a message")
	m.lineNote = lineNote{paneOf(old), "flint-cli ended · the text is not sent"}

	next, _ := m.Update(stateMsg(hostState(sess(1, "hearth-api", "%1", nil), sess(4242, "spark-cli", "", started(200)))))
	m = next.(Model)
	reused := m.sideSessions()[slices.IndexFunc(m.sideSessions(), func(s sample.Session) bool { return s.Name == "spark-cli" })]
	if paneOf(reused) == paneOf(old) {
		t.Fatalf("the new process of pid 4242 has the old one's key %v", paneOf(old))
	}
	if got := m.lines[paneOf(reused)]; got != "" {
		t.Errorf("the new process of pid 4242 starts with the line %q", got)
	}
	if m.lineNote.k == paneOf(reused) {
		t.Errorf("the new process of pid 4242 has the old one's note %q", m.lineNote.text)
	}
}

// TestASessionWithNoPidAndNoPaneSharesNoKey: two sessions with no pane
// and no process are two keys by their session ids; with no id either,
// the row cannot be chosen, and the chat says why.
func TestASessionWithNoPidAndNoPaneSharesNoKey(t *testing.T) {
	id := func(sid string) func(*sample.Session) { return func(s *sample.Session) { s.SessionID = sid } }
	a, b := sess(0, "flint-cli", "", id("a1"))(sample.Session{}), sess(0, "spark-cli", "", id("b2"))(sample.Session{})
	if paneOf(a) == paneOf(b) {
		t.Fatalf("two sessions with no pid and no pane share the key %v", paneOf(a))
	}
	m, _, _ := sideShotOf(t, hostState(sess(1, "hearth-api", "%1", nil), sess(0, "wick-mobile", "", nil)), 160, 38)
	i := slices.IndexFunc(m.sideSessions(), func(s sample.Session) bool { return s.Name == "wick-mobile" })
	if i < 0 {
		t.Fatalf("the list holds no wick-mobile:\n%s", plainFrame(m))
	}
	m.sideAt = paneOf(m.sideSessions()[i])
	if _, ok := m.sideChosen(); ok {
		t.Errorf("a session with no pid, pane or id can be chosen")
	}
	if chat := strings.Join(plainLines(m.chatPanel(100, 20)), "\n"); strings.Contains(chat, "No sessions yet") || !strings.Contains(chat, "wick-mobile") {
		t.Errorf("the chat on it says:\n%s", chat)
	}
}

// TestAPullRequestWithNoSessionHasNoRow: the list shows only sessions. A
// pull request that no session drives has no row and counts as no need,
// even when it waits for your merge. A session's own pull request stays
// the badge on its row.
func TestAPullRequestWithNoSessionHasNoRow(t *testing.T) {
	s := boardMatch("flint-auth", time.Hour)
	st := boardState(s)
	st.Ships.Ships = []proto.Ship{
		shipFor(s, 4551, shipRunning, "CI runs", 1, time.Hour),
		{Repo: "hearth/hearth-api", Number: 4570, Title: "Cache the session store", Kind: shipWaits,
			Note: "waits for your merge", Stage: 2, Since: boardAt.Add(-time.Hour),
			Merge: []string{"gh", "pr", "merge", "4570", "--squash"}},
	}
	m := boardModel(t, st)
	var names []string
	for _, l := range m.sideSessions() {
		names = append(names, l.Name)
	}
	if !slices.Equal(names, []string{"flint-auth"}) {
		t.Fatalf("the list holds %q, want only flint-auth", names)
	}
	if n := m.needCount(); n != 0 {
		t.Errorf("the header counts %d that need you, want 0", n)
	}
	frame := frameOf(m)
	if l := lineWith(frame, "need you"); l != "" {
		t.Errorf("the header says %q", l)
	}
	if l := lineWith(frame, "#4570"); l != "" {
		t.Errorf("the pull request no session drives shows: %q", l)
	}
	if l := lineWith(frame, "flint-auth"); !strings.Contains(l, "#4551 ci") {
		t.Errorf("the row %q lacks the badge #4551 ci", l)
	}
}
