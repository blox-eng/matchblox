//go:build linux

package panes

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

const rawCat = "stty raw -echo; cat -v > "

// besideEnv is a matchblox window with the list pane C, which runs no
// console (sleep), and the agent pane A beside it, with a client attached.
type besideEnv struct {
	tmux       func(args ...string) string
	term       *os.File
	c, a       string
	cOut, aOut string
}

func newBesideEnv(t *testing.T) besideEnv {
	t.Helper()
	sock := tmuxtest.Server(t)
	e := besideEnv{cOut: t.TempDir() + "/c", aOut: t.TempDir() + "/a"}
	e.tmux = func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", sock, "-f", os.DevNull}, args...)...).CombinedOutput() //nolint:gosec // test argv
		if err != nil {
			t.Fatalf("tmux %q: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	e.c = e.tmux("new-session", "-d", "-s", Home, "-x", "100", "-y", "30", "-P", "-F", "#{pane_id}", "sh", "-c", rawCat+e.cOut)
	e.a = e.tmux("split-window", "-d", "-h", "-t", e.c, "-P", "-F", "#{pane_id}", "sh", "-c", rawCat+e.aOut)
	e.tmux("set-option", "-w", "-t", e.c, sample.ConsoleOption, e.c, ";", "set-option", "-w", "-t", e.c, sample.BesideOption, e.a)
	e.tmux(BesideKeysArgv()[1:]...)
	e.term = attach(t, sock, Home)
	waitFor(t, func() bool { return e.tmux("list-clients", "-F", "#{client_session}") == Home })
	return e
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
	e.tmux("select-pane", "-t", e.c)
	e.press(t, "\x11")
	e.waitActive(t, e.a)
	e.press(t, "\x11")
	e.waitActive(t, e.c)
}

func TestCtrlQWithNoLastPaneGoesToTheAgent(t *testing.T) {
	e := newBesideEnv(t)
	e.tmux("kill-pane", "-t", e.a)
	e.a = e.tmux("split-window", "-d", "-h", "-t", e.c, "-P", "-F", "#{pane_id}", "sleep", "600")
	e.tmux("set-option", "-w", "-t", e.c, sample.BesideOption, e.a, ";", "select-pane", "-t", e.c)
	e.press(t, "\x11")
	e.waitActive(t, e.a)
}

func TestTheFlipKeepsTheZoom(t *testing.T) {
	e := newBesideEnv(t)
	e.tmux("select-pane", "-t", e.c, ";", "resize-pane", "-Z", "-t", e.c)
	e.press(t, "\x11")
	e.waitActive(t, e.a)
	if z := e.tmux("display-message", "-p", "-t", Home+":", "#{window_zoomed_flag}"); z != "1" {
		t.Fatalf("window_zoomed_flag = %s, want 1", z)
	}
}

func TestTheFlipKeyTypesOutsideTheConsoleWindow(t *testing.T) {
	e := newBesideEnv(t)
	out := t.TempDir() + "/q"
	e.tmux("new-session", "-d", "-s", "kindle-web", "-x", "80", "-y", "20", "sh", "-c", rawCat+out)
	e.tmux("switch-client", "-t", "=kindle-web")
	waitFor(t, func() bool { return e.tmux("list-clients", "-F", "#{client_session}") == "kindle-web" })
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
