package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/accounts"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

func startOn(t *testing.T, s *Service, n proto.NewSession) proto.Result {
	t.Helper()
	n.Where = t.TempDir()
	return s.act(t.Context(), newAct(t, n))
}

func lastOf(s *Service, agent string) string {
	return lastIn(s, lifeSnap(false), agent)
}

func lastIn(s *Service, snap sample.Snapshot, agent string) string {
	s.publish(snap, func(state.Doc) {})
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur.LastAccount[agent]
}

func TestLastAccountIsRememberedAfterAStart(t *testing.T) {
	s, _, home := accountTest(t)
	prepared(t, home, "claude", "work")
	if got := lastOf(s, "claude"); got != "" {
		t.Fatalf("before any start: %q", got)
	}
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a1", Account: "work"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := lastOf(s, "claude"); got != "work" {
		t.Fatalf("existing account: %q", got)
	}
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a2", Account: "fresh", NewAccount: true}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := lastIn(s, signedInOn(accounts.Folder(home, "claude", "fresh")), "claude"); got != "fresh" {
		t.Fatalf("new account, signed in: %q", got)
	}
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a3", Account: "default"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := lastOf(s, "claude"); got != "default" {
		t.Fatalf("default: %q", got)
	}
	if res := startOn(t, s, proto.NewSession{Agent: "codex", Name: "a4", Account: "default"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := lastOf(s, "codex"); got != "default" || lastOf(s, "claude") != "default" {
		t.Fatalf("per agent: codex %q", got)
	}
}

func TestLastAccountSurvivesARestart(t *testing.T) {
	s, _, home := accountTest(t)
	prepared(t, home, "claude", "work")
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a1", Account: "work"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	again := newTest(t, time.Hour)
	again.Home, again.GOOS, again.StatePath = home, "linux", s.StatePath
	if got := lastOf(again, "claude"); got != "work" {
		t.Fatalf("after a restart: %q", got)
	}
}

func TestFailedStartIsNotRemembered(t *testing.T) {
	s, _, home := accountTest(t)
	prepared(t, home, "claude", "work")
	s.Actions.Run = func([]string) error { return os.ErrPermission }
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a1", Account: "work"}); res.Err == "" {
		t.Fatal("the start did not fail")
	}
	if got := lastOf(s, "claude"); got != "" {
		t.Fatalf("remembered %q", got)
	}
}

func TestGoneAccountIsNotSent(t *testing.T) {
	s, _, home := accountTest(t)
	folder := prepared(t, home, "claude", "work")
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a1", Account: "work"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if err := os.RemoveAll(folder); err != nil {
		t.Fatal(err)
	}
	if got := lastOf(s, "claude"); got != "" {
		t.Fatalf("sent %q", got)
	}
}

func TestStokerStartIsNotRemembered(t *testing.T) {
	s, _ := handsLife(t)
	// It names its account: only the stoker's gate keeps it from being remembered.
	b := newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "n88", Account: "default"})
	a := fromStokerAct("new:session", b.Text)
	if res := s.act(t.Context(), a); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := lastOf(s, "claude"); got != "" {
		t.Fatalf("remembered %q", got)
	}
}

func TestLastAccountFileIsPrivate(t *testing.T) {
	s, _, _ := accountTest(t)
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a1", Account: "default"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	fi, err := os.Stat(filepath.Join(filepath.Dir(s.StatePath), "last-accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
}

func TestUnnamedStartIsNotRemembered(t *testing.T) {
	s, _, home := accountTest(t)
	prepared(t, home, "claude", "work")
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a1", Account: "work"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a2"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := lastOf(s, "claude"); got != "work" {
		t.Fatalf("got %q", got)
	}
}

func TestExplicitDefaultIsRemembered(t *testing.T) {
	s, _, home := accountTest(t)
	prepared(t, home, "claude", "work")
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a1", Account: "work"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a2", Account: "default"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := lastOf(s, "claude"); got != "default" {
		t.Fatalf("got %q", got)
	}
}

func TestUnreadableLastAccountFileMeansNone(t *testing.T) {
	for _, body := range []string{"null", "{not json", "[]"} {
		s, _, home := accountTest(t)
		prepared(t, home, "claude", "work")
		file := filepath.Join(filepath.Dir(s.StatePath), "last-accounts.json")
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := lastOf(s, "claude"); got != "" {
			t.Fatalf("%s: got %q", body, got)
		}
		if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a1", Account: "work"}); res.Err != "" {
			t.Fatal(res.Err)
		}
		if got := lastOf(s, "claude"); got != "work" {
			t.Fatalf("%s: after a start %q", body, got)
		}
		b, _ := os.ReadFile(file)
		if !strings.Contains(string(b), "work") {
			t.Fatalf("%s: not saved: %s", body, b)
		}
	}
}

func TestHandMadeFolderIsRememberedByItsLabel(t *testing.T) {
	for _, acct := range []string{"~/.claude-old", "HOME/.claude-old"} {
		s, _, home := accountTest(t)
		old := filepath.Join(home, ".claude-old")
		snap := lifeSnap(false)
		snap.Sessions[0].AccountDir = old
		acct = strings.Replace(acct, "HOME", home, 1)
		s.publish(snap, func(state.Doc) {})
		if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a1", Account: acct}); res.Err != "" {
			t.Fatalf("%s: %s", acct, res.Err)
		}
		s.publish(snap, func(state.Doc) {})
		s.mu.Lock()
		last, listed := s.cur.LastAccount["claude"], s.cur.Accounts["claude"]
		s.mu.Unlock()
		if last != "~/.claude-old" || !slices.Contains(listed, last) {
			t.Fatalf("%s: last %q listed %q", acct, last, listed)
		}
	}
}

func TestLastAccountWriteIsRetried(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	s, _, home := accountTest(t)
	prepared(t, home, "claude", "work")
	dir := filepath.Dir(s.StatePath)
	// Context remains writable while the account ledger cannot be saved.
	if err := os.MkdirAll(s.matchContextRoot(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a1", Account: "work"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a2", Account: "work"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "last-accounts.json")); !strings.Contains(string(b), "work") {
		t.Fatalf("not saved: %s", b)
	}
}

func lastAgentOf(s *Service) string {
	s.publish(lifeSnap(false), func(state.Doc) {})
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur.LastAgent
}

func TestLastAgentIsRememberedAndSurvivesARestart(t *testing.T) {
	s, _, home := accountTest(t)
	prepared(t, home, "codex", "team")
	if got := lastAgentOf(s); got != "" {
		t.Fatalf("before any start: %q", got)
	}
	if res := startOn(t, s, proto.NewSession{Agent: "codex", Name: "a1", Account: "team"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := lastAgentOf(s); got != "codex" {
		t.Fatalf("after a start: %q", got)
	}
	// A start that names no account, as the CLI's, changes the agent too.
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a2"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := lastAgentOf(s); got != "claude" {
		t.Fatalf("after an unnamed start: %q", got)
	}
	if res := startOn(t, s, proto.NewSession{Agent: "codex", Name: "a3"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	again := newTest(t, time.Hour)
	again.Home, again.GOOS, again.StatePath, again.LookPath = home, "linux", s.StatePath, s.LookPath
	if got := lastAgentOf(again); got != "codex" {
		t.Fatalf("after a restart: %q", got)
	}
	if got := lastOf(again, "codex"); got != "team" {
		t.Fatalf("after a restart, the account: %q", got)
	}
}

func TestFailedOrStokerStartKeepsTheLastAgent(t *testing.T) {
	s, _, _ := accountTest(t)
	if res := startOn(t, s, proto.NewSession{Agent: "codex", Name: "a1"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	s.Actions.Run = func([]string) error { return os.ErrPermission }
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a2"}); res.Err == "" {
		t.Fatal("the start did not fail")
	}
	if got := lastAgentOf(s); got != "codex" {
		t.Fatalf("after a failed start: %q", got)
	}
	h, _ := handsLife(t)
	h.StatePath = s.StatePath
	b := newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "n88"})
	if res := h.act(t.Context(), fromStokerAct("new:session", b.Text)); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := lastAgentOf(h); got != "codex" {
		t.Fatalf("after the stoker's start: %q", got)
	}
}

func TestLastAgentNotInstalledIsNotSent(t *testing.T) {
	s, _, _ := accountTest(t)
	if res := startOn(t, s, proto.NewSession{Agent: "codex", Name: "a1"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	s.LookPath = func(cmd string) (string, error) {
		if cmd == "codex" {
			return "", exec.ErrNotFound
		}
		return "/usr/bin/" + cmd, nil
	}
	if got := lastAgentOf(s); got != "" {
		t.Fatalf("sent %q", got)
	}
}

func TestOldLastAccountFileStillLoads(t *testing.T) {
	s, _, home := accountTest(t)
	prepared(t, home, "claude", "work")
	file := filepath.Join(filepath.Dir(s.StatePath), "last-accounts.json")
	if err := os.WriteFile(file, []byte(`{"claude":"work"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	again := newTest(t, time.Hour)
	again.Home, again.GOOS, again.StatePath = home, "linux", s.StatePath
	if got := lastOf(again, "claude"); got != "work" {
		t.Fatalf("the old file: %q", got)
	}
	if got := lastAgentOf(again); got != "" {
		t.Fatalf("the old file names no agent: %q", got)
	}
}

// signedInOn is lifeSnap with its first Claude session on folder, signed in.
func signedInOn(folder string) sample.Snapshot {
	snap := lifeSnap(false)
	snap.Sessions[0].AccountDir, snap.Sessions[0].Account = folder, "you@example.com · Max"
	return snap
}

// TestNewAccountIsRememberedOnceSignedIn: a new account is the last one only
// once a session on its folder is signed in. Until then N and a switch keep
// the account before it.
func TestNewAccountIsRememberedOnceSignedIn(t *testing.T) {
	s, _, home := accountTest(t)
	work := prepared(t, home, "claude", "work")
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a1", Account: "work"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a2", Account: "fresh", NewAccount: true}); res.Err != "" {
		t.Fatal(res.Err)
	}
	fresh := accounts.Folder(home, "claude", "fresh")
	if got := lastOf(s, "claude"); got != "work" {
		t.Fatalf("not signed in yet: %q, want work", got)
	}
	// A session on the folder that is not signed in yet changes nothing.
	pending := lifeSnap(false)
	pending.Sessions[0].AccountDir, pending.Sessions[0].SigningIn = fresh, true
	if got := lastIn(s, pending, "claude"); got != "work" {
		t.Fatalf("signing in: %q, want work", got)
	}
	if _, acct := s.switchAccount("claude"); acct == nil || acct.folder != work {
		t.Fatalf("a switch starts on %+v, want work", acct)
	}
	if got := lastIn(s, signedInOn(fresh), "claude"); got != "fresh" {
		t.Fatalf("signed in: %q, want fresh", got)
	}
	if _, acct := s.switchAccount("claude"); acct == nil || acct.folder != fresh {
		t.Fatalf("a switch starts on %+v, want fresh", acct)
	}
}

// TestNewAccountNotSignedInLeavesTheDefault: with no account before it, a
// new account that is not signed in leaves N and a switch on default.
func TestNewAccountNotSignedInLeavesTheDefault(t *testing.T) {
	s, _, _ := accountTest(t)
	if res := startOn(t, s, proto.NewSession{Agent: "claude", Name: "a1", Account: "fresh", NewAccount: true}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := lastOf(s, "claude"); got != "" {
		t.Fatalf("remembered %q", got)
	}
	if _, acct := s.switchAccount("claude"); acct != nil {
		t.Fatalf("a switch starts on %+v, want default", acct)
	}
}

// TestNewAccountOnASymlinkedHomeIsRememberedOnceSignedIn: a session that
// names the folder through the real path of a symlinked home is on it.
func TestNewAccountOnASymlinkedHomeIsRememberedOnceSignedIn(t *testing.T) {
	tmp := t.TempDir()
	real := filepath.Join(tmp, "real")
	if err := os.MkdirAll(filepath.Join(real, "u"), 0o700); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(tmp, "home")
	if err := os.Symlink(real, home); err != nil {
		t.Fatal(err)
	}
	folder := accounts.Folder(filepath.Join(home, "u"), "claude", "fresh")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tmp, "state", lastAccountsFile)
	var r lastAccountState
	r.await("claude", "fresh", folder)
	seen := accounts.Folder(filepath.Join(real, "u"), "claude", "fresh")
	r.signedIn(path, []sample.Session{{Agent: "claude", AccountDir: seen, Account: "you@example.com"}})
	if got := r.of(path, "claude"); got != "fresh" {
		t.Fatalf("signed in on the real path: %q, want fresh", got)
	}
}
