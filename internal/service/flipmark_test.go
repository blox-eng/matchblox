package service

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/mux/muxtest"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// TestServiceMarksTheAgentPanesItSamples: after a sample, each agent pane
// has the flip's mark, a pane whose agent left loses it, and a stoker's
// pane never has it.
func TestServiceMarksTheAgentPanesItSamples(t *testing.T) {
	s := newTest(t, time.Hour)
	s.StokerServer, s.MarkMatches = "matchblox", true
	fake := &muxtest.Fake{}
	s.Mux = fake
	pub := func(ss ...sample.Session) {
		s.publish(sample.Snapshot{At: time.Now(), Sessions: ss}, func(state.Doc) {})
		s.headRun.wait()
	}
	pub(sample.Session{Pane: "%1", PID: 11, Agent: "claude"}, sample.Session{Pane: "%2", PID: 12, Agent: "codex"},
		sample.Session{Pane: "%9@matchblox", PID: 19, Agent: "claude"})
	if got := slices.Sorted(maps.Keys(fake.Marked())); !slices.Equal(got, []string{"%1", "%2"}) {
		t.Fatalf("marked %v, want %%1 %%2", got)
	}
	pub(sample.Session{Pane: "%1", PID: 11, Agent: "claude"})
	if got := slices.Sorted(maps.Keys(fake.Marked())); !slices.Equal(got, []string{"%1"}) {
		t.Fatalf("marked %v after %%2's agent left", got)
	}
	if n := fake.MarkCalls(); n != 3 {
		t.Fatalf("%d mark calls, want 3: a pane marked once is not marked again", n)
	}
}

// TestAFixtureServiceMarksNoPane: a fixture's pane ids are this machine's
// own panes.
func TestAFixtureServiceMarksNoPane(t *testing.T) {
	s := newTest(t, time.Hour)
	fake := &muxtest.Fake{}
	s.Mux = fake
	s.publish(sample.Snapshot{At: time.Now(), Sessions: []sample.Session{{Pane: "%1", PID: 11, Agent: "claude"}}}, func(state.Doc) {})
	s.headRun.wait()
	if n := fake.MarkCalls(); n != 0 {
		t.Fatalf("a fixture service marked %d panes", n)
	}
}
