package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
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

// onSession is m with the session named name selected.
func onSession(t *testing.T, m Model, name string) Model {
	t.Helper()
	for _, s := range m.snap.Sessions {
		if s.Name == name {
			m.chose, m.selPID, m.selHost = true, s.PID, s.Host
			return m
		}
	}
	t.Fatalf("no session %s", name)
	return m
}

// standIn is the demo's stand-in on flint-auth, which asks, w columns wide.
func standIn(t *testing.T, w int) Model {
	t.Helper()
	m := onSession(t, demoSessions(t, w), "flint-auth")
	m.opt.Demo = demoLine
	return pressKeys(t, m, "enter")
}

func TestCtrlOInTheDemoOpensTheStandInAsEnterDoes(t *testing.T) {
	m := onSession(t, demoSessions(t, 44), "flint-auth")
	m.opt.Demo = demoLine
	m = pressKeys(t, m, "ctrl+o")
	if !m.standIn || m.flash == demoLine {
		t.Fatalf("ctrl+o: stand-in %v, flash %q", m.standIn, m.flash)
	}
}

func TestEnterInTheDemoOpensTheStandIn(t *testing.T) {
	m := standIn(t, 44)
	if !m.standIn {
		t.Fatal("enter opened no stand-in")
	}
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	if !strings.HasPrefix(strings.TrimSpace(lines[0]), "‹ Sessions") {
		t.Errorf("row 0 is %q, want the way back to Sessions", lines[0])
	}
	if last := lines[len(lines)-1]; !strings.Contains(last, standInHint) {
		t.Errorf("the key line is %q, want %q", last, standInHint)
	}
}

// The stand-in is the whole screen at every width, as a jump into a pane is.
func TestTheStandInIsFullScreenOnADesk(t *testing.T) {
	m := standIn(t, 132)
	if _, right := m.panes(132); right == 0 {
		t.Fatal("132 columns do not split Sessions: the test shows nothing")
	}
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	if !m.standIn || !strings.HasPrefix(strings.TrimSpace(lines[0]), "‹ Sessions") {
		t.Fatalf("stand-in %v, row 0 is %q", m.standIn, lines[0])
	}
	if cols, _ := m.chatSize(); cols != 132 {
		t.Errorf("the agent's window is %d columns, want the whole 132", cols)
	}
}

// A screen with no room for the question closes the stand-in: the list's
// opened row shows it, and no key goes to a menu the person cannot see.
func TestAShortScreenClosesTheStandIn(t *testing.T) {
	m := standIn(t, 44)
	f := m.conn.(*fakeConn)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 106, Height: 10})
	m = next.(Model)
	if m.standIn || m.viewOpen {
		t.Fatalf("106x10: stand-in %v, view %v: want the list", m.standIn, m.viewOpen)
	}
	m = pressKeys(t, m, "down", "2", "esc")
	if got := keyActs(f); len(got) != 0 {
		t.Errorf("keys reached the agent while its question was not drawn: %+v", got)
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 44, Height: 31})
	if next.(Model).standIn {
		t.Error("turning back opened the stand-in again without a new Enter")
	}
}

func TestTheStandInsBackKeyReturnsToTheNextWait(t *testing.T) {
	m := standIn(t, 44)
	want := m.nextAfter(paneNamedIn(t, m, "flint-auth"))
	next, _ := m.Update(uv.UnknownApcEvent(homeKey))
	nm := next.(Model)
	if nm.standIn || nm.viewOpen {
		t.Fatal("the home key left the stand-in open")
	}
	got, _ := nm.selectedSession()
	if w, _ := want.selectedSession(); got.PID != w.PID || got.Name == "flint-auth" {
		t.Errorf("selected %s, want the next wait %s", got.Name, w.Name)
	}
}

func TestTheStandInForwardsKeysToItsPane(t *testing.T) {
	m := standIn(t, 44)
	f := m.conn.(*fakeConn)
	pane := paneNamedIn(t, m, "flint-auth")
	m = pressKeys(t, m, "2")
	got := keyActs(f)
	if len(got) != 1 || got[0].RecID != proto.KeysPrefix+pane || got[0].Text != proto.LitPrefix+"2" {
		t.Fatalf("sent %+v, want one keys:%s of 2", got, pane)
	}
	m = pressKeys(t, m, "ctrl+a", "m")
	if m.standIn || len(keyActs(f)) != 1 {
		t.Errorf("ctrl+a m: stand-in %v, sent %+v", m.standIn, keyActs(f))
	}
}

// paneNamedIn is the pane of the session named name.
func paneNamedIn(t *testing.T, m Model, name string) string {
	t.Helper()
	for _, s := range m.snap.Sessions {
		if s.Name == name {
			return s.Pane
		}
	}
	t.Fatalf("no session %s", name)
	return ""
}

// The line is longer than a phone is wide: it takes the hair's row, all of it.
func TestTheLineReadsWholeOnAPhone(t *testing.T) {
	m := enterOnPanes(t, 44)
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	last := strings.TrimSpace(lines[len(lines)-2]) + " " + strings.TrimSpace(lines[len(lines)-1])
	if last != demoLine || strings.TrimSpace(lines[len(lines)-1]) != "Install matchblox to do that." {
		t.Errorf("the last two rows read %q", last)
	}
	m = enterOnPanes(t, 100)
	lines = strings.Split(ansi.Strip(m.render()), "\n")
	if got := strings.TrimSpace(lines[len(lines)-1]); got != demoLine {
		t.Errorf("wide: the key line reads %q", got)
	}
	if !strings.HasPrefix(lines[len(lines)-2], "─") {
		t.Errorf("wide: the hair went: %q", lines[len(lines)-2])
	}
}

// enterOnPanes is Enter on the demo's Panes tab, w columns wide: a key that
// still leaves the demo.
func enterOnPanes(t *testing.T, w int) Model {
	t.Helper()
	m := demoConsole(t, w, Options{})
	m.tab = tabPanes
	return pressKeys(t, m, "enter")
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

	next, cmd := key(m, "ctrl+o")
	m = next.(Model)
	if cmd == nil || m.pending != nil || m.flash == demoLine {
		t.Fatalf("ctrl+o: pending %+v, flash %q, cmd %v: want the attach at once", m.pending, m.flash, cmd != nil)
	}
	next.Update(cmd())
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

// Two fast keys reach the agent in the order pressed: the next goes only
// after the service answers the one before (#342).
func TestTheStandInSendsOneKeyAtATime(t *testing.T) {
	m := standIn(t, 44)
	f := m.conn.(*fakeConn)
	pane := paneNamedIn(t, m, "flint-auth")
	m = pressKeys(t, m, "down", "enter")
	if got := keyActs(f); len(got) != 1 || got[0].Text != "Down" {
		t.Fatalf("sent %+v before an answer, want Down alone", got)
	}
	next, cmd := m.Update(resultMsg{ActID: keyActID + pane})
	drain(cmd)
	got := keyActs(f)
	if len(got) != 2 || got[1].Text != "Enter" {
		t.Fatalf("after Down's answer sent %+v, want Down then Enter", got)
	}
	next, cmd = next.Update(resultMsg{ActID: keyActID + pane})
	drain(cmd)
	if len(keyActs(f)) != 2 {
		t.Errorf("Enter's answer sent %+v", keyActs(f))
	}
	if m := pressKeys(t, next.(Model), "2"); len(keyActs(f)) != 3 || !m.keysOut.busy {
		t.Errorf("a key after the queue emptied: sent %+v", keyActs(f))
	}
}
