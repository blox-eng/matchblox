package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/remote"
)

// checkTimeout ends a login check that waits: Tailscale's check mode waits
// for a browser.
const checkTimeout = 30 * time.Second

// openTimeout ends a master's open that waits: the builder types its
// password in the console.
const openTimeout = 5 * time.Minute

// loginMsg is the result of the login check of host.
type loginMsg struct {
	host   string
	login  remote.Login
	detail string
}

// checkLogin starts the check of the builder's own ssh login to the host,
// once per console, in the background (design §3.2). It changes nothing on
// either machine; an ok check leaves the master up for the jumps.
func (m Model) checkLogin() (Model, tea.Cmd) {
	if !remote.ValidHost(m.opt.Host) || m.checked {
		return m, nil
	}
	m.checked, m.checking = true, true
	// A directory that is refused: the check goes without a master, and
	// the console says why once. Inside tmux the jumps are refused (see
	// jumpOnHost); outside, they log in each time.
	dir, err := m.masterDir()
	switch {
	case err != nil && m.opt.OutsideTmux:
		m.flash = "jumps to " + m.opt.Host + " ask your login each time: " + masterWhy(err)
	case err != nil:
		m.flash = m.masterRefusal(err)
	case m.opt.Ask == nil && m.opt.NoAsk != "":
		m.flash = "jumps to " + m.opt.Host + " ask your login in the terminal: " + m.opt.NoAsk
	}
	host, check := m.opt.Host, m.opt.Check
	if check == nil {
		check = runCheck
	}
	argv := remote.CheckArgv(host, dir)
	return m, func() tea.Msg {
		stderr, exit := check(argv)
		login, detail := remote.ClassifyLogin(stderr, exit)
		return loginMsg{host: host, login: login, detail: detail}
	}
}

// loggedIn takes the check's result: a login that is not ok shows its fix,
// and the jump that waited goes, except to a changed host key.
func (m Model) loggedIn(msg loginMsg) (tea.Model, tea.Cmd) {
	if msg.host != m.opt.Host {
		return m, nil
	}
	m.checking, m.login = false, msg.login
	a := m.jumpLater
	m.jumpLater = nil
	var cmd tea.Cmd
	if a != nil {
		m.flash = "" // the "checking" line
		if msg.login != remote.LoginHostKeyChanged {
			var next tea.Model
			next, cmd = m.runJump(*a)
			m = next.(Model)
		}
	}
	if note := m.loginNote(msg.login, msg.detail); note != "" {
		m.flash = note
	}
	return m, cmd
}

// jumpAfterLogin runs a jump, or keeps it until the login check ends. A
// second jump replaces the one that waits.
func (m Model) jumpAfterLogin(a action) (tea.Model, tea.Cmd) {
	switch {
	case m.checking:
		m.jumpLater = &a
		m.flash = "checking your ssh login to " + m.opt.Host + "…"
		return m, nil
	case m.login == remote.LoginHostKeyChanged:
		m.flash = m.loginNote(m.login, "")
		return m, nil
	}
	return m.runJump(a)
}

// masterMsg is the end of a master's open before a jump: up, or the stderr
// and exit of the open that failed.
type masterMsg struct {
	a      action
	gen    int
	stderr string
	exit   int
}

// runJump runs a confirmed jump. With prompts in the console, the master
// opens first, while the builder is still on the console: inside tmux the
// jump switches the client at once, and a prompt of its own ssh would show
// where nobody looks (design §3.4). The jump's ssh then never asks.
func (m Model) runJump(a action) (tea.Model, tea.Cmd) {
	if a.master == "" {
		return m.onHost(a)
	}
	host := m.opt.Host
	if m.opening {
		m.flash = "the login to " + host + " runs: answer it, or " + m.keys.ask.Cancel.Help().Key + " to cancel it"
		return m, nil
	}
	m.opening, m.askCancelled = true, false
	m.flash = "logging in to " + host + "…"
	ssh, env, gen := m.ssh(), m.opt.Ask.Env(host), m.opt.Gen
	up, open := remote.MasterUpArgv(host, a.master), remote.OpenArgv(host, a.master)
	return m, func() tea.Msg {
		if _, exit := ssh(up, nil); exit == 0 {
			return masterMsg{a: a, gen: gen}
		}
		stderr, exit := ssh(open, env)
		return masterMsg{a: a, gen: gen, stderr: stderr, exit: exit}
	}
}

// masterOpened runs the jump on a master that is up, or says why it did
// not run and the fix.
func (m Model) masterOpened(msg masterMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.opt.Gen || m.quitting {
		return m, nil
	}
	m.opening = false
	if msg.exit == 0 {
		m.flash = ""
		return m.onHost(msg.a)
	}
	host := m.opt.Host
	login, detail := remote.ClassifyLogin(msg.stderr, msg.exit)
	switch {
	case m.askCancelled:
		m.flash = "cancelled: the login to " + host + ", and the jump did not run"
	case login == remote.LoginPassword:
		m.flash = "the jump did not run: " + host + " refused your login: jump again to type it again, or put your key there (ssh-copy-id " + host + ")"
	case login == remote.LoginOK: // an exit that says nothing
		m.flash = "the jump did not run: ssh " + host + " exited " + strconv.Itoa(msg.exit) + "; check that ssh " + host + " works in a terminal"
	default:
		m.flash = "the jump did not run: " + m.loginNote(login, detail)
	}
	return m, nil
}

// ssh runs an ssh of the builder's own login that the console starts.
func (m Model) ssh() func(argv, env []string) (string, int) {
	if m.opt.SSH != nil {
		return m.opt.SSH
	}
	return func(argv, env []string) (string, int) { return runSSH(argv, env, openTimeout) }
}

// loginNote is one line that names a login result and its fix; "" for ok.
// Until the console asks the password itself, the jump's ssh asks it in
// the terminal.
func (m Model) loginNote(l remote.Login, detail string) string {
	host := m.opt.Host
	switch l {
	case remote.LoginPassword:
		if m.opt.Ask != nil {
			return host + " asks your password: the console asks it at a jump, once in 10 minutes; put your key there to skip it (ssh-copy-id " + host + ")"
		}
		return host + " asks your password at each jump: put your key there (ssh-copy-id " + host + ")"
	case remote.LoginTailscaleCheck:
		if detail == "" {
			return "Tailscale SSH on " + host + " asks for a check in a browser"
		}
		return "Tailscale SSH on " + host + " asks for a check: open " + detail
	case remote.LoginTailscaleDenied:
		return "Tailscale SSH on " + host + " refuses you: " + detail + "; see your tailnet's SSH rules (docs/remote.md)"
	case remote.LoginHostKeyUnknown:
		if m.opt.Ask != nil {
			return host + "'s host key is not known: the console shows it at a jump; check it before you type yes"
		}
		return host + "'s host key is not known: the jump's ssh shows it; check it before you answer yes"
	case remote.LoginHostKeyChanged:
		return "refused: " + host + "'s host key changed: this can be an attack. Only if you trust it: ssh-keygen -R " + host
	case remote.LoginUnreachable:
		return "cannot reach " + host + " with your ssh login: " + detail + "; check that ssh " + host + " works in a terminal"
	}
	return ""
}

// masterDir is the directory of the jumps' ssh masters: "" without a
// base, or when it is refused.
func (m Model) masterDir() (string, error) {
	if m.opt.MasterBase == "" {
		return "", nil
	}
	return remote.EnsureMasterDir(m.opt.MasterBase)
}

// masterWhy is why the master directory is refused, and its fix.
func masterWhy(err error) string {
	fix := "remove it, then open the host again"
	if errors.Is(err, remote.ErrMasterTooLong) {
		fix = "use a shorter directory"
	}
	return err.Error() + ": " + fix
}

// masterRefusal refuses the jumps inside tmux that have no master to open
// first.
func (m Model) masterRefusal(err error) string {
	return "refused: jumps to " + m.opt.Host + " need their ssh master: " + masterWhy(err)
}

// hostTmuxRefusal refuses a jump to a host whose tmux cannot take it, and
// selects the host's tmux door when it shows one.
func (m Model) hostTmuxRefusal(a *action) *action {
	host, v := m.opt.Host, m.host.Tmux
	why := "refused: " + host + " has no tmux: install it with its Install tmux door"
	if v != "none" {
		why = "refused: tmux " + v + " on " + host + " is older than " + remote.MinHostTmux + ": update tmux there"
	}
	b := refused(a, why)
	b.local = func(m Model) (Model, string) {
		for i, d := range m.doors {
			if d.ID == doors.Tmux {
				m.tab, m.doorPick, m.doorAt, m.recPick = tabSessions, d.ID, i, ""
			}
		}
		return m, why
	}
	return b
}

// runCheck runs the check's ssh and gives its stderr and exit; -1 when the
// time limit ended it.
func runCheck(argv []string) (string, int) { return runSSH(argv, nil, checkTimeout) }

// runSSH runs an ssh of the builder's own login, with env added to this
// process's, and gives its stderr and exit; -1 when limit ended it.
func runSSH(argv, env []string, limit time.Duration) (string, int) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // remote's argv of a host ValidHost took
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	// A new master stays in the background with ssh's stderr (-v keeps it
	// open): Wait must not wait for the master to end.
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	if cmd.ProcessState == nil {
		return err.Error(), 255 // it did not start: no ssh
	}
	return stderr.String(), cmd.ProcessState.ExitCode()
}
