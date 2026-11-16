//go:build unix

package remote

import (
	"os"
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

// explain logs, when the test fails, what the view showed and which
// sessions were left: a timeout alone names no cause.
func (e viewEnv) explain(t *testing.T, view string) {
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		v, _ := tmuxOut(e.sock, "-V")
		ls, _ := tmuxOut(e.sock, "list-sessions", "-F", "#{session_name} group=#{session_group} attached=#{session_attached} windows=#{session_windows}")
		cl, _ := tmuxOut(e.sock, "list-clients", "-F", "#{client_tty} #{client_session}")
		t.Logf("%s\nsessions:\n%s\nclients:\n%s\nview %s shows:\n%s", v, ls, cl, view, e.capture(view))
	})
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
		// list-clients prints "can't find session" before the view's session exists
		return len(clientsOf(e.sock, g)) > 0
	})
	mustTmux(t, e.sock, "send-keys", "-t", view, "echo typed-in-view", "Enter")
	waitFor(t, "the typed line in the agent's pane", func() bool {
		return strings.Contains(e.capture(e.pane(t, "kindle-web")), "typed-in-view")
	})
	if got := mustTmux(t, e.sock, "display", "-p", "-t", "="+g+":", "#{session_group}"); got != "kindle-web" {
		t.Fatalf("session_group = %q, want kindle-web", got)
	}
}

// A peer's windows keep the size of the terminal that used them last, 240
// columns from a laptop. A drawn copy cut that at the column's edge (owner,
// 2026-11-16: "going behind the screen", on the phone too). A view is a
// client, so the agent's window takes the view's size.
func TestAViewFitsAWiderAgentToItsPane(t *testing.T) {
	e := newViewEnv(t)
	mustTmux(t, e.sock, "new-session", "-d", "-s", "kindle-web", "-x", "240", "-y", "75", "cat")
	argv, j := e.argv(t, "kindle-web")
	view := e.openView(t, argv)
	waitFor(t, "the view attached", func() bool {
		// list-clients prints "can't find session" before the view's session exists
		return len(clientsOf(e.sock, JumpSessionName(j.ID))) > 0
	})
	want := mustTmux(t, e.sock, "display", "-p", "-t", view, "#{pane_width}x#{pane_height}")
	waitFor(t, "kindle-web's window at the view's size "+want, func() bool {
		got, _ := tmuxOut(e.sock, "display", "-p", "-t", e.pane(t, "kindle-web"), "#{window_width}x#{window_height}")
		return strings.TrimSpace(got) == want
	})
}

func TestARespawnEndsTheOldView(t *testing.T) {
	e := newViewEnv(t, "kindle-web", "flint-auth")
	tmp := t.TempDir()
	mustTmux(t, e.sock, "set-environment", "-g", "TMPDIR", tmp)
	argv1, j1 := e.argv(t, "kindle-web")
	view := e.openView(t, argv1)
	g1 := JumpSessionName(j1.ID)
	waitFor(t, "the first view attached", func() bool {
		// list-clients prints "can't find session" before the view's session exists
		return len(clientsOf(e.sock, g1)) > 0
	})
	argv2, _ := e.argv(t, "flint-auth")
	args := append([]string{"respawn-pane", "-k", "-t", view, "--"}, argv2...)
	mustTmux(t, e.sock, args...)
	waitFor(t, "the old view's session gone", func() bool { return !hasSession(e.sock, g1) })
	if !hasSession(e.sock, "kindle-web") {
		t.Fatal("kindle-web ended with its view")
	}
	if cs := clientsOf(e.sock, "kindle-web"); len(cs) > 0 {
		t.Fatalf("kindle-web has clients: %q", cs)
	}
	// Only the new view's file is left: the old wrapper was killed mid-client.
	waitFor(t, "the old view's temp file removed", func() bool {
		files, _ := os.ReadDir(tmp)
		return len(files) == 1
	})
}

func TestAViewEndsWithItsAgent(t *testing.T) {
	e := newViewEnv(t, "flint-auth")
	argv, j := e.argv(t, "flint-auth")
	view := e.openView(t, argv)
	e.explain(t, view)
	waitFor(t, "the view attached", func() bool {
		// list-clients prints "can't find session" before the view's session exists
		return len(clientsOf(e.sock, JumpSessionName(j.ID))) > 0
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

func TestAPeerViewThatFailsSaysSoAndStays(t *testing.T) {
	e := newViewEnv(t)
	ssh := script(t, t.TempDir()+"/ssh", "#!/bin/sh\necho 'ssh: connect to host ws-1 port 22: Connection refused' >&2\nexit 255\n")
	view := e.openView(t, PeerViewArgv("kindle-web", []string{ssh, "-t", "--", "ws-1", "true"}))
	joined := func() string { out, _ := tmuxOut(e.sock, "capture-pane", "-p", "-J", "-t", view); return out }
	waitFor(t, "the failure line", func() bool {
		return strings.Contains(joined(), "kindle-web cannot be shown: ssh exited 255. ctrl+q: back to the list")
	})
	if !strings.Contains(joined(), "Connection refused") {
		t.Fatalf("ssh's own words are gone:\n%s", joined())
	}
	time.Sleep(200 * time.Millisecond)
	if out, err := tmuxOut(e.sock, "display", "-p", "-t", view, "#{pane_dead}"); err != nil || out != "0" {
		t.Fatalf("the view pane must stay: dead=%q err=%v", out, err)
	}
}

func TestAPeerViewThatEndsSaysSoAndStays(t *testing.T) {
	e := newViewEnv(t)
	ssh := script(t, t.TempDir()+"/ssh", "#!/bin/sh\nexit 0\n")
	view := e.openView(t, PeerViewArgv("kindle-web", []string{ssh, "-t", "--", "ws-1", "true"}))
	waitFor(t, "the ended line", func() bool {
		return strings.Contains(e.capture(view), "kindle-web ended. ctrl+q: back to the list")
	})
	time.Sleep(200 * time.Millisecond)
	if out, err := tmuxOut(e.sock, "display", "-p", "-t", view, "#{pane_dead}"); err != nil || out != "0" {
		t.Fatalf("the view pane must stay: dead=%q err=%v", out, err)
	}
}

// twoWindowAgent is the session work: the agent's window, with a second
// pane there when shared, and a window "other" that says OTHER-WINDOW. It
// answers the agent's pane.
func (e viewEnv) twoWindowAgent(t *testing.T, shared bool) string {
	t.Helper()
	agent := mustTmux(t, e.sock, "new-session", "-d", "-s", "work", "-n", "agent", "-x", "100", "-y", "20", "-P", "-F", "#{pane_id}", "cat")
	if shared {
		mustTmux(t, e.sock, "split-window", "-d", "-t", agent, "sh", "-c", "echo SHARED-PANE; exec sleep 600")
	}
	mustTmux(t, e.sock, "new-window", "-d", "-t", "=work:", "-n", "other", "sh", "-c", "echo OTHER-WINDOW; exec sleep 600")
	return agent
}

func TestAViewEndsWhenItsAgentPaneGoes(t *testing.T) {
	for _, shared := range []bool{false, true} {
		e := newViewEnv(t)
		mustTmux(t, e.sock, ViewHookArgv()[1:]...)
		agent := e.twoWindowAgent(t, shared)
		j := Jump{Pane: agent, Prefix: "C-b", ID: NewJumpID(), Outer: true}
		view := e.openView(t, ViewArgv(j, "work", e.path))
		e.explain(t, view)
		g := JumpSessionName(j.ID)
		waitFor(t, "the view attached", func() bool {
			// list-clients prints "can't find session" before the view's session exists
			return len(clientsOf(e.sock, g)) > 0
		})
		mustTmux(t, e.sock, "kill-pane", "-t", agent)
		waitFor(t, "the ended line", func() bool {
			return strings.Contains(e.capture(view), "work ended. ctrl+q: back to the list")
		})
		if c := e.capture(view); strings.Contains(c, "OTHER-WINDOW") || strings.Contains(c, "SHARED-PANE") {
			t.Fatalf("shared=%v: the view showed another pane:\n%s", shared, c)
		}
		waitFor(t, "no jump session left", func() bool { return len(e.jumpSessions()) == 0 })
		if !hasSession(e.sock, "work") {
			t.Fatalf("shared=%v: the agent's session ended with its view", shared)
		}
	}
}

// TestAHostileNameStaysWords: a view's name is an argument of its script,
// never script: quotes, $(…) and % print as they are.
func TestAHostileNameStaysWords(t *testing.T) {
	e := newViewEnv(t, "flint-auth")
	j, _ := e.jump(t, "flint-auth")
	made := t.TempDir() + "/made"
	name := `it's "$(touch ` + made + `)" 100% $HOME`
	if !ViewAllowed(j, name, e.path) {
		t.Fatalf("%q refused", name)
	}
	view := e.openView(t, ViewArgv(j, name, e.path))
	waitFor(t, "the view attached", func() bool {
		// list-clients prints "can't find session" before the view's session exists
		return len(clientsOf(e.sock, JumpSessionName(j.ID))) > 0
	})
	mustTmux(t, e.sock, "kill-pane", "-t", j.Pane)
	waitFor(t, "the ended line", func() bool {
		out, _ := tmuxOut(e.sock, "capture-pane", "-p", "-J", "-t", view)
		return strings.Contains(out, name+" ended.")
	})
	if _, err := os.Stat(made); err == nil {
		t.Fatal("the name ran as a command")
	}
}
