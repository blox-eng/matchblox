package stoker

import (
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/ships"
)

func TestATriggerNamesOneEventAndOneHand(t *testing.T) {
	ok := Trigger{Ship: 4573, Stage: "merged", Do: "send", Who: "Billed", Pane: "%2", Text: "rebase on main"}
	if err := ok.Check(); err != nil {
		t.Fatal(err)
	}
	if got := ok.Event() + " → " + ok.Done(); got != "#4573 merged → sent Billed: rebase on main" {
		t.Fatalf("words %q", got)
	}
	for name, bad := range map[string]Trigger{
		"no event":   {Do: "compact", Who: "w"},
		"two events": {Ship: 1, Stage: "green", Rest: "w", Do: "compact", Who: "w"},
		"stage":      {Ship: 1, Stage: "approved", Do: "compact", Who: "w"},
		"lone stage": {Rest: "w", Stage: "green", Do: "compact", Who: "w"},
		"hand":       {Ship: 1, Stage: "green", Do: "merge", Who: "w"},
		"no text":    {Ship: 1, Stage: "green", Do: "send", Who: "w"},
		"no why":     {Rest: "w", Do: "hold", Who: "w"},
		"no session": {Rest: "w", Do: "release"},
	} {
		if bad.Check() == nil {
			t.Errorf("%s: %+v is taken", name, bad)
		}
	}
}

func TestATriggerIsDueAtItsEvent(t *testing.T) {
	at := time.Now()
	rep := func(stage int, kind, failed string) *ships.Report {
		return &ships.Report{Ships: []ships.Ship{{Number: 7, Stage: stage, Kind: kind, Failed: failed}}}
	}
	merged := Trigger{Ship: 7, Stage: "merged", At: at}
	switch {
	case merged.Due(nil, nil, nil), merged.Due(rep(ships.StageGreen, ships.WaitsForYou, ""), nil, nil):
		t.Fatal("merged before the merge")
	case !merged.Due(rep(ships.StageMerged, ships.Done, ""), nil, nil), !merged.Due(rep(ships.StageLive, ships.Done, ""), nil, nil):
		t.Fatal("not due once merged")
	}
	red := Trigger{Ship: 7, Stage: "red", At: at}
	if red.Due(rep(ships.StageCI, ships.Running, ""), nil, nil) || !red.Due(rep(ships.StageCI, ships.Stuck, "test"), nil, nil) {
		t.Fatal("red is CI failed")
	}

	rest := Trigger{Rest: "work", RestPane: "%1", At: at}
	s := func(ended time.Time, busy bool) []sample.Session {
		return []sample.Session{{Agent: "claude", Pane: "%1", Name: "work", Status: "idle", TurnEnded: ended, Busy: busy}}
	}
	switch {
	case rest.Due(nil, s(at.Add(-time.Minute), false), nil):
		t.Fatal("a turn that ended before the trigger fired it")
	case rest.Due(nil, s(at.Add(time.Minute), true), nil):
		t.Fatal("a busy session rests")
	case rest.Due(nil, s(at.Add(time.Minute), false), map[string]bool{"%1": true}):
		t.Fatal("a session that waits for you rests")
	case !rest.Due(nil, s(at.Add(time.Minute), false), nil):
		t.Fatal("the turn's end did not fire it")
	}
}

func TestTriggersEndWithTheRun(t *testing.T) {
	m := Mode{}.TurnOn(time.Now())
	m.Triggers = []Trigger{{ID: "t1", Rest: "w", Do: "compact", Who: "w"}}
	if off := m.Off(time.Now()); len(off.Triggers) != 0 {
		t.Fatalf("off kept %+v", off.Triggers)
	}
}
