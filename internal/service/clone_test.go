package service

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/clone"
	"github.com/blox-eng/matchblox/internal/gitremote"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/transport"
)

// cloneTest is a service whose home is a temp folder, with clone_root
// ~/src and a seeded random source.
func cloneTest(t *testing.T) (*Service, string) {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	s, _ := lifeTest(t)
	s.CloneRoot = filepath.Join(home, "src")
	s.UsedPath = filepath.Join(t.TempDir(), "used.json")
	s.cloneRand = rand.New(rand.NewPCG(1, 2))
	return s, s.CloneRoot
}

func gitIn(t *testing.T, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		t.Fatalf("git %q: %s %s", args, err, out)
	}
}

// repoWith makes a repo at dir with origin url ("" for none).
func repoWith(t *testing.T, dir, url string) {
	t.Helper()
	gitIn(t, "init", "-q", dir)
	if url != "" {
		gitIn(t, "-C", dir, "remote", "add", "origin", url)
	}
}

func plan(t *testing.T, s *Service, text string) proto.ClonePlan {
	t.Helper()
	res := s.act(t.Context(), consoleAct("clone:plan", text))
	if res.Err != "" || res.Clone == nil {
		t.Fatalf("plan %q: err %q clone %+v", text, res.Err, res.Clone)
	}
	return *res.Clone
}

func freeIn(t *testing.T, root, repo, got string) {
	t.Helper()
	w, ok := strings.CutPrefix(got, filepath.Join(root, repo)+"-")
	if !ok || !slices.Contains(clone.Words, w) {
		t.Fatalf("%q is not a free name for %s in %s", got, repo, root)
	}
}

func TestClonePlanFreeTarget(t *testing.T) {
	s, root := cloneTest(t)
	p := plan(t, s, "https://github.com/o/r")
	want := proto.ClonePlan{Repo: "o/r", URL: "https://github.com/o/r", Target: filepath.Join(root, "r")}
	if p.Repo != want.Repo || p.URL != want.URL || p.Target != want.Target || p.Taken != "" || p.Have != nil || p.Fresh != "" {
		t.Fatalf("plan %+v, want %+v", p, want)
	}
}

func TestClonePlanSameRepoAtTarget(t *testing.T) {
	s, root := cloneTest(t)
	repoWith(t, filepath.Join(root, "r"), "git@github.com:o/r.git")
	p := plan(t, s, "https://github.com/O/r.git")
	if p.Target != filepath.Join(root, "r") || p.Taken != "" {
		t.Fatalf("plan %+v", p)
	}
	if len(p.Have) == 0 || p.Have[0] != filepath.Join(root, "r") {
		t.Fatalf("have %q", p.Have)
	}
	freeIn(t, root, "r", p.Fresh)
}

func TestClonePlanAnotherRepoAtTarget(t *testing.T) {
	s, root := cloneTest(t)
	repoWith(t, filepath.Join(root, "r"), "https://github.com/x/r")
	p := plan(t, s, "https://github.com/o/r")
	if p.Taken != "another repo" || p.Have != nil || p.Fresh != "" {
		t.Fatalf("plan %+v", p)
	}
	freeIn(t, root, "r", p.Target)

	// a folder that is no repo, and a repo with no remote
	if err := os.RemoveAll(filepath.Join(root, "r")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "r"), 0o755); err != nil {
		t.Fatal(err)
	}
	if p := plan(t, s, "https://github.com/o/r"); p.Taken != "not a repo" {
		t.Fatalf("a plain folder: %+v", p)
	}
	repoWith(t, filepath.Join(root, "r"), "")
	if p := plan(t, s, "https://github.com/o/r"); p.Taken != "another repo" {
		t.Fatalf("a repo with no remote: %+v", p)
	}
}

func TestClonePlanFileAtTarget(t *testing.T) {
	s, root := cloneTest(t)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "r"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	p := plan(t, s, "https://github.com/o/r")
	if p.Taken != "a file" {
		t.Fatalf("plan %+v", p)
	}
	freeIn(t, root, "r", p.Target)
}

func TestClonePlanKnownElsewhere(t *testing.T) {
	s, root := cloneTest(t)
	dev := filepath.Join(filepath.Dir(root), "dev")
	repoWith(t, filepath.Join(dev, "r"), "git@github.com:o/r.git")
	repoWith(t, filepath.Join(dev, "other"), "git@github.com:o/other.git")
	repoWith(t, filepath.Join(dev, "used-r"), "https://github.com/o/r")
	s.mu.Lock()
	s.found = gitscan.Found{Consent: "yes", Repos: []gitscan.Checkout{
		{Path: filepath.Join(dev, "r"), Remote: "git@github.com:o/r.git"},
		{Path: filepath.Join(dev, "other"), Remote: "git@github.com:o/other.git"},
	}}
	s.mu.Unlock()
	p := plan(t, s, "https://github.com/o/r")
	if !slices.Equal(p.Have, []string{filepath.Join(dev, "r")}) || p.Target != filepath.Join(root, "r") || p.Taken != "" {
		t.Fatalf("plan %+v", p)
	}
	// a place in the used record is known too, read from its disk
	s.addUsed(filepath.Join(dev, "used-r"), filepath.Join(dev, "other"))
	p = plan(t, s, "https://github.com/o/r")
	if want := []string{filepath.Join(dev, "r"), filepath.Join(dev, "used-r")}; !slices.Equal(p.Have, want) {
		t.Fatalf("have %q, want %q", p.Have, want)
	}
}

// fakeGit puts a git on PATH that logs its arguments to the returned file
// and then runs body.
func fakeGit(t *testing.T, body string) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "git.log")
	script := "#!/bin/sh\necho \"$*\" >> '" + log + "'\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func TestClonePlanIsOffline(t *testing.T) {
	s, root := cloneTest(t)
	repoWith(t, filepath.Join(root, "r"), "git@github.com:o/r.git")
	log := fakeGit(t, "exit 1")
	plan(t, s, "https://github.com/o/r")
	plan(t, s, "git@github.com:x/r.git")
	b, _ := os.ReadFile(log)
	for _, net := range []string{"fetch", "ls-remote", "clone", "pull", "remote update"} {
		if strings.Contains(string(b), net) {
			t.Fatalf("a plan ran git %s:\n%s", net, b)
		}
	}
}

func TestClonePlanRefusesAPath(t *testing.T) {
	s, _ := cloneTest(t)
	for _, text := range []string{"~/src/r", "/src/r", "./r", "", "file:///src/r"} {
		res := s.act(t.Context(), consoleAct("clone:plan", text))
		if res.Err != "not a git URL" || res.Clone != nil {
			t.Fatalf("%q: err %q clone %+v", text, res.Err, res.Clone)
		}
	}
}

func startAct(t *testing.T, url, target string) proto.Act {
	t.Helper()
	b, err := json.Marshal(proto.CloneStart{URL: url, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	return consoleAct("clone:start", string(b))
}

// clonesUntil waits until ok holds for the snapshot's clones.
func clonesUntil(t *testing.T, s *Service, what string, ok func([]proto.Clone) bool) []proto.Clone {
	t.Helper()
	deadline := time.Now().Add(waitLimit)
	for {
		st, _ := s.stateFor(&client{})
		if ok(st.Clones) {
			return st.Clones
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: clones %+v", what, st.Clones)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCloneStartRunsAndReports(t *testing.T) {
	s, root := cloneTest(t)
	src := filepath.Join(t.TempDir(), "src-r")
	repoWith(t, src, "")
	gitIn(t, "-C", src, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "x")
	s.cloneRemote = func(string) (gitremote.Remote, bool) {
		return gitremote.Remote{URL: src, Host: "github.com", Owner: "o", Repo: "r"}, true
	}
	target := filepath.Join(root, "r")
	if res := s.act(t.Context(), startAct(t, "https://github.com/o/r", target)); res.Err != "" {
		t.Fatal(res.Err)
	}
	cs := clonesUntil(t, s, "the clone is listed", func(cs []proto.Clone) bool { return len(cs) == 1 })
	if cs[0].Target != target || cs[0].Repo != "o/r" {
		t.Fatalf("listed %+v", cs)
	}
	clonesUntil(t, s, "the clone is done", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Done && cs[0].Err == "" })
	if _, err := os.Stat(filepath.Join(target, ".git")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(clone.Cloning(target)); !os.IsNotExist(err) {
		t.Fatalf("the sibling is left: %v", err)
	}
	if got := usedPaths(state.ReadUsed(s.UsedPath)); !slices.Equal(got, []string{target}) {
		t.Fatalf("used %q", got)
	}
	// a done clone stays listed a minute, then goes
	now := time.Now()
	s.Now = func() time.Time { return now.Add(61 * time.Second) }
	if st, _ := s.stateFor(&client{}); len(st.Clones) != 0 {
		t.Fatalf("still listed: %+v", st.Clones)
	}
}

func TestCloneStartRefusesAnExistingTarget(t *testing.T) {
	s, root := cloneTest(t)
	target := filepath.Join(root, "r")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	log := fakeGit(t, "exit 1")
	res := s.act(t.Context(), startAct(t, "https://github.com/o/r", target))
	if res.Err != "~/src/r is there already" {
		t.Fatalf("err %q", res.Err)
	}
	if ents, _ := os.ReadDir(target); len(ents) != 0 {
		t.Fatalf("written into: %v", ents)
	}
	if b, _ := os.ReadFile(log); len(b) != 0 {
		t.Fatalf("ran git %s", b)
	}
	// nor outside clone_root, nor a hidden name
	for _, bad := range []string{filepath.Join(filepath.Dir(root), "r"), filepath.Join(root, "x", "r"), filepath.Join(root, ".r.cloning"), root + "/.."} {
		if res := s.act(t.Context(), startAct(t, "https://github.com/o/r", bad)); res.Err == "" {
			t.Fatalf("%s: not refused", bad)
		}
	}
	if res := s.act(t.Context(), startAct(t, "/src/r", filepath.Join(root, "q"))); res.Err != "not a git URL" {
		t.Fatalf("a path: %q", res.Err)
	}
}

// slowGit is a git clone that makes its folder, says 10% and waits.
const slowGit = `for a; do last=$a; done
mkdir -p "$last"
printf 'Receiving objects:  10%% (1/10)\r' >&2
exec sleep 30`

func TestCloneStartTwiceRefusesTheSecond(t *testing.T) {
	s, root := cloneTest(t)
	fakeGit(t, slowGit)
	target := filepath.Join(root, "r")
	if res := s.act(t.Context(), startAct(t, "https://github.com/o/r", target)); res.Err != "" {
		t.Fatal(res.Err)
	}
	t.Cleanup(func() { s.act(t.Context(), consoleAct("clone:stop", target)) })
	if res := s.act(t.Context(), startAct(t, "https://github.com/o/r", target)); res.Err != "~/src/r is being cloned" {
		t.Fatalf("err %q", res.Err)
	}
	// a plan meanwhile does not offer the name being cloned
	p := plan(t, s, "https://github.com/o/r")
	freeIn(t, root, "r", p.Target)
}

func TestCloneStopDeletesThePartial(t *testing.T) {
	s, root := cloneTest(t)
	fakeGit(t, slowGit)
	target := filepath.Join(root, "r")
	if res := s.act(t.Context(), startAct(t, "https://github.com/o/r", target)); res.Err != "" {
		t.Fatal(res.Err)
	}
	clonesUntil(t, s, "10%", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Pct == 10 })
	if _, err := os.Stat(clone.Cloning(target)); err != nil {
		t.Fatalf("no partial: %v", err)
	}
	if res := s.act(t.Context(), consoleAct("clone:stop", "~/src/r")); res.Err != "" {
		t.Fatal(res.Err)
	}
	clonesUntil(t, s, "stopped", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Err == "stopped" && !cs[0].Done })
	for _, p := range []string{target, clone.Cloning(target)} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatalf("%s is left: %v", p, err)
		}
	}
	if got := usedPaths(state.ReadUsed(s.UsedPath)); len(got) != 0 {
		t.Fatalf("used %q", got)
	}
	// a stopped one may start again
	fakeGit(t, "exit 1")
	if res := s.act(t.Context(), startAct(t, "https://github.com/o/r", target)); res.Err != "" {
		t.Fatalf("again: %q", res.Err)
	}
	clonesUntil(t, s, "failed", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Err != "" && cs[0].Err != "stopped" })
}

func TestCloneFromAnAgentsPaneIsRefused(t *testing.T) {
	s, root := cloneTest(t)
	for _, a := range []proto.Act{
		{RecID: "clone:start", Text: `{"url":"https://github.com/o/r","target":"` + filepath.Join(root, "r") + `"}`, From: "%3"},
		{RecID: "clone:plan", Text: "https://github.com/o/r", From: "%3"},
	} {
		if res := s.act(t.Context(), a); res.Err != notTheBuilders {
			t.Fatalf("%s: err %q", a.RecID, res.Err)
		}
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("clone_root made: %v", err)
	}
	// the builder's own shell plans
	if res := s.act(t.Context(), proto.Act{RecID: "clone:plan", Text: "https://github.com/o/r", From: "%4"}); res.Err != "" {
		t.Fatal(res.Err)
	}
}

func TestCloneDotNamedRepoPlansAndStarts(t *testing.T) {
	s, root := cloneTest(t)
	fakeGit(t, "exit 1")
	p := plan(t, s, "https://github.com/o/.github")
	if p.Target != filepath.Join(root, ".github") || p.Taken != "" {
		t.Fatalf("plan %+v", p)
	}
	if res := s.act(t.Context(), startAct(t, p.URL, p.Target)); res.Err != "" {
		t.Fatalf("start: %q", res.Err)
	}
	clonesUntil(t, s, "ended", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Err != "" })
}

func TestCloneStaleSiblingFreesItsName(t *testing.T) {
	s, root := cloneTest(t)
	target := filepath.Join(root, "r")
	stale := clone.Cloning(target)
	if err := os.MkdirAll(filepath.Join(stale, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := plan(t, s, "https://github.com/o/r")
	if p.Target != target || p.Taken != "" {
		t.Fatalf("plan %+v", p)
	}
	if _, err := os.Lstat(stale); !os.IsNotExist(err) {
		t.Fatalf("the stale sibling is left: %v", err)
	}
	// start alone clears one too
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeGit(t, "exit 1")
	if res := s.act(t.Context(), startAct(t, "https://github.com/o/r", target)); res.Err != "" {
		t.Fatalf("start: %q", res.Err)
	}
	clonesUntil(t, s, "ended", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Err != "" })
	// a link named as the sibling is never followed nor removed
	other := t.TempDir()
	if err := os.Symlink(other, stale); err != nil {
		t.Fatal(err)
	}
	if p := plan(t, s, "https://github.com/o/r"); p.Target == target {
		t.Fatalf("a link freed the name: %+v", p)
	}
	if _, err := os.Lstat(stale); err != nil {
		t.Fatalf("the link is gone: %v", err)
	}
}

func TestClonePlanKeepsARunningSibling(t *testing.T) {
	s, root := cloneTest(t)
	fakeGit(t, slowGit)
	target := filepath.Join(root, "r")
	if res := s.act(t.Context(), startAct(t, "https://github.com/o/r", target)); res.Err != "" {
		t.Fatal(res.Err)
	}
	t.Cleanup(func() { s.act(t.Context(), consoleAct("clone:stop", target)) })
	clonesUntil(t, s, "10%", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Pct == 10 })
	if p := plan(t, s, "https://github.com/o/r"); p.Taken != "being cloned" {
		t.Fatalf("plan %+v", p)
	}
	if _, err := os.Lstat(clone.Cloning(target)); err != nil {
		t.Fatalf("a plan removed a running clone's folder: %v", err)
	}
}

func TestShutdownStopsClonesAndCleansUp(t *testing.T) {
	s, root := cloneTest(t)
	fakeGit(t, slowGit)
	target := filepath.Join(root, "r")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if res := s.act(ctx, startAct(t, "https://github.com/o/r", target)); res.Err != "" {
		t.Fatal(res.Err)
	}
	clonesUntil(t, s, "10%", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Pct == 10 })
	s.stopClones(waitLimit)
	if _, err := os.Lstat(clone.Cloning(target)); !os.IsNotExist(err) {
		t.Fatalf("the partial is left: %v", err)
	}
}

// urlRepo makes a local repo with a commit and points the URL parser at it.
func urlRepo(t *testing.T, s *Service) {
	t.Helper()
	src := filepath.Join(t.TempDir(), "src-r")
	repoWith(t, src, "")
	gitIn(t, "-C", src, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "x")
	s.cloneRemote = func(text string) (gitremote.Remote, bool) {
		if text != "https://github.com/o/r" {
			return gitremote.Remote{}, false
		}
		return gitremote.Remote{URL: "https://github.com/o/r", Host: "github.com", Owner: "o", Repo: "r"}, true
	}
	s.cloneRun = func(ctx context.Context, _ gitremote.Remote, target string, _ func(int)) error {
		return exec.CommandContext(ctx, "git", "clone", "-q", src, target).Run()
	}
}

func TestNewSessionClonesAURL(t *testing.T) {
	s, root := cloneTest(t)
	urlRepo(t, s)
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: "https://github.com/o/r", Agent: "claude", Name: "n1"}))
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	target := filepath.Join(root, "r")
	if _, err := os.Stat(filepath.Join(target, ".git")); err != nil {
		t.Fatal(err)
	}
	if len(res.Ran) != 1 || !strings.Contains(strings.Join(res.Ran[0], " "), "-c "+target+" ") {
		t.Fatalf("ran %q", res.Ran)
	}
	if len(res.Skipped) == 0 || res.Skipped[0] != "cloned o/r into ~/src/r" {
		t.Fatalf("skipped %q", res.Skipped)
	}
	if got := usedPaths(state.ReadUsed(s.UsedPath)); !slices.Contains(got, target) {
		t.Fatalf("used %q", got)
	}
}

func TestNewSessionURLUsesTheExistingClone(t *testing.T) {
	s, root := cloneTest(t)
	urlRepo(t, s)
	target := filepath.Join(root, "r")
	repoWith(t, target, "https://github.com/o/r")
	var ran bool
	s.cloneRun = func(context.Context, gitremote.Remote, string, func(int)) error { ran = true; return nil }
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: "https://github.com/o/r", Agent: "claude", Name: "n1"}))
	if res.Err != "" || ran {
		t.Fatalf("err %q cloned %v", res.Err, ran)
	}
	if len(res.Ran) != 1 || !strings.Contains(strings.Join(res.Ran[0], " "), "-c "+target+" ") {
		t.Fatalf("ran %q", res.Ran)
	}
	if len(res.Skipped) == 0 || res.Skipped[0] != "o/r is already at ~/src/r: started there" {
		t.Fatalf("skipped %q", res.Skipped)
	}
}

func TestNewSessionURLStopsWhenTheCLILeaves(t *testing.T) {
	s, root := cloneTest(t)
	urlRepo(t, s)
	started := make(chan struct{})
	stopped := make(chan struct{})
	s.cloneRun = func(ctx context.Context, _ gitremote.Remote, target string, _ func(int)) error {
		close(started)
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	}
	x, y := net.Pipe()
	go s.HandlePeer(t.Context(), transport.NewConn(y), 0) //nolint:errcheck // ends with the test
	c := transport.NewConn(x)
	defer c.Close()
	go func() { _ = c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version, Client: proto.ClientCLI}) }()
	next(t, c, proto.KindHello)
	b, _ := json.Marshal(proto.NewSession{Where: "https://github.com/o/r", Agent: "claude", Name: "n1"})
	go func() {
		_ = c.Send(proto.KindAct, "x", proto.Act{RecID: "new:session", Confirm: "y", Text: string(b)})
	}()
	select {
	case <-started:
	case <-time.After(waitLimit):
		t.Fatal("the clone did not start")
	}
	c.Close()
	select {
	case <-stopped:
	case <-time.After(waitLimit):
		t.Fatal("the clone went on after the CLI left")
	}
	clonesUntil(t, s, "the clone ended", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Err != "" })
	for _, p := range []string{filepath.Join(root, "r"), clone.Cloning(filepath.Join(root, "r"))} {
		if exists(p) {
			t.Fatalf("%s is left", p)
		}
	}
}

func TestNewSessionPathStillWorks(t *testing.T) {
	s, _ := cloneTest(t)
	urlRepo(t, s)
	dir := t.TempDir()
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: dir, Agent: "claude", Name: "fresh"}))
	if res.Err != "" || len(res.Ran) != 1 || len(res.Skipped) != 0 {
		t.Fatalf("%+v", res)
	}
	if got := strings.Join(res.Ran[0], " "); !strings.HasSuffix(got, "-c "+dir+" claude --name fresh") {
		t.Fatalf("ran %q", got)
	}
	if res := s.act(t.Context(), newAct(t, proto.NewSession{Where: "api", Agent: "claude"})); res.Err != "a new session starts in a full path" {
		t.Fatalf("%q", res.Err)
	}
}
