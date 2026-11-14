package app

import (
	"slices"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// loginChain is kindle-web (idle), flint-auth (needs you) and wick-mobile
// (works) in one chain, in that team order. edit changes the state first.
func loginChain(t *testing.T, edit func(*proto.State, map[string]*sample.Session)) Model {
	t.Helper()
	ss := map[string]*sample.Session{}
	var list []sample.Session
	for _, n := range []string{"kindle-web", "flint-auth", "wick-mobile"} {
		s := boardMatch(n, time.Hour)
		list = append(list, s)
	}
	for i := range list {
		ss[list[i].Name] = &list[i]
	}
	st := boardState()
	st.Queue = append(st.Queue, waitFor(list[1], queue.StatePermission, 4*time.Minute))
	ss["wick-mobile"].Busy, ss["wick-mobile"].Status, ss["wick-mobile"].Idle = true, "busy", 0
	if edit != nil {
		edit(&st, ss)
	}
	st.Sessions = list
	st.Chains = &state.ChainsView{Chains: []chains.Chain{{ID: "c1", Name: "login", Repo: "/home/user/src/hearth"}}}
	for _, s := range list {
		st.Chains.Chains[0].Members = append(st.Chains.Chains[0].Members, memberOf(s))
	}
	return boardModel(t, st)
}

func groupOfState(gs []sideGroup, h sideState) (sideGroup, bool) {
	for _, g := range gs {
		if g.state == h {
			return g, true
		}
	}
	return sideGroup{}, false
}

func unitNames(u sideUnit) []string {
	var out []string
	for _, s := range u.members {
		out = append(out, s.Name)
	}
	return out
}

func TestEmptyGroupHasNoEntry(t *testing.T) {
	s := boardMatch("hearth-api", time.Hour)
	st := boardState(s)
	gs := boardModel(t, st).sideGroups()
	if _, ok := groupOfState(gs, sideNeed); ok {
		t.Fatalf("NEEDS YOU has an entry with no session that needs you: %v", gs)
	}
	if g, ok := groupOfState(gs, sideIdle); !ok || g.count != 1 {
		t.Fatalf("IDLE %v, want one session", gs)
	}
}

func TestChainSitsWithItsMostUrgentMember(t *testing.T) {
	m := loginChain(t, nil)
	gs := m.sideGroups()
	need, ok := groupOfState(gs, sideNeed)
	if !ok || len(need.units) != 1 || need.count != 1 {
		t.Fatalf("NEEDS YOU %+v, want the chain as one unit and one count", need)
	}
	u := need.units[0]
	if got, want := unitNames(u), []string{"kindle-web", "flint-auth", "wick-mobile"}; !slices.Equal(got, want) {
		t.Fatalf("members %v, want team order %v", got, want)
	}
	if u.urgent().Name != "flint-auth" {
		t.Fatalf("urgent %s, want flint-auth", u.urgent().Name)
	}
	if n, of := u.step(); n != 2 || of != 3 {
		t.Fatalf("step %d of %d, want 2 of 3", n, of)
	}
	if work, ok := groupOfState(gs, sideWork); ok {
		t.Fatalf("WORKING %+v counts wick-mobile, which sits in a chain that needs you", work)
	}
}

func TestChainMovesWhenItsWaitIsAnswered(t *testing.T) {
	m := loginChain(t, func(st *proto.State, ss map[string]*sample.Session) {
		st.Queue = nil
		s := ss["flint-auth"]
		s.Busy, s.Status, s.Idle = true, "busy", 0
	})
	gs := m.sideGroups()
	if _, ok := groupOfState(gs, sideNeed); ok {
		t.Fatalf("NEEDS YOU still has an entry: %+v", gs)
	}
	work, ok := groupOfState(gs, sideWork)
	if !ok || len(work.units) != 1 || work.count != 2 {
		t.Fatalf("WORKING %+v, want the chain as one unit and a count of 2", work)
	}
}

func TestOrderInAGroupHoldsWhileStatesHold(t *testing.T) {
	build := func() Model {
		var ss []sample.Session
		st := boardState()
		for i, n := range []string{"hearth-api", "spark-docs", "cinder-ci", "ember-billing"} {
			s := boardMatch(n, time.Hour)
			s.Busy, s.Status, s.Idle = true, "busy", 0
			s.Started = boardAt.Add(-time.Duration(4-i) * time.Hour)
			ss = append(ss, s)
		}
		st.Sessions = []sample.Session{ss[2], ss[0], ss[3], ss[1]}
		return boardModel(t, st)
	}
	a, b := build().sideOrder(), build().sideOrder()
	if !slices.Equal(a, b) {
		t.Fatalf("two refreshes order\n%v\n%v", a, b)
	}
	var names []string
	for _, p := range a {
		names = append(names, p.pane)
	}
	if want := []string{"%hearth-api", "%spark-docs", "%cinder-ci", "%ember-billing"}; !slices.Equal(names, want) {
		t.Fatalf("order %v, want the oldest start first %v", names, want)
	}
}

func TestChainTakesItsUrgentMembersPlace(t *testing.T) {
	// flint-auth waited 4 minutes; spark-docs and cinder-ci, outside the
	// chain, waited 10 and 1. The chain sorts where flint-auth sorts alone.
	st := boardState()
	var list []sample.Session
	for _, n := range []string{"kindle-web", "flint-auth", "wick-mobile", "spark-docs", "cinder-ci"} {
		list = append(list, boardMatch(n, time.Hour))
	}
	st.Queue = []queue.Item{
		waitFor(list[3], queue.StatePermission, 10*time.Minute),
		waitFor(list[1], queue.StatePermission, 4*time.Minute),
		waitFor(list[4], queue.StatePermission, time.Minute),
	}
	st.Sessions = list
	st.Chains = &state.ChainsView{Chains: []chains.Chain{{ID: "c1", Name: "login", Repo: "/home/user/src/hearth"}}}
	for _, s := range list[:3] {
		st.Chains.Chains[0].Members = append(st.Chains.Chains[0].Members, memberOf(s))
	}
	need, _ := groupOfState(boardModel(t, st).sideGroups(), sideNeed)
	var got []string
	for _, u := range need.units {
		got = append(got, u.urgent().Name)
	}
	if want := []string{"spark-docs", "flint-auth", "cinder-ci"}; !slices.Equal(got, want) {
		t.Fatalf("units sort by %v, want %v", got, want)
	}
}

func TestShipOnlyCardIsNotInTheList(t *testing.T) {
	s := boardMatch("hearth-api", time.Hour)
	st := boardState(s)
	st.Ships.Ships = append(st.Ships.Ships, proto.Ship{Repo: "hearth/hearth-api", Number: 4570, Kind: shipWaits,
		Note: "green · waits for your merge", Stage: 2, Since: boardAt.Add(-time.Hour)})
	m := boardModel(t, st)
	if !slices.ContainsFunc(m.shipCards(), isShipCard) {
		t.Fatal("the fixture has no ship-only card")
	}
	for _, g := range m.sideGroups() {
		for _, u := range g.units {
			for _, s := range u.members {
				if isShipCard(s) {
					t.Fatalf("%s is a ship-only card in the list", s.Name)
				}
			}
		}
	}
}
