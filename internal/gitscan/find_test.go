package gitscan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	got, errs := Find(context.Background(), roots, FindOpts{Depth: FindDepth})
	if len(errs) > 0 {
		t.Fatalf("errs: %v", errs)
	}
	return got
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
	got, _ := Find(context.Background(), []string{home}, FindOpts{Home: home})
	if !slices.Equal(paths(got), []string{home, proj}) {
		t.Fatalf("got %v, want the home checkout and %s", paths(got), proj)
	}
}

func TestFindStopsAtARootCheckoutBesidesHome(t *testing.T) {
	home := tmp(t)
	app := filepath.Join(home, "dev", "app")
	mkRepo(t, app, "main", "")
	mkRepo(t, filepath.Join(app, "testdata", "fixture"), "main", "")
	got, _ := Find(context.Background(), []string{home, app}, FindOpts{Home: home})
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
	if _, errs := Find(context.Background(), roots, FindOpts{Depth: FindDepth, MaxDirs: 4}); len(errs) != 0 {
		t.Fatalf("four folders took more than four reads: %v", errs)
	}
}

func TestFindStopsOnCancel(t *testing.T) {
	root := tmp(t)
	mkRepo(t, filepath.Join(root, "a"), "main", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, errs := Find(ctx, []string{root}, FindOpts{Budget: time.Minute})
	if len(errs) != 1 || !strings.Contains(errs[0], "stopped after 1m0s") || len(got) != 0 {
		t.Fatalf("got %v, errs %v", paths(got), errs)
	}
}

func TestFindMaxDirs(t *testing.T) {
	root := tmp(t)
	for i := range 10 {
		mkRepo(t, filepath.Join(root, fmt.Sprint(i)), "main", "")
	}
	got, errs := Find(context.Background(), []string{root}, FindOpts{Depth: FindDepth, MaxDirs: 3})
	if len(errs) != 1 || !strings.Contains(errs[0], "3 folders") {
		t.Fatalf("errs = %v, want one naming the bound", errs)
	}
	if len(got) == 0 || len(got) >= 10 {
		t.Fatalf("found %d, want what was found before the bound", len(got))
	}
}

func TestStripCredentials(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"https://bot:ab?cd@github.com/org/r.git", "https://github.com/org/r.git"},
		{"https://bot:ab/cd@github.com/org/r.git", "https://github.com/org/r.git"},
		{"https://bot:ab#cd@github.com/org/r.git", "https://github.com/org/r.git"},
		// url.Parse accepts this one, with the secret in the path.
		{"https://bot:123/secret@github.com/o/r.git", "https://github.com/o/r.git"},
		{"https://bot:123?x@github.com/o/r.git", "https://github.com/o/r.git"},
		{"https://x:tok@github.com/o/r.git", "https://github.com/o/r.git"},
		{"https://ghp_tok@github.com/o/r.git", "https://github.com/o/r.git"},
		{"https://github.com/o/r.git?token=t", "https://github.com/o/r.git"},
		{"HTTPS://x:t@GitHub.com/o/r", "HTTPS://GitHub.com/o/r"},
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
		// Unparsable: the fallback still drops the userinfo and the query.
		{"https://x:t@h:xyz/r?tok=1#tok", "https://h:xyz/r"},
	} {
		got := StripCredentials(c.in)
		// The scheme's case is url's to choose; the host and the path are not.
		if got != c.want && !(strings.HasPrefix(c.in, "HTTPS://") && got == "https://GitHub.com/o/r") {
			t.Errorf("StripCredentials(%q) = %q, want %q", c.in, got, c.want)
		}
		// "git@" is an ssh user, which is no secret.
		for _, bad := range []string{"tok", "secret", "ab", "cd", "t@"} {
			if strings.Contains(strings.ReplaceAll(got, "git@", ""), bad) {
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
