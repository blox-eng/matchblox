package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
)

// repoWithWorktree makes a repository at main with one commit and a
// worktree of it at wt.
func repoWithWorktree(t *testing.T, main, wt string) {
	t.Helper()
	gitIn(t, "init", "-q", main)
	gitIn(t, "-C", main, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "x")
	gitIn(t, "-C", main, "worktree", "add", "-q", wt)
}

func TestWorkspaceLine(t *testing.T) {
	root := realTemp(t)
	main, wt := filepath.Join(root, "main"), filepath.Join(root, "wt")
	other, otherWT := filepath.Join(root, "other"), filepath.Join(root, "otherwt")
	repoWithWorktree(t, main, wt)
	repoWithWorktree(t, other, otherWT)
	if err := os.Mkdir(filepath.Join(wt, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(wt, link); err != nil {
		t.Fatal(err)
	}
	notOurs := func(p string) string { return p + " is not a worktree of this repository: it starts in " + main }

	fakes := realTemp(t)
	gitFile, gitLink, fakeWT := filepath.Join(fakes, "gitfile"), filepath.Join(fakes, "gitlink"), filepath.Join(fakes, "fakewt")
	for _, d := range []string{gitFile, gitLink, fakeWT} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for d, gitdir := range map[string]string{gitFile: filepath.Join(main, ".git"), fakeWT: filepath.Join(main, ".git", "worktrees", "wt")} {
		if err := os.WriteFile(filepath.Join(d, ".git"), []byte("gitdir: "+gitdir+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(main, ".git"), filepath.Join(gitLink, ".git")); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ name, record, work, refused string }{
		{"relative", "Workspace: relative/x\n", main, notOurs("relative/x")},
		{"outside Git", "Workspace: /tmp\n", main, notOurs("/tmp")},
		{"another repository's worktree", "Workspace: " + otherWT + "\n", main, notOurs(otherWT)},
		{"climbs out", "Workspace: " + wt + "/../../..\n", main, notOurs(wt + "/../../..")},
		{"inside .git", "Workspace: " + filepath.Join(main, ".git") + "\n", main, notOurs(filepath.Join(main, ".git"))},
		{"a .git file pointing at the repository", "Workspace: " + gitFile + "\n", main, notOurs(gitFile)},
		{"a .git link to the repository", "Workspace: " + gitLink + "\n", main, notOurs(gitLink)},
		{"a copy pretending to be the worktree", "Workspace: " + fakeWT + "\n", main, notOurs(fakeWT)},
		{"quoted", "Workspace: \"" + wt + "\"\n", main, notOurs("\"" + wt + "\"")},
		{"worktree", "Workspace: " + wt + "\n", wt, ""},
		{"subfolder names its top level", "Workspace: " + filepath.Join(wt, "sub") + "\n", wt, ""},
		{"link resolves", "Workspace: " + link + "\n", wt, ""},
		{"no line", "# Task record\n", main, ""},
		{"first line counts", "Workspace: " + wt + "\nWorkspace: " + otherWT + "\n", wt, ""},
		{"indented is no line", "  Workspace: " + wt + "\n", main, ""},
		{"quoted mail is no line", "> Workspace: " + wt + "\n", main, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			work, refused := workFolder(t.Context(), nil, tc.record, main)
			if work != tc.work || refused != tc.refused {
				t.Fatalf("workFolder(%q) = %q, %q; want %q, %q", tc.record, work, refused, tc.work, tc.refused)
			}
		})
	}

	sub := filepath.Join(wt, "sub")
	for name, record := range map[string]string{"its own subfolder": "Workspace: " + sub + "\n", "its worktree": "Workspace: " + wt + "\n"} {
		t.Run("a subfolder launch naming "+name+" keeps the subfolder", func(t *testing.T) {
			if work, refused := workFolder(t.Context(), nil, record, sub); work != sub || refused != "" {
				t.Fatalf("got %q, %q; want %q", work, refused, sub)
			}
		})
	}

	t.Run("the caller's GIT_DIR is ignored", func(t *testing.T) {
		t.Setenv("GIT_DIR", filepath.Join(main, ".git"))
		if work, refused := workFolder(t.Context(), nil, "Workspace: /tmp\n", main); work != main || refused != notOurs("/tmp") {
			t.Fatalf("got %q, %q", work, refused)
		}
	})

	t.Run("empty Git output is refused", func(t *testing.T) {
		quiet := func(context.Context, string, ...string) (string, error) { return "", nil }
		work, refused := workFolder(t.Context(), quiet, "Workspace: "+wt+"\n", main)
		if work != main || refused != notOurs(wt) {
			t.Fatalf("got %q, %q", work, refused)
		}
	})

	plain := filepath.Join(realTemp(t), "plain")
	if err := os.Mkdir(plain, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Run("outside Git the launch folder passes", func(t *testing.T) {
		if work, refused := workFolder(t.Context(), nil, "Workspace: "+plain+"\n", plain); work != plain || refused != "" {
			t.Fatalf("got %q, %q", work, refused)
		}
	})
	t.Run("outside Git any other folder is refused", func(t *testing.T) {
		want := wt + " is not a worktree of this repository: it starts in " + plain
		if work, refused := workFolder(t.Context(), nil, "Workspace: "+wt+"\n", plain); work != plain || refused != want {
			t.Fatalf("got %q, %q", work, refused)
		}
	})
}

func TestSessionsCarryTheirWorkFolder(t *testing.T) {
	root := realTemp(t)
	main, wt := filepath.Join(root, "main"), filepath.Join(root, "wt")
	repoWithWorktree(t, main, wt)
	s := newTest(t, time.Hour)
	var runs atomic.Int32
	s.WorkGit = func(ctx context.Context, dir string, args ...string) (string, error) {
		runs.Add(1)
		return workGit(ctx, dir, args...)
	}
	session := func(pane, bundle, workspace string) sample.Session {
		dir := filepath.Join(s.matchContextRoot(), bundle)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "handover.md"), []byte("Workspace: "+workspace+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return sample.Session{Pane: pane, Agent: "claude", Cwd: main, MatchContext: filepath.Join(dir, "awareness.md")}
	}
	sampled := []sample.Session{session("%1", "inwt", wt), session("%2", "intmp", "/tmp")}
	merge := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.sampled = sampled
		s.mergeQueue()
	}

	merge()
	if n := runs.Load(); n != 0 {
		t.Fatalf("mergeQueue ran %d Git commands; want none", n)
	}
	go s.workLoop(t.Context())
	s.works.wait()
	filled := runs.Load()
	if filled == 0 {
		t.Fatal("the fill ran no Git command")
	}
	s.mu.Lock()
	got := slices.Clone(s.cur.Sessions)
	s.mu.Unlock()
	refused := "/tmp is not a worktree of this repository: it starts in " + main
	if got[0].Work != wt || got[0].WorkRefused != "" {
		t.Errorf("worktree record: Work %q, WorkRefused %q; want %q, none", got[0].Work, got[0].WorkRefused, wt)
	}
	if got[1].Work != main || got[1].WorkRefused != refused {
		t.Errorf("/tmp record: Work %q, WorkRefused %q; want %q, %q", got[1].Work, got[1].WorkRefused, main, refused)
	}
	if sampled[0].Work != "" || sampled[1].Work != "" {
		t.Error("the fill wrote into the sampled sessions")
	}

	merge()
	s.works.wait()
	if n := runs.Load(); n != filled {
		t.Errorf("an unchanged record ran %d more Git commands; want none", n-filled)
	}
	s.mu.Lock()
	again := slices.Clone(s.cur.Sessions)
	s.mu.Unlock()
	if again[0].Work != wt || again[1].WorkRefused != refused {
		t.Errorf("after a second merge: %q, %q", again[0].Work, again[1].WorkRefused)
	}
}

// A worktree added after its record named it shows within 30 s with no
// new write: an older answer is asked again, and it stays shown until the
// new one lands.
func TestAWorktreeAddedLaterShowsWithin30s(t *testing.T) {
	root := realTemp(t)
	main, wt := filepath.Join(root, "main"), filepath.Join(root, "wt")
	gitIn(t, "init", "-q", main)
	gitIn(t, "-C", main, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "x")
	s := newTest(t, time.Hour)
	var mu sync.Mutex
	clock := time.Now()
	s.Now = func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }
	dir := filepath.Join(s.matchContextRoot(), "later")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "handover.md"), []byte("Workspace: "+wt+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sampled := []sample.Session{{Pane: "%1", Agent: "claude", Cwd: main, MatchContext: filepath.Join(dir, "awareness.md")}}
	merge := func() string {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.sampled = sampled
		s.mergeQueue()
		return s.cur.Sessions[0].Work
	}
	work := func() string {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.cur.Sessions[0].Work
	}
	go s.workLoop(t.Context())
	merge()
	s.works.wait()
	if got := work(); got != main {
		t.Fatalf("Work %q before the worktree; want %q", got, main)
	}
	gitIn(t, "-C", main, "worktree", "add", "-q", wt)
	mu.Lock()
	clock = clock.Add(31 * time.Second)
	mu.Unlock()
	if got := merge(); got != main {
		t.Errorf("Work %q while Git is asked again; want the older %q", got, main)
	}
	s.works.wait()
	if got := work(); got != wt {
		t.Fatalf("Work %q 31 s on; want %q", got, wt)
	}
}

// TestARefusedWorkspaceShowsNoControlKeys: the refusal quotes an
// agent-written path that holds control keys or runs long, so no escape
// reaches the builder's terminal or the confirm.
func TestARefusedWorkspaceShowsNoControlKeys(t *testing.T) {
	launch := realTemp(t)
	for name, path := range map[string]string{
		"an escape":  "/x\x1b]52;c;cm0gLXJmIH4=\x07\x1b[2J",
		"a new line": "/x\nswitched it anyway",
		"a long one": "/" + strings.Repeat("a", 2000),
	} {
		_, refused := workFolder(t.Context(), nil, "Workspace: "+path+"\n", launch)
		if strings.ContainsAny(refused, "\x1b\x07\n\r") || len(refused) > 600 {
			t.Errorf("%s: refused %q, want no control keys and at most 600 bytes", name, refused)
		}
		if !strings.HasSuffix(refused, ": it starts in "+launch) {
			t.Errorf("%s: refused %q lost its words", name, refused)
		}
	}
}

// TestAWorktreeWithControlKeysIsRefused: a real worktree whose name holds
// an escape never reaches the confirm or the CLI as it is.
func TestAWorktreeWithControlKeysIsRefused(t *testing.T) {
	root := realTemp(t)
	main, wt := filepath.Join(root, "main"), filepath.Join(root, "wt\x1b[2K")
	repoWithWorktree(t, main, filepath.Join(root, "plain"))
	gitIn(t, "-C", main, "worktree", "add", "-q", "-b", "x", wt)
	work, refused := workFolder(t.Context(), nil, "Workspace: "+wt+"\n", main)
	if work != main || refused == "" || strings.Contains(refused, "\x1b") {
		t.Fatalf("workFolder = %q, %q; want %q and a refusal with no ESC", work, refused, main)
	}
}

// TestWorkspaceLineOnOldGit: Git before 2.31 has no --path-format, and it
// answers --git-common-dir relative to the folder it ran in.
func TestWorkspaceLineOnOldGit(t *testing.T) {
	root := realTemp(t)
	main, wt := filepath.Join(root, "main"), filepath.Join(root, "wt")
	other, otherWT := filepath.Join(root, "other"), filepath.Join(root, "otherwt")
	repoWithWorktree(t, main, wt)
	repoWithWorktree(t, other, otherWT)
	if err := os.Mkdir(filepath.Join(wt, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	old := func(ctx context.Context, dir string, args ...string) (string, error) {
		if slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "--path-format") }) {
			t.Errorf("Git 2.30 knows no %q", args)
			return "", errors.New("unknown option")
		}
		out, err := workGit(ctx, dir, args...)
		if err != nil || !slices.Contains(args, "--git-common-dir") || !filepath.IsAbs(strings.TrimSpace(out)) {
			return out, err
		}
		rel, err := filepath.Rel(dir, strings.TrimSpace(out))
		return rel + "\n", err
	}
	for _, tc := range []struct{ name, path, launch, work, refused string }{
		{"its worktree", wt, main, wt, ""},
		{"a subfolder of it", filepath.Join(wt, "sub"), main, wt, ""},
		{"the main folder from the worktree", main, wt, main, ""},
		{"another repository's worktree", otherWT, main, main, otherWT + " is not a worktree of this repository: it starts in " + main},
	} {
		t.Run(tc.name, func(t *testing.T) {
			work, refused := workFolder(t.Context(), old, "Workspace: "+tc.path+"\n", tc.launch)
			if work != tc.work || refused != tc.refused {
				t.Fatalf("got %q, %q; want %q, %q", work, refused, tc.work, tc.refused)
			}
		})
	}
}

// TestAWorktreeWithControlKeysSaysWhy: a real worktree whose name holds a
// tab or a byte that is not UTF-8 is refused for that, not as "not a
// worktree", which would be false.
func TestAWorktreeWithControlKeysSaysWhy(t *testing.T) {
	folders := map[string]string{"a tab": "wt\tx"}
	if runtime.GOOS != "darwin" { // APFS refuses a name that is not UTF-8
		folders["a bad byte"] = "wt\xffx"
	}
	for name, folder := range folders {
		root := realTemp(t)
		main, wt := filepath.Join(root, "main"), filepath.Join(root, folder)
		repoWithWorktree(t, main, filepath.Join(root, "plain"))
		gitIn(t, "-C", main, "worktree", "add", "-q", "-b", "x", wt)
		work, refused := workFolder(t.Context(), nil, "Workspace: "+wt+"\n", main)
		want := fmt.Sprintf("%q has a control key or a bad byte in its name: it starts in %s", wt, main)
		if work != main || refused != want {
			t.Errorf("%s: workFolder = %q, %q; want %q, %q", name, work, refused, main, want)
		}
	}
}

// realTemp is a temp dir with its links resolved: on macOS t.TempDir is
// under /var, a link to /private/var, and the workspace check answers
// resolved paths.
func realTemp(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
