package service

import (
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

var stateAt0 = time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

func emberAt(busy bool) sample.Session {
	return sample.Session{Agent: "claude", PID: 42, Pane: "%1", Name: "ember", SessionID: "s1", Status: "idle", Busy: busy, Started: stateAt0}
}

func sampleAt(s *Service, after time.Duration, sessions ...sample.Session) {
	s.publish(sample.Snapshot{At: stateAt0.Add(after), Sessions: sessions}, func(state.Doc) {})
}

func sampleEmber(s *Service, after time.Duration, sess sample.Session) time.Time {
	sampleAt(s, after, sess)
	return s.State().Sessions[0].StateAt
}

func TestStateAtMovesOnlyOnAChange(t *testing.T) {
	s := newTest(t, time.Hour)
	for _, c := range []struct {
		after time.Duration
		busy  bool
		want  time.Time
	}{
		{time.Minute, true, stateAt0}, // first seen: it works since it started
		{5 * time.Minute, true, stateAt0},
		{7 * time.Minute, false, stateAt0.Add(7 * time.Minute)},
	} {
		if got := sampleEmber(s, c.after, emberAt(c.busy)); !got.Equal(c.want) {
			t.Fatalf("at +%s StateAt %s, want %s", c.after, got, c.want)
		}
	}
}

func TestStateAtSurvivesARestartOfTheConsole(t *testing.T) {
	ended := stateAt0.Add(6 * time.Minute)
	resting := emberAt(false)
	resting.TurnEnded = ended
	s := newTest(t, time.Hour)
	sampleEmber(s, 0, emberAt(true))
	want := sampleEmber(s, 7*time.Minute, resting)
	s.sample = func() sample.Snapshot {
		return sample.Snapshot{At: stateAt0.Add(9 * time.Minute), Sessions: []sample.Session{resting}}
	}
	ctx := run(t, s)
	for i := range 2 {
		c, _ := connect(t, ctx, s)
		st := snapshot(t, c)
		if len(st.Sessions) != 1 || !st.Sessions[0].StateAt.Equal(want) || !st.Sessions[0].TurnEndedAt.Equal(ended) {
			t.Fatalf("console %d reads %+v, want StateAt %s and TurnEndedAt %s", i+1, st.Sessions, want, ended)
		}
		c.Close()
	}
}

func stateAtOf(t *testing.T, s *Service, sid string) time.Time {
	t.Helper()
	for _, x := range s.State().Sessions {
		if x.SessionID == sid {
			return x.StateAt
		}
	}
	t.Fatalf("no session %s in %+v", sid, s.State().Sessions)
	return time.Time{}
}

func TestStateAtCountsAnAskFromTheQueue(t *testing.T) {
	for _, pane := range []string{"%1", ""} {
		sess := emberAt(false)
		sess.Pane = pane
		s := newTest(t, time.Hour)
		sampleAt(s, 0, sess)
		asked := stateAt0.Add(2 * time.Minute)
		s.Hook(hooks.Event{Name: "Notification", Kind: hooks.KindPermission, SessionID: "s1", Pane: pane, At: asked, Message: "Claude needs your permission to use Bash"})
		if got := stateAtOf(t, s, "s1"); !got.Equal(asked) {
			t.Fatalf("pane %q: after the permission StateAt %s, want %s", pane, got, asked)
		}
		s.Hook(hooks.Event{Name: "Notification", Kind: hooks.KindIdlePrompt, SessionID: "s1", Pane: pane, At: asked.Add(time.Minute), Message: "Claude is waiting for your input"})
		if got := stateAtOf(t, s, "s1"); !got.Equal(asked) {
			t.Fatalf("pane %q: the reminder moved StateAt to %s", pane, got)
		}
	}
}

func TestStateAtOutlivesASampleThatMissesIt(t *testing.T) {
	s := newTest(t, time.Hour)
	sampleEmber(s, 0, emberAt(true))
	rested := sampleEmber(s, 7*time.Minute, emberAt(false))
	sampleAt(s, 8*time.Minute)
	sampleAt(s, 9*time.Minute)
	if got := sampleEmber(s, 10*time.Minute, emberAt(false)); !got.Equal(rested) {
		t.Fatalf("back after two missed samples, StateAt %s, want %s", got, rested)
	}
	for m := 11; m <= 13; m++ {
		sampleAt(s, time.Duration(m)*time.Minute)
	}
	if got := sampleEmber(s, 14*time.Minute, emberAt(false)); !got.Equal(stateAt0) {
		t.Fatalf("back after three missed samples, StateAt %s, want it first seen (%s)", got, stateAt0)
	}
}

func TestStateAtOfAnAskFirstSeenIsItsWait(t *testing.T) {
	sess := emberAt(false)
	sess.Started = stateAt0.Add(-3 * time.Hour)
	s := newTest(t, time.Hour)
	asked := stateAt0.Add(-2 * time.Minute)
	s.Hook(hooks.Event{Name: "Notification", Kind: hooks.KindPermission, SessionID: "s1", Pane: "%1", At: asked, Message: "Claude needs your permission to use Bash"})
	sampleAt(s, 0, sess)
	if got := stateAtOf(t, s, "s1"); !got.Equal(asked) {
		t.Fatalf("an ask first seen: StateAt %s, want its wait's start %s", got, asked)
	}
}
