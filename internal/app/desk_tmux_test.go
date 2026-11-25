//go:build linux

package app

import (
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/sample"
)

// deskTmux runs the desk's tmux commands on d's server, and keeps each.
func deskTmux(d agentDesk) (run func(args ...string) (string, error), ran func() [][]string) {
	var mu sync.Mutex
	var all [][]string
	run = func(args ...string) (string, error) {
		mu.Lock()
		all = append(all, args)
		mu.Unlock()
		cmd := exec.Command("tmux", append([]string{"-L", d.sock}, args...)...) //nolint:gosec // a test's own server
		cmd.Env = noTmuxEnv()
		out, err := cmd.Output()
		return string(out), err
	}
	return run, func() [][]string { mu.Lock(); defer mu.Unlock(); return slices.Clone(all) }
}

// TestTheOldSplitMigratesOnce: a server that #488 left with two consoles,
// each beside its viewer pane, and tmux's ctrl+q flipping between them.
// A console's first desk start closes its viewer pane and unbinds ctrl+q,
// and says so once; the agent's own session is untouched, and a second
// start says nothing (design review D9). What the owner made stays: a pane
// of their own in the console's window, a hook of their own, and their own
// ctrl+q.
func TestTheOldSplitMigratesOnce(t *testing.T) {
	d := newAgentDesk(t, 80, 20, map[string]string{"flint-auth": "exec sleep 600"})
	agent := d.tmux("list-panes", "-s", "-t", "=flint-auth", "-F", "#{pane_id} #{pane_pid}")
	consoles := map[string]string{}
	for _, name := range []string{"matchblox", "matchblox-2"} {
		d.tmux("new-session", "-d", "-s", name, "-x", "160", "-y", "40", "sleep", "600")
		console := d.pane(name)
		viewer := d.tmux("split-window", "-h", "-d", "-t", console, "-P", "-F", "#{pane_id}", "sleep", "600")
		d.tmux("set-option", "-w", "-t", console, sample.ConsoleOption, console)
		d.tmux("set-option", "-w", "-t", console, sample.BesideOption, viewer)
		consoles[name] = console
	}
	own := d.tmux("split-window", "-v", "-d", "-t", consoles["matchblox"], "-P", "-F", "#{pane_id}", "sleep", "600")
	// The hooks as v0.58 set them (panes.BesideHookArgv): global, in MarkKey's
	// slot, naming the viewer's option. The owner's own hook sits in slot 0.
	for _, hook := range []string{"pane-exited", "pane-died"} {
		d.tmux("set-hook", "-g", hook+"[950]", `run-shell -C "display-message -d 1 '#{`+sample.BesideOption+`}'"`)
	}
	d.tmux("set-hook", "-g", "pane-exited[0]", "display-message -d 1 mine")
	// The binding as v0.58 made it (panes.BesideKeysArgv).
	d.tmux("bind-key", "-n", "C-q", "if-shell", "-F", "#{&&:#{@matchblox_home},#{"+sample.BesideOption+"}}",
		"run-shell -C 'select-pane -Z -t #{"+sample.BesideOption+"}'", "send-keys C-q")
	run, _ := deskTmux(d)
	start := func(name string) string {
		t.Helper()
		m := New(Options{NoMotion: true, Conn: newFake(), Pane: consoles[name], Tmux: run, Beside: true})
		m = upd(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
		for _, msg := range cmdMsgsWithin(m.migrateSplit(), 5*time.Second) {
			m = upd(t, m, msg)
		}
		return m.sideSays()
	}
	const said = "the agent now shows inside matchblox · ctrl+q flips, as before"
	for _, name := range []string{"matchblox", "matchblox-2"} {
		if got := start(name); got != said {
			t.Errorf("%s's first desk start says %q, want %q", name, got, said)
		}
		want := consoles[name]
		if name == "matchblox" {
			want += "\n" + own
		}
		if panes := d.tmux("list-panes", "-t", consoles[name], "-F", "#{pane_id}"); panes != want {
			t.Errorf("%s's window has the panes %q, want %q: its console's and the owner's", name, panes, want)
		}
	}
	for _, hook := range []string{"pane-exited", "pane-died"} {
		if hooks := d.tmux("show-hooks", "-g", hook); strings.Contains(hooks, "[950]") {
			t.Errorf("the old split's %s hook stays:\n%s", hook, hooks)
		}
	}
	if hooks := d.tmux("show-hooks", "-g", "pane-exited"); !strings.Contains(hooks, "pane-exited[0] display-message -d 1 mine") {
		t.Errorf("the owner's own hook is gone:\n%s", hooks)
	}
	if keys := d.tmux("list-keys", "-T", "root"); strings.Contains(keys, "C-q") {
		t.Errorf("tmux still binds ctrl+q, so it never reaches matchblox:\n%s", keys)
	}
	if got := d.tmux("list-panes", "-s", "-t", "=flint-auth", "-F", "#{pane_id} #{pane_pid}"); got != agent {
		t.Errorf("the agent's session is %q, was %q", got, agent)
	}
	d.tmux("bind-key", "-n", "C-q", "display-message", "mine")
	if got := start("matchblox"); got != "" {
		t.Errorf("a second start says %q, want nothing", got)
	}
	if keys := d.tmux("list-keys", "-T", "root", "C-q"); !strings.Contains(keys, "display-message mine") {
		t.Errorf("the owner's own ctrl+q is gone: %q", keys)
	}
}

// TestNoSplitOnTheDesk: the desk starts and shows a session in its agent
// region; the console's window keeps one pane, and nothing splits it.
func TestNoSplitOnTheDesk(t *testing.T) {
	d := newAgentDesk(t, 119, 36, map[string]string{"flint-auth": `printf 'FLINT SCREEN\n'; exec sleep 600`})
	d.tmux("new-session", "-d", "-s", "matchblox", "-x", "160", "-y", "40", "sleep", "600")
	console := d.pane("matchblox")
	run, ran := deskTmux(d)
	flint := doneMatch("flint-auth", 10*time.Minute)
	flint.Pane = d.pane("flint-auth")
	m := New(Options{NoMotion: true, Conn: newFake(), Pane: console, Socket: d.path, Tmux: run, Beside: true, Now: func() time.Time { return boardAt }})
	x := &regionRun{t: t, m: &m, r: &m.agent, msgs: make(chan tea.Msg, 64)}
	t.Cleanup(x.r.shut)
	x.update(tea.WindowSizeMsg{Width: 160, Height: 40})
	x.update(stateMsg(boardState(flint)))
	x.m.splashDone = true
	x.pump(m.migrateSplit())
	x.press("enter")
	for end := time.Now().Add(10 * time.Second); !x.shows("FLINT SCREEN")(); {
		select {
		case msg := <-x.msgs:
			if _, ok := msg.(shownMsg); ok {
				x.update(msg)
			}
			if _, ok := msg.(agentFrameMsg); ok {
				x.update(msg)
			}
		case <-time.After(20 * time.Millisecond):
		}
		if time.Now().After(end) {
			t.Fatalf("⏎ showed no agent; the desk reads:\n%s", plainFrame(*x.m))
		}
	}
	if line := plainLines(strings.Split(x.m.render(), "\n"))[2]; !strings.HasPrefix(string([]rune(line)[41:]), "FLINT SCREEN") {
		t.Errorf("the agent region's first line is %q, want FLINT SCREEN at column 41", line)
	}
	if panes := d.tmux("list-panes", "-t", console, "-F", "#{pane_id}"); panes != console {
		t.Errorf("the console's window has the panes %q, want one", panes)
	}
	for _, c := range ran() {
		if slices.Contains(c, "split-window") || slices.Contains(c, "respawn-pane") {
			t.Errorf("the desk ran tmux %q", c)
		}
	}
}
