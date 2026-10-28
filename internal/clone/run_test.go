package clone

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
		"fatal: repository 'https://github.com/o/r/' not found":                                         "o/r is not on github.com, or you have no access: run gh auth login",
		"Cloning into 'x'...\nssh: command not found":                                                   "git clone o/r failed: ssh: command not found",
		"Cloning into 'x'...\nfatal: odd thing\n\n":                                                     "git clone o/r failed: fatal: odd thing",
	}
	for in, want := range cases {
		if got := Explain(in, r); got != want {
			t.Errorf("Explain(%q) = %q, want %q", in, got, want)
		}
	}
}

// The sign-in advice names gh only for github.com.
func TestExplainSignInByHost(t *testing.T) {
	for _, c := range []struct{ host, stderr, want string }{
		{"github.com", "fatal: could not read Username for 'https://github.com'", "no access to o/r: run gh auth login"},
		{"github.com", "remote: Repository not found.", "o/r is not on github.com, or you have no access: run gh auth login"},
		{"git.example.org", "fatal: Authentication failed for 'https://git.example.org/o/r'", "no access to o/r: sign in to git.example.org for git"},
		{"git.example.org", "remote: Repository not found.", "o/r is not on git.example.org, or you have no access: sign in to git.example.org for git"},
	} {
		r := gitremote.Remote{Host: c.host, Owner: "o", Repo: "r"}
		if got := Explain(c.stderr, r); got != c.want {
			t.Errorf("Explain(%q, %s) = %q, want %q", c.stderr, c.host, got, c.want)
		}
	}
}

// The last-line fallback never echoes a URL's user or query.
func TestExplainFallbackHidesCredentials(t *testing.T) {
	r := gitremote.Remote{Host: "h", Owner: "o", Repo: "r"}
	for in, want := range map[string]string{
		"fatal: odd thing with 'https://ghp_x@h/o/r'":         "git clone o/r failed: fatal: odd thing with 'https://h/o/r'",
		"fatal: bad 'https://u:tok@h/o/r?access_token=abc' x": "git clone o/r failed: fatal: bad 'https://h/o/r' x",
		"fatal: ssh://git@h/o/r?token=abc":                    "git clone o/r failed: fatal: ssh://h/o/r",
		"fatal: plain https://h/o/r":                          "git clone o/r failed: fatal: plain https://h/o/r",
	} {
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

// fakeGit puts a git on PATH that runs script (sh) in place of the real one.
func fakeGit(t *testing.T, script string) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\n"+script), 0o755); err != nil { //nolint:gosec // a test script
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// A stop mid-clone returns at once, even with a child of git that holds its
// stderr, and leaves neither the target nor the sibling.
func TestRunStopMidCloneDoesNotWaitForChildren(t *testing.T) {
	// The last argument is the sibling: make it, say a percent, and leave a
	// child that keeps stderr open long after git is gone.
	fakeGit(t, `for a; do dir=$a; done
mkdir "$dir"
echo "Receiving objects:   1% (1/100)" >&2
sleep 8 &
sleep 8
`)
	r := gitremote.Remote{URL: "https://github.com/o/r", Host: "github.com", Owner: "o", Repo: "r"}
	target := filepath.Join(t.TempDir(), "r")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	var once sync.Once
	done := make(chan error, 1)
	go func() { done <- Run(ctx, r, target, func(int) { once.Do(func() { close(started) }) }) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the clone did not start")
	}
	at := time.Now()
	cancel()
	select {
	case err := <-done:
		if err == nil || err.Error() != "stopped" {
			t.Fatalf("err = %v", err)
		}
		if d := time.Since(at); d > 2*time.Second {
			t.Fatalf("stopped after %v", d)
		}
	case <-time.After(12 * time.Second):
		t.Fatal("Run did not return")
	}
	if exists(target) || exists(Cloning(target)) {
		t.Error("left something")
	}
}

// git runs with no terminal to ask on and askpass off, so ssh fails
// instead of prompting; the builder's own GIT_SSH_COMMAND reaches git as
// it is, and git speaks English, which Explain reads.
func TestRunSSHNeverPrompts(t *testing.T) {
	out := filepath.Join(t.TempDir(), "env")
	fakeGit(t, `printf '%s|%s|%s|%s' "$GIT_SSH_COMMAND" "$GIT_TERMINAL_PROMPT" "$SSH_ASKPASS_REQUIRE" "$LC_ALL" > "`+out+`"
exit 1
`)
	r := gitremote.Remote{URL: "git@github.com:o/r.git", Host: "github.com", Owner: "o", Repo: "r"}
	for set, want := range map[string]string{"": "|0|never|C", "ssh -i ~/.ssh/k": "ssh -i ~/.ssh/k|0|never|C"} {
		t.Setenv("GIT_SSH_COMMAND", set)
		if set == "" {
			_ = os.Unsetenv("GIT_SSH_COMMAND")
		}
		_ = Run(context.Background(), r, filepath.Join(t.TempDir(), "r"), nil)
		if b, _ := os.ReadFile(out); string(b) != want {
			t.Errorf("with %q: git saw %q, want %q", set, b, want)
		}
	}
}

// No askpass asks either: git's and ssh's are empty in git's env, and a
// core.askPass from the builder's config is off for this clone.
func TestRunTurnsAskpassOff(t *testing.T) {
	out := filepath.Join(t.TempDir(), "env")
	fakeGit(t, `printf '%s|%s|%s|%s|%s' "${GIT_ASKPASS-unset}" "${SSH_ASKPASS-unset}" "$1" "$2" "$3" > "`+out+`"
exit 1
`)
	t.Setenv("GIT_ASKPASS", "/usr/local/bin/ask")
	t.Setenv("SSH_ASKPASS", "/usr/local/bin/ask")
	r := gitremote.Remote{URL: "git@github.com:o/r.git", Host: "github.com", Owner: "o", Repo: "r"}
	_ = Run(context.Background(), r, filepath.Join(t.TempDir(), "r"), nil)
	if b, _ := os.ReadFile(out); string(b) != "||-c|core.askPass=|clone" {
		t.Errorf("git saw %q", b)
	}
}

// A git that is done but leaves a child holding its stderr has cloned:
// the folder is kept.
func TestRunKeepsADoneCloneWhoseChildHoldsStderr(t *testing.T) {
	fakeGit(t, `for a; do dir=$a; done
mkdir "$dir"
sleep 5 &
exit 0
`)
	r := gitremote.Remote{URL: "https://github.com/o/r", Host: "github.com", Owner: "o", Repo: "r"}
	target := filepath.Join(t.TempDir(), "r")
	if err := Run(context.Background(), r, target, nil); err != nil {
		t.Fatal(err)
	}
	if !exists(target) || exists(Cloning(target)) {
		t.Errorf("target %v, sibling %v", exists(target), exists(Cloning(target)))
	}
}

// A rename that fails for another reason than a folder in the way says
// so: the target is not there.
func TestRunSaysWhyTheMoveFailed(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores chmod")
	}
	parent := filepath.Join(t.TempDir(), "p")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) }) //nolint:gosec // a test folder
	fakeGit(t, `for a; do dir=$a; done
mkdir "$dir"
chmod 555 "`+parent+`"
exit 0
`)
	r := gitremote.Remote{URL: "https://github.com/o/r", Host: "github.com", Owner: "o", Repo: "r"}
	target := filepath.Join(parent, "r")
	err := Run(context.Background(), r, target, nil)
	if err == nil || !strings.HasPrefix(err.Error(), "cannot move the clone into "+target+": ") {
		t.Fatalf("err = %v", err)
	}
}

// The owner file names the process that clones, next to the sibling, from
// before git starts until the clone ends: renamed, failed or stopped.
func TestRunKeepsAnOwnerFileWhileItClonesAndNoneAfter(t *testing.T) {
	r := gitremote.Remote{URL: "https://github.com/o/r", Host: "github.com", Owner: "o", Repo: "r"}
	for name, c := range map[string]struct {
		script string
		stop   bool
		ok     bool
	}{
		"success": {script: `for a; do dir=$a; done
mkdir "$dir"
exit 0
`, ok: true},
		"failure": {script: "exit 1\n"},
		"stop":    {script: "sleep 30\n", stop: true},
	} {
		t.Run(name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "r")
			seen := filepath.Join(filepath.Dir(target), "seen")
			// what the owner file holds, as git starts
			fakeGit(t, `cat "`+OwnerFile(target)+`" > "`+seen+`"
`+c.script)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if c.stop {
				// Stop once git has read the owner file: a fixed delay
				// stopped a git the race detector had not started yet.
				go func() {
					for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
						if b, _ := os.ReadFile(seen); len(b) > 0 { //nolint:gosec // the test's own file
							break
						}
					}
					cancel()
				}()
			}
			err := Run(ctx, r, target, nil)
			if (err == nil) != c.ok {
				t.Fatalf("err = %v", err)
			}
			if b, _ := os.ReadFile(seen); string(b) != strconv.Itoa(os.Getpid()) {
				t.Errorf("git saw the owner %q, want this pid", b)
			}
			if exists(OwnerFile(target)) {
				t.Error("owner file left")
			}
		})
	}
}

// An owner file with no sibling, left by a process that is gone, does not
// block a clone; one of a live process does.
func TestRunTakesOverADeadOwnersFile(t *testing.T) {
	r := localRepo(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "r")
	if err := os.WriteFile(OwnerFile(target), []byte("999999999"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), r, target, nil); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "q")
	if err := os.WriteFile(OwnerFile(other), []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), r, other, nil); err == nil || exists(other) || !exists(OwnerFile(other)) {
		t.Fatalf("err %v, target %v, owner file %v", err, exists(other), exists(OwnerFile(other)))
	}
}

// Owner reads the pid, and Alive tells a live process from a gone one.
func TestOwnerAndAlive(t *testing.T) {
	target := filepath.Join(t.TempDir(), "r")
	if _, ok := Owner(target); ok {
		t.Fatal("an owner with no file")
	}
	for text, ok := range map[string]bool{"123": true, "123\n": true, "": false, "x": false, "-4": false, "0": false} {
		if err := os.WriteFile(OwnerFile(target), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		if pid, got := Owner(target); got != ok || (ok && pid != 123) {
			t.Errorf("Owner(%q) = %d %v", text, pid, got)
		}
	}
	if !Alive(os.Getpid()) {
		t.Error("this process is not alive")
	}
	if Alive(999999999) {
		t.Error("a pid that cannot exist is alive")
	}
}

// An existing sibling is refused, git never runs and what is in it stays;
// so is a parent that is missing or is a file.
func TestRunRefusesASiblingAndABadParent(t *testing.T) {
	r := gitremote.Remote{URL: "https://github.com/o/r", Host: "github.com", Owner: "o", Repo: "r"}
	log := filepath.Join(t.TempDir(), "git.log")
	fakeGit(t, `echo ran >> "`+log+`"`+"\nexit 0\n")
	dir := t.TempDir()
	target := filepath.Join(dir, "r")
	marker := filepath.Join(Cloning(target), "marker")
	if err := os.Mkdir(Cloning(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), r, target, nil); err == nil || !strings.Contains(err.Error(), "is there already") {
		t.Fatalf("sibling: err = %v", err)
	}
	if b, _ := os.ReadFile(marker); string(b) != "m" {
		t.Error("the marker did not survive")
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, parent := range []string{filepath.Join(dir, "missing"), file} {
		if err := Run(context.Background(), r, filepath.Join(parent, "r"), nil); err == nil || !strings.HasSuffix(err.Error(), "is not a folder") {
			t.Errorf("parent %s: err = %v", parent, err)
		}
	}
	if b, _ := os.ReadFile(log); len(b) != 0 {
		t.Errorf("git ran: %s", b)
	}
}
