package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/transport"
)

func connectAs(t *testing.T, ctx context.Context, s *Service, client string) transport.Conn {
	t.Helper()
	a, b := net.Pipe()
	go s.HandlePeer(ctx, transport.NewConn(b), 0) //nolint:errcheck // ends with the test
	c := transport.NewConn(a)
	t.Cleanup(func() { c.Close() })
	sent := make(chan error, 1)
	go func() { sent <- c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version, Client: client}) }()
	next(t, c, proto.KindHello)
	if err := within(t, sent, "sent hello"); err != nil {
		t.Fatal(err)
	}
	return c
}

// reader is the one reader of a console: it keeps the screens and the
// results that come, so a test can act and watch over one connection.
type reader struct {
	c       transport.Conn
	mu      sync.Mutex
	screens []proto.Screen
	results map[string]proto.Result
}

func read(c transport.Conn) *reader {
	r := &reader{c: c, results: map[string]proto.Result{}}
	go func() {
		for {
			env, err := c.Recv()
			if err != nil {
				return
			}
			r.mu.Lock()
			switch env.Kind {
			case proto.KindScreen:
				var sc proto.Screen
				if json.Unmarshal(env.Body, &sc) == nil {
					r.screens = append(r.screens, sc)
				}
			case proto.KindResult:
				var res proto.Result
				if json.Unmarshal(env.Body, &res) == nil {
					r.results[env.ID] = res
				}
			}
			r.mu.Unlock()
		}
	}()
	return r
}

func (r *reader) got() []proto.Screen {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]proto.Screen(nil), r.screens...)
}

// act sends one act and waits for its result.
func (r *reader) act(t *testing.T, id, recID string) proto.Result {
	t.Helper()
	return r.actText(t, id, recID, "")
}

// actText is act with the act's text.
func (r *reader) actText(t *testing.T, id, recID, text string) proto.Result {
	t.Helper()
	return r.actWith(t, id, proto.Act{RecID: recID, Text: text})
}

// actWith sends the act a and waits for its result.
func (r *reader) actWith(t *testing.T, id string, a proto.Act) proto.Result {
	t.Helper()
	if err := r.c.Send(proto.KindAct, id, a); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(waitLimit)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		res, ok := r.results[id]
		r.mu.Unlock()
		if ok {
			return res
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no result for %s", id)
	return proto.Result{}
}

// captureOf serves the text of each pane in screens; set changes it.
type captureOf struct {
	mu      sync.Mutex
	screens map[string]string
	calls   int
	// leave publishes a snapshot whose sessions are in panes.
	leave func(panes ...string)
}

func (f *captureOf) capture(panes []string) map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	out := map[string]string{}
	for _, p := range panes {
		if text, ok := f.screens[p]; ok {
			out[p] = text
		}
	}
	return out
}

func (f *captureOf) set(pane, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.screens[pane] = text
}

// watchTest serves screens and a session in each pane of sessions (default:
// each pane with a screen).
func watchTest(t *testing.T, screens map[string]string, sessions ...string) (*Service, *captureOf, context.Context) {
	t.Helper()
	s := newTest(t, time.Hour)
	f := &captureOf{screens: screens}
	s.Capture = f.capture
	if sessions == nil {
		for p := range screens {
			sessions = append(sessions, p)
		}
	}
	snap := func(panes []string) sample.Snapshot {
		out := sample.Snapshot{At: time.Now()}
		for i, p := range panes {
			out.Sessions = append(out.Sessions, sample.Session{Agent: "claude", PID: 100 + i, Pane: p, Name: p, Status: "idle"})
		}
		return out
	}
	s.sample = func() sample.Snapshot { return snap(sessions) }
	s.publish(snap(sessions), func(state.Doc) {})
	f.leave = func(panes ...string) { s.publish(snap(panes), func(state.Doc) {}) }
	return s, f, run(t, s)
}

func TestWatchSendsTheScreen(t *testing.T) {
	s, _, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	c := connectAs(t, ctx, s, proto.ClientConsole)
	r := read(c)
	if res := r.act(t, "w", proto.WatchPrefix+"%1"); res.Err != "" {
		t.Fatal(res.Err)
	}
	deadline := time.Now().Add(waitLimit)
	for len(r.got()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	g := r.got()
	if len(g) == 0 {
		t.Fatal("no screen")
	}
	if sc := g[0]; sc.Pane != "%1" || sc.Text != idleScreen || sc.At.IsZero() || sc.Gone {
		t.Fatalf("screen %+v", sc)
	}
}

func TestWatchSendsOnlyChanges(t *testing.T) {
	s, f, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	// The idle session's wait is new: a still screen goes once more when it
	// settles, by design. Watch after that, so only a change sends again.
	for deadline := time.Now().Add(waitLimit); !s.waitShown("%1", time.Now()); {
		if time.Now().After(deadline) {
			t.Fatal("the wait never settled")
		}
		time.Sleep(10 * time.Millisecond)
	}
	c := connectAs(t, ctx, s, proto.ClientConsole)
	r := read(c)
	if res := r.act(t, "w", proto.WatchPrefix+"%1"); res.Err != "" {
		t.Fatal(res.Err)
	}
	time.Sleep(500 * time.Millisecond)
	if n := len(r.got()); n != 1 {
		t.Fatalf("%d screens for one text, want 1", n)
	}
	f.set("%1", "changed\n")
	deadline := time.Now().Add(waitLimit)
	for len(r.got()) < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if g := r.got(); len(g) != 2 || g[1].Text != "changed\n" {
		t.Fatalf("screens %+v", g)
	}
}

// A wait stamped after the last screen sends that screen again, newer: an
// Enter for it is not refused while the text stands still.
func TestWatchSendsAStillScreenAgainForANewerWait(t *testing.T) {
	s, _, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	c := connectAs(t, ctx, s, proto.ClientConsole)
	r := read(c)
	if res := r.act(t, "w", proto.WatchPrefix+"%1"); res.Err != "" {
		t.Fatal(res.Err)
	}
	deadline := time.Now().Add(waitLimit)
	for len(r.got()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(r.got()) == 0 {
		t.Fatal("no screen")
	}
	since := r.got()[0].At.Add(time.Millisecond)
	s.mu.Lock()
	s.cur.Queue = []queue.Item{{Pane: "%1", Name: "%1", State: queue.StatePermission, Since: since}}
	s.mu.Unlock()
	for len(r.got()) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	g := r.got()
	if len(g) < 2 || g[1].Text != idleScreen || g[1].At.Before(since) {
		t.Fatalf("screens %+v, want the screen again after %v", g, since)
	}
	time.Sleep(400 * time.Millisecond)
	if n := len(r.got()); n != 2 {
		t.Fatalf("%d screens for one wait, want 2", n)
	}
	if res := s.act(ctx, proto.Act{Console: true, RecID: proto.KeysPrefix + "%1", Text: "Enter", PID: 100, Seen: g[1].At}); res.Err != "" || len(res.Ran) != 1 {
		t.Fatalf("enter on the newer screen: %+v", res)
	}
}

// A pane whose session left is gone: one screen says so.
func TestWatchPaneGone(t *testing.T) {
	s, f, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	c := connectAs(t, ctx, s, proto.ClientConsole)
	r := read(c)
	if res := r.act(t, "w", proto.WatchPrefix+"%1"); res.Err != "" {
		t.Fatal(res.Err)
	}
	r.screenThat(t, func(sc proto.Screen) bool { return sc.Text == idleScreen })
	f.leave()
	r.screenThat(t, func(sc proto.Screen) bool { return sc.Gone && sc.Err == "" })
	time.Sleep(500 * time.Millisecond)
	if g := r.got(); len(g) != 2 || !g[1].Gone {
		t.Fatalf("screens %+v, want one gone", g)
	}
}

func TestTwoConsolesWatchTheirOwnPane(t *testing.T) {
	s, _, ctx := watchTest(t, map[string]string{"%1": "one\n", "%2": "two\n"})
	a := connectAs(t, ctx, s, proto.ClientConsole)
	b := connectAs(t, ctx, s, proto.ClientConsole)
	ra, rb := read(a), read(b)
	for r, pane := range map[*reader]string{ra: "%1", rb: "%2"} {
		if res := r.act(t, "w", proto.WatchPrefix+pane); res.Err != "" {
			t.Fatal(res.Err)
		}
	}
	time.Sleep(500 * time.Millisecond)
	for _, tc := range []struct {
		got  []proto.Screen
		pane string
	}{{ra.got(), "%1"}, {rb.got(), "%2"}} {
		if len(tc.got) == 0 {
			t.Fatalf("no screen for %s", tc.pane)
		}
		for _, sc := range tc.got {
			if sc.Pane != tc.pane {
				t.Fatalf("a console watching %s got %+v", tc.pane, sc)
			}
		}
	}
}

func TestWatchEmptyStops(t *testing.T) {
	s, _, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	c := connectAs(t, ctx, s, proto.ClientConsole)
	r := read(c)
	r.act(t, "w", proto.WatchPrefix+"%1")
	deadline := time.Now().Add(waitLimit)
	for len(r.got()) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if res := r.act(t, "stop", proto.WatchPrefix); res.Err != "" {
		t.Fatal(res.Err)
	}
	n := len(r.got())
	time.Sleep(400 * time.Millisecond)
	if after := len(r.got()); after != n {
		t.Fatalf("%d screens after watch:, was %d", after, n)
	}
}

func TestWatchRefusedForCLI(t *testing.T) {
	s, _, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	c := connectAs(t, ctx, s, proto.ClientCLI)
	if res := act(t, c, "w", proto.Act{RecID: proto.WatchPrefix + "%1"}); res.Err != watchOnlyFromConsole {
		t.Fatalf("err %q", res.Err)
	}
}

// Only a console streams screens: a CLI peer starts no watch loop, so a
// watch it holds by any way captures nothing.
func TestNoWatchLoopForTheCLI(t *testing.T) {
	s, f, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	connectAs(t, ctx, s, proto.ClientCLI)
	pane, held := "%1", 0
	for deadline := time.Now().Add(waitLimit); held == 0 && time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		s.mu.Lock()
		for cl := range s.clients {
			cl.watch.Store(&watched{pane: pane})
			held++
		}
		s.mu.Unlock()
	}
	if held == 0 {
		t.Fatal("the CLI peer did not join")
	}
	s.kickScreens(pane)
	time.Sleep(400 * time.Millisecond)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls != 0 {
		t.Fatalf("a CLI peer captured the pane %d times", f.calls)
	}
}

func TestWatchRefusesAnUnknownPane(t *testing.T) {
	s, _, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	c := connectAs(t, ctx, s, proto.ClientConsole)
	r := read(c)
	if res := r.act(t, "w", proto.WatchPrefix+"%7"); res.Err != "no agent session in %7 any more" {
		t.Fatalf("err %q", res.Err)
	}
	time.Sleep(300 * time.Millisecond)
	if g := r.got(); len(g) != 0 {
		t.Fatalf("screens %+v", g)
	}
}

func TestWatchRefusedFromAnAgentPane(t *testing.T) {
	s, _, ctx := watchTest(t, map[string]string{"%1": idleScreen, "%2": idleScreen})
	c := connectAs(t, ctx, s, proto.ClientConsole)
	if err := c.Send(proto.KindAct, "w", proto.Act{RecID: proto.WatchPrefix + "%1", From: "%2"}); err != nil {
		t.Fatal(err)
	}
	var res proto.Result
	if err := json.Unmarshal(next(t, c, proto.KindResult).Body, &res); err != nil {
		t.Fatal(err)
	}
	if res.Err != watchOnlyFromConsole {
		t.Fatalf("err %q", res.Err)
	}
}

func TestWatchStopsWhenTheSessionLeaves(t *testing.T) {
	s, f, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	// A still screen goes once more when the idle session's wait settles.
	// Watch after that, so the screen after the first is the leave (#382).
	for deadline := time.Now().Add(waitLimit); !s.waitShown("%1", time.Now()); {
		if time.Now().After(deadline) {
			t.Fatal("the wait never settled")
		}
		time.Sleep(10 * time.Millisecond)
	}
	c := connectAs(t, ctx, s, proto.ClientConsole)
	r := read(c)
	if res := r.act(t, "w", proto.WatchPrefix+"%1"); res.Err != "" {
		t.Fatal(res.Err)
	}
	deadline := time.Now().Add(waitLimit)
	for len(r.got()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	f.leave()
	for len(r.got()) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if g := r.got(); len(g) != 2 || !g[1].Gone {
		t.Fatalf("screens %+v, want a screen then gone", g)
	}
	f.mu.Lock()
	calls := f.calls
	f.mu.Unlock()
	f.set("%1", "back\n")
	time.Sleep(400 * time.Millisecond)
	f.mu.Lock()
	after := f.calls
	f.mu.Unlock()
	if after != calls || len(r.got()) != 2 {
		t.Fatalf("capture went on: %d calls after %d, %d screens", after, calls, len(r.got()))
	}
}

// screenThat waits for a screen that ok takes.
func (r *reader) screenThat(t *testing.T, ok func(proto.Screen) bool) proto.Screen {
	t.Helper()
	for deadline := time.Now().Add(waitLimit); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		for _, sc := range r.got() {
			if ok(sc) {
				return sc
			}
		}
	}
	t.Fatalf("no such screen in %+v", r.got())
	return proto.Screen{}
}

// A pane with no history answers a watch up with its live screen. Each
// watch act still gets a screen back, so the console that scrolled up and
// back sees the live screen again (it blanks the chat until one comes).
func TestEveryWatchGetsAScreenBack(t *testing.T) {
	s, _, ctx := watchTest(t, map[string]string{"%1": "up 0\n"})
	c := connectAs(t, ctx, s, proto.ClientConsole)
	r := read(c)
	r.act(t, "w", proto.WatchPrefix+"%1")
	r.screenThat(t, func(proto.Screen) bool { return true })
	for i, up := range []string{"up:3", "up:0", "up:6", ""} {
		n := len(r.got())
		r.actText(t, fmt.Sprint("w", i), proto.WatchPrefix+"%1", up)
		deadline := time.Now().Add(waitLimit)
		for len(r.got()) == n && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if len(r.got()) == n {
			t.Fatalf("no screen came back for watch %q", up)
		}
	}
}

// A pane that tmux fails to read is not a session that ended: the screen
// says the read failed, never Gone, and a good read after it is sent again.
func TestACaptureErrorIsNotGone(t *testing.T) {
	s, f, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	c := connectAs(t, ctx, s, proto.ClientConsole)
	r := read(c)
	r.act(t, "w", proto.WatchPrefix+"%1")
	r.screenThat(t, func(sc proto.Screen) bool { return sc.Text == idleScreen })
	f.mu.Lock()
	delete(f.screens, "%1") // tmux fails: the capture drops the pane
	f.mu.Unlock()
	failed := r.screenThat(t, func(sc proto.Screen) bool { return sc.Err != "" || sc.Gone })
	if failed.Gone || failed.Err == "" {
		t.Fatalf("a failed read: %+v", failed)
	}
	f.set("%1", idleScreen)
	back := r.screenThat(t, func(sc proto.Screen) bool { return sc.At.After(failed.At) && sc.Err == "" })
	if back.Text != idleScreen || back.Gone {
		t.Fatalf("the read after it: %+v", back)
	}
}
