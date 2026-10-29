package app

import (
	"errors"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/remote"
)

// forHost runs the console's own steps on host, with the builder's own ssh
// login: a tmux move jumps to the host, a door's command runs there. The
// inner argv is checked here, before ssh wraps it; a step that fails the
// check stays as it was, and the local check refuses it on confirm.
func (m Model) forHost(host string, a *action) *action {
	if a == nil || host == "" || a.remote {
		return a
	}
	b := *a
	switch {
	case a.nav:
		return m.jumpOnHost(host, a)
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

// jumpOnHost turns a tmux move into a jump to host: to the pane of a
// switch or an attach, or to a new shell in the directory of a new-window.
// The host opens in a session of its own on each side, never in the
// console's pane. Inside tmux the step is a tmux command list that returns
// at once; outside, the ssh takes this terminal.
func (m Model) jumpOnHost(host string, a *action) *action {
	var j remote.Jump
	for _, s := range a.steps {
		if !remote.NavAllowed(s) {
			return a
		}
		switch s[1] {
		case "switch-client", "attach-session":
			j.Pane = s[3]
		case "new-window":
			j.Dir = s[3]
		}
	}
	j.Host, j.Prefix, j.Home, j.ID, j.Outer = host, m.prefix(), m.home(), m.jumpID(), !m.opt.OutsideTmux
	if !remote.JumpAllowed(j) {
		return refused(a, jumpRefusal(j))
	}
	if j.Outer && remote.TmuxTooOld(m.opt.LocalTmux, remote.MinLocalTmux) {
		return refused(a, "tmux "+m.opt.LocalTmux+" is older than "+remote.MinLocalTmux+": update tmux, or run the console outside tmux")
	}
	if v := m.helloOf(host).Tmux; v == "none" || remote.TmuxTooOld(v, remote.MinHostTmux) {
		return m.hostTmuxRefusal(host, a)
	}
	if l := m.loginOf(host); l.login == remote.LoginHostKeyChanged {
		return refused(a, m.loginNote(host, l.login, ""))
	}
	dir, err := m.masterDir()
	if err != nil && j.Outer {
		// The jump switches the client at once: with no master to open
		// first, its ssh would ask in a session of its own.
		return refused(a, m.masterRefusal(host, err))
	}
	b := *a
	b.host = host
	master := remote.MasterOpts(dir)
	if dir != "" && m.opt.Ask != nil {
		// The console opens the master first: the jump's ssh never asks.
		master = append(master, "-o", "BatchMode=yes")
		b.master = dir
	}
	ssh := remote.JumpSSH(j.Host, j, master)
	b.steps, b.say, b.remote = [][]string{ssh}, shellLine(ssh), true
	if j.Outer {
		b.steps = [][]string{j.Local(ssh)}
		b.undo = []string{"tmux", "kill-session", "-t", "=" + remote.HostSessionName(j.Host, j.ID)}
	}
	return &b
}

// refused is an action that runs nothing: its confirm and its flash say
// why, and what fixes it.
func refused(a *action, why string) *action {
	return &action{label: a.label, say: why, local: func(m Model) (Model, string) { return m, why }}
}

// jumpRefusal says why JumpAllowed refuses j. The prefix and the id are
// made here and always pass.
func jumpRefusal(j remote.Jump) string {
	noHome := j
	noHome.Home = ""
	switch {
	case !remote.ValidHost(j.Host):
		return "refused: " + j.Host + " is not a host name ssh takes: use a name from your ssh config"
	case j.Home != "" && remote.JumpAllowed(noHome):
		return `refused: tmux session "` + j.Home + `" holds the console, and a jump cannot name it: rename it to letters, digits, _ - @ [ ] only`
	}
	return "refused: a jump goes only to a pane id, or an absolute directory without # or ;"
}

// prefix is this machine's tmux prefix: C-b when tmux did not say, or said
// a key a jump cannot bind.
func (m Model) prefix() string {
	if m.opt.Prefix != nil {
		if p := m.opt.Prefix(); remote.PrefixKey(p) {
			return p
		}
	}
	return "C-b"
}

// home is the session that holds the console: "" is matchblox.
func (m Model) home() string {
	if m.opt.Home == nil {
		return ""
	}
	return m.opt.Home()
}

func (m Model) jumpID() string {
	if m.opt.JumpID == nil {
		return remote.NewJumpID()
	}
	return m.opt.JumpID()
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
func (m Model) connectAction() *action { return m.connectOn(m.opt.Host) }

// connectOn is connectAction for any host of the console.
func (m Model) connectOn(host string) *action {
	blocked, older := m.blockOf(host)
	if host == "" || (blocked == nil && !older) || errors.Is(blocked, remote.ErrHostKeyChanged) {
		return nil // a changed host key is the builder's call, never a connect
	}
	self := m.opt.Self
	if self == "" {
		self = "matchblox"
	}
	argv := []string{self, "connect", host} // it also updates an older one
	return &action{label: "connect", say: shellLine(argv), steps: [][]string{argv}, destructive: true, remote: true, connect: true, host: host}
}

type connectedMsg struct {
	err  error
	host string
}

// onHost gives this terminal to a step that runs on the host. A jump
// inside tmux is a tmux command list instead: it switches the client and
// returns at once.
func (m Model) onHost(a action) (tea.Model, tea.Cmd) {
	argv := a.steps[0]
	if a.nav && !m.opt.OutsideTmux {
		run, undo := m.opt.Run, a.undo
		return m, func() tea.Msg {
			err := run(argv)
			if err != nil && undo != nil {
				_ = run(undo) // the list stopped after its new-session: its ssh would run on, unseen
			}
			return ranMsg{cmd: a.String(), err: err, jumped: a.pane}
		}
	}
	m.flash = "running: " + shellLine(argv)
	return m, m.exec(argv, func(err error) tea.Msg {
		switch {
		case a.connect:
			return connectedMsg{err, a.host}
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
		m.flash = "not connected: " + msg.err.Error() + "; " + m.keys.main.Open.Help().Key + " tries again"
		return m, nil
	}
	if i := m.peerAt(msg.host); msg.host != m.opt.Host && i >= 0 {
		if c := m.peers[i].conn; c != nil {
			_ = c.Close() // a peer of another version still holds it
		}
		next, cmd := m.peerTryAgain(i)
		nm := next.(Model)
		nm.flash = "connected " + msg.host + ", opening…"
		return nm, cmd
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
		note = "Check the new key with the owner of " + host + ". Only if you trust it: ssh-keygen -R " + host + ", then " + m.keys.main.Open.Help().Key + "."
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
		return fit(" "+keyWord(m.keys.confirm.Yes, st.neg, st.muted, " runs it  any other key cancels"), w)
	case m.flash != "":
		return fit(" "+st.text.Render(m.flash), w)
	}
	mk := m.keys.main
	hs := []hint{hintFor(mk.Quit, "")}
	if m.opt.Layered {
		hs = append([]hint{hintFor(mk.Back, "hosts")}, hs...)
	}
	switch {
	case m.connectAction() != nil:
		hs = append([]hint{hintFor(mk.Open, "connect")}, hs...)
	case m.blocked != nil:
		hs = append([]hint{hintFor(mk.Open, "try again")}, hs...)
	}
	return fit(" "+st.muted.Render(plainHints(hs...)), w)
}
