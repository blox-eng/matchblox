package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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
	calls := &[]sshCall{}
	m.opt.SSH = func(argv, env []string) (string, int) {
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
