package app

import (
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
)

// forHost runs the console's own steps on the host it reaches: a tmux move
// attaches to the host's tmux, a door's command runs there. The inner argv
// is checked here, before ssh wraps it; a step that fails the check stays
// as it was, and the local check refuses it on confirm.
func (m Model) forHost(a *action) *action {
	host := m.opt.Host
	if a == nil || host == "" || a.remote {
		return a
	}
	b := *a
	switch {
	case a.nav:
		for _, s := range a.steps {
			if !navAllowed(s) {
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
		b.say = a.label + ": on " + host + ", in this terminal"
	default:
		return a
	}
	b.remote = true
	return &b
}

// installAction installs matchblox on a host that has none, or an older
// one. It waits for a typed y: it runs a script there.
func (m Model) installAction() *action {
	host := m.opt.Host
	if host == "" || !(m.missing || (m.mismatch && m.host.Version < proto.Version)) {
		return nil
	}
	argv := remote.InstallArgv(host)
	return &action{label: "install", say: shellLine(argv), steps: [][]string{argv}, destructive: true, remote: true, install: true}
}

type installedMsg struct{ err error }

// onHost gives this terminal to a step that runs on the host.
func (m Model) onHost(a action) (tea.Model, tea.Cmd) {
	argv := a.steps[0]
	m.flash = "running: " + shellLine(argv)
	return m, m.exec(argv, func(err error) tea.Msg {
		switch {
		case a.install:
			return installedMsg{err}
		case a.door != "":
			return doorRanMsg{door: a.door, cmd: shellLine(argv), err: err}
		}
		return ranMsg{shellLine(argv), err}
	})
}

func (m Model) exec(argv []string, done func(error) tea.Msg) tea.Cmd {
	if m.opt.Exec != nil {
		return m.opt.Exec(argv, done)
	}
	return tea.ExecProcess(exec.Command(argv[0], argv[1:]...), done) //nolint:gosec // ssh argv built by package remote after the inner check
}

// installed connects again after the install.
func (m Model) installed(msg installedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.flash = "the install failed: " + msg.err.Error() + "; ⏎ shows it again"
		return m, nil
	}
	m.missing, m.mismatch = false, false
	m.flash = "installed on " + m.opt.Host + ", connecting…"
	if m.conn != nil {
		_ = m.conn.Close()
	}
	if m.opt.Redial == nil {
		return m, nil
	}
	m.lost, m.redialing = true, true
	return m, func() tea.Msg { return redialMsg{} }
}

// missingText is the screen of a host without matchblox: why, and the
// command that fixes it.
func (m Model) missingText(w int) string {
	a := m.installAction()
	lines := []string{
		fit(" "+m.st.text.Render("matchblox is not installed on "+m.opt.Host), w),
		"",
		fit(" "+m.st.label.Render("install it: ")+m.st.text.Render(a.say), w),
		"",
		m.installKeys(w),
	}
	return strings.Join(lines, "\n")
}

// installKeys is the line under the install command: its keys, its
// confirm, or what happened.
func (m Model) installKeys(w int) string {
	st := m.st
	switch {
	case m.pending != nil:
		return fit(" "+st.neg.Render("y")+st.muted.Render(" runs it  any other key cancels"), w)
	case m.flash != "":
		return fit(" "+st.text.Render(m.flash), w)
	case m.installAction() != nil:
		return fit(" "+st.muted.Render("⏎ install  q quit"), w)
	}
	return fit(" "+st.muted.Render("q quit"), w)
}
