package service

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

var newsGit = gitscan.Report{Repos: []gitscan.Repo{{Path: "/dev/matchblox", Worktrees: []gitscan.Worktree{
	{Path: "/dev/matchblox"}, {Path: "/dev/matchblox-299"}, {Path: "/dev/matchblox-300"},
}}}}

// newsSessions are %1 to %4 of one repo: %1 alone in matchblox-299, %2 in
// matchblox, %3 and %4 in matchblox-300.
var newsSessions = []sample.Session{
	{Pane: "%1", PID: 201, Start: 1, SessionID: "n1", Name: "peer-send", Agent: "claude", Cwd: "/dev/matchblox-299"},
	{Pane: "%2", PID: 202, Start: 2, SessionID: "n2", Name: "two", Agent: "claude", Cwd: "/dev/matchblox/cmd"},
	{Pane: "%3", PID: 203, Start: 3, SessionID: "n3", Name: "three", Agent: "codex", Cwd: "/dev/matchblox-300"},
	{Pane: "%4", PID: 204, Start: 4, SessionID: "n4", Name: "four", Agent: "claude", Cwd: "/dev/matchblox-300"},
}

// headGit is a worktree's HEAD and branch, and the files between two commits.
type headGit struct {
	mu    sync.Mutex
	heads map[string][2]string // dir -> commit, branch
	diffs map[string]string    // "old new" -> files, one a line
	calls int
}

func (g *headGit) run(dir string, args ...string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	switch {
	case slices.Equal(args, []string{"rev-parse", "HEAD", "--abbrev-ref", "HEAD"}):
		h, ok := g.heads[dir]
		if !ok {
			return "", errors.New("not a git repository")
		}
		return h[0] + "\n" + h[1] + "\n", nil
	case len(args) == 4 && args[0] == "diff" && args[1] == "--name-only":
		return g.diffs[args[2]+" "+args[3]], nil
	}
	return "", errors.New("unexpected git " + strings.Join(args, " "))
}

func (g *headGit) set(dir, commit, branch string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.heads[dir] = [2]string{commit, branch}
}

func newsTest(t *testing.T, sessions []sample.Session, chained ...string) (*Service, *headGit) {
	t.Helper()
	s := newTest(t, time.Hour)
	s.ChainsPath = filepath.Join(t.TempDir(), "chains.json")
	g := &headGit{heads: map[string][2]string{
		"/dev/matchblox": {"c0", "main"}, "/dev/matchblox-299": {"c1", "peer-send"}, "/dev/matchblox-300": {"c3", "www"},
	}, diffs: map[string]string{}}
	s.Git = g.run
	s.mu.Lock()
	git := newsGit
	s.cur.Git = &git
	s.mu.Unlock()
	chainSample(s, sessions...)
	for i := 1; i < len(chained); i++ {
		pane := chained[i]
		chainFrom(t, s, chained[0], proto.ChainPrefix+pane)
		chainFrom(t, s, pane, proto.ChainPrefix+chained[0])
	}
	chainSample(s, sessions...) // the first HEAD read of each chained worktree
	return s, g
}

func inbox(s *Service, x sample.Session) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, it := range s.chainsLocked().Take(memberOf(x)) {
		out = append(out, it.Text)
	}
	return out
}

func hookOf(x sample.Session, name string) hooks.Event {
	return hooks.Event{Name: name, SessionID: x.SessionID, Pane: x.Pane, Cwd: x.Cwd, At: time.Now()}
}

func TestAStopReachesTheMatesOnly(t *testing.T) {
	s, _ := newsTest(t, newsSessions, "%1", "%2", "%3")
	s.Hook(hookOf(newsSessions[0], "Stop"))
	for _, x := range newsSessions[1:3] {
		if got := inbox(s, x); !slices.Equal(got, []string{"peer-send finished"}) {
			t.Errorf("%s's inbox = %q", x.Pane, got)
		}
	}
	if got := inbox(s, newsSessions[0]); len(got) != 0 {
		t.Errorf("the source heard itself: %q", got)
	}
	s.mu.Lock()
	c, _ := s.Chains.ChainOf(memberOf(newsSessions[0]))
	s.mu.Unlock()
	if !c.Spark.IsZero() {
		t.Errorf("a finish sparked: %v", c.Spark)
	}
}

func TestACommitIsNamedOnce(t *testing.T) {
	s, g := newsTest(t, newsSessions, "%1", "%2", "%3")
	g.set("/dev/matchblox-299", "c2", "peer-send")
	g.diffs["c1 c2"] = "a.go\nb.go\n"
	ev := hookOf(newsSessions[0], "PostToolUse")
	ev.Command = "git commit -m x"
	s.Hook(ev)
	if got := inbox(s, newsSessions[1]); !slices.Equal(got, []string{"peer-send committed 2 files in a.go, b.go"}) {
		t.Fatalf("%%2's inbox = %q", got)
	}
	doc := chainSample(s, newsSessions...)
	if got := inbox(s, newsSessions[1]); len(got) != 0 {
		t.Fatalf("the sample named the commit again: %q", got)
	}
	if got := inbox(s, newsSessions[0]); len(got) != 0 {
		t.Errorf("the source heard itself: %q", got)
	}
	if doc.Chains == nil || doc.Chains.Chains[0].Spark.IsZero() {
		t.Errorf("a commit did not spark: %+v", doc.Chains)
	}
}

func TestAHeadMoveInASharedWorktreeNamesNoOne(t *testing.T) {
	sessions := slices.Clone(newsSessions[:3])
	sessions[1].Cwd = "/dev/matchblox-299/internal"
	s, g := newsTest(t, sessions, "%1", "%2", "%3")
	g.set("/dev/matchblox-299", "c2", "peer-send")
	g.diffs["c1 c2"] = "a.go\n"
	chainSample(s, sessions...)
	if got := inbox(s, sessions[2]); !slices.Equal(got, []string{"someone in matchblox-299 committed 1 file in a.go"}) {
		t.Fatalf("%%3's inbox = %q", got)
	}
	for _, x := range sessions[:2] {
		if got := inbox(s, x); len(got) != 0 {
			t.Errorf("%s, in the worktree, heard it: %q", x.Pane, got)
		}
	}
}

func TestABranchChangeIsNews(t *testing.T) {
	s, g := newsTest(t, newsSessions, "%1", "%2")
	g.set("/dev/matchblox-299", "c9", "fuse")
	chainSample(s, newsSessions...)
	if got := inbox(s, newsSessions[1]); !slices.Equal(got, []string{"peer-send switched to fuse"}) {
		t.Fatalf("%%2's inbox = %q", got)
	}
}

func TestAWaitIsNewsOncePerWait(t *testing.T) {
	s, _ := newsTest(t, newsSessions, "%1", "%2")
	perm := hookOf(newsSessions[0], "Notification")
	perm.Kind = hooks.KindPermission
	s.Hook(perm)
	idle := hookOf(newsSessions[0], "Notification")
	idle.Kind = hooks.KindIdlePrompt
	s.Hook(idle) // the same wait
	if got := inbox(s, newsSessions[1]); !slices.Equal(got, []string{"peer-send waits for the owner"}) {
		t.Fatalf("%%2's inbox = %q", got)
	}
	s.Hook(hookOf(newsSessions[0], "UserPromptSubmit"))
	perm.At = time.Now()
	s.Hook(perm)
	if got := inbox(s, newsSessions[1]); !slices.Equal(got, []string{"peer-send waits for the owner"}) {
		t.Fatalf("a new wait: %%2's inbox = %q", got)
	}
}

func TestUnchainedMatchesHearNothing(t *testing.T) {
	s, _ := newsTest(t, newsSessions, "%1", "%2", "%3")
	s.Hook(hookOf(newsSessions[3], "Stop"))
	for _, x := range newsSessions {
		if got := inbox(s, x); len(got) != 0 {
			t.Errorf("%s's inbox = %q", x.Pane, got)
		}
	}
}

func TestTheFirstHeadReadPostsNothing(t *testing.T) {
	s, g := newsTest(t, newsSessions)
	calls := g.calls
	chainSample(s, newsSessions...)
	if g.calls != calls {
		t.Errorf("an unchained host read git %d times", g.calls-calls)
	}
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	chainFrom(t, s, "%2", proto.ChainPrefix+"%1")
	g.set("/dev/matchblox-299", "c2", "peer-send")
	chainSample(s, newsSessions...)
	if got := inbox(s, newsSessions[1]); len(got) != 0 {
		t.Errorf("the first read posted %q", got)
	}
}

func TestCommitLine(t *testing.T) {
	for files, want := range map[string]string{
		"a.go":            "committed 1 file in a.go",
		"a\nb\nc":         "committed 3 files in a, b, c",
		"a\nb\nc\nd\ne\n": "committed 5 files in a, b, c, and 2 more",
		"":                "committed",
	} {
		if got := commitLine(strings.Fields(files)); got != want {
			t.Errorf("commitLine(%q) = %q, want %q", files, got, want)
		}
	}
}

func TestMovesHead(t *testing.T) {
	for line, want := range map[string]bool{
		"git commit -m x":                     true,
		"cd sub && git -C .. switch main":     true,
		"git -c user.name=x checkout -b fuse": true,
		"git --no-pager commit --amend":       true,
		"git log --oneline":                   false,
		"echo git commit":                     true, // a read too many costs one git call
		"gitk commit":                         false,
	} {
		if got := movesHead.MatchString(line); got != want {
			t.Errorf("movesHead(%q) = %v, want %v", line, got, want)
		}
	}
}
