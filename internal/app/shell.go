package app

import (
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/mux"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/sample"
)

// shellMsg is the shell pane opened under an agent, or why it did not open.
// host is the session's host: pane ids repeat across hosts.
type shellMsg struct {
	host, agent, pane string
	err               error
}

// shellKey is the map key of host's pane. Pane ids repeat across hosts.
func shellKey(host, pane string) string { return host + "\n" + pane }

// askShell is the list's shell act. t runs it, and so does
// ? then shell then enter. It opens a shell under the agent, or focuses
// the one this session already has.
func (m Model) askShell() (tea.Model, tea.Cmd) {
	m.shellAsks++
	s, ok := m.sideChosen()
	if !ok {
		return m, nil
	}
	peer := s.Host != "" && s.Host != m.opt.Host
	if peer && m.watchConn(s.Host) == nil {
		return m, m.notice("not opened: " + m.nameOf(s.Host) + " is not reached")
	}
	if s.Cwd == "" {
		return m, m.notice("not opened: " + s.Name + " has no folder")
	}
	run, _ := m.shellIO(s.Host)
	if run == nil {
		return m, nil
	}
	// A peer's pane id is another machine's. The console's tmux must not
	// be asked: the same id here is a different pane.
	if !peer && !livePane(run, s.Pane) {
		return m, nil
	}
	return m, m.openShell(s)
}

// hostRun runs argv on another machine and returns its output. A test
// replaces it to record a peer's tmux.
var hostRun = func(argv []string) (string, error) {
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // argv is the peer's tmux over ssh, built here
	out, err := cmd.Output()
	return string(out), err
}

// shellArgv is the shell to run: $SHELL when it is set, else /bin/sh.
// It is not a login shell.
func shellArgv() []string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return []string{sh}
	}
	return []string{"/bin/sh"}
}

// shellIO is the tmux of host: the console's own, or ssh to a peer. The
// mux splits on that same tmux.
func (m Model) shellIO(host string) (func(args ...string) (string, error), mux.Mux) {
	if host == "" || host == m.opt.Host {
		return m.tmuxRun(), m.multiplexer()
	}
	run := func(args ...string) (string, error) {
		return hostRun(peerShellArgv(host, args))
	}
	mx := mux.Tmux{Run: func(argv []string) (string, error) {
		if len(argv) > 0 && argv[0] == "tmux" {
			argv = argv[1:]
		}
		return run(argv...)
	}}
	return run, mx
}

// peerShellArgv is ssh that runs tmux args on host, as the builder's own
// login, and prints the tmux output.
func peerShellArgv(host string, args []string) []string {
	inner := append([]string{"tmux"}, args...)
	script := remote.Quote(append([]string{"/bin/sh", "-c", `PATH="` + remote.HostPath + `:$PATH"; export PATH; exec "$@"`, "sh"}, inner...))
	argv := append([]string{"ssh", "-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8"}, remote.NoForward()...)
	return append(argv, "--", host, script)
}

func (m Model) openShell(s sample.Session) tea.Cmd {
	run, mx := m.shellIO(s.Host)
	local := m.tmuxRun()
	host, agent, cwd, console, beside := s.Host, s.Pane, s.Cwd, m.opt.Pane, m.beside
	peer := host != "" && host != m.opt.Host
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
		// The desk pane id is this machine's. A peer must not store it:
		// t would write @matchblox_mode on whichever pane there shares it.
		desk := console
		if peer {
			desk = ""
		}
		if err := markShell(run, id, agent, desk, line, table); err != nil {
			return shellMsg{err: err}
		}
		focusShell(run, id, beside, console, local)
		return shellMsg{host: host, agent: agent, pane: id}
	}
}

// findShellPane is the shell matchblox opened in the agent's window, or "".
func findShellPane(run func(args ...string) (string, error), agent string) string {
	id, _ := notedShell(run, agent)
	return id
}

// notedShell is that pane, and whether tmux answered. A miss is "" and true.
func notedShell(run func(args ...string) (string, error), agent string) (string, bool) {
	out, err := run("list-panes", "-t", agent, "-F", "#{pane_id} #{"+sample.ShellOption+"}")
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(out, "\n") {
		id, mark, _ := strings.Cut(strings.TrimSpace(line), " ")
		if id != "" && id != agent && mark != "" && paneID.MatchString(id) {
			return id, true
		}
	}
	return "", true
}

// rememberShells fills the shell map from panes that already carry
// @matchblox_shell. Desk start and each list refresh call it. A pane is
// asked once: a later t still focuses the pane this found.
func (m *Model) rememberShells(host string, sessions []sample.Session) {
	if !m.deskFrame {
		return
	}
	peer := host != "" && host != m.opt.Host
	if peer && (m.watchConn(host) == nil || m.helloOf(host).Tmux == "") {
		return
	}
	run, _ := m.shellIO(host)
	if run == nil {
		return
	}
	if m.shellSeen == nil {
		m.shellSeen = map[string]bool{}
	}
	for _, s := range sessions {
		if s.Pane == "" {
			continue
		}
		key := shellKey(host, s.Pane)
		if m.shellSeen[key] {
			continue
		}
		id, ok := notedShell(run, s.Pane)
		if !ok {
			continue
		}
		m.shellSeen[key] = true
		if id == "" {
			continue
		}
		if m.shells == nil {
			m.shells = map[string]string{}
		}
		m.shells[key] = id
	}
}

// markShell names the pane as matchblox's shell, draws the divider, and
// binds t on that pane's view only.
func markShell(run func(args ...string) (string, error), shell, agent, console, line, table string) error {
	opts := [][2]string{
		{sample.ShellOption, "1"},
		{sample.ShellAgentOption, agent},
		{sample.ShellLineOption, line},
	}
	// console is the desk pane, and only when this tmux is the desk's.
	if console != "" {
		opts = append(opts, [2]string{sample.ShellDeskOption, console})
	}
	for _, kv := range opts {
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
	step := panes.ShellFlipArgv(table, console != "")
	if len(step) > 0 && step[0] == "tmux" {
		step = step[1:]
	}
	_, err := run(step...)
	return err
}

// focusShell makes the shell the active pane of its window. select-pane
// does not change which window a session shows, so a client attached to
// the agent's own session stays where it was. select-window -t <pane>
// would move that session. beside and the mode are the console's own tmux:
// local is that runner, and it is run when the shell is on this machine.
func focusShell(run func(args ...string) (string, error), shell, beside, console string, local func(args ...string) (string, error)) {
	_, _ = run("select-pane", "-t", shell)
	if local == nil {
		local = run
	}
	if beside != "" {
		_, _ = local("select-pane", "-t", beside)
	}
	writeMode(local, console, "SHELL")
}
