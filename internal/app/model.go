// Package app is the console: a Bubble Tea program that keeps a copy of the
// state the service sends and draws it. It does not read the machine; it
// asks the service to act, and runs only the steps that move its own
// terminal.
package app

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/accounts"
	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/history"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/remote"
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
	// CompactAt and CompactTokens colour context at the compact threshold.
	CompactAt     float64
	CompactTokens int
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
	// Prefix is this machine's tmux prefix, the key of the way back from a
	// jump to a host. Nil or "": C-b.
	Prefix func() string
	// Home is the tmux session that holds the console, where the way back
	// lands. Nil or "": matchblox.
	Home func() string
	// LocalTmux is this machine's tmux version, as tmux -V says it without
	// "tmux ". "": unknown, and taken as new enough.
	LocalTmux string
	// JumpID names a jump's sessions. Nil: remote.NewJumpID.
	JumpID func() string
	// Check runs the login check (remote.CheckArgv) until ctx ends, gives
	// each line of its stderr to line as ssh prints it, and gives its
	// stderr and exit. Nil: ssh.
	Check func(ctx context.Context, argv []string, line func(string)) (stderr string, exit int)
	// MasterBase holds the ssh masters of the jumps, in MasterBase/ssh:
	// /tmp/matchblox-<uid>. "": no master, and each jump logs in again.
	MasterBase string
	// Ask gives the prompts of the ssh the console starts, for the ask
	// field. Nil: an ssh asks in the terminal, as NoAsk says once.
	Ask AskSource
	// NoAsk is why Ask is nil; "": it says nothing.
	NoAsk string
	// SSH runs an ssh of the builder's own login that the console starts
	// (a master's check, its open), with env added, until ctx ends, gives
	// each line of its stderr to line as ssh prints it, and gives its
	// stderr and exit. Nil: ssh, for 5 minutes at most.
	SSH func(ctx context.Context, argv, env []string, line func(string)) (stderr string, exit int)
	// Keys finds and sets up the builder's own key for the login to the
	// host (design §3.3). Nil: the console offers no key.
	Keys *Keys
	// Here is the root of the Git repo the console was launched in; "" when
	// it was launched elsewhere, or over SSH and from the phone, where the
	// launch folder means nothing. Sessions' ghost row starts there.
	Here string
	// Host is the host this console reaches over SSH; "" is this machine.
	// Its tmux moves and door commands run there, with the builder's own
	// ssh login.
	Host string
	// DialErr is why the first dial failed; the console starts with it.
	DialErr error
	// Peers are the other ticked hosts, in Hosts order.
	Peers []Peer
	// Self is this binary, which connects a host (`matchblox connect`).
	Self string
	// Gen tells this console's redial messages from those of a console the
	// shell closed.
	Gen int
	// Layered: the console sits under the hosts layer, and esc with
	// nothing to cancel or clear goes back to it.
	Layered bool
	// StateDir keeps what the person did to the recommendations (dismissed,
	// folded). "": kept for this run only.
	StateDir string
	// Exec gives this terminal to argv until it exits. Nil: tea.ExecProcess.
	Exec func(argv []string, done func(error) tea.Msg) tea.Cmd
	// Pair writes and reads the pair page's one-time key on this machine.
	// Nil: this console cannot pair.
	Pair *PairHost
	// LastClient is the tty and the creation time of the client that was
	// active last in the home session. Nil: ask tmux. "" or an error: the
	// builder's.
	LastClient func() (tty string, created time.Time, err error)
	// Clients are the ttys of the clients of this console's tmux server, for
	// a focus that moves the phone's client. Nil: ask tmux.
	Clients func() ([]string, error)
	// Clip copies text to the builder's clipboard. Nil: OSC 52 with the output.
	Clip func(text string)
	// Graphics says whether every terminal that shows the console draws
	// images, which they are, and whether any shows it (logos.Viewers).
	// Nil: never.
	Graphics func() (can bool, viewers string, watched bool)
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

// The redial messages carry the generation of the console that asked:
// after the shell opens another console, a late one is dropped.
type redialMsg struct{ gen int }

// silentMsg fires when a connection gave no state in silentAfter.
type silentMsg struct{ conn transport.Conn }
type redialFailed struct {
	err error
	gen int
}
type connMsg struct {
	conn transport.Conn
	gen  int
}
type ranMsg struct {
	cmd    string
	err    error
	jumped string // the pane a jump went to
}

// action waits for the person's confirm. A Nav action runs here; any other
// is sent to the service by name.
type action struct {
	label       string
	steps       [][]string
	destructive bool
	nav         bool
	attach      bool                        // the last step takes the terminal until it exits
	rec         string                      // the service's name for it
	which       string                      // primary | secondary
	text        string                      // what an answer types; a door's preview Sum
	batch       []string                    // worktrees: one guarded act each
	say         string                      // what the confirm shows, when not the first step
	term        bool                        // the step takes this terminal (a door's command)
	pair        bool                        // the pair door: it opens the pair page, never a pending step
	door        string                      // the door it opens or closes
	remote      bool                        // steps[0] is the ssh argv that runs it on the host, checked
	connect     bool                        // it connects the host: install, update or authorize
	pane        string                      // a jump: the pane it goes to
	master      string                      // a jump: the master directory the console opens first; "" none
	undo        []string                    // a jump: ends what a failed steps[0] made; nil none
	local       func(Model) (Model, string) // it runs in this console; its string is the flash
	again       string                      // a kill or switch of a session that works or waits: what the second y's confirm says
	againText   string                      // the Text the second y sends ("": killAnyway)
	done        string                      // a switch: what the flash says when it ran
	agent       string                      // a start on a new account: the last agent once it ran
	previewAct  string                      // switch evidence request; confirmation waits for it
	preview     string                      // checked evidence excerpt for switch consent
	climb       *climb                      // a rung of the key ladder
	ghost       bool                        // a ghost row of Sessions: it opens N on where
	where       string                      // a ghost row: the folder N opens on ("": N as it is)
	host        string                      // the host of the row it acts on; "": the home host, as a peer is never ""
}

// recRun is an act on a recommendation: the row shows its outcome.
type recRun struct {
	rec, act string // the rec, and the act id ("nav": run in the console)
	key      string // the key that runs it again
	out      string // what came of it; "" while it runs
	failed   bool
}

func (m Model) isRec(id string) bool {
	for _, r := range m.recs {
		if r.ID == id {
			return true
		}
	}
	return false
}

func (m *Model) startRec(a action, act string) {
	if a.rec == "" || !m.isRec(a.rec) {
		return
	}
	key := m.keys.main.Open.Help().Key
	if a.which == "secondary" {
		key = m.keys.main.Second.Help().Key
	}
	m.recRun = recRun{rec: a.rec, act: act, key: key}
}

// endRec turns the row of the act into its outcome.
func (m *Model) endRec(act string, r proto.Result) {
	if m.recRun.act != act || act == "" {
		return
	}
	again := " · " + m.recRun.key + " tries again"
	switch {
	case r.Err != "":
		m.recRun.out, m.recRun.failed = "failed: "+r.Err+again, true
	case len(r.Skipped) > 0:
		m.recRun.out, m.recRun.failed = "skipped: "+r.Skipped[len(r.Skipped)-1]+again, true
	default:
		m.recRun.out, m.recRun.failed = describe(r), false
	}
	m.recRun.act = ""
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
	recPrefs   recPrefs // dismissed recommendations and the fold
	opt        Options
	conn       transport.Conn
	peers      []link // the other ticked hosts, as Options.Peers; copied on write
	st         styles
	width      int
	height     int
	snap       sample.Snapshot
	push       string // the push sender's line for the Machine tab, or ""
	have       bool
	at         time.Time // the last stateMsg
	git        *proto.GitReport
	recs       []advice.Rec
	queue      []queue.Item
	doors      []doors.Door // the open doors, above the sessions
	stoker     *proto.StokerView
	doorPick   string // the selected door
	doorAt     int    // its row, for the door after it when it folds
	previewTop int    // the first line of a door's diff on screen
	paneSel    int
	peek       *peek              // the answer peek, over the screen
	peekSent   map[string]string  // the peek's sends in flight: act id -> the session's name
	peekQuiet  time.Time          // the peek takes no key until then: it just moved
	changes    map[paneKey]change // a match change that plays once
	animating  bool
	hist       history.Series
	selPID     int
	selHost    string // the host of selPID: PIDs repeat across hosts
	orphanPID  int
	recPick    string // the selected recommendation, above the sessions
	ghostPick  int    // the ghost row, then the blocked host's row, picked in Sessions, from 1; 0 is none
	chose      bool   // a session was picked; before, the first that waits is
	gitSel     int
	shipReport *proto.Ships
	shipSel    int
	tab        int
	pending    *action
	flash      string
	history    *series
	quitting   bool
	host       proto.Hello
	mismatch   bool
	blocked    error        // why the host cannot be reached, until the builder acts
	login      remote.Login // the builder's own ssh login to the host, as the check found it
	checking   bool         // the login check runs
	checked    bool         // the login check started once
	jumpLater  *action      // the jump that waits for the login check
	opening    bool         // the master of a jump opens, its prompts in the console
	notNow     bool         // the key ladder offers nothing more: not now, or the key is on the host
	lost       bool
	backoff    time.Duration
	redialing  bool
	answered   bool // a state came on the current connection
	replaced   bool
	acts       int
	recRun     recRun // the last act on a recommendation, and what came of it
	dark       bool
	logos      bool   // USAGE draws the providers' logos
	gfxKey     string // the viewers and background the logos went to
	gfxBusy    bool   // a question about who shows the console waits
	gfxAsks    int    // questions asked, for the tests
	splashAt   time.Time
	splashDone bool
	armed      string // the step a second tap runs: what the first tap showed
	// held is what the builder pressed or tapped while the stoker was in
	// charge: it runs once they answer the take-over question.
	held        func(Model) (tea.Model, tea.Cmd)
	touched     time.Time // the last key that kept the stoker's rest
	all         lists     // what every host sent, merged; queue, recs and snap hold the drawn copies
	homeAll     lists     // what the home host sent
	filter      string    // the search of the open tab
	searching   bool      // the search line takes the keys
	sessSort    sortBy
	sessGroup   int // what Sessions is grouped by: groupNeed .. groupNone
	holds       []proto.Hold
	reasons     map[string]proto.IdleReason
	agents      []string                               // the agents a new session can start on the host
	renames     map[string]string                      // the names sessions take at their next rest, by pane
	accounts    map[string][]string                    // the accounts each agent can start on: default first
	skipped     map[string][]accounts.Skip             // the folders the service did not list, per agent
	lastAccount map[string]string                      // the account last started, per agent
	signIns     map[string]string                      // how a new account's sign-in goes, by pane: "by hand" when matchblox gave up driving it
	found       *proto.Found                           // the host's repositories and the scan's consent; nil: the service scans nothing
	scanLater   bool                                   // the scan question was closed or answered at this console
	scanAct     string                                 // the act of the box's yes, for its result
	form        *form                                  // the open New session, Rename, Switch agent or scan folders form; nil none
	jumpAfter   struct{ act, host, pane, done string } // a switch sent: the pane to jump to when it runs
	startAfter  struct{ act, agent, done string }      // a start on a new account sent: its agent, the last once it runs, and what it says then
	lastAgent   string                                 // the agent this console last started
	keptAgent   string                                 // the agent last started, as the service keeps it
	paneSort    sortBy
	picked      map[string]bool // worktrees x removes
	batch       *batch
	showExited  bool // e: the exited agents, one by one
	// dockPanel is the panel a phone's dock shows (usage, the machine, the
	// stoker), u cycling it; dockSwitched: u was pressed while the stoker
	// claimed the dock, so it no longer takes it by itself.
	dockPanel    int
	dockSwitched bool
	// stokerSel is the selected row of the STOKER tab, stokerShown the row
	// whose command it shows, ackSent when the steps were last marked seen.
	stokerSel    int
	stokerShown  string
	ackSent      time.Time
	sheet        *sheet       // the launch sheet, while it is open
	stokerServer string       // the tmux server (-L) the stoker agent runs on; "" none
	toast        string       // what an action did, on the header's right
	toasts       int          // counts the toasts: a later one outlives the timer of an earlier one
	devices      []PairDevice // the keys that open this host, read when the Machine tab opens
	devErr       error        // why the keys could not be read
	devPick      string       // the selected device row: its fingerprint, or devPlus
	pair         *pairPage    // the pair page, while it shows
	pairPolling  bool         // a poll of the pair page is on its way
	pairOffers   *pairWaits   // the offers of a key that still run: the end of the program waits for them
	keys         keymaps      // the keys of every layer, as data

	// scanAsk is the scan question, open at this console; nil none.
	// scanAskUntil ends its quiet; scanUp: it was on top at the last
	// message, so one that shows it again starts the quiet.
	scanAsk      *scanAsk
	scanAskUntil time.Time
	scanUp       bool
	// used is where matchblox started sessions; wrepos the known repos,
	// for N's where list: built when what it reads changes (wfrom), not
	// at each snapshot or draw.
	used   proto.Used
	wrepos []whereRepo
	wfrom  []string
	wghost []whereRepo // the ghost rows' repos, built with wrepos
	// clones are the clones the service runs and the ones that ended in
	// the last minute (#176).
	clones []proto.Clone
	// stopAfter are the clone:starts esc came before: each clone stops
	// when the service takes its start. stopQuiet are the clone:stops
	// with no answer yet, whose answer says nothing.
	stopAfter []cloneStop
	stopQuiet []cloneStop
	focusAt   time.Time // the At of the last focus this console acted on

	// ask is the ssh prompt open in the console; nil none. askCtx lives as
	// long as the console: it ends the wait for prompts. askCancelled: the
	// builder cancelled the last one.
	ask          *askField
	askCtx       context.Context
	askStop      context.CancelFunc
	openStop     context.CancelFunc // ends the open-first that runs
	askCancelled bool

	// checkStop ends the login check that runs; checkCancelled: the builder
	// ended it at its Tailscale link. tailscale: the host is Tailscale SSH,
	// as its last check with the server's version line said. loginURL is
	// Tailscale's check link in the field, while the ssh that printed it
	// waits; "" none. Only remote.VisitURL gives it.
	checkStop      context.CancelFunc
	checkCancelled bool
	tailscale      bool
	loginURL       string
	// note is what the ladder said before the jump it held logs in: the
	// login covers the flash, and the jump that goes shows it again.
	note string
}

func New(opt Options) Model {
	if opt.CompactAt <= 0 {
		opt.CompactAt, opt.CompactTokens = sample.DefaultRules.CompactAt, sample.DefaultRules.CompactTokens
	}
	if opt.Run == nil {
		opt.Run = run
	}
	if opt.Binary == "" {
		opt.Binary = "dev"
	}
	m := Model{opt: opt, conn: opt.Conn, st: newStyles(true), dark: true, width: 100, height: 30, history: newSeries(60),
		recPrefs: loadRecPrefs(opt.StateDir), keys: defaultKeys(), peers: links(opt.Peers)}
	m.splashAt = m.now()
	if opt.Ask != nil {
		m.askCtx, m.askStop = context.WithCancel(context.Background())
	}
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
	var start []tea.Cmd
	for i := range m.peers {
		start = append(start, m.peerStart(i))
	}
	peers := tea.Batch(start...)
	if err := m.opt.DialErr; err != nil && m.conn == nil {
		return tea.Batch(tea.RequestBackgroundColor, func() tea.Msg { return lostMsg{err: err} }, m.splashCmd(), m.waitAsk(), peers)
	}
	return tea.Batch(tea.RequestBackgroundColor, m.hello(), m.recv(), m.watchSilence(), m.splashCmd(), m.graphicsTick(0), m.waitAsk(), peers)
}

func (m Model) watchSilence() tea.Cmd {
	c := m.conn
	return tea.Tick(silentAfter, func(time.Time) tea.Msg { return silentMsg{c} })
}

func (m Model) hello() tea.Cmd { return helloOn(m.conn, m.opt.Binary) }

func helloOn(c transport.Conn, bin string) tea.Cmd {
	if c == nil {
		return nil
	}
	return func() tea.Msg {
		if err := c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version, Binary: bin, Client: proto.ClientConsole}); err != nil {
			return lostMsg{err, c}
		}
		return nil
	}
}

// recv reads one message. Each handled message asks for the next, so one
// read is in flight at a time.
func (m Model) recv() tea.Cmd { return recvOn(m.conn) }

func recvOn(c transport.Conn) tea.Cmd {
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

// send sends a to the home host: the acts that are no row's (doors, the
// scan, clones, recommendations, pairing) stay the home host's.
func (m *Model) send(a proto.Act) tea.Cmd { return m.sendTo(m.opt.Host, a) }

// sendTo sends a to host: the home host on its own conn, a peer through
// sendPeer. A host that is not ticked, or a peer with no conn, gets
// nothing: the caller asked refusal first.
func (m *Model) sendTo(host string, a proto.Act) tea.Cmd {
	if host == m.opt.Host {
		_, cmd := m.sendOn(m.conn, a)
		return cmd
	}
	i := m.peerAt(host)
	if i < 0 || m.peers[i].conn == nil {
		return nil
	}
	return m.sendPeer(i, a)
}

// hostOf is the host a runs on: its row's, else the home host.
func (m Model) hostOf(a action) string { return cmp.Or(a.host, m.opt.Host) }

// sendOn sends a on c with the next act id. One counter serves every host,
// so an act id names one act in the console.
func (m *Model) sendOn(c transport.Conn, a proto.Act) (string, tea.Cmd) {
	m.acts++
	id := "a" + strconv.Itoa(m.acts)
	return id, func() tea.Msg {
		if err := c.Send(proto.KindAct, id, a); err != nil {
			return lostMsg{err, c}
		}
		return nil
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m = m.scanShows()
	next, cmd := m.update(msg)
	if nm, ok := next.(Model); ok {
		next = nm.scanShows()
	}
	return next, cmd
}

// scanShows starts the box's quiet when it is on top and was not: asked
// just now, or a layer over it closed, or the start screen ended.
func (m Model) scanShows() Model {
	up := m.scanShown()
	if up && !m.scanUp {
		m = m.armScan()
	}
	m.scanUp = up
	return m
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		return m.update(msg.msg)
	case peerMsg:
		return m.peerUpdate(msg)
	case peerRedialMsg:
		return m.peerRedial(msg)
	case staleMsg:
		return m, nil // a lost host turns stale: the frame draws it
	case helloMsg:
		m.host = proto.Hello(msg)
		m.host.Tmux = remote.HostTmux(m.host.Tmux)
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
		next, check := m.checkLogin()
		return next, tea.Batch(next.recv(), check)
	case loginMsg:
		return m.loggedIn(msg)
	case urlMsg:
		return m.linkShown(msg)
	case masterMsg:
		return m.masterOpened(msg)
	case stepMsg:
		return m.foundStep(msg)
	case keyMadeMsg:
		return m.keyMade(msg)
	case installedMsg:
		return m.installed(msg)
	case askMsg:
		return m.asked(msg)
	case askGoneMsg:
		return m.askGone(msg)
	case stateMsg:
		first := !m.have
		st := proto.State(msg)
		m.queue = st.Queue
		toast := m.stokerNews(st.Stoker)
		m.stoker, m.stokerServer = st.Stoker, st.StokerServer
		if !m.stokerClaims() {
			m.dockSwitched = false
		}
		m.doors = m.shownDoors(st.Doors)
		m.noteChanges(m.opt.Host, st.Sessions)
		m.snap, m.git, m.recs, m.have, m.answered, m.at = st.Snapshot, st.Git, st.Recommendations, true, true, m.now()
		m.shipReport = st.Ships
		m.holds, m.reasons = st.Holds, st.Reasons
		m.agents, m.renames, m.accounts, m.signIns = st.Agents, st.Renames, st.Accounts, st.SignIns
		m.skipped = st.AccountsSkipped
		m.lastAccount, m.keptAgent = st.LastAccount, st.LastAgent
		if st.Found != nil {
			m.found = st.Found
			switch {
			case st.Found.Consent != "":
				m.scanAsk = nil // answered, here or at another console
			case !m.scanLater && m.scanAsk == nil:
				m.scanAsk = &scanAsk{}
			}
		}
		if st.Used != nil {
			m.used = *st.Used
		}
		m.homeAll = lists{queue: st.Queue, recs: st.Recommendations, sessions: st.Sessions, orphans: st.Orphans, children: st.Children}
		if from := m.whereFrom(m.opt.Host); st.Found != nil || st.Used != nil || !slices.Equal(from, m.wfrom) {
			m.wrepos, m.wfrom = m.whereRepos(m.opt.Host), from
			m.wghost = m.ghostRepos()
		}
		m.clones = st.Clones
		m.push = st.Push
		var again, started tea.Cmd
		if m.lost { // the clone acts' answers went with the connection
			again = m.cloneAgain()
		}
		if m.form != nil && m.form.cloning != "" {
			started = m.cloneNews()
		}
		if m.form != nil && m.form.whereList() && !m.form.cloneList() {
			f := *m.form
			m.form = &f
			sel := m.whereSelected().path()
			m.refreshWhere(&f, sel)
		}
		if m.recRun.rec != "" && m.recRun.act == "" && !m.isRec(m.recRun.rec) {
			m.recRun = recRun{} // its rec is gone: a later one with the same id starts fresh
		}
		m.merge()
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
		m, focus := m.takeFocus(st.Focus)
		var news, order tea.Cmd
		if toast != "" && !first {
			news = m.say(toast)
		}
		if m.sheet != nil && m.sheet.queued && m.sheet.act == "" && m.stoker != nil && m.stoker.Ready {
			m, order = m.sendOrder() // the stoker rests now: the order that waited for it goes
		}
		return m, tea.Batch(m.recv(), m.animate(), news, order, focus, again, started)
	case graphicsMsg:
		return m.graphics(msg)
	case graphicsTickMsg:
		return m.askGraphics(msg)
	case doorRanMsg:
		return m.doorRan(msg)
	case pairOfferMsg:
		return m.offered(msg)
	case pairPollMsg:
		return m.pairPolled()
	case pairFrameMsg:
		if m.pairMoving() {
			return m, pairFrame()
		}
	case animMsg:
		m.animating = false
		return m, m.animate()
	case toastMsg:
		if msg.n == m.toasts {
			m.toast = ""
		}
	case planTickMsg:
		return m.planTick(msg)
	case resultMsg:
		mm, cmd := m.result(proto.Result(msg))
		return mm, tea.Batch(mm.recv(), cmd)
	case lostMsg:
		if m.mismatch || m.quitting || m.redialing || (msg.conn != nil && msg.conn != m.conn) {
			return m, nil
		}
		if m.conn != nil {
			_ = m.conn.Close()
		}
		first := !m.lost
		m.lost = true
		if blocking(msg.err) {
			// Trying again cannot help: connecting the host can.
			m.blocked, m.flash = msg.err, ""
			return m, nil
		}
		// Two or more hosts: a short loss shows nothing, as a peer's;
		// from staleAfter the rows and the key line say so.
		var lost []string
		if m.hostCount() == 1 {
			lost = append(lost, "connection lost: "+msg.err.Error())
		}
		if m.batch != nil {
			// Their results went with the connection.
			lost = append(lost, fmt.Sprintf("%d removals have no answer, %s on Git rescans", len(m.batch.ids), m.keys.main.Rename.Help().Key))
			m.batch = nil
		}
		var stale tea.Cmd
		if first {
			stale = m.staleTick(m.at)
		}
		m.flash = strings.Join(lost, "; ")
		if m.opt.Redial == nil {
			return m, stale
		}
		m.redialing = true
		m.backoff = redialAfter(m.backoff)
		if m.hostCount() == 1 {
			m.flash += "; trying again in " + m.backoff.String()
		}
		gen := m.opt.Gen
		return m, tea.Batch(stale, tea.Tick(m.backoff, func(time.Time) tea.Msg { return redialMsg{gen} }))
	case redialMsg:
		if msg.gen != m.opt.Gen || !m.redialing || m.quitting {
			return m, nil // a tick from a console that was closed
		}
		redial, gen := m.opt.Redial, m.opt.Gen
		return m, func() tea.Msg {
			c, err := redial()
			if err != nil {
				return redialFailed{err, gen}
			}
			return connMsg{c, gen}
		}
	case silentMsg:
		if msg.conn != m.conn || m.answered || m.mismatch {
			return m, nil
		}
		if m.opt.Host != "" {
			m.flash = m.opt.Host + " does not answer: run ssh " + m.opt.Host + " matchblox status to see why"
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
		if msg.gen != m.opt.Gen {
			return m, nil
		}
		m.redialing = false
		return m.Update(lostMsg{err: msg.err})
	case connectedMsg:
		return m.connected(msg)
	case connMsg:
		if msg.gen != m.opt.Gen || m.quitting {
			_ = msg.conn.Close() // the console that dialed it is gone
			return m, nil
		}
		if m.hostCount() == 1 || m.stale(m.opt.Host) {
			m.flash = "connected again" // a short loss of two or more hosts showed nothing: its end shows nothing
		}
		m.conn, m.redialing, m.answered = msg.conn, false, false
		return m, tea.Batch(m.hello(), m.recv(), m.watchSilence())
	case ranMsg:
		if msg.err != nil {
			m.endRec("nav", proto.Result{Err: msg.cmd + ": " + msg.err.Error()})
		} else {
			m.endRec("nav", proto.Result{Ran: [][]string{strings.Fields(msg.cmd)}})
		}
		if msg.err != nil {
			m.flash = "failed: " + msg.cmd + ": " + msg.err.Error()
			return m, nil
		}
		if msg.jumped != "" {
			m = m.nextAfter(msg.jumped)
		}
		toast := m.say("ran: " + msg.cmd)
		return m, toast
	case tea.PasteMsg:
		// Only the ssh prompt and the form take a paste: in their field,
		// as typed.
		if m.ask != nil {
			return m.askPaste(msg.Content), nil
		}
		if m.focus(tea.KeyPressMsg{}) == layerForm {
			return m.formPaste(msg.Content)
		}
		return m, nil
	case tea.MouseClickMsg:
		m, touch := m.touchRest()
		next, cmd := m.tap(msg.Mouse())
		return next, tea.Batch(touch, cmd)
	case tea.MouseWheelMsg:
		m, touch := m.touchRest()
		next, cmd := m.wheel(msg.Mouse())
		return next, tea.Batch(touch, cmd)
	case tea.KeyPressMsg:
		if m.splashing() {
			m.splashDone = true // any key stops the start screen, and does nothing else
			return m, nil
		}
		m.armed = "" // a key between two taps: the next tap shows again
		var touch tea.Cmd
		// f and n are stoker acts themselves: a touch sent with them would
		// race them on the service.
		if m.pair != nil || m.peek != nil || m.sheet != nil || m.form != nil || m.searching || !bkey.Matches(msg, m.keys.main.Stoke, m.keys.main.Night) {
			m, touch = m.touchRest()
		}
		f := m.focus(msg)
		// ctrl+c quits from every layer but a question, where any key but
		// its answers cancels, and a peek on a gone session, which closes.
		if f != layerAsk && f != layerHeld && f != layerPending && f != layerPeekGone && bkey.Matches(msg, m.keys.global.Quit) {
			next, cmd := m.quit()
			if f == layerMain {
				next, cmd = stokerSeen(next, cmd) // as every key of the tabs
			}
			return next, tea.Batch(touch, cmd)
		}
		next, cmd := m.press(f, msg)
		return next, tea.Batch(touch, cmd)
	}
	return m, nil
}

// result ends the act r answers. The caller reads the next message of the
// conn r came on.
func (m Model) result(r proto.Result) (Model, tea.Cmd) {
	if m.pending != nil && m.pending.previewAct != "" && m.pending.previewAct == r.ActID {
		if r.Err != "" || r.Preview == "" {
			m.pending = nil
			m.flash = "handover preview unavailable: " + cmp.Or(r.Err, "update matchblox and try again")
			return m, nil
		}
		a := *m.pending
		a.previewAct, a.preview = "", r.Preview
		m.pending = &a
		return m, nil
	}
	m.endRec(r.ActID, r)
	if mm, ok := m.started(r); ok {
		return mm, nil
	}
	if mm, cmd, ok := m.formResult(r); ok {
		return mm, cmd
	}
	if mm, ok := m.scanResult(r); ok {
		return mm, nil
	}
	if mm, ok := m.sheetResult(r); ok {
		if r.Err == "" && mm.sheet == nil {
			toast := mm.say("the order is on")
			return mm, toast
		}
		return mm, nil
	}
	if name := m.peekSent[r.ActID]; name != "" {
		m.peekSent = maps.Clone(m.peekSent)
		delete(m.peekSent, r.ActID)
		why := r.Err
		if n := len(r.Skipped); n > 0 && why == "" {
			why = r.Skipped[n-1]
		}
		if why != "" {
			m.toast, m.flash = "", tilde("not sent to "+name+": "+why)
			return m, nil
		}
		toast := m.say("sent to " + name)
		return m, toast
	}
	if r.ActID != "" && r.ActID == m.jumpAfter.act {
		return m.switched(r)
	}
	if m.tally(r) {
		return m, nil
	}
	if text := describe(r); text != "" && r.Err == "" && len(r.Skipped) == 0 {
		toast := m.say(tilde(text))
		return m, toast
	} else if text != "" {
		m.flash = tilde(text)
	}
	return m, nil
}

// focusWait is how long a focus waits for the phone's tmux client to
// attach: the gate sends the act before it runs the console.
var focusWait = 10 * time.Second

// focusGone is what a focus on a session that ended says.
const focusGone = "That session ended."

// takeFocus opens the session a tap on the island or a buzz asked for, once
// for each focus the service sends, on Sessions: its answer peek when it
// waits, else its pane on the phone's own tmux client. A form, sheet, pair
// page, take-over or a peek with text typed keeps the keys: what the person
// types there is not thrown away.
func (m Model) takeFocus(f *proto.Focus) (Model, tea.Cmd) {
	if f == nil || f.At.Equal(m.focusAt) {
		return m, nil
	}
	m.focusAt = f.At
	var s sample.Session
	found := false
	for _, x := range m.snap.Sessions {
		if x.Host != m.opt.Host {
			continue // the focus is the home host's
		}
		if f.SessionID != "" && x.SessionID == f.SessionID || f.SessionID == "" && f.Pane != "" && x.Pane == f.Pane {
			s, found = x, true
			break
		}
	}
	typing := m.peek != nil && m.peek.text != ""
	busy := m.pair.owns() || m.sheet != nil || m.form != nil || m.held != nil || typing
	if f.Gone || !found {
		if !busy {
			m.setTab(tabSessions)
			m.pending = nil
		}
		m.flash = focusGone
		return m, nil
	}
	if busy {
		return m, nil
	}
	m.setTab(tabSessions)
	m.pending, m.peek = nil, nil
	m.doorPick, m.recPick, m.ghostPick, m.chose, m.selPID, m.selHost = "", "", 0, true, s.PID, s.Host
	if s.Pane == "" {
		return m, nil // outside tmux: no pane to open
	}
	if _, waits := m.waitOf(s); waits {
		// The home console is shared: the peek shows on the phone too, and an
		// Enter on its way at the desk must not answer it.
		return m.peekTo(paneOf(s)), nil
	}
	// Only the phone's client moves, into a session of its own (FocusArgv),
	// and only on this console's own tmux: a console of another host (over
	// ssh) or outside tmux never moves a terminal for a tap on the phone.
	if m.opt.Host != "" || m.opt.OutsideTmux || !remote.ClientTTY(f.Client) {
		return m, nil
	}
	return m, focusJump(m.clients(), m.opt.Run, f.Client, s.Pane)
}

// clients is how this console reads the clients of its tmux server.
func (m Model) clients() func() ([]string, error) {
	if m.opt.Clients != nil {
		return m.opt.Clients
	}
	return tmuxClients
}

// tmuxClients are the ttys of the clients of this console's tmux server.
func tmuxClients() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "list-clients", "-F", "#{client_tty}").Output()
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

// focusJump waits up to focusWait for the client on tty to attach to this
// console's tmux, then moves it to pane. A client that never comes is not
// this console's: nothing moves.
func focusJump(clients func() ([]string, error), run func([]string) error, tty, pane string) tea.Cmd {
	return func() tea.Msg {
		argv := remote.FocusArgv(tty, pane)
		if !remote.FocusAllowed(argv) {
			return nil
		}
		for deadline := time.Now().Add(focusWait); ; time.Sleep(100 * time.Millisecond) {
			if ttys, err := clients(); err == nil && slices.Contains(ttys, tty) {
				break
			}
			if time.Now().After(deadline) {
				return nil
			}
		}
		// The toast names the move, not the session the phone gets for it.
		return ranMsg{cmd: "tmux switch-client -c " + tty + " -t " + pane, err: run(argv), jumped: pane}
	}
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

// keyed is a key a tap on a key hint presses: it reaches the layers under
// the overlays, as a key does.
func (m Model) keyed(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return m.press(m.under(k), k)
}

// press gives k to the layer f, the one focus chose. A layer under the
// overlays marks the stoker's steps seen when the tab shows them.
func (m Model) press(f layerID, k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var next tea.Model = m
	var cmd tea.Cmd
	switch f {
	case layerAsk:
		next, cmd = m.askKey(k)
	case layerPair:
		next, cmd = m.pairKey(k)
	case layerPairStripEsc:
		next = m.closePairPage("")
	case layerSheet:
		next, cmd = m.sheetKey(k)
	case layerForm:
		next, cmd = m.formKey(k)
	case layerPeek, layerPeekGone:
		next, cmd = m.peekKey(k)
	case layerSearch:
		next, cmd = m.search(k)
	case layerHeld:
		next, cmd = m.answerTakeOver(k)
	case layerScan:
		next, cmd = m.answerScan(k)
	case layerTakeOver:
		m.held, m.flash = m.hold(k), ""
		next = m
	case layerPreview:
		next, _ = m.scrollPreview(k)
	case layerBlocked:
		next, cmd = m.unblock()
	case layerPending:
		next, cmd = m.pendingKey(k)
	case layerMain:
		next, cmd = m.key(k)
	}
	if f.underOverlays() {
		return stokerSeen(next, cmd)
	}
	return next, cmd
}

// stokerSeen is a key's result, with the stoker's steps seen when the tab shows them.
func stokerSeen(next tea.Model, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	if nm, ok := next.(Model); ok {
		nm, ack := nm.seeStoker()
		return nm, tea.Batch(cmd, ack)
	}
	return next, cmd
}

// unblock is ⏎ on a host that is blocked or speaks another version: a
// changed host key tries again, anything else shows the connect.
func (m Model) unblock() (tea.Model, tea.Cmd) {
	if errors.Is(m.blocked, remote.ErrHostKeyChanged) {
		return m.tryAgain()
	}
	m.pending = m.connectAction()
	return m, nil
}

// pendingKey answers the action that waits: y runs it, ⏎ a safe one, and
// any other key cancels.
func (m Model) pendingKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	a := *m.pending
	m.pending = nil
	yes := bkey.Matches(k, m.keys.confirm.Yes)
	if a.previewAct != "" && (yes || bkey.Matches(k, m.keys.confirm.Enter)) {
		m.pending = &a
		m.flash = "reading handover evidence: wait for the preview, or any other key cancels"
		return m, nil
	}
	if a.climb != nil && !yes {
		return m.declined(*a.climb, bkey.Matches(k, m.keys.main.Back))
	}
	if yes && a.again != "" {
		// It works or waits for you: one more y, said plainly.
		a.say, a.again, a.text = a.again, "", killAnyway
		if a.againText != "" {
			a.text = a.againText
		}
		m.pending = &a
		return m, nil
	}
	// A destructive action needs a typed y; Enter alone is not consent.
	if yes || (bkey.Matches(k, m.keys.confirm.Enter) && !a.destructive) {
		return m.confirm(a)
	}
	m.flash = "cancelled: " + a.String()
	return m, nil
}

// key is k on the tabs, with nothing on top.
func (m Model) key(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	mk := m.keys.main
	m.flash = ""
	switch {
	case bkey.Matches(k, mk.Stoker):
		return m.openStoker(), nil
	case bkey.Matches(k, mk.Quit):
		return m.quit()
	case bkey.Matches(k, mk.NextTab):
		m.setTab((m.tab + 1) % len(tabNames))
	case bkey.Matches(k, mk.PrevTab):
		m.setTab((m.tab + len(tabNames) - 1) % len(tabNames))
	case bkey.Matches(k, mk.Search):
		m.searching, m.filter = true, ""
		m.refresh()
	case bkey.Matches(k, mk.Back):
		if m.filter != "" {
			m.filter = ""
			m.refresh()
		} else if m.opt.Layered {
			return m, func() tea.Msg { return layerMsg{} }
		}
	case bkey.Matches(k, mk.Sort):
		if s, _ := m.sortable(); s != nil {
			_, names := m.sortable()
			m.sortOn((s.col + 1) % len(names))
		}
	case bkey.Matches(k, mk.SortBack):
		if s, _ := m.sortable(); s != nil {
			m.sortOn(s.col)
		}
	case bkey.Matches(k, mk.Group):
		if m.tab == tabSessions {
			m.sessGroup = (m.sessGroup + 1) % len(groupModes)
			m.flash = "group: " + groupModes[m.sessGroup]
			m.refresh()
		}
	case bkey.Matches(k, mk.Park):
		if s, ok := m.selectedSession(); ok {
			if s.Pane == "" {
				m.flash = s.Name + " is outside tmux: nothing to park"
				break
			}
			if why := m.refusal(s.Host); why != "" {
				m.flash = why
				break
			}
			cmd := m.sendTo(s.Host, proto.Act{RecID: "park:" + s.Pane})
			return m.pairActed(action{rec: "park:" + s.Pane, host: s.Host}), cmd
		}
	case bkey.Matches(k, mk.Mark):
		if m.tab == tabGit {
			m.mark()
		}
	case bkey.Matches(k, mk.Exited):
		if m.tab == tabSessions {
			m.showExited = !m.showExited
		}
	case bkey.Matches(k, mk.MarkSafe):
		if m.tab == tabGit {
			m.markAllSafe()
		}
	case bkey.Matches(k, mk.QuitAgent):
		if m.tab == tabStoker {
			return m.quitAgent()
		}
	case bkey.Matches(k, mk.Up):
		m.move(-1)
	case bkey.Matches(k, mk.Down):
		m.move(1)
	case bkey.Matches(k, mk.Open):
		switch m.tab {
		case tabShips:
			return m.toDriver(), nil
		case tabStoker:
			if r, ok := m.stokerSelected(); ok && r.kind == rowAgent {
				return m.openAgent()
			}
			m.showCommand()
			return m, nil
		}
		if b, ok := m.selectedBlocked(); ok {
			return m.unblockHost(b)
		}
		if a := m.primary(); a != nil && a.pair {
			return m.openPairDoor()
		}
		if a := m.primary(); a != nil && a.ghost {
			return m.openNewIn(a.where, ""), nil
		}
		m.pending, m.previewTop = m.primary(), 0
		m.refuse()
	case bkey.Matches(k, mk.Second):
		m.pending = m.secondary()
		m.refuse()
	case bkey.Matches(k, mk.New):
		return m.openNew()
	case bkey.Matches(k, mk.Switch):
		if m.tab == tabSessions {
			return m.openSwitch()
		}
	case bkey.Matches(k, mk.Grant):
		if m.tab == tabStoker {
			return m.grantStep()
		}
	case bkey.Matches(k, mk.Peek):
		if m.tab == tabShips {
			return m.shipAnswer(), nil
		}
		if at, why := m.peekTarget(); at.pane != "" {
			m.openPeek(at)
		} else {
			m.flash = why
		}
	case bkey.Matches(k, mk.Dismiss):
		if m.tab == tabSessions {
			m.dismissRec()
		}
	case bkey.Matches(k, mk.Fold):
		if m.tab == tabSessions {
			m.foldRecs()
		}
	case bkey.Matches(k, mk.Ships):
		m.setTab(tabShips)
	case bkey.Matches(k, mk.Dock):
		m.cycleDock()
	case bkey.Matches(k, mk.Stoke):
		return m.stokerKey(false)
	case bkey.Matches(k, mk.Night):
		return m.stokerKey(true)
	case bkey.Matches(k, mk.Order):
		return m.orderKey()
	case bkey.Matches(k, mk.Rename):
		if m.tab == tabSessions {
			return m.openRename()
		}
		if m.tab == tabGit && m.conn != nil && !m.lost {
			m.flash = "scanning git…"
			return m, m.send(proto.Act{RecID: "rescan:git", Which: "primary"})
		}
	case bkey.Matches(k, mk.Tabs):
		m.setTab(int(k.String()[0] - '1'))
	}
	return m, nil
}

// refuse drops the pending act when its host cannot take it, and says why.
func (m *Model) refuse() {
	a := m.pending
	if a == nil || a.rec == "" || a.nav || a.remote || a.term || a.local != nil {
		return
	}
	if why := m.refusal(m.hostOf(*a)); why != "" {
		m.pending, m.flash = nil, why
	}
}

// confirm runs an action the person consented to: a Nav step here, any
// other on the service.
func (m Model) confirm(a action) (tea.Model, tea.Cmd) {
	if a.climb != nil {
		return m.climbed(*a.climb)
	}
	if a.local != nil {
		next, msg := a.local(m)
		next.flash = msg
		return next, nil
	}
	if a.remote {
		if a.nav {
			return m.jumpAfterLogin(a)
		}
		return m.onHost(a)
	}
	host := m.hostOf(a)
	if (a.nav || a.attach) && host != "" {
		// A tmux move of a host names the host's panes: here it would move
		// this machine's tmux. Only a jump (remote) goes there.
		m.flash = "refused: a tmux move on " + host + " never runs on this machine"
		return m, nil
	}
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
		m.startRec(a, "nav")
		return m, func() tea.Msg { return runNav(a, run) }
	}
	if why := m.refusal(host); why != "" {
		m.flash = why
		return m, nil
	}
	if host == m.opt.Host && (m.conn == nil || m.lost) {
		m.flash = "not sent, no connection: " + a.String()
		return m, nil
	}
	confirm := ""
	if a.destructive {
		confirm = "y"
	}
	m.flash = "sent: " + a.String()
	cmd := m.sendTo(host, proto.Act{RecID: a.rec, Which: a.which, Confirm: confirm, Text: a.text})
	m.startRec(a, "a"+strconv.Itoa(m.acts))
	if pane, ok := strings.CutPrefix(a.rec, "switch:"); ok {
		m.jumpAfter.act, m.jumpAfter.host, m.jumpAfter.pane, m.jumpAfter.done = "a"+strconv.Itoa(m.acts), a.host, pane, a.done
	}
	if a.agent != "" {
		m.startAfter.act, m.startAfter.agent, m.startAfter.done = "a"+strconv.Itoa(m.acts), a.agent, a.done
	}
	return m.pairActed(a), cmd
}

// started takes the result of a start on a new account: one that ran sets
// the last agent and says where to sign in, with the notes of what its
// account shared or skipped; one that did not leaves the last agent as it
// was and goes on to the result's usual words (false).
func (m Model) started(r proto.Result) (Model, bool) {
	if r.ActID == "" || r.ActID != m.startAfter.act {
		return m, false
	}
	a := m.startAfter
	m.startAfter.act, m.startAfter.agent, m.startAfter.done = "", "", ""
	notes, ran := proto.StartNotes(r)
	if r.Err != "" || len(r.Ran) == 0 || len(r.Skipped) > 0 && !ran {
		return m, false
	}
	m.lastAgent = a.agent
	m.flash = a.done
	if len(notes) > 0 {
		m.flash += "; " + strings.Join(notes, "; ")
	}
	return m, true
}

// switched takes the result of a switch: one that ran jumps to its pane,
// one that did not says why.
func (m Model) switched(r proto.Result) (Model, tea.Cmd) {
	j := m.jumpAfter
	m.jumpAfter.act, m.jumpAfter.host, m.jumpAfter.pane, m.jumpAfter.done = "", "", "", ""
	// A switch with notes on what its account shared or skipped ran.
	notes, ran := proto.StartNotes(r)
	switch {
	case r.Err != "":
		m.flash = tilde(r.Err)
		return m, nil
	case len(r.Skipped) > 0 && !ran:
		m.flash = tilde(r.Skipped[len(r.Skipped)-1])
		return m, nil
	}
	next, jump := m.confirm(*m.jumpOn(j.host, j.pane))
	nm := next.(Model)
	nm.flash = j.done
	if ran {
		nm.flash += "; " + strings.Join(notes, "; ")
	}
	return nm, jump
}

func runNav(a action, run func([]string) error) ranMsg {
	for _, step := range a.steps {
		if !remote.NavAllowed(step) && !remote.PopupAllowed(step) {
			return ranMsg{cmd: strings.Join(step, " "), err: fmt.Errorf("refused: only tmux switch-client, new-window and the stoker's popup run in the console")}
		}
	}
	for _, step := range a.steps {
		if err := run(step); err != nil {
			return ranMsg{cmd: strings.Join(step, " "), err: err}
		}
	}
	return ranMsg{cmd: a.String(), jumped: a.pane}
}

// jump goes to a pane: inside tmux the client switches to it; outside, the
// console gives the terminal to `tmux attach` and comes back after it.
func (m Model) jump(pane string) *action {
	if m.opt.OutsideTmux {
		return &action{label: "attach", nav: true, attach: true, pane: pane, steps: [][]string{
			{"tmux", "select-window", "-t", pane},
			{"tmux", "select-pane", "-t", pane},
			{"tmux", "attach-session", "-t", pane},
		}}
	}
	return &action{label: "jump", nav: true, pane: pane, steps: [][]string{{"tmux", "switch-client", "-t", pane}}}
}

// jumpOn is jump to a pane of host.
func (m Model) jumpOn(host, pane string) *action {
	a := m.jump(pane)
	a.host = host
	return a
}

// attach runs the moves, then gives the terminal to the last step.
func (m Model) attach(a action) tea.Cmd {
	for _, step := range a.steps {
		if !remote.NavAllowed(step) {
			return func() tea.Msg {
				return ranMsg{cmd: strings.Join(step, " "), err: fmt.Errorf("refused: only tmux moves run in the console")}
			}
		}
	}
	moves, last := a.steps[:len(a.steps)-1], a.steps[len(a.steps)-1]
	if r := runNav(action{steps: moves}, m.opt.Run); r.err != nil {
		return func() tea.Msg { return r }
	}
	return tea.ExecProcess(exec.Command(last[0], last[1:]...), func(err error) tea.Msg { //nolint:gosec // argv checked by remote.NavAllowed
		return ranMsg{cmd: strings.Join(last, " "), err: err, jumped: a.pane}
	})
}

func clampMove(i, d, n int) int { return min(max(i+d, 0), max(n-1, 0)) }

func (m *Model) move(d int) {
	if n := m.rowCount(); n > 0 {
		m.selectRow(clampMove(m.rowIndex(), d, n))
	}
}

// selIndex is the selected session: the one picked, followed by its PID
// across re-sorts; before one is picked, the first that waits.
func (m Model) selIndex() int {
	if !m.chose {
		if i := m.firstWaiting(); i >= 0 {
			return i
		}
	}
	for i, s := range m.snap.Sessions {
		if s.PID == m.selPID && s.Host == m.selHost {
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

// keepSelection follows the selected session by host and PID across re-sorts.
func (m *Model) keepSelection() {
	if s, ok := m.selected(); ok {
		m.selPID, m.selHost = s.PID, s.Host
	}
}

// toastMsg ends the toast it counts.
type toastMsg struct{ n int }

// toastFor is how long a toast stays on the header.
const toastFor = 2500 * time.Millisecond

// say shows what an action did on the header's right, for toastFor.
func (m *Model) say(text string) tea.Cmd {
	m.toasts++
	m.toast = text
	n := m.toasts
	return tea.Tick(toastFor, func(time.Time) tea.Msg { return toastMsg{n} })
}

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion // a tap on a phone selects and acts
	return v
}

// nextAfter selects, after a jump to the selected session's pane, the
// next session that waits, else the next idle one. The way back to the
// console lands on it.
func (m Model) nextAfter(pane string) Model {
	if s, ok := m.selectedSession(); !ok || s.Pane != pane {
		return m
	}
	ss := m.snap.Sessions
	at := m.selIndex()
	for _, want := range []func(sample.Session) bool{
		func(s sample.Session) bool { return m.groupOf(s) == groupWaits },
		func(s sample.Session) bool { return s.Age() == "idle" },
	} {
		for k := 1; k < len(ss); k++ {
			if s := ss[(at+k)%len(ss)]; want(s) {
				m.ghostPick, m.chose, m.selPID, m.selHost = 0, true, s.PID, s.Host
				return m
			}
		}
	}
	return m
}
