package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/askpass"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

// sshCall is one ssh the console started for the builder's login.
type sshCall struct{ argv, env []string }

// askingHost is the console of ws-1 with prompts in the console and a
// master directory, its login check done (ok). up says if the master runs;
// open is the stderr and exit of the master's open.
func askingHost(t *testing.T, up bool, openErr string, openExit int) (Model, *[]sshCall, *[][]string) {
	t.Helper()
	m, _, _ := checkedHost(t, fixtureState(), "", 0)
	m.opt.Ask = fakeAsk{}
	m.askCtx, m.askStop = context.WithCancel(context.Background())
	t.Cleanup(m.askStop)
	calls := &[]sshCall{}
	m.opt.SSH = func(_ context.Context, argv, env []string, _ func(string)) (string, int) {
		*calls = append(*calls, sshCall{argv, env})
		if slices.Contains(argv, "-O") {
			if up {
				return "", 0
			}
			return "Control socket connect: No such file or directory\n", 255
		}
		return openErr, openExit
	}
	ran := &[][]string{}
	m.opt.Run = func(argv []string) error { *ran = append(*ran, argv); return nil }
	next, cmd := m.Update(hostHello("3.4"))
	next, _ = next.Update(loginCheckOf(cmd)[0])
	return next.(Model), calls, ran
}

// confirmJump confirms a jump to %1 and runs what it starts, message by
// message, until nothing is left.
func confirmJump(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := m.confirm(*m.forHost(m.jump("%1")))
	for cmd != nil {
		next, cmd = next.Update(cmd())
	}
	return next.(Model)
}

func batchJump(m *Model) (remote.Jump, []string) {
	j := hostJump(m, remote.Jump{Pane: "%1"})
	dir := filepath.Join(m.opt.MasterBase, "ssh")
	return j, j.Local(remote.JumpSSH("ws-1", j, append(remote.MasterOpts(dir), "-o", "BatchMode=yes")))
}

// TestJumpMasterUpRunsDirectly: a master that runs takes the jump at once;
// the jump's ssh never asks (BatchMode).
func TestJumpMasterUpRunsDirectly(t *testing.T) {
	m, calls, ran := askingHost(t, true, "", 0)
	_, want := batchJump(&m)
	confirmJump(t, m)
	dir := filepath.Join(m.opt.MasterBase, "ssh")
	if len(*calls) != 1 || !slices.Equal((*calls)[0].argv, remote.MasterUpArgv("ws-1", dir)) || (*calls)[0].env != nil {
		t.Fatalf("ssh calls %q", *calls)
	}
	if len(*ran) != 1 || !slices.Equal((*ran)[0], want) {
		t.Fatalf("ran %q\nwant %q", *ran, want)
	}
}

// TestJumpMasterDownOpensFirst: no master: the console opens it first, with
// the prompts in the console and no BatchMode, then the jump runs with
// BatchMode.
func TestJumpMasterDownOpensFirst(t *testing.T) {
	m, calls, ran := askingHost(t, false, "", 0)
	_, want := batchJump(&m)
	confirmJump(t, m)
	dir := filepath.Join(m.opt.MasterBase, "ssh")
	if len(*calls) != 2 {
		t.Fatalf("ssh calls %q", *calls)
	}
	open := (*calls)[1]
	if !slices.Equal(open.argv, remote.OpenArgv("ws-1", dir)) || slices.Contains(open.argv, "BatchMode=yes") {
		t.Fatalf("the open %q", open.argv)
	}
	if !slices.Equal(open.env, fakeAsk{}.Env("ws-1")) {
		t.Fatalf("the open's env %q", open.env)
	}
	if len(*ran) != 1 || !slices.Equal((*ran)[0], want) {
		t.Fatalf("ran %q\nwant %q", *ran, want)
	}
}

// TestJumpOpenFailsRunsNothing: an open that fails runs no jump, and says
// why with the fix.
func TestJumpOpenFailsRunsNothing(t *testing.T) {
	m, _, ran := askingHost(t, false, "builder@ws-1: Permission denied (publickey,password).\r\n", 255)
	hostJump(&m, remote.Jump{Pane: "%1"})
	next := confirmJump(t, m)
	if len(*ran) != 0 {
		t.Fatalf("a jump ran: %q", *ran)
	}
	if want := "the jump did not run: ws-1 refused your login: jump again to type it again, or put your key there (ssh-copy-id ws-1)"; next.flash != want {
		t.Fatalf("flash %q\nwant %q", next.flash, want)
	}
	if next.opening {
		t.Fatal("still opening")
	}
}

// TestJumpOpenCancelledSaysSo: the builder cancelled the prompt: the
// flash says so, not why ssh failed.
func TestJumpOpenCancelledSaysSo(t *testing.T) {
	m, _, ran := askingHost(t, false, "builder@ws-1: Permission denied (publickey,password).\r\n", 255)
	hostJump(&m, remote.Jump{Pane: "%1"})
	next, cmd := m.confirm(*m.forHost(m.jump("%1")))
	if cmd == nil {
		t.Fatalf("no open: flash %q", next.(Model).flash)
	}
	opened := cmd()
	a := &answers{}
	next, _ = next.Update(askMsg{prompt: askpass.Prompt{Host: "ws-1", Text: "Password: "}, answer: a.answer})
	next, _ = key(next, "esc")
	next, cmd = next.Update(opened)
	if cmd != nil {
		next.Update(cmd())
	}
	if len(*ran) != 0 || !strings.HasPrefix(next.(Model).flash, "cancelled") {
		t.Fatalf("ran %q, flash %q", *ran, next.(Model).flash)
	}
}

// TestJumpNoMasterInsideTmuxRefused: inside tmux the jump switches the
// client at once, so with no master to open first it is refused, with the
// fix; outside tmux it runs as before, its prompt in its own terminal.
func TestJumpNoMasterInsideTmuxRefused(t *testing.T) {
	m, calls, ran := askingHost(t, false, "", 0)
	if err := os.Chmod(m.opt.MasterBase, 0o755); err != nil {
		t.Fatal(err)
	}
	hostJump(&m, remote.Jump{Pane: "%1"})
	next := confirmJump(t, m)
	if len(*ran) != 0 || len(*calls) != 0 {
		t.Fatalf("ran %q, ssh %q", *ran, *calls)
	}
	want := "refused: jumps to ws-1 need their ssh master: " + m.opt.MasterBase + " is open to other users (mode 755); it must be 0700: remove it, then open the host again"
	if next.flash != want {
		t.Fatalf("flash %q\nwant %q", next.flash, want)
	}

	m, calls, _ = askingHost(t, false, "", 0)
	execs := &[][]string{}
	m.opt.OutsideTmux = true
	m.opt.Exec = func(argv []string, done func(error) tea.Msg) tea.Cmd {
		*execs = append(*execs, argv)
		return func() tea.Msg { return done(nil) }
	}
	if err := os.Chmod(m.opt.MasterBase, 0o755); err != nil {
		t.Fatal(err)
	}
	j := hostJump(&m, remote.Jump{Pane: "%1"})
	confirmJump(t, m)
	if want := remote.JumpSSH("ws-1", j, nil); len(*execs) != 1 || !slices.Equal((*execs)[0], want) || len(*calls) != 0 {
		t.Fatalf("outside tmux: execs %q, ssh %q", *execs, *calls)
	}
}

// TestJumpOutsideTmuxOpensFirst: outside tmux the master opens first too,
// and the jump's ssh takes the terminal with BatchMode.
func TestJumpOutsideTmuxOpensFirst(t *testing.T) {
	m, calls, _ := askingHost(t, false, "", 0)
	execs := &[][]string{}
	m.opt.OutsideTmux = true
	m.opt.Exec = func(argv []string, done func(error) tea.Msg) tea.Cmd {
		*execs = append(*execs, argv)
		return func() tea.Msg { return done(nil) }
	}
	j := hostJump(&m, remote.Jump{Pane: "%1"})
	confirmJump(t, m)
	want := remote.JumpSSH("ws-1", j, append(remote.MasterOpts(filepath.Join(m.opt.MasterBase, "ssh")), "-o", "BatchMode=yes"))
	if len(*calls) != 2 || len(*execs) != 1 || !slices.Equal((*execs)[0], want) {
		t.Fatalf("ssh %q, execs %q", *calls, *execs)
	}
}

// TestJumpCarriesNoAskpassEnv: the askpass environment goes only on the
// ssh the console starts itself. The jump's command list names none, and
// the jump's session on this machine has none.
func TestJumpCarriesNoAskpassEnv(t *testing.T) {
	m, _, ran := askingHost(t, false, "", 0)
	hostJump(&m, remote.Jump{Pane: "%1"})
	confirmJump(t, m)
	if len(*ran) != 1 {
		t.Fatalf("ran %q", *ran)
	}
	if line := strings.Join((*ran)[0], " "); strings.Contains(line, askpass.EnvSocket) || strings.Contains(line, "SSH_ASKPASS") {
		t.Fatalf("the jump names the askpass env: %s", line)
	}

	// The command list on a tmux server of its own, with an ssh that waits.
	sock := tmuxtest.Server(t)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + t.TempDir(), "TERM=xterm-256color"}
	tmux := func(args ...string) (string, error) {
		cmd := exec.Command("tmux", append([]string{"-L", sock, "-f", "/dev/null"}, args...)...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := tmux("new-session", "-d", "-s", "matchblox", "sleep", "30"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	argv := (*ran)[0]
	_, _ = tmux(argv[1:]...) // switch-client fails: no client
	s := remote.HostSessionName("ws-1", "0a1b2c3d")
	out, err := tmux("show-environment", "-t", "="+s)
	if err != nil {
		t.Fatalf("no session %s: %v: %s", s, err, out)
	}
	// tmux lists a variable it removed as -NAME: SSH_ASKPASS is in its
	// default update-environment.
	for l := range strings.SplitSeq(out, "\n") {
		if !strings.HasPrefix(l, "-") && strings.Contains(l, "ASKPASS") {
			t.Fatalf("the jump's session has the askpass env:\n%s", out)
		}
	}
}

// openingHost is the console of ws-1 whose open-first runs: its ssh waits
// until its context ends, then says so on killed.
func openingHost(t *testing.T) (tea.Model, tea.Cmd, chan struct{}, *[][]string) {
	t.Helper()
	m, _, ran := askingHost(t, false, "", 0)
	killed := make(chan struct{})
	m.opt.SSH = func(ctx context.Context, argv, _ []string, _ func(string)) (string, int) {
		if slices.Contains(argv, "-O") {
			return "", 255
		}
		<-ctx.Done()
		close(killed)
		return "", -1
	}
	hostJump(&m, remote.Jump{Pane: "%1"})
	next, cmd := m.confirm(*m.forHost(m.jump("%1")))
	if !next.(Model).opening {
		t.Fatal("no open-first")
	}
	return next, cmd, killed, ran
}

// bgCmd runs cmd in the background and gives its message.
func bgCmd(cmd tea.Cmd) <-chan tea.Msg {
	out := make(chan tea.Msg, 1)
	go func() { out <- cmd() }()
	return out
}

// TestOpenFirstKeysReachNothingBelow: between two prompts of an open-first
// (sshd's failure delay) the field waits, and every key goes to it: no
// search opens, no confirm, no quit, and nothing is typed anywhere.
func TestOpenFirstKeysReachNothingBelow(t *testing.T) {
	next, cmd, _, _ := openingHost(t)
	nm := next.(Model)
	nm.tab = tabSessions
	nm.pending = nm.forHost(nm.jump("%1")) // a confirm left from before
	next, _ = nm.Update(askMsg{prompt: askpass.Prompt{Host: "ws-1", Text: "Password: "}, answer: func(string, bool) {}})
	next, _ = key(typeIn(next, "wrong"), "enter")
	if out := ansi.Strip(next.(Model).render()); !strings.Contains(out, "waiting for ssh") {
		t.Fatalf("no waiting field:\n%s", out)
	}
	for _, k := range []string{"/", "s", "3", "c", "r", "e", "t", "y", "enter", "q", "x", "N"} {
		var c tea.Cmd
		next, c = key(next, k)
		if c != nil {
			if _, quit := c().(tea.QuitMsg); quit {
				t.Fatalf("%q quit", k)
			}
		}
	}
	nm = next.(Model)
	if nm.searching || nm.filter != "" || nm.pending != nil || nm.quitting || nm.form != nil || nm.held != nil {
		t.Fatalf("a key reached a layer below: search %v %q, pending %v, quitting %v", nm.searching, nm.filter, nm.pending, nm.quitting)
	}
	if out := ansi.Strip(nm.render()); strings.Contains(out, "s3cret") || strings.Contains(out, "s3c") {
		t.Fatalf("typed text drawn:\n%s", out)
	}
	_ = cmd
}

// TestOpenFirstEscCancels: esc while the open-first waits kills its ssh,
// and the jump does not run.
func TestOpenFirstEscCancels(t *testing.T) {
	next, cmd, killed, ran := openingHost(t)
	done := bgCmd(cmd)
	next, _ = key(next, "esc")
	select {
	case <-killed:
	case <-time.After(5 * time.Second):
		t.Fatal("esc did not kill the open-first ssh")
	}
	next, c := next.Update(<-done)
	if c != nil {
		next.Update(c())
	}
	nm := next.(Model)
	if nm.opening || len(*ran) != 0 || !strings.HasPrefix(nm.flash, "cancelled") {
		t.Fatalf("opening %v, ran %q, flash %q", nm.opening, *ran, nm.flash)
	}
	if strings.Contains(ansi.Strip(nm.render()), "waiting for ssh") {
		t.Fatal("the waiting field stays")
	}
}

// TestOpenFirstEndsWithTheConsole: back to Hosts kills the open-first ssh.
func TestOpenFirstEndsWithTheConsole(t *testing.T) {
	next, cmd, killed, _ := openingHost(t)
	bgCmd(cmd)
	next.(Model).stopAsk()
	select {
	case <-killed:
	case <-time.After(5 * time.Second):
		t.Fatal("the open-first ssh outlived its console")
	}
}

// TestRunSSHStreamsLines: each line of stderr reaches line as ssh prints
// it, not when it ends; the stderr comes back whole.
func TestRunSSHStreamsLines(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "go")
	if err := exec.Command("mkfifo", fifo).Run(); err != nil {
		t.Skip("no fifo here:", err)
	}
	lines := make(chan string, 4)
	stderr := make(chan string, 1)
	go func() {
		// The second line waits for the test: the first comes while ssh runs.
		s, _ := runSSH(context.Background(), []string{"sh", "-c", "echo one >&2; read _ < " + fifo + "; printf 'two\\r\\n' >&2"}, nil, time.Minute, func(l string) { lines <- l })
		stderr <- s
	}()
	select {
	case l := <-lines:
		if l != "one" {
			t.Fatalf("line %q", l)
		}
	case s := <-stderr:
		t.Fatalf("the line came only at the end: %q", s)
	case <-time.After(10 * time.Second):
		t.Fatal("no line")
	}
	if err := os.WriteFile(fifo, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := <-stderr; s != "one\ntwo\r\n" || <-lines != "two" {
		t.Fatalf("stderr %q", s)
	}
}

// TestRunSSHNoBinary: an ssh that does not start says why, with ssh's own
// exit for a failure.
func TestRunSSHNoBinary(t *testing.T) {
	if s, exit := runSSH(context.Background(), []string{"/nonexistent/ssh"}, nil, time.Minute, ignoreLine); exit != 255 || s == "" {
		t.Fatalf("exit %d, stderr %q", exit, s)
	}
}

// TestRunSSHLimit: the time limit ends an ssh that hangs.
func TestRunSSHLimit(t *testing.T) {
	start := time.Now()
	if _, exit := runSSH(context.Background(), []string{"sleep", "30"}, nil, 100*time.Millisecond, ignoreLine); exit == 0 || time.Since(start) > 5*time.Second {
		t.Fatalf("exit %d after %v", exit, time.Since(start))
	}
}

// TestRunSSHKilledByContext: the open-first's ssh ends with its context.
func TestRunSSHKilledByContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	start := time.Now()
	if _, exit := runSSH(ctx, []string{"sleep", "30"}, nil, time.Minute, func(string) {}); exit == 0 || time.Since(start) > 5*time.Second {
		t.Fatalf("exit %d after %v", exit, time.Since(start))
	}
}

// TestRunSSHHasNoTerminal: the console's ssh runs in a session of its own,
// with no controlling terminal: an ssh before 8.4 ignores
// SSH_ASKPASS_REQUIRE and would ask on the console's terminal, over the
// screen, and read the keys meant for the console.
func TestRunSSHHasNoTerminal(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ps -o sid is procps")
	}
	script := `[ "$(ps -o sid= -p $$ | tr -d ' ')" = "$$" ] || { echo "sid $(ps -o sid= -p $$)" >&2; exit 1; }`
	if stderr, exit := runSSH(context.Background(), []string{"sh", "-c", script}, nil, 10*time.Second, ignoreLine); exit != 0 {
		t.Fatalf("not a session leader: %s", stderr)
	}
}

// TestJumpLocalFailureEndsItsSession: a local command list that fails after
// it made the jump's session (its ssh runs there) ends that session; else
// an ssh would run on, unseen.
func TestJumpLocalFailureEndsItsSession(t *testing.T) {
	m, _, ran := askingHost(t, true, "", 0)
	j, _ := batchJump(&m)
	m.opt.Run = func(argv []string) error {
		*ran = append(*ran, argv)
		if len(*ran) == 1 {
			return errors.New("exit status 1")
		}
		return nil
	}
	confirmJump(t, m)
	kill := []string{"tmux", "kill-session", "-t", "=" + remote.HostSessionName(j.Host, j.ID)}
	if len(*ran) != 2 || !slices.Equal((*ran)[1], kill) {
		t.Fatalf("ran %q", *ran)
	}
}

// TestJumpMakesTheMasterDirAgain: a /tmp cleaner may remove the master
// directory after the check; the jump makes it again, before ssh needs it.
func TestJumpMakesTheMasterDirAgain(t *testing.T) {
	m, calls, _ := askingHost(t, false, "", 0)
	if err := os.RemoveAll(m.opt.MasterBase); err != nil {
		t.Fatal(err)
	}
	confirmJump(t, m)
	if fi, err := os.Stat(filepath.Join(m.opt.MasterBase, "ssh")); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("no master directory at the open: %v", err)
	}
	if len(*calls) == 0 {
		t.Fatal("no ssh ran")
	}
}
