package app

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/mux"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/sample"
)

// shellMsg is the shell pane opened under an agent, or why it did not open.
type shellMsg struct {
	agent, pane string
	err         error
}

// askShell is the list's shell act. t runs it, and so does
// ? then shell then enter. It opens a shell under the agent, or focuses
// the one this session already has.
func (m Model) askShell() (tea.Model, tea.Cmd) {
	m.shellAsks++
	s, ok := m.sideChosen()
	if !ok {
		return m, nil
	}
	if s.Host != "" && s.Host != m.opt.Host && m.watchConn(s.Host) == nil {
		return m, m.notice("not opened: " + m.nameOf(s.Host) + " is not reached")
	}
	if s.Cwd == "" {
		return m, m.notice("not opened: " + s.Name + " has no folder")
	}
	run := m.tmuxRun()
	if run == nil || !livePane(run, s.Pane) {
		return m, nil
	}
	return m, m.openShell(s)
}

// shellArgv is the shell to run: $SHELL when it is set, else /bin/sh.
// It is not a login shell.
func shellArgv() []string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return []string{sh}
	}
	return []string{"/bin/sh"}
}

func (m Model) openShell(s sample.Session) tea.Cmd {
	run, mx := m.tmuxRun(), m.multiplexer()
	agent, cwd, console, beside := s.Pane, s.Cwd, m.opt.Pane, m.beside
	table := "matchblox-jump-" + m.prefix()
	argv := shellArgv()
	line := "shell · " + cwd + " · t switches"
	return func() tea.Msg {
		id := findShellPane(run, agent)
		if id == "" {
			got, err := mx.SplitIn(agent, argv, cwd)
			if err != nil {
				return shellMsg{err: err}
			}
			_, id = mux.SplitPane(got)
			if id == "" {
				id = got
			}
		}
		if err := markShell(run, id, agent, console, line, table); err != nil {
			return shellMsg{err: err}
		}
		focusShell(run, id, beside, console)
		return shellMsg{agent: agent, pane: id}
	}
}

// findShellPane is the shell matchblox opened in the agent's window, or "".
func findShellPane(run func(args ...string) (string, error), agent string) string {
	out, err := run("list-panes", "-t", agent, "-F", "#{pane_id} #{"+sample.ShellOption+"}")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		id, mark, _ := strings.Cut(strings.TrimSpace(line), " ")
		if id != "" && id != agent && mark != "" && paneID.MatchString(id) {
			return id
		}
	}
	return ""
}

// markShell names the pane as matchblox's shell, draws the divider, and
// binds t on that pane's view only.
func markShell(run func(args ...string) (string, error), shell, agent, console, line, table string) error {
	for _, kv := range [][2]string{
		{sample.ShellOption, "1"},
		{sample.ShellAgentOption, agent},
		{sample.ShellLineOption, line},
		{sample.ShellDeskOption, console},
	} {
		if _, err := run("set-option", "-p", "-t", shell, kv[0], kv[1]); err != nil {
			return err
		}
	}
	if _, err := run("set-option", "-w", "-t", agent, "pane-border-status", "top"); err != nil {
		return err
	}
	if _, err := run("set-option", "-w", "-t", agent, "pane-border-format", "#{"+sample.ShellLineOption+"}"); err != nil {
		return err
	}
	// One pane left: the divider goes with the shell.
	hook := "if-shell -F '#{==:#{window_panes},1}' 'set-option -wu pane-border-status'"
	if _, err := run("set-hook", "-w", "-t", agent, "pane-exited", hook); err != nil {
		return err
	}
	step := panes.ShellFlipArgv(table)
	if len(step) > 0 && step[0] == "tmux" {
		step = step[1:]
	}
	_, err := run(step...)
	return err
}

// focusShell gives the shell the keys inside the agent's window, then the
// desk's agent pane, and publishes SHELL.
func focusShell(run func(args ...string) (string, error), shell, beside, console string) {
	for _, step := range (mux.Tmux{}).Focus(shell) {
		if len(step) > 0 && step[0] == "tmux" {
			step = step[1:]
		}
		_, _ = run(step...)
	}
	if beside != "" {
		_, _ = run("select-pane", "-t", beside)
	}
	writeMode(run, console, "SHELL")
}
