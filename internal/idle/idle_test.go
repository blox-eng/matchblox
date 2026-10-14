package idle

import (
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/ships"
)

var now = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func session(mod func(*sample.Session)) sample.Session {
	s := sample.Session{Provider: "anthropic", Account: "a@b.c · Max", Status: "idle", Idle: time.Hour,
		Progress: &sample.Progress{Pct: 40, Step: "x"}}
	if mod != nil {
		mod(&s)
	}
	return s
}

func TestReasonPrecedence(t *testing.T) {
	out := now.Add(90 * time.Minute)
	green := &ships.Ship{Number: 12, Kind: ships.WaitsForYou, Note: ships.NoteWaitsForMerge}
	red := &ships.Ship{Number: 12, Kind: ships.Stuck, Note: "CI failed · e2e-web", Failed: "e2e-web"}
	live := &ships.Ship{Number: 12, Kind: ships.Done, Stage: ships.StageLive}
	merged := &ships.Ship{Number: 12, Kind: ships.Running, Stage: ships.StageMerged}
	weekOut := []limits.Account{{Provider: "anthropic", Account: "a@b.c · Max", Windows: []limits.Window{
		{UsedPct: 30, Minutes: limits.BoxMinutes, ResetsAt: now.Add(time.Hour)},
		{UsedPct: 100, Minutes: limits.WeekMinutes, ResetsAt: out}}}}
	boxOut := []limits.Account{{Provider: "anthropic", Account: "a@b.c · Max", Windows: []limits.Window{
		{UsedPct: 100, Minutes: limits.BoxMinutes, ResetsAt: out}}}}
	stopped := func(s *sample.Session) { s.Out = out }
	for _, c := range []struct {
		name   string
		s      sample.Session
		asking bool
		ship   *ships.Ship
		accts  []limits.Account
		want   Reason
	}{
		{"green waits for the merge", session(nil), false, green, nil, Reason{"#12 waits your merge", "accent"}},
		{"green beats a limit", session(stopped), false, green, boxOut, Reason{"#12 waits your merge", "accent"}},
		{"red check", session(nil), false, red, nil, Reason{"CI red · e2e-web", "neg"}},
		{"merged", session(nil), false, merged, nil, Reason{"#12 merged · done", "ok"}},
		{"live", session(nil), false, live, nil, Reason{"#12 merged · done", "ok"}},
		{"out of the 5h box", session(stopped), false, nil, boxOut, Reason{"5h limit · back 13:30", "warn"}},
		{"out of the week", session(stopped), false, nil, weekOut, Reason{"week limit · back 13:30", "warn"}},
		{"out with no window read", session(stopped), false, nil, nil, Reason{"5h limit · back 13:30", "warn"}},
		{"failed turn", session(func(s *sample.Session) { s.Failed = "API Error: 529" }), false, nil, nil, Reason{"stuck: API Error: 529", "neg"}},
		{"progress done", session(func(s *sample.Session) { s.Progress.Pct = 100 }), false, nil, nil, Reason{"out of work", "faint"}},
		{"no progress, no ask", session(func(s *sample.Session) { s.Progress = nil }), false, nil, nil, Reason{"out of work", "faint"}},
		{"mid-work", session(nil), false, nil, nil, Reason{"idle", "faint"}},
		{"busy is not at rest", session(func(s *sample.Session) { s.Busy, s.Status = true, "busy" }), false, green, nil, Reason{}},
		{"asking is at no rest", session(nil), true, green, nil, Reason{}},
		{"asks in its last reply", session(func(s *sample.Session) { s.Asks = true }), false, green, nil, Reason{}},
		{"a fresh session has a reason too", session(func(s *sample.Session) { s.Idle = time.Minute }), false, green, nil, Reason{"#12 waits your merge", "accent"}},
		{"shells are not quiet", session(func(s *sample.Session) { s.Shells = []sample.Shell{{PID: 1}} }), false, nil, nil, Reason{}},
	} {
		if got := Of(c.s, c.asking, c.ship, c.accts, now); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// TestANewSessionIsIdleNotOutOfWork: F11, a session that never had a turn
// has had no work to run out of.
func TestANewSessionIsIdleNotOutOfWork(t *testing.T) {
	fresh := session(func(s *sample.Session) { s.Progress, s.Context = nil, "fresh" })
	if got := Of(fresh, false, nil, nil, now); got != (Reason{"idle", "faint"}) {
		t.Fatalf("a session with no turn: %+v", got)
	}
	worked := session(func(s *sample.Session) { s.Progress, s.Context = nil, "known" })
	if got := Of(worked, false, nil, nil, now); got.Word != "out of work" {
		t.Fatalf("a session after its turn: %+v", got)
	}
}
