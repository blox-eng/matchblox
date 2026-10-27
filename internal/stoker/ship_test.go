package stoker

import (
	"testing"

	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/ships"
)

// The brief names the ship a session's row leads with: a stuck one before
// one that waits for your merge, whatever the report's order.
func TestTheBriefTakesTheShipTheRowLeadsWith(t *testing.T) {
	rep := &ships.Report{Ships: []ships.Ship{
		{Number: 7, Kind: ships.WaitsForYou, Stage: ships.StageGreen, Session: ships.SessionRef{Pane: "%1"}},
		{Number: 9, Kind: ships.Stuck, Stage: ships.StageCI, Session: ships.SessionRef{Pane: "%1"}},
	}}
	if sh := shipOfPane(rep, sample.Session{Pane: "%1", SessionID: "s1"}); sh == nil || sh.Number != 9 {
		t.Fatalf("got %+v, want the stuck #9", sh)
	}
}
