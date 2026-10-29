package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/transport"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
)

// checkMsgsOf runs cmd and the commands it batches, at once, for the
// login check's messages (its result, a link it printed). A recv on an
// empty connection never ends: it waits up to a second for the result,
// then a moment more for what comes with it.
func checkMsgsOf(cmd tea.Cmd) []tea.Msg {
	type result struct {
		cmds []tea.Cmd
		msg  tea.Msg
	}
	results := make(chan result, 64)
	running := 0
	run := func(c tea.Cmd) {
		if c == nil {
			return
		}
		running++
		go func() {
			msg := c()
			if b, ok := msg.(tea.BatchMsg); ok {
				results <- result{cmds: b}
				return
			}
			results <- result{msg: msg}
		}()
	}
	run(cmd)
	var got []tea.Msg
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for running > 0 {
		select {
		case r := <-results:
			running--
			for _, c := range r.cmds {
				run(c)
			}
			switch r.msg.(type) {
			case loginMsg:
				deadline.Reset(50 * time.Millisecond)
				got = append(got, r.msg)
			case urlMsg:
				got = append(got, r.msg)
			}
		case <-deadline.C:
			return got
		}
	}
	return got
}

// loginCheckOf is the login check's result among cmd's messages.
func loginCheckOf(cmd tea.Cmd) []loginMsg {
	var got []loginMsg
	for _, msg := range checkMsgsOf(cmd) {
		if l, ok := msg.(loginMsg); ok {
			got = append(got, l)
		}
	}
	return got
}

// shortTemp is a temporary directory with a short path: the master's
// socket path under it must fit sun_path.
func shortTemp(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "mb") // not macOS $TMPDIR, too long
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// checkedHost is the onHost console with a cache directory and a check
// that records its argv and answers with the fixture's stderr and exit.
func checkedHost(t *testing.T, st proto.State, stderr string, exit int) (Model, *[][]string, *[][]string) {
	t.Helper()
	m, execs := onHost(t, st)
	m.opt.MasterBase = shortTemp(t)
	var checks [][]string
	m.opt.Check = func(_ context.Context, argv []string, _ func(string)) (string, int) {
		checks = append(checks, argv)
		return stderr, exit
	}
	return m, execs, &checks
}

func hostHello(tmux string) helloMsg {
	return helloMsg(proto.Hello{Version: proto.Version, Host: "ws-1", Tmux: tmux})
}

// TestHostOpenRunsLoginCheck: the first hello of a host's console starts
// one check of the builder's own login, with the master options, in the
// background; a hello after a reconnect does not start another.
func TestHostOpenRunsLoginCheck(t *testing.T) {
	m, _, checks := checkedHost(t, fixtureState(), "", 0)
	next, cmd := m.Update(hostHello("3.4"))
	if !homeLogin(next.(Model)).checking {
		t.Fatal("the hello did not start the check")
	}
	got := loginCheckOf(cmd)
	want := remote.CheckArgv("ws-1", filepath.Join(m.opt.MasterBase, "ssh"))
	if len(*checks) != 1 || !slices.Equal((*checks)[0], want) {
		t.Fatalf("checks %q, want %q", *checks, want)
	}
	if len(got) != 1 || got[0].login != remote.LoginOK {
		t.Fatalf("messages %+v", got)
	}
	next, _ = next.Update(got[0])
	if homeLogin(next.(Model)).checking || next.(Model).flash != "" {
		t.Fatalf("an ok login shows nothing: flash %q", next.(Model).flash)
	}
	_, cmd = next.Update(hostHello("3.4"))
	loginCheckOf(cmd)
	if len(*checks) != 1 {
		t.Fatalf("a second hello checked again: %q", *checks)
	}
}

// TestLocalConsoleRunsNoLoginCheck: this machine's console has no login to
// check.
func TestLocalConsoleRunsNoLoginCheck(t *testing.T) {
	m, _ := loadedWith(t, 100, fixtureState())
	m.opt.Check = func(_ context.Context, argv []string, _ func(string)) (string, int) {
		t.Fatalf("checked %q", argv)
		return "", 0
	}
	next, cmd := m.Update(helloMsg(proto.Hello{Version: proto.Version}))
	loginCheckOf(cmd)
	if homeLogin(next.(Model)).checking {
		t.Fatal("this machine's console checks a login")
	}
}

// TestJumpWaitsForLoginCheck: a jump confirmed while the check runs waits
// for its result, then runs once; a second jump replaces the first.
func TestJumpWaitsForLoginCheck(t *testing.T) {
	m, _, _ := checkedHost(t, fixtureState(), "", 0)
	var ran [][]string
	m.opt.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	next, cmd := m.Update(hostHello("3.4"))
	login := loginCheckOf(cmd)
	m = next.(Model)
	hostJump(&m, remote.Jump{Pane: "%1"})
	next, cmd = m.confirm(*m.forHost(m.opt.Host, m.jump("%1")))
	if cmd != nil || len(ran) != 0 {
		t.Fatalf("the jump ran before the check: %q", ran)
	}
	next, _ = next.(Model).confirm(*m.forHost(m.opt.Host, m.jump("%2")))
	if !strings.Contains(next.(Model).flash, "checking") {
		t.Fatalf("flash %q", next.(Model).flash)
	}
	next, cmd = next.Update(login[0])
	if cmd == nil {
		t.Fatal("the check ended and the jump did not run")
	}
	cmd()
	if len(ran) != 1 || !strings.Contains(strings.Join(ran[0], " "), "%2") {
		t.Fatalf("ran %q, want one jump to %%2", ran)
	}
	if homeLogin(next.(Model)).checking {
		t.Fatal("still checking")
	}
}

// TestJumpUsesMaster: the jump's ssh shares the master in the cache.
func TestJumpUsesMaster(t *testing.T) {
	m, _, _ := checkedHost(t, fixtureState(), "", 0)
	j := hostJump(&m, remote.Jump{Pane: "%1"})
	a := m.forHost(m.opt.Host, m.jump("%1"))
	ssh := remote.JumpSSH("ws-1", j, remote.MasterOpts(filepath.Join(m.opt.MasterBase, "ssh")))
	if a == nil || len(a.steps) != 1 || !slices.Equal(a.steps[0], j.Local(ssh)) {
		t.Fatalf("pending %+v\nwant %q", a, j.Local(ssh))
	}
}

// TestJumpNoMasterWithoutCache: no cache directory, no master; each jump
// logs in again.
func TestJumpNoMasterWithoutCache(t *testing.T) {
	m, _ := onHost(t, fixtureState())
	j := hostJump(&m, remote.Jump{Pane: "%1"})
	a := m.forHost(m.opt.Host, m.jump("%1"))
	if a == nil || !slices.Equal(a.steps[0], j.Local(remote.JumpSSH("ws-1", j, nil))) {
		t.Fatalf("pending %+v", a)
	}
}

// TestLoginResultSaysTheFix: a login that is not ok shows one line that
// names it and its fix; the jump still goes, except to a changed host key.
func TestLoginResultSaysTheFix(t *testing.T) {
	for _, c := range []struct {
		login  remote.Login
		detail string
		want   string
		jumps  bool
	}{
		{remote.LoginPassword, "", "ws-1 asks your password at each jump: put your key there (ssh-copy-id ws-1)", true},
		{remote.LoginTailscaleCheck, "https://login.tailscale.com/a/0123", "Tailscale SSH on ws-1 asks for a check in a browser: the next jump shows its link", true},
		{remote.LoginTailscaleDenied, "tailnet policy does not permit you", "Tailscale SSH on ws-1 refuses you: tailnet policy does not permit you; see your tailnet's SSH rules (docs/remote.md#tailscale-ssh)", true},
		{remote.LoginHostKeyUnknown, "", "ws-1's host key is not known: the jump's ssh shows it; check it before you answer yes", true},
		{remote.LoginUnreachable, "ssh: connect to host ws-1 port 22: Connection refused", "cannot reach ws-1 with your ssh login: ssh: connect to host ws-1 port 22: Connection refused; check that ssh ws-1 works in a terminal", true},
		{remote.LoginUnreachable, "timed out", "cannot reach ws-1 with your ssh login: timed out; check that ssh ws-1 works in a terminal", true},
		{remote.LoginHostKeyChanged, "", "refused: ws-1's host key changed: this can be an attack. Only if you trust it: ssh-keygen -R ws-1", false},
	} {
		m, _, _ := checkedHost(t, fixtureState(), "", 0)
		var ran [][]string
		m.opt.Run = func(argv []string) error { ran = append(ran, argv); return nil }
		next, _ := m.Update(hostHello("3.4"))
		m = next.(Model)
		hostJump(&m, remote.Jump{Pane: "%1"})
		next, _ = m.confirm(*m.forHost(m.opt.Host, m.jump("%1")))
		next, cmd := next.Update(loginMsg{host: "ws-1", login: c.login, detail: c.detail})
		if f := next.(Model).flash; f != c.want {
			t.Errorf("%v: flash %q\nwant %q", c.login, f, c.want)
		}
		if cmd != nil {
			cmd()
		}
		if jumped := len(ran) == 1; jumped != c.jumps {
			t.Errorf("%v: ran %q", c.login, ran)
		}
		if c.jumps {
			continue
		}
		// A jump after the result is refused at once, with the same fix.
		next, _ = next.(Model).confirm(*next.(Model).forHost(next.(Model).opt.Host, m.jump("%1")))
		if f := next.(Model).flash; f != c.want || len(ran) != 0 {
			t.Errorf("%v: a later jump: flash %q, ran %q", c.login, f, ran)
		}
	}
}

// TestOldHostTmuxShowsDoor: a host without tmux, or with one older than
// MinHostTmux, cannot take a jump: it is refused, and the tmux door is
// selected. A host that does not say (an older matchblox) tries, as does a
// host whose version is not a plain one: it is never shown.
func TestOldHostTmuxShowsDoor(t *testing.T) {
	tmux := doors.Door{ID: doors.Tmux, Title: "Install tmux", Term: []string{"sudo", "sh", "-c", "apt-get update && apt-get install -y tmux"}}
	for _, c := range []struct {
		tmux, want string
	}{
		{"none", "refused: ws-1 has no tmux: install it with its Install tmux door, then open the host again"},
		{"3.1", "refused: tmux 3.1 on ws-1 is older than 3.2: update tmux there, then open the host again"},
		{"", ""},
		{"3.4", ""},
		{"3.1\x1b]0;owned\x07", ""},
		{"3.1" + strings.Repeat("a", 18), ""},
	} {
		m, execs, _ := checkedHost(t, doorState(tmux), "", 0)
		next, cmd := m.Update(hostHello(c.tmux))
		next, _ = next.Update(loginCheckOf(cmd)[0])
		m = next.(Model)
		m.tab = tabSessions
		a := m.forHost(m.opt.Host, m.jump("%1"))
		if c.want == "" {
			if a == nil || a.local != nil {
				t.Errorf("%q: refused %+v", c.tmux, a)
			}
			continue
		}
		next, cmd = m.confirm(*a)
		if cmd != nil || len(*execs) != 0 {
			t.Errorf("%q: it ran: %q", c.tmux, *execs)
		}
		if f := next.(Model).flash; f != c.want {
			t.Errorf("%q: flash %q", c.tmux, f)
		}
		if p := next.(Model).doorPick; p != doors.Tmux {
			t.Errorf("%q: the tmux door is not selected: %q", c.tmux, p)
		}
	}
}

// TestHostConsoleKeepsTheLoginOptions: the shell gives a host's console
// the base's check and cache.
func TestHostConsoleKeepsTheLoginOptions(t *testing.T) {
	sh := NewShell(ShellOptions{
		Base: Options{NoMotion: true, MasterBase: "/c", Check: func(context.Context, []string, func(string)) (string, int) { return "", 0 },
			Ask: fakeAsk{}, NoAsk: "why", SSH: func(context.Context, []string, []string, func(string)) (string, int) { return "", 0 }},
		Open: func(host string) Options { return Options{Conn: newFake(), Host: host} },
	})
	opened, _ := sh.open("ws-1", false)
	if o := opened.cur.opt; o.MasterBase != "/c" || o.Check == nil || o.Ask == nil || o.NoAsk != "why" || o.SSH == nil {
		t.Fatalf("the host's console lost the login options: %+v", o)
	}
	// Back to Hosts ends the console's wait for prompts.
	ctx := opened.cur.askCtx
	opened.back()
	if ctx == nil || ctx.Err() == nil {
		t.Fatal("the closed console still waits for prompts")
	}
}

// TestMasterRefusedJumpsWithoutIt: a master directory that is not the
// builder's alone gives no master: outside tmux the check and the jumps go
// without it, and the console says why once, with the fix. Inside tmux the
// jumps are refused (TestJumpNoMasterInsideTmuxRefused).
func TestMasterRefusedJumpsWithoutIt(t *testing.T) {
	m, _, checks := checkedHost(t, fixtureState(), "", 0)
	m.opt.OutsideTmux = true
	if err := os.Chmod(m.opt.MasterBase, 0o755); err != nil {
		t.Fatal(err)
	}
	next, cmd := m.Update(hostHello("3.4"))
	loginCheckOf(cmd)
	want := "jumps to ws-1 ask your login each time: " + m.opt.MasterBase + " is open to other users (mode 755); it must be 0700: remove it, then open the host again"
	if f := next.(Model).flash; f != want {
		t.Fatalf("flash %q\nwant %q", f, want)
	}
	if len(*checks) != 1 || !slices.Equal((*checks)[0], remote.CheckArgv("ws-1", "")) {
		t.Fatalf("checks %q", *checks)
	}
	m = next.(Model)
	j := hostJump(&m, remote.Jump{Pane: "%1"})
	if a := m.forHost(m.opt.Host, m.jump("%1")); a == nil || a.local != nil || !slices.Equal(a.steps[0], remote.JumpSSH("ws-1", j, nil)) {
		t.Fatalf("pending %+v", a)
	}
}

// TestMasterTooLongJumpsWithoutIt: a socket path that does not fit gives
// no master, and the console says why.
func TestMasterTooLongJumpsWithoutIt(t *testing.T) {
	m, _, checks := checkedHost(t, fixtureState(), "", 0)
	m.opt.OutsideTmux = true
	m.opt.MasterBase = filepath.Join(m.opt.MasterBase, strings.Repeat("b", 70))
	next, cmd := m.Update(hostHello("3.4"))
	loginCheckOf(cmd)
	if f := next.(Model).flash; !strings.HasPrefix(f, "jumps to ws-1 ask your login each time: ") || !strings.Contains(f, "too long") {
		t.Fatalf("flash %q", f)
	}
	if len(*checks) != 1 || !slices.Equal((*checks)[0], remote.CheckArgv("ws-1", "")) {
		t.Fatalf("checks %q", *checks)
	}
}

// TestNoAskSaysWhyOnce: with no prompts in the console, a host's console
// says why when it opens, and its jumps ask as before: no master opens
// first, and the jump's ssh may ask.
func TestNoAskSaysWhyOnce(t *testing.T) {
	m, _, _ := checkedHost(t, fixtureState(), "", 0)
	m.opt.NoAsk = "the prompt socket: permission denied"
	m.opt.SSH = func(_ context.Context, argv, _ []string, _ func(string)) (string, int) {
		t.Fatalf("ssh %q", argv)
		return "", 0
	}
	next, cmd := m.Update(hostHello("3.4"))
	if want := "jumps to ws-1 ask your login in the terminal: the prompt socket: permission denied"; next.(Model).flash != want {
		t.Fatalf("flash %q", next.(Model).flash)
	}
	next, _ = next.Update(loginCheckOf(cmd)[0])
	m = next.(Model)
	j := hostJump(&m, remote.Jump{Pane: "%1"})
	a := m.forHost(m.opt.Host, m.jump("%1"))
	if a == nil || a.master != "" || !slices.Equal(a.steps[0], j.Local(remote.JumpSSH("ws-1", j, remote.MasterOpts(filepath.Join(m.opt.MasterBase, "ssh"))))) {
		t.Fatalf("pending %+v", a)
	}
}

// tailscaleFixture is a recorded stderr of Tailscale SSH (internal/remote).
func tailscaleFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "remote", "testdata", "login", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// streamLines gives text to line, line by line, as ssh prints it.
func streamLines(text string, line func(string)) {
	for l := range strings.Lines(text) {
		line(strings.TrimSuffix(l, "\n"))
	}
}

// waitingCheck streams the recorded check mode as tailscaled prints it:
// up to its link, then it waits for approve (exit 0) or its context (-1).
func waitingCheck(t *testing.T, approve <-chan struct{}) func(context.Context, []string, func(string)) (string, int) {
	before, after, _ := strings.Cut(tailscaleFixture(t, "tailscale-check.stderr"), "# Authentication checked")
	after = "# Authentication checked" + after
	return func(ctx context.Context, _ []string, line func(string)) (string, int) {
		streamLines(before, line)
		select {
		case <-approve:
		case <-ctx.Done():
			return before, -1
		}
		streamLines(after, line)
		return before + after, 0
	}
}

// urlOf runs cmd and the commands it batches for the first link message.
func urlOf(t *testing.T, cmd tea.Cmd) urlMsg {
	t.Helper()
	for _, msg := range checkMsgsOf(cmd) {
		if u, ok := msg.(urlMsg); ok {
			return u
		}
	}
	t.Fatal("no link message")
	return urlMsg{}
}

const checkURL = "https://login.tailscale.com/a/0123456789abcd"

// tailscaleHost is ws-1 whose check streams check mode and waits, with a
// jump to %1 confirmed while it runs, and the link on the screen.
func tailscaleHost(t *testing.T, approve <-chan struct{}) (Model, tea.Cmd, *[][]string, *[]string) {
	t.Helper()
	m, _, _ := checkedHost(t, fixtureState(), "", 0)
	m.opt.Check = waitingCheck(t, approve)
	clipped, ran := &[]string{}, &[][]string{}
	m.opt.Clip = func(s string) { *clipped = append(*clipped, s) }
	m.opt.Run = func(argv []string) error { *ran = append(*ran, argv); return nil }
	next, cmd := m.Update(hostHello("3.4"))
	m = next.(Model)
	hostJump(&m, remote.Jump{Pane: "%1"})
	next, _ = m.confirm(*m.forHost(m.opt.Host, m.jump("%1")))
	u := urlOf(t, cmd)
	if u.url != checkURL || !u.tailscale {
		t.Fatalf("link %+v", u)
	}
	next, cmd = next.Update(u)
	return next.(Model), cmd, ran, clipped
}

// TestTailscaleCheckShowsURL: check mode shows its link the moment
// tailscaled prints it, as an OSC 8 link in the field, while the check
// waits; c copies it; the field holds the other keys; the ladder is never
// offered; the approved check runs the jump and leaves no old link.
func TestTailscaleCheckShowsURL(t *testing.T) {
	approve := make(chan struct{})
	m, wait, ran, clipped := tailscaleHost(t, approve)
	if homeLogin(m).loginURL != checkURL || !m.asking() {
		t.Fatalf("link %q, asking %v", homeLogin(m).loginURL, m.asking())
	}
	out := m.render()
	if !strings.Contains(out, ansi.SetHyperlink(checkURL)) || !strings.Contains(ansi.Strip(out), "ssh ws-1 asks") {
		t.Fatalf("no OSC 8 link in the field:\n%q", out)
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd != nil {
		cmd()
	}
	if !slices.Equal(*clipped, []string{checkURL}) {
		t.Fatalf("clipped %q", *clipped)
	}
	next, _ = next.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if len(*ran) != 0 || homeLogin(next.(Model)).loginURL != checkURL {
		t.Fatalf("a key went past the field: ran %q", *ran)
	}
	close(approve)
	next, cmd = next.Update(wait())
	if cmd == nil {
		t.Fatal("the approved check did not run the jump")
	}
	cmd()
	m = next.(Model)
	if len(*ran) != 1 || homeLogin(m).loginURL != "" || m.asking() || strings.Contains(m.flash, "tailscale.com") {
		t.Fatalf("ran %q, link %q, flash %q", *ran, homeLogin(m).loginURL, m.flash)
	}
	if !homeLogin(m).tailscale {
		t.Fatal("the console forgot that ws-1 is Tailscale SSH")
	}
	m.opt.Keys, m.opt.Ask = &Keys{}, fakeAsk{}
	m.editLogin(m.opt.Host, func(l *hostLogin) { l.login = remote.LoginPassword })
	if m.offersLadder(m.opt.Host) {
		t.Fatal("the ladder is offered on Tailscale SSH: authorized_keys does nothing there")
	}
}

// TestTailscaleCheckEscCancels: esc on the link ends the check's ssh, and
// the jump that waited does not run.
func TestTailscaleCheckEscCancels(t *testing.T) {
	m, wait, ran, _ := tailscaleHost(t, make(chan struct{}))
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	done := make(chan tea.Msg, 1)
	go func() { done <- wait() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("esc did not end the check's ssh")
	}
	next, cmd := next.Update(msg)
	if cmd != nil {
		cmd()
	}
	m = next.(Model)
	if len(*ran) != 0 || homeLogin(m).loginURL != "" || !strings.HasPrefix(m.flash, "cancelled") {
		t.Fatalf("ran %q, link %q, flash %q", *ran, homeLogin(m).loginURL, m.flash)
	}
	if homeLogin(m).login != remote.LoginTailscaleCheck {
		t.Fatalf("login %v: the next jump must ask for the check again", homeLogin(m).login)
	}
}

// TestTailscaleDeniedSaysWhy: a refusal by the tailnet's rules says
// Tailscale's reason and where the rules are explained.
func TestTailscaleDeniedSaysWhy(t *testing.T) {
	m, _, _ := checkedHost(t, fixtureState(), tailscaleFixture(t, "tailscale-denied.stderr"), 255)
	next, cmd := m.Update(hostHello("3.4"))
	next, _ = next.Update(loginCheckOf(cmd)[0])
	want := `Tailscale SSH on ws-1 refuses you: tailnet policy does not permit you to SSH as user "nosuchuser"; see your tailnet's SSH rules (docs/remote.md#tailscale-ssh)`
	if f := next.(Model).flash; f != want {
		t.Fatalf("flash %q\nwant %q", f, want)
	}
}

// TestTailscaleOpenFirstShowsURL: a jump meets check mode in the
// open-first (no -v, so no version line): the host's kind from its check
// lets the link show, live; the approved open runs the jump.
func TestTailscaleOpenFirstShowsURL(t *testing.T) {
	m, _, ran := askingHost(t, false, "", 0)
	m.editLogin(m.opt.Host, func(l *hostLogin) { l.tailscale = true })
	approve := make(chan struct{})
	open := waitingCheck(t, approve)
	m.opt.SSH = func(ctx context.Context, argv, _ []string, line func(string)) (string, int) {
		if slices.Contains(argv, "-O") {
			return "", 255
		}
		return open(ctx, argv, func(l string) {
			if !strings.HasPrefix(l, "debug1:") {
				line(l)
			}
		})
	}
	hostJump(&m, remote.Jump{Pane: "%1"})
	next, cmd := m.confirm(*m.forHost(m.opt.Host, m.jump("%1")))
	u, ok := cmd().(urlMsg)
	if !ok || u.url != checkURL {
		t.Fatalf("link %+v", u)
	}
	next, cmd = next.Update(u)
	if homeLogin(next.(Model)).loginURL != checkURL || !strings.Contains(next.(Model).render(), ansi.SetHyperlink(checkURL)) {
		t.Fatal("the open-first's link is not shown")
	}
	close(approve)
	for cmd != nil {
		next, cmd = next.Update(cmd())
	}
	if m := next.(Model); len(*ran) != 1 || homeLogin(m).loginURL != "" || homeLogin(m).opening || strings.Contains(m.flash, "tailscale.com") {
		t.Fatalf("ran %q, link %q, flash %q", *ran, homeLogin(m).loginURL, m.flash)
	}
}

// TestUnexpectedLoginLinkNotShown: a host that is not Tailscale SSH and
// prints Tailscale's link line gets no link on the screen.
func TestUnexpectedLoginLinkNotShown(t *testing.T) {
	m, _, _ := askingHost(t, false, "", 0)
	m.opt.SSH = func(_ context.Context, argv, _ []string, line func(string)) (string, int) {
		if slices.Contains(argv, "-O") {
			return "", 255
		}
		line("# To authenticate, visit: " + checkURL)
		return "", 255
	}
	hostJump(&m, remote.Jump{Pane: "%1"})
	next, cmd := m.confirm(*m.forHost(m.opt.Host, m.jump("%1")))
	u, ok := cmd().(urlMsg)
	if !ok {
		t.Fatal("no link message")
	}
	next, _ = next.Update(u)
	m = next.(Model)
	if homeLogin(m).loginURL != "" || strings.Contains(m.render(), checkURL) {
		t.Fatal("a link from a host that is not Tailscale SSH is shown")
	}
	if want := "ws-1 printed an unexpected login link: nothing was opened"; m.flash != want {
		t.Fatalf("flash %q", m.flash)
	}
}

// TestStreamedLimit: an ssh that waits is ended at its limit; a Tailscale
// link moves the limit out (to openTimeout in the console).
func TestStreamedLimit(t *testing.T) {
	waits := func(link string) func(context.Context, func(string)) tea.Msg {
		return func(ctx context.Context, line func(string)) tea.Msg {
			line("debug1: Remote protocol version 2.0, remote software version Tailscale")
			if link != "" {
				line("# To authenticate, visit: " + link)
			}
			select {
			case <-ctx.Done():
				return "ended"
			case <-time.After(300 * time.Millisecond):
				return "done"
			}
		}
	}
	drain := func(cmd tea.Cmd) tea.Msg {
		msg := cmd()
		for u, ok := msg.(urlMsg); ok; u, ok = msg.(urlMsg) {
			msg = u.next()
		}
		return msg
	}
	if got := drain(streamed(context.Background(), "ws-1", 1, false, 50*time.Millisecond, 5*time.Second, waits(""))); got != "ended" {
		t.Fatalf("no link: %v", got)
	}
	if got := drain(streamed(context.Background(), "ws-1", 1, false, 50*time.Millisecond, 5*time.Second, waits(checkURL))); got != "done" {
		t.Fatalf("a link: %v", got)
	}
	if got := drain(streamed(context.Background(), "ws-1", 1, false, 50*time.Millisecond, 5*time.Second, waits("https://evil.example/a/1"))); got != "ended" {
		t.Fatalf("a link that is not Tailscale's: %v", got)
	}
}

// TestMasterWhyNamesAFixThatWorks: a directory another user made cannot be
// removed by the builder; the fix says who can.
func TestMasterWhyNamesAFixThatWorks(t *testing.T) {
	if got := masterWhy(fmt.Errorf("/tmp/matchblox-1000 belongs to uid 1001: %w", transport.ErrNotYours)); strings.Contains(got, "remove it, then") || !strings.Contains(got, "only its owner or root") {
		t.Errorf("masterWhy = %q", got)
	}
}

// TestPrefixFallbackSaysSo: a tmux prefix a jump cannot bind becomes C-b
// for the host's keys; the console says so once, at the check, not later
// in a jump that answers to another key.
func TestPrefixFallbackSaysSo(t *testing.T) {
	m, _, _ := checkedHost(t, fixtureState(), "", 0)
	m.opt.Prefix = func() string { return "C-F1" }
	next, _ := m.Update(hostHello("3.4"))
	if f := next.(Model).flash; !strings.Contains(f, "C-F1") || !strings.Contains(f, "C-b") {
		t.Fatalf("flash %q", f)
	}
	m, _, _ = checkedHost(t, fixtureState(), "", 0)
	m.opt.Prefix = func() string { return "C-Space" }
	if next, _ := m.Update(hostHello("3.4")); next.(Model).flash != "" {
		t.Fatalf("a prefix a jump binds: flash %q", next.(Model).flash)
	}
}

// TestCheckerAsksInTheConsole: the background check and the key check run
// with the console's prompt socket: a security key asks its touch there
// (BatchMode lets nothing else ask), and does not time out unseen.
func TestCheckerAsksInTheConsole(t *testing.T) {
	m, _ := loadedWith(t, 100, fixtureState())
	m.opt.Host, m.opt.Ask = "ws-1", fakeAsk{}
	if _, exit := m.checker(m.opt.Host)(context.Background(), []string{"sh", "-c", `[ "$SSH_ASKPASS_REQUIRE" = force ] && [ -n "$MATCHBLOX_ASKPASS_HOST" ]`}, ignoreLine); exit != 0 {
		t.Fatal("the check's ssh has no prompt socket")
	}
}
