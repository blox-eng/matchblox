package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/transport"
)

// checkTimeout ends a login check that waits, unless it shows a Tailscale
// link.
const checkTimeout = 30 * time.Second

// openTimeout ends an ssh of the builder's login that waits: the builder
// types a password in the console, or approves a Tailscale check in a
// browser.
const openTimeout = 5 * time.Minute

// loginMsg is the result of the login check of host, of the console of
// gen. seen: its stderr has the server's version line, and tailscale is the
// host's kind.
type loginMsg struct {
	host            string
	gen             int
	login           remote.Login
	detail          string
	seen, tailscale bool
}

// urlMsg: an ssh of the builder's login, of the console of gen, printed
// Tailscale's link line. url is "" when the link is not a plain
// login.tailscale.com one; tailscale says the host is Tailscale SSH. next
// waits for the rest of the ssh.
type urlMsg struct {
	gen       int
	url       string
	tailscale bool
	next      tea.Cmd
}

// streamed runs an ssh of the builder's login in the background, until
// ctx ends or limit passes, and gives run's message when it ends. The link
// line of Tailscale's check comes first, as a urlMsg, the moment ssh
// prints it: the check waits for a browser. A plain link of a Tailscale
// SSH host moves the end out to withURL from the start. The host is
// Tailscale SSH when tailscale says so, or when a version line says so
// before the link (ssh -v).
func streamed(ctx context.Context, gen int, tailscale bool, limit, withURL time.Duration, run func(ctx context.Context, line func(string)) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		ctx, stop := context.WithCancel(ctx)
		start, end := time.Now(), time.AfterFunc(limit, stop)
		urls, done := make(chan urlMsg, 1), make(chan tea.Msg, 1)
		ts := tailscale
		go func() {
			defer stop()
			defer end.Stop()
			done <- run(ctx, func(l string) {
				ts = ts || remote.Tailscale(l)
				url, ok := remote.VisitURL(l)
				if !ok {
					return
				}
				if ts && url != "" {
					end.Reset(time.Until(start.Add(withURL)))
				}
				select {
				case urls <- urlMsg{gen: gen, url: url, tailscale: ts}:
				default: // one waits already
				}
			})
		}()
		return nextURL(urls, done)()
	}
}

// nextURL waits for the next link, or the end; a link that came first
// goes first.
func nextURL(urls <-chan urlMsg, done <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		select {
		case u := <-urls:
			u.next = nextURL(urls, done)
			return u
		default:
		}
		select {
		case u := <-urls:
			u.next = nextURL(urls, done)
			return u
		case msg := <-done:
			return msg
		}
	}
}

// linkShown shows Tailscale's link in the field while its ssh waits. A
// host that is not Tailscale SSH gets no link: its line can say anything.
func (m Model) linkShown(msg urlMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.opt.Gen || m.quitting {
		return m, nil
	}
	host := m.opt.Host
	switch {
	case !msg.tailscale:
		m.flash = host + " printed an unexpected login link: nothing was opened"
	case msg.url == "":
		m.flash = "Tailscale SSH on " + host + " printed a link that is not login.tailscale.com: nothing was opened"
	default:
		m.loginURL = msg.url
		m.pending, m.held = nil, nil // the field is on top: nothing waits under it
	}
	return m, msg.next
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
	case !m.opt.OutsideTmux && m.opt.Prefix != nil && m.opt.Prefix() != "" && !remote.PrefixKey(m.opt.Prefix()):
		m.flash = "jumps to " + m.opt.Host + " answer C-b, not your tmux prefix " + m.opt.Prefix() + ": a jump binds one key with C-, M- or C-M-"
	}
	host, check, gen := m.opt.Host, m.checker(), m.opt.Gen
	argv := remote.CheckArgv(host, dir)
	ctx, stop := context.WithCancel(context.Background())
	m.checkStop, m.checkCancelled = stop, false
	return m, streamed(ctx, gen, false, checkTimeout, openTimeout, func(ctx context.Context, line func(string)) tea.Msg {
		defer stop()
		stderr, exit := check(ctx, argv, line)
		login, detail := remote.ClassifyLogin(stderr, exit)
		return loginMsg{host: host, gen: gen, login: login, detail: detail, seen: remote.ServerSeen(stderr), tailscale: remote.Tailscale(stderr)}
	})
}

// checker runs a login check: Options.Check, or ssh.
func (m Model) checker() func(ctx context.Context, argv []string, line func(string)) (string, int) {
	if m.opt.Check != nil {
		return m.opt.Check
	}
	return runCheck
}

// loggedIn takes the check's result: a login that is not ok shows its fix,
// and the jump that waited goes, except to a changed host key or a check
// the builder cancelled; a password login offers the key first.
func (m Model) loggedIn(msg loginMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.opt.Gen || m.quitting || msg.host != m.opt.Host {
		return m, nil
	}
	m.checking, m.login, m.loginURL, m.checkStop = false, msg.login, "", nil
	if msg.seen {
		m.tailscale = msg.tailscale
	}
	a := m.jumpLater
	m.jumpLater = nil
	if m.checkCancelled {
		if m.login != remote.LoginOK {
			m.login = remote.LoginTailscaleCheck // the next jump asks for it again
		}
		m.flash = "cancelled: the Tailscale check of " + m.opt.Host + "; the next jump asks for it again"
		if a != nil {
			m.flash = "cancelled: the Tailscale check of " + m.opt.Host + ", and the jump did not run"
		}
		return m, nil
	}
	var cmd tea.Cmd
	if note := m.loginNote(msg.login, msg.detail); note != "" {
		m.flash = note
	}
	if a != nil {
		var next tea.Model
		next, cmd = m.jumpAfterLogin(*a)
		m = next.(Model)
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
	case m.offersLadder():
		return m.findStep(a)
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
	// The open lives as long as the console, and esc on its field ends it.
	ctx, stop := context.WithCancel(m.askCtx)
	m.opening, m.openStop, m.askCancelled = true, stop, false
	m.pending, m.held = nil, nil // the field is on top: nothing waits under it
	m.note, m.flash = m.flash, "logging in to "+host+"…"
	ssh, env, gen := m.ssh(), m.opt.Ask.Env(host), m.opt.Gen
	up, open := remote.MasterUpArgv(host, a.master), remote.OpenArgv(host, a.master)
	// The open has no -v: the host's kind comes from its check.
	return m, streamed(ctx, gen, m.tailscale, openTimeout, openTimeout, func(ctx context.Context, line func(string)) tea.Msg {
		defer stop()
		if _, exit := ssh(ctx, up, nil, ignoreLine); exit == 0 {
			return masterMsg{a: a, gen: gen}
		}
		stderr, exit := ssh(ctx, open, env, line)
		return masterMsg{a: a, gen: gen, stderr: stderr, exit: exit}
	})
}

// masterOpened runs the jump on a master that is up, or says why it did
// not run and the fix.
func (m Model) masterOpened(msg masterMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.opt.Gen || m.quitting {
		return m, nil
	}
	m.opening, m.openStop, m.loginURL = false, nil, ""
	note := m.note
	m.note = ""
	if msg.exit == 0 && !m.askCancelled {
		m.flash = note
		return m.onHost(msg.a)
	}
	host := m.opt.Host
	login, detail := remote.ClassifyAs(msg.stderr, msg.exit, m.tailscale)
	switch {
	case m.askCancelled:
		m.flash = "cancelled: the login to " + host + ", and the jump did not run"
	case login == remote.LoginTailscaleCheck:
		m.flash = "the jump did not run: the Tailscale check of " + host + " was not approved in time; jump again for a new link"
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
func (m Model) ssh() func(ctx context.Context, argv, env []string, line func(string)) (string, int) {
	if m.opt.SSH != nil {
		return m.opt.SSH
	}
	return func(ctx context.Context, argv, env []string, line func(string)) (string, int) {
		return runSSH(ctx, argv, env, openTimeout, line)
	}
}

// loginNote is one line that names a login result and its fix; "" for ok.
// Until the console asks the password itself, the jump's ssh asks it in
// the terminal.
func (m Model) loginNote(l remote.Login, detail string) string {
	host := m.opt.Host
	switch l {
	case remote.LoginPassword:
		if m.offersLadder() {
			return host + " asks your password: your first jump offers to put your key there"
		}
		if m.opt.Ask != nil {
			return host + " asks your password: the console asks it at a jump, once in 10 minutes; put your key there to skip it (ssh-copy-id " + host + ")"
		}
		return host + " asks your password at each jump: put your key there (ssh-copy-id " + host + ")"
	case remote.LoginKeyOnly:
		return host + " takes keys only, and none of yours: add your public key to ~/.ssh/authorized_keys on " + host + " another way, such as from a session you have there, then open " + host + " again"
	case remote.LoginTailscaleCheck:
		// Its link is old when the check ended: the next one shows a new one.
		return "Tailscale SSH on " + host + " asks for a check in a browser: the next jump shows its link"
	case remote.LoginTailscaleDenied:
		return "Tailscale SSH on " + host + " refuses you: " + detail + "; see your tailnet's SSH rules (docs/remote.md#tailscale-ssh)"
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
// base, or when it is refused. It is read at each jump (a few stat calls):
// a /tmp cleaner may remove it, or its mode change, after the check.
func (m Model) masterDir() (string, error) {
	if m.opt.MasterBase == "" {
		return "", nil
	}
	return remote.EnsureMasterDir(m.opt.MasterBase)
}

// masterWhy is why the master directory is refused, and its fix.
func masterWhy(err error) string {
	fix := "remove it, then open the host again"
	switch {
	case errors.Is(err, remote.ErrMasterTooLong):
		fix = "use a shorter directory"
	case errors.Is(err, transport.ErrNotYours):
		fix = "only its owner or root can remove it; then open matchblox again"
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
	why := "refused: " + host + " has no tmux: install it with its Install tmux door, then open the host again"
	if v != "none" {
		why = "refused: tmux " + v + " on " + host + " is older than " + remote.MinHostTmux + ": update tmux there, then open the host again"
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

// runCheck runs the check's ssh and gives its stderr and exit; -1 when ctx
// ended it. Its caller sets the time limit: a Tailscale link moves it.
func runCheck(ctx context.Context, argv []string, line func(string)) (string, int) {
	return runSSH(ctx, argv, nil, openTimeout, line)
}

// runSSH runs an ssh of the builder's own login, with env added to this
// process's, gives each line of its stderr to line as ssh prints it, and
// gives its stderr and exit; -1 when ctx or limit ended it. It has no
// terminal: an ssh before 8.4 would ask on the console's, over the screen.
func runSSH(ctx context.Context, argv, env []string, limit time.Duration, line func(string)) (string, int) {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // remote's argv of a host ValidHost took
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	stderr := &lineWriter{line: line}
	cmd.Stderr = stderr
	ownSession(cmd)
	// A new master stays in the background with ssh's stderr (-v keeps it
	// open): Wait must not wait for the master to end.
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	if cmd.ProcessState == nil {
		return err.Error(), 255 // it did not start: no ssh
	}
	return stderr.String(), cmd.ProcessState.ExitCode()
}

// ignoreLine takes the lines of an ssh whose stderr is read at its end.
func ignoreLine(string) {}

// lineWriter keeps what ssh writes and gives each whole line to line, as
// it comes, without its line break. A host can write without end: it keeps
// the start (ssh's version line) and the end (the result) of it, and drops
// a line longer than maxLine.
type lineWriter struct {
	mu   sync.Mutex
	head []byte
	tail []byte
	part []byte
	skip bool // the line that comes is past maxLine: dropped up to its end
	line func(string)
}

const (
	keepHead = 16 << 10
	keepTail = 48 << 10
	maxLine  = 4 << 10
)

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	w.keep(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			if !w.skip {
				w.part = append(w.part, p...)
			}
			if len(w.part) > maxLine {
				w.part, w.skip = nil, true
			}
			return n, nil
		}
		if !w.skip {
			w.part = append(w.part, p[:i]...)
			w.line(strings.TrimSuffix(string(w.part), "\r"))
		}
		w.part, w.skip, p = w.part[:0], false, p[i+1:]
	}
	return n, nil
}

// keep adds p to the start until it is full, then to the end, which keeps
// at least its last keepTail bytes: it is cut to them at twice that, so a
// flood of small writes copies little.
func (w *lineWriter) keep(p []byte) {
	if n := min(keepHead-len(w.head), len(p)); n > 0 {
		w.head, p = append(w.head, p[:n]...), p[n:]
	}
	w.tail = append(w.tail, p...)
	if len(w.tail) > 2*keepTail {
		w.tail = append(w.tail[:0:0], w.tail[len(w.tail)-keepTail:]...)
	}
}

func (w *lineWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.head) + string(w.tail[max(0, len(w.tail)-keepTail):])
}
