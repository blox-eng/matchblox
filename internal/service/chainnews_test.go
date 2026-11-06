package service

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
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

// headGit is a worktree's HEAD and branch, what its last reflog entry
// says moved it ("commit: x" when unset), and the files between two
// commits.
type headGit struct {
	mu    sync.Mutex
	heads map[string][2]string // dir -> commit, branch
	how   map[string]string    // dir -> reflog subject
	diffs map[string]string    // "old new" -> files, one a line
	stats map[string]int       // "old new" -> files changed, when not the lines of diffs
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
	case slices.Equal(args, []string{"reflog", "-1", "--format=%gs", "HEAD"}):
		return cmp.Or(g.how[dir], "commit: x") + "\n", nil
	case len(args) == 4 && args[0] == "diff" && args[1] == "--name-only":
		return g.diffs[args[2]+" "+args[3]], nil
	case len(args) == 4 && args[0] == "diff" && args[1] == "--shortstat":
		k := args[2] + " " + args[3]
		n, ok := g.stats[k]
		if !ok {
			n = len(strings.Fields(g.diffs[k]))
		}
		if n == 0 {
			return "", nil
		}
		return fmt.Sprintf(" %d files changed, %d insertions(+)\n", n, n), nil
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
	}, how: map[string]string{}, diffs: map[string]string{}, stats: map[string]int{}}
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

// headsDone waits for the HEAD reads a publish started.
func headsDone(s *Service) { s.headRun.wait() }

func inbox(s *Service, x sample.Session) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chainsLocked().Take(memberOf(x)).Texts()
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
	for _, c := range []struct {
		n     int
		names string
		want  string
	}{
		{1, "a.go", "committed 1 file in a.go"},
		{3, "a\nb\nc", "committed 3 files in a, b, c"},
		{5, "a\nb\nc\nd\ne\n", "committed 5 files in a, b, c, and 2 more"},
		{900, "a\nb\nc\nd", "committed 900 files in a, b, c, and 897 more"},
		{2, "", "committed 2 files"},
		{0, "", "committed"},
	} {
		if got := commitLine(c.n, strings.Fields(c.names)); got != c.want {
			t.Errorf("commitLine(%d, %q) = %q, want %q", c.n, c.names, got, c.want)
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

// TestPublishDoesNotWaitForGit: the HEAD reads run out of the sample loop,
// one run at a time: a publish while one runs starts no other.
func TestPublishDoesNotWaitForGit(t *testing.T) {
	s, g := newsTest(t, newsSessions, "%1", "%2")
	var calls atomic.Int32
	block, started := make(chan struct{}), make(chan struct{}, 8)
	s.Git = func(dir string, args ...string) (string, error) {
		calls.Add(1)
		started <- struct{}{}
		<-block
		return g.run(dir, args...)
	}
	publish := func() {
		done := make(chan struct{})
		go func() {
			s.publish(sample.Snapshot{At: time.Now(), Sessions: newsSessions}, func(state.Doc) {})
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("publish waits for git")
		}
	}
	publish()
	<-started
	publish()
	close(block)
	headsDone(s)
	if n := calls.Load(); n != 2 { // one rev-parse for each chained worktree, once
		t.Fatalf("git ran %d times", n)
	}
}

// TestAHookCommitInASharedWorktreeReachesNoWorktreeMate: the matches that
// share the committer's checkout hear nothing, on the hook path as on the
// sample's.
func TestAHookCommitInASharedWorktreeReachesNoWorktreeMate(t *testing.T) {
	sessions := slices.Clone(newsSessions[:3])
	sessions[1].Cwd = "/dev/matchblox-299/internal"
	s, g := newsTest(t, sessions, "%1", "%2", "%3")
	g.set("/dev/matchblox-299", "c2", "peer-send")
	g.diffs["c1 c2"] = "a.go\n"
	ev := hookOf(sessions[0], "PostToolUse")
	ev.Command = "git commit -m x"
	s.Hook(ev)
	if got := inbox(s, sessions[2]); !slices.Equal(got, []string{"peer-send committed 1 file in a.go"}) {
		t.Fatalf("%%3's inbox = %q", got)
	}
	if got := inbox(s, sessions[1]); len(got) != 0 {
		t.Fatalf("%%2, in the worktree, heard %q", got)
	}
}

// TestAHeadReadKeepsTheMoveOfAHookInFlight: a sample that no longer sees a
// chained match in a worktree does not forget its HEAD while a hook reads
// it, so the hook's commit is still news.
func TestAHeadReadKeepsTheMoveOfAHookInFlight(t *testing.T) {
	s, g := newsTest(t, newsSessions, "%1", "%2")
	block, reading := make(chan struct{}), make(chan struct{}, 1)
	s.Git = func(dir string, args ...string) (string, error) {
		if dir == "/dev/matchblox-299" && args[0] == "rev-parse" {
			select {
			case reading <- struct{}{}:
				<-block
			default:
			}
		}
		return g.run(dir, args...)
	}
	g.set("/dev/matchblox-299", "c2", "peer-send")
	g.diffs["c1 c2"] = "a.go\n"
	ev := hookOf(newsSessions[0], "PostToolUse")
	ev.Command = "git commit -m x"
	hooked := make(chan struct{})
	go func() { s.Hook(ev); close(hooked) }()
	<-reading
	moved := slices.Clone(newsSessions)
	moved[0].Cwd = "/dev/matchblox-300"
	chainSample(s, moved...)
	close(block)
	<-hooked
	if got := inbox(s, newsSessions[1]); !slices.Equal(got, []string{"peer-send committed 1 file in a.go"}) {
		t.Fatalf("%%2's inbox = %q", got)
	}
}

// TestACommitCountsEveryFileAndKeepsGitsWarningsOut: the count is git's
// shortstat, not the names it lists; a path keeps its letters; what git
// says on stderr is no file.
func TestACommitCountsEveryFileAndKeepsGitsWarningsOut(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	repo := t.TempDir()
	gitIn := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %q: %s: %s", args, err, out)
		}
	}
	gitIn("init", "-q", "-b", "main")
	gitIn("commit", "-q", "--allow-empty", "-m", "first")
	other := filepath.Join(t.TempDir(), "other")
	gitIn("worktree", "add", "-q", "-b", "other", other)
	sessions := []sample.Session{
		{Pane: "%1", PID: 201, Start: 1, SessionID: "n1", Name: "one", Agent: "claude", Cwd: repo},
		{Pane: "%2", PID: 202, Start: 2, SessionID: "n2", Name: "two", Agent: "claude", Cwd: other},
	}
	s := newTest(t, time.Hour)
	s.ChainsPath = filepath.Join(t.TempDir(), "chains.json")
	s.mu.Lock()
	s.cur.Git = &gitscan.Report{Repos: []gitscan.Repo{{Path: repo, Worktrees: []gitscan.Worktree{{Path: repo}, {Path: other}}}}}
	s.mu.Unlock()
	chainSample(s, sessions...)
	chainFrom(t, s, "%1", proto.ChainPrefix+"%2")
	chainFrom(t, s, "%2", proto.ChainPrefix+"%1")
	chainSample(s, sessions...)
	for _, f := range []string{"ä.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(repo, f), []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitIn("add", ".")
	gitIn("commit", "-q", "-m", "second")
	t.Setenv("GIT_TRACE", "1") // git writes its trace to stderr
	ev := hookOf(sessions[0], "PostToolUse")
	ev.Command = "git commit -m second"
	s.Hook(ev)
	if got := inbox(s, sessions[1]); !slices.Equal(got, []string{"one committed 2 files in b.go, ä.go"}) {
		t.Fatalf("two's inbox = %q", got)
	}
}

// TestAMoveReadsWhatMovedIt: the reflog tells a commit from a pull, a
// reset or a rebase, which are no commit of the match's.
func TestAMoveReadsWhatMovedIt(t *testing.T) {
	for how, want := range map[string]string{
		"commit: x":                           "peer-send committed 2 files in a.go, b.go",
		"commit (amend): x":                   "peer-send committed 2 files in a.go, b.go",
		"pull: Fast-forward":                  "peer-send pulled 2 files into peer-send",
		"reset: moving to HEAD~1":             "peer-send moved peer-send to c2abcde",
		"rebase (finish): returning to x":     "peer-send moved peer-send to c2abcde",
		"merge fuse: Merge made by the 'ort'": "peer-send moved peer-send to c2abcde",
	} {
		s, g := newsTest(t, newsSessions, "%1", "%2")
		g.set("/dev/matchblox-299", "c2abcdef0", "peer-send")
		g.how["/dev/matchblox-299"] = how
		g.diffs["c1 c2abcdef0"] = "a.go\nb.go\n"
		chainSample(s, newsSessions...)
		if got := inbox(s, newsSessions[1]); !slices.Equal(got, []string{want}) {
			t.Errorf("%q: %%2's inbox = %q, want %q", how, got, want)
		}
	}
}

func TestACheckoutThroughTheHookIsNews(t *testing.T) {
	s, g := newsTest(t, newsSessions, "%1", "%2")
	g.set("/dev/matchblox-299", "c9", "fuse")
	ev := hookOf(newsSessions[0], "PostToolUse")
	ev.Command = "git checkout fuse"
	s.Hook(ev)
	if got := inbox(s, newsSessions[1]); !slices.Equal(got, []string{"peer-send switched to fuse"}) {
		t.Fatalf("%%2's inbox = %q", got)
	}
}

// TestAMoveTheSampleNamedFirstIsNotNamedAgain: the hook that comes after
// the sample read the move posts nothing.
func TestAMoveTheSampleNamedFirstIsNotNamedAgain(t *testing.T) {
	s, g := newsTest(t, newsSessions, "%1", "%2")
	g.set("/dev/matchblox-299", "c2", "peer-send")
	g.diffs["c1 c2"] = "a.go\n"
	chainSample(s, newsSessions...)
	if got := inbox(s, newsSessions[1]); !slices.Equal(got, []string{"peer-send committed 1 file in a.go"}) {
		t.Fatalf("%%2's inbox = %q", got)
	}
	ev := hookOf(newsSessions[0], "PostToolUse")
	ev.Command = "git commit -m x"
	s.Hook(ev)
	if got := inbox(s, newsSessions[1]); len(got) != 0 {
		t.Fatalf("the hook named it again: %q", got)
	}
}

// TestNewsFindsAPaneOnTheStokersServer: a hook on the stoker's own tmux
// server names its pane by $TMUX_PANE alone; its socket tells the service
// which pane it is, as for the hook's event.
func TestNewsFindsAPaneOnTheStokersServer(t *testing.T) {
	sessions := slices.Clone(newsSessions[:2])
	sessions[1].Pane = "%2@mbtest"
	s, _ := newsTest(t, sessions, "%1", "%2@mbtest")
	s.StokerServer = "mbtest"
	sock := s.stokerSocket()
	s.PeerPane = func(pid int) string { return map[int]string{100: "%2@mbtest"}[pid] }
	post(s, "%2@mbtest", "peer-send finished")
	if got := newsOver(t, s, 100, proto.News{Pane: "%2", SessionID: "n2", Socket: sock}, true); got != "[matchblox] since your last turn:\n  peer-send finished" {
		t.Fatalf("news = %q", got)
	}
}

// TestSpooledEventsPostNoNews: events read back from the spool are old; a
// finish there is no news of now.
func TestSpooledEventsPostNoNews(t *testing.T) {
	s, _ := newsTest(t, newsSessions, "%1", "%2")
	s.Spool = filepath.Join(t.TempDir(), "spool.jsonl")
	if err := hooks.Append(s.Spool, hookOf(newsSessions[0], "Stop")); err != nil {
		t.Fatal(err)
	}
	s.readSpool()
	if got := inbox(s, newsSessions[1]); len(got) != 0 {
		t.Fatalf("a spooled finish was news: %q", got)
	}
}

// TestAPostedMessageBroadcasts: a console sees the spark of a message at
// once, not at the next sample.
func TestAPostedMessageBroadcasts(t *testing.T) {
	s, _ := newsTest(t, newsSessions, "%1", "%2")
	cl := &client{kick: make(chan struct{}, 1), screenKick: make(chan struct{}, 1)}
	s.join(cl)
	defer s.leave(cl)
	s.mu.Lock()
	c, _ := s.Chains.ChainOf(memberOf(newsSessions[0]))
	s.mu.Unlock()
	s.postMessage(c.ID, newsSessions[1], "[from peer-send %1] hi", time.Now())
	select {
	case <-cl.kick:
	default:
		t.Fatal("no broadcast followed the message")
	}
	s.spark(c.ID, time.Now())
	select {
	case <-cl.kick:
	default:
		t.Fatal("no broadcast followed the spark")
	}
}
