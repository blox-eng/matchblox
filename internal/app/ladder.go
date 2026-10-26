package app

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/login"
	"github.com/blox-eng/matchblox/internal/remote"
)

// Keys is this machine's side of the key ladder (design §3.3): the
// builder's own keys, and the macOS door for ~/.ssh/config.
type Keys struct {
	// Step is the ladder's first step: login.Ladder over ssh-add -L and
	// ~/.ssh. It reads files and runs ssh-add, so it runs in the
	// background.
	Step func() login.Step
	// GOOS is this machine's: ssh-add on darwin keeps the passphrase in
	// the Keychain.
	GOOS string
	// Keychain is the door that adds AddKeysToAgent and UseKeychain for a
	// host to ~/.ssh/config; OpenKeychain writes it with the Sum the
	// builder saw. Nil: not macOS, no door.
	Keychain     func(host string) doors.Door
	OpenKeychain func(host, seen string) (backup string, err error)
}

// climb is one rung of the ladder the console offers: the step, the jump
// that waits for it, and, after an install on macOS, the Keychain door.
type climb struct {
	step login.Step
	jump *action
	door *doors.Door
	note string // one more line for the offer (a new key not in the agent)
}

// newKeyHost names this machine in the field while ssh-keygen and ssh-add
// ask the new key's passphrase.
const newKeyHost = "this machine"

type stepMsg struct {
	step login.Step
	jump *action
	gen  int
}

// keyMadeMsg is the end of a new key: made (its step, added to the agent
// or not), or why not.
type keyMadeMsg struct {
	c       climb
	gen     int
	refused string // why ssh-keygen did not run
	exit    int    // ssh-keygen's exit
	open    bool   // the new key is not locked by a passphrase: it was removed
	removed error  // why a file of that key stays
	added   bool
}

// installedMsg is the end of an install: ssh's stderr and exit, and the
// login check after it.
type installedMsg struct {
	c      climb
	gen    int
	stderr string
	exit   int
	check  remote.Login
	door   *doors.Door
}

// offersLadder: a jump to a host that asks a password offers the builder's
// key first, once per console, unless they chose not now.
func (m Model) offersLadder() bool {
	return m.login == remote.LoginPassword && m.opt.Keys != nil && m.opt.Ask != nil && !m.notNow
}

// findStep finds the ladder's first step for the jump a, in the
// background.
func (m Model) findStep(a action) (tea.Model, tea.Cmd) {
	m.flash = "looking for your ssh keys…"
	find, gen := m.opt.Keys.Step, m.opt.Gen
	return m, func() tea.Msg { return stepMsg{step: find(), jump: &a, gen: gen} }
}

// foundStep offers the step.
func (m Model) foundStep(msg stepMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.opt.Gen || m.quitting {
		return m, nil
	}
	m.flash = ""
	m.pending = m.offer(climb{step: msg.step, jump: msg.jump})
	return m, nil
}

// offer is the confirm of a rung: what runs, exactly, and which key, for a
// typed y. Any other key is not now; esc cancels the jump too.
func (m Model) offer(c climb) *action {
	host, s := m.opt.Host, c.step
	a := &action{destructive: true, climb: &c}
	notNow := "any other key: not now; " + host + " asks your password here at a jump, once in 10 minutes"
	line := func(l string) { a.steps = append(a.steps, []string{l}) }
	switch {
	case c.door != nil:
		a.label = c.door.Title
		line(c.door.Why)
		line("in " + c.door.Path + ":")
		for l := range strings.SplitSeq(c.door.Preview, "\n") {
			line(l)
		}
		line("any other key: " + c.door.Path + " stays as it is")
		return a
	case s.Source == login.NewKey:
		a.label = "make your ssh key"
		line("you have no ssh key: make " + s.Path + ", locked by a passphrase you type here")
		line("runs: " + remote.Quote(login.NewKeyArgv(s.Path)))
		line("then: " + remote.Quote(login.AddArgv(s.Path, m.opt.Keys.GOOS)))
		line("then it shows the key, and asks to put it on " + host)
		line(notNow)
		return a
	}
	where := "in your ssh-agent"
	if s.Source == login.FromFile {
		where = s.Path
	}
	a.label = "put your key on " + host
	line("key " + s.Fingerprint + " (" + strings.Fields(s.Line)[0] + ", " + where + ")")
	if c.note != "" {
		line(c.note)
	}
	if s.NoPassphrase && !strings.HasPrefix(s.Line, "sk-") { // a security key needs its token too
		line("this key has no passphrase: anyone who reads " + s.Path + " logs in as you; lock it: ssh-keygen -p -f " + s.Path)
	}
	if argv, err := m.installArgv(s.Line); err == nil {
		line("runs: " + remote.Quote(argv[:len(argv)-1]) + " " + argv[len(argv)-1])
	}
	line(host + " asks your password here, once")
	line(notNow)
	return a
}

// installArgv puts line on the host through the jumps' master; without a
// master directory, the install logs in on its own.
func (m Model) installArgv(line string) ([]string, error) {
	dir, err := m.masterDir()
	if err != nil {
		dir = ""
	}
	return login.InstallArgv(m.opt.Host, line, remote.MasterOpts(dir))
}

// climbed runs the rung the builder confirmed.
func (m Model) climbed(c climb) (tea.Model, tea.Cmd) {
	switch {
	case c.door != nil:
		return m.openKeychain(c)
	case c.step.Source == login.NewKey:
		return m.makeKey(c)
	}
	return m.install(c)
}

// declined is any key but y on a rung: esc cancels the jump, and the next
// jump offers again; any other key is not now, and the jump goes. After
// the install, the door's no only keeps the file.
func (m Model) declined(c climb, esc bool) (tea.Model, tea.Cmd) {
	switch {
	case c.door != nil:
		m.flash = c.door.Path + " stays as it is"
	case esc:
		m.flash = "cancelled: your key is not on " + m.opt.Host
		if c.jump != nil {
			m.flash += ", and the jump did not run"
		}
		return m, nil
	default:
		m.notNow = true
		m.flash = "not now: " + m.opt.Host + " asks your password at a jump"
	}
	if c.jump == nil {
		return m, nil
	}
	return m.runJump(*c.jump)
}

// startLogin starts an ssh of the ladder with its prompts in the console:
// the field waits on top, and esc ends it.
func (m Model) startLogin(flash string) (Model, context.Context, context.CancelFunc) {
	ctx, stop := context.WithCancel(m.askCtx)
	m.opening, m.openStop, m.askCancelled = true, stop, false
	m.pending, m.held = nil, nil
	m.flash = flash
	return m, ctx, stop
}

// install puts the key on the host with the builder's login, its password
// in the field, then checks the login with the key alone.
func (m Model) install(c climb) (tea.Model, tea.Cmd) {
	host := m.opt.Host
	argv, err := m.installArgv(c.step.Line)
	if err != nil {
		m.flash = "refused: " + err.Error()
		return m, nil
	}
	m, ctx, stop := m.startLogin("putting your key on " + host + "…")
	ssh, env, check, gen := m.ssh(), m.opt.Ask.Env(host), m.checker(), m.opt.Gen
	keychain := m.opt.Keys.Keychain
	return m, func() tea.Msg {
		defer stop()
		stderr, exit := ssh(ctx, argv, env)
		msg := installedMsg{c: c, gen: gen, stderr: stderr, exit: exit}
		if exit != 0 || ctx.Err() != nil {
			return msg
		}
		msg.check, _ = remote.ClassifyLogin(check(remote.KeyCheckArgv(host)))
		if keychain != nil {
			if d := keychain(host); d.Open() && d.Problem == "" {
				msg.door = &d
			}
		}
		return msg
	}
}

// installed says what came of the install. A key on the host ends the
// offers; the macOS door comes next, then the jump that waited.
func (m Model) installed(msg installedMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.opt.Gen || m.quitting {
		return m, nil
	}
	m.opening, m.openStop = false, nil
	host, s := m.opt.Host, msg.c.step
	if m.askCancelled || msg.exit != 0 {
		m.flash = m.notInstalled(msg)
		return m, nil
	}
	m.notNow = true
	m.flash = "your key " + s.Fingerprint + " is on " + host
	if msg.check == remote.LoginOK {
		m.login = remote.LoginOK
		m.flash += ": your login uses it, no password"
	} else {
		m.flash += ", but ssh cannot use it without a prompt: a jump asks its passphrase here"
		if s.Path != "" {
			m.flash += "; ssh-add " + s.Path + " keeps it in your agent"
		}
	}
	if msg.door != nil {
		m.pending = m.offer(climb{step: s, jump: msg.c.jump, door: msg.door})
		return m, nil
	}
	if msg.c.jump == nil {
		return m, nil
	}
	return m.runJump(*msg.c.jump)
}

func (m Model) notInstalled(msg installedMsg) string {
	host := m.opt.Host
	if m.askCancelled {
		return "cancelled: your key is not on " + host
	}
	login, detail := remote.ClassifyLogin(msg.stderr, msg.exit)
	switch login {
	case remote.LoginPassword:
		return "your key is not on " + host + ": it refused your login; jump again to type it again"
	case remote.LoginOK: // an exit that says nothing
		return "your key is not on " + host + ": ssh exited " + strconv.Itoa(msg.exit)
	}
	return "your key is not on " + host + ": " + m.loginNote(login, detail)
}

// makeKey makes the new key, its passphrase typed in the field (twice),
// and adds it to the agent (the passphrase once more).
func (m Model) makeKey(c climb) (tea.Model, tea.Cmd) {
	path := c.step.Path
	m, ctx, stop := m.startLogin("making your ssh key…")
	ssh, env, gen, add := m.ssh(), m.opt.Ask.Env(newKeyHost), m.opt.Gen, login.AddArgv(path, m.opt.Keys.GOOS)
	return m, func() tea.Msg {
		defer stop()
		msg := keyMadeMsg{c: c, gen: gen}
		for _, p := range []string{path, path + ".pub"} {
			if _, err := os.Lstat(p); !errors.Is(err, fs.ErrNotExist) {
				msg.refused = p + " is there, but not its key pair: move it away, then jump again"
				return msg
			}
		}
		if _, msg.exit = ssh(ctx, login.NewKeyArgv(path), env); msg.exit != 0 {
			return msg
		}
		made, err := login.StatMade(path)
		if err != nil {
			msg.refused = "ssh-keygen made no key at " + path + ": " + err.Error()
			return msg
		}
		// matchblox never leaves a key without a passphrase: only a key
		// ssh-keygen refuses to open empty is kept.
		if !login.Locked(path) {
			msg.open, msg.removed = true, made.Remove()
			return msg
		}
		s, ok := login.KeyFile(path)
		if !ok {
			msg.refused = "the new key " + path + " has no .pub ssh-keygen wrote"
			return msg
		}
		msg.c.step = s
		_, exit := ssh(ctx, add, env)
		msg.added = exit == 0
		return msg
	}
}

// keyMade offers the new key's install, or says loudly why there is none.
func (m Model) keyMade(msg keyMadeMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.opt.Gen || m.quitting {
		return m, nil
	}
	m.opening, m.openStop = false, nil
	path := msg.c.step.Path
	switch {
	case m.askCancelled:
		m.flash = "cancelled: no key made"
	case msg.refused != "":
		m.flash = "refused: " + msg.refused
	case msg.exit != 0:
		m.flash = "no key made: ssh-keygen exited " + strconv.Itoa(msg.exit)
	case msg.open && msg.removed != nil:
		m.flash = "refused: the new key " + path + " is not locked by a passphrase, and not all of it was removed (" + msg.removed.Error() + "): remove it yourself, then jump again and type a passphrase"
	case msg.open:
		m.flash = "refused: the new key " + path + " is not locked by a passphrase, so matchblox removed it and put nothing on " + m.opt.Host + ": jump again and type a passphrase"
	default:
		c := msg.c
		if !msg.added {
			c.note = "not in your agent: a jump asks its passphrase here"
		}
		m.flash = ""
		m.pending = m.offer(c)
	}
	return m, nil
}

// openKeychain writes the macOS door, then the jump that waited goes.
func (m Model) openKeychain(c climb) (tea.Model, tea.Cmd) {
	backup, err := m.opt.Keys.OpenKeychain(m.opt.Host, c.door.Sum)
	switch {
	case err != nil:
		m.flash = c.door.Path + " not changed: " + err.Error()
	case backup != "":
		m.flash = "added to " + c.door.Path + " (backup: " + backup + ")"
	default:
		m.flash = "added to " + c.door.Path
	}
	if c.jump == nil {
		return m, nil
	}
	return m.runJump(*c.jump)
}
