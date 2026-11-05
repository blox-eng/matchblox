package service

import (
	"context"
	"os"
	"path/filepath"
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

func TestChainActRefusesAnotherRepo(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	res := chainFrom(t, s, "%1", proto.ChainPrefix+"%3")
	if res.Err != "three works in www, not app: a chain is one repo" {
		t.Fatalf("err = %q", res.Err)
	}
	if doc := chainSample(s, chainSessions...); doc.Chains != nil {
		t.Fatalf("a refused ask left %+v", doc.Chains)
	}
}

func TestChainActRefusesASessionInNoRepo(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	lost := sample.Session{Pane: "%4", PID: 104, SessionID: "s4", Name: "four", Cwd: "/tmp/x"}
	chainSample(s, append(chainSessions, lost)...)
	if res := chainFrom(t, s, "%1", proto.ChainPrefix+"%4"); res.Err != "four is in no repo matchblox knows: a chain is one repo" {
		t.Fatalf("err = %q", res.Err)
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

func TestAMatchThatMovesTellsItsMates(t *testing.T) {
	s := chainTest(t, filepath.Join(t.TempDir(), "chains.json"))
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	chainFrom(t, s, "%2", proto.ChainPrefix+"%1")
	moved := chainSessions[1]
	moved.Cwd = "/dev/www"
	chainSample(s, chainSessions[0], moved, chainSessions[2])
	s.mu.Lock()
	items := s.Chains.Take(memberOf(chainSessions[0]))
	s.mu.Unlock()
	if len(items) != 1 || items[0].Text != "two left the chain: it moved to www" {
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
