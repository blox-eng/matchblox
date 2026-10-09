package stoker

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/sample"
)

var t0 = time.Date(2026, 10, 9, 23, 10, 0, 0, time.Local)

func full(pane string) sample.Session {
	return sample.Session{
		Agent: "claude", Pane: pane, SessionID: "s" + pane, Name: "work" + pane,
		Status: "idle", Idle: 5 * time.Minute, Tokens: 176000, ContextPct: 88,
	}
}

func on() Mode { return Mode{On: true, Since: t0} }

func TestStokerOffPlansNothing(t *testing.T) {
	if got := Plan(Mode{}, []sample.Session{full("%1")}, nil, 85, nil, t0); len(got) != 0 {
		t.Fatalf("off planned %+v", got)
	}
}

func TestStokerCompactsIdleAtLimit(t *testing.T) {
	got := Plan(on(), []sample.Session{full("%1")}, nil, 85, nil, t0)
	if len(got) != 1 {
		t.Fatalf("want one step, got %+v", got)
	}
	want := [][]string{
		{"tmux", "send-keys", "-t", "%1", "-l", "--", "/compact " + Instruction},
		{"tmux", "send-keys", "-t", "%1", "Enter"},
	}
	s := got[0]
	if !reflect.DeepEqual(s.Action.Steps, want) {
		t.Fatalf("argv %q", s.Action.Steps)
	}
	g := advice.Guard{IdlePane: "%1"}
	if !reflect.DeepEqual(s.Action.Guards, []advice.Guard{g, g}) || s.Session != "s%1" || s.Tokens != 176000 {
		t.Fatalf("step %+v", s)
	}
}

func TestStokerLeavesWhatItMustNotTouch(t *testing.T) {
	below := full("%1")
	below.ContextPct = 84
	busy := full("%2")
	busy.Status, busy.Busy = "busy", true
	cold := full("%3")
	cold.Idle = sample.ColdAfter
	codex := full("%4")
	codex.Agent = "codex" // its /compact takes no instruction
	asks := full("%5")
	asks.Asks = true
	permits := full("%6")
	permits.Permits = true
	queued := full("%7")
	nopane := full("")
	got := Plan(on(), []sample.Session{below, busy, cold, codex, asks, permits, queued, nopane}, map[string]bool{"%7": true}, 85, nil, t0)
	if len(got) != 0 {
		t.Fatalf("planned %+v", got)
	}
}

func TestStokerRateLimits(t *testing.T) {
	recent := []Entry{{Session: "s%1", At: t0.Add(-29 * time.Minute), Result: Sent}}
	if got := Plan(on(), []sample.Session{full("%1")}, nil, 85, recent, t0); len(got) != 0 {
		t.Fatalf("compacted twice within 30 min: %+v", got)
	}
	recent[0].At = t0.Add(-31 * time.Minute)
	if got := Plan(on(), []sample.Session{full("%1")}, nil, 85, recent, t0); len(got) != 1 {
		t.Fatalf("after 30 min: %+v", got)
	}
	var many []sample.Session
	for i := range 31 {
		many = append(many, full(fmt.Sprintf("%%%d", i)))
	}
	if got := Plan(on(), many, nil, 85, nil, t0); len(got) != MaxSteps {
		t.Fatalf("31 sessions planned %d steps", len(got))
	}
	// Steps of this run count; a step from before the run does not.
	var run []Entry
	for i := range 29 {
		run = append(run, Entry{Session: fmt.Sprint("old", i), At: t0.Add(time.Minute), Result: Sent})
	}
	run = append(run, Entry{Session: "before", At: t0.Add(-time.Minute), Result: Sent})
	if got := Plan(on(), many, nil, 85, run, t0.Add(2*time.Minute)); len(got) != 1 {
		t.Fatalf("29 steps in the run left %d", len(got))
	}
}

func TestNightEndsAtMorning(t *testing.T) {
	m, err := Mode{}.Night(t0, "07:00")
	want := time.Date(2026, 10, 10, 7, 0, 0, 0, time.Local)
	if err != nil || !m.On || !m.Until.Equal(want) || !m.Since.Equal(t0) {
		t.Fatalf("night %+v %v", m, err)
	}
	early := time.Date(2026, 10, 10, 2, 0, 0, 0, time.Local)
	if m2, _ := (Mode{}).Night(early, "07:00"); !m2.Until.Equal(want) {
		t.Fatalf("night at 02:00 ends %v", m2.Until)
	}
	if _, err := (Mode{}).Night(t0, "7am"); err == nil {
		t.Fatal("a bad night_ends was taken")
	}
	if _, ended := m.Expire(want.Add(-time.Second)); ended {
		t.Fatal("ended before morning")
	}
	off, ended := m.Expire(want)
	if !ended || off.On || !off.Ended.Equal(want) || !off.Since.Equal(t0) {
		t.Fatalf("morning %+v", off)
	}
	if _, ended := on().Expire(want.Add(24 * time.Hour)); ended {
		t.Fatal("a stoker with no end ended")
	}
}

func TestTurningOnAgainKeepsTheRun(t *testing.T) {
	m := Mode{}.TurnOn(t0)
	if again := m.TurnOn(t0.Add(time.Hour)); !again.Since.Equal(t0) {
		t.Fatalf("a second k started a new run: %+v", again)
	}
	n, _ := m.Night(t0.Add(time.Hour), "07:00")
	if !n.Since.Equal(t0) || n.Until.IsZero() {
		t.Fatalf("night over a running stoker %+v", n)
	}
	off := m.Off(t0.Add(2 * time.Hour))
	if off.On || !off.Until.IsZero() || !off.Ended.Equal(t0.Add(2*time.Hour)) {
		t.Fatalf("off %+v", off)
	}
	if next := off.TurnOn(t0.Add(3 * time.Hour)); !next.Since.Equal(t0.Add(3*time.Hour)) || !next.Ended.IsZero() {
		t.Fatalf("a new run %+v", next)
	}
}

func TestStoreSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set(on()); err != nil {
		t.Fatal(err)
	}
	e := Entry{ID: "a", At: t0.Add(time.Minute), Session: "s%1", Result: Sent, Before: 176000}
	if err := s.Append(e); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(Entry{ID: "a", After: 9400, Resume: "run the test"}); err != nil {
		t.Fatal(err)
	}
	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Mode().On || !again.Mode().Since.Equal(t0) {
		t.Fatalf("mode %+v", again.Mode())
	}
	es := again.Entries()
	if len(es) != 1 || es[0].Before != 176000 || es[0].After != 9400 || es[0].Resume != "run the test" || es[0].Session != "s%1" {
		t.Fatalf("entries %+v", es)
	}
}

func TestViewShowsTheRunUntilAcked(t *testing.T) {
	s, _ := Open(t.TempDir())
	_ = s.Set(on())
	_ = s.Append(Entry{ID: "old", At: t0.Add(-time.Hour), Result: Sent})
	_ = s.Append(Entry{ID: "a", At: t0.Add(time.Minute), Result: Sent})
	v := s.View()
	if len(v.Run) != 1 || v.Run[0].ID != "a" || !v.Unseen {
		t.Fatalf("view %+v", v)
	}
	_ = s.Set(s.Mode().Ack(t0.Add(2 * time.Minute)))
	if s.View().Unseen {
		t.Fatal("acked run still unseen")
	}
	_ = s.Append(Entry{ID: "b", At: t0.Add(3 * time.Minute), Result: Sent})
	if !s.View().Unseen {
		t.Fatal("a step after the ack is not unseen")
	}
}
