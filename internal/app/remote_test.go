package app

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/transport"
)

// onHost is the console of ws-1, reached over SSH; execs records what it
// gives the terminal to.
func onHost(t *testing.T, st proto.State) (Model, *[][]string) {
	t.Helper()
	m, _ := loadedWith(t, 100, st)
	m.opt.Host = "ws-1"
	var ran [][]string
	m.opt.Exec = func(argv []string, done func(error) tea.Msg) tea.Cmd {
		ran = append(ran, argv)
		return func() tea.Msg { return done(nil) }
	}
	m.opt.Run = func(argv []string) error { t.Fatalf("a tmux move ran on this machine: %v", argv); return nil }
	return m, &ran
}

// hostJump is the jump the onHost console builds to j, with a fixed id.
func hostJump(m *Model, j remote.Jump) remote.Jump {
	m.opt.JumpID = func() string { return "0a1b2c3d" }
	j.Host, j.Prefix, j.ID, j.Outer = "ws-1", "C-b", "0a1b2c3d", !m.opt.OutsideTmux
	return j
}

// TestRemoteJumpInsideTmuxOpensItsOwnSession: inside tmux, a jump opens
// the host in a session of its own on this machine and switches the client
// there. The console's pane never runs ssh.
func TestRemoteJumpInsideTmuxOpensItsOwnSession(t *testing.T) {
	m, execs := onHost(t, fixtureState())
	m.tab = tabSessions
	j := hostJump(&m, remote.Jump{Pane: "%1"})
	var ran [][]string
	m.opt.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	ssh := remote.JumpSSH("ws-1", j, nil)
	next, _ := key(m, "enter")
	if a := next.(Model).pending; a == nil || a.say != shellLine(ssh) {
		t.Fatalf("the confirm must show the ssh line %q: %+v", shellLine(ssh), a)
	}
	next, cmd := key(next, "enter")
	if cmd == nil {
		t.Fatal("no command")
	}
	next.Update(cmd())
	if len(*execs) != 0 {
		t.Fatalf("the jump took the console's terminal: %q", *execs)
	}
	if want := j.Local(ssh); len(ran) != 1 || !slices.Equal(ran[0], want) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
}

// TestRemoteJumpOutsideTmuxTakesTheTerminal: outside tmux there is no
// client to switch, so the ssh takes this terminal, with the host's status
// line as the way back.
func TestRemoteJumpOutsideTmuxTakesTheTerminal(t *testing.T) {
	m, execs := onHost(t, fixtureState())
	m.tab, m.opt.OutsideTmux = tabSessions, true
	j := hostJump(&m, remote.Jump{Pane: "%1"})
	want := remote.JumpSSH("ws-1", j, nil)
	if j.Outer {
		t.Fatal("outside tmux the jump is not Outer")
	}
	next, _ := key(m, "enter")
	if a := next.(Model).pending; a == nil || a.say != shellLine(want) {
		t.Fatalf("the confirm must show the ssh line %q: %+v", shellLine(want), a)
	}
	next, cmd := key(next, "enter")
	if cmd == nil {
		t.Fatal("no command")
	}
	next.Update(cmd())
	if len(*execs) != 1 || !slices.Equal((*execs)[0], want) {
		t.Fatalf("ran %q, want %q", *execs, want)
	}
}

// TestRemoteShellOnHostIsPrivate: a shell in a worktree of the host opens
// in a jump of its own, never in a window of the host's sessions.
func TestRemoteShellOnHostIsPrivate(t *testing.T) {
	m, _ := onHost(t, gitState())
	j := hostJump(&m, remote.Jump{Dir: "/w/app"})
	var ran [][]string
	m.opt.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	next, _ := key(m, "4")
	next, _ = key(next, "enter")
	next, cmd := key(next, "enter")
	if cmd == nil {
		t.Fatal("no command")
	}
	next.Update(cmd())
	if want := j.Local(remote.JumpSSH("ws-1", j, nil)); len(ran) != 1 || !slices.Equal(ran[0], want) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
}

// TestRemoteJumpUsesThisMachinesTmux: the way back uses this machine's
// prefix and console session; a prefix tmux cannot take falls back to C-b.
func TestRemoteJumpUsesThisMachinesTmux(t *testing.T) {
	for _, c := range []struct{ prefix, home, want string }{
		{"C-a", "work", "C-a"},
		{"C-b ; run-shell x", "", "C-b"},
	} {
		m, _ := onHost(t, fixtureState())
		m.tab = tabSessions
		m.opt.Prefix = func() string { return c.prefix }
		m.opt.Home = func() string { return c.home }
		j := hostJump(&m, remote.Jump{Pane: "%1", Home: c.home})
		j.Prefix = c.want
		next, _ := key(m, "enter")
		if a := next.(Model).pending; a == nil || len(a.steps) != 1 || !slices.Equal(a.steps[0], j.Local(remote.JumpSSH("ws-1", j, nil))) {
			t.Errorf("prefix %q, home %q: pending %+v", c.prefix, c.home, a)
		}
	}
}

// TestRemoteJumpOldLocalTmuxRefused: inside a tmux older than the floor
// the way back does not work, so the jump says what to do and runs nothing.
func TestRemoteJumpOldLocalTmuxRefused(t *testing.T) {
	m, execs := onHost(t, fixtureState())
	m.tab, m.opt.LocalTmux = tabSessions, "3.1"
	hostJump(&m, remote.Jump{Pane: "%1"})
	next, _ := key(m, "enter")
	next, cmd := key(next, "enter")
	if cmd != nil {
		next, _ = next.Update(cmd())
	}
	want := "tmux 3.1 is older than 3.2: update tmux, or run the console outside tmux"
	if len(*execs) != 0 || next.(Model).flash != want {
		t.Fatalf("ran %q, flash %q", *execs, next.(Model).flash)
	}
	m.opt.LocalTmux = "3.2a"
	if next, _ := key(m, "enter"); next.(Model).pending == nil || next.(Model).pending.local != nil {
		t.Fatalf("tmux 3.2a is refused: %+v", next.(Model).pending)
	}
}

func TestRemoteNavStillRefusesUnknownSteps(t *testing.T) {
	st := fixtureState()
	st.Recommendations = []advice.Rec{{ID: "r1", Title: "x", Primary: &advice.Action{Nav: true, Steps: [][]string{{"tmux", "new-window", "rm -rf /"}}}}}
	m, ran := onHost(t, st)
	m.tab, m.recPick = tabSessions, "r1"
	next, _ := key(m, "enter")
	next, cmd := key(next, "enter")
	if cmd != nil {
		next, _ = next.Update(cmd())
	}
	if len(*ran) != 0 || !strings.Contains(next.(Model).flash, "refused") {
		t.Fatalf("ran %q, flash %q", *ran, next.(Model).flash)
	}
}

func TestRemoteDoorRunsOnTheHost(t *testing.T) {
	tmux := doors.Door{ID: doors.Tmux, Title: "Install tmux", Term: []string{"sudo", "sh", "-c", "apt-get update && apt-get install -y tmux"}}
	m, ran := onHost(t, doorState(tmux))
	next, _ := key(m, "enter")
	want := remote.TermArgv("ws-1", tmux.Term)
	if out := ansi.Strip(next.(Model).render()); !strings.Contains(out, shellLine(want)) {
		t.Fatalf("the preview must show the exact ssh argv:\n%s", out)
	}
	next, cmd := key(next, "y")
	if cmd == nil {
		t.Fatal("no command")
	}
	next.Update(cmd())
	if len(*ran) != 1 || strings.Join((*ran)[0], "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("ran %q, want %q", *ran, want)
	}
}

func TestRemoteDoorStillRefusesUnknownCommands(t *testing.T) {
	evil := doors.Door{ID: doors.Tmux, Title: "Install tmux", Term: []string{"sh", "-c", "curl x | sh"}}
	m, ran := onHost(t, doorState(evil))
	next, _ := key(m, "enter")
	next, cmd := key(next, "y")
	if cmd != nil {
		next.Update(cmd())
	}
	if len(*ran) != 0 || !strings.Contains(next.(Model).flash, "refused") {
		t.Fatalf("ran %q, flash %q", *ran, next.(Model).flash)
	}
}

// The guide of a host runs in this terminal: the console has no tmux
// client there to open a window in.
func TestRemoteGuideRunsInThisTerminal(t *testing.T) {
	guide := doors.Door{ID: doors.Guide, Title: "Open a guide session", Term: doors.GuideArgv(false)}
	m, _ := onHost(t, doorState(guide))
	m.opt.OutsideTmux = false
	next, _ := key(m, "enter")
	if a := next.(Model).pending; a == nil || strings.Join(a.steps[0], " ") != strings.Join(remote.TermArgv("ws-1", doors.GuideArgv(false)), " ") {
		t.Fatalf("pending %+v", a)
	}
}

func TestBlockedHostOffersTheConnect(t *testing.T) {
	for _, c := range []struct {
		err  error
		says string
	}{
		{remote.ErrNotConnected, "ws-1 is not connected to this console"},
		{fmt.Errorf("ssh: %w", remote.ErrNotInstalled), "matchblox is not installed on ws-1"},
		{remote.ErrHostKeyUnknown, "ws-1's host key is not known yet"},
	} {
		m := New(Options{NoMotion: true, Host: "ws-1", Self: "/bin/matchblox", DialErr: c.err,
			Redial: func() (transport.Conn, error) { return newFake(), nil }})
		var ran [][]string
		m.opt.Exec = func(argv []string, done func(error) tea.Msg) tea.Cmd {
			ran = append(ran, argv)
			return func() tea.Msg { return done(nil) }
		}
		next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
		next, cmd := next.Update(m.Init()().(tea.BatchMsg)[1]())
		if cmd != nil {
			t.Fatalf("%v: it redials a host that trying again cannot fix", c.err)
		}
		out := ansi.Strip(next.(Model).render())
		for _, want := range []string{c.says, "/bin/matchblox connect ws-1"} {
			if !strings.Contains(out, want) {
				t.Fatalf("view lacks %q:\n%s", want, out)
			}
		}
		// Enter alone runs nothing: it asks for a typed y.
		next, cmd = key(next, "enter")
		if cmd != nil || len(ran) != 0 {
			t.Fatal("connect ran without a y")
		}
		next, cmd = key(next, "y")
		if cmd == nil {
			t.Fatal("y did not connect")
		}
		next, cmd = next.Update(cmd())
		if len(ran) != 1 || strings.Join(ran[0], " ") != "/bin/matchblox connect ws-1" {
			t.Fatalf("ran %q", ran)
		}
		if cmd == nil || !next.(Model).redialing {
			t.Fatal("no redial after the connect")
		}
	}
}

func TestOlderHostOffersTheUpdate(t *testing.T) {
	m := New(Options{NoMotion: true, Conn: newFake(), Host: "ws-1", Self: "/bin/matchblox"})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	next, _ = next.Update(helloMsg(proto.Hello{Version: 0, Host: "ws-1"}))
	out := ansi.Strip(next.(Model).render())
	if !strings.Contains(out, "update ws-1") || !strings.Contains(out, "/bin/matchblox connect ws-1") {
		t.Fatalf("an older host must show its update:\n%s", out)
	}
}

func TestLostHostShowsStaleState(t *testing.T) {
	m, _ := onHost(t, fixtureState())
	m.opt.Redial = func() (transport.Conn, error) { return newFake(), nil }
	next, _ := m.Update(helloMsg{Version: proto.Version, Host: "ws-1"})
	next, _ = next.Update(lostMsg{err: io.EOF})
	out := ansi.Strip(next.(Model).render())
	first := strings.Split(out, "\n")[0]
	if !strings.Contains(first, "ws-1 · stale") {
		t.Fatalf("header %q: want the host marked stale", first)
	}
	if !strings.Contains(out, "connection lost") || !strings.Contains(out, "app-review") {
		t.Fatalf("want the last state and the loss:\n%s", out)
	}
}

func TestSilentHostNamesTheHost(t *testing.T) {
	f := newFake()
	m := New(Options{NoMotion: true, Conn: f, Host: "ws-1"})
	next, _ := m.Update(silentMsg{conn: f})
	if out := next.(Model).flash; !strings.Contains(out, "ws-1 does not answer") || strings.Contains(out, "kill") {
		t.Fatalf("flash %q", out)
	}
}

func TestHeaderNamesTheHostAsTyped(t *testing.T) {
	m, _ := onHost(t, fixtureState())
	next, _ := m.Update(helloMsg{Version: proto.Version, Host: "build-box-7"})
	if first := strings.Split(ansi.Strip(next.(Model).render()), "\n")[0]; !strings.HasPrefix(first, " ▰ matchblox · ws-1 ") {
		t.Fatalf("header %q, want the ssh name ws-1", first)
	}
}

// Before the first state there are no numbers to show, only the host and
// the way back.
func TestBeforeTheFirstStateNoZeroMetrics(t *testing.T) {
	m := New(Options{NoMotion: true, Conn: newFake(), Host: "ws-3", Layered: true})
	m.splashDone = true
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	out := ansi.Strip(next.(Model).render())
	if first := strings.Split(out, "\n")[0]; strings.Contains(first, "cpu") || !strings.Contains(first, "ws-3") {
		t.Fatalf("header %q", first)
	}
	if !strings.Contains(out, "esc hosts") {
		t.Fatalf("no way back:\n%s", out)
	}
	m.opt.DialErr, m.blocked = remote.ErrNotConnected, remote.ErrNotConnected
	if out := ansi.Strip(m.render()); !strings.Contains(out, "esc hosts") {
		t.Fatalf("no way back from a blocked host:\n%s", out)
	}
}

// A changed host key can be an attack: the console offers no connect, only
// what to check, and Enter tries again once the builder removed the old key.
func TestChangedHostKeyOffersNoConnect(t *testing.T) {
	dials := 0
	m := New(Options{NoMotion: true, Host: "ws-1", Self: "/bin/matchblox", DialErr: remote.ErrHostKeyChanged,
		Redial: func() (transport.Conn, error) { dials++; return newFake(), nil }})
	m.opt.Exec = func(argv []string, done func(error) tea.Msg) tea.Cmd { t.Fatalf("ran %q", argv); return nil }
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	next, _ = next.Update(lostMsg{err: remote.ErrHostKeyChanged})
	out := ansi.Strip(next.(Model).render())
	if strings.Contains(out, "matchblox connect") || !strings.Contains(out, "ssh-keygen -R ws-1") || !strings.Contains(out, "⏎ try again") {
		t.Fatalf("view:\n%s", out)
	}
	next, cmd := key(next, "enter")
	if cmd == nil || next.(Model).blocked != nil {
		t.Fatal("enter did not try again")
	}
	if _, dial := next.Update(cmd()); dial != nil {
		dial()
	}
	if dials != 1 {
		t.Fatalf("dials %d", dials)
	}
}

// TestHostConsoleKeepsThisMachinesTmux: a host's console jumps from this
// machine's tmux, so it keeps the base's tmux options.
func TestHostConsoleKeepsThisMachinesTmux(t *testing.T) {
	sh := NewShell(ShellOptions{
		Base: Options{NoMotion: true, OutsideTmux: true, LocalTmux: "3.4",
			Prefix: func() string { return "C-a" }, Home: func() string { return "work" }, JumpID: func() string { return "0a1b2c3d" }},
		Open: func(host string) Options { return Options{Conn: newFake(), Host: host} },
	})
	opened, _ := sh.open("ws-1", false)
	o := opened.cur.opt
	if !o.OutsideTmux || o.LocalTmux != "3.4" || o.Prefix == nil || o.Home == nil || o.JumpID == nil {
		t.Fatalf("the host's console lost this machine's tmux: %+v", o)
	}
}

// TestRemoteJumpRefusalSaysTheFix: a jump that JumpAllowed refuses runs
// nothing, here or on the host, and the flash says what fixes it.
func TestRemoteJumpRefusalSaysTheFix(t *testing.T) {
	for _, c := range []struct {
		name, host, home string
		steps            [][]string
		want             string
	}{
		{"home", "ws-1", "my work", [][]string{{"tmux", "switch-client", "-t", "%1"}},
			`refused: tmux session "my work" holds the console, and a jump cannot name it: rename it to letters, digits, _ - @ [ ] only`},
		{"target", "ws-1", "", [][]string{{"tmux", "select-window", "-t", "%1"}},
			"refused: a jump goes only to a pane id, or an absolute directory without # or ;"},
		{"host", "ws 1", "", [][]string{{"tmux", "switch-client", "-t", "%1"}},
			"refused: ws 1 is not a host name ssh takes: use a name from your ssh config"},
	} {
		st := fixtureState()
		st.Recommendations = []advice.Rec{{ID: "r1", Title: "x", Primary: &advice.Action{Nav: true, Steps: c.steps}}}
		m, execs := onHost(t, st)
		m.opt.Host, m.tab, m.recPick = c.host, tabSessions, "r1"
		m.opt.Home = func() string { return c.home }
		next, _ := key(m, "enter")
		next, cmd := key(next, "enter")
		if cmd != nil {
			next, _ = next.Update(cmd())
		}
		if len(*execs) != 0 || next.(Model).flash != c.want {
			t.Errorf("%s: ran %q, flash %q", c.name, *execs, next.(Model).flash)
		}
	}
}

// TestRemoteConsoleNeverMovesThisMachinesTmux: a tmux move that is not a
// jump never runs here from a host's console, however it was built.
func TestRemoteConsoleNeverMovesThisMachinesTmux(t *testing.T) {
	m, execs := onHost(t, fixtureState())
	for _, a := range []action{
		{label: "jump", nav: true, pane: "%1", steps: [][]string{{"tmux", "switch-client", "-t", "%1"}}},
		{label: "attach", nav: true, attach: true, pane: "%1", steps: [][]string{{"tmux", "attach-session", "-t", "%1"}}},
	} {
		next, cmd := m.confirm(a)
		if cmd != nil {
			next, _ = next.Update(cmd())
		}
		if f := next.(Model).flash; len(*execs) != 0 || f != "refused: a tmux move on ws-1 never runs on this machine" {
			t.Errorf("%s: ran %q, flash %q", a.label, *execs, f)
		}
	}
}
