package recovery

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)

// live is a Claude Code session in pane %3 of server 100.
func live(id string) Entry {
	return Entry{Agent: "claude", ConversationID: id, Session: "api", Pane: "%3", Cwd: "/src/api", Socket: "/tmp/tmux-501/default", ServerPID: "100", ServerStart: "1"}
}

func open(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Git = func(dir string) Checkout { return Checkout{Root: dir, Branch: "main", Ident: "origin " + dir} }
	return s
}

func ids(s *Store) string {
	var out []string
	for _, e := range s.List() {
		out = append(out, e.ConversationID)
	}
	return strings.Join(out, ",")
}

// TestTheRecordFollowsTheConversation: a /clear or /resume gives the pane a
// new conversation id. The record keeps the pane's entry and takes the new
// id, so recover reopens the conversation the builder last had there.
func TestTheRecordFollowsTheConversation(t *testing.T) {
	s := open(t, t.TempDir())
	alive := map[string]string{"/tmp/tmux-501/default": "100@1"}
	must(t, s.Update([]Entry{live("a")}, alive, t0))
	must(t, s.Update([]Entry{live("b")}, alive, t0.Add(time.Second)))
	if got := ids(s); got != "b" {
		t.Fatalf("after /clear the record holds %q, want b", got)
	}
}

// TestAQuitAgentLeavesTheRecord: the builder quit the agent while its
// server runs. Nothing was lost, so recover must not bring it back.
func TestAQuitAgentLeavesTheRecord(t *testing.T) {
	s := open(t, t.TempDir())
	alive := map[string]string{"/tmp/tmux-501/default": "100@1"}
	must(t, s.Update([]Entry{live("a")}, alive, t0))
	must(t, s.Update(nil, alive, t0.Add(time.Second)))
	if got := ids(s); got != "" {
		t.Fatalf("a quit agent stays in the record: %q", got)
	}
}

// TestALostServerKeepsItsAgents: the server is gone, or a new server took
// its socket. Its agents stay in the record until ExpireAfter.
func TestALostServerKeepsItsAgents(t *testing.T) {
	for name, alive := range map[string]map[string]string{
		"no server":  {},
		"new server": {"/tmp/tmux-501/default": "200@2"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			s := open(t, dir)
			must(t, s.Update([]Entry{live("a")}, map[string]string{"/tmp/tmux-501/default": "100@1"}, t0))
			must(t, s.Update(nil, alive, t0.Add(time.Hour)))
			if got := ids(open(t, dir)); got != "a" {
				t.Fatalf("after the server loss the file holds %q, want a", got)
			}
			must(t, s.Update(nil, alive, t0.Add(ExpireAfter)))
			if got := ids(s); got != "" {
				t.Fatalf("an entry older than ExpireAfter stays: %q", got)
			}
		})
	}
}

// TestANewServerDoesNotOverwriteTheLostOne: tmux numbers panes from %0 on
// each server. A pane of the new server with a lost pane's id is another
// pane, and both entries stay.
func TestANewServerDoesNotOverwriteTheLostOne(t *testing.T) {
	s := open(t, t.TempDir())
	must(t, s.Update([]Entry{live("a")}, map[string]string{"/tmp/tmux-501/default": "100@1"}, t0))
	b := live("b")
	b.ServerPID, b.ServerStart = "200", "2"
	must(t, s.Update([]Entry{b}, map[string]string{"/tmp/tmux-501/default": "200@2"}, t0.Add(time.Minute)))
	if got := ids(s); got != "a,b" {
		t.Fatalf("the record holds %q, want a,b", got)
	}
}

// TestAReopenedConversationHasOneEntry: recover reopens a conversation in
// a new pane. The entry of the lost pane goes, so a second recover finds
// one live entry and nothing to reopen.
func TestAReopenedConversationHasOneEntry(t *testing.T) {
	s := open(t, t.TempDir())
	must(t, s.Update([]Entry{live("a")}, map[string]string{"/tmp/tmux-501/default": "100@1"}, t0))
	a := live("a")
	a.ServerPID, a.ServerStart, a.Pane = "200", "2", "%0"
	must(t, s.Update([]Entry{a}, map[string]string{"/tmp/tmux-501/default": "200@2"}, t0.Add(time.Minute)))
	if l := s.List(); len(l) != 1 || l[0].ServerPID != "200" {
		t.Fatalf("the record holds %+v, want one entry on server 200", l)
	}
}

// TestTheRecordKeepsTheCheckout: each entry names the repository and the
// branch of its folder, for recover to check before it reopens.
func TestTheRecordKeepsTheCheckout(t *testing.T) {
	s := open(t, t.TempDir())
	must(t, s.Update([]Entry{live("a")}, map[string]string{"/tmp/tmux-501/default": "100@1"}, t0))
	if l := s.List(); len(l) != 1 || l[0].Repo != "/src/api" || l[0].Branch != "main" || !l[0].Seen.Equal(t0) {
		t.Fatalf("the entry is %+v", l)
	}
}

// TestABrokenRecordIsMovedAside: a record that does not parse never stops
// the service. It is kept beside, for the builder to read.
func TestABrokenRecordIsMovedAside(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, File), []byte("{"), 0o600))
	s := open(t, dir)
	if len(s.List()) != 0 {
		t.Fatal("a broken record gives entries")
	}
	if bad, _ := filepath.Glob(filepath.Join(dir, File+".bad-*")); len(bad) != 1 || s.Moved != bad[0] {
		t.Fatalf("the broken record is not kept beside, or Open does not say where: %v %q", bad, s.Moved)
	}
}

// TestARebootedServerWithTheLostPidKeepsTheRecord: after a reboot, a new
// server can get the lost one's pid. Its start time tells them apart, so
// the lost agents stay.
func TestARebootedServerWithTheLostPidKeepsTheRecord(t *testing.T) {
	s := open(t, t.TempDir())
	must(t, s.Update([]Entry{live("a")}, map[string]string{"/tmp/tmux-501/default": Server("100", "1")}, t0))
	must(t, s.Update(nil, map[string]string{"/tmp/tmux-501/default": Server("100", "9")}, t0.Add(time.Hour)))
	if got := ids(s); got != "a" {
		t.Fatalf("a new server with the lost pid emptied the record: %q", got)
	}
}

// TestAHungGitDoesNotHoldTheRecord: a folder on a dead network mount makes
// git hang. Reading its checkout gives up within gitWait, and List does not
// wait for it.
func TestAHungGitDoesNotHoldTheRecord(t *testing.T) {
	bin := t.TempDir()
	must(t, os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o700)) //nolint:gosec // a test program
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	s, err := Open(t.TempDir())
	must(t, err)
	done := make(chan error, 1)
	start := time.Now()
	go func() {
		done <- s.Update([]Entry{live("a")}, map[string]string{"/tmp/tmux-501/default": "100@1"}, t0)
	}()
	time.Sleep(100 * time.Millisecond)
	listed := make(chan struct{})
	go func() { s.List(); close(listed) }()
	select {
	case <-listed:
	case <-time.After(time.Second):
		t.Fatal("List waits for git")
	}
	select {
	case err := <-done:
		must(t, err)
	case <-time.After(gitWait + 3*time.Second):
		t.Fatal("Update still waits for a hung git")
	}
	if l := s.List(); len(l) != 1 || l[0].Repo != "" || time.Since(start) > gitWait+3*time.Second {
		t.Fatalf("after a hung git the entry is %+v", l)
	}
}

// TestASlowGitKeepsTheCheckout: git did not answer in time for a live
// agent's folder. The record keeps what it read before, which recover
// checks.
func TestASlowGitKeepsTheCheckout(t *testing.T) {
	s := open(t, t.TempDir())
	alive := map[string]string{"/tmp/tmux-501/default": "100@1"}
	must(t, s.Update([]Entry{live("a")}, alive, t0))
	s.Git = func(string) Checkout { return Checkout{Unread: true} }
	must(t, s.Update([]Entry{live("a")}, alive, t0.Add(2*refresh)))
	if l := s.List(); len(l) != 1 || l[0].Repo != "/src/api" || l[0].Branch != "main" || l[0].Unread || !l[0].Seen.Equal(t0.Add(2*refresh)) {
		t.Fatalf("after a slow git the entry is %+v", l)
	}
	b := live("b")
	b.Pane = "%4"
	must(t, s.Update([]Entry{live("a"), b}, alive, t0.Add(4*refresh)))
	for _, e := range s.List() {
		if e.ConversationID == "b" && (!e.Unread || e.Repo != "") {
			t.Fatalf("a new entry git never answered for: %+v", e)
		}
	}
}

// TestAHungGitHoldsTheSampleOnce: the reads of many folders on a dead
// mount run at once, so Update waits about gitWait, not gitWait for each.
func TestAHungGitHoldsTheSampleOnce(t *testing.T) {
	bin := t.TempDir()
	must(t, os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o700)) //nolint:gosec // a test program
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	s, err := Open(t.TempDir())
	must(t, err)
	var many []Entry
	for i := range 5 {
		e := live(string(rune('a' + i)))
		e.Pane = "%" + string(rune('0'+i))
		many = append(many, e)
	}
	start := time.Now()
	must(t, s.Update(many, nil, t0))
	if d := time.Since(start); d > gitWait+2*time.Second {
		t.Fatalf("five hung reads held Update %v", d)
	}
}

// TestAMovedCloneKeepsItsIdent: a clone moved by hand is the same
// repository; another repository is not.
func TestAMovedCloneKeepsItsIdent(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	base := t.TempDir()
	repo := func(name string) string {
		dir := filepath.Join(base, name)
		must(t, os.MkdirAll(dir, 0o700))
		for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", name}} {
			if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil { //nolint:gosec // test argv
				t.Fatalf("git %v: %v %s", args, err, out)
			}
		}
		return dir
	}
	api, local, other := repo("api"), repo("local"), repo("other")
	if out, err := exec.Command("git", "-C", api, "remote", "add", "origin", "https://example.invalid/o/api.git").CombinedOutput(); err != nil { //nolint:gosec // test argv
		t.Fatalf("git remote add: %v %s", err, out)
	}
	for _, dir := range []string{api, local} { // with an origin, and with none
		was := ReadCheckout(dir)
		moved := dir + "-moved"
		must(t, os.Rename(dir, moved))
		now := ReadCheckout(moved)
		if was.Ident == "" || now.Ident != was.Ident || now.Branch != "main" || now.Unread {
			t.Errorf("the moved clone reads %+v, it read %+v", now, was)
		}
		if o := ReadCheckout(other); o.Ident == was.Ident {
			t.Errorf("another repository has the same ident %q", o.Ident)
		}
	}
}

// TestTheRecordKeepsNoOriginCredential: an origin URL can carry a token.
// The record keeps a hash of the place it names, so no token is written,
// and a clone over ssh names the same repository as one over https.
func TestTheRecordKeepsNoOriginCredential(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	const token = "ghp_SECRETTOKEN123"
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"remote", "add", "origin", "https://x-access-token:" + token + "@GitHub.com/o/api.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil { //nolint:gosec // test argv
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	rec := t.TempDir()
	s, err := Open(rec)
	must(t, err)
	e := live("a")
	e.Cwd = dir
	must(t, s.Update([]Entry{e}, nil, t0))
	b, err := os.ReadFile(filepath.Join(rec, File))
	must(t, err)
	if strings.Contains(string(b), token) || strings.Contains(string(b), "x-access-token") {
		t.Fatalf("the record holds the origin's credential:\n%s", b)
	}
	got := s.List()[0].Ident
	for _, url := range []string{"git@github.com:o/api.git", "ssh://git@github.com:22/o/api", "https://github.com/o/api/"} {
		if id := hashed("origin " + originPlace(url)); id != got {
			t.Errorf("%s names another repository: %s, want %s", url, id, got)
		}
	}
	if id := hashed("origin " + originPlace("git@github.com:o/web.git")); id == got {
		t.Error("o/web names the same repository as o/api")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
