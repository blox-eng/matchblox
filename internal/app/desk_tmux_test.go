//go:build linux

package app

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

// TestUpgradeLeavesTheAgentSession: an upgraded desk must kill only the
// old two-pane viewer, never the agent's own session (surface: an agent's
// tools).
func TestUpgradeLeavesTheAgentSession(t *testing.T) {
	e := newDeskEnv(t, "160", "40")
	agent := e.tmux("new-session", "-d", "-s", "ux-pass", "-x", "80", "-y", "24", "-P", "-F", "#{pane_id}", "sleep", "600")
	viewer := e.tmux("split-window", "-d", "-h", "-t", e.console, "-P", "-F", "#{pane_id}", "sleep", "600")
	e.tmux("set-option", "-w", "-t", e.console, sample.ConsoleOption, e.console,
		";", "set-option", "-w", "-t", e.console, sample.BesideOption, viewer)

	m := e.start(t)
	if !e.hasSession("ux-pass") {
		t.Fatal("ux-pass was killed")
	}
	if !e.live(agent) {
		t.Fatal("the agent pane of ux-pass was killed")
	}
	if e.live(viewer) {
		t.Fatal("the old viewer pane was kept")
	}
	if said := deskSaid(m); !strings.Contains(said, deskFrameNotice) {
		t.Fatalf("list foot %q, want %q", said, deskFrameNotice)
	}
	if !m.deskFrame {
		t.Fatal("Doc.DeskFrame was not set")
	}

	m.noticeText, m.flash = "", ""
	m = feed(t, m, m.besideCmd())
	if said := deskSaid(m); strings.Contains(said, deskFrameNotice) {
		t.Fatalf("a second start repeated the notice: %q", said)
	}
	if !e.hasSession("ux-pass") {
		t.Fatal("ux-pass was killed on the second start")
	}
}

func TestDeskFrameOpensFourPanes(t *testing.T) {
	e := newDeskEnv(t, "160", "40")
	agent := e.tmux("new-session", "-d", "-s", "ux-pass", "-x", "80", "-y", "24", "-P", "-F", "#{pane_id}", "sleep", "600")
	m := e.start(t)
	starts := e.startCommands()
	if len(starts) != 4 {
		t.Fatalf("panes = %d %q, want 4", len(starts), starts)
	}
	if !hasStart(starts, "frame --kind=header") || !hasStart(starts, "frame --kind=bar") {
		t.Fatalf("header and bar do not run matchblox frame: %q", starts)
	}
	if !hasStart(starts, "sleep 600") {
		t.Fatalf("the list is not the console: %q", starts)
	}
	if header := e.option("@matchblox_header"); header == "" {
		t.Fatal("no @matchblox_header")
	}
	if bar := e.option("@matchblox_bar"); bar == "" {
		t.Fatal("no @matchblox_bar")
	}

	st := boardFixture(t)
	st.Sessions[0].Name, st.Sessions[0].Pane, st.Sessions[0].Host = "ux-pass", agent, ""
	next, _ := m.Update(stateMsg(st))
	m = next.(Model)
	m.opt.Socket = e.tmux("display", "-p", "#{socket_path}")
	m = chooseNamed(t, m, "ux-pass")
	m = enter(t, m)
	r := e.agentStart(m.beside)
	if strings.Contains(r, "frame") || !strings.Contains(r, "env") {
		t.Fatalf("the agent pane is not ViewArgv: %q", r)
	}
}

func TestDeskFrameStaysTwoPanesUnder100(t *testing.T) {
	e := newDeskEnv(t, "90", "40")
	_ = e.start(t)
	starts := e.startCommands()
	if len(starts) != 2 {
		t.Fatalf("panes = %d %q, want 2", len(starts), starts)
	}
	if hasStart(starts, "frame --kind=header") || hasStart(starts, "frame --kind=bar") {
		t.Fatalf("header or bar opened under 100 columns: %q", starts)
	}
	if e.option("@matchblox_header") != "" || e.option("@matchblox_bar") != "" {
		t.Fatalf("header/bar options set under 100 columns")
	}
}

func TestDeskFrameHeaderGoneSaysSo(t *testing.T) {
	tm := &besideTmux{winW: "160", winH: "40"}
	opt := Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }, Beside: true, Pane: "%1", Tmux: func(args ...string) (string, error) {
		if args[0] == "split-window" && slices.Contains(args, "-b") {
			return "", errors.New("no space for a pane")
		}
		return tm.run(args...)
	}, Self: "/bin/true"}
	m := New(opt)
	m.splashDone = true
	m = feed(t, m, m.besideCmd())
	said := deskSaid(m)
	if !strings.Contains(said, "the desk frame did not open:") || !strings.Contains(said, "no space for a pane") {
		t.Fatalf("said %q", said)
	}
}

type deskEnv struct {
	sock, console, self string
	tmux                func(args ...string) string
}

func newDeskEnv(t *testing.T, w, h string) deskEnv {
	t.Helper()
	sock := tmuxtest.Server(t)
	e := deskEnv{sock: sock, self: frameStub(t)}
	e.tmux = func(args ...string) string {
		t.Helper()
		cmd := exec.Command("tmux", append([]string{"-L", sock}, args...)...) //nolint:gosec // a test's own server
		cmd.Env = noTmuxEnv()
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %q: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	e.console = e.tmux("-f", "/dev/null", "new-session", "-d", "-s", "matchblox", "-x", w, "-y", h, "-P", "-F", "#{pane_id}", "sleep", "600")
	return e
}

func (e deskEnv) start(t *testing.T) Model {
	t.Helper()
	sock := e.tmux("display", "-p", "#{socket_path}")
	opt := Options{
		NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt },
		Beside: true, Pane: e.console, Tmux: e.run, Self: e.self, Socket: sock, HomeSession: "matchblox",
	}
	m := New(opt)
	m.splashDone = true
	m.viewPrefs.CopyCheckedPhone, m.viewPrefs.CopyCheckedDesk = true, true
	return feed(t, m, m.besideCmd())
}

func (e deskEnv) run(args ...string) (string, error) {
	cmd := exec.Command("tmux", append([]string{"-L", e.sock}, args...)...) //nolint:gosec // a test's own server
	cmd.Env = noTmuxEnv()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (e deskEnv) live(pane string) bool {
	got, err := e.run("display", "-p", "-t", pane, "#{pane_id}")
	return err == nil && strings.TrimSpace(got) == pane
}

func (e deskEnv) hasSession(name string) bool {
	_, err := e.run("has-session", "-t", "="+name)
	return err == nil
}

func deskSaid(m Model) string {
	if s := m.footSays(); s != "" {
		return s
	}
	if m.noticeText != "" {
		return m.noticeText
	}
	return m.flash
}

func (e deskEnv) startCommands() []string {
	out := e.tmux("list-panes", "-t", e.console, "-F", "#{pane_start_command}")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func (e deskEnv) option(name string) string {
	out, _ := e.run("show-option", "-wqv", "-t", e.console, name)
	return strings.TrimSpace(out)
}

func (e deskEnv) agentStart(pane string) string {
	out, _ := e.run("display", "-p", "-t", pane, "#{pane_start_command}")
	return strings.TrimSpace(out)
}

func hasStart(starts []string, want string) bool {
	for _, s := range starts {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}

func frameStub(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "matchblox")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec sleep 600\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
