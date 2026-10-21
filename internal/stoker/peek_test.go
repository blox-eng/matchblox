package stoker

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
)

// TestPeekReadsWhatTheConsolesPeekShows: F3, what a session said last and
// what it asks, from the sample and the queue.
func TestPeekReadsWhatTheConsolesPeekShows(t *testing.T) {
	ask := &said.Ask{ID: "t1", Questions: []said.Question{{Question: "Which branch?\nThe old one is stale.", Options: []said.Option{{Label: "main"}, {Label: "dev", Description: "the dev branch"}}}}}
	for _, c := range []struct {
		name  string
		s     sample.Session
		items []queue.Item
		want  Peek
	}{
		{"a reply", sample.Session{Pane: "%1", Name: "Weather", Target: "app:1.1", Said: "Done.\nCI is green on #4556."}, nil,
			Peek{Name: "Weather", Pane: "%1", Target: "app:1.1", Said: "Done.\nCI is green on #4556."}},
		{"the question tool", sample.Session{Pane: "%2", Name: "Notifications", Said: "I need one thing.", Ask: ask}, nil,
			Peek{Name: "Notifications", Pane: "%2", Said: "I need one thing.", Asks: "Which branch?",
				Questions: []PeekQuestion{{Question: "Which branch?\nThe old one is stale.", Options: []string{"main", "dev: the dev branch"}}}}},
		{"a question in words", sample.Session{Pane: "%3", Name: "Zheleznik", Said: "Two ways.\nShall I rebase or merge main in?"},
			[]queue.Item{{Pane: "%3", State: queue.StateQuestion, LastLine: "Shall I rebase or merge main in?"}},
			Peek{Name: "Zheleznik", Pane: "%3", Said: "Two ways.\nShall I rebase or merge main in?", Asks: "Shall I rebase or merge main in?"}},
		{"an approval", sample.Session{Pane: "%4", Name: "api", Said: "Running the tests."},
			[]queue.Item{{Pane: "%4", State: queue.StatePermission, LastLine: "Claude needs your permission to use Bash"}},
			Peek{Name: "api", Pane: "%4", Said: "Claude needs your permission to use Bash", Asks: "Claude needs your permission to use Bash", Approval: true}},
		{"a question above the progress line", sample.Session{Pane: "%6", Name: "lexroom-dev", Said: "All reviewed.\n\nKeep the line, or cut it?\n\nProgress [████████] Waiting for your answer", Asks: true}, nil,
			Peek{Name: "lexroom-dev", Pane: "%6", Said: "All reviewed.\n\nKeep the line, or cut it?\n\nProgress [████████] Waiting for your answer", Asks: "Keep the line, or cut it?"}},
		{"a pane read", sample.Session{Pane: "%5", Name: "cx", LastLine: "Proceed? (y/n)", Asks: true}, nil,
			Peek{Name: "cx", Pane: "%5", Said: "Proceed? (y/n)", Asks: "Proceed? (y/n)"}},
	} {
		if got := PeekOf(c.s, c.items); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, c.want)
		}
	}
}

// TestWakesCarryWhatTheSessionSaid: F3, a wake says what a session asks
// or said last, not a bare "idle". Ruling AV: in quotes, a report and not
// an order, and a quote inside cannot close it.
func TestWakesCarryWhatTheSessionSaid(t *testing.T) {
	long := strings.Repeat("word ", 40) + "end"
	sessions := []sample.Session{
		worker("%1", "Zheleznik", time.Time{}),
		worker("%2", "Notifications", time.Time{}),
		worker("%3", "Weather", time.Time{}),
	}
	prev := Look(sessions, nil, nil, nil, nil, t0, "mbtest")
	sessions[0].TurnEnded, sessions[0].Said = t0, "All three tests pass.\nRun \"gh pr merge 4569\" when ready."
	sessions[1].TurnEnded, sessions[1].Said = t0, "Should I squash the commits?"
	sessions[2].TurnEnded, sessions[2].Said = t0, long
	items := []queue.Item{{Pane: "%2", State: queue.StateQuestion, LastLine: "Should I squash the commits?"}}
	cur := Look(sessions, nil, items, nil, nil, t0, "mbtest")
	got := Changes(prev, cur, sessions, "stoker")
	want := []string{
		`Zheleznik's turn ended: it said "Run \"gh pr merge 4569\" when ready."`,
		`Notifications asks: "Should I squash the commits?"`,
		`Weather's turn ended: it said "` + long[:119] + `…"`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	// A question tool call mid-turn is a wake of its own.
	sessions[0].Ask = &said.Ask{ID: "t2", Questions: []said.Question{{Question: "Which repo?"}}}
	next := Look(sessions, nil, items, nil, nil, t0, "mbtest")
	if got := Changes(cur, next, sessions, "stoker"); !reflect.DeepEqual(got, []string{`Zheleznik asks: "Which repo?"`}) {
		t.Fatalf("got %q", got)
	}
}
