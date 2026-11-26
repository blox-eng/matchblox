package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceLine(t *testing.T) {
	root := t.TempDir()
	main, wt := filepath.Join(root, "main"), filepath.Join(root, "wt")
	other, otherWT := filepath.Join(root, "other"), filepath.Join(root, "otherwt")
	for _, r := range [][2]string{{main, wt}, {other, otherWT}} {
		gitIn(t, "init", "-q", r[0])
		gitIn(t, "-C", r[0], "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "x")
		gitIn(t, "-C", r[0], "worktree", "add", "-q", r[1])
	}
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
