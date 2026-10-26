package app

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/remote"
)

// checkTimeout ends a login check that waits: Tailscale's check mode waits
// for a browser.
const checkTimeout = 30 * time.Second

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
	// A directory that is refused: the check and the jumps go without a
	// master, and the console says why once.
	dir, err := m.masterDir()
	if err != nil {
		fix := "remove it, then open the host again"
		if errors.Is(err, remote.ErrMasterTooLong) {
			fix = "use a shorter directory"
		}
		m.flash = "jumps to " + m.opt.Host + " ask your login each time: " + err.Error() + ": " + fix
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
			next, cmd = m.onHost(*a)
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
	return m.onHost(a)
}

// loginNote is one line that names a login result and its fix; "" for ok.
// Until the console asks the password itself, the jump's ssh asks it in
// the terminal.
func (m Model) loginNote(l remote.Login, detail string) string {
	host := m.opt.Host
	switch l {
	case remote.LoginPassword:
		return host + " asks your password at each jump: put your key there (ssh-copy-id " + host + ")"
	case remote.LoginTailscaleCheck:
		if detail == "" {
			return "Tailscale SSH on " + host + " asks for a check in a browser"
		}
		return "Tailscale SSH on " + host + " asks for a check: open " + detail
	case remote.LoginTailscaleDenied:
		return "Tailscale SSH on " + host + " refuses you: " + detail + "; see your tailnet's SSH rules (docs/remote.md)"
	case remote.LoginHostKeyUnknown:
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
func runCheck(argv []string) (string, int) {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // remote.CheckArgv of a host ValidHost took
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
