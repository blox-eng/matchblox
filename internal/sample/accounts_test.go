package sample

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blox-eng/matchblox/internal/limits"
)

// accountFixture is a fixture machine and one agent process in it.
func accountSession(t *testing.T, root string, pid int, agent, pane string, env ...string) {
	t.Helper()
	addProc(t, root, pid, 1, agent, []string{agent}, pane)
	setEnv(t, root, pid, append([]string{"TMUX_PANE=" + pane}, env...)...)
	addPane(t, root, pane+"\tacct:1.1\tw\t"+itoa(int64(pid))+"\t"+agent+"\t/work/app")
}

func sessionOf(t *testing.T, snap Snapshot, pid int) Session {
	t.Helper()
	for _, s := range snap.Sessions {
		if s.PID == pid {
			return s
		}
	}
	t.Fatalf("no session for pid %d", pid)
	return Session{}
}

// TestSessionUnderAnAccountFolder: a config folder other than the default is
// the session's account folder, one made by matchblox or by hand.
func TestSessionUnderAnAccountFolder(t *testing.T) {
	root := copyFixture(t)
	home := filepath.Join(root, "home")
	made := filepath.Join(home, ".matchblox", "accounts", "claude", "work")
	hand := filepath.Join(home, ".claude-work")
	accountSession(t, root, 950, "claude", "%40", "CLAUDE_CONFIG_DIR="+made)
	accountSession(t, root, 951, "claude", "%41", "CLAUDE_CONFIG_DIR="+hand)
	accountSession(t, root, 952, "claude", "%42", "CLAUDE_CONFIG_DIR="+filepath.Join(home, ".claude"))
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude"}
	snap := smp.Sample()
	if got := sessionOf(t, snap, 950).AccountDir; got != made {
		t.Fatalf("a matchblox folder: %q", got)
	}
	if got := sessionOf(t, snap, 951).AccountDir; got != hand {
		t.Fatalf("a folder made by hand: %q", got)
	}
	if got := sessionOf(t, snap, 952).AccountDir; got != "" {
		t.Fatalf("the default folder named out loud: %q", got)
	}
	for _, s := range snap.Sessions {
		if s.PID != 950 && s.PID != 951 && s.AccountDir != "" {
			t.Fatalf("a default session has a folder: %+v", s)
		}
	}
}

// TestTwoUnsignedAccountsStayApart: two Codex accounts with no auth.json are
// two lines, told apart by their folders.
func TestTwoUnsignedAccountsStayApart(t *testing.T) {
	root := copyFixture(t)
	acc := filepath.Join(root, "home", ".matchblox", "accounts", "codex")
	a, b := filepath.Join(acc, "one"), filepath.Join(acc, "two")
	accountSession(t, root, 950, "codex", "%40", "CODEX_HOME="+a)
	accountSession(t, root, 951, "codex", "%41", "CODEX_HOME="+b)
	smp := newFixtureSampler(root)
	smp.Agents = []string{"codex"}
	snap := smp.Sample()
	var dirs []string
	for _, l := range snap.Limits {
		if l.Provider == "openai" && l.Dir != "" {
			dirs = append(dirs, l.Dir)
		}
	}
	if len(dirs) != 2 || dirs[0] != a || dirs[1] != b {
		t.Fatalf("lines by folder: %v (%+v)", dirs, snap.Limits)
	}
}

// TestDefaultKeepsItsKey: the default account has no folder, on its session
// and on its line.
func TestDefaultKeepsItsKey(t *testing.T) {
	root := copyFixture(t)
	writeFile(t, filepath.Join(root, "home", ".claude.json"), `{"oauthAccount":{"emailAddress":"you@example.com","organizationType":"claude_max"}}`)
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude"}
	snap := smp.Sample()
	l, ok := byAccount(snap)["anthropic you@example.com · Max"]
	if !ok || l.Dir != "" {
		t.Fatalf("the default line: %+v", snap.Limits)
	}
	for _, s := range snap.Sessions {
		if s.AccountDir != "" || s.SigningIn {
			t.Fatalf("a default session: %+v", s)
		}
	}
	if l.State != limits.Unmeasured && l.State != limits.Measured {
		t.Fatalf("state %q", l.State)
	}
}

// TestCursorAccountReadsItsOwnHome: a Cursor process whose HOME is under the
// accounts root reads its login there; a foreign HOME still reads nothing.
func TestCursorAccountReadsItsOwnHome(t *testing.T) {
	root := copyFixture(t)
	folder := filepath.Join(root, "home", ".matchblox", "accounts", "cursor-agent", "work")
	writeFile(t, filepath.Join(folder, "cfg", "cli-config.json"), `{"authInfo":{"email":"you@example.com"}}`)
	accountSession(t, root, 950, "cursor-agent", "%40",
		"HOME="+filepath.Join(folder, "home"), "CURSOR_CONFIG_DIR="+filepath.Join(folder, "cfg"))
	foreign := filepath.Join(root, "elsewhere")
	writeFile(t, filepath.Join(foreign, ".cursor", "cli-config.json"), `{"authInfo":{"email":"other@example.com"}}`)
	accountSession(t, root, 951, "cursor-agent", "%41", "HOME="+foreign)
	// A hand-made config folder outside the root does not separate sign-ins.
	accountSession(t, root, 952, "cursor-agent", "%42", "CURSOR_CONFIG_DIR="+filepath.Join(foreign, "cfg"))
	smp := newFixtureSampler(root)
	smp.Agents = []string{"cursor-agent"}
	snap := smp.Sample()
	own := sessionOf(t, snap, 950)
	if own.Account != "you@example.com" || own.AccountDir != folder || own.SigningIn {
		t.Fatalf("its own home: %+v", own)
	}
	if o := sessionOf(t, snap, 951); o.Account != "" || o.AccountDir != "" {
		t.Fatalf("a foreign home: %+v", o)
	}
	if o := sessionOf(t, snap, 952); o.AccountDir != "" {
		t.Fatalf("a hand-made folder counts: %+v", o)
	}
}

// TestSigningInWhileNoName: a session on an account folder with no login is
// signing in.
func TestSigningInWhileNoName(t *testing.T) {
	root := copyFixture(t)
	folder := filepath.Join(root, "home", ".matchblox", "accounts", "claude", "work")
	accountSession(t, root, 950, "claude", "%40", "CLAUDE_CONFIG_DIR="+folder)
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude"}
	s := sessionOf(t, smp.Sample(), 950)
	if !s.SigningIn || s.Account != "" {
		t.Fatalf("no login yet: %+v", s)
	}
	writeFile(t, filepath.Join(folder, ".claude.json"), `{"oauthAccount":{"emailAddress":"you@example.com"}}`)
	if s := sessionOf(t, smp.Sample(), 950); s.SigningIn || s.Account != "you@example.com" {
		t.Fatalf("signed in: %+v", s)
	}
}

// TestAHandMadeFolderIsNotSigningIn: a config folder made by hand, with no
// login matchblox can read (a keyring sign-in), is not signing in: only a
// folder under the accounts root is.
func TestAHandMadeFolderIsNotSigningIn(t *testing.T) {
	root := copyFixture(t)
	hand := filepath.Join(root, "home", ".codex-work")
	accountSession(t, root, 950, "codex", "%40", "CODEX_HOME="+hand)
	smp := newFixtureSampler(root)
	smp.Agents = []string{"codex"}
	s := sessionOf(t, smp.Sample(), 950)
	if s.AccountDir != hand || s.SigningIn {
		t.Fatalf("a hand-made folder: %+v, want its folder and not signing in", s)
	}
}

// TestSymlinkedHomeAccountIsSigningIn: the sampler's Home is a symlink and
// the session env names the real path of its account folder: the folder is
// under the accounts root, and the session is signing in.
func TestSymlinkedHomeAccountIsSigningIn(t *testing.T) {
	root := copyFixture(t)
	real, err := filepath.EvalSymlinks(filepath.Join(root, "home"))
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(real, ".matchblox", "accounts", "claude", "work")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	accountSession(t, root, 950, "claude", "%40", "CLAUDE_CONFIG_DIR="+folder)
	smp := newFixtureSampler(root)
	smp.Home = filepath.Join(root, "link")
	if err := os.Symlink(real, smp.Home); err != nil {
		t.Fatal(err)
	}
	smp.Agents = []string{"claude"}
	s := sessionOf(t, smp.Sample(), 950)
	if s.AccountDir == "" || !s.SigningIn {
		t.Fatalf("symlinked home: %+v, want an account folder and signing in", s)
	}
}
