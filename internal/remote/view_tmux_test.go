//go:build unix

package remote

import (
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

// viewEnv is one test server with a console session and agents that run
// cat, so what a view types shows in the agent's own pane.
type viewEnv struct {
	sock, path string
}

func newViewEnv(t *testing.T, agents ...string) viewEnv {
	t.Helper()
	sock := tmuxtest.Server(t)
	mustTmux(t, sock, "-f", "/dev/null", "new-session", "-d", "-s", "console", "-x", "120", "-y", "30", "sleep", "600")
	for _, a := range agents {
		mustTmux(t, sock, "new-session", "-d", "-s", a, "-x", "100", "-y", "20", "cat")
	}
	return viewEnv{sock: sock, path: mustTmux(t, sock, "display", "-p", "#{socket_path}")}
}

func (e viewEnv) pane(t *testing.T, session string) string {
	t.Helper()
	return mustTmux(t, e.sock, "list-panes", "-t", "="+session, "-F", "#{pane_id}")
}

func (e viewEnv) jump(t *testing.T, session string) (Jump, string) {
	t.Helper()
	j := Jump{Pane: e.pane(t, session), Prefix: "C-b", ID: NewJumpID(), Outer: true}
	return j, session
}

func (e viewEnv) argv(t *testing.T, session string) ([]string, Jump) {
	t.Helper()
	j, name := e.jump(t, session)
	if !ViewAllowed(j, name, e.path) {
		t.Fatalf("view of %s not allowed", name)
	}
	return ViewArgv(j, name, e.path), j
}

func (e viewEnv) openView(t *testing.T, argv []string) string {
	t.Helper()
	args := append([]string{"split-window", "-d", "-h", "-t", "=console:", "-P", "-F", "#{pane_id}", "--"}, argv...)
	return mustTmux(t, e.sock, args...)
}

func (e viewEnv) capture(pane string) string {
	out, _ := tmuxOut(e.sock, "capture-pane", "-p", "-t", pane)
	return out
}

func (e viewEnv) jumpSessions() []string {
	out, _ := tmuxOut(e.sock, "list-sessions", "-F", "#{session_name}")
	var names []string
	for _, n := range strings.Fields(out) {
		if strings.HasPrefix(n, "matchblox-jump-") {
			names = append(names, n)
		}
	}
	return names
}

func TestAViewTypesIntoTheAgentsOwnPane(t *testing.T) {
	e := newViewEnv(t, "kindle-web")
	argv, j := e.argv(t, "kindle-web")
	view := e.openView(t, argv)
	g := JumpSessionName(j.ID)
	waitFor(t, "the view's session", func() bool { return hasSession(e.sock, g) })
	// the nested client needs a moment to be ready for keys
	waitFor(t, "the view attached", func() bool {
		out, _ := tmuxOut(e.sock, "list-clients", "-t", "="+g)
		return out != ""
	})
	mustTmux(t, e.sock, "send-keys", "-t", view, "echo typed-in-view", "Enter")
	waitFor(t, "the typed line in the agent's pane", func() bool {
		return strings.Contains(e.capture(e.pane(t, "kindle-web")), "typed-in-view")
	})
	if got := mustTmux(t, e.sock, "display", "-p", "-t", "="+g+":", "#{session_group}"); got != "kindle-web" {
		t.Fatalf("session_group = %q, want kindle-web", got)
	}
}

func TestARespawnEndsTheOldView(t *testing.T) {
	e := newViewEnv(t, "kindle-web", "flint-auth")
	argv1, j1 := e.argv(t, "kindle-web")
	view := e.openView(t, argv1)
	g1 := JumpSessionName(j1.ID)
	waitFor(t, "the first view attached", func() bool {
		out, _ := tmuxOut(e.sock, "list-clients", "-t", "="+g1)
		return out != ""
	})
	argv2, _ := e.argv(t, "flint-auth")
	args := append([]string{"respawn-pane", "-k", "-t", view, "--"}, argv2...)
	mustTmux(t, e.sock, args...)
	waitFor(t, "the old view's session gone", func() bool { return !hasSession(e.sock, g1) })
	if out, _ := tmuxOut(e.sock, "list-clients", "-t", "=kindle-web"); out != "" {
		t.Fatalf("kindle-web has clients: %q", out)
	}
}

func TestAViewEndsWithItsAgent(t *testing.T) {
	e := newViewEnv(t, "flint-auth")
	argv, j := e.argv(t, "flint-auth")
	view := e.openView(t, argv)
	waitFor(t, "the view attached", func() bool {
		out, _ := tmuxOut(e.sock, "list-clients", "-t", "="+JumpSessionName(j.ID))
		return out != ""
	})
	mustTmux(t, e.sock, "send-keys", "-t", "=flint-auth:", "C-d")
	waitFor(t, "the ended line", func() bool {
		return strings.Contains(e.capture(view), "flint-auth ended. ctrl+q: back to the list")
	})
	waitFor(t, "no jump session left", func() bool { return len(e.jumpSessions()) == 0 })
	time.Sleep(200 * time.Millisecond)
	if out, err := tmuxOut(e.sock, "display", "-p", "-t", view, "#{pane_dead}"); err != nil || out != "0" {
		t.Fatalf("the view pane must stay: dead=%q err=%v", out, err)
	}
}

func TestAViewOfAGonePaneSaysSo(t *testing.T) {
	e := newViewEnv(t)
	j := Jump{Pane: "%999", Prefix: "C-b", ID: NewJumpID(), Outer: true}
	if !ViewAllowed(j, "gone-one", e.path) {
		t.Fatal("not allowed")
	}
	view := e.openView(t, ViewArgv(j, "gone-one", e.path))
	waitFor(t, "the failure line", func() bool {
		c := e.capture(view)
		return strings.Contains(c, "gone-one cannot be shown") && strings.Contains(c, "ctrl+q")
	})
	waitFor(t, "no jump session left", func() bool { return len(e.jumpSessions()) == 0 })
}
