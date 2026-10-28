package recovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)

// live is a Claude Code session in pane %3 of server 100.
func live(id string) Entry {
	return Entry{Agent: "claude", ConversationID: id, Session: "api", Pane: "%3", Cwd: "/src/api", Socket: "/tmp/tmux-501/default", ServerPID: "100"}
}

func open(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Git = func(dir string) (string, string) { return dir, "main" }
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
	alive := map[string]string{"/tmp/tmux-501/default": "100"}
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
	alive := map[string]string{"/tmp/tmux-501/default": "100"}
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
		"new server": {"/tmp/tmux-501/default": "200"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			s := open(t, dir)
			must(t, s.Update([]Entry{live("a")}, map[string]string{"/tmp/tmux-501/default": "100"}, t0))
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
	must(t, s.Update([]Entry{live("a")}, map[string]string{"/tmp/tmux-501/default": "100"}, t0))
	b := live("b")
	b.ServerPID = "200"
	must(t, s.Update([]Entry{b}, map[string]string{"/tmp/tmux-501/default": "200"}, t0.Add(time.Minute)))
	if got := ids(s); got != "a,b" {
		t.Fatalf("the record holds %q, want a,b", got)
	}
}

// TestAReopenedConversationHasOneEntry: recover reopens a conversation in
// a new pane. The entry of the lost pane goes, so a second recover finds
// one live entry and nothing to reopen.
func TestAReopenedConversationHasOneEntry(t *testing.T) {
	s := open(t, t.TempDir())
	must(t, s.Update([]Entry{live("a")}, map[string]string{"/tmp/tmux-501/default": "100"}, t0))
	a := live("a")
	a.ServerPID, a.Pane = "200", "%0"
	must(t, s.Update([]Entry{a}, map[string]string{"/tmp/tmux-501/default": "200"}, t0.Add(time.Minute)))
	if l := s.List(); len(l) != 1 || l[0].ServerPID != "200" {
		t.Fatalf("the record holds %+v, want one entry on server 200", l)
	}
}

// TestTheRecordKeepsTheCheckout: each entry names the repository and the
// branch of its folder, for recover to check before it reopens.
func TestTheRecordKeepsTheCheckout(t *testing.T) {
	s := open(t, t.TempDir())
	must(t, s.Update([]Entry{live("a")}, map[string]string{"/tmp/tmux-501/default": "100"}, t0))
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
	if bad, _ := filepath.Glob(filepath.Join(dir, File+".bad-*")); len(bad) != 1 {
		t.Fatalf("the broken record is not kept beside: %v", bad)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
