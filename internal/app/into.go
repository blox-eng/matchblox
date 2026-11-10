package app

import (
	"cmp"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/sample"
)

// noPane is what a session outside tmux says to every way in.
func noPane(name string) string { return name + " runs outside tmux: no pane to go into" }

// goInto takes the builder into the pane of an agent. The demo has no tmux:
// it opens the stand-in for the pane.
func (m Model) goInto(host, pane string) (tea.Model, tea.Cmd) {
	if m.opt.Demo != "" {
		return m.openStandIn(pane), nil
	}
	host = cmp.Or(host, m.opt.Host)
	return m.runInto(m.forHost(host, m.jumpOn(host, pane)))
}

// runInto runs the jump at once when it moves this machine's tmux, after
// the app hears which pane, and whose, it goes to. Another host's ssh jump,
// or a refusal, waits for Enter, as every act of a host.
func (m Model) runInto(a *action) (tea.Model, tea.Cmd) {
	if a.nav && !a.remote {
		return m.confirm(*a)
	}
	m.pending, m.previewTop = a, 0
	return m, nil
}

// signalled puts the pane signal before a jump of this machine's tmux: the
// app hears which pane, and whose, the jump goes to. Only a marked key, an
// app's, gets it: the signal reaches every screen on the console, and a
// phone would take the desk's pane as its own.
func (m Model) signalled(a action, jump tea.Cmd) tea.Cmd {
	if a.pane == "" || a.tty == "" {
		return jump
	}
	return tea.Sequence(sayAgent(keycmd.PaneSignal(m.agentIn(a), a.pane), m.opt), jump)
}

// agentIn is the agent signal's id of the pane a jumps into: the agent a
// switch started there, else the home session's in it.
func (m Model) agentIn(a action) string {
	if a.paneAgent != "" {
		return signalAgent(a.paneAgent)
	}
	return m.agentOfPane(a.pane)
}

// agentOfPane is the agent signal's id of the home session in pane.
func (m Model) agentOfPane(pane string) string {
	i := slices.IndexFunc(m.all.sessions, func(s sample.Session) bool { return s.Host == m.opt.Host && s.Pane == pane })
	if i < 0 {
		return keycmd.OtherAgent
	}
	return signalAgent(m.all.sessions[i].Agent)
}

// signalAgent is the agent signal's id of agent: other for one with no
// command keys.
func signalAgent(agent string) string {
	if id := keycmd.AgentOf(agent); len(keycmd.ForAgent(id)) > 0 {
		return id
	}
	return keycmd.OtherAgent
}

// paneMark is the argv that names pane's agent for the command keys, or
// takes an old name off when agent has none; nil when neither is allowed.
func paneMark(pane, agent string) []string {
	if mark := remote.AgentMarkArgv(pane, agent); remote.AgentMarkAllowed(mark) {
		return mark
	}
	if unmark := remote.AgentUnmarkArgv(pane); remote.AgentUnmarkAllowed(unmark) {
		return unmark
	}
	return nil
}

// intoCmd runs a jump of this machine's tmux off the update loop. A jump
// that names its client moves that client alone, and marks the pane with
// its agent first; a client that left is refused, never another moved.
func (m Model) intoCmd(a action) tea.Cmd {
	run := m.opt.Run
	if !a.asker || m.opt.OutsideTmux || m.opt.Pane == "" {
		return func() tea.Msg { return runNav(a, run) } // no client list to read: the client tmux has
	}
	read, mark, at := m.readClients(), paneMark(a.pane, m.agentIn(a)), m.now()
	return func() tea.Msg {
		cs, desk := read()
		if a.tty = cmp.Or(a.tty, desk); a.tty == "" || cs == nil { // nil: tmux did not list its clients
			return ranMsg{cmd: "tmux switch-client -t " + a.pane, err: errNoClient}
		}
		focus := remote.FocusArgv(a.tty, a.pane, at)
		cmd := "tmux switch-client -c " + a.tty + " -t " + a.pane
		switch {
		case !slices.ContainsFunc(cs, func(c consoleClient) bool { return c.here && c.tty == a.tty }):
			return ranMsg{cmd: cmd, err: errNotAttached} // a mark moves only a screen that shows the console
		case len(a.steps) != 1 || !remote.NavAllowed(a.steps[0]) || !remote.FocusAllowed(focus):
			return ranMsg{cmd: cmd, err: errNavRefused}
		}
		if mark != nil {
			_ = run(mark) // the mark only names the agent: the jump goes without it
		}
		if err := run(focus); err != nil {
			return ranMsg{cmd: cmd, err: err}
		}
		return ranMsg{cmd: cmd, jumped: a.pane, tty: a.tty, watched: othersHere(cs, a.tty), alone: !othersHere(cs, a.tty)}
	}
}

// goIntoAsking is goInto for a tap, which no key hold has asked for: on a
// host the stoker drives, the take-over question comes first.
func (m Model) goIntoAsking(host, pane string) (tea.Model, tea.Cmd) {
	if h := cmp.Or(host, m.opt.Host); m.opt.Demo == "" && m.takeOverOn(h) {
		m.held, m.heldOn, m.flash = heldInto(m.askedBy, host, pane), h, ""
		return m, nil
	}
	return m.goInto(host, pane)
}

// heldInto is goInto for a key the take-over question held: the answer
// moves the client that asked, not the one that answered. An unmarked key
// leaves the client to the jump's run: the desk then.
func heldInto(tty, host, pane string) func(Model) (tea.Model, tea.Cmd) {
	return func(m Model) (tea.Model, tea.Cmd) {
		m.askedBy = tty
		return m.goInto(host, pane)
	}
}

// intoSelected goes into the pane of the selected session.
func (m Model) intoSelected() (tea.Model, tea.Cmd) {
	s, ok := m.selectedSession()
	if !ok {
		return m, nil
	}
	if s.Pane == "" {
		m.flash = noPane(s.Name)
		return m, nil
	}
	return m.goIntoAsking(s.Host, s.Pane)
}

// tapTarget is the pane a second tap on the selected row goes into: a home
// session's, or a failed step's. A row of another host acts through tapAct.
func (m Model) tapTarget() (host, pane string) {
	if m.tab == tabStoker {
		pane, _ = m.stokerTarget()
		return m.opt.Host, pane
	}
	if s, ok := m.selectedSession(); ok && s.Host == m.opt.Host {
		return s.Host, s.Pane
	}
	return "", ""
}
