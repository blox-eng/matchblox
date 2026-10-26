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

// A repo the Git tab shows is known too, with no scan consent: its
// checkout is read now, as the found ones are.
func TestClonePlanKnowsTheGitTabsRepos(t *testing.T) {
	s, _ := cloneTest(t)
	dev := filepath.Join(filepath.Dir(s.CloneRoot), "dev")
	repoWith(t, filepath.Join(dev, "r"), "git@github.com:o/r.git")
	repoWith(t, filepath.Join(dev, "other"), "git@github.com:o/other.git")
	s.mu.Lock()
	s.cur.Git = &gitscan.Report{Repos: []gitscan.Repo{{Path: filepath.Join(dev, "r")}, {Path: filepath.Join(dev, "other")}}}
	s.mu.Unlock()
	if p := plan(t, s, "o/r"); !slices.Equal(p.Have, []string{filepath.Join(dev, "r")}) {
		t.Fatalf("plan %+v", p)
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

// Each start of a folder is a clone of its own id, and the listing
// carries the id of the one that runs there now.
func TestCloneStartsHaveTheirOwnIDs(t *testing.T) {
	s, root := cloneTest(t)
	fakeGit(t, "exit 1")
	target := filepath.Join(root, "r")
	first := s.act(t.Context(), startAct(t, "https://github.com/o/r", target))
	if first.Err != "" || first.CloneID == 0 {
		t.Fatalf("first %+v", first)
	}
	clonesUntil(t, s, "the first failed", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Err != "" && cs[0].ID == first.CloneID })
	second := s.act(t.Context(), startAct(t, "https://github.com/o/r", target))
	if second.Err != "" || second.CloneID == 0 || second.CloneID == first.CloneID {
		t.Fatalf("first %d, second %+v", first.CloneID, second)
	}
	clonesUntil(t, s, "the second is listed", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].ID == second.CloneID })
}

// A start's token is in its clone's record, so a console that lost the
// answer knows its own clone; a start with none, from an older console,
// starts all the same.
func TestCloneStartTokenIsListed(t *testing.T) {
	s, root := cloneTest(t)
	fakeGit(t, "exit 1")
	b, err := json.Marshal(proto.CloneStart{URL: "https://github.com/o/r", Target: filepath.Join(root, "r"), Token: "t0k3n"})
	if err != nil {
		t.Fatal(err)
	}
	res := s.act(t.Context(), consoleAct("clone:start", string(b)))
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	clonesUntil(t, s, "the token is listed", func(cs []proto.Clone) bool {
		return len(cs) == 1 && cs[0].ID == res.CloneID && cs[0].Token == "t0k3n"
	})
	old := s.act(t.Context(), startAct(t, "https://github.com/o/q", filepath.Join(root, "q")))
	if old.Err != "" {
		t.Fatal(old.Err)
	}
	clonesUntil(t, s, "the tokenless start is listed", func(cs []proto.Clone) bool {
		i := slices.IndexFunc(cs, func(c proto.Clone) bool { return c.ID == old.CloneID })
		return i >= 0 && cs[i].Token == ""
	})
}

func TestCloneFromAnAgentsPaneIsRefused(t *testing.T) {
	s, root := cloneTest(t)
	for _, a := range []proto.Act{
		{RecID: "clone:start", Text: `{"url":"https://github.com/o/r","target":"` + filepath.Join(root, "r") + `"}`, From: "%3"},
		{RecID: "clone:plan", Text: "https://github.com/o/r", From: "%3"},
		// a console hello does not make an agent's pane a person's (R16)
		{RecID: "clone:plan", Text: "https://github.com/o/r", From: "%3", Console: true},
		{RecID: "clone:stop", Text: filepath.Join(root, "r"), From: "%3", Console: true},
		{RecID: NewSessionID, Confirm: "y", Text: `{"where":"https://github.com/o/r","agent":"claude"}`, From: "%3", Console: true},
	} {
		if res := s.act(t.Context(), a); res.Err != noClone {
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
	if len(res.Skipped) != 0 || res.Note != "cloned o/r into ~/src/r" {
		t.Fatalf("skipped %q note %q", res.Skipped, res.Note)
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
	if len(res.Skipped) != 0 || res.Note != "o/r is already at ~/src/r: started there" {
		t.Fatalf("skipped %q note %q", res.Skipped, res.Note)
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
	if res.Err != "" || len(res.Ran) != 1 || len(res.Skipped) != 0 || res.Note != "" {
		t.Fatalf("%+v", res)
	}
	if got := strings.Join(res.Ran[0], " "); !strings.HasSuffix(got, "-c "+dir+" claude --name fresh") {
		t.Fatalf("ran %q", got)
	}
	if res := s.act(t.Context(), newAct(t, proto.NewSession{Where: "api", Agent: "claude"})); res.Err != "a new session starts in a full path" {
		t.Fatalf("%q", res.Err)
	}
}

// noCloneRun fails the test if a clone runs.
func noCloneRun(t *testing.T, s *Service) {
	t.Helper()
	s.cloneRemote = func(text string) (gitremote.Remote, bool) { return gitremote.Parse(text, "https") }
	s.cloneRun = func(context.Context, gitremote.Remote, string, func(int)) error {
		t.Error("a clone ran")
		return nil
	}
}

// The stoker and the sessions it drives never clone through new:session.
func TestStokerNewSessionNeverClones(t *testing.T) {
	s, ran := handsLife(t)
	s.CloneRoot = t.TempDir()
	noCloneRun(t, s)
	for _, where := range []string{"o/r", "https://github.com/o/r", "gh:o/r"} {
		b, _ := json.Marshal(proto.NewSession{Where: where, Agent: "claude", Name: "n1"})
		if res := s.act(t.Context(), fromStokerAct(NewSessionID, string(b))); res.Err != noClone {
			t.Errorf("%s from the stoker: err %q", where, res.Err)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

// A new session that will be refused never clones first.
func TestNewSessionURLChecksBeforeItClones(t *testing.T) {
	s, _ := cloneTest(t)
	noCloneRun(t, s)
	for why, n := range map[string]proto.NewSession{
		"aider is not an agent matchblox starts: claude, codex, opencode or cursor-agent": {Where: "o/r", Agent: "aider"},
		"opencode is not installed here":                           {Where: "o/r", Agent: "opencode"},
		"a first message does not start with -":                    {Where: "o/r", Agent: "claude", First: "--x"},
		"a tmux session is named dev already: choose another name": {Where: "o/r", Agent: "claude", Name: "dev"},
	} {
		if res := s.act(t.Context(), newAct(t, n)); res.Err != why {
			t.Errorf("want %q, got %q", why, res.Err)
		}
	}
	if exists(s.CloneRoot) {
		t.Fatal("clone_root made")
	}
}

// Cursor asks whether to trust a folder just cloned from a URL; a
// checkout already there, reused for a URL, and a path the builder typed
// it trusts.
func TestNewSessionURLLetsCursorAsk(t *testing.T) {
	s, root := cloneTest(t)
	urlRepo(t, s)
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: "https://github.com/o/r", Agent: "cursor-agent", Name: "n1"}))
	if res.Err != "" || len(res.Ran) != 1 {
		t.Fatalf("%+v", res)
	}
	if got := strings.Join(res.Ran[0], " "); !strings.HasSuffix(got, "-c "+filepath.Join(root, "r")+" cursor-agent") {
		t.Fatalf("ran %q", got)
	}
	// the same URL again: the checkout is there, reused
	gitIn(t, "-C", filepath.Join(root, "r"), "remote", "set-url", "origin", "https://github.com/o/r")
	res = s.act(t.Context(), newAct(t, proto.NewSession{Where: "https://github.com/o/r", Agent: "cursor-agent", Name: "n2"}))
	if res.Err != "" || len(res.Ran) != 1 || !strings.HasPrefix(res.Note, "o/r is already at") || !slices.Contains(res.Ran[0], "--trust") {
		t.Fatalf("reused: %+v", res)
	}
	res = s.act(t.Context(), newAct(t, proto.NewSession{Where: filepath.Join(root, "r"), Agent: "cursor-agent", Name: "n3"}))
	if res.Err != "" || len(res.Ran) != 1 || !slices.Contains(res.Ran[0], "--trust") {
		t.Fatalf("a typed path: %+v", res)
	}
}

// A folder the console cloned for this session is one Cursor asks about,
// though it comes as a path.
func TestNewSessionClonedFolderLetsCursorAsk(t *testing.T) {
	s, ran := lifeTest(t)
	dir := t.TempDir()
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: dir, Agent: "cursor-agent", Name: "n1", Cloned: true}))
	if res.Err != "" || len(*ran) != 1 {
		t.Fatalf("%+v ran %q", res, *ran)
	}
	if slices.Contains((*ran)[0], "--trust") {
		t.Fatalf("ran %q", (*ran)[0])
	}
}

// On a disk that ignores case, .Foo.cloning is .foo.cloning: a plan for
// o/Foo leaves the running clone of o/foo alone.
func TestClonePlanKeepsARunningSiblingOfAnotherCase(t *testing.T) {
	s, root := cloneTest(t)
	fakeGit(t, slowGit)
	target := filepath.Join(root, "foo")
	if res := s.act(t.Context(), startAct(t, "https://github.com/o/foo", target)); res.Err != "" {
		t.Fatal(res.Err)
	}
	t.Cleanup(func() { s.act(t.Context(), consoleAct("clone:stop", target)) })
	clonesUntil(t, s, "10%", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Pct == 10 })
	other := filepath.Join(root, "Foo")
	if err := os.MkdirAll(clone.Cloning(other), 0o755); err != nil { // one folder where case is ignored
		t.Fatal(err)
	}
	if p := plan(t, s, "https://github.com/o/Foo"); p.Taken != "being cloned" || p.Target == other {
		t.Fatalf("plan %+v", p)
	}
	if res := s.act(t.Context(), startAct(t, "https://github.com/o/Foo", other)); res.Err != "~/src/Foo is being cloned" {
		t.Fatalf("start: %q", res.Err)
	}
	for _, p := range []string{clone.Cloning(target), clone.Cloning(other)} {
		if _, err := os.Lstat(p); err != nil {
			t.Fatalf("%s removed: %v", p, err)
		}
	}
}

// A found checkout is offered only if its .git still names the repo.
func TestClonePlanReadsAFoundCheckoutNow(t *testing.T) {
	s, root := cloneTest(t)
	dev := filepath.Join(filepath.Dir(root), "dev")
	repoWith(t, filepath.Join(dev, "r"), "https://github.com/x/r")
	s.mu.Lock()
	s.found = gitscan.Found{Consent: "yes", Repos: []gitscan.Checkout{{Path: filepath.Join(dev, "r"), Remote: "git@github.com:o/r.git"}}}
	s.mu.Unlock()
	if p := plan(t, s, "https://github.com/o/r"); len(p.Have) != 0 {
		t.Fatalf("offered a checkout of another repo: %+v", p)
	}
}

func stopAct(t *testing.T, target string, id int64) proto.Act {
	t.Helper()
	b, err := json.Marshal(proto.CloneStop{Target: target, ID: id})
	if err != nil {
		t.Fatal(err)
	}
	return consoleAct("clone:stop", string(b))
}

// A stop names the clone it stops: another clone into the same folder
// runs on.
func TestCloneStopOfAnotherClone(t *testing.T) {
	s, root := cloneTest(t)
	fakeGit(t, slowGit)
	target := filepath.Join(root, "r")
	res := s.act(t.Context(), startAct(t, "https://github.com/o/r", target))
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	t.Cleanup(func() { s.act(t.Context(), stopAct(t, target, res.CloneID)) })
	clonesUntil(t, s, "10%", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Pct == 10 })
	got := s.act(t.Context(), stopAct(t, target, res.CloneID+1))
	if got.Err != "" || !slices.Equal(got.Skipped, []string{"that clone is not running"}) {
		t.Fatalf("%+v", got)
	}
	if cs := clonesUntil(t, s, "listed", func(cs []proto.Clone) bool { return len(cs) == 1 }); cs[0].Err != "" || cs[0].Done {
		t.Fatalf("stopped: %+v", cs)
	}
	// its own id stops it
	if got := s.act(t.Context(), stopAct(t, "~/src/r", res.CloneID)); got.Err != "" || len(got.Skipped) != 0 {
		t.Fatalf("%+v", got)
	}
	clonesUntil(t, s, "stopped", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Err == "stopped" })
}

// A stop that comes when the clone is done says so: the folder stays.
func TestCloneStopOfADoneClone(t *testing.T) {
	s, root := cloneTest(t)
	fakeGit(t, `for a; do last=$a; done
mkdir -p "$last"`)
	target := filepath.Join(root, "r")
	res := s.act(t.Context(), startAct(t, "https://github.com/o/r", target))
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	clonesUntil(t, s, "done", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Done })
	for _, a := range []proto.Act{stopAct(t, target, res.CloneID), consoleAct("clone:stop", target)} {
		got := s.act(t.Context(), a)
		if got.Err != "" || len(got.Skipped) != 0 || got.Note != "the clone of o/r was done: it is in ~/src/r" {
			t.Fatalf("%s: %+v", a.Text, got)
		}
	}
	if !exists(target) {
		t.Fatal("the folder is gone")
	}
	if got := usedPaths(state.ReadUsed(s.UsedPath)); !slices.Equal(got, []string{target}) {
		t.Fatalf("used %q", got)
	}
}

// A bare target, from an older console, stops the clone there.
func TestCloneStopOfABareTarget(t *testing.T) {
	s, root := cloneTest(t)
	fakeGit(t, slowGit)
	target := filepath.Join(root, "r")
	if res := s.act(t.Context(), startAct(t, "https://github.com/o/r", target)); res.Err != "" {
		t.Fatal(res.Err)
	}
	clonesUntil(t, s, "10%", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Pct == 10 })
	if got := s.act(t.Context(), consoleAct("clone:stop", target)); got.Err != "" || len(got.Skipped) != 0 || got.Note != "" {
		t.Fatalf("%+v", got)
	}
	clonesUntil(t, s, "stopped", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Err == "stopped" })
}

// A clone that failed: a stop is a quiet no-op, it ages out, and nothing
// is on disk.
func TestCloneThatFailed(t *testing.T) {
	s, root := cloneTest(t)
	fakeGit(t, "exit 1")
	target := filepath.Join(root, "r")
	res := s.act(t.Context(), startAct(t, "https://github.com/o/r", target))
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	clonesUntil(t, s, "failed", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Err != "" })
	if got := s.act(t.Context(), stopAct(t, target, res.CloneID)); got.Err != "" || len(got.Skipped) != 0 || got.Note != "" {
		t.Fatalf("%+v", got)
	}
	for _, p := range []string{target, clone.Cloning(target)} {
		if exists(p) {
			t.Fatalf("%s is there", p)
		}
	}
	now := time.Now()
	s.Now = func() time.Time { return now.Add(61 * time.Second) }
	if st, _ := s.stateFor(&client{}); len(st.Clones) != 0 {
		t.Fatalf("still listed: %+v", st.Clones)
	}
}

// A running clone is listed however long it runs.
func TestARunningCloneNeverAgesOut(t *testing.T) {
	s, root := cloneTest(t)
	fakeGit(t, slowGit)
	target := filepath.Join(root, "r")
	if res := s.act(t.Context(), startAct(t, "https://github.com/o/r", target)); res.Err != "" {
		t.Fatal(res.Err)
	}
	t.Cleanup(func() { s.act(t.Context(), consoleAct("clone:stop", target)) })
	now := time.Now()
	s.Now = func() time.Time { return now.Add(time.Hour) }
	if st, _ := s.stateFor(&client{}); len(st.Clones) != 1 {
		t.Fatalf("listed %+v", st.Clones)
	}
}

// The service's end leaves a done clone's folder.
func TestShutdownKeepsADoneClone(t *testing.T) {
	s, root := cloneTest(t)
	fakeGit(t, `for a; do last=$a; done
mkdir -p "$last"`)
	target := filepath.Join(root, "r")
	if res := s.act(t.Context(), startAct(t, "https://github.com/o/r", target)); res.Err != "" {
		t.Fatal(res.Err)
	}
	clonesUntil(t, s, "done", func(cs []proto.Clone) bool { return len(cs) == 1 && cs[0].Done })
	s.stopClones(waitLimit)
	if !exists(target) {
		t.Fatal("the folder is gone")
	}
}

func TestNoCloneRoot(t *testing.T) {
	s, root := cloneTest(t)
	s.CloneRoot = ""
	log := fakeGit(t, "exit 1")
	if res := s.act(t.Context(), consoleAct("clone:plan", "https://github.com/o/r")); res.Err != "no clone_root is set" {
		t.Fatalf("plan: %q", res.Err)
	}
	if res := s.act(t.Context(), startAct(t, "https://github.com/o/r", filepath.Join(root, "r"))); res.Err != "no clone_root is set" {
		t.Fatalf("start: %q", res.Err)
	}
	if res := s.act(t.Context(), newAct(t, proto.NewSession{Where: "https://github.com/o/r", Agent: "claude", Name: "n1"})); res.Err != "no clone_root is set" {
		t.Fatalf("new: %q", res.Err)
	}
	if b, _ := os.ReadFile(log); len(b) != 0 {
		t.Fatalf("ran git %s", b)
	}
}

// fakeGh puts a gh on PATH that logs its arguments and says protocol.
func fakeGh(t *testing.T, s *Service, protocol string) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "gh.log")
	script := "#!/bin/sh\necho \"$*\" >> '" + log + "'\necho " + protocol + "\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil { //nolint:gosec // a test script
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	s.LookPath = exec.LookPath
	return log
}

func TestGitProtocol(t *testing.T) {
	s, _ := cloneTest(t)
	fakeGh(t, s, "ssh")
	if p := plan(t, s, "o/r"); p.URL != "git@github.com:o/r.git" {
		t.Fatalf("gh says ssh: %+v", p)
	}

	s, _ = cloneTest(t)
	s.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	if p := plan(t, s, "o/r"); p.URL != "https://github.com/o/r.git" {
		t.Fatalf("no gh: %+v", p)
	}

	s, _ = cloneTest(t)
	log := fakeGh(t, s, "ssh")
	if p := plan(t, s, "https://github.com/o/r"); p.URL != "https://github.com/o/r" {
		t.Fatalf("a full URL: %+v", p)
	}
	if b, _ := os.ReadFile(log); len(b) != 0 {
		t.Fatalf("a full URL asked gh: %s", b)
	}
}

func TestTildeIn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for in, want := range map[string]string{
		home + "/x":                          "~/x",
		home + "x":                           home + "x",
		home:                                 "~",
		home + "/a and " + home + "/b":       "~/a and ~/b",
		"cannot make " + home + ": no space": "cannot make ~: no space",
		"'" + home + "/r' is there":          "'~/r' is there",
		"/elsewhere/x":                       "/elsewhere/x",
	} {
		if got := tildeIn(in); got != want {
			t.Errorf("tildeIn(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for in, want := range map[string]string{
		"~":       home,
		"~/a":     filepath.Join(home, "a"),
		"src":     filepath.Join(home, "src"),
		"/abs/x/": "/abs/x",
		"~a":      filepath.Join(home, "~a"),
	} {
		if got := ExpandHome(in); got != want {
			t.Errorf("ExpandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

// A used.json that does not read is kept aside, not written over.
func TestABrokenUsedFileIsKept(t *testing.T) {
	s, _ := cloneTest(t)
	if err := os.WriteFile(s.UsedPath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.addUsed("/a")
	if b, err := os.ReadFile(s.UsedPath + ".broken"); err != nil || string(b) != "{not json" {
		t.Fatalf("broken: %q %v", b, err)
	}
	if got := usedPaths(state.ReadUsed(s.UsedPath)); !slices.Equal(got, []string{"/a"}) {
		t.Fatalf("used %q", got)
	}
	// a good file is never moved
	s.addUsed("/b")
	if b, _ := os.ReadFile(s.UsedPath + ".broken"); string(b) != "{not json" {
		t.Fatalf("broken: %q", b)
	}
}
