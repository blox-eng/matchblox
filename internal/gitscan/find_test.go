package gitscan

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var active = time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// mkRepo writes the files of a main checkout that Find reads: HEAD, config
// and the reflog, the reflog last touched at active. Empty branch: detached.
func mkRepo(t *testing.T, dir, branch, config string) {
	t.Helper()
	head := "ref: refs/heads/" + branch + "\n"
	if branch == "" {
		head = "3f786850e387550fdab836ed7e6dc881de23001b\n"
	}
	write(t, filepath.Join(dir, ".git", "HEAD"), head)
	write(t, filepath.Join(dir, ".git", "config"), "[core]\n\tbare = false\n"+config)
	logs := filepath.Join(dir, ".git", "logs", "HEAD")
	write(t, logs, "0 1 t <t> 1 +0000\tcommit: a\n")
	if err := os.Chtimes(logs, active, active); err != nil {
		t.Fatal(err)
	}
}

func origin(url string) string {
	return "[remote \"origin\"]\n\turl = " + url + "\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n"
}

// mkWorktree links dir to main as git worktree add does.
func mkWorktree(t *testing.T, main, dir, name, branch string) {
	t.Helper()
	gitdir := filepath.Join(main, ".git", "worktrees", name)
	write(t, filepath.Join(gitdir, "HEAD"), "ref: refs/heads/"+branch+"\n")
	write(t, filepath.Join(gitdir, "commondir"), "../..\n")
	write(t, filepath.Join(gitdir, "logs", "HEAD"), "x\n")
	write(t, filepath.Join(dir, ".git"), "gitdir: "+gitdir+"\n")
}

func tmp(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func find(t *testing.T, roots ...string) []Checkout {
	t.Helper()
	res := Find(context.Background(), roots, FindOpts{Depth: FindDepth})
	if len(res.Errors) > 0 || len(res.Skipped) > 0 || len(res.Hung) > 0 {
		t.Fatalf("errors %v, skipped %v, hung %v", res.Errors, res.Skipped, res.Hung)
	}
	return res.Repos
}

func paths(cs []Checkout) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Path)
	}
	return out
}

func TestFindReadsACheckout(t *testing.T) {
	root := tmp(t)
	mkRepo(t, filepath.Join(root, "a"), "main", origin("git@github.com:o/a.git"))
	got := find(t, root)
	want := []Checkout{{Path: filepath.Join(root, "a"), Branch: "main", Remote: "git@github.com:o/a.git", Active: active}}
	if len(got) != 1 || got[0].Path != want[0].Path || got[0].Branch != want[0].Branch || got[0].Remote != want[0].Remote || !got[0].Active.Equal(active) || got[0].Main != "" {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestFindDetachedHead(t *testing.T) {
	root := tmp(t)
	mkRepo(t, filepath.Join(root, "a"), "", origin("x"))
	if got := find(t, root); len(got) != 1 || got[0].Branch != "" {
		t.Fatalf("got %+v, want one with no branch", got)
	}
}

func TestFindRemoteFallsBackToFirst(t *testing.T) {
	root := tmp(t)
	mkRepo(t, filepath.Join(root, "a"), "main", "[remote \"up\"]\n\turl = https://h/o/up.git\n[remote \"two\"]\n\turl = https://h/o/two.git\n")
	if got := find(t, root); len(got) != 1 || got[0].Remote != "https://h/o/up.git" {
		t.Fatalf("got %+v, want the first remote's URL", got)
	}
}

func TestFindLinkedWorktree(t *testing.T) {
	root := tmp(t)
	main := filepath.Join(root, "app")
	mkRepo(t, main, "main", origin("git@github.com:o/app.git"))
	wt := filepath.Join(root, "wt", "feat")
	mkWorktree(t, main, wt, "feat", "feat")
	got := find(t, root)
	if !slices.Equal(paths(got), []string{main, wt}) {
		t.Fatalf("paths = %v", paths(got))
	}
	if w := got[1]; w.Main != main || w.Branch != "feat" || w.Remote != "git@github.com:o/app.git" {
		t.Fatalf("worktree = %+v", w)
	}
}

func TestFindStopsAtGit(t *testing.T) {
	root := tmp(t)
	mkRepo(t, filepath.Join(root, "a"), "main", "")
	mkRepo(t, filepath.Join(root, "a", "sub"), "main", "")
	if got := paths(find(t, root)); !slices.Equal(got, []string{filepath.Join(root, "a")}) {
		t.Fatalf("got %v", got)
	}
}

func TestFindDescendsIntoARootCheckout(t *testing.T) {
	home := tmp(t)
	mkRepo(t, home, "main", "")
	proj := filepath.Join(home, "git", "proj")
	mkRepo(t, proj, "main", "")
	mkRepo(t, filepath.Join(proj, "sub"), "main", "")
	got := Find(context.Background(), []string{home}, FindOpts{Home: home}).Repos
	if !slices.Equal(paths(got), []string{home, proj}) {
		t.Fatalf("got %v, want the home checkout and %s", paths(got), proj)
	}
}

func TestFindStopsAtARootCheckoutBesidesHome(t *testing.T) {
	home := tmp(t)
	app := filepath.Join(home, "dev", "app")
	mkRepo(t, app, "main", "")
	mkRepo(t, filepath.Join(app, "testdata", "fixture"), "main", "")
	got := Find(context.Background(), []string{home, app}, FindOpts{Home: home}).Repos
	if !slices.Equal(paths(got), []string{app}) {
		t.Fatalf("got %v, want %s alone: a configured repository is not searched below its .git", paths(got), app)
	}
}

func TestFindDepth(t *testing.T) {
	root := tmp(t)
	four := filepath.Join(root, "1", "2", "3", "4")
	mkRepo(t, four, "main", "")
	mkRepo(t, filepath.Join(root, "x", "2", "3", "4", "5"), "main", "")
	if got := paths(find(t, root)); !slices.Equal(got, []string{four}) {
		t.Fatalf("got %v, want depth 4 only", got)
	}
}

func TestFindSkips(t *testing.T) {
	root := tmp(t)
	for _, d := range []string{"node_modules", ".cache", "Library", "vendor", ".venv", "target", ".hidden"} {
		mkRepo(t, filepath.Join(root, d, "r"), "main", "")
	}
	if got := find(t, root); len(got) != 0 {
		t.Fatalf("got %v, want none", paths(got))
	}
}

func TestFindFollowsNoSymlink(t *testing.T) {
	root, other := tmp(t), tmp(t)
	mkRepo(t, filepath.Join(other, "r"), "main", "")
	if err := os.Symlink(other, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if got := find(t, root); len(got) != 0 {
		t.Fatalf("got %v, want none", paths(got))
	}
}

func TestFindSkipsMissingRoot(t *testing.T) {
	root := tmp(t)
	mkRepo(t, filepath.Join(root, "a"), "main", "")
	if got := paths(find(t, filepath.Join(root, "missing"), root)); !slices.Equal(got, []string{filepath.Join(root, "a")}) {
		t.Fatalf("got %v", got)
	}
}

func TestFindOverlappingRoots(t *testing.T) {
	home := tmp(t)
	deep := filepath.Join(home, "dev", "a", "b", "c", "d")
	mkRepo(t, deep, "main", "")
	if got := paths(find(t, home, filepath.Join(home, "dev"))); !slices.Equal(got, []string{deep}) {
		t.Fatalf("got %v, want %s once", got, deep)
	}
}

func TestFindWalksOverlappingRootsOnce(t *testing.T) {
	home := tmp(t)
	if err := os.MkdirAll(filepath.Join(home, "dev", "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	roots := []string{home, filepath.Join(home, "dev")}
	if errs := Find(context.Background(), roots, FindOpts{Depth: FindDepth, MaxDirs: 4}).Errors; len(errs) != 0 {
		t.Fatalf("four folders took more than four reads: %v", errs)
	}
}

func TestFindStopsOnCancel(t *testing.T) {
	root := tmp(t)
	mkRepo(t, filepath.Join(root, "a"), "main", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := Find(ctx, []string{root}, FindOpts{Budget: time.Minute})
	got, errs := res.Repos, res.Errors
	if len(errs) != 1 || !strings.Contains(errs[0], "stopped after 1m0s") || len(got) != 0 {
		t.Fatalf("got %v, errs %v", paths(got), errs)
	}
}

func TestFindMaxDirs(t *testing.T) {
	root := tmp(t)
	for i := range 10 {
		mkRepo(t, filepath.Join(root, fmt.Sprint(i)), "main", "")
	}
	res := Find(context.Background(), []string{root}, FindOpts{Depth: FindDepth, MaxDirs: 3})
	got, errs := res.Repos, res.Errors
	if len(errs) != 1 || !strings.Contains(errs[0], "3 folders") {
		t.Fatalf("errs = %v, want one naming the bound", errs)
	}
	if len(got) == 0 || len(got) >= 10 {
		t.Fatalf("found %d, want what was found before the bound", len(got))
	}
}

func TestStripCredentials(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		// A delimiter before the last @: ambiguous, so only the scheme shows.
		{"https://bot:ab?cd@github.com/org/r.git", "https://"},
		{"https://bot:ab/cd@github.com/org/r.git", "https://"},
		{"https://bot:ab#cd@github.com/org/r.git", "https://"},
		// url.Parse accepts this one, with the secret in the path.
		{"https://bot:123/secret@github.com/o/r.git", "https://"},
		{"https://bot:123?x@github.com/o/r.git", "https://"},
		{"https://x:tok@github.com/o/r.git", "https://github.com/o/r.git"},
		{"https://ghp_tok@github.com/o/r.git", "https://github.com/o/r.git"},
		{"https://user:p@ss@host/r", "https://host/r"},
		{"https://github.com/o/r.git?token=t", "https://github.com/o/r.git"},
		{"HTTPS://x:t@GitHub.com/o/r", "https://GitHub.com/o/r"},
		{"https://x:t@host:8443/o/r", "https://host:8443/o/r"},
		{"https://x:t@[::1]:8443/r", "https://[::1]:8443/r"},
		{"ssh://git:secret@host/r", "ssh://git@host/r"},
		{"git+ssh://git:s@host/r", "git+ssh://git@host/r"},
		{"persistent-https://x:t@host/r", "persistent-https://host/r"},
		{"file:///srv/r.git", "file:///srv/r.git"},
		{"git@github.com:o/r.git", "git@github.com:o/r.git"},
		{"", ""},
		{"no-scheme", "no-scheme"},
		{"ssh://git@h/o/r", "ssh://git@h/o/r"},
		{"git+https://tok@h/o/r", "git+https://h/o/r"},
		{"git://tok@h/o/r", "git://h/o/r"},
		{"https://h?private_token=tok", "https://h"},
		{"https://h/o/r#frag", "https://h/o/r"},
		{"/srv/git/r.git", "/srv/git/r.git"},
		// An @ after the path: the rule cannot tell it from a password's.
		{"https://host/r?x=y@ghp_SECRETTOKEN", "https://"},
		{"https://host/r#user@SECRETTOKEN", "https://"},
		{"https://host/r?u=a@b/token=SECRET", "https://"},
		{"https://host/r?redirect=me@evil.com/path/SECRET", "https://"},
		{"https://host/r?email=a@b.com", "https://"},
		{"https://u:SECRET@host/r?e=a@b", "https://"},
		{"https://host/@scope/r", "https://"},
		// Unparsable, or a host with other than host characters.
		{"https://x:t@h:xyz/r?tok=1#tok", "https://"},
		{"https://a@b=SECRET/r", "https://"},
		// No scheme: text before the first :// that is not a scheme, or no
		// :// at all. A password before the last @ goes with all before it.
		{"user:SECRET@host:o/r?x=http://y", "host:o/r"},
		{"user:SECRET@host:o/r#http://y", "host:o/r"},
		{"u:SECRET@h:o/r?y=a://b", "h:o/r"},
		{"https:u:SECRET@host/r", "host/r"},
		{"https:/u:SECRET@host/r", "host/r"},
		{"u:SECRET@github.com:o/r.git", "github.com:o/r.git"},
		{"git@github.com:o/r.git?x=1#f", "git@github.com:o/r.git"},
	} {
		got := StripCredentials(c.in)
		if got != c.want {
			t.Errorf("StripCredentials(%q) = %q, want %q", c.in, got, c.want)
		}
		// "git@" is an ssh user, which is no secret.
		shown := got
		if strings.Contains(strings.SplitN(c.in, "://", 2)[0], "ssh") || !strings.Contains(c.in, "://") {
			shown = strings.ReplaceAll(got, "git@", "")
		}
		for _, bad := range []string{"SECRET", "tok", "secret", "ab", "cd", "t@"} {
			if strings.Contains(shown, bad) {
				t.Errorf("StripCredentials(%q) = %q, still has %q", c.in, got, bad)
			}
		}
	}
}

func TestFindStripsCredentials(t *testing.T) {
	root := tmp(t)
	mkRepo(t, filepath.Join(root, "a"), "main", origin("https://u:secret@github.com/o/a.git"))
	if got := find(t, root); len(got) != 1 || got[0].Remote != "https://github.com/o/a.git" {
		t.Fatalf("got %+v", got)
	}
}

func TestScanRoots(t *testing.T) {
	home := "/h"
	got := ScanRoots(home, nil, []string{"~/dev/app", "/srv/x"}, []string{"~/w", "/h/src"})
	want := []string{"/h", "/h/dev", "/h/src", "/h/code", "/h/projects", "/h/work", "/h/dev/app", "/srv/x", "/h/w"}
	if !slices.Equal(got, want) {
		t.Fatalf("defaults: got %v, want %v", got, want)
	}
	got = ScanRoots(home, []string{"~/w"}, []string{"/srv/x"}, nil)
	if want := []string{"/h/w", "/srv/x"}; !slices.Equal(got, want) {
		t.Fatalf("picked: got %v, want %v", got, want)
	}
	if got := ScanRoots(home, []string{"dev"}, nil, nil); !slices.Equal(got, []string{"/h/dev"}) {
		t.Fatalf("a relative folder: got %v, want it under the home", got)
	}
}

// fakeFS swaps readDir and resolve for one test, and restores them. reads
// records each folder read.
type fakeFS struct {
	mu    sync.Mutex
	reads map[string]int
}

func swapFS(t *testing.T, rd func(string) ([]os.DirEntry, error), rs func(string) (string, error)) *fakeFS {
	t.Helper()
	f := &fakeFS{reads: map[string]int{}}
	if rd == nil {
		rd = os.ReadDir
	}
	if rs == nil {
		rs = filepath.EvalSymlinks
	}
	readDir = func(dir string) ([]os.DirEntry, error) {
		f.mu.Lock()
		f.reads[dir]++
		f.mu.Unlock()
		return rd(dir)
	}
	resolve = rs
	t.Cleanup(func() { readDir, resolve = os.ReadDir, filepath.EvalSymlinks })
	return f
}

func (f *fakeFS) read(dir string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads[dir]
}

// blockOn makes resolve hang on path until the test ends, and counts its calls.
func blockOn(t *testing.T, path string) (func(string) (string, error), *atomic.Int32) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	var calls atomic.Int32
	return func(p string) (string, error) {
		if p == path {
			calls.Add(1)
			<-release
		}
		return filepath.EvalSymlinks(p)
	}, &calls
}

func mounts(t mountTable) func() mountTable { return func() mountTable { return t } }

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestFindRootResolveIsBounded(t *testing.T) {
	a, dead := tmp(t), tmp(t)
	mkRepo(t, filepath.Join(a, "r"), "main", "")
	rs, _ := blockOn(t, dead)
	swapFS(t, nil, rs)
	o := FindOpts{Budget: 200 * time.Millisecond, Grace: 100 * time.Millisecond}
	start := time.Now()
	res := Find(context.Background(), []string{dead, a}, o)
	if took := time.Since(start); took > o.Limit()+100*time.Millisecond {
		t.Fatalf("Find took %s on a root that never resolves, Limit %s", took, o.Limit())
	}
	if !slices.Equal(res.Hung, []string{dead}) {
		t.Fatalf("Hung = %v, want %s", res.Hung, dead)
	}
	if !slices.Equal(paths(res.Repos), []string{filepath.Join(a, "r")}) {
		t.Fatalf("repos = %v, want the other root's", paths(res.Repos))
	}
}

func TestFindStopsWaitingOnAHungFolder(t *testing.T) {
	root := tmp(t)
	mkRepo(t, filepath.Join(root, "a"), "main", "")
	stuck := filepath.Join(root, "z")
	mkdir(t, stuck)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	swapFS(t, func(dir string) ([]os.DirEntry, error) {
		if dir == stuck {
			<-release
		}
		return os.ReadDir(dir)
	}, nil)
	o := FindOpts{Budget: 200 * time.Millisecond, Grace: 50 * time.Millisecond}
	start := time.Now()
	res := Find(context.Background(), []string{root}, o)
	if took := time.Since(start); took > o.Limit()+100*time.Millisecond {
		t.Fatalf("Find waited %s on a folder that never answered", took)
	}
	if !slices.Equal(paths(res.Repos), []string{filepath.Join(root, "a")}) || len(res.Errors) != 1 ||
		res.Errors[0] != "stopped: a folder did not answer in 250ms: some repositories may be missing" {
		t.Fatalf("got %v, errors %q: want what was found before the hang, and why it stopped", paths(res.Repos), res.Errors)
	}
	if !slices.Equal(res.Hung, []string{stuck}) {
		t.Fatalf("Hung = %v, want %s", res.Hung, stuck)
	}
}

func TestFindSkipsNetworkBelowRoot(t *testing.T) {
	root := tmp(t)
	net := filepath.Join(root, "net")
	mkRepo(t, filepath.Join(net, "r"), "main", "")
	mkRepo(t, filepath.Join(root, "a"), "main", "")
	f := swapFS(t, nil, nil)
	res := Find(context.Background(), []string{root}, FindOpts{Mounts: mounts(mountTable{net: "nfs"})})
	if n := f.read(net); n != 0 {
		t.Fatalf("read the network folder %d times", n)
	}
	if want := []Skip{{net, "a network folder (nfs)"}}; !slices.Equal(res.Skipped, want) || len(res.Errors) != 0 {
		t.Fatalf("skipped %v, errors %v: want %v", res.Skipped, res.Errors, want)
	}
	if !slices.Equal(paths(res.Repos), []string{filepath.Join(root, "a")}) {
		t.Fatalf("repos = %v", paths(res.Repos))
	}
}

func TestFindWalksNetworkRoot(t *testing.T) {
	root := tmp(t)
	mkRepo(t, filepath.Join(root, "r"), "main", "")
	res := Find(context.Background(), []string{root}, FindOpts{Mounts: mounts(mountTable{root: "nfs"})})
	if !slices.Equal(paths(res.Repos), []string{filepath.Join(root, "r")}) || len(res.Skipped) != 0 {
		t.Fatalf("repos %v, skipped %v: a root on nfs is still walked", paths(res.Repos), res.Skipped)
	}
}

func TestFindWalksLocalMount(t *testing.T) {
	root := tmp(t)
	sub := filepath.Join(root, "repos")
	mkRepo(t, filepath.Join(sub, "r"), "main", "")
	res := Find(context.Background(), []string{root}, FindOpts{Mounts: mounts(mountTable{sub: "btrfs"})})
	if !slices.Equal(paths(res.Repos), []string{filepath.Join(sub, "r")}) || len(res.Skipped) != 0 {
		t.Fatalf("repos %v, skipped %v: a local mount is walked", paths(res.Repos), res.Skipped)
	}
}

func TestFindHonorsSkip(t *testing.T) {
	root := tmp(t)
	dead := filepath.Join(root, "dead")
	mkRepo(t, filepath.Join(dead, "r"), "main", "")
	mkRepo(t, filepath.Join(root, "a"), "main", "")
	f := swapFS(t, nil, nil)
	res := Find(context.Background(), []string{root}, FindOpts{Skip: map[string]bool{dead: true}})
	if n := f.read(dead); n != 0 {
		t.Fatalf("read a skipped folder %d times", n)
	}
	if want := []Skip{{dead, "it did not answer before"}}; !slices.Equal(res.Skipped, want) {
		t.Fatalf("skipped %v, want %v", res.Skipped, want)
	}
	if !slices.Equal(paths(res.Repos), []string{filepath.Join(root, "a")}) {
		t.Fatalf("repos = %v", paths(res.Repos))
	}
}

func TestFindHungRootSkippedByWrittenPath(t *testing.T) {
	real := tmp(t)
	r := filepath.Join(tmp(t), "link") // written; resolves to real
	if err := os.Symlink(real, r); err != nil {
		t.Fatal(err)
	}
	rs, calls := blockOn(t, r)
	swapFS(t, nil, rs)
	o := FindOpts{Budget: 100 * time.Millisecond, Grace: 50 * time.Millisecond}
	if res := Find(context.Background(), []string{r}, o); !slices.Equal(res.Hung, []string{r}) {
		t.Fatalf("Hung = %v, want the written %s", res.Hung, r)
	}
	if calls.Load() != 1 {
		t.Fatalf("resolve(r) called %d times, want once", calls.Load())
	}
	o.Skip = map[string]bool{r: true}
	res := Find(context.Background(), []string{r}, o)
	if calls.Load() != 1 {
		t.Fatal("a root in Skip was looked up again")
	}
	if want := []Skip{{r, "it did not answer before"}}; !slices.Equal(res.Skipped, want) || len(res.Hung) != 0 {
		t.Fatalf("skipped %v, hung %v: want %v", res.Skipped, res.Hung, want)
	}
}

func TestFindListsUnreadable(t *testing.T) {
	root := tmp(t)
	locked, gone := filepath.Join(root, "locked"), filepath.Join(root, "gone")
	mkdir(t, locked)
	mkdir(t, gone)
	swapFS(t, func(dir string) ([]os.DirEntry, error) {
		switch dir {
		case locked:
			return nil, &fs.PathError{Op: "open", Path: dir, Err: fs.ErrPermission}
		case gone:
			return nil, &fs.PathError{Op: "open", Path: dir, Err: fs.ErrNotExist}
		}
		return os.ReadDir(dir)
	}, nil)
	res := Find(context.Background(), []string{root}, FindOpts{})
	if want := []string{"cannot read " + locked + ": permission denied"}; !slices.Equal(res.Errors, want) {
		t.Fatalf("errors %q, want %q", res.Errors, want)
	}
}

func TestFindListsUnresolvableRoot(t *testing.T) {
	root := tmp(t)
	bad := filepath.Join(root, "bad")
	swapFS(t, nil, func(p string) (string, error) {
		if p == bad {
			return "", &fs.PathError{Op: "lstat", Path: p, Err: fs.ErrPermission}
		}
		return filepath.EvalSymlinks(p)
	})
	res := Find(context.Background(), []string{bad, filepath.Join(root, "missing"), root}, FindOpts{})
	if want := []string{"cannot find root " + bad + ": permission denied"}; !slices.Equal(res.Errors, want) {
		t.Fatalf("errors %q, want %q: a missing root is not listed", res.Errors, want)
	}
}

func TestFindErrorsCapped(t *testing.T) {
	root := tmp(t)
	for i := range 30 {
		mkdir(t, filepath.Join(root, fmt.Sprintf("d%02d", i)))
	}
	swapFS(t, func(dir string) ([]os.DirEntry, error) {
		if dir != root {
			return nil, fs.ErrPermission
		}
		return os.ReadDir(dir)
	}, nil)
	res := Find(context.Background(), []string{root}, FindOpts{})
	if len(res.Errors) != 20 || res.Errors[19] != "and 11 more" || res.Errors[0] != "cannot read "+filepath.Join(root, "d00")+": permission denied" {
		t.Fatalf("%d errors, %q", len(res.Errors), res.Errors)
	}
}

func TestFindSkippedCapped(t *testing.T) {
	root := tmp(t)
	skip := map[string]bool{}
	for i := range 503 {
		p := filepath.Join(root, fmt.Sprintf("d%03d", i))
		mkdir(t, p)
		skip[p] = true
	}
	res := Find(context.Background(), []string{root}, FindOpts{Skip: skip, Depth: 1})
	if len(res.Skipped) != 500 || res.SkippedMore != 3 {
		t.Fatalf("%d skipped, %d more", len(res.Skipped), res.SkippedMore)
	}
}

func TestFindProtectedBelowRootOnly(t *testing.T) {
	root := tmp(t)
	docs := filepath.Join(root, "Documents")
	code := filepath.Join(docs, "code")
	mkRepo(t, code, "main", "")
	o := FindOpts{Protected: []string{docs}}
	res := Find(context.Background(), []string{root, filepath.Join(root, "other")}, o)
	if want := []Skip{{docs, "macOS asks first; p picks it"}}; !slices.Equal(res.Skipped, want) || len(res.Repos) != 0 || len(res.Errors) != 0 {
		t.Fatalf("repos %v, skipped %v, errors %v: want %v alone", paths(res.Repos), res.Skipped, res.Errors, want)
	}
	for _, r := range []string{code, docs} {
		res = Find(context.Background(), []string{r}, o)
		if !slices.Equal(paths(res.Repos), []string{code}) || len(res.Skipped) != 0 {
			t.Fatalf("root %s: repos %v, skipped %v: a root in a protected folder is walked", r, paths(res.Repos), res.Skipped)
		}
	}
	// Reached from a root above, it is skipped whichever root that is.
	res = Find(context.Background(), []string{code, root}, o)
	if !slices.Equal(paths(res.Repos), []string{code}) || !slices.Equal(res.Skipped, []Skip{{docs, "macOS asks first; p picks it"}}) {
		t.Fatalf("repos %v, skipped %v", paths(res.Repos), res.Skipped)
	}
}

func TestFindProtectedUnderSymlinkedHome(t *testing.T) {
	real := tmp(t)
	h := filepath.Join(tmp(t), "h")
	if err := os.Symlink(real, h); err != nil {
		t.Fatal(err)
	}
	mkRepo(t, filepath.Join(real, "Documents", "r"), "main", "")
	protected := DefaultProtected(h)
	if len(protected) == 0 {
		protected = []string{filepath.Join(h, "Documents")}
	}
	res := Find(context.Background(), []string{h}, FindOpts{Home: h, Protected: protected})
	if want := []Skip{{filepath.Join(real, "Documents"), "macOS asks first; p picks it"}}; !slices.Equal(res.Skipped, want) || len(res.Repos) != 0 {
		t.Fatalf("repos %v, skipped %v: want %v", paths(res.Repos), res.Skipped, want)
	}
}

func TestDefaultProtected(t *testing.T) {
	got := DefaultProtected("/h")
	if runtime.GOOS != "darwin" {
		if len(got) != 0 {
			t.Fatalf("got %v off macOS", got)
		}
		return
	}
	want := []string{"/h/Documents", "/h/Desktop", "/h/Downloads", "/h/Library", "/h/Pictures", "/h/Movies", "/h/Music"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFindStaleWorktreeGitFile(t *testing.T) {
	root := tmp(t)
	write(t, filepath.Join(root, "a", ".git"), "gitdir: "+filepath.Join(root, "gone", ".git", "worktrees", "a")+"\n")
	mkRepo(t, filepath.Join(root, "b"), "main", "")
	if got := paths(find(t, root)); !slices.Equal(got, []string{filepath.Join(root, "b")}) {
		t.Fatalf("got %v, want the sibling alone and no error", got)
	}
}

func TestFindSymlinkedHome(t *testing.T) {
	real := tmp(t)
	h := filepath.Join(tmp(t), "h")
	if err := os.Symlink(real, h); err != nil {
		t.Fatal(err)
	}
	mkRepo(t, filepath.Join(real, "dev", "a"), "main", "")
	if got := paths(find(t, h)); !slices.Equal(got, []string{filepath.Join(real, "dev", "a")}) {
		t.Fatalf("got %v, want the resolved path", got)
	}
}

func TestLimit(t *testing.T) {
	if got := (FindOpts{Budget: time.Second}).Limit(); got != 6*time.Second {
		t.Fatalf("default grace: %s", got)
	}
	if got := (FindOpts{Budget: time.Second, Grace: time.Millisecond}).Limit(); got != time.Second+time.Millisecond {
		t.Fatalf("grace: %s", got)
	}
}

func TestFindSkipsRootByResolvedPath(t *testing.T) {
	real := tmp(t)
	mkRepo(t, real, "main", "")
	link := filepath.Join(tmp(t), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	f := swapFS(t, nil, nil)
	res := Find(context.Background(), []string{link}, FindOpts{Skip: map[string]bool{real: true}})
	if f.read(real) != 0 || len(res.Repos) != 0 || !slices.Equal(res.Skipped, []Skip{{real, "it did not answer before"}}) {
		t.Fatalf("reads %d, repos %v, skipped %v", f.read(real), paths(res.Repos), res.Skipped)
	}
}

func TestFoundSame(t *testing.T) {
	at := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	base := func() Found {
		return Found{Consent: "yes", Roots: []string{"/h"}, At: at, Took: time.Second,
			Repos:   []Checkout{{Path: "/h/a", Branch: "main", Active: at}},
			Errors:  []string{"e"},
			Skipped: []Skip{{Path: "/h/n", Why: whyHung}}}
	}
	a, b := base(), base()
	b.At, b.Took = at.Add(time.Hour), 2*time.Second
	if !a.Same(b) {
		t.Fatal("only At and Took differ: want same")
	}
	b = base()
	b.Repos[0].Branch = "dev"
	if a.Same(b) {
		t.Fatal("a branch differs: want different")
	}
	b = base()
	b.SkippedMore = 1
	if a.Same(b) {
		t.Fatal("SkippedMore differs: want different")
	}
	b = base()
	b.Consent = "no"
	if a.Same(b) {
		t.Fatal("Consent differs: want different")
	}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var rt Found
	if err := json.Unmarshal(raw, &rt); err != nil {
		t.Fatal(err)
	}
	if !a.Same(rt) {
		t.Fatalf("a JSON round trip differs: %+v vs %+v", a, rt)
	}
}

func BenchmarkFoundSame(b *testing.B) {
	at := time.Now()
	mk := func() Found {
		f := Found{Consent: "yes"}
		for i := range 5000 {
			f.Repos = append(f.Repos, Checkout{Path: fmt.Sprintf("/h/r%05d", i), Branch: "main", Remote: "https://x/y", Active: at})
		}
		return f
	}
	x, y := mk(), mk()
	b.ResetTimer()
	for range b.N {
		if !x.Same(y) {
			b.Fatal("want same")
		}
	}
}

// FuzzStripCredentials puts a marker password in a userinfo u:p, in the
// shapes a remote takes, and checks the marker never comes out.
func FuzzStripCredentials(f *testing.F) {
	for _, s := range [][3]string{
		{"https", "u", ""},
		{"ssh", "git", "a@b"},
		{"http", "user", "x?y#z/w"},
		{"a", "u", "//"},
		{"git+ssh", "", ":"},
		{"https", "u", "?x=http://y"},
	} {
		f.Add(s[0], s[1], s[2])
	}
	const mark = "PWMARK"
	f.Fuzz(func(t *testing.T, scheme, user, pass string) {
		p := pass + mark + pass
		ui := user + ":" + p
		for _, in := range []string{
			scheme + "://" + ui + "@host/path?q=1#f",
			scheme + "://" + ui + "@host:22/o/r.git",
			ui + "@host:o/r.git",
			ui + "@host:o/r?x=http://y#f",
			scheme + ":" + ui + "@host/r",
			scheme + ":/" + ui + "@host/r",
		} {
			if strings.Contains(strings.Replace(in, mark, "", 1), mark) {
				continue // the marker also outside the password: not a leak to find
			}
			got := StripCredentials(in)
			if !strings.Contains(got, mark) {
				continue
			}
			// An ssh URL keeps its user: an input that names the marker as
			// that user, before the first ':' of its userinfo, says so.
			if sch, rest, ok := strings.Cut(in, "://"); ok {
				switch strings.ToLower(sch) {
				case "ssh", "git+ssh", "ssh+git":
					if u, _, _ := strings.Cut(rest[:max(strings.LastIndex(rest, "@"), 0)], ":"); strings.Contains(u, mark) {
						continue
					}
				}
			}
			t.Fatalf("StripCredentials(%q) = %q, has the password", in, got)
		}
	})
}

func TestFindListsARootThatDidNotAnswer(t *testing.T) {
	a, dead := tmp(t), tmp(t)
	mkRepo(t, filepath.Join(a, "r"), "main", "")
	mkRepo(t, filepath.Join(dead, "r"), "main", "")
	rs, _ := blockOn(t, dead)
	swapFS(t, nil, rs)
	o := FindOpts{Budget: 200 * time.Millisecond, Grace: 100 * time.Millisecond}
	res := Find(context.Background(), []string{dead, a}, o)
	if !slices.Equal(res.Hung, []string{dead}) {
		t.Fatalf("Hung = %v, want %s", res.Hung, dead)
	}
	if want := []Skip{{dead, "it did not answer"}}; !slices.Equal(res.Skipped, want) {
		t.Fatalf("skipped %v, want %v: a root that did not answer is not silent", res.Skipped, want)
	}
	if !res.Skipped[0].Unanswered() {
		t.Fatal("Unanswered() false for a root that did not answer")
	}
}

func TestFindListsAFolderThatHungMidWalk(t *testing.T) {
	root := tmp(t)
	stuck := filepath.Join(root, "z")
	mkdir(t, stuck)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	swapFS(t, func(dir string) ([]os.DirEntry, error) {
		if dir == stuck {
			<-release
		}
		return os.ReadDir(dir)
	}, nil)
	o := FindOpts{Budget: 200 * time.Millisecond, Grace: 50 * time.Millisecond}
	res := Find(context.Background(), []string{root}, o)
	if want := []Skip{{stuck, "it did not answer"}}; !slices.Equal(res.Skipped, want) {
		t.Fatalf("skipped %v, want %v", res.Skipped, want)
	}
}

func TestSkipUnanswered(t *testing.T) {
	for why, want := range map[string]bool{
		"it did not answer": true, "it did not answer before": true,
		"macOS asks first; p picks it": false, "a network folder (nfs)": false,
	} {
		if got := (Skip{Path: "/x", Why: why}).Unanswered(); got != want {
			t.Errorf("%q: Unanswered() = %v, want %v", why, got, want)
		}
	}
}

// answersTo reports whether dir/name exists: on a case-insensitive
// volume, a folder in dir answers to name in another case.
func answersTo(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func TestFindRootInAnotherCaseIsOneCheckout(t *testing.T) {
	h := tmp(t)
	mkRepo(t, filepath.Join(h, "Code", "app"), "main", "")
	res := Find(context.Background(), ScanRoots(h, nil, nil, nil), FindOpts{Home: h})
	got := paths(res.Repos)
	if !answersTo(h, "code") {
		if !slices.Equal(got, []string{filepath.Join(h, "Code", "app")}) {
			t.Fatalf("repos %v", got)
		}
		t.Skip("case-sensitive volume: no second spelling to collapse")
	}
	if want := []string{filepath.Join(h, "Code", "app")}; !slices.Equal(got, want) {
		t.Fatalf("repos %v, want %v: one checkout, in the case on disk", got, want)
	}
}

func TestFindProtectedBelowRootInAnotherCase(t *testing.T) {
	base := tmp(t)
	h := filepath.Join(base, "Home")
	mkRepo(t, filepath.Join(h, "Documents", "secret"), "main", "")
	mkRepo(t, filepath.Join(h, "dev", "app"), "main", "")
	o := FindOpts{Home: h, Protected: []string{filepath.Join(h, "Documents")}}
	if !answersTo(base, "hOME") {
		t.Skip("case-sensitive volume: hOME is another folder")
	}
	want := []string{filepath.Join(h, "dev", "app")}
	wantSkip := []Skip{{filepath.Join(h, "Documents"), whyProtected}}
	for _, root := range []string{h, filepath.Join(base, "hOME")} {
		res := Find(context.Background(), []string{root}, o)
		if got := paths(res.Repos); !slices.Equal(got, want) || !slices.Equal(res.Skipped, wantSkip) {
			t.Fatalf("root %s: repos %v, skipped %v: want %v, %v", root, got, res.Skipped, want, wantSkip)
		}
	}
}
