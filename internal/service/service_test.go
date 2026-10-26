package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/accounts"
	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/ships"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/transport"
)

var orphanSnap = sample.Snapshot{
	Orphans: []sample.Orphan{{PID: 42, Start: 7, Comm: "bash", CPU: 99, HotFor: time.Hour, Parent: "init",
		Kill: []string{"kill", "42"}}},
	Sessions: []sample.Session{{Pane: "%1", Name: "app", Status: "idle"}},
}

var gitReport = gitscan.Report{Repos: []gitscan.Repo{{Path: "/w/app", Main: "main",
	Worktrees: []gitscan.Worktree{{Path: "/w/wt/a", Safe: true, Remove: []string{"git", "worktree", "remove", "/w/wt/a"}}}}}}

func newTest(t *testing.T, interval time.Duration) *Service {
	t.Helper()
	cfg := config.Default()
	cfg.Interval.Duration = interval
	s := New(cfg, nil, func([]string) gitscan.Report { return gitReport })
	s.sample = func() sample.Snapshot { snap := orphanSnap; snap.At = time.Now(); return snap }
	s.StatePath = filepath.Join(t.TempDir(), "state.json")
	s.Actions.Run = func([]string) error { return nil }
	s.Actions.Check = nil // the guards of the fake snapshot do not hold on this machine
	return s
}

func run(t *testing.T, s *Service) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	// Wait for the loops to end: a state write after cancel races the
	// removal of the test's temp dir.
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(waitLimit):
			t.Errorf("the service did not stop in %v", waitLimit)
		}
	})
	return ctx
}

// connect joins a console to the service, sends its hello and reads the
// service hello.
func connect(t *testing.T, ctx context.Context, s *Service) (transport.Conn, proto.Hello) {
	t.Helper()
	a, b := net.Pipe()
	go s.Handle(ctx, transport.NewConn(b))
	c := transport.NewConn(a)
	t.Cleanup(func() { c.Close() })
	// net.Pipe has no buffer: send the hello while reading the service's,
	// and finish it before anything else goes out.
	sent := make(chan error, 1)
	go func() { sent <- c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version}) }()
	env, err := recv(t, c, "hello")
	if err != nil || env.Kind != proto.KindHello {
		t.Fatalf("first message %+v, %v", env, err)
	}
	if err := within(t, sent, "sent hello"); err != nil {
		t.Fatal(err)
	}
	var h proto.Hello
	_ = json.Unmarshal(env.Body, &h)
	return c, h
}

// waitLimit bounds every wait of a test: a service that never answers
// fails the test with what it waited for, never hangs the run.
const waitLimit = 10 * time.Second

// within waits for one value of ch, at most waitLimit.
func within[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(waitLimit):
		t.Fatalf("no %s in %v", what, waitLimit)
		var zero T
		return zero
	}
}

// recv reads one message, at most waitLimit; on timeout it closes c.
func recv(t *testing.T, c transport.Conn, what string) (proto.Envelope, error) {
	t.Helper()
	type got struct {
		env proto.Envelope
		err error
	}
	ch := make(chan got, 1)
	go func() {
		env, err := c.Recv()
		ch <- got{env, err}
	}()
	select {
	case g := <-ch:
		return g.env, g.err
	case <-time.After(waitLimit):
		c.Close()
		t.Fatalf("no %s in %v", what, waitLimit)
		return proto.Envelope{}, nil
	}
}

func next(t *testing.T, c transport.Conn, kind proto.Kind) proto.Envelope {
	t.Helper()
	for {
		env, err := recv(t, c, string(kind))
		if err != nil {
			t.Fatalf("waiting for %s: %v", kind, err)
		}
		if env.Kind == kind {
			return env
		}
	}
}

func snapshot(t *testing.T, c transport.Conn) proto.State {
	t.Helper()
	var st proto.State
	if err := json.Unmarshal(next(t, c, proto.KindSnapshot).Body, &st); err != nil {
		t.Fatal(err)
	}
	return st
}

func act(t *testing.T, c transport.Conn, id string, a proto.Act) proto.Result {
	t.Helper()
	if err := c.Send(proto.KindAct, id, a); err != nil {
		t.Fatal(err)
	}
	for {
		env := next(t, c, proto.KindResult)
		if env.ID != id {
			continue
		}
		var r proto.Result
		_ = json.Unmarshal(env.Body, &r)
		return r
	}
}

func recID(t *testing.T, st proto.State, prefix string) string {
	t.Helper()
	for _, r := range st.Recommendations {
		if strings.HasPrefix(r.Title, prefix) {
			return r.ID
		}
	}
	t.Fatalf("no rec %q in %+v", prefix, st.Recommendations)
	return ""
}

func TestServiceSendsHelloThenSnapshot(t *testing.T) {
	s := newTest(t, time.Hour)
	ctx := run(t, s)
	c, h := connect(t, ctx, s)
	if h.Version != proto.Version || h.Binary == "" || h.OS == "" {
		t.Fatalf("hello %+v", h)
	}
	st := snapshot(t, c)
	if st.History == nil {
		t.Fatal("the first snapshot must carry the history")
	}
	if len(st.Recommendations) == 0 {
		t.Fatal("snapshot has no recommendations")
	}
}

func TestSnapshotEverySample(t *testing.T) {
	s := newTest(t, 50*time.Millisecond)
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	snapshot(t, c)
	n := 0
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		st := snapshot(t, c)
		if st.History != nil {
			t.Fatal("only the first snapshot carries the history")
		}
		n++
	}
	if n < 3 {
		t.Fatalf("%d snapshots in 200 ms at 50 ms, want >= 3", n)
	}
}

func TestOneSampleLoopForManyClients(t *testing.T) {
	s := newTest(t, 50*time.Millisecond)
	var samples atomic.Int32
	inner := s.sample
	s.sample = func() sample.Snapshot { samples.Add(1); return inner() }
	ctx := run(t, s)
	for range 5 {
		c, _ := connect(t, ctx, s)
		go func() {
			for {
				if _, err := c.Recv(); err != nil {
					return
				}
			}
		}()
	}
	time.Sleep(260 * time.Millisecond)
	if n := samples.Load(); n < 3 || n > 7 {
		t.Fatalf("%d samples in 260 ms at 50 ms with 5 clients, want one loop (about 6)", n)
	}
}

func TestActUnknownRecIsError(t *testing.T) {
	s := newTest(t, time.Hour)
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	snapshot(t, c)
	if r := act(t, c, "a1", proto.Act{RecID: "nope", Which: "primary"}); r.Err != "unknown action" || r.ActID != "a1" {
		t.Fatalf("result %+v", r)
	}
}

func TestNavActionRunsInTheConsole(t *testing.T) {
	s := newTest(t, time.Hour)
	s.sample = func() sample.Snapshot {
		return sample.Snapshot{At: time.Now(), Sessions: []sample.Session{{Pane: "%1", Name: "app", Busy: true, Do: "compact", ContextPct: 95, Why: "context 95%"}}}
	}
	s.Actions.Run = func([]string) error { t.Fatal("a nav step ran on the service"); return nil }
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	st := snapshot(t, c)
	if r := act(t, c, "a1", proto.Act{RecID: st.Recommendations[0].ID, Which: "primary"}); r.Err != "runs in the console" {
		t.Fatalf("result %+v", r)
	}
}

func TestDoubleConfirmRunsOnce(t *testing.T) {
	s := newTest(t, time.Hour)
	var runs atomic.Int32
	s.Actions.Run = func([]string) error { runs.Add(1); time.Sleep(50 * time.Millisecond); return nil }
	ctx := run(t, s)
	c1, _ := connect(t, ctx, s)
	c2, _ := connect(t, ctx, s)
	id := recID(t, snapshot(t, c1), "Kill")
	snapshot(t, c2)

	var wg sync.WaitGroup
	results := make([]proto.Result, 2)
	for i, c := range []transport.Conn{c1, c2} {
		wg.Go(func() { results[i] = act(t, c, "a", proto.Act{RecID: id, Which: "secondary", Confirm: "y"}) })
	}
	wg.Wait()
	if n := runs.Load(); n != 1 {
		t.Fatalf("the step ran %d times, want 1", n)
	}
	ran, done := 0, 0
	for _, r := range results {
		if len(r.Ran) == 1 {
			ran++
		}
		if len(r.Skipped) == 1 && r.Skipped[0] == "already done" {
			done++
		}
	}
	if ran != 1 || done != 1 {
		t.Fatalf("results %+v", results)
	}
}

func TestPanelTargets(t *testing.T) {
	s := newTest(t, time.Hour)
	var ran []string
	s.Actions.Run = func(argv []string) error { ran = append(ran, strings.Join(argv, " ")); return nil }
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	snapshot(t, c)
	time.Sleep(20 * time.Millisecond) // the first git scan
	for _, a := range []proto.Act{
		{RecID: "orphan:42", Which: "secondary", Confirm: "y"},
		{RecID: "worktree:/w/wt/a", Which: "secondary", Confirm: "y"},
	} {
		if r := act(t, c, a.RecID, a); r.Err != "" || len(r.Ran) != 1 {
			t.Fatalf("%s: %+v", a.RecID, r)
		}
	}
	if got := strings.Join(ran, ","); got != "kill 42,git worktree remove /w/wt/a" {
		t.Fatalf("ran %q", got)
	}
	if r := act(t, c, "x", proto.Act{RecID: "orphan:9", Which: "secondary", Confirm: "y"}); r.Err != "unknown action" {
		t.Fatalf("a pid that is not an orphan: %+v", r)
	}
}

func TestIdleGuardReadsServiceState(t *testing.T) {
	s := newTest(t, time.Hour)
	if s.idle("%1") {
		t.Fatal("before the first sample nothing is idle")
	}
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	snapshot(t, c)
	if !s.idle("%1") || s.idle("%9@mbtest") {
		t.Fatal("idle must follow the latest sample")
	}
}

func TestStateFileStillWritten(t *testing.T) {
	s := newTest(t, 20*time.Millisecond)
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	snapshot(t, c)
	snapshot(t, c)
	doc, err := state.Read(s.StatePath)
	if err != nil {
		t.Fatalf("state file: %v", err)
	}
	if len(doc.Recommendations) == 0 || time.Since(doc.At) > time.Minute {
		t.Fatalf("state file is empty or old: %+v", doc)
	}
}

// TestServeStopsWhenBinaryGoneBeforeItWatches: a binary removed between
// the listen and the watch's first look is gone all the same: the service
// stops as replaced, and its caller sees the binary gone.
func TestServeStopsWhenBinaryGoneBeforeItWatches(t *testing.T) {
	s := newTest(t, time.Hour)
	exe := filepath.Join(t.TempDir(), "matchblox")
	if err := os.WriteFile(exe, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.Exe, s.ExeEvery = exe, 10*time.Millisecond
	_, done := serveForTestBefore(t, s, func(string) {
		if err := os.Remove(exe); err != nil {
			t.Fatal(err)
		}
	})
	waitServe(t, done, ErrReplaced)
}

// TestServeStopsWhenBinaryReplacedSinceItStarted: with ExeAt, the binary
// as the process started on it, an install before the watch's first look
// (a unit's serve that waited long for the socket) is seen.
func TestServeStopsWhenBinaryReplacedSinceItStarted(t *testing.T) {
	s := newTest(t, time.Hour)
	exe := filepath.Join(t.TempDir(), "matchblox")
	if err := os.WriteFile(exe, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	at, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	s.Exe, s.ExeAt, s.ExeEvery = exe, at, 10*time.Millisecond
	_, done := serveForTestBefore(t, s, func(string) {
		if err := os.WriteFile(exe, []byte("v2 is longer"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	waitServe(t, done, ErrReplaced)
}

func TestServeStopsWhenBinaryReplaced(t *testing.T) {
	s := newTest(t, time.Hour)
	exe := filepath.Join(t.TempDir(), "matchblox")
	if err := os.WriteFile(exe, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.Exe, s.ExeEvery = exe, 10*time.Millisecond
	dir, _ := os.MkdirTemp("", "mb")
	defer os.RemoveAll(dir)
	l, err := transport.Listen(filepath.Join(dir, "s.sock"))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Serve(context.Background(), l) }()
	waitWatching(t, s)
	if err := os.WriteFile(exe, []byte("v2 is longer"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrReplaced) {
			t.Fatalf("Serve returned %v, want ErrReplaced", err)
		}
	case <-time.After(waitLimit):
		t.Fatal("Serve kept running on a replaced binary")
	}
}

func TestRescanGitRunsTheScanNow(t *testing.T) {
	s := newTest(t, time.Hour)
	var scans atomic.Int32
	s.git = func([]string) gitscan.Report { scans.Add(1); return gitReport }
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	snapshot(t, c)
	deadline := time.Now().Add(time.Second)
	for scans.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if r := act(t, c, "g", proto.Act{RecID: "rescan:git", Which: "primary"}); r.Err != "" {
		t.Fatalf("result %+v", r)
	}
	for scans.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := scans.Load(); n != 2 {
		t.Fatalf("%d scans, want 2 (the first, then the rescan)", n)
	}
}

// An expired done entry for K must not let a second confirm of K run while
// the first still runs, even when another act finishes in between.
func TestRunOnceSurvivesTheSweep(t *testing.T) {
	s := newTest(t, time.Hour)
	var calls atomic.Int32
	release := make(chan struct{})
	s.Actions.Run = func([]string) error {
		if calls.Add(1) == 1 {
			<-release
		}
		return nil
	}
	ctx := run(t, s)
	c1, _ := connect(t, ctx, s)
	c2, _ := connect(t, ctx, s)
	k := recID(t, snapshot(t, c1), "Kill")
	snapshot(t, c2)
	// K ran once, long enough ago that its done entry expired.
	defer func(d time.Duration) { doneFor = d }(doneFor)
	doneFor = 20 * time.Millisecond
	close(release)
	if r := act(t, c1, "k0", proto.Act{RecID: k, Which: "secondary", Confirm: "y"}); len(r.Ran) != 1 {
		t.Fatalf("first K: %+v", r)
	}
	time.Sleep(30 * time.Millisecond)
	release = make(chan struct{})
	calls.Store(0)
	s.Actions.Run = func([]string) error {
		if calls.Add(1) == 1 {
			<-release
		}
		return nil
	}

	first := make(chan proto.Result, 1)
	go func() { first <- act(t, c1, "k1", proto.Act{RecID: k, Which: "secondary", Confirm: "y"}) }()
	for calls.Load() < 1 {
		time.Sleep(time.Millisecond)
	}
	// Another act finishes and sweeps expired entries.
	if r := act(t, c2, "j", proto.Act{RecID: "orphan:42", Which: "secondary", Confirm: "y"}); len(r.Ran) != 1 {
		t.Fatalf("act J: %+v", r)
	}
	second := make(chan proto.Result, 1)
	go func() { second <- act(t, c2, "k2", proto.Act{RecID: k, Which: "secondary", Confirm: "y"}) }()
	time.Sleep(30 * time.Millisecond)
	close(release)
	<-first
	r := <-second
	if n := calls.Load(); n != 2 {
		t.Fatalf("%d runs, want 2 (K once, J once)", n)
	}
	if len(r.Skipped) != 1 || r.Skipped[0] != "already done" {
		t.Fatalf("second K: %+v", r)
	}
}

func TestActsForgetKeysThatNeverRan(t *testing.T) {
	s := newTest(t, time.Hour)
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	snapshot(t, c)
	for i := range 50 {
		act(t, c, "u", proto.Act{RecID: "nope" + strconv.Itoa(i), Which: "primary"})
	}
	s.acts.mu.Lock()
	n := len(s.acts.inflight)
	s.acts.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d keys kept after their acts ended", n)
	}
}

func serveForTest(t *testing.T, s *Service) (string, chan error) {
	t.Helper()
	dir, _ := os.MkdirTemp("", "mb")
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "s.sock")
	l, err := transport.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Serve(context.Background(), l) }()
	return path, done
}

// serveForTestBefore is serveForTest with before run between the listen
// and Serve: what a console that dials at once can do before the watch.
func serveForTestBefore(t *testing.T, s *Service, before func(sock string)) (string, chan error) {
	t.Helper()
	dir, err := os.MkdirTemp("", "mb") // short: a socket path has a length limit
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "s.sock")
	l, err := transport.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	before(path)
	done := make(chan error, 1)
	go func() { done <- s.Serve(context.Background(), l) }()
	// Serve stops at once here, while Run may still write its state: wait
	// for Run before the test's folders go (cleanups run last in, first out).
	t.Cleanup(func() {
		s.mu.Lock()
		runDone := s.runDone
		s.mu.Unlock()
		if runDone == nil {
			return // Serve never started
		}
		select {
		case <-runDone:
		case <-time.After(waitLimit):
			t.Errorf("Run did not stop in %v", waitLimit)
		}
	})
	return path, done
}

// waitWatching waits for Serve's watch to take its first look, so a change
// the test makes next is one it sees.
func waitWatching(t *testing.T, s *Service) {
	t.Helper()
	select {
	case <-s.Watching():
	case <-time.After(waitLimit):
		t.Fatalf("the watch did not start in %v", waitLimit)
	}
}

func waitServe(t *testing.T, done chan error, want error) {
	t.Helper()
	select {
	case err := <-done:
		if !errors.Is(err, want) {
			t.Fatalf("Serve returned %v, want %v", err, want)
		}
	case <-time.After(waitLimit):
		t.Fatal("Serve kept running")
	}
}

// Homebrew and nix upgrade by repointing a link; the old file never changes.
func TestServeStopsWhenSymlinkRepointed(t *testing.T) {
	s := newTest(t, time.Hour)
	dir := t.TempDir()
	v1, v2, link := filepath.Join(dir, "v1"), filepath.Join(dir, "v2"), filepath.Join(dir, "matchblox")
	for _, p := range []string{v1, v2} {
		if err := os.WriteFile(p, []byte("same"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	at := time.Now().Add(-time.Hour)
	_ = os.Chtimes(v1, at, at)
	_ = os.Chtimes(v2, at, at)
	if err := os.Symlink(v1, link); err != nil {
		t.Fatal(err)
	}
	s.Exe, s.ExeEvery = link, 10*time.Millisecond
	_, done := serveForTest(t, s)
	waitWatching(t, s)
	tmp := link + ".new"
	if err := os.Symlink(v2, tmp); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, link); err != nil {
		t.Fatal(err)
	}
	waitServe(t, done, ErrReplaced)
}

// A login manager can remove the runtime directory under a live service;
// the next console starts a new one, so this one must go.
func TestServeStopsWhenSocketRemoved(t *testing.T) {
	s := newTest(t, time.Hour)
	s.ExeEvery = 10 * time.Millisecond
	path, done := serveForTest(t, s)
	waitWatching(t, s)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	waitServe(t, done, ErrSocketGone)
}

// TestServeStopsWhenSocketGoneBeforeItWatches: a socket removed between
// the listen and the start of the watch (a console that dials and removes
// it at once, a slow start) is gone all the same.
func TestServeStopsWhenSocketGoneBeforeItWatches(t *testing.T) {
	s := newTest(t, time.Hour)
	s.ExeEvery = 10 * time.Millisecond
	_, done := serveForTestBefore(t, s, func(sock string) {
		if err := os.Remove(sock); err != nil {
			t.Fatal(err)
		}
	})
	waitServe(t, done, ErrSocketGone)
}

func TestReplaceActStopsServe(t *testing.T) {
	s := newTest(t, time.Hour)
	path, done := serveForTest(t, s)
	c, err := transport.Dial(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version}); err != nil {
		t.Fatal(err)
	}
	next(t, c, proto.KindHello)
	if r := act(t, c, "r", proto.Act{RecID: "service:replace"}); r.Err != "" {
		t.Fatalf("result %+v", r)
	}
	waitServe(t, done, ErrReplaced)
}

func TestServiceHostOverrides(t *testing.T) {
	s := newTest(t, time.Hour)
	s.Host = "ws-1"
	ctx := run(t, s)
	if _, h := connect(t, ctx, s); h.Host != "ws-1" {
		t.Fatalf("host %q, want ws-1", h.Host)
	}
}

// hook sends one hook event the way `matchblox hook` does: read the
// service hello, send the event, leave.
func hook(t *testing.T, ctx context.Context, s *Service, ev hooks.Event) {
	t.Helper()
	a, b := net.Pipe()
	done := make(chan struct{})
	go func() { s.Handle(ctx, transport.NewConn(b)); close(done) }()
	c := transport.NewConn(a)
	if env, err := recv(t, c, "hello"); err != nil || env.Kind != proto.KindHello {
		t.Fatalf("hello %+v %v", env, err)
	}
	if err := c.Send(proto.KindHook, "", ev); err != nil {
		t.Fatal(err)
	}
	within(t, done, "end of the hook's connection")
	c.Close()
}

func waitQueue(t *testing.T, c transport.Conn, ok func([]queue.Item) bool) []queue.Item {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st := snapshot(t, c); ok(st.Queue) {
			return st.Queue
		}
	}
	t.Fatal("the queue never matched")
	return nil
}

func TestHookEventReachesQueue(t *testing.T) {
	s := newTest(t, time.Hour)
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	estimated := waitQueue(t, c, func(q []queue.Item) bool { return len(q) == 1 })
	if !estimated[0].Estimated {
		t.Fatalf("before any hook the idle session is a guess: %+v", estimated)
	}
	hook(t, ctx, s, hooks.Event{Name: "Notification", Kind: hooks.KindPermission, SessionID: "s9", Pane: "%1",
		Message: "Claude needs your permission to use Bash", At: time.Now()})
	q := waitQueue(t, c, func(q []queue.Item) bool { return len(q) == 1 && !q[0].Estimated })
	if q[0].State != queue.StatePermission || q[0].Name != "app" || q[0].LastLine != "Claude needs your permission to use Bash" {
		t.Fatalf("%+v", q[0])
	}
}

func TestServiceReplaysSpool(t *testing.T) {
	s := newTest(t, time.Hour)
	s.Spool = filepath.Join(t.TempDir(), "spool.jsonl")
	if err := hooks.Append(s.Spool, hooks.Event{Name: "Stop", SessionID: "s9", Pane: "%1", Message: "Done.", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	q := waitQueue(t, c, func(q []queue.Item) bool { return len(q) == 1 && !q[0].Estimated })
	if q[0].State != queue.StateFinished || q[0].LastLine != "Done." {
		t.Fatalf("%+v", q[0])
	}
	if _, err := os.Stat(s.Spool); !os.IsNotExist(err) {
		t.Fatalf("spool still there: %v", err)
	}
}

func answerTest(t *testing.T) (*Service, context.Context, *[][]string) {
	t.Helper()
	s := newTest(t, time.Hour)
	var ran [][]string
	s.Actions.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	hook(t, ctx, s, hooks.Event{Name: "Stop", SessionID: "s9", Pane: "%1", At: time.Now()})
	waitQueue(t, c, func(q []queue.Item) bool { return len(q) == 1 && !q[0].Estimated })
	return s, ctx, &ran
}

func TestAnswerTypesLiteral(t *testing.T) {
	s, ctx, ran := answerTest(t)
	res := s.act(ctx, proto.Act{RecID: "answer:%1", Which: "secondary", Confirm: "y", Text: "; rm -rf /"})
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	want := [][]string{{"tmux", "send-keys", "-t", "%1", "-l", "--", "; rm -rf /"}, {"tmux", "send-keys", "-t", "%1", "Enter"}}
	if fmt.Sprint(*ran) != fmt.Sprint(want) {
		t.Fatalf("ran %q", *ran)
	}
}

func TestAnswerNeedsTypedY(t *testing.T) {
	s, ctx, ran := answerTest(t)
	if res := s.act(ctx, proto.Act{RecID: "answer:%1", Which: "secondary", Text: "yes"}); len(*ran) != 0 || res.Err == "" {
		t.Fatalf("ran %q res %+v", *ran, res)
	}
}

func TestAnswerRefusesPaneNotWaiting(t *testing.T) {
	s, ctx, ran := answerTest(t)
	for _, a := range []proto.Act{
		{RecID: "answer:%2", Which: "secondary", Confirm: "y", Text: "yes"},
		{RecID: "answer:%1", Which: "secondary", Confirm: "y", Text: "two\nlines"},
	} {
		if res := s.act(ctx, a); res.Err == "" {
			t.Fatalf("%+v ran: %+v", a, res)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

// Review 1: Enter at a permission prompt approves it, so an answer never
// goes to a session that asks for permission.
func TestAnswerRefusesPermission(t *testing.T) {
	s, ctx, ran := answerTest(t)
	hook(t, ctx, s, hooks.Event{Name: "Notification", Kind: hooks.KindPermission, SessionID: "s9", Pane: "%1", At: time.Now()})
	res := s.act(ctx, proto.Act{RecID: "answer:%1", Which: "secondary", Confirm: "y", Text: "no, stop"})
	if res.Err == "" || len(*ran) != 0 {
		t.Fatalf("answered a permission prompt: %+v ran %q", res, *ran)
	}
}

// Review 3: the Enter step is guarded too.
func TestAnswerGuardsEveryStep(t *testing.T) {
	a := answerAction("%1", "yes")
	if len(a.Guards) != len(a.Steps) {
		t.Fatalf("%d guards for %d steps", len(a.Guards), len(a.Steps))
	}
	for _, g := range a.Guards {
		if g.AnswerPane != "%1" {
			t.Fatalf("guard %+v", g)
		}
	}
}

// Review 2: the idle guard (compact, clear, night mode) reads the sampler
// only; a queued session at a permission prompt is not idle.
func TestIdleGuardIgnoresQueue(t *testing.T) {
	s := newTest(t, time.Hour)
	s.sample = func() sample.Snapshot {
		return sample.Snapshot{At: time.Now(), Sessions: []sample.Session{{Pane: "%1", Name: "app", Status: "busy", Busy: true}}}
	}
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	hook(t, ctx, s, hooks.Event{Name: "Notification", Kind: hooks.KindPermission, SessionID: "s9", Pane: "%1", At: time.Now()})
	waitQueue(t, c, func(q []queue.Item) bool { return len(q) == 1 })
	if s.idle("%1") {
		t.Fatal("a session at a permission prompt passes the idle guard")
	}
}

// Review 4: the same answer to a new question is not "already done".
func TestAnswerAgainToANewQuestion(t *testing.T) {
	s, ctx, ran := answerTest(t)
	if res := s.act(ctx, proto.Act{RecID: "answer:%1", Which: "secondary", Confirm: "y", Text: "y"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	hook(t, ctx, s, hooks.Event{Name: "UserPromptSubmit", SessionID: "s9", Pane: "%1", At: time.Now()})
	hook(t, ctx, s, hooks.Event{Name: "Stop", SessionID: "s9", Pane: "%1", At: time.Now().Add(time.Millisecond)})
	res := s.act(ctx, proto.Act{RecID: "answer:%1", Which: "secondary", Confirm: "y", Text: "y"})
	if res.Err != "" || len(res.Skipped) != 0 || len(*ran) != 4 {
		t.Fatalf("second answer: %+v ran %q", res, *ran)
	}
}

// Review 5: a hook that missed the service waits in the spool for the next
// tick, not for the next restart.
func TestServiceReadsSpoolEveryTick(t *testing.T) {
	s := newTest(t, 20*time.Millisecond)
	s.Spool = filepath.Join(t.TempDir(), "spool.jsonl")
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	snapshot(t, c)
	if err := hooks.Append(s.Spool, hooks.Event{Name: "Stop", SessionID: "s9", Pane: "%1", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	waitQueue(t, c, func(q []queue.Item) bool { return len(q) == 1 && !q[0].Estimated })
}

// TestLimitsAreStoredAndInTheState: each new reading of an account goes to
// the local limits file, and the state carries the limits for `status`.
func TestLimitsAreStoredAndInTheState(t *testing.T) {
	s := newTest(t, 20*time.Millisecond)
	s.LimitsLog = &limits.Store{Path: filepath.Join(t.TempDir(), "limits.jsonl")}
	at := time.Now().Add(-time.Minute)
	s.sample = func() sample.Snapshot {
		snap := orphanSnap
		snap.At = time.Now()
		snap.Limits = []limits.Account{{Provider: "anthropic", Account: "a@example.com · Max", State: limits.Measured, At: at,
			Windows: []limits.Window{{UsedPct: 30, Minutes: limits.WeekMinutes}}}}
		return snap
	}
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	if st := snapshot(t, c); len(st.Limits) != 1 || st.Limits[0].Windows[0].UsedPct != 30 {
		t.Fatalf("limits in the state: %+v", st.Limits)
	}
	snapshot(t, c)
	b, err := os.ReadFile(s.LimitsLog.Path)
	if err != nil || strings.Count(string(b), "\n") != 1 || !strings.Contains(string(b), `"used_pct":30`) {
		t.Fatalf("the store: %q %v", b, err)
	}
}

func TestARemovedWorktreeLeavesTheGitList(t *testing.T) {
	s := newTest(t, time.Hour)
	removed := false
	var mu sync.Mutex
	s.git = func([]string) gitscan.Report {
		mu.Lock()
		defer mu.Unlock()
		if removed {
			return gitscan.Report{Repos: []gitscan.Repo{{Path: "/w/app", Main: "main"}}}
		}
		return gitReport
	}
	s.Actions.Run = func([]string) error { mu.Lock(); removed = true; mu.Unlock(); return nil }
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	snapshot(t, c)
	time.Sleep(20 * time.Millisecond) // the first git scan
	if r := act(t, c, "x", proto.Act{RecID: "worktree:/w/wt/a", Which: "secondary", Confirm: "y"}); r.Err != "" || len(r.Ran) != 1 {
		t.Fatalf("%+v", r)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if g := s.State().Git; g != nil && len(g.Repos) == 1 && len(g.Repos[0].Worktrees) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the removed worktree is still listed: %+v", s.State().Git)
}

// TestShipsReachTheConsole: the service reads the ships with the sessions
// of its first sample and the git scan, and every console gets them.
func TestShipsReachTheConsole(t *testing.T) {
	s := newTest(t, time.Hour)
	got := make(chan []sample.Session, 1)
	s.Ships = func(_ context.Context, ss []sample.Session, _ *gitscan.Report) ships.Report {
		select {
		case got <- ss:
		default:
		}
		return ships.Report{InFlight: 1, Ships: []ships.Ship{{Repo: "o/r", Number: 7, Kind: ships.WaitsForYou,
			Note: "green · waits for your merge", Merge: []string{"gh", "pr", "merge", "7", "--squash", "-R", "o/r"}}}}
	}
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	if ss := <-got; len(ss) != 1 || ss[0].Pane != "%1" {
		t.Fatalf("the source got sessions %+v", ss)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		st := snapshot(t, c)
		if st.Ships != nil {
			if st.Ships.InFlight != 1 || st.Ships.Ships[0].Number != 7 || len(st.Ships.Ships[0].Merge) != 7 {
				t.Fatalf("ships %+v", st.Ships)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no ships in the state")
		}
	}
	// x on a ship that waits for your merge: its merge, after a typed y.
	var ran [][]string
	s.Actions.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	for _, a := range []proto.Act{{RecID: "ship:o/r#7", Which: "secondary"}, {RecID: "ship:o/r#8", Which: "secondary", Confirm: "y"}} {
		if res := s.act(ctx, a); res.Err == "" {
			t.Fatalf("%+v ran: %+v", a, res)
		}
	}
	if res := s.act(ctx, proto.Act{RecID: "ship:o/r#7", Which: "secondary", Confirm: "y"}); res.Err != "" ||
		fmt.Sprint(ran) != "[[gh pr merge 7 --squash -R o/r]]" {
		t.Fatalf("res %+v ran %q", res, ran)
	}
}

// TestPickDrivesTheQuestionMenu: a pick goes to the pane as the question
// tool's keys, after its own check of the call it answers; a pick of
// another call, an option the call lacks, or one with no confirm runs
// nothing.
func TestPickDrivesTheQuestionMenu(t *testing.T) {
	s := newTest(t, time.Hour)
	ask := &said.Ask{ID: "toolu_1", Questions: []said.Question{{Question: "Which next?", Options: []said.Option{{Label: "a"}, {Label: "b"}}}}}
	s.sample = func() sample.Snapshot {
		return sample.Snapshot{At: time.Now(), Sessions: []sample.Session{{Pane: "%1", Name: "app", Ask: ask}}}
	}
	s.Capture = func([]string) map[string]string {
		return map[string]string{"%1": "Which next?\n❯ 1. a\n  2. b\n  3. Type something.\n"}
	}
	var ran [][]string
	s.Actions.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	ctx := run(t, s)
	for deadline := time.Now().Add(5 * time.Second); !s.asking("%1", "toolu_1"); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the service never sampled the question")
		}
	}
	pick := func(id string, picks ...int) string {
		b, _ := json.Marshal(said.Answer{ID: id, Choices: []said.Choice{{Picks: picks}}})
		return string(b)
	}
	for _, a := range []proto.Act{
		{RecID: "pick:%1", Confirm: "y", Text: pick("toolu_0", 1)},
		{RecID: "pick:%1", Confirm: "y", Text: pick("toolu_1", 3)},
		{RecID: "pick:%1", Text: pick("toolu_1", 2)},
		{RecID: "pick:%2", Confirm: "y", Text: pick("toolu_1", 2)},
	} {
		if res := s.act(ctx, a); res.Err == "" {
			t.Fatalf("%+v ran: %+v", a, res)
		}
	}
	if len(ran) != 0 {
		t.Fatalf("ran %q", ran)
	}
	if res := s.act(ctx, proto.Act{RecID: "pick:%1", Confirm: "y", Text: pick("toolu_1", 2)}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if want := [][]string{{"tmux", "send-keys", "-t", "%1", "2"}}; fmt.Sprint(ran) != fmt.Sprint(want) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
}

// pickTest is a service whose session in %1 asks two questions, with the
// pane's screen in *screen and every key it ran in *ran.
func pickTest(t *testing.T) (s *Service, ctx context.Context, screen *string, ran *[][]string, mu *sync.Mutex) {
	t.Helper()
	old := menuWait
	menuWait = 200 * time.Millisecond
	t.Cleanup(func() { menuWait = old })
	s = newTest(t, time.Hour)
	ask := &said.Ask{ID: "toolu_1", Questions: []said.Question{
		{Question: "Which cache?", Options: []said.Option{{Label: "Redis"}, {Label: "Memory"}}},
		{Question: "Which parts?", Options: []said.Option{{Label: "Sessions"}, {Label: "Tokens"}}, Multi: true},
	}}
	s.sample = func() sample.Snapshot {
		return sample.Snapshot{At: time.Now(), Sessions: []sample.Session{{Pane: "%1", Name: "app", Ask: ask}}}
	}
	mu, ran, screen = &sync.Mutex{}, &[][]string{}, new(string)
	*screen = pickPage1
	s.Capture = func([]string) map[string]string {
		mu.Lock()
		defer mu.Unlock()
		return map[string]string{"%1": *screen}
	}
	s.Actions.Run = func(argv []string) error {
		mu.Lock()
		defer mu.Unlock()
		*ran = append(*ran, argv)
		return nil
	}
	ctx = run(t, s)
	for deadline := time.Now().Add(5 * time.Second); !s.asking("%1", "toolu_1"); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the service never sampled the question")
		}
	}
	return s, ctx, screen, ran, mu
}

const (
	pickPage1  = "Which cache?\n❯ 1. Redis\n  2. Memory\n  3. Type something.\n"
	pickPage2  = "Which parts?\n❯ 1. [ ] Sessions\n  2. [ ] Tokens\n  3. [ ] Type something\n     Next\n"
	pickReview = "Ready to submit your answers?\n❯ 1. Submit answers\n  2. Cancel\n"
)

func twoPicks(picks ...int) string {
	b, _ := json.Marshal(said.Answer{ID: "toolu_1", Choices: []said.Choice{{Picks: picks[:1]}, {Picks: picks[1:]}}})
	return string(b)
}

// A person answered question 1 in the pane: the pane shows question 2, so
// the pick sends nothing (review of #68, finding 2).
func TestPickChecksThePaneBeforeEachPage(t *testing.T) {
	s, ctx, screen, ran, mu := pickTest(t)
	mu.Lock()
	*screen = pickPage2
	mu.Unlock()
	res := s.act(ctx, proto.Act{RecID: "pick:%1", Confirm: "y", Text: twoPicks(1, 2)})
	if len(*ran) != 0 || len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0], "does not show question 1") {
		t.Fatalf("ran %q: %+v", *ran, res)
	}
}

// The pane stays on question 1 after its keys (the key was lost): the pick
// stops before question 2's keys.
func TestPickStopsWhenThePageDoesNotMove(t *testing.T) {
	s, ctx, _, ran, _ := pickTest(t)
	res := s.act(ctx, proto.Act{RecID: "pick:%1", Confirm: "y", Text: twoPicks(1, 2)})
	if want := [][]string{{"tmux", "send-keys", "-t", "%1", "1"}, panes.Pause}; fmt.Sprint(*ran) != fmt.Sprint(want) {
		t.Fatalf("ran %q, want %q", *ran, want)
	}
	if len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0], "does not show question 2") {
		t.Fatalf("result %+v", res)
	}
}

// The pane moves through the pages as the keys go: every page's keys run,
// the last one the review page's.
func TestPickFollowsThePages(t *testing.T) {
	s, ctx, screen, ran, mu := pickTest(t)
	s.Actions.Run = func(argv []string) error {
		mu.Lock()
		defer mu.Unlock()
		*ran = append(*ran, argv)
		switch k := argv[len(argv)-1]; {
		case len(*ran) == 1 && k == "1":
			*screen = pickPage2
		case k == "Enter":
			*screen = pickReview
		}
		return nil
	}
	if res := s.act(ctx, proto.Act{RecID: "pick:%1", Confirm: "y", Text: twoPicks(1, 2)}); res.Err != "" || len(res.Skipped) > 0 {
		t.Fatalf("result %+v", res)
	}
	if n := len(*ran); n == 0 || fmt.Sprint((*ran)[n-1]) != fmt.Sprint([]string{"tmux", "send-keys", "-t", "%1", "1"}) {
		t.Fatalf("ran %q", *ran)
	}
}

// Two consoles' picks to one pane never interleave their keys.
func TestKeysToOnePaneNeverInterleave(t *testing.T) {
	s, ctx, _, _, _ := pickTest(t)
	var mu sync.Mutex
	inside, most := 0, 0
	s.Actions.Run = func(argv []string) error {
		mu.Lock()
		inside++
		most = max(most, inside)
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		inside--
		mu.Unlock()
		return nil
	}
	done := make(chan proto.Result)
	for _, picks := range [][]int{{1, 2}, {2, 1}} {
		go func() { done <- s.act(ctx, proto.Act{RecID: "pick:%1", Confirm: "y", Text: twoPicks(picks...)}) }()
	}
	within(t, done, "first pick")
	within(t, done, "second pick")
	if most != 1 {
		t.Fatalf("the keys of two picks overlapped (%d at once)", most)
	}
}

// A ships scan in flight stops when the service does: the source gets the
// service's context, not one of its own.
func TestAShipsScanStopsWithTheService(t *testing.T) {
	s := newTest(t, 10*time.Millisecond)
	started, stopped := make(chan struct{}), make(chan struct{})
	s.Ships = func(ctx context.Context, _ []sample.Session, _ *gitscan.Report) ships.Report {
		close(started)
		select {
		case <-ctx.Done():
			close(stopped)
		case <-time.After(10 * time.Second):
		}
		return ships.Report{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	within(t, started, "ships scan")
	cancel()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the scan did not see the service stop")
	}
	within(t, done, "end of the service")
}

// TestPickTakesTheHookCall: in auto mode the transcript shows no call while
// the menu waits; the PreToolUse hook's call is answered, and its
// PostToolUse ends it (design 0006 §11.8, review of #68).
func TestPickTakesTheHookCall(t *testing.T) {
	s := newTest(t, time.Hour)
	s.sample = func() sample.Snapshot {
		return sample.Snapshot{At: time.Now(), Sessions: []sample.Session{{SessionID: "s1", Pane: "%1", Name: "app"}}}
	}
	s.Capture = func([]string) map[string]string {
		return map[string]string{"%1": "Which next?\n❯ 1. a\n  2. b\n  3. Type something.\n"}
	}
	var ran [][]string
	s.Actions.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	ctx := run(t, s)
	for deadline := time.Now().Add(5 * time.Second); !s.sampledOnce(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the service never sampled")
		}
	}
	answer, _ := json.Marshal(said.Answer{ID: "toolu_1", Choices: []said.Choice{{Picks: []int{2}}}})
	pick := proto.Act{RecID: "pick:%1", Confirm: "y", Text: string(answer)}
	if res := s.act(ctx, pick); !strings.Contains(res.Err, "no longer asks") {
		t.Fatalf("no call yet: %+v", res)
	}
	ask := &said.Ask{ID: "toolu_1", Questions: []said.Question{{Question: "Which next?", Options: []said.Option{{Label: "a"}, {Label: "b"}}}}}
	s.Hook(hooks.Event{Name: "PreToolUse", SessionID: "s1", Pane: "%1", At: time.Now(), ToolUseID: "toolu_1", Ask: ask})
	if res := s.act(ctx, pick); res.Err != "" {
		t.Fatal(res.Err)
	}
	if want := [][]string{{"tmux", "send-keys", "-t", "%1", "2"}}; fmt.Sprint(ran) != fmt.Sprint(want) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
	s.Hook(hooks.Event{Name: "PostToolUse", SessionID: "s1", Pane: "%1", At: time.Now(), ToolUseID: "toolu_1"})
	if _, ok := s.askIn("%1"); ok {
		t.Fatal("the answered call still waits")
	}
}

func (s *Service) sampledOnce() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.have
}

func messageTest(t *testing.T) (*Service, context.Context, *[][]string) {
	t.Helper()
	s := newTest(t, time.Hour)
	s.sample = func() sample.Snapshot {
		return sample.Snapshot{At: time.Now(), Sessions: []sample.Session{
			{PID: 9, Start: 3, Pane: "%1", Name: "app", Status: "busy", Busy: true},
			{PID: 10, Pane: "%2", Name: "web", Status: "idle", Ask: &said.Ask{ID: "toolu_1"}},
		}}
	}
	var ran [][]string
	s.Actions.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	snapshot(t, c)
	return s, ctx, &ran
}

// A message goes to a session that works: Claude Code queues it.
func TestMessageTypesIntoABusySession(t *testing.T) {
	s, ctx, ran := messageTest(t)
	res := s.act(ctx, proto.Act{RecID: "message:%1", Which: "secondary", Confirm: "y", Text: "also run the e2e"})
	if res.Err != "" || len(res.Skipped) != 0 {
		t.Fatalf("%+v", res)
	}
	want := [][]string{{"tmux", "send-keys", "-t", "%1", "-l", "--", "also run the e2e"}, {"tmux", "send-keys", "-t", "%1", "Enter"}}
	if fmt.Sprint(*ran) != fmt.Sprint(want) {
		t.Fatalf("ran %q", *ran)
	}
}

// A message never goes into a question menu as text, nor to a permission
// prompt, nor to a pane without an agent; it is one line, after a y.
func TestMessageRefuses(t *testing.T) {
	s, ctx, ran := messageTest(t)
	for _, a := range []proto.Act{
		{RecID: "message:%2", Which: "secondary", Confirm: "y", Text: "go"},
		{RecID: "message:%3", Which: "secondary", Confirm: "y", Text: "go"},
		{RecID: "message:%1", Which: "secondary", Confirm: "y", Text: "two\nlines"},
		{RecID: "message:%1", Which: "secondary", Text: "go"},
	} {
		if res := s.act(ctx, a); res.Err == "" && len(res.Skipped) == 0 {
			t.Fatalf("%+v ran: %+v", a, res)
		}
	}
	hook(t, ctx, s, hooks.Event{Name: "Notification", Kind: hooks.KindPermission, SessionID: "s9", Pane: "%1", At: time.Now()})
	for deadline := time.Now().Add(5 * time.Second); ; {
		if it, ok := s.queued("%1"); ok && it.State == queue.StatePermission {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the permission prompt never queued")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if res := s.act(ctx, proto.Act{RecID: "message:%1", Which: "secondary", Confirm: "y", Text: "go"}); res.Err == "" {
		t.Fatalf("messaged a permission prompt: %+v", res)
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

// Each step is guarded on the same agent: another pid in the pane stops it.
func TestMessageGuardsEveryStep(t *testing.T) {
	s, _, _ := messageTest(t)
	a := messageAction("%1", sample.Session{PID: 9, Start: 3}, "go")
	if len(a.Guards) != len(a.Steps) {
		t.Fatalf("%d guards for %d steps", len(a.Guards), len(a.Steps))
	}
	for _, g := range a.Guards {
		if g.MessagePane != "%1" || g.MessagePID != 9 || g.PID != 9 || g.StartTicks != 3 {
			t.Fatalf("guard %+v", g)
		}
	}
	if err := s.messageable("%1", 11); err == nil {
		t.Fatal("a message goes to another agent in the pane")
	}
	if err := s.messageable("%1", 9); err != nil {
		t.Fatal(err)
	}
}

// TestAConsoleThatJoinsDuringABroadcastGetsItsSnapshot: a broadcast that
// lands between a console's join and its first wake-up must not leave the
// console without a snapshot (TestIdleGuardIgnoresQueue once hung there).
func TestAConsoleThatJoinsDuringABroadcastGetsItsSnapshot(t *testing.T) {
	s := newTest(t, time.Hour)
	s.publish(s.sample(), func(state.Doc) {})
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				s.broadcast()
			}
		}
	}()
	for range 300 {
		c, _ := connect(t, t.Context(), s)
		snapshot(t, c)
		c.Close()
	}
}

func TestKnownAccountsPerAgent(t *testing.T) {
	s, _, home := accountTest(t)
	s.GOOS = "darwin"
	prepared(t, home, "claude", "personal")
	prepared(t, home, "claude", "work")
	prepared(t, home, "codex", "work")
	snap := lifeSnap(false)
	snap.Sessions[0].AccountDir = filepath.Join(home, ".claude-old")
	snap.Sessions[1].AccountDir = filepath.Join(home, ".claude-old") // twice: listed once
	snap.Sessions[2].AccountDir = accounts.Folder(home, "codex", "work")
	s.publish(snap, func(state.Doc) {})
	want := map[string][]string{
		"claude":       {"default", "personal", "work", "~/.claude-old"},
		"codex":        {"default", "work"},
		"cursor-agent": {"default"},
	}
	if got := s.State().Accounts; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestKnownAccountsHideCursorOffMac(t *testing.T) {
	s, _, home := accountTest(t) // linux
	prepared(t, home, "cursor-agent", "work")
	s.publish(lifeSnap(false), func(state.Doc) {})
	got := s.State().Accounts
	if _, ok := got["cursor-agent"]; ok {
		t.Fatalf("Cursor has accounts off macOS: %v", got)
	}
	if _, ok := got["claude"]; !ok {
		t.Fatalf("claude lost its accounts: %v", got)
	}
}

// TestKnownAccountsOnASymlinkedHomeListOnce: a session whose account folder
// is named by its real path, under a symlinked home, is that account, and
// the list has it once.
func TestKnownAccountsOnASymlinkedHomeListOnce(t *testing.T) {
	s, _, home := accountTest(t)
	prepared(t, home, "claude", "work")
	real, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	link := real + "-link"
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(link) })
	s.Home = link
	snap := lifeSnap(false)
	snap.Sessions[0].AccountDir = accounts.Folder(real, "claude", "work")
	s.publish(snap, func(state.Doc) {})
	want := []string{"default", "work"}
	if got := s.State().Accounts["claude"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestAFolderWithABadNameIsNamed: the state names a folder under the
// accounts root that no account may be called, and why, beside the list.
func TestAFolderWithABadNameIsNamed(t *testing.T) {
	s, _, home := accountTest(t)
	prepared(t, home, "claude", "work")
	bad := filepath.Join(accounts.Root(home), "claude", "my_acct")
	if err := os.MkdirAll(bad, 0o700); err != nil {
		t.Fatal(err)
	}
	s.publish(lifeSnap(false), func(state.Doc) {})
	st := s.State()
	if want := []string{"default", "work"}; !reflect.DeepEqual(st.Accounts["claude"], want) {
		t.Fatalf("accounts %v, want %v", st.Accounts["claude"], want)
	}
	want := map[string][]accounts.Skip{"claude": {{Path: bad, Why: "not a-z, 0-9 and -, up to 10"}}}
	if !reflect.DeepEqual(st.AccountsSkipped, want) {
		t.Fatalf("skipped %v, want %v", st.AccountsSkipped, want)
	}
}

// TestUnreadableAccountsRootIsNamed: when the root of an agent's accounts
// cannot be read, the state lists default only and names the error.
func TestUnreadableAccountsRootIsNamed(t *testing.T) {
	s, _, home := accountTest(t)
	root := filepath.Join(accounts.Root(home), "codex")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
	if _, err := os.ReadDir(root); err == nil {
		t.Skip("this user reads a folder with no permission")
	}
	s.publish(lifeSnap(false), func(state.Doc) {})
	st := s.State()
	if want := []string{"default"}; !reflect.DeepEqual(st.Accounts["codex"], want) {
		t.Fatalf("accounts %v, want %v", st.Accounts["codex"], want)
	}
	got := st.AccountsSkipped["codex"]
	if len(got) != 1 || got[0].Path != root || !strings.Contains(got[0].Why, "permission denied") {
		t.Fatalf("skipped %v", st.AccountsSkipped)
	}
	if len(st.AccountsSkipped) != 1 {
		t.Fatalf("only codex is unreadable: %v", st.AccountsSkipped)
	}
}

// agedAccounts sets the mtime of the accounts folders back an hour, so a
// read of them is not too fresh to trust.
func agedAccounts(t *testing.T, home string) {
	t.Helper()
	old := time.Now().Add(-time.Hour)
	for _, a := range accountAgents {
		if err := os.Chtimes(filepath.Join(accounts.Root(home), a), old, old); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
}

// TestKnownAccountsNotReadEachSample: the account folders are read when
// their root changes or a live session's folder does, not on every sample.
func TestKnownAccountsNotReadEachSample(t *testing.T) {
	s, _, home := accountTest(t)
	s.GOOS = "darwin"
	prepared(t, home, "claude", "work")
	agedAccounts(t, home)
	var reads int
	s.readDir = func(p string) ([]os.DirEntry, error) { reads++; return os.ReadDir(p) }
	snap := lifeSnap(false)
	snap.Sessions[0].AccountDir = filepath.Join(home, ".claude-old")
	s.publish(snap, func(state.Doc) {})
	first := reads
	if first == 0 {
		t.Fatal("the first publish read no folder")
	}
	for range 3 {
		s.publish(snap, func(state.Doc) {})
	}
	if reads != first {
		t.Fatalf("%d reads after the first publish's %d: the folders are read once", reads, first)
	}
	if want := []string{"default", "work", "~/.claude-old"}; !reflect.DeepEqual(s.State().Accounts["claude"], want) {
		t.Fatalf("got %v, want %v", s.State().Accounts["claude"], want)
	}
	prepared(t, home, "claude", "new")
	s.publish(snap, func(state.Doc) {})
	if want := []string{"default", "new", "work", "~/.claude-old"}; !reflect.DeepEqual(s.State().Accounts["claude"], want) {
		t.Fatalf("a new account: got %v, want %v", s.State().Accounts["claude"], want)
	}
	snap.Sessions[1].AccountDir = filepath.Join(home, ".claude-two")
	s.publish(snap, func(state.Doc) {})
	if want := []string{"default", "new", "work", "~/.claude-old", "~/.claude-two"}; !reflect.DeepEqual(s.State().Accounts["claude"], want) {
		t.Fatalf("a new live folder: got %v, want %v", s.State().Accounts["claude"], want)
	}
}

// TestLiveFolderByLinkOrCase: a shell names a live hand-made folder by its
// real path, or in another letter case: it is the same folder.
func TestLiveFolderByLinkOrCase(t *testing.T) {
	s, _, home := accountTest(t)
	hand := filepath.Join(home, ".claude-work")
	if err := os.Mkdir(hand, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, "link")
	if err := os.Symlink(hand, link); err != nil {
		t.Fatal(err)
	}
	snap := lifeSnap(false)
	snap.Sessions[0].AccountDir = hand
	s.publish(snap, func(state.Doc) {})
	if f, ok := s.liveFolder("claude", link); !ok || f != hand {
		t.Fatalf("by a link: %q, %v", f, ok)
	}
	t.Run("case", func(t *testing.T) {
		other := filepath.Join(home, ".CLAUDE-WORK")
		if _, err := os.Stat(other); err != nil {
			t.Skip("this file system tells letter case apart")
		}
		if f, ok := s.liveFolder("claude", other); !ok || f != hand {
			t.Fatalf("by another case: %q, %v", f, ok)
		}
	})
}

// TestKnownAccountsReadAgainAfterAnError: a root that could not be read is
// read again at the next publish, though a chmod leaves its mtime as it was.
func TestKnownAccountsReadAgainAfterAnError(t *testing.T) {
	s, _, home := accountTest(t)
	prepared(t, home, "codex", "work")
	root := filepath.Join(accounts.Root(home), "codex")
	agedAccounts(t, home)
	if err := os.Chmod(root, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
	if _, err := os.ReadDir(root); err == nil {
		t.Skip("this user reads a folder with no permission")
	}
	s.publish(lifeSnap(false), func(state.Doc) {})
	if got := s.State().Accounts["codex"]; !reflect.DeepEqual(got, []string{"default"}) {
		t.Fatalf("unreadable: %v", got)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	s.publish(lifeSnap(false), func(state.Doc) {})
	if got, want := s.State().Accounts["codex"], []string{"default", "work"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after the fix: %v, want %v", got, want)
	}
	if len(s.State().AccountsSkipped) != 0 {
		t.Fatalf("still named: %v", s.State().AccountsSkipped)
	}
}

// TestKnownAccountsNotKeptFromAFreshRoot: an answer read while the root was
// too new to trust is not kept once it is old enough.
func TestKnownAccountsNotKeptFromAFreshRoot(t *testing.T) {
	s, _, home := accountTest(t)
	prepared(t, home, "claude", "work")
	fi, err := os.Stat(filepath.Join(accounts.Root(home), "claude"))
	if err != nil {
		t.Fatal(err)
	}
	at := fi.ModTime().Add(time.Second)
	s.Now = func() time.Time { return at }
	var reads int
	s.readDir = func(p string) ([]os.DirEntry, error) { reads++; return os.ReadDir(p) }
	s.publish(lifeSnap(false), func(state.Doc) {})
	first := reads
	at = at.Add(time.Minute)
	s.publish(lifeSnap(false), func(state.Doc) {})
	if reads == first {
		t.Fatal("the answer read from a fresh root was kept")
	}
	again := reads
	s.publish(lifeSnap(false), func(state.Doc) {})
	if reads != again {
		t.Fatal("an old root is read again")
	}
}

// TestTwoSpellingsOfAHandMadeFolderListOnce: one folder named by a link and
// by its real path is one entry, in the spelling first seen.
func TestTwoSpellingsOfAHandMadeFolderListOnce(t *testing.T) {
	s, _, home := accountTest(t)
	hand := filepath.Join(home, ".claude-work")
	if err := os.Mkdir(hand, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".claude-link")
	if err := os.Symlink(hand, link); err != nil {
		t.Fatal(err)
	}
	snap := lifeSnap(false)
	snap.Sessions[0].AccountDir = link
	snap.Sessions[1].AccountDir = hand
	s.publish(snap, func(state.Doc) {})
	if got, want := s.State().Accounts["claude"], []string{"default", "~/.claude-link"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
