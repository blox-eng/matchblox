package app

import (
	"cmp"

	tea "charm.land/bubbletea/v2"
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

// runInto runs the jump at once when it moves this machine's tmux. Another
// host's ssh jump, or a refusal, waits for Enter, as every act of a host.
func (m Model) runInto(a *action) (tea.Model, tea.Cmd) {
	if a.nav && !a.remote {
		return m.confirm(*a)
	}
	m.pending, m.previewTop = a, 0
	return m, nil
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
	return m.goInto(s.Host, s.Pane)
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
