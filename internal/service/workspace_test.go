package service

import (
	"context"
	"os"
	"path/filepath"
	"slices"
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
	root := t.TempDir()
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

	fakes := t.TempDir()
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

	plain := filepath.Join(t.TempDir(), "plain")
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
	root := t.TempDir()
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
