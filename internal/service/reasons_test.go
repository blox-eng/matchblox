package service

import (
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/ships"
	"github.com/blox-eng/matchblox/internal/state"
)

func TestEverySessionAtRestSaysWhyItIsIdle(t *testing.T) {
	s := newTest(t, time.Hour)
	quiet := func(pane, id string) sample.Session {
		return sample.Session{Pane: pane, SessionID: id, Name: id, Status: "idle", Idle: 30 * time.Hour}
	}
	busy := quiet("%2", "b")
	busy.Busy, busy.Status, busy.Idle = true, "busy", 0
	failed := quiet("%3", "c")
	failed.Failed = "API Error: 529"
	byID := quiet("%4", "d")
	s.publish(sample.Snapshot{At: time.Now(), Sessions: []sample.Session{quiet("%1", "a"), busy, failed, byID}}, func(state.Doc) {})

	s.mu.Lock()
	s.cur.Ships = &ships.Report{Ships: []proto.Ship{
		{Number: 7, Kind: ships.WaitsForYou, Note: ships.NoteWaitsForMerge, Session: proto.ShipSession{Pane: "%1"}},
		{Number: 8, Kind: ships.Done, Stage: ships.StageLive, Session: proto.ShipSession{ID: "d"}},
	}}
	s.setReasons()
	s.mu.Unlock()

	fresh := sample.Session{Pane: "%5", SessionID: "e", Status: "idle", Idle: time.Minute}
	asking := sample.Session{Pane: "%6", SessionID: "f", Status: "idle", Idle: time.Minute, Permits: true}
	s.publish(sample.Snapshot{At: time.Now(), Sessions: []sample.Session{quiet("%1", "a"), busy, failed, byID, fresh, asking}}, func(state.Doc) {})
	r := s.State().Reasons
	for pane, want := range map[string]string{"%1": "#7 waits your merge", "%3": "stuck: API Error: 529", "%4": "#8 merged · done", "%5": "out of work"} {
		if r[pane].Word != want {
			t.Errorf("%s: %+v, want %q", pane, r[pane], want)
		}
	}
	for _, pane := range []string{"%2", "%6"} { // working; asking permission
		if _, ok := r[pane]; ok {
			t.Errorf("%s is not quiet: %+v", pane, r[pane])
		}
	}
}
