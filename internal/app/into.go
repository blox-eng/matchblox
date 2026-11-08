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
// it opens the chat.
func (m Model) goInto(host, pane string) (tea.Model, tea.Cmd) {
	if m.opt.Demo != "" {
		return m.chatOn(pane), nil
	}
	host = cmp.Or(host, m.opt.Host)
	return m.runInto(m.forHost(host, m.jumpOn(host, pane)))
}

// runInto runs the jump at once when it moves this machine's tmux, after
// the app hears which pane, and whose, it goes to. Another host's ssh jump,
// or a refusal, waits for Enter, as every act of a host.
func (m Model) runInto(a *action) (tea.Model, tea.Cmd) {
	if a.nav && !a.remote {
		next, jump := m.confirm(*a)
		if jump == nil {
			return next, nil
		}
		return next, tea.Sequence(sayAgent(keycmd.PaneSignal(m.agentOfPane(a.pane), a.pane), m.opt), jump)
	}
	m.pending, m.previewTop = a, 0
	return m, nil
}

// agentOfPane is the agent signal's id of the home session in pane.
func (m Model) agentOfPane(pane string) string {
	i := slices.IndexFunc(m.all.sessions, func(s sample.Session) bool { return s.Host == m.opt.Host && s.Pane == pane })
	if i < 0 {
		return keycmd.OtherAgent
	}
	if id := keycmd.AgentOf(m.all.sessions[i].Agent); len(keycmd.ForAgent(id)) > 0 {
		return id
	}
	return keycmd.OtherAgent
}

// intoCmd runs a jump of this machine's tmux off the update loop. A jump
// that names its client moves that client alone, and marks the pane with
// its agent first; a client that left is refused, never another moved.
func (m Model) intoCmd(a action) tea.Cmd {
	run := m.opt.Run
	if a.tty == "" {
		return func() tea.Msg { return runNav(a, run) }
	}
	attached, list, consolePane, mark := m.clients(), m.clientList(), m.opt.Pane, remote.AgentMarkArgv(a.pane, m.agentOfPane(a.pane))
	return func() tea.Msg {
		focus := remote.FocusArgv(a.tty, a.pane)
		cmd := "tmux switch-client -c " + a.tty + " -t " + a.pane
		ttys, err := attached()
		switch {
		case err != nil:
			return ranMsg{cmd: cmd, err: err}
		case !slices.Contains(ttys, a.tty):
			return ranMsg{cmd: cmd, err: errNotAttached}
		case len(a.steps) != 1 || !remote.NavAllowed(a.steps[0]) || !remote.FocusAllowed(focus):
			return ranMsg{cmd: cmd, err: errNavRefused}
		}
		if remote.AgentMarkAllowed(mark) {
			_ = run(mark) // the mark only names the agent: the jump goes without it
		}
		if err := run(focus); err != nil {
			return ranMsg{cmd: cmd, err: err}
		}
		return ranMsg{cmd: cmd, jumped: a.pane, tty: a.tty, watched: watchedBesides(list, consolePane, a.tty)}
	}
}

// jumpKey is ctrl+o: into the selected session's pane. The demo leaves it to
// say what it cannot do.
func (m Model) jumpKey() (tea.Model, tea.Cmd) {
	if m.opt.Demo != "" {
		m.flash = m.opt.Demo
		return m, nil
	}
	return m.intoSelected()
}

// goIntoAsking is goInto for a tap, which no key hold has asked for: on a
// host the stoker drives, the take-over question comes first.
func (m Model) goIntoAsking(host, pane string) (tea.Model, tea.Cmd) {
	if h := cmp.Or(host, m.opt.Host); m.opt.Demo == "" && m.takeOverOn(h) {
		m.held, m.heldOn, m.flash = heldInto(m.askingTTY(), host, pane), h, ""
		return m, nil
	}
	return m.goInto(host, pane)
}

// heldInto is goInto for a key the take-over question held: the answer
// moves the client that asked, not the one that answered.
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
