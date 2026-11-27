//go:build linux

package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
)

// TestTFromAgentTypesT: with the agent pane's keys, t is text for the
// agent. No root binding takes it (surface: an agent's tools).
func TestTFromAgentTypesT(t *testing.T) {
	e := newDeskEnv(t, "160", "40")
	cwd := t.TempDir()
	agent := e.tmux("new-session", "-d", "-s", "ux-pass", "-c", cwd, "-x", "80", "-y", "24", "-P", "-F", "#{pane_id}", "cat")
	m := e.deskWith(t, agent, cwd)
	m = enter(t, m)
	waitTMUX(t, func() bool {
		out, err := e.run("list-clients", "-F", "#{client_session}")
		return err == nil && strings.Contains(out, "matchblox-jump-")
	})
	sendT(t, e, m.beside)
	waitAgentText(t, e, agent, "t")
	if line := rootBind(e, "t"); line != "" {
		t.Fatalf("root binds t: %s", line)
	}

	// The shell bind must not steal a later t. Open the shell, flip back
	// to the agent, and type t again.
	m = pressShell(t, m)
	if e.findShell(agent) == "" {
		t.Fatal("shell did not open under the agent, so a later t could not be checked")
	}
	sendT(t, e, m.beside)
	waitTMUX(t, func() bool { return e.paneField("=ux-pass:", "#{pane_id}") == agent })
	sendT(t, e, m.beside)
	got := waitAgentText(t, e, agent, "tt")
	if strings.Count(got, "t") < 2 {
		t.Fatalf("agent capture %q, want t twice", got)
	}
	if line := rootBind(e, "t"); line != "" {
		t.Fatalf("root binds t after the shell: %s", line)
	}
}

func (e deskEnv) deskWith(t *testing.T, agent, cwd string) Model {
	t.Helper()
	m := e.start(t)
	st := boardFixture(t)
	st.Sessions = []sample.Session{{
		Name: "ux-pass", Pane: agent, Cwd: cwd, Agent: "claude", Status: "idle",
		SessionID: "sid-ux", Target: "ux-pass:1.0",
	}}
	st.Queue, st.Ships = nil, nil
	next, look := m.Update(stateMsg(st))
	m = feed(t, next.(Model), look) // the look for shells the list's refresh starts
	m.opt.Socket = e.tmux("display", "-p", "#{socket_path}")
	m.toList()
	m.width, m.height = 120, 40
	return chooseNamed(t, m, "ux-pass")
}

func pressShell(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := key(m, "t")
	return feed(t, next.(Model), cmd)
}

func sendT(t *testing.T, e deskEnv, pane string) {
	t.Helper()
	if _, err := e.run("send-keys", "-t", pane, "t"); err != nil {
		t.Fatal(err)
	}
}

func waitAgentText(t *testing.T, e deskEnv, agent, want string) string {
	t.Helper()
	var capAgent string
	waitTMUX(t, func() bool {
		out, err := e.run("capture-pane", "-p", "-t", agent)
		if err != nil {
			return false
		}
		capAgent = out
		return strings.Contains(capAgent, want)
	})
	return capAgent
}

func (e deskEnv) paneField(pane, format string) string {
	out, err := e.run("display", "-p", "-t", pane, format)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (e deskEnv) findShell(agent string) string {
	out, err := e.run("list-panes", "-t", agent, "-F", "#{pane_id} #{@matchblox_shell}")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		id, mark, _ := strings.Cut(strings.TrimSpace(line), " ")
		if id != "" && id != agent && mark != "" {
			return id
		}
	}
	return ""
}

func (e deskEnv) panePath(pane string) string {
	return e.paneField(pane, "#{pane_current_path}")
}

func shellDivider(cwd string) string { return "shell · " + cwd + " · t switches" }

// viewOf enters ux-pass so the desk's agent pane is the nested client.
func (e deskEnv) viewOf(t *testing.T, m Model) Model {
	t.Helper()
	m = enter(t, m)
	waitTMUX(t, func() bool {
		out, err := e.run("list-clients", "-F", "#{client_session}")
		return err == nil && strings.Contains(out, "matchblox-jump-")
	})
	return m
}

func uxPass(t *testing.T) (deskEnv, Model, string, string) {
	t.Helper()
	e := newDeskEnv(t, "160", "40")
	cwd, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	agent := e.tmux("new-session", "-d", "-s", "ux-pass", "-c", cwd, "-x", "80", "-y", "24", "-P", "-F", "#{pane_id}", "cat")
	return e, e.deskWith(t, agent, cwd), agent, cwd
}

func TestTOpensShellInAgentWindow(t *testing.T) {
	e, m, agent, cwd := uxPass(t)
	m = e.viewOf(t, m)
	m = pressShell(t, m)
	shell := e.findShell(agent)
	if shell == "" {
		t.Fatal("no shell pane in ux-pass")
	}
	if got := e.panePath(shell); got != cwd {
		t.Fatalf("shell cwd %q, want %q", got, cwd)
	}
	want := shellDivider(cwd)
	var capDesk string
	waitTMUX(t, func() bool {
		out, err := e.run("capture-pane", "-p", "-t", m.beside)
		if err != nil {
			return false
		}
		capDesk = out
		return strings.Contains(capDesk, want)
	})
	if !strings.Contains(capDesk, want) {
		t.Fatalf("desk agent capture missing %q:\n%s", want, capDesk)
	}
}

func TestTSecondPressFocusesShell(t *testing.T) {
	e, m, agent, _ := uxPass(t)
	m = pressShell(t, m)
	shell := e.findShell(agent)
	if shell == "" {
		t.Fatal("no shell pane")
	}
	n := len(strings.Split(e.tmux("list-panes", "-t", "=ux-pass", "-F", "#{pane_id}"), "\n"))
	m = pressShell(t, m)
	got := strings.Split(e.tmux("list-panes", "-t", "=ux-pass", "-F", "#{pane_id}"), "\n")
	if len(got) != n {
		t.Fatalf("second t made %d panes, want %d", len(got), n)
	}
	if cur := e.paneField("=ux-pass:", "#{pane_id}"); cur != shell {
		t.Fatalf("active pane %q, want the shell %q", cur, shell)
	}
}

func TestTSurvivesQuit(t *testing.T) {
	e, m, agent, cwd := uxPass(t)
	m = pressShell(t, m)
	shell := e.findShell(agent)
	if shell == "" {
		t.Fatal("no shell pane")
	}
	e.tmux("kill-session", "-t", "=matchblox")
	e.console = e.tmux("-f", "/dev/null", "new-session", "-d", "-s", "matchblox", "-x", "160", "-y", "40", "-P", "-F", "#{pane_id}", "sleep", "600")
	m = e.deskWith(t, agent, cwd)
	if !e.live(shell) {
		t.Fatal("the shell pane died when matchblox quit")
	}
	line := sessionRow(m, "ux-pass")
	if !strings.Contains(line, m.st.muted.Render("shell")) {
		t.Fatalf("row after restart has no shell: %q", ansi.Strip(line))
	}
	before := e.tmux("list-panes", "-t", "=ux-pass", "-F", "#{pane_id}")
	m = pressShell(t, m)
	after := e.tmux("list-panes", "-t", "=ux-pass", "-F", "#{pane_id}")
	if after != before {
		t.Fatalf("t after restart split again:\n%s", after)
	}
	if cur := e.paneField("=ux-pass:", "#{pane_id}"); cur != shell {
		t.Fatalf("t after restart left %q active, want the shell %q", cur, shell)
	}
}

func TestTFocusLeavesTheAgentSession(t *testing.T) {
	e, m, agent, cwd := uxPass(t)
	e.tmux("new-window", "-t", "=ux-pass", "-n", "other")
	other := e.paneField("=ux-pass:", "#{window_id}")
	m = pressShell(t, m)
	if got := e.paneField("=ux-pass:", "#{window_id}"); got != other {
		t.Fatalf("t moved ux-pass to %s, want %s", got, other)
	}
	if e.findShell(agent) == "" {
		t.Fatal("no shell under the agent")
	}
	m = e.viewOf(t, m)
	want := shellDivider(cwd)
	out, err := e.run("capture-pane", "-p", "-t", m.beside)
	if err != nil || !strings.Contains(out, want) {
		t.Fatalf("desk capture missing %q:\n%s", want, out)
	}
}

func TestTFromShellFlipsToAgent(t *testing.T) {
	e, m, agent, _ := uxPass(t)
	m = e.viewOf(t, m)
	m = pressShell(t, m)
	shell := e.findShell(agent)
	if shell == "" {
		t.Fatal("no shell pane")
	}
	if cur := e.paneField("=ux-pass:", "#{pane_id}"); cur != shell {
		t.Fatalf("shell is not active: %q", cur)
	}
	sendT(t, e, m.beside)
	waitTMUX(t, func() bool { return e.paneField("=ux-pass:", "#{pane_id}") == agent })
	if line := rootBind(e, "t"); line != "" {
		t.Fatalf("root binds t: %s", line)
	}
	binds, _ := e.run("list-keys", "-T", "matchblox-jump-C-b")
	if !strings.Contains(binds, "select-pane") || !strings.Contains(binds, "@matchblox_mode") {
		t.Fatalf("shell t does not select the agent and write AGENT:\n%s", binds)
	}
	if mark := e.paneField(agent, "#{@matchblox_shell}"); mark != "" {
		t.Fatalf("the agent pane is marked as the shell: %q", mark)
	}
}

func TestTRowSaysShell(t *testing.T) {
	e, m, agent, _ := uxPass(t)
	m = pressShell(t, m)
	if e.findShell(agent) == "" {
		t.Fatal("no shell pane")
	}
	line := sessionRow(m, "ux-pass")
	plain := ansi.Strip(line)
	word, mark := strings.Index(plain, "shell"), strings.Index(plain, "✻")
	if word < 0 || mark < 0 || word > mark {
		t.Fatalf("row %q, want shell before ✻", plain)
	}
	if strings.Contains(plain, "▣") {
		t.Fatalf("row uses ▣ for the shell: %q", plain)
	}
	if !strings.Contains(line, m.st.muted.Render("shell")) {
		t.Fatalf("shell is not muted: %q", line)
	}
}

func TestTEndsWhenTheShellExits(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh") // the machine zsh runs its new-user menu and eats the first e of exit
	e, m, agent, cwd := uxPass(t)
	m = e.viewOf(t, m)
	m = pressShell(t, m)
	shell := e.findShell(agent)
	if shell == "" {
		t.Fatal("no shell pane")
	}
	if _, err := e.run("send-keys", "-t", shell, "exit", "Enter"); err != nil {
		t.Fatal(err)
	}
	waitTMUX(t, func() bool { return !e.live(shell) })
	plain := ansi.Strip(sessionRow(m, "ux-pass"))
	if strings.Contains(plain, "shell") {
		t.Fatalf("row still says shell: %q", plain)
	}
	want := shellDivider(cwd)
	var capDesk string
	waitTMUX(t, func() bool {
		out, err := e.run("capture-pane", "-p", "-t", m.beside)
		if err != nil {
			return false
		}
		capDesk = out
		return !strings.Contains(capDesk, want)
	})
	if strings.Contains(capDesk, want) {
		t.Fatalf("divider stayed:\n%s", capDesk)
	}
}

func sessionRow(m Model, name string) string {
	for _, l := range strings.Split(m.render(), "\n") {
		if strings.Contains(ansi.Strip(l), name) {
			return l
		}
	}
	return ""
}

func rootBind(e deskEnv, key string) string {
	out, err := e.run("list-keys", "-T", "root")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == "bind-key" && fields[2] == "root" && fields[3] == key {
			return line
		}
	}
	return ""
}
