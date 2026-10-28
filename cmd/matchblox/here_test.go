package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestLaunchRepo: the console launched in a repo, or a folder under it,
// starts Sessions' ghost row there; outside one, at home, or over SSH
// there is no here.
func TestLaunchRepo(t *testing.T) {
	home, _ := filepath.EvalSymlinks(t.TempDir())
	repo := filepath.Join(home, "src", "api")
	sub := filepath.Join(repo, "cmd")
	plain := filepath.Join(home, "notes")
	for _, d := range []string{sub, plain} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{repo, home} {
		if out, err := exec.Command("git", "init", "-q", d).CombinedOutput(); err != nil {
			t.Fatalf("git init: %v %s", err, out)
		}
	}
	for _, c := range []struct {
		dir  string
		ssh  bool
		want string
	}{
		{repo, false, repo},
		{sub, false, repo},
		{plain, false, ""}, // under home's repo: home is no place to start
		{home, false, ""},
		{repo, true, ""},
		{filepath.Join(home, "gone"), false, ""},
	} {
		if got := launchRepo(c.dir, home, c.ssh); got != c.want {
			t.Errorf("launchRepo(%s, ssh %v) = %q, want %q", c.dir, c.ssh, got, c.want)
		}
	}
}
