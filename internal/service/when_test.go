package service

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/holds"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/ships"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// whenRun is a running day order with its clock, the stoker agent idle in
// %9 and work idle in %1, and #4573 green.
func whenRun(t *testing.T, night bool) (*Service, *[][]string, *time.Time) {
	t.Helper()
	s, ran := orderTest(t)
	clock := night0
	s.Now = func() time.Time { return clock }
	s.Capture = func(panes []string) map[string]string {
		out := map[string]string{}
		for _, p := range panes {
			out[p] = idleScreen
		}
		return out
	}
	mustAct(t, s, orderAct(t, "rebase work once #4573 is in", night))
	s.publish(sample.Snapshot{At: clock, Sessions: []sample.Session{
		{Agent: "claude", Pane: "%1", Target: "app:1.1", SessionID: "s1", Name: "work", Status: "idle", Idle: 5 * time.Minute},
		{Agent: "claude", Pane: "%9@mbtest", Target: "stoker:1.1", SessionID: "s9", Name: "stoker", Status: "idle"},
	}}, func(state.Doc) {})
	setShip(s, 4573, ships.StageGreen, ships.WaitsForYou, "")
	s.stoke(t.Context(), clock) // the wakes see the run
	return s, ran, &clock
}

func setShip(s *Service, n, stage int, kind, failed string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur.Ships = &ships.Report{Ships: []ships.Ship{{Repo: "o/r", Number: n, Stage: stage, Kind: kind, Failed: failed,
		Session: ships.SessionRef{Pane: "%7", Name: "billed"}}}}
}

func whenAct(text string) proto.Act { return fromStokerAct("stoker:when", text) }

// fired are the steps the run's triggers logged.
func fired(s *Service) []stoker.Entry {
	var out []stoker.Entry
	for _, e := range s.Stoker.View().Run {
		if e.Rule == stoker.RuleWhen {
			out = append(out, e)
		}
	}
	return out
}

func TestAShipThatMergesFiresItsSendOnce(t *testing.T) {
	s, ran, clock := whenRun(t, false)
	mustAct(t, s, whenAct(`{"ship":4573,"stage":"merged","do":"send","who":"work","text":"rebase on main"}`))
	m := s.Stoker.Mode()
	if len(m.Triggers) != 1 || m.Triggers[0].ID != "t1" || m.Triggers[0].Pane != "%1" || m.Triggers[0].Who != "work" {
		t.Fatalf("triggers %+v", m.Triggers)
	}
	*clock = clock.Add(time.Minute)
	s.stoke(t.Context(), *clock)
	if got := typed(*ran, "%1"); len(got) != 0 {
		t.Fatalf("fired before the merge: %q", got)
	}
	setShip(s, 4573, ships.StageMerged, ships.Done, "")
	for range 3 {
		*clock = clock.Add(time.Minute)
		s.stoke(t.Context(), *clock)
	}
	if got := typed(*ran, "%1"); !slices.Equal(got, []string{"rebase on main"}) {
		t.Fatalf("work was told %q", got)
	}
	f := fired(s)
	if len(f) != 1 || f[0].State != stoker.StepDone || f[0].Text != "when #4573 merged → sent work: rebase on main" || f[0].Pane != "%1" || len(f[0].Argv) == 0 {
		t.Fatalf("fired %+v", f)
	}
	if m := s.Stoker.Mode(); len(m.Triggers) != 0 {
		t.Fatalf("a fired trigger stays: %+v", m.Triggers)
	}
	if said := typed(*ran, "%9@mbtest"); !slices.ContainsFunc(said, func(l string) bool {
		return strings.Contains(l, "when #4573 merged: sent work: rebase on main")
	}) {
		t.Fatalf("the stoker was told %q", said)
	}
	// The fired send counts as a step of the run, and drives the pane.
	if n := ordered(s); n != 1 {
		t.Fatalf("%d messages counted, want 1", n)
	}
}

func TestARestTriggerFiresAtTheTurnsEnd(t *testing.T) {
	s, ran, clock := whenRun(t, false)
	mustAct(t, s, whenAct(`{"rest":"work","do":"send","who":"work","text":"now the docs"}`))
	*clock = clock.Add(time.Minute)
	s.stoke(t.Context(), *clock)
	if got := typed(*ran, "%1"); len(got) != 0 {
		t.Fatalf("fired before the turn ended: %q", got)
	}
	ended := *clock
	*clock = clock.Add(time.Minute)
	s.publish(sample.Snapshot{At: *clock, Sessions: []sample.Session{
		{Agent: "claude", Pane: "%1", Target: "app:1.1", SessionID: "s1", Name: "work", Status: "idle", Idle: time.Minute, TurnEnded: ended},
		{Agent: "claude", Pane: "%9@mbtest", Target: "stoker:1.1", SessionID: "s9", Name: "stoker", Status: "idle"},
	}}, func(state.Doc) {})
	s.stoke(t.Context(), *clock)
	if got := typed(*ran, "%1"); !slices.Equal(got, []string{"now the docs"}) {
		t.Fatalf("work was told %q", got)
	}
	if f := fired(s); len(f) != 1 || f[0].Text != "when work rests → sent work: now the docs" {
		t.Fatalf("fired %+v", f)
	}
}

func TestARefusedTriggerLogsFailed(t *testing.T) {
	s, ran, clock := whenRun(t, false)
	hs, err := holds.Open("")
	if err != nil {
		t.Fatal(err)
	}
	s.Holds = hs
	if err = s.Holds.Set(holds.Hold{SessionID: "s1", Pane: "%1", Name: "work", By: holds.ByYou}); err != nil {
		t.Fatal(err)
	}
	s.republish()
	mustAct(t, s, whenAct(`{"ship":4573,"stage":"red","do":"send","who":"work","text":"fix the test"}`))
	setShip(s, 4573, ships.StageCI, ships.Stuck, "test")
	*clock = clock.Add(time.Minute)
	s.stoke(t.Context(), *clock)
	if got := typed(*ran, "%1"); len(got) != 0 {
		t.Fatalf("a parked session was told %q", got)
	}
	f := fired(s)
	if len(f) != 1 || f[0].State != stoker.StepFailed || !strings.HasPrefix(f[0].Text, "when #4573 red → send work: fix the test failed: ") {
		t.Fatalf("fired %+v", f)
	}
	*clock = clock.Add(time.Minute)
	s.stoke(t.Context(), *clock)
	if said := typed(*ran, "%9@mbtest"); !slices.ContainsFunc(said, func(l string) bool { return strings.Contains(l, "when #4573 red: failed: ") }) {
		t.Fatalf("the stoker was told %q", said)
	}
	if m := s.Stoker.Mode(); len(m.Triggers) != 0 {
		t.Fatalf("a refused trigger fires again: %+v", m.Triggers)
	}
}

func TestTriggersAreDroppedWhenTheRunEnds(t *testing.T) {
	s, ran, clock := whenRun(t, false)
	mustAct(t, s, whenAct(`{"ship":4573,"stage":"merged","do":"send","who":"work","text":"rebase on main"}`))
	mustAct(t, s, proto.Act{Console: true, RecID: "stoker:off", Confirm: "y"})
	if m := s.Stoker.Mode(); len(m.Triggers) != 0 {
		t.Fatalf("the run ended and its triggers stay: %+v", m.Triggers)
	}
	mustAct(t, s, orderAct(t, "a new run", false))
	setShip(s, 4573, ships.StageMerged, ships.Done, "")
	*clock = clock.Add(time.Minute)
	s.stoke(t.Context(), *clock)
	if got := typed(*ran, "%1"); len(got) != 0 {
		t.Fatalf("an ended run's trigger fired: %q", got)
	}
}

func TestANightsTriggersFireWhileSealed(t *testing.T) {
	s, ran, clock := whenRun(t, true)
	mustAct(t, s, whenAct(`{"ship":4573,"stage":"merged","do":"send","who":"work","text":"rebase on main"}`))
	setShip(s, 4573, ships.StageMerged, ships.Done, "")
	*clock = clock.Add(time.Minute)
	s.stoke(t.Context(), *clock)
	if got := typed(*ran, "%1"); !slices.Equal(got, []string{"rebase on main"}) {
		t.Fatalf("the sealed night's trigger did not fire: %q", got)
	}
}

func TestOnlyTheStokerSetsTriggersAndDropsThem(t *testing.T) {
	s, _, _ := whenRun(t, false)
	a := proto.Act{RecID: "stoker:when", Confirm: "y", Text: `{"rest":"work","do":"compact","who":"work"}`, From: "%1"}
	if res := s.act(t.Context(), a); res.Err == "" {
		t.Fatal("a session the stoker does not run set a trigger")
	}
	for text, want := range map[string]string{
		`{"rest":"nobody","do":"compact","who":"work"}`:                "no session named nobody",
		`{"rest":"work","do":"compact","who":"nobody"}`:                "no session named nobody",
		`{"ship":4573,"stage":"approved","do":"compact","who":"work"}`: `"approved" is no stage: ci, green, merged, live, red`,
	} {
		if res := s.act(t.Context(), whenAct(text)); res.Err != want {
			t.Errorf("%s: %q, want %q", text, res.Err, want)
		}
	}
	mustAct(t, s, whenAct(`{"rest":"work","do":"compact","who":"work"}`))
	mustAct(t, s, whenAct(`{"ship":4573,"stage":"merged","do":"release","who":"work"}`))
	if res := s.act(t.Context(), whenAct(`{"drop":"t9"}`)); res.Err != "no trigger t9: `matchblox stoker when --list` lists them" {
		t.Fatalf("drop t9: %q", res.Err)
	}
	mustAct(t, s, whenAct(`{"drop":"t1"}`))
	if m := s.Stoker.Mode(); len(m.Triggers) != 1 || m.Triggers[0].ID != "t2" {
		t.Fatalf("triggers %+v", m.Triggers)
	}
}

// TestATriggerFiresOnlyWhatItTookOut: a trigger runs only once it is out
// of the run's list, so a failed write fires nothing, not the same
// trigger at every sample.
func TestATriggerFiresOnlyWhatItTookOut(t *testing.T) {
	s, ran, clock := whenRun(t, false)
	mustAct(t, s, whenAct(`{"ship":4573,"stage":"merged","do":"send","who":"work","text":"rebase on main"}`))
	setShip(s, 4573, ships.StageMerged, ships.Done, "")
	dir := s.Stoker.Dir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	for range 3 {
		*clock = clock.Add(time.Minute)
		s.fire(t.Context(), *clock)
	}
	if got := typed(*ran, "%1"); len(got) != 0 {
		t.Fatalf("a trigger it could not take out fired: %q", got)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		*clock = clock.Add(time.Minute)
		s.fire(t.Context(), *clock)
	}
	if got := typed(*ran, "%1"); !slices.Equal(got, []string{"rebase on main"}) {
		t.Fatalf("work was told %q", got)
	}
}

// TestTriggersEndWithTheDayWhenANightStarts: n over a day run is a new
// order, and the day's triggers end with the day's.
func TestTriggersEndWithTheDayWhenANightStarts(t *testing.T) {
	s, ran, clock := whenRun(t, false)
	mustAct(t, s, whenAct(`{"ship":4573,"stage":"merged","do":"send","who":"work","text":"rebase on main"}`))
	mustAct(t, s, orderAct(t, "the night", true))
	if m := s.Stoker.Mode(); len(m.Triggers) != 0 || m.NextTrigger != 0 || !m.Nightly {
		t.Fatalf("the night kept the day's triggers: %+v", m)
	}
	setShip(s, 4573, ships.StageMerged, ships.Done, "")
	*clock = clock.Add(time.Minute)
	s.stoke(t.Context(), *clock)
	if got := typed(*ran, "%1"); len(got) != 0 {
		t.Fatalf("the day's trigger fired in the night: %q", got)
	}
}

// TestAHoldTriggersWhyIsAHoldsWhy: a hold trigger's why has the cap and
// the cleaning of hold's own: no longer than a line may be, and no
// control keys.
func TestAHoldTriggersWhyIsAHoldsWhy(t *testing.T) {
	s, _, _ := whenRun(t, false)
	long := strings.Repeat("x", 501)
	want := "the why is longer than 500 characters"
	if res := s.act(t.Context(), whenAct(`{"rest":"work","do":"hold","who":"work","text":"`+long+`"}`)); res.Err != want {
		t.Fatalf("a long why on a trigger: %q", res.Err)
	}
	if res := s.act(t.Context(), fromStokerAct("hold:%1", long)); res.Err != want {
		t.Fatalf("a long why on hold: %q", res.Err)
	}
	mustAct(t, s, whenAct(`{"rest":"work","do":"hold","who":"work","text":"wait\u001b[2Jnow"}`))
	if m := s.Stoker.Mode(); len(m.Triggers) != 1 || m.Triggers[0].Text != "wait [2Jnow" {
		t.Fatalf("triggers %+v", m.Triggers)
	}
}
