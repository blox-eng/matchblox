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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/history"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/transport"
)

// installCmd updates matchblox on the side that is older.
const installCmd = "curl -fsSL https://matchblox.sh | sh"

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
	// Now is the clock for ages on the screen. Nil: time.Now. The replay
	// sets a fixed clock.
	Now func() time.Time
	// NoMotion makes every motion an instant change.
	NoMotion bool
	// OutsideTmux: the console does not run in tmux, so going to a pane
	// attaches to it, and the console comes back when tmux detaches.
	OutsideTmux bool
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
	attach      bool     // the last step takes the terminal until it exits
	rec         string   // the service's name for it
	which       string   // primary | secondary
	text        string   // what an answer types; a door's preview Sum
	batch       []string // worktrees: one guarded act each
	say         string   // what the confirm shows, when not the first step
	term        bool     // the step takes this terminal (a door's command)
	door        string   // the door it opens or closes
}

func (a action) String() string {
	if a.say != "" {
		return a.say
	}
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
	opt        Options
	conn       transport.Conn
	st         styles
	width      int
	height     int
	snap       sample.Snapshot
	have       bool
	git        *proto.GitReport
	recs       []advice.Rec
	queue      []queue.Item
	queuePane  string
	doors      []doors.Door // the open doors, above the queue
	doorPick   string       // the selected door
	doorAt     int          // its row, for the door after it when it folds
	previewTop int          // the first line of a door's diff on screen
	paneSel    int
	input      *answerInput
	changes    map[string]change // pane -> a match change that plays once
	animating  bool
	hist       history.Series
	selPID     int
	orphanPID  int
	recPick    string // the selected recommendation, under the queue
	gitSel     int
	tab        int
	pending    *action
	flash      string
	history    *series
	quitting   bool
	host       proto.Hello
	mismatch   bool
	lost       bool
	backoff    time.Duration
	redialing  bool
	answered   bool // a state came on the current connection
	replaced   bool
	acts       int
	dark       bool
	splashAt   time.Time
	splashDone bool
	armed      string // the step a second tap runs: what the first tap showed
	all        lists  // what the service sent; queue, recs and snap hold the drawn copies
	filter     string // the search of the open tab
	searching  bool   // the search line takes the keys
	sessSort   sortBy
	paneSort   sortBy
	picked     map[string]bool // worktrees x removes
	batch      *batch
	showExited bool // e: the exited agents, one by one
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
	m := Model{opt: opt, conn: opt.Conn, st: newStyles(true), dark: true, width: 100, height: 30, history: newSeries(60)}
	m.splashAt = m.now()
	return m
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
	return tea.Batch(tea.RequestBackgroundColor, m.hello(), m.recv(), m.watchSilence(), m.splashCmd())
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
		m.dark = msg.IsDark()
		m.st = newStyles(m.dark)
	case splashMsg:
		if m.splashing() {
			return m, m.splashCmd()
		}
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
		m.queue = st.Queue
		m.doors = openDoors(st.Doors)
		m.noteChanges(st.Sessions)
		m.snap, m.git, m.recs, m.have, m.answered = st.Snapshot, st.Git, st.Recommendations, true, true
		m.all = lists{queue: st.Queue, recs: st.Recommendations, sessions: st.Sessions, orphans: st.Orphans}
		m.refresh()
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
		return m, tea.Batch(m.recv(), m.animate())
	case doorRanMsg:
		return m.doorRan(msg)
	case animMsg:
		m.animating = false
		return m, m.animate()
	case resultMsg:
		if m.tally(proto.Result(msg)) {
			return m, m.recv()
		}
		if text := describe(proto.Result(msg)); text != "" {
			m.flash = tilde(text)
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
		if m.batch != nil {
			// Their results went with the connection.
			m.flash += fmt.Sprintf("; %d removals have no answer, r on Git rescans", len(m.batch.ids))
			m.batch = nil
		}
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
	case tea.MouseClickMsg:
		return m.tap(msg.Mouse())
	case tea.MouseWheelMsg:
		return m.wheel(msg.Mouse())
	case tea.KeyPressMsg:
		if m.splashing() {
			m.splashDone = true // any key stops the start screen, and does nothing else
			return m, nil
		}
		m.armed = "" // a key between two taps: the next tap shows again
		if m.input != nil {
			return m.typing(msg)
		}
		if m.searching {
			return m.search(msg)
		}
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
	if m.pending != nil && m.pending.door != "" && m.pending.which != "secondary" {
		if mm, ok := m.scrollPreview(k); ok {
			return mm, nil
		}
	}
	if m.pending != nil {
		a := *m.pending
		m.pending = nil
		// A destructive action needs a typed y; Enter alone is not consent.
		if k == "y" || (k == "enter" && !a.destructive) {
			return m.confirm(a)
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
		m.setTab((m.tab + 1) % len(tabNames))
	case "shift+tab":
		m.setTab((m.tab + len(tabNames) - 1) % len(tabNames))
	case "/":
		m.searching, m.filter = true, ""
		m.refresh()
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.refresh()
		}
	case "s":
		if s, _ := m.sortable(); s != nil {
			_, names := m.sortable()
			m.sortOn((s.col + 1) % len(names))
		}
	case "S", "shift+s":
		if s, _ := m.sortable(); s != nil {
			m.sortOn(s.col)
		}
	case "space", " ":
		if m.tab == tabGit {
			m.mark()
		}
	case "e":
		if m.tab == tabSessions {
			m.showExited = !m.showExited
		}
	case "X", "shift+x":
		if m.tab == tabGit {
			m.markAllSafe()
		}
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "enter":
		m.pending, m.previewTop = m.primary(), 0
	case "x":
		m.pending = m.secondary()
		if m.pending == nil && m.tab == tabSessions {
			m.flash = "x ends a stale session: one idle 7 days or more"
		}
	case "a":
		if pane, why := m.answerTarget(); pane != "" {
			m.input = &answerInput{pane: pane}
		} else {
			m.flash = why
		}
	case "r":
		if m.tab == tabGit && m.conn != nil && !m.lost {
			m.flash = "scanning git…"
			return m, m.send(proto.Act{RecID: "rescan:git", Which: "primary"})
		}
	default:
		if len(k) == 1 && k[0] >= '1' && int(k[0]-'1') < len(tabNames) {
			m.setTab(int(k[0] - '1'))
		}
	}
	return m, nil
}

// confirm runs an action the person consented to: a Nav step here, any
// other on the service.
func (m Model) confirm(a action) (tea.Model, tea.Cmd) {
	if len(a.batch) > 0 {
		if m.conn == nil || m.lost {
			m.flash = "not sent, no connection: " + a.String()
			return m, nil
		}
		return m.sendBatch(a)
	}
	if a.attach {
		return m, m.attach(a)
	}
	if a.term {
		return m.runDoor(a)
	}
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
	return m, m.send(proto.Act{RecID: a.rec, Which: a.which, Confirm: confirm, Text: a.text})
}

// navAllowed is what a Nav step may be: it runs here on one key, so a
// service can never make the console run anything but the two tmux moves
// advice builds, in exactly that shape. tmux runs a trailing argument of
// new-window as a shell command, splits commands on ";", and format-expands
// a start directory (#() runs a shell command). So: four arguments, a pane
// id for switch-client, and an absolute directory without "#" or ";".
func navAllowed(argv []string) bool {
	if len(argv) != 4 || argv[0] != "tmux" {
		return false
	}
	target := argv[3]
	switch argv[1] {
	case "switch-client", "select-window", "select-pane", "attach-session":
		return argv[2] == "-t" && paneID(target)
	case "new-window":
		return argv[2] == "-c" && filepath.IsAbs(target) && !strings.ContainsAny(target, "#;")
	}
	return false
}

// paneID is a tmux pane id such as %12.
func paneID(s string) bool {
	if len(s) < 2 || s[0] != '%' {
		return false
	}
	for _, r := range s[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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

// jump goes to a pane: inside tmux the client switches to it; outside, the
// console gives the terminal to `tmux attach` and comes back after it.
func (m Model) jump(pane string) *action {
	if m.opt.OutsideTmux {
		return &action{label: "attach", nav: true, attach: true, steps: [][]string{
			{"tmux", "select-window", "-t", pane},
			{"tmux", "select-pane", "-t", pane},
			{"tmux", "attach-session", "-t", pane},
		}}
	}
	return &action{label: "jump", nav: true, steps: [][]string{{"tmux", "switch-client", "-t", pane}}}
}

// attach runs the moves, then gives the terminal to the last step.
func (m Model) attach(a action) tea.Cmd {
	for _, step := range a.steps {
		if !navAllowed(step) {
			return func() tea.Msg {
				return ranMsg{strings.Join(step, " "), fmt.Errorf("refused: only tmux moves run in the console")}
			}
		}
	}
	moves, last := a.steps[:len(a.steps)-1], a.steps[len(a.steps)-1]
	if r := runNav(action{steps: moves}, m.opt.Run); r.err != nil {
		return func() tea.Msg { return r }
	}
	return tea.ExecProcess(exec.Command(last[0], last[1:]...), func(err error) tea.Msg { //nolint:gosec // argv checked by navAllowed
		return ranMsg{strings.Join(last, " "), err}
	})
}

// typing edits the answer line. Every printable key is text, q too.
func (m Model) typing(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	in := *m.input
	switch k.String() {
	case "esc":
		m.input, m.flash = nil, "cancelled: answer"
		return m, nil
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "enter":
		m.input = nil
		if strings.TrimSpace(in.text) == "" {
			m.flash = "cancelled: the answer is empty"
			return m, nil
		}
		m.pending = &action{label: "answer", steps: answerSteps(in.pane, in.text), destructive: true,
			rec: "answer:" + in.pane, which: "secondary", text: in.text}
		return m, nil
	case "backspace":
		in.text = trimLast(in.text)
	default:
		if k.Text != "" && len([]rune(in.text)) < 500 {
			in.text += k.Text
		}
	}
	m.input = &in
	return m, nil
}

func clampMove(i, d, n int) int { return min(max(i+d, 0), max(n-1, 0)) }

func (m *Model) move(d int) {
	if n := m.rowCount(); n > 0 {
		m.selectRow(clampMove(m.rowIndex(), d, n))
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

// selectedRec is the recommendation selected under the queue.
func (m Model) selectedRec() (advice.Rec, bool) {
	if m.tab != tabQueue {
		return advice.Rec{}, false
	}
	i := m.queueIndex() - len(m.doors) - len(m.queue)
	if i < 0 || i >= len(m.recs) {
		return advice.Rec{}, false
	}
	return m.recs[i], true
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
	v.MouseMode = tea.MouseModeCellMotion // a tap on a phone selects and acts
	return v
}
