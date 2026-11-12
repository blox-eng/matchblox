package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/proto"
)

func limitsState() proto.State {
	st := queueState()
	at := st.At
	st.Limits = []limits.Account{
		{Provider: "anthropic", Account: "builder@example.com · Max", State: limits.Measured, At: at, Forecast: &limits.Outlook{Lasts: true},
			Windows: []limits.Window{{UsedPct: 62, Minutes: limits.BoxMinutes, ResetsAt: at.Add(2 * time.Hour)},
				{UsedPct: 41, Minutes: limits.WeekMinutes, ResetsAt: at.Add(3 * 24 * time.Hour)}}},
		{Provider: "openai", Account: "builder@example.org · Plus", State: limits.Measured, At: at, Forecast: &limits.Outlook{Out: at.Add(26 * time.Hour)},
			Windows: []limits.Window{{UsedPct: 12, Minutes: limits.BoxMinutes, ResetsAt: at.Add(4 * time.Hour)},
				{UsedPct: 92, Minutes: limits.WeekMinutes, ResetsAt: at.Add(5 * 24 * time.Hour)}}},
		{Provider: "anthropic", Account: "work@example.net · Team", State: limits.Unmeasured, Fix: "open the door “Show your limits”"},
	}
	return st
}

// selectRec presses ↑ until the recommendation id is selected.
func selectRec(t *testing.T, m tea.Model, id string) tea.Model {
	t.Helper()
	for range m.(Model).rowCount() {
		m, _ = key(m, "up")
	}
	for range m.(Model).rowCount() {
		if r, ok := m.(Model).selectedRec(); ok && r.ID == id {
			return m
		}
		m, _ = key(m, "down")
	}
	t.Fatalf("no recommendation %s to select", id)
	return m
}

// TestTheHeaderHasNoLimits: the header is one line and the tabs are under
// it; the limits are in the dock (design/0006 §11.5).
func TestTheHeaderHasNoLimits(t *testing.T) {
	for _, w := range []int{44, 86, 132} {
		m, _ := loadedWith(t, w, limitsState())
		lines := strings.Split(screen(m), "\n")
		if !strings.Contains(lines[1], "BOARD") || strings.Contains(strings.Join(lines[:3], "\n"), "62%") {
			t.Fatalf("width %d: the header is not one line:\n%s", w, strings.Join(lines[:4], "\n"))
		}
		if !strings.Contains(lines[0], "● 2 need you") { // the board's words
			t.Fatalf("width %d: the header does not say who waits: %q", w, lines[0])
		}
	}
}

func (f *fakeConn) lastAct(t *testing.T) string {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.sent) - 1; i >= 0; i-- {
		if f.sent[i].Kind == proto.KindAct {
			return f.sent[i].ID
		}
	}
	t.Fatal("no act was sent")
	return ""
}

// TestAJumpLaterDoesNotRewriteTheRow: the jump of a recommendation shows
// on its row; a jump from a queue row afterwards does not replace it.
func TestAJumpLaterDoesNotRewriteTheRow(t *testing.T) {
	st := limitsState()
	st.Recommendations = []advice.Rec{{ID: "wk", Level: "warn", Title: "The week runs out Wed ~19:00: compact app-feature",
		Primary: &advice.Action{Label: "jump to the session", Nav: true, Steps: [][]string{{"tmux", "switch-client", "-t", "%1"}}}}}
	m, _ := loadedWith(t, 100, st)
	var next tea.Model = m
	next = selectRec(t, next, "wk")
	next, _ = key(next, "enter")
	next, _ = key(next, "enter")
	next, _ = next.Update(ranMsg{cmd: "tmux switch-client -t %1", jumped: "%1"})
	if !strings.Contains(screen(next), "✓ ran: tmux switch-client -t %1") {
		t.Fatalf("the jump is not on the row:\n%s", screen(next))
	}
	next, _ = next.Update(ranMsg{cmd: "tmux switch-client -t %2", err: errors.New("no pane %2")})
	if out := screen(next); strings.Contains(out, "✗ failed") || !strings.Contains(out, "✓ ran: tmux switch-client -t %1") {
		t.Fatalf("another jump rewrote the row:\n%s", out)
	}
}

// TestStatusTextHasTheLimits: `matchblox status --text` says each
// account's limits as the dock does.
func TestStatusTextHasTheLimits(t *testing.T) {
	st := limitsState()
	out := Summary(st.Doc)
	for _, want := range []string{
		"limits anthropic builder@example.com · Max: 5h 62% · wk 41% · lasts the week\n",
		"limits anthropic work@example.net · Team: not measured · open the door “Show your limits”\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in\n%s", want, out)
		}
	}
}

// TestCursorsAccountIsInTheDock: a Cursor session's account has a line of
// its own in USAGE, with Cursor's mark, as every agent's account does.
func TestCursorsAccountIsInTheDock(t *testing.T) {
	st := limitsState()
	st.Limits = append(st.Limits, limits.Account{Provider: "cursor", Account: "builder@example.com · Acme", State: limits.Unmeasured, Fix: "Cursor's plan usage is not read yet"})
	m, _ := loadedWith(t, 200, st)
	m.tab = tabMachine // read on Machine, the dock off the board (Task 12 step 2)
	if out := screen(m); !strings.Contains(out, "⬢ Cursor Acme") || !strings.Contains(out, "not measured · Cursor's plan usage is not read yet") {
		t.Errorf("the dock lacks Cursor's account:\n%s", out)
	}
}
