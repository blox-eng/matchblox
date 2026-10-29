package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/login"
	"github.com/blox-eng/matchblox/internal/remote"
)

const testKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIM4bfwfUcmyCX2tnMZ4mHkeEjSRJnkf0ehLEuB8iWLHU me@laptop"

// passwordHost is the console of ws-1 whose login check found a password
// login, with prompts in the console and the ladder's step. The master
// is up once an install or an open ran. Later checks are ok.
type passwordHost struct {
	m      Model
	calls  *[]sshCall
	ran    *[][]string
	checks *[][]string
}

func ladderHost(t *testing.T, step login.Step, firstCheck string) passwordHost {
	t.Helper()
	m, _, _ := checkedHost(t, fixtureState(), "", 0)
	h := passwordHost{calls: &[]sshCall{}, ran: &[][]string{}, checks: &[][]string{}}
	m.opt.Check = func(_ context.Context, argv []string, _ func(string)) (string, int) {
		*h.checks = append(*h.checks, argv)
		if len(*h.checks) == 1 {
			return firstCheck, 255
		}
		return "", 0
	}
	m.opt.Ask = fakeAsk{}
	m.askCtx, m.askStop = context.WithCancel(context.Background())
	t.Cleanup(m.askStop)
	m.opt.Keys = &Keys{Step: func() login.Step { return step }, GOOS: "linux"}
	up := false
	m.opt.SSH = func(_ context.Context, argv, env []string, _ func(string)) (string, int) {
		*h.calls = append(*h.calls, sshCall{argv, env})
		if slices.Contains(argv, "-O") {
			if up {
				return "", 0
			}
			return "Control socket connect: No such file or directory\n", 255
		}
		up = true
		return "", 0
	}
	m.opt.Run = func(argv []string) error { *h.ran = append(*h.ran, argv); return nil }
	next, cmd := m.Update(hostHello("3.4"))
	next, _ = next.Update(loginCheckOf(cmd)[0])
	h.m = next.(Model)
	if h.m.login != remote.LoginPassword && firstCheck == passwordRefusal {
		t.Fatalf("login %v", h.m.login)
	}
	return h
}

const passwordRefusal = "builder@ws-1: Permission denied (publickey,password).\r\n"

// settle runs cmd and the messages it starts until nothing is left.
func settle(t *testing.T, m tea.Model, cmd tea.Cmd) Model {
	t.Helper()
	for cmd != nil {
		m, cmd = m.Update(cmd())
	}
	return m.(Model)
}

// jumpOffer confirms a jump to %1 and runs it up to the ladder's offer.
func jumpOffer(t *testing.T, h passwordHost) Model {
	t.Helper()
	hostJump(&h.m, remote.Jump{Pane: "%1"})
	next, cmd := h.m.confirm(*h.m.forHost(h.m.opt.Host, h.m.jump("%1")))
	m := settle(t, next, cmd)
	if m.pending == nil || m.pending.climb == nil {
		t.Fatalf("no ladder offer: pending %+v, flash %q", m.pending, m.flash)
	}
	return m
}

func offerWords(m Model) string {
	head, body := m.pending.consentWords()
	return head + "\n" + strings.Join(body, "\n")
}

func TestPasswordLoginOffersLadder(t *testing.T) {
	step := login.Step{Source: login.FromAgent, Line: testKey, Fingerprint: login.Fingerprint(testKey)}
	t.Run("y installs, then the jump runs", func(t *testing.T) {
		h := ladderHost(t, step, passwordRefusal)
		m := jumpOffer(t, h)
		dir := filepath.Join(m.opt.MasterBase, "ssh")
		install, err := login.InstallArgv("ws-1", testKey, remote.MasterOpts(dir))
		if err != nil {
			t.Fatal(err)
		}
		words := offerWords(m)
		for _, want := range []string{"Put your key on ws-1", step.Fingerprint, "in your ssh-agent", remote.Quote(install[:len(install)-1]) + " " + install[len(install)-1]} {
			if !strings.Contains(words, want) {
				t.Fatalf("the offer lacks %q:\n%s", want, words)
			}
		}
		if s := ansi.Strip(m.render()); !strings.Contains(s, step.Fingerprint) {
			t.Fatalf("the screen lacks the fingerprint:\n%s", s)
		}
		if !m.pending.destructive {
			t.Fatal("⏎ would run it: a key install needs a typed y")
		}
		next, cmd := key(m, "y")
		m = settle(t, next, cmd)
		calls := *h.calls
		if len(calls) < 1 || !slices.Equal(calls[0].argv, install) || !slices.Equal(calls[0].env, fakeAsk{}.Env("ws-1")) {
			t.Fatalf("ssh calls %q", calls)
		}
		if checks := *h.checks; len(checks) != 2 || !slices.Equal(checks[1], remote.KeyCheckArgv("ws-1")) {
			t.Fatalf("checks %q", checks)
		}
		if m.login != remote.LoginOK || len(*h.ran) != 1 {
			t.Fatalf("login %v, ran %q, flash %q", m.login, *h.ran, m.flash)
		}
		// The offer is gone: the next jump just goes.
		next, cmd = m.confirm(*m.forHost(m.opt.Host, m.jump("%1")))
		if m = settle(t, next, cmd); m.pending != nil || len(*h.ran) != 2 {
			t.Fatalf("second jump: pending %+v, ran %q", m.pending, *h.ran)
		}
	})
	t.Run("n is not now, and the jump asks in the field", func(t *testing.T) {
		h := ladderHost(t, step, passwordRefusal)
		m := jumpOffer(t, h)
		next, cmd := key(m, "n")
		m = settle(t, next, cmd)
		dir := filepath.Join(m.opt.MasterBase, "ssh")
		calls := *h.calls
		if len(calls) != 2 || !slices.Equal(calls[1].argv, remote.OpenArgv("ws-1", dir)) || !slices.Equal(calls[1].env, fakeAsk{}.Env("ws-1")) {
			t.Fatalf("ssh calls %q", calls)
		}
		if len(*h.ran) != 1 {
			t.Fatalf("ran %q", *h.ran)
		}
		next, cmd = m.confirm(*m.forHost(m.opt.Host, m.jump("%1")))
		if m = settle(t, next, cmd); m.pending != nil || len(*h.ran) != 2 {
			t.Fatalf("not now asks again: pending %+v", m.pending)
		}
	})
	t.Run("esc cancels the jump, and the next jump offers again", func(t *testing.T) {
		h := ladderHost(t, step, passwordRefusal)
		m := jumpOffer(t, h)
		next, cmd := key(m, "esc")
		m = settle(t, next, cmd)
		if len(*h.calls) != 0 || len(*h.ran) != 0 || !strings.HasPrefix(m.flash, "cancelled") {
			t.Fatalf("calls %q, ran %q, flash %q", *h.calls, *h.ran, m.flash)
		}
		jumpOffer(t, passwordHost{m: m, calls: h.calls, ran: h.ran, checks: h.checks})
	})
}

func TestNoPassphraseKeyWarns(t *testing.T) {
	const path = "/home/builder/.ssh/id_ed25519"
	h := ladderHost(t, login.Step{Source: login.FromFile, Path: path, Line: testKey, Fingerprint: login.Fingerprint(testKey), NoPassphrase: true}, passwordRefusal)
	m := jumpOffer(t, h)
	words := offerWords(m)
	for _, want := range []string{"no passphrase", "ssh-keygen -p -f " + path, path} {
		if !strings.Contains(words, want) {
			t.Fatalf("the offer lacks %q:\n%s", want, words)
		}
	}
}

// TestKeyOnlyHostOffersNoLadder: a host that takes keys only cannot take
// a password to install one: the console says the fix, and offers nothing.
func TestKeyOnlyHostOffersNoLadder(t *testing.T) {
	h := ladderHost(t, login.Step{Source: login.FromAgent, Line: testKey, Fingerprint: login.Fingerprint(testKey)}, "me@ws-1: Permission denied (publickey).\r\n")
	if h.m.login != remote.LoginKeyOnly {
		t.Fatalf("login %v", h.m.login)
	}
	want := "ws-1 takes keys only, and none of yours: add your public key to ~/.ssh/authorized_keys on ws-1 another way, such as from a session you have there, then open ws-1 again"
	if h.m.flash != want {
		t.Fatalf("flash %q\nwant %q", h.m.flash, want)
	}
	hostJump(&h.m, remote.Jump{Pane: "%1"})
	next, cmd := h.m.confirm(*h.m.forHost(h.m.opt.Host, h.m.jump("%1")))
	if m := settle(t, next, cmd); m.pending != nil && m.pending.climb != nil {
		t.Fatalf("a ladder on a key-only host: %+v", m.pending)
	}
}

// fakeKeygen is the console's ssh runner with ssh-keygen made real: the
// passphrase a builder would type in the field is pass. ssh-add and the
// host's ssh only answer.
func fakeKeygen(t *testing.T, h *passwordHost, pass string) {
	t.Helper()
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("no ssh-keygen here")
	}
	inner := h.m.opt.SSH
	h.m.opt.SSH = func(ctx context.Context, argv, env []string, line func(string)) (string, int) {
		if argv[0] != "ssh-keygen" {
			return inner(ctx, argv, env, line)
		}
		*h.calls = append(*h.calls, sshCall{argv, env})
		out, err := exec.Command(argv[0], append(argv[1:], "-N", pass)...).CombinedOutput() //nolint:gosec // test
		if err != nil {
			return string(out), 1
		}
		return "", 0
	}
}

// TestNewKeyThenInstall: no key: y makes one through the field, adds it to
// the agent, then shows it with its fingerprint for one more y.
func TestNewKeyThenInstall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_ed25519")
	h := ladderHost(t, login.Step{Source: login.NewKey, Path: path}, passwordRefusal)
	fakeKeygen(t, &h, "p4ss")
	m := jumpOffer(t, h)
	words := offerWords(m)
	if !strings.Contains(words, remote.Quote(login.NewKeyArgv(path))) || !strings.Contains(words, "passphrase") {
		t.Fatalf("the offer:\n%s", words)
	}
	next, cmd := key(m, "y")
	m = settle(t, next, cmd)
	calls := *h.calls
	if len(calls) != 2 || !slices.Equal(calls[0].argv, login.NewKeyArgv(path)) || !slices.Equal(calls[1].argv, login.AddArgv(path, "linux")) {
		t.Fatalf("calls %q", calls)
	}
	for _, c := range calls {
		if !slices.Equal(c.env, fakeAsk{}.Env("this machine")) {
			t.Fatalf("%q env %q", c.argv, c.env)
		}
	}
	pub, err := os.ReadFile(path + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(pub))
	if m.pending == nil || m.pending.climb == nil || m.pending.climb.step.Line != line || !strings.Contains(offerWords(m), login.Fingerprint(line)) {
		t.Fatalf("no install offer for the new key: %+v", m.pending)
	}
	next, cmd = key(m, "y")
	m = settle(t, next, cmd)
	if m.login != remote.LoginOK || len(*h.ran) != 1 {
		t.Fatalf("login %v, ran %q, flash %q", m.login, *h.ran, m.flash)
	}
}

// TestNewKeyWithoutPassphraseRefused: a new key that ssh-keygen does not
// refuse to open empty never goes on the host: the console removes the
// files it made and says so loudly.
func TestNewKeyWithoutPassphraseRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_ed25519")
	h := ladderHost(t, login.Step{Source: login.NewKey, Path: path}, passwordRefusal)
	fakeKeygen(t, &h, "")
	m := jumpOffer(t, h)
	next, cmd := key(m, "y")
	m = settle(t, next, cmd)
	if m.pending != nil || len(*h.ran) != 0 || len(*h.calls) != 1 {
		t.Fatalf("pending %+v, ran %q, calls %q", m.pending, *h.ran, *h.calls)
	}
	if want := "refused: the new key " + path + " is not locked by a passphrase, so matchblox removed it and put nothing on ws-1: jump again and type a passphrase"; m.flash != want {
		t.Fatalf("flash %q\nwant %q", m.flash, want)
	}
	for _, p := range []string{path, path + ".pub"} {
		if _, err := os.Lstat(p); err == nil {
			t.Errorf("%s is still there", p)
		}
	}
}

// TestNewKeyNeverOverwrites: a .pub or a key at the path stops the new key
// before ssh-keygen runs, and stays as it is.
func TestNewKeyNeverOverwrites(t *testing.T) {
	for _, name := range []string{"id_ed25519", "id_ed25519.pub"} {
		dir := t.TempDir()
		path := filepath.Join(dir, "id_ed25519")
		if err := os.WriteFile(filepath.Join(dir, name), []byte("mine\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		h := ladderHost(t, login.Step{Source: login.NewKey, Path: path}, passwordRefusal)
		fakeKeygen(t, &h, "p4ss")
		m := jumpOffer(t, h)
		next, cmd := key(m, "y")
		m = settle(t, next, cmd)
		if len(*h.calls) != 0 || !strings.HasPrefix(m.flash, "refused: "+filepath.Join(dir, name)+" is there") {
			t.Fatalf("%s: calls %q, flash %q", name, *h.calls, m.flash)
		}
		if b, _ := os.ReadFile(filepath.Join(dir, name)); string(b) != "mine\n" {
			t.Fatalf("%s changed: %q", name, b)
		}
	}
}

// TestSecurityKeyNoPassphraseWarning: a security key's file needs the
// token too: no "anyone who reads it" warning.
func TestSecurityKeyNoPassphraseWarning(t *testing.T) {
	const sk = "sk-ssh-ed25519@openssh.com AAAAGnNrLXNzaC1lZDI1NTE5QG9wZW5zc2guY29tAAAAIM4bfwfUcmyCX2tnMZ4mHkeEjSRJnkf0ehLEuB8iWLHUAAAABHNzaDo= yubikey"
	h := ladderHost(t, login.Step{Source: login.FromFile, Path: "/home/builder/.ssh/id_ed25519_sk", Line: sk, Fingerprint: login.Fingerprint(sk), NoPassphrase: true}, passwordRefusal)
	if words := offerWords(jumpOffer(t, h)); strings.Contains(words, "no passphrase") || strings.Contains(words, "anyone who reads") {
		t.Fatalf("a warning for a security key:\n%s", words)
	}
}

// TestKeychainDoorAfterInstall: on macOS, after the install, the console
// shows the ~/.ssh/config diff for a typed y, then the jump goes.
func TestKeychainDoorAfterInstall(t *testing.T) {
	h := ladderHost(t, login.Step{Source: login.FromAgent, Line: testKey, Fingerprint: login.Fingerprint(testKey)}, passwordRefusal)
	var opened []string
	h.m.opt.Keys.Keychain = func(host string) doors.Door {
		return doors.Door{ID: doors.SSHKeychain, Title: "Keep the key's passphrase in the Keychain", Path: "/h/.ssh/config", Sum: "s1",
			Preview: "+Host " + host + "\n+  AddKeysToAgent yes\n+  UseKeychain yes"}
	}
	h.m.opt.Keys.OpenKeychain = func(host, seen string) (string, error) { opened = append(opened, host+" "+seen); return "", nil }
	m := jumpOffer(t, h)
	next, cmd := key(m, "y")
	m = settle(t, next, cmd)
	if m.pending == nil || m.pending.climb == nil || !strings.Contains(offerWords(m), "+  UseKeychain yes") || len(*h.ran) != 0 {
		t.Fatalf("no door: pending %+v, ran %q", m.pending, *h.ran)
	}
	next, cmd = key(m, "y")
	m = settle(t, next, cmd)
	if !slices.Equal(opened, []string{"ws-1 s1"}) || len(*h.ran) != 1 {
		t.Fatalf("opened %q, ran %q", opened, *h.ran)
	}
}

// TestLadderDeclines: n and esc on each offer that the other tests answer
// with y. n is not now and the jump goes; esc on an offer before the
// install cancels the jump; after the install, the door's n and esc only
// keep the file, and the jump goes.
func TestLadderDeclines(t *testing.T) {
	newKey := func(t *testing.T) passwordHost {
		return ladderHost(t, login.Step{Source: login.NewKey, Path: filepath.Join(t.TempDir(), "id_ed25519")}, passwordRefusal)
	}
	keygens := func(h passwordHost) int {
		n := 0
		for _, c := range *h.calls {
			if c.argv[0] == "ssh-keygen" {
				n++
			}
		}
		return n
	}
	t.Run("new key, n", func(t *testing.T) {
		h := newKey(t)
		m := jumpOffer(t, h)
		next, cmd := key(m, "n")
		m = settle(t, next, cmd)
		if keygens(h) != 0 || !m.notNow || len(*h.ran) != 1 {
			t.Fatalf("keygen %d, notNow %v, ran %q, flash %q", keygens(h), m.notNow, *h.ran, m.flash)
		}
	})
	t.Run("new key, esc", func(t *testing.T) {
		h := newKey(t)
		m := jumpOffer(t, h)
		next, cmd := key(m, "esc")
		m = settle(t, next, cmd)
		if len(*h.calls) != 0 || m.notNow || len(*h.ran) != 0 || !strings.Contains(m.flash, "the jump did not run") {
			t.Fatalf("calls %q, notNow %v, ran %q, flash %q", *h.calls, m.notNow, *h.ran, m.flash)
		}
	})
	for _, k := range []string{"n", "esc"} {
		t.Run("Keychain door, "+k, func(t *testing.T) {
			h := ladderHost(t, login.Step{Source: login.FromAgent, Line: testKey, Fingerprint: login.Fingerprint(testKey)}, passwordRefusal)
			var opened []string
			h.m.opt.Keys.Keychain = func(host string) doors.Door {
				return doors.Door{ID: doors.SSHKeychain, Title: "Keep the key's passphrase in the Keychain", Path: "/h/.ssh/config", Sum: "s1", Preview: "+Host " + host}
			}
			h.m.opt.Keys.OpenKeychain = func(host, seen string) (string, error) { opened = append(opened, host); return "", nil }
			m := jumpOffer(t, h)
			next, cmd := key(m, "y")
			m = settle(t, next, cmd)
			next, cmd = key(m, k)
			m = settle(t, next, cmd)
			if len(opened) != 0 || len(*h.ran) != 1 || !strings.Contains(m.flash, "/h/.ssh/config stays as it is") {
				t.Fatalf("opened %q, ran %q, flash %q", opened, *h.ran, m.flash)
			}
		})
	}
}

// TestInstallFailureSaysWhy: a failed install says why, and that the jump
// it held did not run; an install whose key ssh then does not use says so,
// and the jump goes with the field.
func TestInstallFailureSaysWhy(t *testing.T) {
	step := login.Step{Source: login.FromAgent, Line: testKey, Fingerprint: login.Fingerprint(testKey)}
	failInstall := func(h *passwordHost, stderr string, exit int) {
		inner := h.m.opt.SSH
		h.m.opt.SSH = func(ctx context.Context, argv, env []string, line func(string)) (string, int) {
			if strings.Contains(argv[len(argv)-1], "authorized_keys") {
				*h.calls = append(*h.calls, sshCall{argv, env})
				return stderr, exit
			}
			return inner(ctx, argv, env, line)
		}
	}
	t.Run("refused", func(t *testing.T) {
		h := ladderHost(t, step, passwordRefusal)
		failInstall(&h, passwordRefusal, 255)
		m := jumpOffer(t, h)
		next, cmd := key(m, "y")
		m = settle(t, next, cmd)
		if len(*h.ran) != 0 || !strings.Contains(m.flash, "it refused your login") || !strings.HasSuffix(m.flash, "and the jump did not run") {
			t.Fatalf("ran %q, flash %q", *h.ran, m.flash)
		}
	})
	t.Run("ssh says nothing", func(t *testing.T) {
		h := ladderHost(t, step, passwordRefusal)
		failInstall(&h, "", 1)
		m := jumpOffer(t, h)
		next, cmd := key(m, "y")
		m = settle(t, next, cmd)
		if len(*h.ran) != 0 || !strings.HasPrefix(m.flash, "your key is not on ws-1") {
			t.Fatalf("ran %q, flash %q", *h.ran, m.flash)
		}
	})
	t.Run("installed, but ssh does not use it", func(t *testing.T) {
		h := ladderHost(t, step, passwordRefusal)
		h.m.opt.Check = func(context.Context, []string, func(string)) (string, int) { return passwordRefusal, 255 }
		m := jumpOffer(t, h)
		next, cmd := key(m, "y")
		m = settle(t, next, cmd)
		if !strings.Contains(m.flash, "ssh cannot use it without a prompt") || len(*h.ran) != 1 {
			t.Fatalf("ran %q, flash %q", *h.ran, m.flash)
		}
	})
}

// TestStaleStepOffersNothing: two jumps confirmed while the keys were read
// give two offers; once one is answered, the other is old and shows
// nothing.
func TestStaleStepOffersNothing(t *testing.T) {
	step := login.Step{Source: login.FromAgent, Line: testKey, Fingerprint: login.Fingerprint(testKey)}
	h := ladderHost(t, step, passwordRefusal)
	m := jumpOffer(t, h)
	a := *m.forHost(m.opt.Host, m.jump("%1"))
	stale := stepMsg{step: step, jump: &a, gen: m.opt.Gen}
	next, cmd := key(m, "y")
	m = settle(t, next, cmd)
	if next, _ := m.Update(stale); next.(Model).pending != nil {
		t.Fatalf("an old offer came back: %+v", next.(Model).pending)
	}
}
