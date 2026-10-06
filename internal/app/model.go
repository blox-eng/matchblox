// Package app is the console: a Bubble Tea program that keeps a copy of the
// state the service sends and draws it. It does not read the machine; it
// asks the service to act, and runs only the steps that move its own
// terminal.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/history"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/transport"
)

// installCmd updates matchblox on the side that is older.
const installCmd = "curl -fsSL https://matchblox.com/install.sh | sh"

type Options struct {
	Conn transport.Conn
	// Redial connects again after the connection drops. Nil: no retry.
	Redial func() (transport.Conn, error)
	// Binary is the version in the console's hello.
	Binary string
	// CompactAt colours context at the compact threshold.
	CompactAt float64
	// Run executes one step that moves this terminal (Nav). Nil means exec.
	Run func(argv []string) error
	// Local: the service runs on this machine from the same install, so an
	// older one is replaced instead of asking the person to update it.
	Local bool
	// Owner is the pid of the local service, to name it when it does not
	// answer. Nil or 0: unknown.
	Owner func() int
}

type helloMsg proto.Hello
type stateMsg proto.State
type resultMsg proto.Result
type lostMsg struct {
	err  error
	conn transport.Conn
}

// fromConn tags a message with the connection it came from: after a
// reconnect, messages from the old one are dropped.
type fromConn struct {
	conn transport.Conn
	msg  tea.Msg
}
type redialMsg struct{}

// silentMsg fires when a connection gave no state in silentAfter.
type silentMsg struct{ conn transport.Conn }
type redialFailed struct{ err error }
type connMsg struct{ conn transport.Conn }
type ranMsg struct {
	cmd string
	err error
}

// action waits for the person's confirm. A Nav action runs here; any other
// is sent to the service by name.
type action struct {
	label       string
	steps       [][]string
	destructive bool
	nav         bool
	rec         string // the service's name for it
	which       string // primary | secondary
}

func (a action) String() string {
	s := strings.Join(a.steps[0], " ")
	if n := len(a.steps); n > 1 {
		s += fmt.Sprintf("  (+%d more)", n-1)
	}
	return s
}

func fromAdvice(r advice.Rec, which string) *action {
	a := r.Primary
	if which == "secondary" {
		a = r.Second
	}
	if a == nil || len(a.Steps) == 0 {
		return nil
	}
	return &action{label: a.Label, steps: a.Steps, destructive: a.Destructive, nav: a.Nav, rec: r.ID, which: which}
}

type Model struct {
	opt       Options
	conn      transport.Conn
	st        styles
	width     int
	height    int
	snap      sample.Snapshot
	have      bool
	git       *proto.GitReport
	recs      []advice.Rec
	hist      history.Series
	selPID    int
	orphanPID int
	recSel    int
	gitSel    int
	tab       int
	pending   *action
	flash     string
	history   *series
	quitting  bool
	host      proto.Hello
	mismatch  bool
	lost      bool
	backoff   time.Duration
	redialing bool
	answered  bool // a state came on the current connection
	replaced  bool
	acts      int
}

func New(opt Options) Model {
	if opt.CompactAt <= 0 {
		opt.CompactAt = sample.DefaultRules.CompactAt
	}
	if opt.Run == nil {
		opt.Run = run
	}
	if opt.Binary == "" {
		opt.Binary = "dev"
	}
	return Model{opt: opt, conn: opt.Conn, st: newStyles(true), width: 100, height: 30, history: newSeries(60)}
}

// run executes a Nav step: it only moves this terminal, so a short bound
// is enough.
func run(argv []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, argv[0], argv[1:]...).Run() //nolint:gosec // argv from the service's advice, never a shell string
}

// silentAfter is how long a connected service may stay silent before the
// console says so and how to fix it.
const silentAfter = 3 * time.Second

func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, m.hello(), m.recv(), m.watchSilence())
}

func (m Model) watchSilence() tea.Cmd {
	c := m.conn
	return tea.Tick(silentAfter, func(time.Time) tea.Msg { return silentMsg{c} })
}

func (m Model) hello() tea.Cmd {
	c, bin := m.conn, m.opt.Binary
	if c == nil {
		return nil
	}
	return func() tea.Msg {
		if err := c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version, Binary: bin}); err != nil {
			return lostMsg{err, c}
		}
		return nil
	}
}

// recv reads one message. Each handled message asks for the next, so one
// read is in flight at a time.
func (m Model) recv() tea.Cmd {
	c := m.conn
	if c == nil {
		return nil
	}
	return func() tea.Msg {
		for {
			env, err := c.Recv()
			if err != nil {
				return lostMsg{err, c}
			}
			switch env.Kind {
			case proto.KindHello:
				var h proto.Hello
				if json.Unmarshal(env.Body, &h) == nil {
					return fromConn{c, helloMsg(h)}
				}
			case proto.KindSnapshot:
				var st proto.State
				if json.Unmarshal(env.Body, &st) == nil {
					return fromConn{c, stateMsg(st)}
				}
			case proto.KindResult:
				var r proto.Result
				if json.Unmarshal(env.Body, &r) == nil {
					return fromConn{c, resultMsg(r)}
				}
			}
		}
	}
}

func (m *Model) send(a proto.Act) tea.Cmd {
	c := m.conn
	m.acts++
	id := "a" + strconv.Itoa(m.acts)
	return func() tea.Msg {
		if err := c.Send(proto.KindAct, id, a); err != nil {
			return lostMsg{err, c}
		}
		return nil
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.st = newStyles(msg.IsDark())
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case fromConn:
		if msg.conn != m.conn {
			return m, nil
		}
		return m.Update(msg.msg)
	case helloMsg:
		m.host = proto.Hello(msg)
		if m.olderLocal() {
			// The old service exits; the redial starts this binary.
			m.replaced = true
			m.flash = fmt.Sprintf("replacing the service %s with %s…", m.host.Binary, m.opt.Binary)
			return m, tea.Batch(m.send(proto.Act{RecID: "service:replace"}), m.recv())
		}
		m.mismatch = m.host.Version != proto.Version
		if m.mismatch {
			return m, nil
		}
		return m, m.recv()
	case stateMsg:
		first := !m.have
		st := proto.State(msg)
		m.snap, m.git, m.recs, m.have, m.answered = st.Snapshot, st.Git, st.Recommendations, true, true
		if m.lost {
			m.lost, m.backoff, m.flash = false, 0, ""
		}
		m.history.push(m.snap.Machine)
		switch {
		case st.History != nil:
			m.hist = *st.History
		case !first:
			m.hist.Append(history.FromSnapshot(m.snap))
		}
		m.keepSelection()
		return m, m.recv()
	case resultMsg:
		if text := describe(proto.Result(msg)); text != "" {
			m.flash = text
		}
		return m, m.recv()
	case lostMsg:
		if m.mismatch || m.quitting || m.redialing || (msg.conn != nil && msg.conn != m.conn) {
			return m, nil
		}
		if m.conn != nil {
			_ = m.conn.Close()
		}
		m.lost = true
		m.flash = "connection lost: " + msg.err.Error()
		if m.opt.Redial == nil {
			return m, nil
		}
		m.redialing = true
		m.backoff = min(max(m.backoff*2, 250*time.Millisecond), 10*time.Second)
		m.flash += "; trying again in " + m.backoff.String()
		return m, tea.Tick(m.backoff, func(time.Time) tea.Msg { return redialMsg{} })
	case redialMsg:
		redial := m.opt.Redial
		return m, func() tea.Msg {
			c, err := redial()
			if err != nil {
				return redialFailed{err}
			}
			return connMsg{c}
		}
	case silentMsg:
		if msg.conn != m.conn || m.answered || m.mismatch {
			return m, nil
		}
		pid := 0
		if m.opt.Owner != nil {
			pid = m.opt.Owner()
		}
		if pid > 0 {
			m.flash = fmt.Sprintf("the service (pid %d) does not answer: kill %d, then start matchblox again", pid, pid)
		} else {
			m.flash = "the service does not answer: stop `matchblox serve`, then start matchblox again"
		}
		return m, nil
	case redialFailed:
		m.redialing = false
		return m.Update(lostMsg{err: msg.err})
	case connMsg:
		m.conn, m.redialing, m.answered = msg.conn, false, false
		m.flash = "connected again"
		return m, tea.Batch(m.hello(), m.recv(), m.watchSilence())
	case ranMsg:
		if msg.err != nil {
			m.flash = "failed: " + msg.cmd + ": " + msg.err.Error()
		} else {
			m.flash = "ran: " + msg.cmd
		}
	case tea.KeyPressMsg:
		return m.key(msg.String())
	}
	return m, nil
}

// olderLocal tells if the local service should give way to this console:
// an older wire, or another build. Once only, so two consoles of two
// versions do not replace each other forever.
func (m Model) olderLocal() bool {
	if !m.opt.Local || m.replaced || m.opt.Redial == nil {
		return false
	}
	return m.host.Version < proto.Version || (m.opt.Binary != "dev" && m.host.Binary != m.opt.Binary)
}

func describe(r proto.Result) string {
	var parts []string
	if len(r.Ran) > 0 {
		cmd := strings.Join(r.Ran[0], " ")
		if n := len(r.Ran); n > 1 {
			cmd += fmt.Sprintf("  (+%d more)", n-1)
		}
		parts = append(parts, "ran: "+cmd)
	}
	if len(r.Skipped) > 0 {
		parts = append(parts, fmt.Sprintf("skipped %d: %s", len(r.Skipped), r.Skipped[len(r.Skipped)-1]))
	}
	if r.Err != "" {
		parts = append(parts, "failed: "+r.Err)
	}
	return strings.Join(parts, "; ")
}

func (m Model) key(k string) (tea.Model, tea.Cmd) {
	if m.pending != nil {
		a := *m.pending
		m.pending = nil
		// A destructive action needs a typed y; Enter alone is not consent.
		if k == "y" || (k == "enter" && !a.destructive) {
			if a.nav {
				run := m.opt.Run
				return m, func() tea.Msg { return runNav(a, run) }
			}
			if m.conn == nil || m.lost {
				m.flash = "not sent, no connection: " + a.String()
				return m, nil
			}
			confirm := ""
			if a.destructive {
				confirm = "y"
			}
			m.flash = "sent: " + a.String()
			return m, m.send(proto.Act{RecID: a.rec, Which: a.which, Confirm: confirm})
		}
		m.flash = "cancelled: " + a.String()
		return m, nil
	}
	m.flash = ""
	switch k {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "tab":
		m.tab = (m.tab + 1) % len(tabNames)
	case "shift+tab":
		m.tab = (m.tab + len(tabNames) - 1) % len(tabNames)
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "enter":
		m.pending = m.primary()
	case "x":
		m.pending = m.secondary()
	case "r":
		if m.tab == tabGit && m.conn != nil && !m.lost {
			m.flash = "scanning git…"
			return m, m.send(proto.Act{RecID: "rescan:git", Which: "primary"})
		}
	default:
		if len(k) == 1 && k[0] >= '1' && int(k[0]-'1') < len(tabNames) {
			m.tab = int(k[0] - '1')
		}
	}
	return m, nil
}

// navAllowed is what a Nav step may be: it runs here on one key, so a
// service can never make the console run anything but a tmux move.
func navAllowed(argv []string) bool {
	return len(argv) >= 2 && argv[0] == "tmux" && (argv[1] == "switch-client" || argv[1] == "new-window")
}

func runNav(a action, run func([]string) error) ranMsg {
	for _, step := range a.steps {
		if !navAllowed(step) {
			return ranMsg{strings.Join(step, " "), fmt.Errorf("refused: only tmux switch-client and new-window run in the console")}
		}
	}
	for _, step := range a.steps {
		if err := run(step); err != nil {
			return ranMsg{strings.Join(step, " "), err}
		}
	}
	return ranMsg{a.String(), nil}
}

func jump(pane string) *action {
	return &action{label: "jump", nav: true, steps: [][]string{{"tmux", "switch-client", "-t", pane}}}
}

// primary is what Enter does on the current panel: never destructive.
func (m Model) primary() *action {
	switch m.tab {
	case tabSessions:
		if s, ok := m.selected(); ok && s.Pane != "" {
			return jump(s.Pane)
		}
	case tabProcs:
		if o, ok := m.selectedOrphan(); ok && o.PaneAlive {
			return jump(o.Pane)
		}
	case tabGit:
		if wt, ok := m.selectedWorktree(); ok {
			return &action{label: "shell", nav: true, steps: [][]string{{"tmux", "new-window", "-c", wt.Path}}}
		}
	case tabRecs:
		if r, ok := m.selectedRec(); ok {
			return fromAdvice(r, "primary")
		}
	}
	return nil
}

// secondary is what x does: the destructive action, behind a typed y. The
// service builds and guards it again from its own state.
func (m Model) secondary() *action {
	switch m.tab {
	case tabProcs:
		if o, ok := m.selectedOrphan(); ok {
			return &action{label: "kill", steps: [][]string{o.Kill}, destructive: true,
				rec: "orphan:" + strconv.Itoa(o.PID), which: "secondary"}
		}
	case tabGit:
		if wt, ok := m.selectedWorktree(); ok && wt.Safe {
			return &action{label: "remove", steps: [][]string{wt.Remove}, destructive: true,
				rec: "worktree:" + wt.Path, which: "secondary"}
		}
	case tabRecs:
		if r, ok := m.selectedRec(); ok {
			return fromAdvice(r, "secondary")
		}
	}
	return nil
}

func clampMove(i, d, n int) int { return min(max(i+d, 0), max(n-1, 0)) }

func (m *Model) move(d int) {
	switch m.tab {
	case tabProcs:
		if o := m.snap.Orphans; len(o) > 0 {
			m.orphanPID = o[clampMove(m.orphanIndex(), d, len(o))].PID
		}
	case tabGit:
		m.gitSel = clampMove(m.gitSel, d, len(m.worktreeRows()))
	case tabRecs:
		m.recSel = clampMove(m.recSel, d, len(m.recs))
	case tabSessions:
		if ss := m.snap.Sessions; len(ss) > 0 {
			m.selPID = ss[clampMove(m.selIndex(), d, len(ss))].PID
		}
	}
}

func (m Model) selIndex() int {
	for i, s := range m.snap.Sessions {
		if s.PID == m.selPID {
			return i
		}
	}
	return 0
}

func (m Model) selected() (sample.Session, bool) {
	if len(m.snap.Sessions) == 0 {
		return sample.Session{}, false
	}
	return m.snap.Sessions[m.selIndex()], true
}

func (m Model) selectedRec() (advice.Rec, bool) {
	if len(m.recs) == 0 {
		return advice.Rec{}, false
	}
	return m.recs[min(m.recSel, len(m.recs)-1)], true
}

// keepSelection follows the selected session by PID across re-sorts.
func (m *Model) keepSelection() {
	if _, ok := m.selected(); ok {
		m.selPID = m.snap.Sessions[m.selIndex()].PID
	}
}

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}
