package app

import (
	"errors"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
)

// forHost runs the console's own steps on the host it reaches, with the
// builder's own ssh login: a tmux move attaches to the host's tmux, a
// door's command runs there. The inner argv is checked here, before ssh
// wraps it; a step that fails the check stays as it was, and the local
// check refuses it on confirm.
func (m Model) forHost(a *action) *action {
	if a == nil || m.opt.Host == "" || a.remote {
		return a
	}
	host := m.opt.Host
	b := *a
	switch {
	case a.nav:
		for _, s := range a.steps {
			if !remote.NavAllowed(s) {
				return a
			}
		}
		b.steps = [][]string{remote.NavArgv(host, a.steps)}
		b.say = shellLine(b.steps[0])
	case a.term:
		argv := a.steps[0]
		if a.door == doors.Guide {
			argv = doors.GuideArgv(false) // no tmux client of the host to open a window in
		}
		if !doors.TermAllowed(argv) {
			return a
		}
		b.steps = [][]string{remote.TermArgv(host, argv)}
		b.say = a.label + ": on " + m.opt.Host + ", in this terminal"
	default:
		return a
	}
	b.remote = true
	return &b
}

// blocking: trying again cannot help, connecting the host can.
func blocking(err error) bool {
	for _, e := range []error{remote.ErrNotConnected, remote.ErrNotInstalled, remote.ErrHostKeyUnknown, remote.ErrHostKeyChanged} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// connectAction connects the host: it installs or updates matchblox there
// and lets this console's key start it. It runs `matchblox connect` in this
// terminal after a typed y: the ssh login there is the builder's own.
func (m Model) connectAction() *action {
	host := m.opt.Host
	older := m.mismatch && m.host.Version < proto.Version
	if host == "" || (m.blocked == nil && !older) || errors.Is(m.blocked, remote.ErrHostKeyChanged) {
		return nil // a changed host key is the builder's call, never a connect
	}
	self := m.opt.Self
	if self == "" {
		self = "matchblox"
	}
	argv := []string{self, "connect", host} // it also updates an older one
	return &action{label: "connect", say: shellLine(argv), steps: [][]string{argv}, destructive: true, remote: true, connect: true}
}

type connectedMsg struct{ err error }

// onHost gives this terminal to a step that runs on the host.
func (m Model) onHost(a action) (tea.Model, tea.Cmd) {
	argv := a.steps[0]
	m.flash = "running: " + shellLine(argv)
	return m, m.exec(argv, func(err error) tea.Msg {
		switch {
		case a.connect:
			return connectedMsg{err}
		case a.door != "":
			return doorRanMsg{door: a.door, cmd: shellLine(argv), err: err}
		}
		return ranMsg{cmd: shellLine(argv), err: err}
	})
}

func (m Model) exec(argv []string, done func(error) tea.Msg) tea.Cmd {
	if m.opt.Exec != nil {
		return m.opt.Exec(argv, done)
	}
	return tea.ExecProcess(exec.Command(argv[0], argv[1:]...), done) //nolint:gosec // argv built here or by package remote after the inner check
}

// tryAgain dials again, after the builder fixed what the console showed.
func (m Model) tryAgain() (tea.Model, tea.Cmd) {
	m.blocked, m.flash = nil, "trying "+m.opt.Host+" again…"
	if m.opt.Redial == nil {
		return m, nil
	}
	m.lost, m.redialing = true, true
	gen := m.opt.Gen
	return m, func() tea.Msg { return redialMsg{gen} }
}

// connected dials again after `matchblox connect`.
func (m Model) connected(msg connectedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.flash = "not connected: " + msg.err.Error() + "; ⏎ tries again"
		return m, nil
	}
	m.blocked, m.mismatch = nil, false
	m.flash = "connected " + m.opt.Host + ", opening…"
	if m.conn != nil {
		_ = m.conn.Close()
	}
	if m.opt.Redial == nil {
		return m, nil
	}
	m.lost, m.redialing = true, true
	gen := m.opt.Gen
	return m, func() tea.Msg { return redialMsg{gen} }
}

// blockedText is the screen of a host the console cannot reach: why, and
// the command that fixes it.
func (m Model) blockedText(w int) string {
	host := m.opt.Host
	why, note := host+" is not connected to this console",
		"It runs ssh with your own login once. It installs matchblox there if needed, and lets this console's key start only matchblox."
	switch {
	case errors.Is(m.blocked, remote.ErrNotInstalled):
		why = "matchblox is not installed on " + host
	case errors.Is(m.blocked, remote.ErrHostKeyUnknown):
		why = host + "'s host key is not known yet"
		note = "ssh shows the host key and asks you to check it. Then it connects as above."
	case errors.Is(m.blocked, remote.ErrHostKeyChanged):
		why = host + "'s host key changed: this can be an attack"
		note = "Check the new key with the owner of " + host + ". Only if you trust it: ssh-keygen -R " + host + ", then ⏎."
	}
	lines := []string{fit(" "+m.st.text.Render(why), w), ""}
	if a := m.connectAction(); a != nil {
		lines = append(lines, fit(" "+m.st.label.Render("connect it: ")+m.st.text.Render(a.say), w))
	}
	for _, l := range wrap(note, w-2, m.st.muted.Render) {
		lines = append(lines, " "+l)
	}
	return strings.Join(append(lines, "", m.connectKeys(w)), "\n")
}

// connectKeys is the line under the connect command: its keys, its
// confirm, or what happened.
func (m Model) connectKeys(w int) string {
	st := m.st
	switch {
	case m.pending != nil:
		return fit(" "+st.neg.Render("y")+st.muted.Render(" runs it  any other key cancels"), w)
	case m.flash != "":
		return fit(" "+st.text.Render(m.flash), w)
	}
	keys := "q quit"
	if m.opt.Layered {
		keys = "esc hosts  " + keys
	}
	switch {
	case m.connectAction() != nil:
		keys = "⏎ connect  " + keys
	case m.blocked != nil:
		keys = "⏎ try again  " + keys
	}
	return fit(" "+st.muted.Render(keys), w)
}
