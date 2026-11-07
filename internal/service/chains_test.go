package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

var chainGit = gitscan.Report{Repos: []gitscan.Repo{
	{Path: "/dev/app", Worktrees: []gitscan.Worktree{{Path: "/dev/app"}, {Path: "/dev/app-wt"}}},
	{Path: "/dev/www"},
}}

// chainSessions are %1 and %2 in two worktrees of app, and %3 in www.
var chainSessions = []sample.Session{
	{Pane: "%1", PID: 101, Start: 1, SessionID: "s1", Name: "one", Agent: "claude", Cwd: "/dev/app"},
	{Pane: "%2", PID: 102, Start: 2, SessionID: "s2", Name: "two", Agent: "claude", Cwd: "/dev/app-wt/cmd"},
	{Pane: "%3", PID: 103, Start: 3, SessionID: "s3", Name: "three", Agent: "codex", Cwd: "/dev/www"},
}

func chainTest(t *testing.T, path string) *Service {
	t.Helper()
	s := newTest(t, time.Hour)
	s.ChainsPath = path
	s.Git = func(string, ...string) (string, error) { return "", errors.New("no git here") }
	s.mu.Lock()
	git := chainGit
	s.cur.Git = &git
	s.mu.Unlock()
	chainSample(s, chainSessions...)
	return s
}

func chainSample(s *Service, sessions ...sample.Session) state.Doc {
	var doc state.Doc
	s.publish(sample.Snapshot{At: time.Now(), Sessions: sessions}, func(d state.Doc) { doc = d })
	s.headRun.wait() // the HEAD reads it started, out of the sample loop
	return doc
}

func chainFrom(t *testing.T, s *Service, from, recID string) proto.Result {
	t.Helper()
	return s.act(context.Background(), proto.Act{RecID: recID, From: from})
}

func TestChainActFormsAChainOnlyBothWays(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	res := chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	if res.Err != "" || res.Note != "asked two: the chain forms when it chains back" {
		t.Fatalf("ask = %+v", res)
	}
	doc := chainSample(s, chainSessions...)
	if doc.Chains == nil || len(doc.Chains.Chains) != 0 || len(doc.Chains.Requests) != 1 {
		t.Fatalf("after one side: %+v", doc.Chains)
	}
	res = chainFrom(t, s, "%2", proto.ChainPrefix+"%1")
	if res.Err != "" || res.Note != "chained with one: 2 matches" {
		t.Fatalf("ask back = %+v", res)
	}
	doc = chainSample(s, chainSessions...)
	if doc.Chains == nil || len(doc.Chains.Chains) != 1 || len(doc.Chains.Requests) != 0 {
		t.Fatalf("after both sides: %+v", doc.Chains)
	}
	if c := doc.Chains.Chains[0]; c.Repo != "/dev/app" || len(c.Members) != 2 {
		t.Fatalf("chain = %+v", c)
	}
}

func TestChainActChainsAcrossRepos(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"%3"); res.Err != "" {
		t.Fatalf("ask = %+v", res)
	}
	if res := chainFrom(t, s, "%3", proto.ChainPrefix+"%1"); res.Err != "" || res.Note != "chained with one: 2 matches" {
		t.Fatalf("ask back = %+v", res)
	}
	for range 3 { // the chain outlives the samples after it
		doc := chainSample(s, chainSessions...)
		if doc.Chains == nil || len(doc.Chains.Chains) != 1 || len(doc.Chains.Chains[0].Members) != 2 {
			t.Fatalf("after a sample: %+v", doc.Chains)
		}
	}
}

func TestAnAskAcrossReposLastsItsTTL(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	chainFrom(t, s, "%1", proto.ChainPrefix+"%3")
	if doc := chainSample(s, chainSessions...); doc.Chains == nil || len(doc.Chains.Requests) != 1 {
		t.Fatalf("a sample dropped the ask across repos: %+v", doc.Chains)
	}
}

func TestChainActChainsASessionInNoRepo(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	lost := sample.Session{Pane: "%4", PID: 104, SessionID: "s4", Name: "four", Cwd: "/tmp/x"}
	sessions := append(slices.Clone(chainSessions), lost)
	chainSample(s, sessions...)
	chainFrom(t, s, "%1", proto.ChainPrefix+"%4")
	if res := chainFrom(t, s, "%4", proto.ChainPrefix+"%1"); res.Err != "" || res.Note != "chained with one: 2 matches" {
		t.Fatalf("ask back = %+v", res)
	}
	if doc := chainSample(s, sessions...); doc.Chains == nil || len(doc.Chains.Chains) != 1 {
		t.Fatalf("a sample dropped the no-repo match: %+v", doc.Chains)
	}
}

func TestChainsPruneBeforeTheFirstGitScan(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	chainFrom(t, s, "%2", proto.ChainPrefix+"%1")
	s.mu.Lock()
	s.cur.Git = nil
	s.mu.Unlock()
	if doc := chainSample(s, chainSessions[0], chainSessions[2]); doc.Chains != nil {
		t.Fatalf("with no git scan, %%2 went and its chain stayed: %+v", doc.Chains)
	}
}

func TestChainActRefusesTheConsole(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	if res := chainFrom(t, s, "", proto.ChainPrefix+"%2"); res.Err != "chain needs a match: run it from an agent's pane" {
		t.Fatalf("err = %q", res.Err)
	}
	if res := chainFrom(t, s, "%9", proto.ChainPrefix+"%2"); res.Err != "chain needs a match: run it from an agent's pane" {
		t.Fatalf("a pane with no agent: err = %q", res.Err)
	}
}

func TestChainActRefusesAGonePane(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"%9"); res.Err != "no agent session in %9 any more" {
		t.Fatalf("err = %q", res.Err)
	}
}

func TestChainActLeavesAndTurnsOff(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	chainFrom(t, s, "%2", proto.ChainPrefix+"%1")
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"-"); res.Note != "left the chain" {
		t.Fatalf("leave = %+v", res)
	}
	if doc := chainSample(s, chainSessions...); doc.Chains != nil {
		t.Fatalf("after leave: %+v", doc.Chains)
	}
	if res := chainFrom(t, s, "%2", proto.ChainPrefix+"off"); res.Note != "off: no notices, and no chains" {
		t.Fatalf("off = %+v", res)
	}
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	chainFrom(t, s, "%2", proto.ChainPrefix+"%1")
	if doc := chainSample(s, chainSessions...); doc.Chains != nil && len(doc.Chains.Chains) > 0 {
		t.Fatalf("an off match chained: %+v", doc.Chains)
	}
	if res := chainFrom(t, s, "%2", proto.ChainPrefix+"on"); res.Note != "on" {
		t.Fatalf("on = %+v", res)
	}
}

func TestChainsSurviveARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chains.json")
	s := chainTest(t, path)
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	chainFrom(t, s, "%2", proto.ChainPrefix+"%1")

	again := chainTest(t, path)
	doc := chainSample(again, chainSessions...)
	if doc.Chains == nil || len(doc.Chains.Chains) != 1 {
		t.Fatalf("after a restart: %+v", doc.Chains)
	}
}

func TestACorruptChainsFileStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chains.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := chainTest(t, path)
	if doc := chainSample(s, chainSessions...); doc.Chains != nil {
		t.Fatalf("from a corrupt file: %+v", doc.Chains)
	}
	if b, err := os.ReadFile(path + ".bad"); err != nil || string(b) != "{not json" {
		t.Fatalf("chains.json.bad = %q, %v", b, err)
	}
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the empty store did not save: %v", err)
	}
}

func TestAGoneMemberIsPrunedOnSample(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	chainFrom(t, s, "%2", proto.ChainPrefix+"%1")
	if doc := chainSample(s, chainSessions[0], chainSessions[2]); doc.Chains != nil {
		t.Fatalf("after %%2 went: %+v", doc.Chains)
	}
}

func TestAMatchThatMovesStaysChained(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	chainFrom(t, s, "%2", proto.ChainPrefix+"%1")
	moved := chainSessions[1]
	moved.Cwd = "/dev/www"
	doc := chainSample(s, chainSessions[0], moved, chainSessions[2])
	if doc.Chains == nil || len(doc.Chains.Chains) != 1 || len(doc.Chains.Chains[0].Members) != 2 {
		t.Fatalf("a move left the chain: %+v", doc.Chains)
	}
	s.mu.Lock()
	items := s.Chains.Take(memberOf(chainSessions[0])).Texts()
	s.mu.Unlock()
	if len(items) != 0 {
		t.Fatalf("one's inbox = %+v", items)
	}
}

func TestAFailedSampleKeepsTheChains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chains.json")
	s := chainTest(t, path)
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	chainFrom(t, s, "%2", proto.ChainPrefix+"%1")
	for _, why := range []string{"tmux: x", "proc: x"} {
		var doc state.Doc
		s.publish(sample.Snapshot{At: time.Now(), Errors: []string{why}}, func(d state.Doc) { doc = d })
		if doc.Chains == nil || len(doc.Chains.Chains) != 1 {
			t.Fatalf("after a sample with %q: %+v", why, doc.Chains)
		}
	}
	if doc := chainSample(chainTest(t, path), chainSessions...); doc.Chains == nil || len(doc.Chains.Chains) != 1 {
		t.Fatalf("on disk after the failed samples: %+v", doc.Chains)
	}
}

func chainInbox(s *Service, x sample.Session) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Chains.Take(memberOf(x)).Texts()
}

// TestAMatchWithANewSessionIDStaysChained: a cleared chat (/clear, Codex
// /new) is a new session id in the same pane, pid and start: the same match.
func TestAMatchWithANewSessionIDStaysChained(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	chainFrom(t, s, "%2", proto.ChainPrefix+"%1")
	cleared := chainSessions[1]
	cleared.SessionID = "s2-new"
	doc := chainSample(s, chainSessions[0], cleared, chainSessions[2])
	if doc.Chains == nil || len(doc.Chains.Chains) != 1 || !slices.ContainsFunc(doc.Chains.Chains[0].Members, memberOf(cleared).Same) {
		t.Fatalf("after the clear: %+v", doc.Chains)
	}
	if got := chainInbox(s, chainSessions[0]); len(got) != 0 {
		t.Fatalf("one heard %q", got)
	}
}

func TestAGoneMemberTellsItsMates(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	chainFrom(t, s, "%2", proto.ChainPrefix+"%1")
	chainSample(s, chainSessions[0], chainSessions[2])
	if got := chainInbox(s, chainSessions[0]); !slices.Equal(got, []string{"two left the chain"}) {
		t.Fatalf("one's inbox = %q", got)
	}
}

func TestTheOffMarkOfAGoneMatchGoes(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	chainFrom(t, s, "%3", proto.ChainPrefix+"off")
	chainSample(s, chainSessions[:2]...)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Chains.Off) != 0 {
		t.Fatalf("off = %+v", s.Chains.Off)
	}
}

func TestLeavingNoChainSavesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chains.json")
	s := chainTest(t, path)
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"-"); res.Note != "not in a chain" || res.Err != "" {
		t.Fatalf("leave = %+v", res)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("saved: %v", err)
	}
}

func TestChainActRefusals(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"%1"); res.Err != "a match cannot chain with itself" {
		t.Errorf("itself: %+v", res)
	}
	chainFrom(t, s, "%2", proto.ChainPrefix+"off")
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"%2"); res.Err != "two is off: it takes no chains" {
		t.Errorf("off: %+v", res)
	}
	chainFrom(t, s, "%2", proto.ChainPrefix+"on")
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	chainFrom(t, s, "%2", proto.ChainPrefix+"%1")
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"%2"); res.Note != "already chained with two" || res.Err != "" {
		t.Errorf("again: %+v", res)
	}
	far := chainSessions[2]
	far.Cwd, far.Host = "/dev/app", "far"
	chainSample(s, chainSessions[0], chainSessions[1], far)
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"%3"); res.Err != "three is on another host: a chain is one host" {
		t.Errorf("another host: %+v", res)
	}
}

// TestChainActWaitsForASampleThatReadTheSessions: a sample that missed
// the sessions cannot tell a cleared chat from a new match.
func TestChainActWaitsForASampleThatReadTheSessions(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	s.publish(sample.Snapshot{At: time.Now(), Sessions: chainSessions, Errors: []string{"tmux: x"}}, func(state.Doc) {})
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"%2"); res.Err != "not now: matchblox could not read the sessions; try again" {
		t.Fatalf("%+v", res)
	}
}

func TestReadGitNeedsADirectory(t *testing.T) {
	s := newTest(t, time.Hour)
	if _, err := s.readGit("", "rev-parse", "HEAD"); err == nil || err.Error() != "workspace is unknown" {
		t.Fatalf("err = %v", err)
	}
}

// TestOffWhileTheSampleMissedFindsAClearedMatch: off, on and leave act on
// the match as it is now, even when the last sample missed: they refresh
// first. Only an ask waits.
func TestOffWhileTheSampleMissedFindsAClearedMatch(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	chainFrom(t, s, "%2", proto.ChainPrefix+"%1")
	cleared := slices.Clone(chainSessions)
	cleared[1].SessionID = "s2-new"
	s.publish(sample.Snapshot{At: time.Now(), Sessions: cleared, Errors: []string{"tmux: x"}}, func(state.Doc) {})
	if res := chainFrom(t, s, "%2", proto.ChainPrefix+"off"); res.Err != "" || res.Note != "off: no notices, and no chains" {
		t.Fatalf("off = %+v", res)
	}
	s.mu.Lock()
	chained, off := len(s.Chains.Chains), slices.Clone(s.Chains.Off)
	s.mu.Unlock()
	if chained != 0 || len(off) != 1 || off[0].SessionID != "s2-new" {
		t.Fatalf("chains %d, off %+v", chained, off)
	}
	if res := chainFrom(t, s, "%2", proto.ChainPrefix+"on"); res.Err != "" || res.Note != "on" {
		t.Fatalf("on = %+v", res)
	}
	if res := chainFrom(t, s, "%2", proto.ChainPrefix+"-"); res.Err != "" || res.Note != "not in a chain" {
		t.Fatalf("leave = %+v", res)
	}
}
