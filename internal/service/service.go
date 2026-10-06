// Package service owns every loop, the state and the event stream. Consoles
// connect to it and draw what it sends; one service samples the machine
// once, however many consoles watch.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/blox-eng/matchblox/internal/actions"
	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/history"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/transport"
)

// GitSource scans the repositories the given session directories belong to.
type GitSource func(sessionCwds []string) gitscan.Report

// ErrReplaced means the binary on disk changed. The service stops so that
// the next console starts the new one.
var ErrReplaced = errors.New("service: the binary on disk changed")

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
	// Exe is checked every ExeEvery; Serve stops when it changes. Empty: never.
	Exe      string
	ExeEvery time.Duration

	interval time.Duration
	gitEvery time.Duration
	sample   func() sample.Snapshot
	git      GitSource

	mu      sync.Mutex
	cur     state.Doc
	have    bool
	hist    history.Series
	clients map[*client]struct{}
	gitKick chan struct{}

	acts acts
}

func New(cfg config.Config, smp *sample.Sampler, git GitSource) *Service {
	s := &Service{
		Binary:   "dev",
		ExeEvery: time.Minute,
		interval: cfg.Interval.Duration,
		gitEvery: cfg.Git.Interval.Duration,
		git:      git,
		clients:  map[*client]struct{}{},
		gitKick:  make(chan struct{}, 1),
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
	s.Actions = actions.Runner{Check: actions.CheckGuard, Idle: s.idle}
	return s
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
	go s.Run(ctx)
	if s.Exe != "" {
		go s.watchExe(ctx, cancel)
	}
	go func() {
		for {
			nc, err := l.Accept()
			if err != nil {
				cancel(err)
				return
			}
			go s.Handle(ctx, transport.NewConn(nc)) //nolint:errcheck // a console that leaves is not an error
		}
	}()
	<-ctx.Done()
	l.Close()
	if err := context.Cause(ctx); errors.Is(err, ErrReplaced) {
		return ErrReplaced
	}
	return ctx.Err()
}

func (s *Service) watchExe(ctx context.Context, stop context.CancelCauseFunc) {
	first, err := os.Stat(s.Exe)
	if err != nil {
		return
	}
	t := time.NewTicker(s.ExeEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fi, err := os.Stat(s.Exe)
			if err != nil || fi.Size() != first.Size() || !fi.ModTime().Equal(first.ModTime()) {
				stop(ErrReplaced)
				return
			}
		}
	}
}

// Run samples every interval and scans git every gitEvery until ctx ends.
// Call it once: it is the only sample loop of the service.
func (s *Service) Run(ctx context.Context) {
	rec := s.recorder()
	go s.gitLoop(ctx)
	s.loadHistory()
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for first := true; ; first = false {
		s.publish(s.sample(), rec)
		if first {
			s.rescanGit()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) publish(snap sample.Snapshot, rec func(state.Doc)) {
	s.mu.Lock()
	if s.have { // the first sample has no rates yet
		s.hist.Append(history.FromSnapshot(snap))
	}
	s.cur.Snapshot, s.have = snap, true
	s.cur.Recommendations = advice.Build(snap, s.cur.Git)
	doc := s.cur
	s.mu.Unlock()
	rec(doc)
	s.broadcast()
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

func (s *Service) loadHistory() {
	var loaded history.Series
	if paths := append(append([]string{}, s.HistorySources...), s.HistoryLog); s.HistoryLog != "" || len(s.HistorySources) > 0 {
		loaded, _ = history.Load(time.Now(), paths...)
	}
	s.mu.Lock()
	s.hist = loaded
	s.mu.Unlock()
}

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

// client is one console. kick holds at most one wake-up, so a slow console
// gets the latest state and never makes the loops wait.
type client struct {
	c        transport.Conn
	kick     chan struct{}
	histSent bool
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
// in its first snapshot.
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
	return st, true
}

// Handle serves one console until it leaves or ctx ends.
func (s *Service) Handle(ctx context.Context, c transport.Conn) error {
	defer c.Close()
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()

	host, _ := os.Hostname()
	if err := c.Send(proto.KindHello, "", proto.Hello{
		Version: proto.Version, Binary: s.Binary, Host: host, OS: runtime.GOOS, Arch: runtime.GOARCH,
	}); err != nil {
		return err
	}
	env, err := c.Recv()
	if err != nil {
		return err
	}
	var h proto.Hello
	if env.Kind != proto.KindHello || json.Unmarshal(env.Body, &h) != nil {
		return c.Send(proto.KindError, "", proto.Error{Text: "the first message must be hello"})
	}
	if h.Version != proto.Version {
		return nil // the console has our hello and shows the fix
	}

	cl := &client{c: c, kick: make(chan struct{}, 1)}
	s.mu.Lock()
	s.clients[cl] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, cl)
		s.mu.Unlock()
	}()
	cl.kick <- struct{}{}

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
		go func(id string) {
			res := s.act(ctx, a)
			res.ActID = id
			_ = c.Send(proto.KindResult, id, res)
		}(env.ID)
	}
}
