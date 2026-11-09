package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func TestChainActFormsATeamAtOnce(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	res := chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	if res.Err != "" || res.Note != "joined one: 2 matches. To leave it, run: matchblox unchain one" {
		t.Fatalf("chain = %+v", res)
	}
	doc := chainSample(s, chainSessions...)
	if doc.Chains == nil || len(doc.Chains.Chains) != 1 {
		t.Fatalf("after one command: %+v", doc.Chains)
	}
	if c := doc.Chains.Chains[0]; c.Name != "one" || c.Repo != "/dev/app" || len(c.Members) != 2 {
		t.Fatalf("chain = %+v", c)
	}
}

func TestChainActChainsAcrossRepos(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"%3"); res.Err != "" || res.Note != "joined one: 2 matches. To leave it, run: matchblox unchain one" {
		t.Fatalf("chain = %+v", res)
	}
	for range 3 { // the chain outlives the samples after it
		doc := chainSample(s, chainSessions...)
		if doc.Chains == nil || len(doc.Chains.Chains) != 1 || len(doc.Chains.Chains[0].Members) != 2 {
			t.Fatalf("after a sample: %+v", doc.Chains)
		}
	}
}

func TestChainActChainsASessionInNoRepo(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	lost := sample.Session{Pane: "%4", PID: 104, SessionID: "s4", Name: "four", Cwd: "/tmp/x"}
	sessions := append(slices.Clone(chainSessions), lost)
	chainSample(s, sessions...)
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"%4"); res.Err != "" || res.Note != "joined one: 2 matches. To leave it, run: matchblox unchain one" {
		t.Fatalf("chain = %+v", res)
	}
	if doc := chainSample(s, sessions...); doc.Chains == nil || len(doc.Chains.Chains) != 1 {
		t.Fatalf("a sample dropped the no-repo match: %+v", doc.Chains)
	}
}

func TestChainsPruneBeforeTheFirstGitScan(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
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
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"-"); res.Note != "left one" {
		t.Fatalf("leave = %+v", res)
	}
	if doc := chainSample(s, chainSessions...); doc.Chains != nil {
		t.Fatalf("after leave: %+v", doc.Chains)
	}
	if res := chainFrom(t, s, "%2", proto.ChainPrefix+"off"); res.Note != "off: no notices, and no chains" {
		t.Fatalf("off = %+v", res)
	}
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
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
	if doc := chainSample(s, chainSessions[0], chainSessions[2]); doc.Chains != nil {
		t.Fatalf("after %%2 went: %+v", doc.Chains)
	}
}

func TestAMatchThatMovesStaysChained(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
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
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"%2"); res.Err != "two is off: it takes no chains. It can run: matchblox chain --on" {
		t.Errorf("off: %+v", res)
	}
	chainFrom(t, s, "%2", proto.ChainPrefix+"on")
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"%2"); res.Note != "already in one" || res.Err != "" {
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

// fourSessions are chainSessions and four in www.
var fourSessions = append(slices.Clone(chainSessions),
	sample.Session{Pane: "%4", PID: 104, Start: 4, SessionID: "s4", Name: "four", Agent: "cursor", Cwd: "/dev/www"})

func teamTest(t *testing.T) *Service {
	t.Helper()
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	chainSample(s, fourSessions...)
	return s
}

func mustChain(t *testing.T, s *Service, from, recID string) proto.Result {
	t.Helper()
	res := chainFrom(t, s, from, recID)
	if res.Err != "" {
		t.Fatalf("%s %s: %+v", from, recID, res)
	}
	return res
}

func TestChainJoinsTheOnlyTeamOfWho(t *testing.T) {
	s := teamTest(t)
	mustChain(t, s, "%2", proto.ChainPrefix+"%3") // team "two": two, three
	res := mustChain(t, s, "%1", proto.ChainPrefix+"%3")
	if res.Note != "joined two: 3 matches. To leave it, run: matchblox unchain two" {
		t.Fatalf("join = %+v", res)
	}
	if got := chainInbox(s, fourSessions[1]); !slices.Contains(got, "[matchblox] one joined the chain") {
		t.Fatalf("two heard %q", got)
	}
	if got := chainInbox(s, fourSessions[2]); !slices.Contains(got, "[matchblox] one joined the chain (in app)") {
		t.Fatalf("three heard %q", got)
	}
	if got := chainInbox(s, fourSessions[0]); len(got) != 0 {
		t.Fatalf("the joiner heard %q", got)
	}
	s.mu.Lock()
	spark := s.Chains.Chains[0].Spark
	s.mu.Unlock()
	if spark.IsZero() {
		t.Fatal("a join did not spark")
	}
}

func TestChainFormsATeamNamedAfterTheCaller(t *testing.T) {
	s := teamTest(t)
	res := mustChain(t, s, "%1", proto.ChainPrefix+"%3")
	if res.Note != "joined one: 2 matches. To leave it, run: matchblox unchain one" {
		t.Fatalf("form = %+v", res)
	}
}

func TestChainRefusesWhoInTwoTeams(t *testing.T) {
	s := teamTest(t)
	mustChain(t, s, "%1", proto.ChainPrefix+"%2") // one: one, two
	mustChain(t, s, "%3", proto.ChainPrefix+"%4") // three: three, four
	mustChain(t, s, "%2", proto.ChainPrefix+"@three")
	res := chainFrom(t, s, "%4", proto.ChainPrefix+"%2")
	if want := "two is in 2 chains. Run one:\n  matchblox chain one\n  matchblox chain three"; res.Err != want {
		t.Fatalf("err = %q, want %q", res.Err, want)
	}
}

func TestChainByTeamName(t *testing.T) {
	s := teamTest(t)
	mustChain(t, s, "%1", proto.ChainPrefix+"%2")
	if res := mustChain(t, s, "%4", proto.ChainPrefix+"@one"); res.Note != "joined one: 3 matches. To leave it, run: matchblox unchain one" {
		t.Fatalf("join = %+v", res)
	}
	if res := chainFrom(t, s, "%4", proto.ChainPrefix+"@none"); res.Err != "no chain is named none" {
		t.Fatalf("no team = %+v", res)
	}
}

func TestChainRefusesOff(t *testing.T) {
	s := teamTest(t)
	mustChain(t, s, "%2", proto.ChainPrefix+"off")
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"%2"); res.Err != "two is off: it takes no chains. It can run: matchblox chain --on" {
		t.Fatalf("to off = %q", res.Err)
	}
	if res := chainFrom(t, s, "%2", proto.ChainPrefix+"%1"); res.Err != "two is off: it takes no chains. It can run: matchblox chain --on" {
		t.Fatalf("from off = %q", res.Err)
	}
	mustChain(t, s, "%1", proto.ChainPrefix+"%3")
	if res := chainFrom(t, s, "%2", proto.ChainPrefix+"@one"); res.Err != "two is off: it takes no chains. It can run: matchblox chain --on" {
		t.Fatalf("off by name = %q", res.Err)
	}
}

func TestBareUnchainInTwoTeamsRefuses(t *testing.T) {
	s := teamTest(t)
	mustChain(t, s, "%1", proto.ChainPrefix+"%2") // one
	mustChain(t, s, "%3", proto.ChainPrefix+"%4") // three
	mustChain(t, s, "%1", proto.ChainPrefix+"@three")
	want := "you are in one and three. Run one:\n  matchblox unchain one\n  matchblox unchain three\n  matchblox unchain --all"
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"-"); res.Err != want {
		t.Fatalf("err = %q, want %q", res.Err, want)
	}
	s.mu.Lock()
	_, err := s.Chains.Form(memberOf(fourSessions[0]), memberOf(fourSessions[3]), "web", "")
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"-"); !strings.HasPrefix(res.Err, "you are in one, three and web. Run one:\n") {
		t.Fatalf("three teams = %q", res.Err)
	}
}

func TestUnchainTeamAndAll(t *testing.T) {
	s := teamTest(t)
	mustChain(t, s, "%1", proto.ChainPrefix+"%2")
	mustChain(t, s, "%3", proto.ChainPrefix+"%4")
	mustChain(t, s, "%1", proto.ChainPrefix+"@three")
	if res := mustChain(t, s, "%1", proto.ChainPrefix+"-@one"); res.Note != "left one" {
		t.Fatalf("leave = %+v", res)
	}
	mustChain(t, s, "%1", proto.ChainPrefix+"%2") // one again, with two
	if res := mustChain(t, s, "%1", proto.ChainPrefix+"-*"); res.Note != "left three and one" {
		t.Fatalf("leave all = %+v", res)
	}
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"-@three"); res.Note != "not in three" {
		t.Fatalf("leave again = %+v", res)
	}
}

func TestBareUnchainLeavesTheOnlyTeam(t *testing.T) {
	s := teamTest(t)
	mustChain(t, s, "%1", proto.ChainPrefix+"%2")
	if res := mustChain(t, s, "%1", proto.ChainPrefix+"-"); res.Note != "left one" {
		t.Fatalf("leave = %+v", res)
	}
}

func TestOwnerTakesAMatchOut(t *testing.T) {
	s := teamTest(t)
	mustChain(t, s, "%1", proto.ChainPrefix+"%2")
	mustChain(t, s, "%4", proto.ChainPrefix+"@one")
	chainInbox(s, fourSessions[3])
	res := s.act(context.Background(), proto.Act{RecID: proto.ChainPrefix + "out:%4@one", Console: true})
	if res.Err != "" || res.Note != "took four out of one" {
		t.Fatalf("out = %+v", res)
	}
	if got := chainInbox(s, fourSessions[3]); !slices.Equal(got, []string{"[matchblox] the owner took you out of one"}) {
		t.Fatalf("four heard %q", got)
	}
	s.mu.Lock()
	c, _ := s.Chains.Team("one")
	s.mu.Unlock()
	if len(c.Members) != 2 {
		t.Fatalf("team = %+v", c)
	}
}

func TestAnAgentCannotTakeAnotherOut(t *testing.T) {
	s := teamTest(t)
	mustChain(t, s, "%1", proto.ChainPrefix+"%2")
	for _, a := range []proto.Act{
		{RecID: proto.ChainPrefix + "out:%2@one", From: "%1"},
		{RecID: proto.ChainPrefix + "out:%2@one", From: "%1", Console: true},
	} {
		if res := s.act(context.Background(), a); res.Err == "" {
			t.Fatalf("%+v took a match out: %+v", a, res)
		}
	}
	s.mu.Lock()
	c, _ := s.Chains.Team("one")
	s.mu.Unlock()
	if len(c.Members) != 2 {
		t.Fatalf("team = %+v", c)
	}
}

func TestStokerJoinsNoChain(t *testing.T) {
	s := teamTest(t)
	s.StokerServer = "mbstoker"
	stoker := sample.Session{Pane: sample.OnServer("%7", "mbstoker"), PID: 107, Start: 7, SessionID: "s7", Name: "stoker", Agent: "claude", Cwd: "/dev/app"}
	chainSample(s, append(slices.Clone(fourSessions), stoker)...)
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+stoker.Pane); res.Err != "the stoker joins no chain" {
		t.Fatalf("to the stoker = %+v", res)
	}
	if res := chainFrom(t, s, stoker.Pane, proto.ChainPrefix+"%1"); res.Err != "the stoker joins no chain" {
		t.Fatalf("from the stoker = %+v", res)
	}
}

func TestOldStoreTeamsGetNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chains.json")
	old := `{"chains":[{"id":"%2#1","repo":"/dev/app","members":[{"session_id":"s9","pane":"%9"},{"session_id":"s2","pane":"%2"},{"session_id":"s1","pane":"%1"}]}]}`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s := chainTest(t, path)
	doc := chainSample(s, chainSessions...)
	if doc.Chains == nil || len(doc.Chains.Chains) != 1 || doc.Chains.Chains[0].Name != "two" {
		t.Fatalf("chains = %+v", doc.Chains)
	}
}

// paneSample publishes sessions with the tmux panes listed in panes.
func paneSample(s *Service, at time.Time, sessions []sample.Session, panes ...string) state.Doc {
	snap := sample.Snapshot{At: at, Sessions: sessions}
	for _, p := range panes {
		snap.Panes = append(snap.Panes, sample.Pane{ID: p})
	}
	var doc state.Doc
	s.publish(snap, func(d state.Doc) { doc = d })
	s.headRun.wait()
	return doc
}

func TestEmptyPaneKeepsTheMateUnseen(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	clock := night0
	s.Now = func() time.Time { return clock }
	mustChain(t, s, "%1", proto.ChainPrefix+"%2")
	clock = clock.Add(time.Minute)
	doc := paneSample(s, clock, chainSessions[:1:1], "%1", "%2") // two's agent is gone; its pane lives
	if doc.Chains == nil || len(doc.Chains.Chains) != 1 {
		t.Fatalf("two left while its pane lives: %+v", doc.Chains)
	}
	first := clock
	clock = clock.Add(11 * time.Minute)
	doc = paneSample(s, clock, chainSessions[:1:1], "%1", "%2")
	i := slices.IndexFunc(doc.Chains.Mates, func(v state.MateView) bool { return v.Member.Pane == "%2" })
	if i < 0 || !doc.Chains.Mates[i].Unseen.Equal(first) {
		t.Fatalf("mates = %+v", doc.Chains.Mates)
	}
	if got := chainInbox(s, chainSessions[0]); len(got) != 0 {
		t.Fatalf("one heard %q", got)
	}
}

func TestClosedPaneLeaves(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	mustChain(t, s, "%1", proto.ChainPrefix+"%2")
	chainInbox(s, chainSessions[1])
	if doc := paneSample(s, time.Now(), chainSessions[:1:1], "%1"); doc.Chains != nil {
		t.Fatalf("a closed pane stayed: %+v", doc.Chains)
	}
	if got := chainInbox(s, chainSessions[0]); !slices.Equal(got, []string{"two left the chain"}) {
		t.Fatalf("one heard %q", got)
	}
}

func TestSampleMissKeepsTeamsAndTimes(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	mustChain(t, s, "%1", proto.ChainPrefix+"%2")
	s.publish(sample.Snapshot{At: time.Now(), Sessions: chainSessions[:1:1], Errors: []string{"tmux: no server"}}, func(state.Doc) {})
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Chains.Chains) != 1 || len(s.Chains.Unseen) != 0 {
		t.Fatalf("a missed sample changed the chains: %+v %v", s.Chains.Chains, s.Chains.Unseen)
	}
}

func TestSeenAgainClearsUnseen(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	mustChain(t, s, "%1", proto.ChainPrefix+"%2")
	paneSample(s, time.Now(), chainSessions[:1:1], "%1", "%2")
	doc := paneSample(s, time.Now(), chainSessions[:2:2], "%1", "%2")
	for _, v := range doc.Chains.Mates {
		if !v.Unseen.IsZero() {
			t.Fatalf("seen again, still unseen: %+v", v)
		}
	}
}

func TestChainsViewCarriesMates(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	mustChain(t, s, "%1", proto.ChainPrefix+"%2")
	chainInbox(s, chainSessions[1])
	s.mu.Lock()
	for i := range 3 {
		s.Chains.Post(memberOf(chainSessions[1]), fmt.Sprint(i), time.Now())
	}
	for i := range 21 {
		s.Chains.Post(memberOf(chainSessions[0]), fmt.Sprint(i), time.Now())
	}
	s.mu.Unlock()
	doc := chainSample(s, chainSessions...)
	got := map[string]state.MateView{}
	for _, v := range doc.Chains.Mates {
		got[v.Member.Pane] = v
	}
	if len(doc.Chains.Mates) != 2 || got["%2"].Inbox != 3 || got["%2"].Full || got["%1"].Inbox != 20 || !got["%1"].Full {
		t.Fatalf("mates = %+v", doc.Chains.Mates)
	}
}
