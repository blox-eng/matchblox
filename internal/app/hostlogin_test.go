package app

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/askpass"
	"github.com/blox-eng/matchblox/internal/login"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
)

// homeLogin is the login of the console's home host.
func homeLogin(m Model) hostLogin { return m.loginOf(m.opt.Host) }

// loginHosts is a console on ws-1 with the peer gpu-1, prompts in the
// console, a master directory and the ladder's key. Each host's check
// answers with first, exit 255 when it is not "". No master is up, and
// an open succeeds.
type loginHosts struct {
	m      Model
	gpu    *fakeConn
	checks *[][]string
	opens  *[][]string
}

func twoLogins(t *testing.T, first string) loginHosts {
	t.Helper()
	h := loginHosts{gpu: newFake(), checks: &[][]string{}, opens: &[][]string{}}
	m := New(Options{NoMotion: true, Host: "ws-1", Conn: newFake(), Peers: []Peer{{Host: "gpu-1", Conn: h.gpu}},
		MasterBase: shortTemp(t), Ask: fakeAsk{}, JumpID: func() string { return "0a1b2c3d" },
		Keys: &Keys{Step: func() login.Step { return login.Step{Source: login.FromAgent, Line: testKey, Fingerprint: "SHA256:k"} }, GOOS: "linux"},
		Check: func(_ context.Context, argv []string, _ func(string)) (string, int) {
			*h.checks = append(*h.checks, argv)
			if first != "" {
				return first, 255
			}
			return "", 0
		},
		SSH: func(_ context.Context, argv, _ []string, _ func(string)) (string, int) {
			if slices.Contains(argv, "-O") {
				return "Control socket connect: No such file or directory\n", 255
			}
			*h.opens = append(*h.opens, argv)
			return "", 0
		},
		Run: func([]string) error { return nil },
	})
	t.Cleanup(m.askStop)
	m = upd(t, m, tea.WindowSizeMsg{Width: 132, Height: 30})
	m = upd(t, m, stateMsg(hostState(sess(1, "home-1", "%1", nil))))
	m = upd(t, m, peerMsg{host: "gpu-1", conn: h.gpu, msg: stateMsg(hostState(sess(2, "gpu-run", "%2", nil)))})
	next, cmd := m.Update(helloMsg(proto.Hello{Version: proto.Version, Host: "ws-1", Tmux: "3.4"}))
	checks := loginCheckOf(cmd)
	next, cmd = next.Update(peerMsg{host: "gpu-1", conn: h.gpu, msg: helloMsg(proto.Hello{Version: proto.Version, Host: "gpu-1", Tmux: "3.4"})})
	checks = append(checks, loginCheckOf(cmd)...)
	for _, c := range checks {
		next, _ = next.Update(c)
	}
	h.m = next.(Model)
	return h
}

// jumpTo confirms a jump to pane on host. Its cmd opens the master, or
// finds the ladder's step; it does not run here.
func (h *loginHosts) jumpTo(host, pane string) tea.Cmd {
	next, cmd := h.m.confirm(*h.m.forHost(host, h.m.jumpOn(host, pane)))
	h.m = next.(Model)
	return cmd
}

// offerOn jumps to pane on host and takes the ladder's step it finds.
func (h *loginHosts) offerOn(t *testing.T, host, pane string) {
	t.Helper()
	cmd := h.jumpTo(host, pane)
	if cmd == nil {
		t.Fatalf("the jump to %s found no step: flash %q", host, h.m.flash)
	}
	h.m = upd(t, h.m, cmd())
}

// TestChecksAtOpenNeverPrompt: each remote host's login is checked once at
// its open, in the background and in batch mode. A check that finds a
// password asks nothing: no field, no offer.
func TestChecksAtOpenNeverPrompt(t *testing.T) {
	h := twoLogins(t, passwordRefusal)
	dir := filepath.Join(h.m.opt.MasterBase, "ssh")
	want := [][]string{remote.CheckArgv("ws-1", dir), remote.CheckArgv("gpu-1", dir)}
	if !slices.EqualFunc(*h.checks, want, slices.Equal) {
		t.Fatalf("checks %q\nwant %q", *h.checks, want)
	}
	for _, host := range []string{"ws-1", "gpu-1"} {
		if l := h.m.loginOf(host); !l.checked || l.checking || l.login != remote.LoginPassword {
			t.Errorf("%s: checked %v, checking %v, login %v", host, l.checked, l.checking, l.login)
		}
	}
	if h.m.asking() || h.m.pending != nil || len(*h.opens) != 0 {
		t.Fatalf("a check prompted: asking %v, pending %+v, opens %q", h.m.asking(), h.m.pending, *h.opens)
	}
	// A hello after a reconnect checks nothing again.
	_, cmd := h.m.Update(peerMsg{host: "gpu-1", conn: h.gpu, msg: helloMsg(proto.Hello{Version: proto.Version, Host: "gpu-1", Tmux: "3.4"})})
	if got := loginCheckOf(cmd); len(got) != 0 || len(*h.checks) != 2 {
		t.Fatalf("checked again: %v", *h.checks)
	}
}

// TestTwoHostsPromptOneAtATime: two jumps open their hosts' masters at
// once. Both ssh ask a password; the field shows the first, names its
// host, and the second shows only once the first has its answer.
func TestTwoHostsPromptOneAtATime(t *testing.T) {
	h := twoLogins(t, "")
	h.jumpTo("ws-1", "%1")
	h.jumpTo("gpu-1", "%2")
	for _, host := range []string{"ws-1", "gpu-1"} {
		if !h.m.loginOf(host).opening {
			t.Fatalf("%s does not open: flash %q", host, h.m.flash)
		}
	}
	ws, gpu := &answers{}, &answers{}
	h.m = upd(t, h.m, askMsg{prompt: askpass.Prompt{Host: "ws-1", Text: "builder@ws-1's password:"}, answer: ws.answer})
	h.m = upd(t, h.m, askMsg{prompt: askpass.Prompt{Host: "gpu-1", Text: "builder@gpu-1's password:"}, answer: gpu.answer})
	if gpu.sent != 0 {
		t.Fatalf("the second prompt was answered %q before it showed", gpu.got)
	}
	frame := ansi.Strip(h.m.render())
	if !strings.Contains(frame, "ssh ws-1 asks") || !strings.Contains(frame, "ws-1's password") || strings.Contains(frame, "gpu-1's password") {
		t.Fatalf("the field is not ws-1's alone:\n%s", frame)
	}
	h.m = typeIn(h.m, "pw1").(Model)
	h.m = upd(t, h.m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(ws.got) != 1 || ws.got[0] != "pw1" || !ws.oks[0] {
		t.Fatalf("ws-1 got %q %v", ws.got, ws.oks)
	}
	frame = ansi.Strip(h.m.render())
	if h.m.ask == nil || !strings.Contains(frame, "ssh gpu-1 asks") || !strings.Contains(frame, "gpu-1's password") {
		t.Fatalf("gpu-1's prompt does not show next:\n%s", frame)
	}
	h.m = typeIn(h.m, "pw2").(Model)
	h.m = upd(t, h.m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(gpu.got) != 1 || gpu.got[0] != "pw2" || h.m.ask != nil {
		t.Fatalf("gpu-1 got %q, field %+v", gpu.got, h.m.ask)
	}
}

// TestLadderIsPerHost: not now on ws-1's offer keeps the ladder of gpu-1,
// whose first jump still offers the key.
func TestLadderIsPerHost(t *testing.T) {
	h := twoLogins(t, passwordRefusal)
	h.offerOn(t, "ws-1", "%1")
	if a := h.m.pending; a == nil || a.climb == nil || a.label != "put your key on ws-1" {
		t.Fatalf("ws-1 offers %+v, flash %q", a, h.m.flash)
	}
	h.m = upd(t, h.m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if !h.m.loginOf("ws-1").notNow || h.m.loginOf("gpu-1").notNow {
		t.Fatalf("not now: ws-1 %v, gpu-1 %v", h.m.loginOf("ws-1").notNow, h.m.loginOf("gpu-1").notNow)
	}
	h.offerOn(t, "gpu-1", "%2")
	a := h.m.pending
	if a == nil || a.climb == nil || a.label != "put your key on gpu-1" {
		t.Fatalf("gpu-1 offers %+v, flash %q", a, h.m.flash)
	}
	if words := offerWords(h.m); !strings.Contains(words, "gpu-1 asks your password here, once") || strings.Contains(words, "ws-1") {
		t.Fatalf("the offer names another host:\n%s", words)
	}
}

// TestPeerJumpFailureNamesThePeer: an open of gpu-1 that fails says why
// with gpu-1's name and fix, never the home host's.
func TestPeerJumpFailureNamesThePeer(t *testing.T) {
	h := twoLogins(t, "")
	h.jumpTo("gpu-1", "%2")
	a := h.m.forHost("gpu-1", h.m.jumpOn("gpu-1", "%2"))
	h.m = upd(t, h.m, masterMsg{a: *a, gen: h.m.opt.Gen, stderr: "builder@gpu-1: Permission denied (publickey).\r\n", exit: 255})
	if !strings.HasPrefix(h.m.flash, "the jump did not run: gpu-1 takes keys only") || strings.Contains(h.m.flash, "ws-1") {
		t.Fatalf("flash %q", h.m.flash)
	}
	if h.m.loginOf("gpu-1").opening || h.m.asking() {
		t.Fatal("gpu-1 still opens")
	}
}

// TestEscEndsOnlyItsOwnLogin: two opens wait on their prompts; esc on
// gpu-1's ends gpu-1's alone.
func TestEscEndsOnlyItsOwnLogin(t *testing.T) {
	h := twoLogins(t, "")
	h.jumpTo("ws-1", "%1")
	h.jumpTo("gpu-1", "%2")
	h.m = upd(t, h.m, askMsg{prompt: askpass.Prompt{Host: "gpu-1", Text: "builder@gpu-1's password:"}, answer: (&answers{}).answer})
	h.m = upd(t, h.m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if ws, gpu := h.m.loginOf("ws-1"), h.m.loginOf("gpu-1"); ws.askCancelled || !ws.opening || !gpu.askCancelled {
		t.Fatalf("ws-1 cancelled %v, opening %v; gpu-1 cancelled %v", ws.askCancelled, ws.opening, gpu.askCancelled)
	}
}

// makingKey is twoLogins with gpu-1's ladder making a new key, its
// passphrase prompt (named this machine) in the field.
func makingKey(t *testing.T) loginHosts {
	t.Helper()
	h := twoLogins(t, "")
	next, _ := h.m.makeKey(climb{host: "gpu-1", step: login.Step{Source: login.NewKey, Path: filepath.Join(t.TempDir(), "id_ed25519")}})
	h.m = next.(Model)
	return h
}

func keyPrompt() askMsg {
	return askMsg{prompt: askpass.Prompt{Host: newKeyHost, Text: "Enter passphrase (empty for no passphrase):"}, answer: (&answers{}).answer}
}

// TestEscOnTheNewKeyEndsThatLadder: home opens while gpu-1 makes a key;
// esc on the key's passphrase ends gpu-1's ladder, and home's open goes on.
func TestEscOnTheNewKeyEndsThatLadder(t *testing.T) {
	h := makingKey(t)
	h.jumpTo("ws-1", "%1") // its check ended: it opens now
	h.m = upd(t, h.m, keyPrompt())
	h.m = upd(t, h.m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if ws, gpu := h.m.loginOf("ws-1"), h.m.loginOf("gpu-1"); ws.askCancelled || !ws.opening || !gpu.askCancelled {
		t.Fatalf("ws-1 cancelled %v, opening %v; gpu-1 cancelled %v", ws.askCancelled, ws.opening, gpu.askCancelled)
	}
}

// TestEscOnTheNewKeyBesideALink: another host shows a Tailscale link;
// esc on the key's passphrase says no key was made because it cancelled.
func TestEscOnTheNewKeyBesideALink(t *testing.T) {
	h := makingKey(t)
	h.m.editLogin("ws-1", func(l *hostLogin) { l.loginURL = checkURL })
	h.m = upd(t, h.m, keyPrompt())
	h.m = upd(t, h.m, tea.KeyPressMsg{Code: tea.KeyEscape})
	h.m = upd(t, h.m, keyMadeMsg{c: climb{host: "gpu-1"}, gen: h.m.opt.Gen, exit: 1})
	if h.m.flash != "cancelled: no key made" || h.m.loginOf("ws-1").askCancelled {
		t.Fatalf("flash %q, ws-1 cancelled %v", h.m.flash, h.m.loginOf("ws-1").askCancelled)
	}
}
