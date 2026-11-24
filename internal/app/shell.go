package app

import (
	"cmp"
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/sample"
)

// The desk shell under an agent: t in the list opens a real tmux pane in
// the agent's own window (the one ⌃o reaches), in the session's folder.
// A second t selects it (mode SHELL); in the shell, letters type; ctrl+q
// gives the keys back to the list. The pane carries @matchblox-shell, so
// the sampler and a restart of the console both show ▣.

// shellOf is the pane id of name's open desk shell, or "".
func (m Model) shellOf(name string) string {
	if m.shells != nil {
		return m.shells[name]
	}
	return ""
}

// hasShell: the session has a desk shell open (from the sample or from
// this console's own look at the panes).
func (m Model) hasShell(s sample.Session) bool {
	return s.OpenShell || m.shellOf(s.Name) != ""
}

// openShell is t in the list: open a shell under the chosen agent, or
// select it when one is already open.
func (m Model) openShell(s sample.Session, ok bool) (tea.Model, tea.Cmd) {
	if !ok {
		return m, m.notice("no session for a shell · " + m.keys.main.New.Help().Key + " starts one")
	}
	name := m.cardName(s)
	if s.Cwd == "" {
		return m, m.notice("not opened: " + name + " has no folder")
	}
	host := cmp.Or(s.Host, m.opt.Host)
	if host != "" && host != m.opt.Host {
		if m.stale(s.Host) || m.cannotReach(s.Host) != "" {
			return m, m.notice("not opened: " + m.nameOf(s.Host) + " is not reached")
		}
		return m, m.notice("not opened: " + name + " is on " + m.nameOf(s.Host))
	}
	run := m.tmuxRun()
	if run == nil || s.Pane == "" {
		return m, m.notice("not opened: " + name + " has no pane")
	}
	if id := m.shellOf(s.Name); id != "" {
		if _, err := run("display", "-p", "-t", id, "#{pane_id}"); err != nil {
			m.forgetShell(s.Name)
		} else {
			return m.selectShell(s, id)
		}
	}
	if found := m.findShell(run, s.Name); found != "" {
		m.rememberShell(s.Name, found)
		return m.selectShell(s, found)
	}
	out, err := run("split-window", "-v", "-l", "40%", "-d", "-c", s.Cwd, "-t", s.Pane, "-P", "-F", "#{pane_id}")
	if err != nil {
		return m, m.notice("not opened: " + err.Error())
	}
	id := strings.TrimSpace(out)
	if id == "" {
		return m, m.notice("not opened: tmux made no pane")
	}
	_, _ = run("set-option", "-p", "-t", id, sample.ShellOption, s.Name)
	m.styleShellWindow(run, s.Pane, s.Name)
	m.rememberShell(s.Name, id)
	return m, nil
}

// selectShell gives the keys to the shell pane (mode SHELL).
func (m Model) selectShell(s sample.Session, id string) (tea.Model, tea.Cmd) {
	if run := m.tmuxRun(); run != nil {
		_, _ = run("select-pane", "-t", id)
		m.styleShellWindow(run, s.Pane, s.Name)
	}
	m.agent.keys, m.agent.shell = true, true
	return m, m.grantKeys()
}

// styleShellWindow sets the agent's window border: the shell line in
// faint, the active pane's border in accent.
func (m Model) styleShellWindow(run func(...string) (string, error), agentPane, name string) {
	faint, accent := themeHex(m.st.theme.Faint), themeHex(m.st.theme.Accent)
	format := fmt.Sprintf("#[fg=%s]shell · %s · t switches", faint, name)
	_, _ = run("set-option", "-w", "-t", agentPane, "pane-border-status", "top")
	_, _ = run("set-option", "-w", "-t", agentPane, "pane-border-format", format)
	_, _ = run("set-option", "-w", "-t", agentPane, "pane-active-border-style", "fg="+accent)
}

// themeHex is c as #RRGGBB for a tmux style.
func themeHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}

// rememberShell notes an open shell for name.
func (m *Model) rememberShell(name, id string) {
	if m.shells == nil {
		m.shells = map[string]string{}
	}
	m.shells[name] = id
}

// forgetShell drops name's shell mark.
func (m *Model) forgetShell(name string) {
	if m.shells != nil {
		delete(m.shells, name)
	}
	if m.agent.shell && m.agent.name == name {
		m.agent.shell, m.agent.keys = false, false
	}
}

// findShell is the pane id of an existing @matchblox-shell for name.
func (m Model) findShell(run func(...string) (string, error), name string) string {
	out, err := run("list-panes", "-a", "-F", "#{pane_id}\t#{"+sample.ShellOption+"}")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		id, shell, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if ok && shell == name {
			return id
		}
	}
	return ""
}

// syncShells looks at the panes for @matchblox-shell: remembers new ones,
// and says shell closed when one this console knew is gone.
func (m *Model) syncShells() tea.Cmd {
	run := m.tmuxRun()
	if run == nil {
		return nil
	}
	out, err := run("list-panes", "-a", "-F", "#{pane_id}\t#{"+sample.ShellOption+"}")
	if err != nil {
		return nil
	}
	found := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		id, shell, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if ok && shell != "" {
			found[shell] = id
		}
	}
	var closed []string
	for name := range m.shells {
		if found[name] == "" {
			closed = append(closed, name)
		}
	}
	m.shells = found
	for _, name := range closed {
		if m.agent.shell && m.agent.name == name {
			m.agent.shell, m.agent.keys = false, false
		}
	}
	if len(closed) == 0 {
		return nil
	}
	return m.notice("shell closed")
}
