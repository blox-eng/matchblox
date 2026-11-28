package app

import (
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/mux"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/sample"
)

// shellMsg is the shell pane opened under an agent, or why it did not open.
// host is the session's host: pane ids repeat across hosts.
type shellMsg struct {
	host, agent, pane string
	err               error
}

// shellLookupMsg is the answer to one look for a host's shell panes: the
// agent panes it asked about, and the shell pane each one has.
type shellLookupMsg struct {
	host   string
	asked  []string
	shells map[string]string // agent pane to shell pane
	ok     bool              // the host answered
}

// localShells tells if the list keeps a shell pane on this machine's tmux.
func (m Model) localShells() bool {
	for key := range m.shells {
		if host, _, _ := strings.Cut(key, "\n"); host == "" || host == m.opt.Host {
			return true
		}
	}
	return false
}

// shellLives tells if the shell pane id is on this tmux as far as the last
// poll knows. Before the first read it is taken as there.
func (m Model) shellLives(id string) bool { return m.shellLive == nil || m.shellLive[id] }

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
		return m, m.notice("not opened: the shell needs the agent's tmux · " + m.keys.side.Enter.Help().Key + " goes into the session")
	}
	// A peer's pane id is another machine's. The console's tmux must not
	// be asked: the same id here is a different pane.
	if !peer && !livePane(run, s.Pane) {
		return m, m.notice("not opened: " + s.Name + "'s pane is gone")
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
	inner := append(mux.Command(""), args...)
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

// rememberShells looks for the panes that already carry @matchblox_shell,
// as a command: one tmux call for the host, never one for each session and
// never inside Update. Desk start and each list refresh call it. A pane is
// asked once, also when the host did not answer: a later t still finds its
// own shell (findShellPane). Nil when nothing is left to ask.
func (m *Model) rememberShells(host string, sessions []sample.Session) tea.Cmd {
	if !m.deskFrame || m.shellAsking[host] {
		return nil
	}
	peer := host != "" && host != m.opt.Host
	if peer && (m.watchConn(host) == nil || m.helloOf(host).Tmux == "") {
		return nil
	}
	run, _ := m.shellIO(host)
	if run == nil {
		return nil
	}
	var asked []string
	for _, s := range sessions {
		if s.Pane != "" && !m.shellSeen[shellKey(host, s.Pane)] {
			asked = append(asked, s.Pane)
		}
	}
	if len(asked) == 0 {
		return nil
	}
	if m.shellAsking == nil {
		m.shellAsking = map[string]bool{}
	}
	m.shellAsking[host] = true
	return func() tea.Msg {
		shells, ok := shellPanes(run)
		return shellLookupMsg{host: host, asked: asked, shells: shells, ok: ok}
	}
}

// shellPanes is each agent pane's shell pane on a host, from one
// list-panes of every pane that names its agent. ok is false when the host
// did not answer.
func shellPanes(run func(args ...string) (string, error)) (shells map[string]string, ok bool) {
	out, err := run("list-panes", "-a", "-F", "#{pane_id} #{"+sample.ShellAgentOption+"} #{"+sample.ShellOption+"}")
	if err != nil {
		return nil, false
	}
	shells = map[string]string{}
	for line := range strings.SplitSeq(out, "\n") {
		f := strings.Split(strings.TrimSpace(line), " ")
		if len(f) < 3 || f[2] == "" || !paneID.MatchString(f[0]) || !paneID.MatchString(f[1]) || f[0] == f[1] {
			continue
		}
		shells[f[1]] = f[0]
	}
	return shells, true
}

// shellLooked takes the answer of one look: the shells it found, and every
// pane it asked about is marked asked.
func (m *Model) shellLooked(msg shellLookupMsg) {
	delete(m.shellAsking, msg.host)
	if m.shellSeen == nil {
		m.shellSeen = map[string]bool{}
	}
	host := msg.host
	for _, pane := range msg.asked {
		key := shellKey(msg.host, pane)
		m.shellSeen[key] = true
		if id := msg.shells[pane]; msg.ok && id != "" {
			if m.shells == nil {
				m.shells = map[string]string{}
			}
			m.shells[key] = id
			if m.shellLive != nil && (host == "" || host == m.opt.Host) {
				m.shellLive[id] = true
			}
		}
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
	step := shellFlipArgv(table, console != "")
	if len(step) > 0 && step[0] == "tmux" {
		step = step[1:]
	}
	_, err := run(step...)
	return err
}

// shellFlipArgv binds t on the shell's key table: t focuses the agent pane.
func shellFlipArgv(table string, writeDesk bool) []string {
	act := "run-shell -C 'select-pane -t #{" + sample.ShellAgentOption + "}'"
	if writeDesk {
		act = "run-shell -C 'select-pane -t #{" + sample.ShellAgentOption + "} ; set-option -w -t #{" + sample.ShellDeskOption + "} " + sample.ModeOption + " AGENT'"
	}
	return append(mux.Command(""), "bind-key", "-T", table, "t", "if-shell", "-F", "#{"+sample.ShellOption+"}", act, "send-keys -l t")
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
