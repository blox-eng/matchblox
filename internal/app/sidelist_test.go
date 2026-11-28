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
	return boardModel(t, loginState(edit))
}

// loginState is the state of loginChain.
func loginState(edit func(*proto.State, map[string]*sample.Session)) proto.State {
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
	return st
}

// waitingLogin is loginChain with the named members gone from the host:
// each waits for recover.
func waitingLogin(t *testing.T, names ...string) Model {
	t.Helper()
	st := loginState(nil)
	st.Sessions = slices.DeleteFunc(st.Sessions, func(s sample.Session) bool {
		if !slices.Contains(names, s.Name) {
			return false
		}
		st.Chains.Mates = append(st.Chains.Mates, state.MateView{Member: memberOf(s), Wait: &state.WaitView{Name: s.Name, Agent: "claude"}})
		return true
	})
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
	if g, ok := groupOfState(gs, sideDone); !ok || g.count != 1 {
		t.Fatalf("DONE %v, want one session", g)
	}
	if _, ok := groupOfState(gs, sideIdle); ok {
		t.Fatalf("IDLE has an entry for a quiet turn: %v", gs)
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
	done, ok := groupOfState(gs, sideDone)
	if !ok || len(done.units) != 1 || done.count != 1 {
		t.Fatalf("DONE %+v, want the chain as one unit and kindle-web counted", done)
	}
}

// A session in two chains is one row: the list walks by session, so a
// second row of it stopped ↓ and ↑ there (owner, 2026-11-16).
func TestASessionInTwoChainsIsOneRow(t *testing.T) {
	var list []sample.Session
	for _, n := range []string{"kindle-web", "flint-auth", "ember-billing"} {
		list = append(list, boardMatch(n, time.Hour))
	}
	st := boardState()
	st.Sessions = list
	st.Chains = &state.ChainsView{Chains: []chains.Chain{
		{ID: "c1", Name: "login", Repo: "/home/user/src/hearth", Members: []chains.Member{memberOf(list[0]), memberOf(list[1])}},
		{ID: "c2", Name: "billing", Repo: "/home/user/src/hearth", Members: []chains.Member{memberOf(list[1]), memberOf(list[2])}},
	}}
	m := boardModel(t, st)
	walk := m.sideWalk()
	if len(walk) != 3 {
		t.Fatalf("the list has %d rows for 3 sessions: %v", len(walk), walk)
	}
	m.sideAt = walk[0]
	seen := []paneKey{m.chosenSide()}
	for range 4 {
		m.sideMove(1)
		if c := m.chosenSide(); c != seen[len(seen)-1] {
			seen = append(seen, c)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("↓ from the top reached %v, want all 3 sessions", seen)
	}
}

func TestOrderInAGroupHoldsWhileStatesHold(t *testing.T) {
	build := func(order ...int) Model {
		var ss []sample.Session
		st := boardState()
		for i, n := range []string{"hearth-api", "spark-docs", "cinder-ci", "ember-billing"} {
			s := boardMatch(n, time.Hour)
			s.Busy, s.Status, s.Idle = true, "busy", 0
			s.Started = boardAt.Add(-time.Duration(4-i) * time.Hour)
			ss = append(ss, s)
		}
		for _, i := range order {
			st.Sessions = append(st.Sessions, ss[i])
		}
		return boardModel(t, st)
	}
	a, b := build(2, 0, 3, 1).sideOrder(), build(3, 1, 0, 2).sideOrder()
	if !slices.Equal(a, b) {
		t.Fatalf("two refreshes with the input in another order\n%v\n%v", a, b)
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

func TestChainKeepsAMemberTheTeamDoesNotList(t *testing.T) {
	// spark-docs shares flint-auth's session id: the team names one of them,
	// and the chain still holds both.
	st := boardState()
	var list []sample.Session
	for _, n := range []string{"kindle-web", "flint-auth", "spark-docs"} {
		list = append(list, boardMatch(n, time.Hour))
	}
	list[2].SessionID = list[1].SessionID
	st.Queue = []queue.Item{waitFor(list[2], queue.StatePermission, time.Minute)}
	st.Sessions = list
	st.Chains = &state.ChainsView{Chains: []chains.Chain{{ID: "c1", Name: "login", Repo: "/home/user/src/hearth"}}}
	for _, s := range list[:2] {
		st.Chains.Chains[0].Members = append(st.Chains.Chains[0].Members, memberOf(s))
	}
	m := boardModel(t, st)
	need, ok := groupOfState(m.sideGroups(), sideNeed)
	if !ok || len(need.units) != 1 {
		t.Fatalf("NEEDS YOU %+v, want the chain with the session that asks", need)
	}
	if got, want := unitNames(need.units[0]), []string{"kindle-web", "flint-auth", "spark-docs"}; !slices.Equal(got, want) {
		t.Fatalf("members %v, want %v", got, want)
	}
}
