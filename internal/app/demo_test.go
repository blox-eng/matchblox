package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
)

const demoLine = "This is a demo. Install matchblox to do that."

// demoConsole is the console of the demo host, on the fixture machine.
func demoConsole(t *testing.T, w int, o Options) Model {
	t.Helper()
	o.NoMotion, o.Conn, o.Demo = true, newFake(), demoLine
	next, _ := New(o).Update(tea.WindowSizeMsg{Width: w, Height: 30})
	next, _ = next.Update(stateMsg(fixtureState()))
	m := next.(Model)
	m.tab, m.splashDone = tabSessions, true
	return m
}

func saysTheLine(t *testing.T, m Model) {
	t.Helper()
	if m.flash != demoLine {
		t.Errorf("the key line says %q, want the demo's line", m.flash)
	}
	if m.pending != nil {
		t.Errorf("a confirm opened: %s", m.pending)
	}
}

// Each seam of Options that would run a command answers with the line, for a
// demo that names no other.
func TestADemoClosesEverySeamToTheMachine(t *testing.T) {
	m := demoConsole(t, 100, Options{})
	refusal := func(err error) {
		t.Helper()
		if err == nil || err.Error() != demoLine {
			t.Errorf("got %v, want the demo's line", err)
		}
	}
	refusal(m.opt.Run([]string{"tmux", "switch-client"}))
	msg := m.exec([]string{"tmux", "attach-session"}, func(err error) tea.Msg { return err })()
	if err, _ := msg.(error); err == nil {
		t.Errorf("the terminal was given away: %v", msg)
	} else {
		refusal(err)
	}
	_, err := m.clients()()
	refusal(err)
	_, _, err = m.opt.LastClient()
	refusal(err)
	if said, exit := m.checker("ws-1")(context.Background(), []string{"ssh"}, ignoreLine); said != demoLine || exit == 0 {
		t.Errorf("the login check says %q, exit %d", said, exit)
	}
	if said, exit := m.ssh()(context.Background(), []string{"ssh"}, nil, ignoreLine); said != demoLine || exit == 0 {
		t.Errorf("the ssh says %q, exit %d", said, exit)
	}
}

// spyExec is an Exec that writes down what it is given.
func spyExec(saw *[][]string) func([]string, func(error) tea.Msg) tea.Cmd {
	return func(argv []string, done func(error) tea.Msg) tea.Cmd {
		*saw = append(*saw, argv)
		return func() tea.Msg { return done(nil) }
	}
}

func TestWhatLeavesTheDemoSaysSoAndRunsNothing(t *testing.T) {
	var saw [][]string
	run := func([]string) error {
		saw = append(saw, []string{"run"})
		return nil
	}
	o := Options{Exec: spyExec(&saw), Run: run}
	for name, try := range map[string]func(t *testing.T) Model{
		"ctrl+o": func(t *testing.T) Model { return pressKeys(t, demoConsole(t, 100, o), "ctrl+o") },
		"ctrl+o in the chat": func(t *testing.T) Model {
			m := pressKeys(t, demoSized(t, 44, 31), "enter")
			if !m.chatKeys {
				t.Fatal("the chat did not open")
			}
			m.opt.Demo, m.opt.Exec, m.opt.Run = demoLine, o.Exec, o.Run
			return pressKeys(t, m, "ctrl+o")
		},
		"a pane": func(t *testing.T) Model {
			m := demoConsole(t, 100, o)
			m.tab = tabPanes
			return pressKeys(t, m, "enter")
		},
		"a device": func(t *testing.T) Model {
			o := o
			o.Pair = &PairHost{}
			m := demoConsole(t, 100, o)
			m.tab = tabMachine
			m.machinePick(m.machineRows() - 1)
			return pressKeys(t, m, "enter")
		},
		"a door's command": func(t *testing.T) Model {
			m := demoConsole(t, 100, o)
			m.doors = []doors.Door{{ID: doors.Tmux, Title: "Install tmux", Term: []string{"sh", "-c", "true"}}}
			m.doorPick = doors.Tmux
			return pressKeys(t, m, "enter")
		},
		"a confirm that got past the words": func(t *testing.T) Model {
			next, _ := demoConsole(t, 100, o).confirm(action{label: "connect", connect: true, remote: true, steps: [][]string{{"matchblox", "connect", "ws-1"}}})
			return next.(Model)
		},
	} {
		t.Run(name, func(t *testing.T) {
			saw = nil
			saysTheLine(t, try(t))
			if len(saw) > 0 {
				t.Errorf("it ran %v", saw)
			}
		})
	}
}

// A host of another version: connecting is the way to reach it, and also what
// updates it.
func TestConnectingOrUpdatingAHostSaysSo(t *testing.T) {
	for name, set := range map[string]func(m *Model){
		"connect": func(m *Model) { m.blocked = errors.New("not connected") },
		"update":  func(m *Model) { m.mismatch, m.host = true, proto.Hello{Version: proto.Version - 1} },
	} {
		t.Run(name, func(t *testing.T) {
			var saw [][]string
			m := demoConsole(t, 100, Options{Host: "ws-1", Exec: spyExec(&saw)})
			set(&m)
			next, cmd := m.unblock()
			nm := next.(Model)
			if nm.pending == nil || !nm.pending.connect {
				t.Fatalf("no connect to refuse: %+v", nm.pending)
			}
			next, _ = nm.Update(cmd)
			saysTheLine(t, next.(Model))
			if len(saw) > 0 {
				t.Errorf("it ran %v", saw)
			}
		})
	}
}

// However a refusal reaches the console, from the world by way of the service,
// the key line is the line alone.
func TestARefusalFromTheServiceIsTheLineAlone(t *testing.T) {
	m := demoConsole(t, 100, Options{})
	next, _ := m.Update(resultMsg(proto.Result{ActID: "a1", Err: "gh pr merge 214: " + demoLine}))
	saysTheLine(t, next.(Model))

	m.form = &form{act: "a2"}
	next, _ = m.Update(resultMsg(proto.Result{ActID: "a2", Err: demoLine}))
	got := next.(Model)
	saysTheLine(t, got)
	if got.form != nil {
		t.Error("the form stayed on the refusal: a dead end")
	}
}

// A switch of agent ran: what it says is its own, for only the jump into the
// pane that would follow cannot go.
func TestASwitchInTheDemoSaysItRan(t *testing.T) {
	m := demoConsole(t, 100, Options{})
	m.jumpAfter.act, m.jumpAfter.pane, m.jumpAfter.done = "a1", "%1", "switched to codex"
	next, _ := m.Update(resultMsg(proto.Result{ActID: "a1", Ran: [][]string{{"tmux", "respawn-pane"}}}))
	if got := next.(Model).flash; got != "switched to codex" {
		t.Errorf("the key line says %q", got)
	}
}

// A git URL in where: a clone is what Enter and tab would start. The form goes,
// for no dead end, and no clone is asked of the service.
func TestAFormThatWouldCloneSaysSo(t *testing.T) {
	for _, k := range []string{"enter", "tab"} {
		m := demoConsole(t, 100, Options{})
		f := newFake()
		m.conn = f
		m.form = &form{at: fieldWhere, where: "example/cloned", agent: "claude"}
		m = pressKeys(t, m, k)
		saysTheLine(t, m)
		if m.form != nil {
			t.Errorf("%s: the form stayed", k)
		}
		if acts := f.acts(); len(acts) > 0 {
			t.Errorf("%s: it sent %+v", k, acts)
		}
	}
}

// The chat's key line offers no way into tmux on the demo host; the key answers
// all the same.
// A tap on the row a URL shows is what Enter is: the form goes, and the line is
// in the key line. Both before the plan's answer and after it.
func TestATapOnACloneRowSaysSo(t *testing.T) {
	for _, planErr := range []string{"", demoLine} {
		m := demoConsole(t, 44, Options{})
		f := newFake()
		m.conn = f
		next, _ := m.Update(stateMsg(lifeState()))
		m = pressKeys(t, next.(Model), "N")
		for _, r := range "example/cloned" {
			m = pressKeys(t, m, string(r))
		}
		if m.form == nil {
			t.Fatal("no form")
		}
		fm := *m.form
		fm.planErr, fm.planFor = planErr, "example/cloned"
		m.form = &fm
		sc := m.draw(44)
		row := slices.IndexFunc(sc.boxRows, func(p string) bool { return strings.HasPrefix(p, "row:") })
		if row < 0 {
			t.Fatalf("no row in %q", sc.boxRows)
		}
		next, _ = m.Update(tea.MouseClickMsg{X: sc.boxX + 4, Y: sc.boxAt + row, Button: tea.MouseLeft})
		got := next.(Model)
		saysTheLine(t, got)
		if got.form != nil {
			t.Errorf("planErr %q: the form stayed", planErr)
		}
		for _, a := range f.acts() {
			if a.RecID == "clone:start" {
				t.Errorf("a clone started: %+v", a)
			}
		}
	}
}

func TestTheChatsKeyLineOffersNoTmuxInTheDemo(t *testing.T) {
	m := demoSized(t, 132, 31)
	m.opt.Demo = demoLine
	m = pressKeys(t, m, "enter")
	if !m.chatKeys {
		t.Fatal("the chat did not open")
	}
	line, spans := m.chatKeyLine(132)
	if got := ansi.Strip(line); strings.Contains(got, "ctrl+o") || strings.Contains(got, "tmux") || !strings.Contains(got, "ctrl+a m back") {
		t.Errorf("the key line says %q", got)
	}
	for _, s := range spans {
		if s.press == "ctrl+o" {
			t.Errorf("a tap presses ctrl+o: %+v", s)
		}
	}
	saysTheLine(t, pressKeys(t, m, "ctrl+o"))
}

func TestTheChatsShortScreenSendsNoOneToAPane(t *testing.T) {
	m := demoConsole(t, 100, Options{})
	s := m.snap.Sessions[0]
	if got := m.tooShort(s); strings.Contains(got, "ctrl+o") || strings.Contains(got, "pane") || !strings.Contains(got, s.Name) {
		t.Errorf("the demo says %q", got)
	}
	m.opt.Demo = ""
	if got := m.tooShort(s); !strings.Contains(got, "ctrl+o goes into its pane") {
		t.Errorf("a real console says %q", got)
	}
}

// The line is longer than a phone is wide: it takes the hair's row, all of it.
func TestTheLineReadsWholeOnAPhone(t *testing.T) {
	for _, chat := range []bool{false, true} {
		m := demoConsole(t, 44, Options{})
		if chat {
			m = demoSized(t, 44, 31)
			m.opt.Demo = demoLine
			m = pressKeys(t, m, "enter")
		}
		m = pressKeys(t, m, "ctrl+o")
		lines := strings.Split(ansi.Strip(m.render()), "\n")
		last := strings.TrimSpace(lines[len(lines)-2]) + " " + strings.TrimSpace(lines[len(lines)-1])
		if last != demoLine || strings.TrimSpace(lines[len(lines)-1]) != "Install matchblox to do that." {
			t.Errorf("chat %v: the last two rows read %q", chat, last)
		}
	}
	m := pressKeys(t, demoConsole(t, 100, Options{}), "ctrl+o")
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	if got := strings.TrimSpace(lines[len(lines)-1]); got != demoLine {
		t.Errorf("wide: the key line reads %q", got)
	}
	if !strings.HasPrefix(lines[len(lines)-2], "─") {
		t.Errorf("wide: the hair went: %q", lines[len(lines)-2])
	}
}

// Without a demo each site is as it was: a confirm, then the terminal.
func TestAConsoleWithoutADemoIsAsItWas(t *testing.T) {
	var saw [][]string
	moved := 0
	o := Options{NoMotion: true, Conn: newFake(), OutsideTmux: true, Exec: spyExec(&saw),
		Run: func([]string) error { moved++; return nil }}
	next, _ := New(o).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	next, _ = next.Update(stateMsg(fixtureState()))
	m := next.(Model)
	m.tab, m.splashDone = tabSessions, true

	m = pressKeys(t, m, "ctrl+o")
	if m.pending == nil || !m.pending.attach || m.flash == demoLine {
		t.Fatalf("ctrl+o: pending %+v, flash %q", m.pending, m.flash)
	}
	next, cmd := key(m, "enter")
	if cmd == nil {
		t.Fatal("the confirm ran nothing")
	}
	next.(Model).Update(cmd())
	if moved != 2 || len(saw) != 1 || !slices.Equal(saw[0][:2], []string{"tmux", "attach-session"}) {
		t.Errorf("moved %d times, gave the terminal to %v", moved, saw)
	}

	saw = nil
	d := doors.Door{ID: doors.Tmux, Title: "Install tmux", Term: doors.GuideArgv(false)}
	_, cmd = m.runDoor(*m.doorAction(d))
	if cmd == nil || !doors.TermAllowed(d.Term) {
		t.Fatal("the door runs nothing")
	}
	cmd()
	if len(saw) != 1 {
		t.Errorf("the door gave the terminal to %v", saw)
	}

	real := New(Options{Conn: newFake()})
	if real.opt.Exec != nil || real.opt.Clients != nil || real.opt.LastClient != nil || real.opt.Check != nil || real.opt.SSH != nil {
		t.Error("a real console's seams were filled")
	}
}
