package service

import (
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/stoker"
)

func TestFamilyIsHostOnlyWithoutChains(t *testing.T) {
	s, _, _ := stokerTest(t)
	if err := s.loadFamily(); err != nil {
		t.Fatal(err)
	}
	fam := s.family()
	if len(fam) != 1 || fam[0].id != stoker.HostID || fam[0].store != s.Stoker {
		t.Fatalf("the family is not the host alone: %+v", fam)
	}
	if s.unitByID(stoker.HostID) != fam[0] || s.hostUnit() != fam[0] {
		t.Fatal("the host unit is not one unit")
	}
	s.Stoker = nil
	if fam := s.family(); len(fam) != 0 || s.hostUnit() != nil {
		t.Fatalf("a service with no stoker has a family: %+v", fam)
	}
}

func TestFamilyReloadsAfterRestart(t *testing.T) {
	s, _, _ := stokerTest(t)
	root := s.Stoker.Dir()
	dir := stoker.FamilyDir(root, "s1")
	if err := stoker.WriteLink(dir, stoker.Link{ID: "s1", Chain: "c1", Giver: "person"}); err != nil {
		t.Fatal(err)
	}
	st, err := stoker.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Set(stoker.Mode{}.TurnOn(night0)); err != nil {
		t.Fatal(err)
	}

	again := newTest(t, time.Hour) // the service restarts
	if again.Stoker, err = stoker.Open(root); err != nil {
		t.Fatal(err)
	}
	if err := again.loadFamily(); err != nil {
		t.Fatal(err)
	}
	u := again.unitByID("s1")
	if u == nil || !u.store.Mode().On || u.link.Chain != "c1" || u.store.Dir() != dir {
		t.Fatalf("the chain unit did not come back: %+v", u)
	}
	if fam := again.family(); len(fam) != 2 || fam[0].id != stoker.HostID || fam[1] != u {
		t.Fatalf("the family is not host, then s1: %+v", fam)
	}
	if again.unitOfChain("c1") != u || again.unitOfChain("c2") != nil {
		t.Fatal("the unit of chain c1 is not s1")
	}
}

func TestUnknownStokerSessionFailsClosed(t *testing.T) {
	s, ran := handsTest(t)
	s.publish(sample.Snapshot{At: night0, Sessions: []sample.Session{
		{Agent: "claude", Pane: "%1", Target: "app:1.1", SessionID: "s1", Name: "work", Status: "idle", Idle: 5 * time.Minute, Tokens: 176000, ContextPct: 88},
		{Agent: "claude", Pane: "%9@mbtest", Target: "stoker:1.1", SessionID: "s9", Name: "stoker", Status: "busy", Busy: true},
		{Agent: "claude", Pane: "%8@mbtest", Target: "stoker-s9:1.1", SessionID: "s8", Name: "stoker-s9", Status: "idle"},
	}}, func(state.Doc) {})
	if u, on := s.unitOf(proto.Act{From: "%9@mbtest"}); !on || u != s.hostUnit() {
		t.Fatalf("the host's pane is not the host's: %+v %v", u, on)
	}
	if u, on := s.unitOf(proto.Act{From: "%8@mbtest"}); !on || u != nil {
		t.Fatalf("an unknown stoker session has a unit: %+v %v", u, on)
	}
	for _, a := range []proto.Act{
		{RecID: "message:%1", Text: "rebase on main"},
		{RecID: "stoker:plan", Text: `{"steps":[{"n":1,"session":"work","text":"x"}]}`},
		{RecID: "stoker:step", Text: `{"step":1,"state":"doing","text":"x"}`},
	} {
		a.Confirm, a.From, a.FromPlaced = "y", "%8@mbtest", true
		if res := s.act(t.Context(), a); res.Err != "no agent acts on the stoker's own session" {
			t.Fatalf("%s: %q", a.RecID, res.Err)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

// A stoker pane the sample does not name yet is the host's while the host
// is the whole family, and fails closed once a chain unit runs.
func TestAnUnsampledStokerPaneIsTheHostsAlone(t *testing.T) {
	s, _ := handsTest(t)
	if u, on := s.unitOf(proto.Act{From: "%7@mbtest"}); !on || u != s.hostUnit() {
		t.Fatalf("an unsampled stoker pane is not the host's: %+v %v", u, on)
	}
	dir := stoker.FamilyDir(s.Stoker.Dir(), "s1")
	if err := stoker.WriteLink(dir, stoker.Link{ID: "s1", Chain: "c1", Giver: "person"}); err != nil {
		t.Fatal(err)
	}
	if err := s.loadFamily(); err != nil {
		t.Fatal(err)
	}
	if u, on := s.unitOf(proto.Act{From: "%7@mbtest"}); !on || u != nil {
		t.Fatalf("with a chain unit, an unsampled stoker pane has a unit: %+v %v", u, on)
	}
	s.publish(sample.Snapshot{At: night0, Panes: []sample.Pane{{ID: "%7@mbtest", Target: "stoker-s1:1.1"}}}, func(state.Doc) {})
	if u, on := s.unitOf(proto.Act{From: "%7@mbtest"}); !on || u == nil || u.id != "s1" {
		t.Fatalf("the pane of stoker-s1 is not s1's: %+v %v", u, on)
	}
}
