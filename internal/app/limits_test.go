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
	n := func(i int) *int { return &i }
	st.Limits = []limits.Account{
		{Provider: "anthropic", Account: "builder@example.com · Max", State: limits.Measured, At: at,
			Matches: n(4), Sparks: n(8), Forecast: &limits.Outlook{Lasts: true}},
		{Provider: "openai", Account: "builder@example.org · Plus", State: limits.Measured, At: at,
			Matches: n(5), Sparks: n(2), SparksReset: at.Add(5 * 24 * time.Hour), Forecast: &limits.Outlook{Out: at.Add(26 * time.Hour)}},
		{Provider: "anthropic", Account: "work@example.net · Team", State: limits.Unmeasured, Fix: "open the door “Show your limits”"},
	}
	return st
}

// TestEachAccountHasALimitsLine: under the metrics, one line per account:
// its mark, a short name, the matches, the sparks and the forecast. Two
// accounts of one provider are told apart by name; the same line fits a
// phone, and the tabs move down under them.
func TestEachAccountHasALimitsLine(t *testing.T) {
	for _, c := range []struct {
		w    int
		want []string
	}{
		{80, []string{
			" ✻ builder  4 matches · 8 sparks · lasts the week",
			" ✻ work     not measured · open the door “Show your limits”",
			" ❋ Plus     5 matches · 2 sparks until ",
		}},
		{40, []string{
			" ✻ 4 matches · 8 sparks · lasts the week",
			" ❋ 5 matches · 2 sparks · out Mon",
		}},
	} {
		m, _ := loadedWith(t, c.w, limitsState())
		lines := strings.Split(screen(m), "\n")
		head := strings.Join(lines[1:4], "\n")
		for _, w := range c.want {
			if !strings.Contains(head, w) {
				t.Errorf("width %d: no line %q in\n%s", c.w, w, strings.Join(lines[:5], "\n"))
			}
		}
		if !strings.Contains(lines[4], "QUEUE") && !strings.Contains(lines[4], "1") {
			t.Errorf("width %d: the tabs are not under the limits:\n%s", c.w, strings.Join(lines[:6], "\n"))
		}
		for i, l := range lines {
			if w := len([]rune(l)); w > c.w {
				t.Errorf("width %d: line %d is %d wide: %q", c.w, i, w, l)
			}
		}
	}
	// A tap on the tabs still switches tabs.
	m, _ := loadedWith(t, 100, limitsState())
	next, _ := m.Update(tea.MouseClickMsg{X: 14, Y: 4, Button: tea.MouseLeft})
	if next.(Model).tab == tabQueue {
		t.Fatalf("a tap on the tab row did not switch:\n%s", screen(next))
	}
	// A tap on a body row under three limits lines selects that row.
	for i, l := range strings.Split(screen(m), "\n") {
		if strings.Contains(l, "app-review") {
			tapped, _ := m.Update(tea.MouseClickMsg{X: 10, Y: i, Button: tea.MouseLeft})
			if it, ok := tapped.(Model).selectedQueue(); !ok || it.Name != "app-review" {
				t.Fatalf("a tap on line %d selected %+v", i, it)
			}
			break
		}
	}
	// No account: no line, the tabs stay where they were.
	none := queueState()
	none.Limits = nil
	plain, _ := loadedWith(t, 100, none)
	if l := strings.Split(screen(plain), "\n")[1]; !strings.Contains(l, "QUEUE") {
		t.Fatalf("without limits line 1 is %q", l)
	}
}

// TestARecommendationShowsItsResult: Enter on a recommendation shows the
// step, a second Enter runs it, and the row then says what happened: done,
// or what failed and that Enter tries again.
func TestARecommendationShowsItsResult(t *testing.T) {
	st := limitsState()
	st.Recommendations = []advice.Rec{{ID: "wk", Level: "warn", Title: "The week runs out Wed ~19:00: compact app-feature",
		Evidence: "builder@example.org · Plus: 2 sparks left",
		Primary:  &advice.Action{Label: "jump to the session", Nav: true, Steps: [][]string{{"tmux", "switch-client", "-t", "%1"}}},
		Second: &advice.Action{Label: "send /compact", Destructive: true, Steps: [][]string{{"tmux", "send-keys", "-t", "%1", "/compact", "Enter"}},
			Guards: []advice.Guard{{IdlePane: "%1"}}}}}
	m, f := loadedWith(t, 100, st)
	var next tea.Model = m
	for r, ok := next.(Model).selectedRec(); !ok || r.ID != "wk"; r, ok = next.(Model).selectedRec() {
		next, _ = key(next, "down")
	}
	next, _ = key(next, "x")
	if p := next.(Model).pending; p == nil || p.String() != "tmux send-keys -t %1 /compact Enter" {
		t.Fatalf("x shows %+v", p)
	}
	next = runCmd(key(next, "y"))
	sent := f.lastAct(t)
	next, _ = next.Update(resultMsg(proto.Result{ActID: sent, Skipped: []string{"%1 is busy now"}}))
	if out := screen(next); !strings.Contains(out, "✗ skipped: %1 is busy now · x tries again") {
		t.Fatalf("a skipped step does not say so on the row:\n%s", out)
	}
	next, _ = key(next, "x")
	next = runCmd(key(next, "y"))
	next, _ = next.Update(resultMsg(proto.Result{ActID: f.lastAct(t), Ran: [][]string{{"tmux", "send-keys", "-t", "%1", "/compact", "Enter"}}}))
	if out := screen(next); !strings.Contains(out, "✓ ran: tmux send-keys -t %1 /compact Enter") {
		t.Fatalf("a step that ran does not say so on the row:\n%s", out)
	}
	next, _ = next.Update(resultMsg(proto.Result{ActID: "a-other", Err: "nope"}))
	if out := screen(next); strings.Contains(out, "✗ failed: nope") {
		t.Fatalf("another act's result landed on the row:\n%s", out)
	}
	next, _ = key(next, "x")
	next = runCmd(key(next, "y"))
	next, _ = next.Update(resultMsg(proto.Result{ActID: f.lastAct(t), Err: "the pane is gone"}))
	if out := screen(next); !strings.Contains(out, "✗ failed: the pane is gone · x tries again") {
		t.Fatalf("a failure does not say so on the row:\n%s", out)
	}
	// The rec goes, and comes back later: it shows its title, not an old result.
	gone := st
	gone.Recommendations = nil
	next, _ = next.Update(stateMsg(gone))
	next, _ = next.Update(stateMsg(st))
	if out := screen(next); strings.Contains(out, "✗ failed") || !strings.Contains(out, "▲ The week runs out Wed ~19:00") {
		t.Fatalf("an old result came back with the rec:\n%s", out)
	}
}

// runCmd runs the command a key returned, as the program would.
func runCmd(m tea.Model, cmd tea.Cmd) tea.Model {
	if cmd != nil {
		cmd()
	}
	return m
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
	for r, ok := next.(Model).selectedRec(); !ok || r.ID != "wk"; r, ok = next.(Model).selectedRec() {
		next, _ = key(next, "down")
	}
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
// account's limits as the header does.
func TestStatusTextHasTheLimits(t *testing.T) {
	st := limitsState()
	out := Summary(st.Doc)
	for _, want := range []string{
		"limits anthropic builder@example.com · Max: 4 matches · 8 sparks · lasts the week\n",
		"limits anthropic work@example.net · Team: not measured · open the door “Show your limits”\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in\n%s", want, out)
		}
	}
}
