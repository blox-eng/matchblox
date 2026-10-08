package queue

import (
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/sample"
)

var t0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func at(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }

func session(id, pane, name, status string, idle time.Duration) sample.Session {
	return sample.Session{SessionID: id, Pane: pane, Target: "lab:1." + pane[1:], Name: name, Status: status, Busy: status == "busy", Idle: idle}
}

// TestQueueStates covers each row of design 0001 §7.1.
func TestQueueStates(t *testing.T) {
	for _, tc := range []struct {
		name   string
		events []hooks.Event
		want   string // "" = not in the queue
	}{
		{"permission", []hooks.Event{{Name: "Notification", Kind: hooks.KindPermission}}, StatePermission},
		{"idle prompt", []hooks.Event{{Name: "Notification", Kind: hooks.KindIdlePrompt}}, StateQuestion},
		{"stop", []hooks.Event{{Name: "Stop"}}, StateFinished},
		{"prompt submit leaves", []hooks.Event{{Name: "Stop"}, {Name: "UserPromptSubmit"}}, ""},
		{"session start appears, not waiting", []hooks.Event{{Name: "SessionStart"}}, ""},
		{"session end disappears", []hooks.Event{{Name: "Stop"}, {Name: "SessionEnd"}}, ""},
		{"other notification ignored", []hooks.Event{{Name: "Notification", Kind: "auth_success"}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := New()
			for i, ev := range tc.events {
				ev.SessionID, ev.Pane, ev.At = "s1", "%1", at(i)
				q.Apply(ev)
			}
			q.Merge([]sample.Session{session("s1", "%1", "api", "idle", time.Minute)}, at(10))
			items := q.Items()
			if tc.want == "" {
				if len(items) != 0 {
					t.Fatalf("items %+v", items)
				}
				return
			}
			if len(items) != 1 || items[0].State != tc.want || items[0].Estimated {
				t.Fatalf("items %+v, want one %s", items, tc.want)
			}
		})
	}
}

func TestQueueFillsTheSession(t *testing.T) {
	q := New()
	q.Apply(hooks.Event{Name: "Stop", SessionID: "s1", At: at(0), Message: "Done."})
	q.Merge([]sample.Session{session("s1", "%3", "api-auth", "idle", 0)}, at(5))
	it := q.Items()[0]
	if it.Pane != "%3" || it.Target != "lab:1.3" || it.Name != "api-auth" || it.LastLine != "Done." || !it.Since.Equal(at(0)) {
		t.Fatalf("%+v", it)
	}
}

func TestQueueKeepsTheWaitStart(t *testing.T) {
	q := New()
	q.Apply(hooks.Event{Name: "Stop", SessionID: "s1", At: at(0), Message: "Shall I merge?"})
	q.Apply(hooks.Event{Name: "Notification", Kind: hooks.KindIdlePrompt, SessionID: "s1", At: at(60), Message: "Claude is waiting for your input"})
	q.Merge([]sample.Session{session("s1", "%1", "api", "idle", 0)}, at(61))
	it := q.Items()[0]
	if it.State != StateQuestion || !it.Since.Equal(at(0)) || it.LastLine != "Shall I merge?" {
		t.Fatalf("%+v", it)
	}
}

func TestQueueOldestFirst(t *testing.T) {
	q := New()
	q.Apply(hooks.Event{Name: "Stop", SessionID: "new", At: at(30)})
	q.Apply(hooks.Event{Name: "Notification", Kind: hooks.KindPermission, SessionID: "old", At: at(10)})
	q.Merge([]sample.Session{
		session("new", "%1", "web", "idle", 0),
		session("old", "%2", "api", "idle", 0),
		session("guess", "%3", "docs", "idle", 5*time.Minute), // since at(40)-5m: oldest
	}, at(40))
	var got []string
	for _, it := range q.Items() {
		got = append(got, it.SessionID)
	}
	if len(got) != 3 || got[0] != "guess" || got[1] != "old" || got[2] != "new" {
		t.Fatalf("order %v", got)
	}
}

func TestUserPromptSubmitLeavesQueue(t *testing.T) {
	q := New()
	q.Apply(hooks.Event{Name: "Notification", Kind: hooks.KindPermission, SessionID: "s1", At: at(0)})
	q.Apply(hooks.Event{Name: "UserPromptSubmit", SessionID: "s1", At: at(1)})
	// The sampler still reads idle for a moment: a hooked session is never guessed.
	q.Merge([]sample.Session{session("s1", "%1", "api", "idle", time.Second)}, at(2))
	if items := q.Items(); len(items) != 0 {
		t.Fatalf("items %+v", items)
	}
}

func TestFallbackIsEstimated(t *testing.T) {
	q := New()
	q.Merge([]sample.Session{
		session("a", "%1", "api", "idle", 4*time.Minute),
		session("b", "%2", "web", "busy", 0),
	}, at(600))
	items := q.Items()
	if len(items) != 1 {
		t.Fatalf("items %+v", items)
	}
	it := items[0]
	if !it.Estimated || it.State != StateFinished || it.SessionID != "a" || !it.Since.Equal(at(600).Add(-4*time.Minute)) {
		t.Fatalf("%+v", it)
	}
}

func TestQueueDropsAGoneSession(t *testing.T) {
	q := New()
	q.Apply(hooks.Event{Name: "Stop", SessionID: "s1", At: at(0)})
	q.Merge(nil, at(1))
	if items := q.Items(); len(items) != 0 {
		t.Fatalf("items %+v", items)
	}
}

func TestQueueMatchesByPaneWithoutSessionID(t *testing.T) {
	q := New()
	q.Apply(hooks.Event{Name: "Stop", SessionID: "s1", Pane: "%7", At: at(0)})
	q.Merge([]sample.Session{session("", "%7", "api", "idle", 0)}, at(1))
	items := q.Items()
	if len(items) != 1 || items[0].Name != "api" || items[0].Estimated {
		t.Fatalf("items %+v", items)
	}
}

// Review 5: a spooled event older than what the queue already knows is stale.
func TestQueueIgnoresOlderEvent(t *testing.T) {
	q := New()
	q.Apply(hooks.Event{Name: "UserPromptSubmit", SessionID: "s1", Pane: "%1", At: at(20)})
	q.Apply(hooks.Event{Name: "Stop", SessionID: "s1", Pane: "%1", At: at(10)})
	q.Merge([]sample.Session{session("s1", "%1", "api", "idle", time.Second)}, at(30))
	if items := q.Items(); len(items) != 0 {
		t.Fatalf("a stale Stop came back: %+v", items)
	}
}

// Review 6: the person approved in the pane; a tool ran.
func TestPostToolUseClearsPermission(t *testing.T) {
	q := New()
	q.Apply(hooks.Event{Name: "Notification", Kind: hooks.KindPermission, SessionID: "s1", Pane: "%1", At: at(0)})
	q.Apply(hooks.Event{Name: "PostToolUse", SessionID: "s1", Pane: "%1", At: at(5)})
	q.Merge([]sample.Session{session("s1", "%1", "api", "busy", 0)}, at(6))
	if items := q.Items(); len(items) != 0 {
		t.Fatalf("items %+v", items)
	}
}

// PostToolUse does not clear a session that finished its turn.
func TestPostToolUseKeepsFinished(t *testing.T) {
	q := New()
	q.Apply(hooks.Event{Name: "Stop", SessionID: "s1", Pane: "%1", At: at(0)})
	q.Apply(hooks.Event{Name: "PostToolUse", SessionID: "s1", Pane: "%1", At: at(1)})
	q.Merge([]sample.Session{session("s1", "%1", "api", "idle", 0)}, at(2))
	if items := q.Items(); len(items) != 1 {
		t.Fatalf("items %+v", items)
	}
}

// Review 8: the sampler can lag behind a hook (a new session id after
// /clear); the event waits a grace period instead of being dropped.
func TestQueueKeepsUnseenForAGrace(t *testing.T) {
	q := New()
	q.Apply(hooks.Event{Name: "Stop", SessionID: "new", Pane: "%9", At: at(0)})
	q.Merge(nil, at(1))
	q.Merge([]sample.Session{session("new", "%9", "api", "idle", 0)}, at(10))
	if items := q.Items(); len(items) != 1 || items[0].Estimated {
		t.Fatalf("the event was dropped before the sampler saw the session: %+v", items)
	}
	q.Apply(hooks.Event{Name: "Stop", SessionID: "gone", Pane: "%8", At: at(10)})
	q.Merge(nil, at(10+int(UnseenGrace/time.Second)+1))
	if _, ok := q.hooked["gone"]; ok {
		t.Fatal("an unseen session is kept forever")
	}
}

// TestAPaneThatAsksWaitsForYou: an agent without hooks waits for the person
// when its pane is idle and asks; the item says it was read from the pane.
func TestAPaneThatAsksWaitsForYou(t *testing.T) {
	asks := session("", "%4", "codex", "idle", time.Minute)
	asks.FromPane, asks.Asks, asks.LastLine = true, true, "Would you like to run the following command?"
	done := session("", "%5", "opencode", "idle", 2*time.Minute)
	done.FromPane, done.LastLine = true, "Done. All 42 tests pass."
	q := New()
	q.Merge([]sample.Session{asks, done}, at(600))
	items := q.Items()
	if len(items) != 2 {
		t.Fatalf("items %+v", items)
	}
	byName := map[string]Item{items[0].Name: items[0], items[1].Name: items[1]}
	if it := byName["codex"]; it.State != StateQuestion || !it.FromPane || !it.Estimated || it.LastLine != asks.LastLine {
		t.Fatalf("codex %+v", it)
	}
	if it := byName["opencode"]; it.State != StateFinished || !it.FromPane || it.LastLine != done.LastLine {
		t.Fatalf("opencode %+v", it)
	}
}
