package service

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// rebootPair is one and two on the builder's server "100", chained, with
// the recovery record holding both conversations (#391).
func rebootPair(t *testing.T) (s *Service, one, two sample.Session) {
	t.Helper()
	one = sample.Session{Pane: "%1", PID: 101, Start: 1, SessionID: "s1", Name: "one", Agent: "claude", Target: "one:1.0", Cwd: "/src/one"}
	two = sample.Session{Pane: "%2", PID: 102, Start: 2, SessionID: "s2", Name: "two", Agent: "claude", Target: "two:1.0", Cwd: "/src/two"}
	s, _ = recoverTest(t, one, two)
	s.mu.Lock()
	s.ChainsPath, s.Chains = filepath.Join(t.TempDir(), "chains.json"), nil
	s.mu.Unlock()
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"%2"); res.Err != "" {
		t.Fatalf("chain = %+v", res)
	}
	publishOn(s, "100", one, two)
	chainInbox(s, one) // the join notices
	chainInbox(s, two)
	return s, one, two
}

// reopened is x reopened by recover on the server "200", in pane.
func reopened(x sample.Session, pane string, pid int) sample.Session {
	x.Pane, x.PID, x.Start = pane, pid, uint64(pid)
	return x
}

func teamOf(t *testing.T, s *Service) []string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.chainsLocked()
	if len(st.Chains) != 1 {
		t.Fatalf("chains = %+v", st.Chains)
	}
	var panes []string
	for _, m := range st.Chains[0].Members {
		panes = append(panes, m.Pane)
	}
	return panes
}

// TestAChainOutlivesALostServer: a reboot takes every pane, and no tmux
// server runs until the builder starts one. recover then reopens each
// conversation in a new pane, with the same session id: the team is back,
// on the new panes, and nobody heard that a mate left.
func TestAChainOutlivesALostServer(t *testing.T) {
	s, one, two := rebootPair(t)
	s.publish(sample.Snapshot{At: time.Now()}, func(state.Doc) {}) // no tmux server at all
	loseServer(s)                                                  // a new server, with a shell only
	// The new server hands out the old pane ids: %1 is now another agent.
	other := sample.Session{Pane: "%1", PID: 300, Start: 300, SessionID: "x", Name: "other", Agent: "claude", Target: "other:1.0", Cwd: "/src/other"}
	publishOn(s, "200", other)
	if got := teamOf(t, s); len(got) != 2 {
		t.Fatalf("after the loss, the team = %q", got)
	}
	one2, two2 := reopened(one, "%5", 201), reopened(two, "%6", 202)
	publishOn(s, "200", other, one2, two2)
	if got := teamOf(t, s); !slices.Equal(got, []string{"%5", "%6"}) {
		t.Fatalf("after recover, the team's panes = %q", got)
	}
	for _, x := range []sample.Session{one2, two2} {
		if got := chainInbox(s, x); len(got) != 0 {
			t.Fatalf("%s heard %q", x.Name, got)
		}
	}
}

// TestAForgottenConversationLeavesItsTeam: after the loss, a member whose
// conversation the record no longer holds is gone, and its mates hear it.
func TestAForgottenConversationLeavesItsTeam(t *testing.T) {
	s, one, _ := rebootPair(t)
	loseServer(s)
	one2 := reopened(one, "%5", 201)
	publishOn(s, "200", one2)
	if got := teamOf(t, s); len(got) != 2 {
		t.Fatalf("with two still in the record, the team = %q", got)
	}
	// Two's entry expires: recover can no longer bring it back.
	live, alive := recordable([]sample.Session{one2}, []sample.Pane{{ID: "%5", Socket: recSock, Server: "200", ServerStart: "1200"}})
	if err := s.Recovery.Update(live, alive, time.Now().Add(15*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	publishOn(s, "200", one2)
	if got := chainInbox(s, one2); !slices.Equal(got, []string{"two left the chain"}) {
		t.Fatalf("one's inbox = %q", got)
	}
}

// TestAMatchThatQuitsOnALiveServerLeavesAtOnce: the record still holds a
// conversation whose server runs, but that is a quit, not a loss.
func TestAMatchThatQuitsOnALiveServerLeavesAtOnce(t *testing.T) {
	s, one, _ := rebootPair(t)
	publishOn(s, "100", one)
	if got := chainInbox(s, one); !slices.Equal(got, []string{"two left the chain"}) {
		t.Fatalf("one's inbox = %q", got)
	}
}

// forget expires the record's entry of each conversation but keep's: the
// record no longer holds them, so recover cannot bring them back.
func forget(t *testing.T, s *Service, keep ...sample.Session) {
	t.Helper()
	var panes []sample.Pane
	for _, x := range keep {
		panes = append(panes, sample.Pane{ID: x.Pane, Socket: recSock, Server: "200", ServerStart: "1200"})
	}
	live, alive := recordable(keep, panes)
	if err := s.Recovery.Update(live, alive, time.Now().Add(15*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
}

// TestAForgottenMateIsNamedAfterARestart: a reboot restarts the service
// too, so nothing it held in memory names the waiting member; its mates
// still hear its name when the record forgets it.
func TestAForgottenMateIsNamedAfterARestart(t *testing.T) {
	s, one, _ := rebootPair(t)
	loseServer(s)
	s.mu.Lock()
	s.mateNames, s.waitNames = nil, nil // what a restart forgets
	s.mu.Unlock()
	one2 := reopened(one, "%5", 201)
	publishOn(s, "200", one2)
	forget(t, s, one2)
	publishOn(s, "200", one2)
	if got := chainInbox(s, one2); !slices.Equal(got, []string{"two left the chain"}) {
		t.Fatalf("one's inbox = %q", got)
	}
}

// TestAReusedPaneDoesNotRenameAForgottenMate: the new server gives one's
// old pane id to another chained agent; when the record forgets one, its
// mate hears one's name, not the other agent's.
func TestAReusedPaneDoesNotRenameAForgottenMate(t *testing.T) {
	s, _, two := rebootPair(t)
	loseServer(s)
	other := sample.Session{Pane: "%1", PID: 300, Start: 300, SessionID: "x", Name: "other", Agent: "claude", Target: "other:1.0", Cwd: "/src/other"}
	third := sample.Session{Pane: "%7", PID: 301, Start: 301, SessionID: "y", Name: "third", Agent: "claude", Target: "third:1.0", Cwd: "/src/third"}
	two2 := reopened(two, "%6", 202)
	publishOn(s, "200", other, third, two2)
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"%7"); res.Err != "" {
		t.Fatalf("chain = %+v", res)
	}
	publishOn(s, "200", other, third, two2)
	chainInbox(s, two2)
	forget(t, s, other, third, two2)
	publishOn(s, "200", other, third, two2)
	if got := chainInbox(s, two2); !slices.Equal(got, []string{"one left the chain"}) {
		t.Fatalf("two's inbox = %q", got)
	}
}

// TestTakeOutPicksTheLiveMemberAtAReusedPane: a waiting member and a
// reopened one share a pane id; out of team takes the live one.
func TestTakeOutPicksTheLiveMemberAtAReusedPane(t *testing.T) {
	s, _, two := rebootPair(t)
	loseServer(s)
	two2 := reopened(two, "%1", 202) // one's old pane id
	publishOn(s, "200", two2)
	res := s.act(context.Background(), proto.Act{RecID: proto.ChainPrefix + "out:%1@one", Console: true})
	if res.Err != "" || res.Note != "took two out of one" {
		t.Fatalf("out = %+v", res)
	}
	if got := chainInbox(s, two2); !slices.Contains(got, "[matchblox] the owner took you out of one") {
		t.Fatalf("two's inbox = %q", got)
	}
}

// TestASwitchCutByALostServerTakesNoNewAgent: one's agent switch was under
// way when its server died. The new server gives one's old pane id to
// another agent: that agent is not one's new agent, and joins no team.
func TestASwitchCutByALostServerTakesNoNewAgent(t *testing.T) {
	s, one, _ := rebootPair(t)
	s.mu.Lock()
	s.chainsLocked().Move(memberOf(one), time.Now())
	s.mu.Unlock()
	loseServer(s)
	other := sample.Session{Pane: "%1", PID: 300, Start: 300, SessionID: "x", Name: "other", Agent: "claude", Target: "other:1.0", Cwd: "/src/other"}
	publishOn(s, "200", other)
	publishOn(s, "200", other)
	s.mu.Lock()
	defer s.mu.Unlock()
	if teams := s.chainsLocked().TeamsOf(memberOf(other)); len(teams) != 0 {
		t.Fatalf("other took one's place in %+v", teams)
	}
	if teams := s.chainsLocked().TeamsOf(memberOf(one)); len(teams) != 1 {
		t.Fatalf("one, waiting for recover, is in %+v", teams)
	}
}

// TestAWaitingMateKeepsTheNameItsMatesKnew: a match named by its window is
// announced by that name, not by its tmux session's.
func TestAWaitingMateKeepsTheNameItsMatesKnew(t *testing.T) {
	s, one, two := rebootPair(t)
	two.Name = "reviewer" // its window's name; its tmux session is "two"
	publishOn(s, "100", one, two)
	loseServer(s)
	one2 := reopened(one, "%5", 201)
	publishOn(s, "200", one2)
	forget(t, s, one2)
	publishOn(s, "200", one2)
	if got := chainInbox(s, one2); !slices.Equal(got, []string{"reviewer left the chain"}) {
		t.Fatalf("one's inbox = %q", got)
	}
}
