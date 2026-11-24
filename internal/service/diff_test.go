package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// hostileDiffBytes sit in a file's content. The diff must keep them as data;
// drawing them is Task 14's job.
const hostileDiffBytes = "\x1b]0;pwned\x07\x1b]52;c;cHduZWQ=\x07"

func diffAct(pane string) proto.Act {
	return proto.Act{RecID: proto.DiffPrefix + pane, Console: true}
}

func diffActFile(pane, file string) proto.Act {
	return proto.Act{RecID: proto.DiffPrefix + pane + ":" + file, Console: true}
}

// gitRepo builds a temp git repo with main, returns its path. The caller
// adds commits and unstaged changes on a branch.
func gitRepo(t *testing.T) string {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(tmp, "repo")
	runGit(t, "", "init", "-q", "-b", "main", repo)
	runGit(t, repo, "config", "user.name", "t")
	runGit(t, repo, "config", "user.email", "t@t")
	runGit(t, repo, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README")
	runGit(t, repo, "commit", "-q", "-m", "base")
	return repo
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func diffSession(t *testing.T, cwd, name, pane string) *Service {
	t.Helper()
	s := newTest(t, time.Hour)
	s.Host = "ws-1"
	sess := sample.Session{Agent: "claude", Pane: pane, Name: name, SessionID: "s-diff", Status: "idle", Cwd: cwd}
	s.publish(sample.Snapshot{At: seenAt0, Sessions: []sample.Session{sess}}, func(state.Doc) {})
	s.mu.Lock()
	s.cur.Git = &gitscan.Report{Repos: []gitscan.Repo{{
		Path: cwd, Main: "main",
		Worktrees: []gitscan.Worktree{{Path: cwd, Branch: "feat"}},
	}}}
	s.mu.Unlock()
	return s
}

// TestDiffKeepsBytesAsData: a file whose content holds OSC title and OSC 52
// clipboard sequences comes back as the same bytes, as data.
func TestDiffKeepsBytesAsData(t *testing.T) {
	repo := gitRepo(t)
	runGit(t, repo, "checkout", "-q", "-b", "feat")
	path := filepath.Join(repo, "hostile.txt")
	if err := os.WriteFile(path, []byte("safe\n"+hostileDiffBytes+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "hostile.txt")
	runGit(t, repo, "commit", "-q", "-m", "hostile")

	s := diffSession(t, repo, "ux-pass", "%3")
	res := s.act(t.Context(), diffAct("%3"))
	if res.Err != "" || res.Diff == nil {
		t.Fatalf("diff:%%3 answered %+v", res)
	}
	var found string
	for _, f := range res.Diff.Files {
		for _, h := range f.Hunks {
			for _, line := range h.Lines {
				if strings.Contains(line.Text, "\x1b]0;pwned\x07") || strings.Contains(line.Text, "\x1b]52;c;") {
					found = line.Text
				}
			}
		}
	}
	if !strings.Contains(found, "\x1b]0;pwned\x07") || !strings.Contains(found, "\x1b]52;c;cHduZWQ=\x07") {
		t.Fatalf("diff lost the hostile bytes: got %q, want both OSC sequences as data", found)
	}
}

// TestDiffIsTheWorktreeAgainstItsBase: one commit and one unstaged change
// on a branch off main give both files, with their counts, against
// merge-base main HEAD.
func TestDiffIsTheWorktreeAgainstItsBase(t *testing.T) {
	repo := gitRepo(t)
	runGit(t, repo, "checkout", "-q", "-b", "feat")
	if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("package a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "a.go")
	runGit(t, repo, "commit", "-q", "-m", "a")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("base\nchanged\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	s := diffSession(t, repo, "ux-pass", "%3")
	res := s.act(t.Context(), diffAct("%3"))
	if res.Err != "" || res.Diff == nil {
		t.Fatalf("diff:%%3 answered %+v", res)
	}
	if res.Diff.Against != "main" {
		t.Fatalf("against %q, want main", res.Diff.Against)
	}
	by := map[string]proto.DiffFile{}
	for _, f := range res.Diff.Files {
		by[f.Path] = f
	}
	if len(by) != 2 || by["a.go"].Added < 1 || by["README"].Added < 1 {
		t.Fatalf("files %+v, want a.go and README with adds", res.Diff.Files)
	}
	if len(by["a.go"].Hunks) == 0 || len(by["README"].Hunks) == 0 {
		t.Fatalf("hunks missing: %+v", res.Diff.Files)
	}
}

// TestDiffFallsBackToTheRemote: main deleted, origin/main present: the
// result carries against origin/main and the note that main is gone.
func TestDiffFallsBackToTheRemote(t *testing.T) {
	repo := gitRepo(t)
	bare := filepath.Join(filepath.Dir(repo), "bare.git")
	runGit(t, "", "clone", "-q", "--bare", repo, bare)
	runGit(t, repo, "remote", "add", "origin", bare)
	runGit(t, repo, "fetch", "-q", "origin")
	runGit(t, repo, "checkout", "-q", "-b", "feat")
	if err := os.WriteFile(filepath.Join(repo, "c.go"), []byte("package c\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "c.go")
	runGit(t, repo, "commit", "-q", "-m", "c")
	runGit(t, repo, "branch", "-q", "-D", "main")

	s := diffSession(t, repo, "ux-pass", "%3")
	res := s.act(t.Context(), diffAct("%3"))
	if res.Err != "" || res.Diff == nil {
		t.Fatalf("diff:%%3 answered %+v", res)
	}
	if res.Diff.Against != "origin/main" {
		t.Fatalf("against %q, want origin/main", res.Diff.Against)
	}
	if res.Diff.Note != "main is gone on ws-1" {
		t.Fatalf("note %q, want %q", res.Diff.Note, "main is gone on ws-1")
	}
	if len(res.Diff.Files) == 0 {
		t.Fatalf("no files: %+v", res.Diff)
	}
}

func TestDiffWithNoChanges(t *testing.T) {
	repo := gitRepo(t)
	s := diffSession(t, repo, "ux-pass", "%3")
	res := s.act(t.Context(), diffAct("%3"))
	if res.Err != "ux-pass has no changes against main." || res.Diff != nil {
		t.Fatalf("diff:%%3 answered %+v, want no-changes err", res)
	}
}

func TestDiffWithNoGit(t *testing.T) {
	dir := t.TempDir()
	s := diffSession(t, dir, "ux-pass", "%3")
	s.mu.Lock()
	s.cur.Git = &gitscan.Report{}
	s.mu.Unlock()
	res := s.act(t.Context(), diffAct("%3"))
	if res.Err != "No changes to review: ux-pass has no git folder." || res.Diff != nil {
		t.Fatalf("diff:%%3 answered %+v, want no-git err", res)
	}
}

// TestABigDiffSendsFilesOnly: over 2 MB, the result has the file list and
// no hunks, and each file can be asked for alone.
func TestABigDiffSendsFilesOnly(t *testing.T) {
	repo := gitRepo(t)
	runGit(t, repo, "checkout", "-q", "-b", "feat")
	big := strings.Repeat("x", 2<<20+1024)
	if err := os.WriteFile(filepath.Join(repo, "big.txt"), []byte(big+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "small.txt"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-q", "-m", "big")

	s := diffSession(t, repo, "ux-pass", "%3")
	res := s.act(t.Context(), diffAct("%3"))
	if res.Err != "" || res.Diff == nil {
		t.Fatalf("diff:%%3 answered %+v", res)
	}
	if len(res.Diff.Files) < 2 {
		t.Fatalf("files %+v", res.Diff.Files)
	}
	for _, f := range res.Diff.Files {
		if len(f.Hunks) != 0 {
			t.Fatalf("big diff still has hunks on %s: %+v", f.Path, f)
		}
	}
	one := s.act(t.Context(), diffActFile("%3", "small.txt"))
	if one.Err != "" || one.Diff == nil || len(one.Diff.Files) != 1 || one.Diff.Files[0].Path != "small.txt" {
		t.Fatalf("diff:%%3:small.txt answered %+v", one)
	}
	if len(one.Diff.Files[0].Hunks) == 0 {
		t.Fatalf("single-file diff has no hunks: %+v", one.Diff.Files[0])
	}
}
