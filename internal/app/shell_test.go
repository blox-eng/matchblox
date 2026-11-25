//go:build linux

package app

import (
	"fmt"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
)

// shellDesk is a desk on a real tmux server with ux-pass in cwd, ready
// for t to split under the agent.
func shellDesk(t *testing.T, cwd string) (agentDesk, *regionRun, sample.Session) {
	t.Helper()
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	d := newAgentDesk(t, 80, 24, map[string]string{
		"ux-pass": "printf 'UX READY\\n'; exec sleep 600",
	})
	d.tmux("new-session", "-d", "-s", "matchblox", "-x", "160", "-y", "40", "sleep", "600")
	console := d.pane("matchblox")
	run, _ := deskTmux(d)
	ux := doneMatch("ux-pass", 10*time.Minute)
	ux.Pane, ux.Target, ux.Cwd = d.pane("ux-pass"), "ux-pass:0.0", cwd
	m := New(Options{
		NoMotion: true, Conn: newFake(), Pane: console, Socket: d.path,
		Tmux: run, Beside: true, Now: func() time.Time { return boardAt },
	})
	x := &regionRun{t: t, m: &m, r: &m.agent, msgs: make(chan tea.Msg, 64)}
	t.Cleanup(x.r.shut)
	x.update(tea.WindowSizeMsg{Width: 160, Height: 40})
	x.update(stateMsg(boardState(ux)))
	x.m.splashDone = true
	x.m.toList()
	x.press("enter") // open the agent region on ux-pass
	for end := time.Now().Add(10 * time.Second); !x.shows("UX READY")(); {
		select {
		case msg := <-x.msgs:
			switch msg.(type) {
			case shownMsg, agentFrameMsg:
				x.update(msg)
			}
		case <-time.After(20 * time.Millisecond):
		}
		if time.Now().After(end) {
			t.Fatalf("⏎ showed no agent; the desk reads:\n%s", plainFrame(*x.m))
		}
	}
	x.m.agent.keys = false // list has the keys for t
	return d, x, ux
}

// shellPanes are panes in the agent's session that carry @matchblox-shell.
func shellPanes(d agentDesk, session string) string {
	d.t.Helper()
	return d.tmux("list-panes", "-s", "-t", "="+session,
		"-F", "#{pane_id} #{@matchblox-shell} #{pane_current_path}")
}

// pressT is t on the list, with any cmd it returns settled.
func pressT(x *regionRun) {
	x.t.Helper()
	next, cmd := x.m.Update(keyPress("t"))
	*x.m = next.(Model)
	for _, msg := range cmdMsgs(cmd) {
		x.update(msg)
	}
}

// TestTOpensAShellInTheSessionsFolder: t on ux-pass splits its own window
// under the agent; pwd in the new pane prints the session's Cwd.
func TestTOpensAShellInTheSessionsFolder(t *testing.T) {
	cwd := t.TempDir()
	d, x, ux := shellDesk(t, cwd)
	pressT(x)
	panes := shellPanes(d, "ux-pass")
	if !strings.Contains(panes, "ux-pass") || !strings.Contains(panes, cwd) {
		t.Fatalf("after t the agent's panes are %q, want a shell for ux-pass in %s", panes, cwd)
	}
	lines := strings.Split(strings.TrimSpace(panes), "\n")
	var shellID string
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) >= 2 && f[1] == "ux-pass" {
			shellID = f[0]
			break
		}
	}
	if shellID == "" {
		t.Fatalf("no @matchblox-shell=ux-pass pane:\n%s", panes)
	}
	if got := d.tmux("list-panes", "-t", ux.Pane, "-F", "#{pane_id}"); !strings.Contains(got, shellID) {
		t.Fatalf("the shell %s is not in ux-pass's window (agent %s):\n%s", shellID, ux.Pane, got)
	}
	d.tmux("send-keys", "-t", shellID, "pwd", "Enter")
	deadline := time.Now().Add(5 * time.Second)
	for {
		out := d.tmux("capture-pane", "-p", "-t", shellID)
		if strings.Contains(out, cwd) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("pwd in the shell did not print %s:\n%s", cwd, out)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestTSwitchesTheKeys: a second t selects the shell (mode SHELL); in the
// shell, t types t; ctrl+q gives the keys back to the list.
func TestTSwitchesTheKeys(t *testing.T) {
	cwd := t.TempDir()
	d, x, _ := shellDesk(t, cwd)
	pressT(x)
	pressT(x)
	if mode := modeOf(*x.m); mode != "SHELL" {
		t.Fatalf("second t: mode %s, want SHELL", mode)
	}
	shellID := ""
	for _, line := range strings.Split(strings.TrimSpace(shellPanes(d, "ux-pass")), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[1] == "ux-pass" {
			shellID = f[0]
			break
		}
	}
	if shellID == "" {
		t.Fatal("no shell pane after the second t")
	}
	active := d.tmux("display", "-p", "-t", "="+"ux-pass:", "#{pane_id}")
	if active != shellID {
		t.Fatalf("active pane is %s, want the shell %s", active, shellID)
	}
	before := d.tmux("capture-pane", "-p", "-t", shellID)
	x.press("t")
	deadline := time.Now().Add(5 * time.Second)
	for {
		out := d.tmux("capture-pane", "-p", "-t", shellID)
		if out != before && strings.Contains(out, "t") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("t in SHELL did not type into the shell (mode %s):\n%s", modeOf(*x.m), out)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if mode := modeOf(*x.m); mode != "SHELL" {
		t.Fatalf("after typing t the mode is %s, want SHELL", mode)
	}
	x.press(flipKey)
	if mode := modeOf(*x.m); mode != "LIST" {
		t.Fatalf("ctrl+q: mode %s, want LIST", mode)
	}
}

// TestTheShellOutlivesTheConsole: quit and start again; the pane stays,
// and the row shows ▣.
func TestTheShellOutlivesTheConsole(t *testing.T) {
	cwd := t.TempDir()
	d, x, ux := shellDesk(t, cwd)
	pressT(x)
	before := shellPanes(d, "ux-pass")
	if !strings.Contains(before, "ux-pass") {
		t.Fatalf("no shell before quit:\n%s", before)
	}
	x.r.shut()
	run, _ := deskTmux(d)
	m := New(Options{
		NoMotion: true, Conn: newFake(), Pane: d.pane("matchblox"), Socket: d.path,
		Tmux: run, Beside: true, Now: func() time.Time { return boardAt },
	})
	m = upd(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = upd(t, m, stateMsg(boardState(ux)))
	m.splashDone = true
	m.toList()
	after := shellPanes(d, "ux-pass")
	if !strings.Contains(after, "ux-pass") {
		t.Fatalf("after a new console the shell is gone:\n%s", after)
	}
	row := listRowOf(m, "ux-pass")
	if !strings.Contains(ansi.Strip(row), "▣") {
		t.Fatalf("the row has no ▣ after restart:\n%s\nframe:\n%s", row, plainFrame(m))
	}
}

// TestTRefusesWithWords: a host not reached, and a session with no Cwd,
// each say why.
func TestTRefusesWithWords(t *testing.T) {
	t.Run("not reached", func(t *testing.T) {
		now := boardAt
		m, _, conns := hostsAt(t, &now, "ws-1", 160, 40, "gpu-1")
		gpu := doneMatch("ux-pass", 10*time.Minute)
		gpu.Host, gpu.Cwd = "gpu-1", "/tmp/ux-pass"
		m = upd(t, m, stateMsg(boardState()))
		m = upd(t, m, peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: stateMsg(boardState(gpu))})
		m = upd(t, m, peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: lostMsg{err: io.EOF, conn: conns["gpu-1"]}})
		m.opt.Beside = true
		m.toList()
		m = pressOn(m, "t")
		if got := m.sideSays(); got != "not opened: gpu-1 is not reached" {
			t.Fatalf("t on a host not reached says %q", got)
		}
	})
	t.Run("no folder", func(t *testing.T) {
		ux := doneMatch("ux-pass", 10*time.Minute)
		ux.Cwd = ""
		m := boardModel(t, boardState(ux))
		m.opt.Beside = true
		m.opt.Pane = "%1"
		m.opt.Tmux = func(args ...string) (string, error) {
			t.Fatalf("tmux ran on a session with no folder: %v", args)
			return "", nil
		}
		m.toList()
		m = pressOn(m, "t")
		if got := m.sideSays(); got != "not opened: ux-pass has no folder" {
			t.Fatalf("t with no Cwd says %q", got)
		}
	})
}

// TestTheShellLineIsNamed: the agent's window shows the divider in faint,
// and the active border in accent. tmux draws it, so the region (and ⌃o)
// show the same line.
func TestTheShellLineIsNamed(t *testing.T) {
	cwd := t.TempDir()
	d, x, ux := shellDesk(t, cwd)
	pressT(x)
	pressT(x) // shell has the keys: its border is accent
	const want = "shell · ux-pass · t switches"
	// The nested client redraws the border; pump its frames.
	for end := time.Now().Add(10 * time.Second); !x.shows(want)(); {
		select {
		case msg := <-x.msgs:
			if _, ok := msg.(agentFrameMsg); ok {
				x.update(msg)
			}
		case <-time.After(20 * time.Millisecond):
		}
		if time.Now().After(end) {
			t.Fatalf("the agent's window has no %q; the region shows:\n%s", want,
				strings.Join(plainLines(x.r.lines(newStyles(true), x.r.w, x.r.h)), "\n"))
		}
	}
	faint := colorHex(x.m.st.theme.Faint)
	accent := colorHex(x.m.st.theme.Accent)
	format := d.tmux("show-options", "-wv", "-t", ux.Pane, "pane-border-format")
	if !strings.Contains(format, want) || !strings.Contains(strings.ToLower(format), strings.ToLower(faint)) {
		t.Fatalf("pane-border-format is %q, want %q in faint %s", format, want, faint)
	}
	style := d.tmux("show-options", "-wv", "-t", ux.Pane, "pane-active-border-style")
	if !strings.Contains(strings.ToLower(style), strings.ToLower(accent)) {
		t.Fatalf("pane-active-border-style is %q, want accent %s", style, accent)
	}
}

// TestExitClosesTheShell: exit removes the pane and the ▣, and says so.
func TestExitClosesTheShell(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "wt")
	d, x, ux := shellDesk(t, cwd)
	pressT(x)
	shellID := ""
	for _, line := range strings.Split(strings.TrimSpace(shellPanes(d, "ux-pass")), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[1] == "ux-pass" {
			shellID = f[0]
			break
		}
	}
	if shellID == "" {
		t.Fatal("no shell to exit")
	}
	if row := listRowOf(*x.m, "ux-pass"); !strings.Contains(ansi.Strip(row), "▣") {
		t.Fatalf("before exit the row has no ▣:\n%s", row)
	}
	d.tmux("send-keys", "-t", shellID, "exit", "Enter")
	deadline := time.Now().Add(5 * time.Second)
	for strings.Contains(shellPanes(d, "ux-pass"), shellID) {
		if time.Now().After(deadline) {
			t.Fatalf("exit left the shell pane:\n%s", shellPanes(d, "ux-pass"))
		}
		time.Sleep(20 * time.Millisecond)
	}
	// A fresh state (and the console's look at the panes) sees the shell gone.
	x.update(stateMsg(boardState(ux)))
	row := listRowOf(*x.m, "ux-pass")
	if strings.Contains(ansi.Strip(row), "▣") {
		t.Fatalf("after exit the row still has ▣:\n%s", row)
	}
	if got := x.m.sideSays(); !strings.Contains(got, "shell closed") {
		t.Fatalf("after exit the console says %q, want shell closed", got)
	}
}

// listRowOf is the list's own row for name (not the shell line in the
// agent region, which also holds the name).
func listRowOf(m Model, name string) string {
	for _, s := range m.all.sessions {
		if s.Name == name || m.cardName(s) == name {
			return strings.Join(m.sideRow(s, 40, true, sideLink{}), "\n")
		}
	}
	return ""
}

// colorHex is c as #RRGGBB for a tmux style.
func colorHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}
