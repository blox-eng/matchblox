package clone

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/blox-eng/matchblox/internal/gitremote"
)

func TestProgress(t *testing.T) {
	for line, want := range map[string]int{"Receiving objects:  42% (420/1000), 1.2 MiB | 2 MiB/s": 42, "Receiving objects: 100% (1000/1000), done.": 100} {
		if got, ok := Progress(line); !ok || got != want {
			t.Errorf("Progress(%q) = %d %v", line, got, ok)
		}
	}
	if _, ok := Progress("Resolving deltas:  50% (1/2)"); ok {
		t.Error("Resolving deltas gave a percent")
	}
}

func TestExplain(t *testing.T) {
	r := gitremote.Remote{Host: "github.com", Owner: "o", Repo: "r"}
	cases := map[string]string{
		"git@github.com: Permission denied (publickey).\nfatal: Could not read from remote repository.": "no access to o/r: add an SSH key for github.com, or use its https URL",
		"fatal: could not read Username for 'https://github.com'":                                       "no access to o/r: run gh auth login",
		"remote: Repository not found.":                                                                 "o/r is not on github.com, or you have no access: run gh auth login",
		"fatal: unable to access 'https://github.com/o/r/': Could not resolve host: github.com":         "no network: github.com does not answer",
		"error: No space left on device":                                                                "the disk is full: o/r is not cloned",
		"Cloning into 'x'...\nfatal: odd thing\n\n":                                                     "git clone o/r failed: fatal: odd thing",
	}
	for in, want := range cases {
		if got := Explain(in, r); got != want {
			t.Errorf("Explain(%q) = %q, want %q", in, got, want)
		}
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// localRepo makes a repo with one commit and returns its path, cloned as a URL by Run.
func localRepo(t *testing.T) gitremote.Remote {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "src")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "one")
	return gitremote.Remote{URL: dir, Host: "github.com", Owner: "o", Repo: "r"}
}

func TestRunClonesAndReportsProgress(t *testing.T) {
	r := localRepo(t)
	target := filepath.Join(t.TempDir(), "r")
	if err := Run(context.Background(), r, target, func(int) {}); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(target, ".git")) || !exists(filepath.Join(target, "a.txt")) {
		t.Error("target is not a clone")
	}
	if exists(Cloning(target)) {
		t.Error("sibling left")
	}
}

func TestRunNeverWritesIntoAnExistingFolder(t *testing.T) {
	r := localRepo(t)
	target := filepath.Join(t.TempDir(), "r")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "keep"), []byte("k"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), r, target, nil); err == nil {
		t.Fatal("want error")
	}
	ents, _ := os.ReadDir(target)
	if len(ents) != 1 || ents[0].Name() != "keep" {
		t.Errorf("folder changed: %v", ents)
	}
	if b, _ := os.ReadFile(filepath.Join(target, "keep")); string(b) != "k" {
		t.Error("keep changed")
	}
	if exists(Cloning(target)) {
		t.Error("sibling made")
	}
}

func TestRunCancelLeavesNothing(t *testing.T) {
	r := localRepo(t)
	target := filepath.Join(t.TempDir(), "r")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Run(ctx, r, target, nil)
	if err == nil || err.Error() != "stopped" {
		t.Fatalf("err = %v", err)
	}
	if exists(target) || exists(Cloning(target)) {
		t.Error("left something")
	}
}

func TestRunFailureLeavesNothing(t *testing.T) {
	r := gitremote.Remote{URL: filepath.Join(t.TempDir(), "missing"), Host: "github.com", Owner: "o", Repo: "r"}
	target := filepath.Join(t.TempDir(), "r")
	err := Run(context.Background(), r, target, nil)
	if err == nil || err.Error() == "" {
		t.Fatal("want error")
	}
	if exists(target) || exists(Cloning(target)) {
		t.Error("left something")
	}
}
