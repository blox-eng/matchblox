//go:build linux

package panes

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

const rawCat = "stty raw -echo; exec cat -v > " // exec: the pane runs cat once raw

// besideEnv is a matchblox window with the list pane C, which runs no
// console (sleep), and the agent pane A beside it, with a client attached.
type besideEnv struct {
	tmux       func(args ...string) string
	sock       string
	term       *os.File
	c, a       string
	cOut, aOut string
}

func newBesideEnv(t *testing.T) besideEnv {
	t.Helper()
	sock := tmuxtest.Server(t)
	e := besideEnv{sock: sock, cOut: t.TempDir() + "/c", aOut: t.TempDir() + "/a"}
	e.tmux = func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", sock, "-f", os.DevNull}, args...)...).CombinedOutput() //nolint:gosec // test argv
		if err != nil {
			t.Fatalf("tmux %q: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	e.c = e.tmux("new-session", "-d", "-s", Home, "-x", "100", "-y", "30", "-P", "-F", "#{pane_id}", "sh", "-c", rawCat+e.cOut)
	e.tmux("set-option", "-t", e.c, HomeOption, "1")
	e.a = e.tmux("split-window", "-d", "-h", "-t", e.c, "-P", "-F", "#{pane_id}", "sh", "-c", rawCat+e.aOut)
	e.tmux("set-option", "-w", "-t", e.c, sample.ConsoleOption, e.c, ";", "set-option", "-w", "-t", e.c, sample.BesideOption, e.a)
	e.tmux(BesideKeysArgv()[1:]...)
	e.waitRaw(t, e.c)
	e.waitRaw(t, e.a)
	e.term = attach(t, sock, Home)
	waitFor(t, func() bool { return e.tmux("list-clients", "-F", "#{client_session}") == Home })
	return e
}

// waitRaw waits until pane runs cat, after its stty raw: before it the
// terminal's flow control takes ctrl+q (XON) and the pane reads nothing.
func (e besideEnv) waitRaw(t *testing.T, pane string) {
	t.Helper()
	waitFor(t, func() bool { return e.tmux("display-message", "-p", "-t", pane, "#{pane_current_command}") == "cat" })
}

func (e besideEnv) active() string {
	return e.tmux("display-message", "-p", "-t", Home+":", "#{pane_id}")
}

func (e besideEnv) press(t *testing.T, s string) {
	t.Helper()
	if _, err := e.term.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func (e besideEnv) waitActive(t *testing.T, pane string) {
	t.Helper()
	waitFor(t, func() bool { return e.active() == pane })
}

func TestCtrlQGoesToTheAgentAndBack(t *testing.T) {
	e := newBesideEnv(t)
	e.tmux("select-pane", "-t", e.a)
	e.press(t, "\x11")
	e.waitActive(t, e.c)
	e.press(t, "\x11")
	waitFor(t, func() bool {
		b, _ := os.ReadFile(e.cOut)
		return strings.Contains(string(b), "^Q")
	})
	if e.active() != e.c {
		t.Fatalf("focus left the list for %s", e.active())
	}
}

func TestCtrlQWithNoLastPaneGoesToTheAgent(t *testing.T) {
	e := newBesideEnv(t)
	e.tmux("kill-pane", "-t", e.a)
	e.tmux("select-pane", "-t", e.c)
	e.press(t, "\x11")
	waitFor(t, func() bool {
		b, _ := os.ReadFile(e.cOut)
		return strings.Contains(string(b), "^Q")
	})
}

func TestTheFlipKeepsTheZoom(t *testing.T) {
	e := newBesideEnv(t)
	e.tmux("select-pane", "-t", e.a, ";", "resize-pane", "-Z", "-t", e.a)
	e.press(t, "\x11")
	e.waitActive(t, e.c)
	if z := e.tmux("display-message", "-p", "-t", Home+":", "#{window_zoomed_flag}"); z != "1" {
		t.Fatalf("window_zoomed_flag = %s, want 1", z)
	}
}

func TestCtrlQFromListSendsToMatchblox(t *testing.T) {
	e := newBesideEnv(t)
	e.tmux("select-pane", "-t", e.c)
	e.press(t, "\x11")
	waitFor(t, func() bool {
		b, _ := os.ReadFile(e.cOut)
		return strings.Contains(string(b), "^Q")
	})
	if b, _ := os.ReadFile(e.aOut); len(b) != 0 {
		t.Fatalf("C-q was sent into the agent: %q", b)
	}
	if e.active() != e.c {
		t.Fatalf("focus left the list for %s", e.active())
	}
}

func TestTheFlipKeyTypesOutsideTheConsoleWindow(t *testing.T) {
	e := newBesideEnv(t)
	out := t.TempDir() + "/q"
	e.tmux("new-session", "-d", "-s", "kindle-web", "-x", "80", "-y", "20", "sh", "-c", rawCat+out)
	e.waitRaw(t, "=kindle-web:")
	e.tmux("switch-client", "-t", "=kindle-web")
	waitFor(t, func() bool { return e.tmux("list-clients", "-F", "#{client_session}") == "kindle-web" })
	t.Cleanup(func() {
		if t.Failed() {
			b, _ := os.ReadFile(out)
			t.Logf("%s; kindle-web's pane read %q; client in %s; C-q binds %s", e.tmux("-V"), b,
				e.tmux("list-clients", "-F", "#{client_session}"), e.tmux("list-keys", "-T", "root", BesideKey))
		}
	})
	e.press(t, "\x11")
	waitFor(t, func() bool { b, _ := os.ReadFile(out); return string(b) == "^Q" })
}

func TestTheWayBackSelectsTheConsolePane(t *testing.T) {
	e := newBesideEnv(t)
	e.tmux(HomeKeyArgv()[1:]...)
	e.tmux("select-pane", "-t", e.a, ";", "resize-pane", "-Z", "-t", e.a)
	e.press(t, HomeKey)
	e.waitActive(t, e.c)
	if z := e.tmux("display-message", "-p", "-t", Home+":", "#{window_zoomed_flag}"); z != "1" {
		t.Fatalf("window_zoomed_flag = %s, want 1", z)
	}
}

func TestAMarkReachesTheConsoleFromTheAgentPane(t *testing.T) {
	e := newBesideEnv(t)
	e.tmux(MarkKeyArgv()[1:]...)
	e.tmux("select-pane", "-t", e.a)
	e.press(t, keycmd.MarkKey)
	waitFor(t, func() bool {
		b, _ := os.ReadFile(e.cOut)
		return strings.Contains(string(b), "matchblox;from=")
	})
	if b, _ := os.ReadFile(e.aOut); len(b) != 0 {
		t.Fatalf("the agent pane read %q", b)
	}
}

// consoleEnds respawns the console pane with a command that ends once the
// returned stop is called, and adds a chain pane under the agent's.
func (e besideEnv) consoleEnds(t *testing.T) (chain string, stop func()) {
	t.Helper()
	e.tmux("new-session", "-d", "-s", "keep", "sleep", "600") // the server outlives the console's session
	chain = e.tmux("split-window", "-d", "-v", "-t", e.a, "-P", "-F", "#{pane_id}", "sleep", "600")
	e.tmux("set-option", "-w", "-t", e.c, sample.ChainOption, e.a+" "+chain)
	e.tmux(BesideHookArgv()[1:]...)
	flag := t.TempDir() + "/stop"
	e.tmux("respawn-pane", "-k", "-t", e.c, "sh", "-c", "until [ -e "+flag+" ]; do sleep 0.05; done")
	return chain, func() { _ = os.WriteFile(flag, nil, 0o600) }
}

func (e besideEnv) panes() []string {
	return strings.Fields(e.tmux("list-panes", "-a", "-F", "#{pane_id}"))
}

func TestTheAgentPanesEndWithTheConsole(t *testing.T) {
	e := newBesideEnv(t)
	chain, stop := e.consoleEnds(t)
	stop()
	waitFor(t, func() bool {
		p := e.panes()
		return !slices.Contains(p, e.c) && !slices.Contains(p, e.a) && !slices.Contains(p, chain)
	})
	if got := e.tmux("list-sessions", "-F", "#{session_name}"); got != "keep" {
		t.Fatalf("sessions left: %q", got)
	}
	// The hook ran with no client: an error of it would cover the next
	// console, as tmux shows a client the errors kept for it.
	e.tmux("new-session", "-d", "-s", Home, "sleep", "600")
	attach(t, e.sock, Home)
	waitFor(t, func() bool { return strings.Contains(e.tmux("list-clients", "-F", "#{client_session}"), Home) })
	time.Sleep(200 * time.Millisecond)
	if m := e.tmux("display-message", "-p", "-t", Home+":", "#{pane_in_mode}"); m != "0" {
		t.Fatalf("the next console is covered: pane_in_mode = %s", m)
	}
}

func TestARespawnedConsoleKeepsItsAgentPane(t *testing.T) {
	e := newBesideEnv(t)
	chain, stop := e.consoleEnds(t)
	defer stop()
	e.tmux("respawn-pane", "-k", "-t", e.c, "sleep", "600")
	time.Sleep(300 * time.Millisecond)
	if p := e.panes(); !slices.Contains(p, e.a) || !slices.Contains(p, chain) || !slices.Contains(p, e.c) {
		t.Fatalf("panes after a respawn: %q (console %s, agent %s, chain %s)", p, e.c, e.a, chain)
	}
}
