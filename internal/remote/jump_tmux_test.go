//go:build unix

package remote

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"

	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

// These tests run a jump against two real tmux servers: the "outer" one is
// the laptop, the "inner" one is the host. A fake ssh runs the host's
// command list on the inner server.

type jumpEnv struct {
	outer, inner string
	ssh          string // the fake ssh
	pane         string // the agent pane in window 1 of session work
}

func tmuxOut(sock string, args ...string) (string, error) {
	cmd := exec.Command("tmux", append([]string{"-L", sock}, args...)...) //nolint:gosec // a test's own socket name
	cmd.Env = withoutTMUX()
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func withoutTMUX() []string {
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "TMUX=") && !strings.HasPrefix(e, "TMUX_PANE=") {
			env = append(env, e)
		}
	}
	return append(env, "TERM=xterm-256color")
}

func mustTmux(t *testing.T, sock string, args ...string) string {
	t.Helper()
	out, err := tmuxOut(sock, args...)
	if err != nil {
		t.Fatalf("tmux -L %s %s: %v\n%s", sock, strings.Join(args, " "), err, out)
	}
	return out
}

// waitFor polls until cond is true; it fails the test after five seconds.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout: %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func hasSession(sock, name string) bool {
	_, err := tmuxOut(sock, "has-session", "-t", "="+name)
	return err == nil
}

// requireTmux skips the test below tmux major.minor.
func requireTmux(t *testing.T, major, minor int, why string) {
	t.Helper()
	out, err := exec.Command("tmux", "-V").Output()
	if err != nil {
		t.Skip("tmux is not installed")
	}
	var ma, mi int
	v := strings.TrimPrefix(strings.TrimSpace(string(out)), "tmux ")
	v = strings.TrimPrefix(v, "next-")
	if _, err := fmt.Sscanf(v, "%d.%d", &ma, &mi); err != nil {
		return // a development build: assume it is new
	}
	if ma < major || (ma == major && mi < minor) {
		t.Skipf("tmux %d.%d or newer is needed: %s", major, minor, why)
	}
}

func newJumpEnv(t *testing.T) jumpEnv {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the clients run on util-linux script -qfc")
	}
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script is not installed")
	}
	e := jumpEnv{outer: tmuxtest.Server(t), inner: tmuxtest.Server(t)}
	// The agent's window prints a marker; the host's own console would
	// show a status line with a tap, which a jump must not show twice.
	mustTmux(t, e.outer, "-f", "/dev/null", "new-session", "-d", "-s", "matchblox", "-x", "120", "-y", "30", "sleep", "600")
	mustTmux(t, e.inner, "-f", "/dev/null", "new-session", "-d", "-s", "work", "-x", "120", "-y", "30", "sleep", "600")
	mustTmux(t, e.inner, "new-window", "-t", "=work:", "sh", "-c", "echo AGENT-PANE; sleep 600")
	mustTmux(t, e.inner, "set-option", "-g", "status", "on")
	mustTmux(t, e.inner, "set-option", "-g", "status-left", "◂ matchblox ")
	mustTmux(t, e.inner, "select-window", "-t", "=work:0")
	e.pane = mustTmux(t, e.inner, "list-panes", "-t", "=work:1", "-F", "#{pane_id}")
	e.ssh = jumpSSH(t, e.inner, "")
	return e
}

// jumpSSH is an ssh that ignores its options and runs its last argument
// (the host's command, see onHost) as the host would, with a tmux on the
// host's own PATH that is the inner server: every tmux the command runs
// goes there. A command of another shape exits 99: it must never reach the
// default server. With fail set, it prints that line and exits 255, as ssh
// does for a refused port.
func jumpSSH(t *testing.T, inner, fail string) string {
	t.Helper()
	return jumpSSHWith(t, inner, fail, "")
}

// jumpSSHWith is jumpSSH whose host tmux first runs pre, a shell line that
// sees the tmux arguments as "$@".
func jumpSSHWith(t *testing.T, inner, fail, pre string) string {
	t.Helper()
	dir := t.TempDir()
	if fail != "" {
		return script(t, filepath.Join(dir, "ssh"), "#!/bin/sh\necho '"+fail+"' >&2\nexit 255\n")
	}
	real, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}
	home := filepath.Join(dir, "home")
	script(t, filepath.Join(home, ".local", "bin", "tmux"), "#!/bin/bash\n"+pre+"\nunset TMUX TMUX_PANE\nexec "+real+" -L "+inner+" \"$@\"\n")
	return script(t, filepath.Join(dir, "ssh"), `#!/bin/bash
cmd="${@: -1}"
case "$cmd" in
"/bin/sh -c "*) ;;
*) echo "fake ssh: not an onHost command: $cmd" >&2; exit 99 ;;
esac
export HOME=`+home+`
eval "exec $cmd"
`)
}

func script(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil { //nolint:gosec // a test script must be executable
		t.Fatal(err)
	}
	return path
}

// client is a real tmux client of a server, on a pty made by script.
type client struct {
	t     *testing.T
	sock  string
	stdin io.WriteCloser
	tty   string
}

// attach starts a tmux client on session and waits for it to show up.
func attach(t *testing.T, sock, session string) *client {
	t.Helper()
	before := clientTTYs(sock)
	cmd := exec.Command("script", "-qfc", //nolint:gosec // a test's own socket name
		"stty rows 30 cols 120; exec tmux -L "+sock+" attach -t '="+session+"'", "/dev/null")
	cmd.Env = withoutTMUX()
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = in.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	c := &client{t: t, sock: sock, stdin: in}
	waitFor(t, "client of "+sock+" attached to "+session, func() bool {
		for _, tty := range clientTTYs(sock) {
			if !contains(before, tty) {
				c.tty = tty
				return true
			}
		}
		return false
	})
	return c
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func clientTTYs(sock string) []string {
	out, _ := tmuxOut(sock, "list-clients", "-F", "#{client_tty}")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func (c *client) session() string {
	out, _ := tmuxOut(c.sock, "list-clients", "-F", "#{client_tty} #{client_session}")
	for _, l := range strings.Split(out, "\n") {
		if tty, s, ok := strings.Cut(l, " "); ok && tty == c.tty {
			return s
		}
	}
	return ""
}

func (c *client) waitSession(want string) {
	c.t.Helper()
	waitFor(c.t, "client on "+want+", is on "+c.session(), func() bool { return c.session() == want })
}

// type_ sends each key on its own, as a person types.
func (c *client) type_(keys ...string) {
	c.t.Helper()
	for _, k := range keys {
		if _, err := io.WriteString(c.stdin, k); err != nil {
			c.t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
	}
}

// jump runs the laptop's command list with the console's own ssh command
// (ssh is the fake), for a client: the one change is -c, since the test
// has no pane to take the client from.
func (e jumpEnv) jump(t *testing.T, c *client, j Jump, ssh string) {
	t.Helper()
	argv := j.Local(append([]string{ssh}, JumpSSH(j.Host, j, nil)[1:]...))
	i := slices.Index(argv, "switch-client")
	if i < 0 {
		t.Fatalf("no switch-client in the list: %v", argv)
	}
	argv = slices.Insert(argv, i+1, "-c", c.tty)
	mustTmux(t, e.outer, argv[1:]...)
}

func (e jumpEnv) newJump(id string) Jump {
	return Jump{Host: "ws-1", Pane: e.pane, Prefix: "C-b", ID: id, Outer: true}
}

func TestJumpOneStatusLine(t *testing.T) {
	e := newJumpEnv(t)
	c := attach(t, e.outer, "matchblox")
	j := e.newJump("0a1b2c3d")
	e.jump(t, c, j, e.ssh)
	s, g := HostSessionName(j.Host, j.ID), JumpSessionName(j.ID)
	c.waitSession(s)
	waitFor(t, "the pane of the agent shows in the outer pane", func() bool {
		out, _ := tmuxOut(e.outer, "capture-pane", "-p", "-t", "="+s+":")
		return strings.Contains(out, "AGENT-PANE")
	})
	if got := mustTmux(t, e.inner, "show-options", "-v", "-t", "="+g+":", "status"); got != "off" {
		t.Errorf("status of the host's jump session = %q, want off", got)
	}
	if out := mustTmux(t, e.outer, "capture-pane", "-p", "-t", "="+s+":"); strings.Contains(out, "◂ matchblox") {
		t.Errorf("the host's console status line shows in the jump:\n%s", out)
	}
	// The sampler on either side sees no pane of a jump's session.
	for sock, name := range map[string]string{e.outer: s, e.inner: g} {
		for _, p := range sample.ParsePanes([]byte(mustTmux(t, sock, "list-panes", "-a", "-F", sample.PaneFormat))) {
			if strings.HasPrefix(p.Target, name+":") {
				t.Errorf("the sampler lists %s", p.Target)
			}
		}
	}
}

func TestJumpKeepsOtherClientsWindow(t *testing.T) {
	e := newJumpEnv(t)
	other := attach(t, e.inner, "work") // on window 0
	c := attach(t, e.outer, "matchblox")
	j := e.newJump("0a1b2c3d")
	e.jump(t, c, j, e.ssh)
	g := JumpSessionName(j.ID)
	waitFor(t, "the jump is attached", func() bool { return len(clientsOf(e.inner, g)) == 1 })
	if got := mustTmux(t, e.inner, "display-message", "-p", "-t", "="+g+":", "#{window_index}"); got != "1" {
		t.Errorf("the jump is on window %s, want 1", got)
	}
	if got := mustTmux(t, e.inner, "display-message", "-p", "-t", "=work:", "#{window_index}"); got != "0" {
		t.Errorf("another client's window is %s, want 0", got)
	}
	if other.session() != "work" {
		t.Errorf("another client is on %q", other.session())
	}
}

func clientsOf(sock, session string) []string {
	out, err := tmuxOut(sock, "list-clients", "-t", "="+session, "-F", "#{client_tty}")
	if err != nil || out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func TestJumpWayBackHomeKey(t *testing.T) {
	requireTmux(t, 3, 4, "send-keys -K")
	e := newJumpEnv(t)
	c := attach(t, e.outer, "matchblox")
	j := e.newJump("0a1b2c3d")
	e.jump(t, c, j, e.ssh)
	c.waitSession(HostSessionName(j.Host, j.ID))
	mustTmux(t, e.outer, "send-keys", "-K", "-c", c.tty, "User900")
	c.waitSession("matchblox")
	waitFor(t, "the jump session is gone", func() bool { return !hasSession(e.outer, HostSessionName(j.Host, j.ID)) })
}

func TestJumpWayBackTap(t *testing.T) {
	requireTmux(t, 3, 4, "user mouse ranges and mouse_status_range")
	e := newJumpEnv(t)
	c := attach(t, e.outer, "matchblox")
	j := e.newJump("0a1b2c3d")
	e.jump(t, c, j, e.ssh)
	c.waitSession(HostSessionName(j.Host, j.ID))
	// A left click (SGR mouse) on the first columns of the status line.
	c.type_("\x1b[<0;5;30M\x1b[<0;5;30m")
	c.waitSession("matchblox")
	waitFor(t, "the jump session is gone", func() bool { return !hasSession(e.outer, HostSessionName(j.Host, j.ID)) })
}

func TestJumpWayBackWithPrefixCa(t *testing.T) {
	e := newJumpEnv(t)
	mustTmux(t, e.outer, "set-option", "-g", "prefix", "C-a")
	mustTmux(t, e.outer, "bind-key", "C-a", "send-prefix")
	c := attach(t, e.outer, "matchblox")
	j := e.newJump("0a1b2c3d")
	j.Prefix = "C-a"
	e.jump(t, c, j, e.ssh)
	s, g := HostSessionName(j.Host, j.ID), JumpSessionName(j.ID)
	c.waitSession(s)
	waitFor(t, "the jump is attached", func() bool { return len(clientsOf(e.inner, g)) == 1 })
	c.type_("\x01", "\x01", "m") // the outer tmux takes the first C-a (bind C-a send-prefix), the host the second
	c.waitSession("matchblox")
	waitFor(t, "both jump sessions are gone", func() bool {
		return !hasSession(e.outer, s) && !hasSession(e.inner, g)
	})
}

// TestJumpDropLeavesNothing: the link drops (the laptop slept) while the
// builder is in the jump: the host's session goes, the error stays on the
// screen though the killed ssh left the terminal raw, and Enter goes back
// with nothing left here.
func TestJumpDropLeavesNothing(t *testing.T) {
	e := newJumpEnv(t)
	c := attach(t, e.outer, "matchblox")
	j := e.newJump("0a1b2c3d")
	e.jump(t, c, j, e.ssh)
	s, g := HostSessionName(j.Host, j.ID), JumpSessionName(j.ID)
	c.waitSession(s)
	waitFor(t, "the jump is attached", func() bool { return len(clientsOf(e.inner, g)) == 1 })
	killSSH(t, e.outer, s) // the laptop slept: the ssh died under the builder
	waitFor(t, "the host's jump session is gone", func() bool { return !hasSession(e.inner, g) })
	waitFor(t, "the drop says so", func() bool {
		out, _ := tmuxOut(e.outer, "capture-pane", "-p", "-t", "="+s+":")
		return strings.Contains(out, "ssh exited")
	})
	c.type_("\r")
	waitFor(t, "the outer session is gone", func() bool { return !hasSession(e.outer, s) })
	c.waitSession("matchblox")
}

// killSSH kills the ssh of the session's pane (the fake ssh is the host's
// tmux client itself), as a dropped link ends it: a failed exit.
func killSSH(t *testing.T, sock, session string) {
	t.Helper()
	sh := mustTmux(t, sock, "display-message", "-p", "-t", "="+session+":", "#{pane_pid}")
	var tree []int
	waitFor(t, "ssh runs under the wrapper", func() bool {
		tree = descendants(sh)
		return len(tree) > 0
	})
	// A dead ssh leaves nothing here: the fake's own children go too,
	// deepest first.
	for i := len(tree) - 1; i >= 0; i-- {
		_ = syscall.Kill(tree[i], syscall.SIGKILL)
	}
}

// descendants are the processes under pid, parents first.
func descendants(pid string) []int {
	out, err := exec.Command("pgrep", "-P", pid).Output() //nolint:gosec // a pid of the test's own pane
	if err != nil {
		return nil
	}
	var all []int
	for _, f := range strings.Fields(string(out)) {
		if n, err := strconv.Atoi(f); err == nil {
			all = append(append(all, n), descendants(f)...)
		}
	}
	return all
}

func TestJumpFailedSSHIsReadable(t *testing.T) {
	e := newJumpEnv(t)
	c := attach(t, e.outer, "matchblox")
	j := e.newJump("0a1b2c3d")
	e.jump(t, c, j, jumpSSH(t, e.inner, "ssh: connect to host ws-1 port 22: Connection refused"))
	s := HostSessionName(j.Host, j.ID)
	c.waitSession(s)
	var out string
	waitFor(t, "the error is on the screen", func() bool {
		out, _ = tmuxOut(e.outer, "capture-pane", "-p", "-t", "="+s+":")
		return strings.Contains(out, "ssh exited 255")
	})
	if !strings.Contains(out, "Connection refused") {
		t.Errorf("ssh's own error is lost:\n%s", out)
	}
	// Without the read, the session ends within milliseconds of the line;
	// a wait many times that is the probe. gstack-timing-ok: no clock asserted.
	time.Sleep(300 * time.Millisecond)
	if !hasSession(e.outer, s) || c.session() != s {
		t.Fatal("the session ended before Enter")
	}
	c.type_("\r")
	c.waitSession("matchblox")
	waitFor(t, "the session is gone after Enter", func() bool { return !hasSession(e.outer, s) })
}

func TestTwoJumpsSameHost(t *testing.T) {
	e := newJumpEnv(t)
	a, b := attach(t, e.outer, "matchblox"), attach(t, e.outer, "matchblox")
	ja, jb := e.newJump("0a1b2c3d"), e.newJump("4e5f6a7b")
	e.jump(t, a, ja, e.ssh)
	e.jump(t, b, jb, e.ssh)
	sa, sb := HostSessionName(ja.Host, ja.ID), HostSessionName(jb.Host, jb.ID)
	a.waitSession(sa)
	b.waitSession(sb)
	waitFor(t, "both jumps are attached", func() bool {
		return len(clientsOf(e.inner, JumpSessionName(ja.ID))) == 1 && len(clientsOf(e.inner, JumpSessionName(jb.ID))) == 1
	})
	if !hasSession(e.outer, sa) || !hasSession(e.outer, sb) || a.session() != sa || b.session() != sb {
		t.Errorf("a jump moved the other: a on %q, b on %q", a.session(), b.session())
	}
}

// TestWayBackEndsOnlyItsJump: two clients of the console jump, and the
// first goes back: its jump ends, the other's stays. Each jump binds its
// way back in a table of its own; one shared table would end the jump
// that bound it last. The way back also drops its table.
func TestWayBackEndsOnlyItsJump(t *testing.T) {
	requireTmux(t, 3, 4, "send-keys -K")
	e := newJumpEnv(t)
	a, b := attach(t, e.outer, "matchblox"), attach(t, e.outer, "matchblox")
	ja, jb := e.newJump("0a1b2c3d"), e.newJump("4e5f6a7b")
	sa, sb := HostSessionName(ja.Host, ja.ID), HostSessionName(jb.Host, jb.ID)
	e.jump(t, a, ja, e.ssh)
	a.waitSession(sa)
	e.jump(t, b, jb, e.ssh)
	b.waitSession(sb)
	mustTmux(t, e.outer, "send-keys", "-K", "-c", a.tty, "User900")
	a.waitSession("matchblox")
	waitFor(t, "the first jump is gone", func() bool { return !hasSession(e.outer, sa) })
	if !hasSession(e.outer, sb) || b.session() != sb {
		t.Fatalf("the way back of one jump ended the other: b on %q", b.session())
	}
	if out, err := tmuxOut(e.outer, "list-keys", "-T", KeyTable(ja.ID)); err == nil {
		t.Errorf("the way back left its table:\n%s", out)
	}
}

// TestJumpHostFailureLeavesNothing: a host command list that fails after
// it made the jump's session ends that session; it would stay forever,
// unattached, hidden from the sampler. Here the agent's pane is gone when
// select-pane runs (tmux parses the whole list first, so only a failure
// while it runs can leave the session).
func TestJumpHostFailureLeavesNothing(t *testing.T) {
	e := newJumpEnv(t)
	j := e.newJump("0a1b2c3d")
	gone := jumpSSHWith(t, e.inner, "", `a=(); sp=0; for w in "$@"; do if [ $sp = 2 ]; then w=%99999; sp=0; fi; [ $sp = 1 ] && [ "$w" = -t ] && sp=2; [ "$w" = select-pane ] && sp=1; a+=("$w"); done; set -- "${a[@]}"`)
	c := attach(t, e.outer, "matchblox")
	e.jump(t, c, j, gone)
	s := HostSessionName(j.Host, j.ID)
	c.waitSession(s)
	waitFor(t, "the failed ssh says so", func() bool {
		out, _ := tmuxOut(e.outer, "capture-pane", "-p", "-t", "="+s+":")
		return strings.Contains(out, "ssh exited")
	})
	if out, _ := tmuxOut(e.outer, "capture-pane", "-p", "-t", "="+s+":"); !strings.Contains(out, "can't find pane") {
		t.Fatalf("not the failure this test makes:\n%s", out)
	}
	if hasSession(e.inner, JumpSessionName(j.ID)) {
		t.Fatal("the failed jump left its session on the host")
	}
}

// TestLeavingEndsTheJump: prefix m (the way back door's switch-client)
// leaves the jump, and like the tap and the home key it ends it: the
// session here, its table and the host's session go. Nothing runs unseen.
func TestLeavingEndsTheJump(t *testing.T) {
	e := newJumpEnv(t)
	c := attach(t, e.outer, "matchblox")
	j := e.newJump("0a1b2c3d")
	e.jump(t, c, j, e.ssh)
	s, g := HostSessionName(j.Host, j.ID), JumpSessionName(j.ID)
	c.waitSession(s)
	waitFor(t, "the jump is attached on the host", func() bool { return len(clientsOf(e.inner, g)) == 1 })
	mustTmux(t, e.outer, "switch-client", "-c", c.tty, "-t", "=matchblox")
	c.waitSession("matchblox")
	waitFor(t, "the jump's session here is gone", func() bool { return !hasSession(e.outer, s) })
	waitFor(t, "the host's session is gone", func() bool { return !hasSession(e.inner, g) })
	waitFor(t, "the jump's table is gone", func() bool {
		_, err := tmuxOut(e.outer, "list-keys", "-T", KeyTable(j.ID))
		return err != nil
	})
}

// TestJumpEndLandsOnTheConsole: when a jump ends by itself (Enter after a
// failed ssh), its client goes to the console, not to the session tmux
// would pick (the one most recently active).
func TestJumpEndLandsOnTheConsole(t *testing.T) {
	e := newJumpEnv(t)
	mustTmux(t, e.outer, "new-session", "-d", "-s", "busy", "sleep", "600")
	c := attach(t, e.outer, "matchblox")
	j := e.newJump("0a1b2c3d")
	e.jump(t, c, j, jumpSSH(t, e.inner, "ssh: connect to host ws-1 port 22: Connection refused"))
	s := HostSessionName(j.Host, j.ID)
	c.waitSession(s)
	other := attach(t, e.outer, "busy")
	other.type_("x") // busy is now the most recently active session
	waitFor(t, "the error is on the screen", func() bool {
		out, _ := tmuxOut(e.outer, "capture-pane", "-p", "-t", "="+s+":")
		return strings.Contains(out, "ssh exited")
	})
	c.type_("\r")
	waitFor(t, "the session is gone", func() bool { return !hasSession(e.outer, s) })
	c.waitSession("matchblox")
}
