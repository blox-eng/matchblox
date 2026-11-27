package app

import (
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

func TestSideStateOf(t *testing.T) {
	type edit = func(*proto.State, *sample.Session)
	merge := func(st *proto.State, s *sample.Session) {
		sh := shipFor(*s, 4556, shipWaits, "green · waits for your merge", 2, time.Hour)
		sh.Merge = []string{"gh", "pr", "merge", "4556"}
		st.Ships.Ships = append(st.Ships.Ships, sh)
	}
	for _, c := range []struct {
		name string
		set  []edit
		want sideState
		why  string
	}{
		{"flint-auth", []edit{func(st *proto.State, s *sample.Session) {
			st.Queue = append(st.Queue, waitFor(*s, queue.StatePermission, 4*time.Minute))
		}}, sideNeed, ""},
		{"hearth-api", []edit{func(_ *proto.State, s *sample.Session) { s.Busy, s.Status, s.Idle = true, "busy", 0 }}, sideWork, ""},
		{"kindle-web", []edit{func(st *proto.State, s *sample.Session) {
			st.Holds = append(st.Holds, proto.Hold{SessionID: s.SessionID, Pane: s.Pane, Name: s.Name, By: holdByYou})
		}}, sideHeld, "parked by you · z lets it go"},
		{"spark-docs", []edit{func(st *proto.State, s *sample.Session) {
			st.Holds = append(st.Holds, proto.Hold{SessionID: s.SessionID, Pane: s.Pane, Name: s.Name, By: "stoker", Why: "not the priority"})
		}}, sideHeld, "held by the stoker · not the priority"},
		{"spark-docs", []edit{func(st *proto.State, s *sample.Session) {
			st.Holds = append(st.Holds, proto.Hold{SessionID: s.SessionID, Pane: s.Pane, Name: s.Name, By: "stoker"})
			s.Busy, s.Status, s.Idle = true, "busy", 0
		}}, sideHeld, "held by the stoker"},
		{"cinder-ci", []edit{func(_ *proto.State, s *sample.Session) { s.Idle = 30 * time.Hour }}, sideIdle, ""},
		{"cinder-ci", []edit{func(_ *proto.State, s *sample.Session) { s.Status = "stopped" }}, sideDone, ""},
		{"ember-billing", []edit{merge}, sideNeed, ""},
		{"wick-mobile", []edit{func(st *proto.State, s *sample.Session) {
			st.Ships.Ships = append(st.Ships.Ships, shipFor(*s, 4556, shipRunning, "CI · 2 of 5 jobs", 1, time.Hour))
		}}, sideDone, ""},
	} {
		t.Run(c.name+"/"+c.want.head(), func(t *testing.T) {
			s := boardMatch(c.name, time.Hour)
			st := boardState()
			for _, f := range c.set {
				f(&st, &s)
			}
			st.Sessions = []sample.Session{s}
			m := boardModel(t, st)
			got := m.all.sessions[0]
			if state := m.sideStateOf(got); state != c.want {
				t.Fatalf("state %s, want %s", state.head(), c.want.head())
			}
			if why := m.heldWhy(got); why != c.why {
				t.Fatalf("heldWhy %q, want %q", why, c.why)
			}
		})
	}
}

func TestSideHeads(t *testing.T) {
	var got []string
	for h := sideNeed; h <= sideHeld; h++ {
		got = append(got, h.head())
	}
	want := []string{"NEEDS YOU", "DONE", "WORKING", "IDLE", "HELD"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("heads %v, want %v", got, want)
		}
	}
}

// TestTheListEndsInHeld: a session the builder parked sits below the idle
// ones, at the end of the list (the owner, 2026-11-20, #211).
func TestTheListEndsInHeld(t *testing.T) {
	idle, parked := boardMatch("cinder-ci", time.Hour), boardMatch("ember-billing", time.Hour)
	idle.Status = "stopped"
	st := boardState()
	st.Sessions = []sample.Session{parked, idle}
	st.Holds = []proto.Hold{{SessionID: parked.SessionID, Pane: parked.Pane, Name: parked.Name, By: holdByYou}}
	lines, _, _ := boardModel(t, st).sideList(80, 30)
	text := strings.Join(lines, "\n")
	i, h := strings.Index(text, "DONE"), strings.Index(text, "HELD")
	if i < 0 || h < 0 || h < i {
		t.Fatalf("DONE at %d, HELD at %d, want HELD last:\n%s", i, h, text)
	}
}
