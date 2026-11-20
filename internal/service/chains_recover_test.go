package service

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
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

// TestTheDocShowsAWaitingMate: the doc names a member that waits for
// recover, with its agent, its conversation and when the record forgets it.
// A live mate has no wait.
func TestTheDocShowsAWaitingMate(t *testing.T) {
	s, one, _ := rebootPair(t)
	loseServer(s)
	publishOn(s, "200", reopened(one, "%5", 201))
	s.mu.Lock()
	v := s.cur.Chains
	s.mu.Unlock()
	if v == nil || len(v.Mates) != 2 {
		t.Fatalf("chains = %+v", v)
	}
	var seen time.Time // when the record last saw two, on the lost server
	for _, e := range s.Recovery.List() {
		if e.ConversationID == "s2" {
			seen = e.Seen
		}
	}
	met := map[string]bool{}
	for _, mv := range v.Mates {
		met[mv.Member.SessionID] = true
		switch mv.Member.SessionID {
		case "s1":
			if mv.Wait != nil {
				t.Fatalf("one, live, waits: %+v", mv.Wait)
			}
		case "s2":
			w := mv.Wait
			if w == nil || w.Name != "two" || w.Agent != "claude" || w.Conversation != "s2" || seen.IsZero() || !w.Since.Equal(seen) || !w.Forget.Equal(seen.Add(14*24*time.Hour)) {
				t.Fatalf("two's wait = %+v, the record saw it at %v", w, seen)
			}
		}
	}
	if !met["s1"] || !met["s2"] {
		t.Fatalf("mates = %+v", v.Mates)
	}
}

// TestTakeOutAWaitingMateBySessionID: a waiting member's pane id is stale,
// so the console names it by its session id to take it out of its team.
func TestTakeOutAWaitingMateBySessionID(t *testing.T) {
	s, one, two := rebootPair(t)
	three := sample.Session{Pane: "%3", PID: 103, Start: 3, SessionID: "s3", Name: "three", Agent: "claude", Target: "three:1.0", Cwd: "/src/three"}
	publishOn(s, "100", one, two, three)
	if res := chainFrom(t, s, "%3", proto.ChainPrefix+"%1"); res.Err != "" {
		t.Fatalf("chain = %+v", res)
	}
	publishOn(s, "100", one, two, three)
	loseServer(s)
	one2, three2 := reopened(one, "%5", 201), reopened(three, "%7", 203)
	publishOn(s, "200", one2, three2)
	out := proto.Act{RecID: proto.ChainPrefix + "out:sid:s2@one"}
	if res := s.act(context.Background(), out); res.Err != "only the person at the console takes a match out of a chain" {
		t.Fatalf("a non-console out = %+v", res)
	}
	out.Console = true
	if res := s.act(context.Background(), out); res.Err != "" || res.Note != "took two out of one" {
		t.Fatalf("out = %+v", res)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.chainsLocked()
	if teams := st.TeamsOf(memberOf(two)); len(teams) != 0 {
		t.Fatalf("two is still in %+v", teams)
	}
	c, ok := st.Team("one")
	if !ok || len(c.Members) != 2 || !slices.ContainsFunc(c.Members, memberOf(one2).Same) || !slices.ContainsFunc(c.Members, memberOf(three2).Same) {
		t.Fatalf("team one = %+v", c)
	}
}

// TestALineToAWaitingMateWaits: a mate sends to a member that waits for
// recover by its session id. The line waits in its inbox, and the sender
// hears that it waits. The member gets it when recover reopens it.
func TestALineToAWaitingMateWaits(t *testing.T) {
	s, one, two := rebootPair(t)
	loseServer(s)
	one2 := reopened(one, "%5", 201)
	three := sample.Session{Pane: "%7", PID: 203, Start: 203, SessionID: "s3", Name: "three", Agent: "claude", Target: "three:1.0", Cwd: "/src/three"}
	publishOn(s, "200", one2, three)
	send := func(from, recID, text string) proto.Result {
		return s.act(context.Background(), proto.Act{RecID: recID, Confirm: "y", Text: text, From: from, FromKnown: true, FromPlaced: true})
	}
	bad := "the fix\nis on main"
	if res, want := send("%5", "message:sid:s2", bad), checkPeerText(bad).Error(); res.Err != want {
		t.Fatalf("a line with a newline = %+v, want %q", res, want)
	}
	if res := send("%7", "message:sid:s2", "hello"); res.Err != "you are not chained with two" {
		t.Fatalf("an unchained send = %+v", res)
	}
	if res := send("%5", "message:sid:nope", "hello"); res.Err != "no match waits for recover as nope" {
		t.Fatalf("a send to no waiting match = %+v", res)
	}
	if res := send("%5", "message:sid:s2", "the fix is on main"); res.Err != "" || res.Note != "two waits for recover: your line waits in its inbox (1 now)" {
		t.Fatalf("send = %+v", res)
	}
	two2 := reopened(two, "%6", 202)
	publishOn(s, "200", one2, three, two2)
	if got := chainInbox(s, two2); !slices.Equal(got, []string{"[from one %5] the fix is on main"}) {
		t.Fatalf("two's inbox = %q", got)
	}
}

// chainedOnWork is a service that keeps chains, and one, an agent with a
// saved history in a folder that is there.
func chainedOnWork(t *testing.T) (*Service, sample.Session) {
	t.Helper()
	s, _ := recoverTest(t)
	one := agentIn(t, s, "claude", "s1", "one", "%1")
	s.mu.Lock()
	s.ChainsPath, s.Chains = filepath.Join(t.TempDir(), "chains.json"), nil
	s.mu.Unlock()
	return s, one
}

// inWork is an agent with a saved history in tmux session work, window
// window, its window named name.
func inWork(t *testing.T, s *Service, id, name, pane string, window int) sample.Session {
	t.Helper()
	x := agentIn(t, s, "claude", id, name, pane)
	x.Target = fmt.Sprintf("work:%d.0", window)
	return x
}

// waitOf is the wait the doc shows for the member of conversation id.
func waitOf(t *testing.T, s *Service, id string) state.WaitView {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if v := s.cur.Chains; v != nil {
		for _, mv := range v.Mates {
			if mv.Member.SessionID == id && mv.Wait != nil {
				return *mv.Wait
			}
		}
	}
	t.Fatalf("the doc shows no wait for %s: %+v", id, s.cur.Chains)
	return state.WaitView{}
}

// TestReopenAWindowNamedWaitingMate: a match whose window is named api, in
// tmux session work, waits for recover. Its mates know it as api, and the
// record by its tmux session. Recover by the conversation the doc shows
// reopens it.
func TestReopenAWindowNamedWaitingMate(t *testing.T) {
	s, one := chainedOnWork(t)
	api := inWork(t, s, "s2", "api", "%2", 2)
	publishOn(s, "100", one, api)
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"%2"); res.Err != "" {
		t.Fatalf("chain = %+v", res)
	}
	publishOn(s, "100", one, api)
	loseServer(s)
	w := waitOf(t, s, "s2")
	if w.Name != "api" {
		t.Fatalf("the wait is named %q, want api", w.Name)
	}
	res := recoverAct(t, s, proto.Recover{Conversations: []string{w.Conversation}}, "")
	if res.Err != "" || len(res.Recover) != 1 || res.Recover[0].Conversation != "s2" || res.Recover[0].State != proto.RecoverReopened {
		t.Fatalf("recover = %+v", res)
	}
}

// TestReopenOneWaitingMateStartsOnlyIt: two conversations ran in tmux
// session work, and only one is chained. Recover by its conversation
// starts it, not the other agent of its tmux session.
func TestReopenOneWaitingMateStartsOnlyIt(t *testing.T) {
	s, one := chainedOnWork(t)
	api := inWork(t, s, "s2", "api", "%2", 2)
	web := inWork(t, s, "s3", "web", "%3", 3)
	publishOn(s, "100", one, api, web)
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"%2"); res.Err != "" {
		t.Fatalf("chain = %+v", res)
	}
	publishOn(s, "100", one, api, web)
	loseServer(s)
	res := recoverAct(t, s, proto.Recover{Conversations: []string{waitOf(t, s, "s2").Conversation}}, "")
	if res.Err != "" || len(res.Recover) != 1 || res.Recover[0].Conversation != "s2" || res.Recover[0].State != proto.RecoverReopened {
		t.Fatalf("recover = %+v", res)
	}
}

// TestRecoverRefusesAnUnknownConversation: recover names the conversation
// the record does not hold, and how to see those it does.
func TestRecoverRefusesAnUnknownConversation(t *testing.T) {
	s, one := chainedOnWork(t)
	publishOn(s, "100", one)
	loseServer(s)
	res := recoverAct(t, s, proto.Recover{Conversations: []string{"s1", "gone"}}, "")
	if want := "no conversation gone in the recovery record: matchblox recover lists them"; res.Err != want || len(res.Recover) != 0 {
		t.Fatalf("recover = %+v, want %q", res, want)
	}
}

// restart is what a service restart forgets: the chains it holds in memory
// load again from chains.json.
func restart(s *Service) {
	s.mu.Lock()
	s.Chains, s.mateNames, s.waitNames = nil, nil, nil
	s.mu.Unlock()
}

// TestALineToAWaitingMateOutlivesARestart: a mate sends to a member that
// waits for recover, and the service restarts before recover reopens it.
// The member still gets the line; once it took it, a further restart
// gives it no second copy.
func TestALineToAWaitingMateOutlivesARestart(t *testing.T) {
	s, one, two := rebootPair(t)
	loseServer(s)
	one2 := reopened(one, "%5", 201)
	publishOn(s, "200", one2)
	res := s.act(context.Background(), proto.Act{RecID: "message:sid:s2", Confirm: "y", Text: "the fix is on main", From: "%5", FromKnown: true, FromPlaced: true})
	if res.Err != "" {
		t.Fatalf("send = %+v", res)
	}
	restart(s)
	two2 := reopened(two, "%6", 202)
	publishOn(s, "200", one2, two2)
	if got := chainInbox(s, two2); !slices.Equal(got, []string{"[from one %5] the fix is on main"}) {
		t.Fatalf("after a restart two's inbox = %q", got)
	}
	publishOn(s, "200", one2, two2)
	restart(s)
	publishOn(s, "200", one2, two2)
	if got := chainInbox(s, two2); len(got) != 0 {
		t.Fatalf("after a second restart two's inbox = %q", got)
	}
}

// TestADeliveredLineDoesNotComeBack: a match hears its line, and the
// service stops before the next sample saves. After the restart the line
// is not in its inbox again.
func TestADeliveredLineDoesNotComeBack(t *testing.T) {
	s, one, two := rebootPair(t)
	publishOn(s, "100", one, two)
	s.mu.Lock()
	s.chainsLocked().PostLine(memberOf(two), memberOf(one), "[from one %1] the fix is on main", time.Now())
	if err := s.saveChainsLocked(); err != nil {
		t.Fatal(err)
	}
	s.mu.Unlock()
	d := s.news(proto.News{Pane: two.Pane})
	if !strings.Contains(d.text, "the fix is on main") {
		t.Fatalf("news = %q", d.text)
	}
	s.heard(d)
	restart(s)
	s.mu.Lock()
	got := s.chainsLocked().Take(memberOf(two)).Texts()
	s.mu.Unlock()
	if len(got) != 0 {
		t.Fatalf("after a restart two's inbox = %q", got)
	}
}
