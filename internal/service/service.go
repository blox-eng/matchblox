// Package service owns every loop, the state and the event stream. Consoles
// connect to it and draw what it sends; one service samples the machine
// once, however many consoles watch.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blox-eng/matchblox/internal/actions"
	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/gitremote"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/history"
	"github.com/blox-eng/matchblox/internal/holds"
	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/idle"
	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/push"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/recovery"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/setup"
	"github.com/blox-eng/matchblox/internal/ships"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/stoker"
	"github.com/blox-eng/matchblox/internal/transport"
)

// GitSource scans the repositories the given session directories belong to.
type GitSource func(sessionCwds []string) gitscan.Report

// ShipsSource reads the pull requests the sessions drive: their
// transcripts' new bytes, and GitHub when a poll is due. ctx ends with the
// service: a scan in flight stops with it.
type ShipsSource func(ctx context.Context, sessions []sample.Session, git *gitscan.Report) ships.Report

// shipsEvery is how often the transcripts are read for new evidence; the
// source reads GitHub at its own, slower cadence.
const shipsEvery = 15 * time.Second

// ErrReplaced means the binary on disk changed. The service stops so that
// the next console starts the new one.
var ErrReplaced = errors.New("service: replaced by a newer binary")

// ErrSocketGone means the socket file was removed or replaced. A console
// that cannot find this service starts another, so this one stops.
var ErrSocketGone = errors.New("service: the socket file is gone")

type Service struct {
	// Actions runs confirmed steps. New sets Check and Idle.
	Actions actions.Runner
	// StatePath gets the state file for `status` without a socket. Empty: none.
	StatePath string
	// HistoryLog gets one row every history.Every. Empty: none.
	HistoryLog string
	// HistorySources are other CSV files to chart.
	HistorySources []string
	// AlertHook runs with the title and evidence of each new alert.
	AlertHook []string
	// Binary is the version in the hello.
	Binary string
	// Host is the host name in the hello. Empty: os.Hostname().
	Host string
	// Spool holds the hook events sent while no service ran; Run reads and
	// empties it at start. Empty: none.
	Spool string
	// Exe is the path matchblox was started by, checked every ExeEvery with
	// the socket file; Serve stops when either changes. Empty Exe: no check.
	Exe      string
	ExeEvery time.Duration
	// ExeAt is Exe as the process started on it. Nil: the watch's first look
	// stands for it, so a binary replaced before that look is the one it
	// knows.
	ExeAt os.FileInfo
	// watching closes once Serve's watch has taken its first look at Exe
	// and the socket (Watching).
	watching     chan struct{}
	watchingOnce sync.Once
	// runDone closes when the Run that Serve started returns.
	runDone chan struct{}
	// Home is the builder's home directory, where the accounts live. Empty:
	// os.UserHomeDir().
	Home string
	// GOOS is the system the codex socket check is for. Empty: runtime.GOOS.
	GOOS string
	// Setup is the machine the doors are for. Nil: no doors (fixtures).
	Setup *setup.Env
	// Stoker keeps the stoker mode and its steps. Nil: no stoker.
	Stoker *stoker.Store
	// Holds are the parked and stoker-held sessions. Nil: none kept.
	Holds *holds.Store
	// Recovery is the record of the agents on the builder's tmux server,
	// for matchblox recover (#215). Nil: none kept.
	Recovery *recovery.Store
	// NightEnds is when a night turns the stoker off ("07:00").
	NightEnds string
	// StokerServer is the tmux server (-L) matchblox keeps the stoker agent
	// on, in the session sample.StokerSession: an act sent from one of its
	// panes is the stoker's. Empty: no stoker agent (fixtures).
	StokerServer string
	// StokerAgent is the command the stoker agent runs ([stoker] agent).
	StokerAgent string
	// LookPath finds a command on PATH. Nil: exec.LookPath.
	LookPath func(string) (string, error)
	// Ways is the builder's ways file: every brief names it, and its
	// recipes are the launch sheet's chips. Empty: none.
	Ways string
	// OrderSteps is how many messages the stoker agent may send in a run.
	OrderSteps int
	// PeerPane is the tmux pane a CLI that connected runs in, by its pid
	// ("" outside every pane). An act's From is ignored where it is known.
	PeerPane func(pid int) string
	// Capture reads the text of panes on the host, for the stoker's guard,
	// with its styles: the guard tells Claude Code's grey suggestion from a
	// draft a person typed.
	Capture func(panes []string) map[string]string
	// CaptureUp reads a pane up lines up from its bottom, with its styles,
	// and the offset it read: no more than the history holds.
	CaptureUp func(pane string, up int) (text string, real int, ok bool)
	// TmuxOut runs a tmux command and returns what it prints: the pane of
	// a session matchblox started. Nil: exec.
	TmuxOut func(argv []string) (string, error)
	// IsDir tells if a new session's directory is there. Nil: os.Stat.
	IsDir func(path string) bool
	// Activity is the builder's last key in any tmux client: it keeps a
	// stoker that rests resting while they work in a pane.
	Activity func() time.Time
	// LimitsLog keeps each account's limit readings. Nil: none kept.
	LimitsLog *limits.Store
	// Now is the service's clock for the stoker and the queue. Nil: time.Now.
	Now func() time.Time
	// Ships follows the pull requests sessions drive. Nil: no ships.
	Ships ShipsSource
	// Find scans the host from roots, after a yes, never touching skip.
	// named are the roots the builder picked, as walked: one that is no
	// folder is said. Nil: no scan, and no Found sent (fixtures).
	Find func(ctx context.Context, roots []string, skip, named map[string]bool) gitscan.Result
	// Missing is the folders among roots that are not there, looked up
	// within a bound: a yes that picks one fails before it is recorded.
	// Nil: none missing.
	Missing func(roots []string) []string
	// ScanRoots are a scan's roots for the ones picked (empty: the
	// defaults). Nil: the picked ones.
	ScanRoots func(picked []string) []string
	// ConsentPath and ReposPath keep the answer and the last scan. Empty:
	// kept in memory only.
	ConsentPath, ReposPath string
	// UsedPath keeps where sessions started. Empty: kept in memory only.
	UsedPath string
	// CloneRoot is where a clone goes ([git] clone_root), absolute.
	// Empty: no clones.
	CloneRoot string
	// GitRemote is the remote a checkout is compared at ([git] remote).
	// Empty: origin.
	GitRemote string
	// PushNumbers are the island's numbers of sessions and accounts; the
	// names act names them. Nil: it names nothing.
	PushNumbers *push.Numbers
	// Push sends the buzz and the island of each published doc. Nil: none
	// ([push] off, or a fixture).
	Push *push.Sender
	// PushOff is the Machine tab's push line while Push is nil because push
	// could not start (`push: numbers: <why>`); "" when it is off by choice.
	PushOff string

	// alone is when the last console closed; zero while one is open. A new
	// service starts alone.
	alone time.Time

	interval time.Duration
	gitEvery time.Duration
	full     sample.Full
	sample   func() sample.Snapshot
	git      GitSource

	tmuxMu sync.Mutex
	tmuxV  string // the hello's tmux, read at tmuxAt
	tmuxAt time.Time

	// recovering runs one recover act at a time; reopened is when each
	// conversation it reopened started, until a sample shows it.
	recovering sync.Mutex
	reopened   map[string]reopenedAt
	// settle is how long recover waits before it checks that each agent it
	// reopened still runs (0: recoverSettle).
	settle time.Duration

	mu  sync.Mutex
	cur state.Doc
	// startedAt is when matchblox started the stoker agent; zero once it
	// is ready, or gave up on it.
	startedAt time.Time
	// sampled are the sessions as the sampler read them; cur.Sessions adds
	// the question tool's calls the hooks announced.
	sampled  []sample.Session
	have     bool
	hist     history.Series
	queue    *queue.Queue
	clients  map[*client]struct{}
	gitKick  chan struct{}
	shipKick chan struct{}
	// sampleKick asks the sample loop for a sample now: a session started
	// here shows without waiting for the tick.
	sampleKick chan struct{}
	stop       context.CancelCauseFunc

	acts  acts
	ways  waysCache
	wakes wakeState
	// driven are the panes the stoker drove, for the merge guard.
	driven drivenState
	// upScreens are the screens of the history sent, which no Enter answers.
	upScreens upScreens
	gen       uint64 // counts door writes, so an older sample keeps its hands off
	// panes holds a *sync.Mutex per pane: the keys of one act to a pane
	// never interleave with another's.
	panes sync.Map
	// grantMu makes each y one check and one go: two consoles pressing it
	// at once on one step give one go.
	grantMu sync.Mutex
	// renames are the renames that wait for their session's rest, by pane.
	renames renameState
	// found goes to each console when foundGen passes the one it has.
	found     gitscan.Found
	foundGen  uint64
	foundKick chan struct{}
	// used is where sessions started; it goes to each console when usedGen
	// passes the one it has. usedMu orders the writes of its file.
	used    state.Used
	usedGen uint64
	usedMu  sync.Mutex
	// scanReqs are the yeses that wait for foundLoop, the only walker;
	// scanEnd closes when it ends.
	scanReqs chan scanReq
	scanEnd  chan struct{}
	// hung are the folders that did not answer a walk, and when: the
	// walks of the next 6 hours skip them. foundLoop's alone.
	hung map[string]time.Time
	// rootsResolved is where each root, as written, last resolved to,
	// so a root that hangs in its lookup keeps the repos found where it
	// led. foundLoop's alone, and not persisted: after a restart, a root
	// that hangs on its first lookup has none.
	rootsResolved map[string]string
	// nos counts the noes, so a walk knows a no came while it ran even
	// when a yes for the same roots came after it.
	nos uint64
	// consentMu orders the answer and the cache, so a no is never
	// followed by a scan's write.
	consentMu sync.Mutex
	// clones are the clones that run or ended in the last minute, by
	// target, under mu.
	clones map[string]*cloneJob
	// cloneSeq is the id of the last clone started, under mu: each clone
	// has its own, so a console tells it from an earlier one there.
	cloneSeq int64
	// cloneRand picks free names, under cloneMu. Nil: seeded from the time.
	cloneRand *rand.Rand
	cloneMu   sync.Mutex
	// cloneRun clones (tests). Nil: clone.Run.
	cloneRun func(ctx context.Context, r gitremote.Remote, target string, progress func(pct int)) error
	// cloneRemote reads a typed URL (tests). Nil: gitremote.Parse.
	cloneRemote func(text string) (gitremote.Remote, bool)
	// protocol is gh's git_protocol, asked once.
	protoOnce sync.Once
	protocol  string
	// signIns are the notes on a new account's sign-in, by pane.
	signIns signInState
	// accountLocks hold an account folder from its check to its start, by
	// folder: two starts of one new account make it once.
	accountLocks sync.Map
	// lastAccounts are the accounts the builder last started, per agent, and
	// the agent they last started.
	lastAccounts lastAccountState
	// readDir reads an accounts folder; nil is os.ReadDir. Tests count it.
	readDir func(string) ([]os.DirEntry, error)
	// known is the last list of accounts, kept while its sources hold.
	knownMu sync.Mutex
	known   knownCache
	// signInWait and signInEvery are how long the sign-in driver waits for
	// the agent's screens and how often it looks. Zero: 20 s and 500 ms.
	signInWait, signInEvery time.Duration
}

func New(cfg config.Config, smp *sample.Sampler, git GitSource) *Service {
	s := &Service{
		Binary:    "dev",
		ExeEvery:  time.Minute,
		interval:  cfg.Interval.Duration,
		gitEvery:  cfg.Git.Interval.Duration,
		git:       git,
		clients:   map[*client]struct{}{},
		gitKick:   make(chan struct{}, 1),
		shipKick:  make(chan struct{}, 1),
		foundKick: make(chan struct{}, 1),
		scanReqs:  make(chan scanReq, 64),
		scanEnd:   make(chan struct{}),

		sampleKick: make(chan struct{}, 1),
		queue:      queue.New(),

		NightEnds:   cfg.Stoker.NightEnds,
		StokerAgent: cfg.Stoker.Agent,
		OrderSteps:  cfg.Stoker.MaxSteps,
		full:        sample.Full{Pct: cfg.Sessions.CompactAt, Tokens: cfg.Sessions.CompactTokens},
	}
	if s.interval <= 0 {
		s.interval = 2 * time.Second
	}
	if s.gitEvery <= 0 {
		s.gitEvery = 5 * time.Minute
	}
	if smp != nil {
		s.sample = smp.Sample
	}
	s.Capture, s.CaptureUp = sample.CapturePanesStyled, sample.CapturePaneUp
	s.PeerPane = s.paneOfPeer
	s.Activity = sample.ClientActivity
	s.alone = s.now()
	s.Actions = actions.Runner{Check: actions.CheckGuard, Idle: s.idle, Resting: s.resting, Answerable: s.answerable, Messageable: s.messageable, Asking: s.asking, Menu: s.menu, Stokable: s.stokable}
	return s
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// State is the latest state, without history.
func (s *Service) State() proto.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return proto.State{Doc: s.cur}
}

// Serve runs the loops and accepts consoles until ctx ends or the binary
// on disk changes.
func (s *Service) Serve(ctx context.Context, l net.Listener) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	s.mu.Lock()
	s.stop = cancel
	s.mu.Unlock()
	runDone := make(chan struct{})
	s.mu.Lock()
	s.runDone = runDone
	s.mu.Unlock()
	go func() { s.Run(ctx); close(runDone) }()
	sock := ""
	if a, ok := l.Addr().(*net.UnixAddr); ok {
		sock = a.Name
	}
	go s.watch(ctx, cancel, sock)
	go func() {
		for {
			nc, err := l.Accept()
			if err != nil {
				cancel(err)
				return
			}
			pid, _ := transport.PeerPID(nc)
			go s.HandlePeer(ctx, transport.NewConn(nc), pid) //nolint:errcheck // a console that leaves is not an error
		}
	}()
	<-ctx.Done()
	l.Close()
	s.stopClones(cloneStopWait) // a clone cut off removes its partial folder first
	// The sample loop can still be writing its first state file when the
	// socket disappears during startup. Finish it before Serve returns.
	<-runDone
	if err := context.Cause(ctx); errors.Is(err, ErrReplaced) || errors.Is(err, ErrSocketGone) {
		return err
	}
	return ctx.Err()
}

// watch stops the service when the binary it was started by changes (an
// install over it, or a link repointed to a new version) or when its
// socket file is removed or replaced.
func (s *Service) watch(ctx context.Context, stop context.CancelCauseFunc, sock string) {
	stat := func(p string) os.FileInfo {
		if p == "" {
			return nil
		}
		fi, _ := os.Stat(p)
		return fi
	}
	changed := func(was os.FileInfo, p string) bool {
		if was == nil {
			return false
		}
		now := stat(p)
		return now == nil || !os.SameFile(was, now) || now.Size() != was.Size() || !now.ModTime().Equal(was.ModTime())
	}
	exe, socket := stat(s.Exe), stat(sock)
	if s.ExeAt != nil && s.Exe != "" {
		was := exe
		exe = s.ExeAt
		if was == nil || changed(exe, s.Exe) {
			// Installed over or removed since the process started, as while
			// a unit's serve waits for the socket: not this process's code.
			s.watchingOnce.Do(func() { close(s.watchingChan()) })
			stop(ErrReplaced)
			return
		}
	}
	s.watchingOnce.Do(func() { close(s.watchingChan()) })
	// Removed between the listen and this first look: gone all the same,
	// or no later look would ever tell. A binary gone is uninstalled.
	switch {
	case s.Exe != "" && exe == nil:
		stop(ErrReplaced)
		return
	case sock != "" && socket == nil:
		stop(ErrSocketGone)
		return
	}
	t := time.NewTicker(s.ExeEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			switch {
			case changed(exe, s.Exe):
				stop(ErrReplaced)
				return
			case changed(socket, sock):
				stop(ErrSocketGone)
				return
			}
		}
	}
}

// Watching closes once Serve's watch has taken its first look at the
// binary and the socket: a change after it is seen. Without ExeAt, a binary
// replaced before that look is the one the watch knows; with it, a change
// since the process started is seen too.
func (s *Service) Watching() <-chan struct{} { return s.watchingChan() }

func (s *Service) watchingChan() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watching == nil {
		s.watching = make(chan struct{})
	}
	return s.watching
}

// replace stops Serve so that the console that asked starts its own,
// newer binary. The result goes out first.
func (s *Service) replace() {
	s.mu.Lock()
	stop := s.stop
	s.mu.Unlock()
	if stop != nil {
		time.AfterFunc(100*time.Millisecond, func() { stop(ErrReplaced) })
	}
}

// Run samples every interval and scans git every gitEvery until ctx ends.
// Call it once: it is the only sample loop of the service.
func (s *Service) Run(ctx context.Context) {
	s.mu.Lock()
	if len(s.clients) == 0 {
		s.alone = s.now() // Now is set by now
	}
	s.mu.Unlock()
	rec := s.recorder()
	s.loadFound()
	s.loadUsed()
	go s.gitLoop(ctx)
	go s.shipsLoop(ctx)
	go s.foundLoop(ctx)
	s.loadHistory()
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for first := true; ; first = false {
		s.readSpool()
		s.publish(s.sample(), rec)
		s.renameAtRest(ctx)
		s.stoke(ctx, s.now())
		if first {
			s.rescanGit()
			s.shipKick <- struct{}{}
			s.kickFound()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.sampleKick:
		}
	}
}

func (s *Service) publish(snap sample.Snapshot, rec func(state.Doc)) {
	if s.LimitsLog != nil { // only the sample loop calls publish
		if err := s.LimitsLog.Append(snap.Limits, snap.At); err != nil {
			snap.Errors = append(snap.Errors, "limits: "+err.Error())
		}
	}
	gen := s.doorsGen()
	ds := s.doors()
	var view *stoker.View
	if s.Stoker != nil {
		view = s.stokerView(snap.Sessions)
	}
	known, skipped := s.knownAccounts(snap.Sessions)
	s.lastAccounts.signedIn(lastAccountsPath(s.StatePath), snap.Sessions)
	last := s.lastListed(known)
	lastAgent := s.lastAccounts.lastAgent(lastAccountsPath(s.StatePath))
	s.mu.Lock()
	s.setDoors(gen, ds)
	if s.have { // the first sample has no rates yet
		s.hist.Append(history.FromSnapshot(snap))
	}
	s.cur.Snapshot, s.sampled, s.have = snap, snap.Sessions, true
	s.cur.Agents, s.cur.Renames = s.startable(), s.renames.names()
	s.cur.Accounts, s.cur.AccountsSkipped = known, skipped
	s.cur.LastAccount = last
	s.cur.LastAgent = ""
	if slices.Contains(s.cur.Agents, lastAgent) { // an agent no longer installed is not sent
		s.cur.LastAgent = lastAgent
	}
	here := make(map[string]bool, len(snap.Panes))
	for _, p := range snap.Panes {
		here[p.ID] = true
		if name, _, ok := strings.Cut(p.Target, ":"); ok {
			here[name] = true
		}
	}
	s.cur.SignIns = s.signIns.keep(here)
	if s.Stoker != nil {
		s.cur.Stoker = view
	}
	s.cur.Recommendations = advice.Build(snap, s.cur.Git)
	// Join hooks before holds: Cursor has no session file, so its id comes
	// from the hooked chat, and prune must see that id as present.
	s.mergeQueue()
	if s.Holds != nil {
		present := make(map[string]bool, len(s.cur.Sessions))
		for _, x := range s.cur.Sessions {
			present[x.SessionID] = true
			if x.Pane == "" {
				continue
			}
			if _, err := s.Holds.Rebind(x.Pane, x.SessionID); err != nil {
				s.cur.Errors = append(s.cur.Errors, "holds: "+err.Error())
			}
		}
		if err := s.Holds.Prune(present, s.now()); err != nil {
			s.cur.Errors = append(s.cur.Errors, "holds: "+err.Error())
		}
		s.cur.Holds = s.Holds.List()
	}
	var live []recovery.Entry
	var alive map[string]string
	if s.Recovery != nil {
		live, alive = recordable(s.cur.Sessions, snap.Panes)
	}
	s.expireFocus(s.now())
	s.cur.Push = s.PushOff
	if s.Push != nil {
		s.cur.Push = s.Push.Status() // the last send's: this doc's goes after
	}
	doc := s.cur
	s.mu.Unlock()
	if s.Recovery != nil {
		// Out of the lock: it reads the checkout of a new agent's folder.
		if err := s.Recovery.Update(live, alive, s.now()); err != nil {
			doc.Errors = append(slices.Clip(doc.Errors), "recovery record: "+err.Error())
		}
	}
	rec(doc)
	if s.Push != nil {
		// Every tick, changed or not: a change the rates held goes out on
		// the first tick after its wait. OnDoc never blocks.
		s.Push.OnDoc(doc)
	}
	s.broadcast()
}

// readSpool applies the hook events that missed the service: at start, and
// on each tick for a hook that timed out under load. The queue drops an
// event older than what it knows.
func (s *Service) readSpool() {
	if s.Spool == "" {
		return
	}
	var evs []hooks.Event
	sock := s.stokerSocket()
	s.mu.Lock()
	_ = hooks.Replay(s.Spool, func(ev hooks.Event) { // a spool we cannot read loses old waits, not the service
		ev = s.onServer(ev, sock)
		s.queue.Apply(ev)
		evs = append(evs, ev)
	})
	s.mu.Unlock()
	// A step's go that a spooled call used is used: as if the hook had
	// reached the service.
	for _, ev := range evs {
		s.useGrants(ev)
		s.askGo(ev)
	}
}

// Hook takes one event from an agent hook and updates the queue at once,
// without waiting for the next sample.
func (s *Service) Hook(ev hooks.Event) {
	s.useGrants(ev)
	s.askGo(ev)
	s.mu.Lock()
	s.queue.Apply(ev)
	if s.have {
		s.mergeQueue()
	}
	s.mu.Unlock()
	s.broadcast()
}

// mergeQueue joins the queue with the current sessions. Hold s.mu.
func (s *Service) mergeQueue() {
	now := s.cur.At
	if now.IsZero() {
		now = s.now()
	}
	s.queue.Merge(s.sampled, now)
	s.cur.Sessions = s.queue.WithJoined(s.queue.WithAsks(s.sampled))
	s.cur.Queue = s.queue.Items()
	s.cur.StokerServer = s.StokerServer
	s.setReasons()
}

// setReasons says why each session at rest is idle. The ship of a session is
// the one its row leads with (ships.Of). Hold s.mu.
func (s *Service) setReasons() {
	now := s.cur.At
	if now.IsZero() {
		now = s.now()
	}
	var reasons map[string]idle.Reason
	for _, sess := range s.cur.Sessions {
		if sess.Pane == "" {
			continue
		}
		asking := false
		for _, it := range s.cur.Queue {
			if it.Pane == sess.Pane && (it.State == queue.StatePermission || it.State == queue.StateQuestion && !it.Reminder) {
				asking = true
			}
		}
		r := idle.Of(sess, asking, s.shipOf(sess), s.cur.Limits, now)
		if r == (idle.Reason{}) {
			continue
		}
		if reasons == nil {
			reasons = map[string]idle.Reason{}
		}
		reasons[sess.Pane] = r
	}
	s.cur.Reasons = reasons
}

// shipOf is the ship a session drives that its row leads with, as the
// console finds it (ships.Of). Hold s.mu.
func (s *Service) shipOf(sess sample.Session) *ships.Ship {
	if s.cur.Ships == nil {
		return nil
	}
	if ss := ships.Of(s.cur.Ships.Ships, sess.Pane, sess.SessionID); len(ss) > 0 {
		return &ss[0]
	}
	return nil
}

// rescanGit asks the git loop for a scan now; a scan already asked for
// absorbs it.
func (s *Service) rescanGit() {
	select {
	case s.gitKick <- struct{}{}:
	default:
	}
}

func (s *Service) gitLoop(ctx context.Context) {
	if s.git == nil {
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-s.gitKick:
	}
	t := time.NewTicker(s.gitEvery)
	defer t.Stop()
	for {
		s.mu.Lock()
		var cwds []string
		for _, ss := range s.cur.Sessions {
			cwds = append(cwds, ss.Cwd)
		}
		s.mu.Unlock()
		r := s.git(cwds)
		s.mu.Lock()
		s.cur.Git = &r
		s.cur.Recommendations = advice.Build(s.cur.Snapshot, s.cur.Git)
		s.mu.Unlock()
		s.broadcast()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.gitKick:
		}
	}
}

// shipsLoop starts after the first sample, which finds the sessions and
// their transcripts, and runs every shipsEvery.
func (s *Service) shipsLoop(ctx context.Context) {
	if s.Ships == nil {
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-s.shipKick:
	}
	t := time.NewTicker(shipsEvery)
	defer t.Stop()
	for {
		s.mu.Lock()
		sessions, git := append([]sample.Session(nil), s.cur.Sessions...), s.cur.Git
		s.mu.Unlock()
		r := s.Ships(ctx, sessions, git)
		s.mu.Lock()
		s.cur.Ships = &r
		s.setReasons()
		s.mu.Unlock()
		s.syncDriven() // a merge:@<session> go covers the PRs the poll found
		s.broadcast()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) loadHistory() {
	var loaded history.Series
	if paths := append(append([]string{}, s.HistorySources...), s.HistoryLog); s.HistoryLog != "" || len(s.HistorySources) > 0 {
		loaded, _ = history.Load(time.Now(), paths...)
	}
	s.mu.Lock()
	s.hist = loaded
	s.mu.Unlock()
}

// idle is the guard of /compact and /clear: the agent reports itself
// idle. A session in the queue can sit at a permission prompt, where the
// Enter of the step would answer it, so the queue does not count.
func (s *Service) idle(pane string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ss := range s.cur.Sessions {
		if ss.Pane == pane {
			return ss.Status == "idle"
		}
	}
	return false
}

// resting tells if no turn runs in the pane: idle, or only its subagents
// work.
func (s *Service) resting(pane string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ss := range s.cur.Sessions {
		if ss.Pane == pane {
			return ss.Resting()
		}
	}
	return false
}

// client is one console. kick holds at most one wake-up, so a slow console
// gets the latest state and never makes the loops wait.
type client struct {
	c    transport.Conn
	kick chan struct{}
	// watch is the pane this console streams; screenKick wakes watchLoop
	// before its next tick.
	watch      atomic.Pointer[watched]
	screenKick chan struct{}
	histSent   bool
	foundGen   uint64
	usedGen    uint64
}

func (s *Service) join(cl *client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[cl] = struct{}{}
	s.alone = time.Time{}
}

// leave notes when the last console closed: a stoker that rests goes on
// a little after.
func (s *Service) leave(cl *client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, cl)
	if len(s.clients) == 0 {
		s.alone = s.now()
	}
}

func (s *Service) broadcast() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for cl := range s.clients {
		select {
		case cl.kick <- struct{}{}:
		default:
		}
	}
}

// stateFor is the next snapshot for one console. The history goes once,
// in its first snapshot; what the scan found, then and when it changes.
func (s *Service) stateFor(cl *client) (proto.State, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.have {
		return proto.State{}, false
	}
	st := proto.State{Doc: s.cur}
	if !cl.histSent {
		h := history.Series{Points: append([]history.Point(nil), s.hist.Points...)}
		st.History, cl.histSent = &h, true
	}
	if cl.foundGen != s.foundGen {
		f := s.found
		st.Found, cl.foundGen = &f, s.foundGen
	}
	if cl.usedGen != s.usedGen {
		u := state.Used{Repos: slices.Clone(s.used.Repos)}
		st.Used, cl.usedGen = &u, s.usedGen
	}
	st.Clones = s.cloneList()
	return st, true
}

// Handle serves one console until it leaves or ctx ends.
func (s *Service) Handle(ctx context.Context, c transport.Conn) error {
	return s.HandlePeer(ctx, c, 0)
}

// HandlePeer serves one console or command whose process is pid (0: not
// known). With pid known, the pane of each act is the one pid runs in,
// whatever the act says.
func (s *Service) HandlePeer(ctx context.Context, c transport.Conn, pid int) error {
	defer c.Close()
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()

	host := s.Host
	if host == "" {
		host, _ = os.Hostname()
	}
	if err := c.Send(proto.KindHello, "", proto.Hello{
		Version: proto.Version, Binary: s.Binary, Host: host, OS: runtime.GOOS, Arch: runtime.GOARCH, Tmux: s.tmuxVersion(),
	}); err != nil {
		return err
	}
	env, err := c.Recv()
	if err != nil {
		return err
	}
	if env.Kind == proto.KindHook {
		var ev hooks.Event
		if json.Unmarshal(env.Body, &ev) == nil {
			if ev = s.onServer(ev, s.stokerSocket()); pid <= 0 || s.hookFrom(pid, ev) {
				s.Hook(ev)
			}
		}
		return nil
	}
	var h proto.Hello
	if env.Kind != proto.KindHello || json.Unmarshal(env.Body, &h) != nil {
		return c.Send(proto.KindError, "", proto.Error{Text: "the first message must be hello"})
	}
	if h.Version != proto.Version {
		return nil // the console has our hello and shows the fix
	}

	// The first wake-up goes in before the console joins: a broadcast
	// after the join fills kick, and a send then would wait for a reader
	// that starts below.
	cl := &client{c: c, kick: make(chan struct{}, 1), screenKick: make(chan struct{}, 1)}
	cl.kick <- struct{}{}
	s.join(cl)
	defer s.leave(cl)

	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-cl.kick:
			}
			if st, ok := s.stateFor(cl); ok {
				if c.Send(proto.KindSnapshot, "", st) != nil {
					c.Close()
					return
				}
			}
		}
	}()

	connCtx, connDone := context.WithCancel(ctx)
	defer connDone() // a clone for this connection's act stops with it
	actCtx := context.WithValue(ctx, connKey{}, connCtx)
	if h.Client == proto.ClientConsole {
		go s.watchLoop(connCtx, cl)
	}
	for {
		env, err := c.Recv()
		if err != nil {
			return nil
		}
		if env.Kind != proto.KindAct {
			continue
		}
		var a proto.Act
		if err := json.Unmarshal(env.Body, &a); err != nil {
			_ = c.Send(proto.KindResult, env.ID, proto.Result{ActID: env.ID, Err: "bad act"})
			continue
		}
		if pid > 0 {
			a.From = s.PeerPane(pid)
		}
		if pane, ok := strings.CutPrefix(a.RecID, proto.WatchPrefix); ok {
			res := s.watchAct(cl, h.Client, a, pane)
			res.ActID = env.ID
			_ = c.Send(proto.KindResult, env.ID, res)
			continue
		}
		go func(id string) {
			// A console in a pane the stoker drove, or in the stoker's own,
			// is an agent that says it is one.
			a.Console = h.Client == proto.ClientConsole && (pid <= 0 || !s.agentsPane(a))
			res := s.act(actCtx, a)
			res.ActID = id
			_ = c.Send(proto.KindResult, id, res)
		}(env.ID)
	}
}
