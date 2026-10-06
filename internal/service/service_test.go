package service

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
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
	t.Cleanup(cancel)
	go s.Run(ctx)
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
	env, err := c.Recv()
	if err != nil || env.Kind != proto.KindHello {
		t.Fatalf("first message %+v, %v", env, err)
	}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	var h proto.Hello
	_ = json.Unmarshal(env.Body, &h)
	return c, h
}

func next(t *testing.T, c transport.Conn, kind proto.Kind) proto.Envelope {
	t.Helper()
	for {
		env, err := c.Recv()
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
	if !s.idle("%1") || s.idle("%9") {
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
	time.Sleep(30 * time.Millisecond)
	if err := os.WriteFile(exe, []byte("v2 is longer"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrReplaced) {
			t.Fatalf("Serve returned %v, want ErrReplaced", err)
		}
	case <-time.After(2 * time.Second):
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

func waitServe(t *testing.T, done chan error, want error) {
	t.Helper()
	select {
	case err := <-done:
		if !errors.Is(err, want) {
			t.Fatalf("Serve returned %v, want %v", err, want)
		}
	case <-time.After(2 * time.Second):
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
	time.Sleep(30 * time.Millisecond)
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
	time.Sleep(30 * time.Millisecond)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
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
