package service

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
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
	if err := r.c.Send(proto.KindAct, id, proto.Act{RecID: recID}); err != nil {
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

func decodeScreen(t *testing.T, env proto.Envelope) proto.Screen {
	t.Helper()
	var sc proto.Screen
	if err := json.Unmarshal(env.Body, &sc); err != nil {
		t.Fatal(err)
	}
	return sc
}

// captureOf serves the text of each pane in screens; set changes it.
type captureOf struct {
	mu      sync.Mutex
	screens map[string]string
	calls   int
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

func watchTest(t *testing.T, screens map[string]string) (*Service, *captureOf, context.Context) {
	t.Helper()
	s := newTest(t, time.Hour)
	f := &captureOf{screens: screens}
	s.Capture = f.capture
	return s, f, run(t, s)
}

func TestWatchSendsTheScreen(t *testing.T) {
	s, _, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	c := connectAs(t, ctx, s, proto.ClientConsole)
	if res := act(t, c, "w", proto.Act{RecID: proto.WatchPrefix + "%1"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	sc := decodeScreen(t, next(t, c, proto.KindScreen))
	if sc.Pane != "%1" || sc.Text != idleScreen || sc.At.IsZero() || sc.Gone {
		t.Fatalf("screen %+v", sc)
	}
}

func TestWatchSendsOnlyChanges(t *testing.T) {
	s, f, ctx := watchTest(t, map[string]string{"%1": idleScreen})
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

func TestWatchPaneGone(t *testing.T) {
	s, _, ctx := watchTest(t, map[string]string{})
	c := connectAs(t, ctx, s, proto.ClientConsole)
	r := read(c)
	if res := r.act(t, "w", proto.WatchPrefix+"%1"); res.Err != "" {
		t.Fatal(res.Err)
	}
	time.Sleep(500 * time.Millisecond)
	if g := r.got(); len(g) != 1 || g[0].Pane != "%1" || !g[0].Gone {
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
	if res := act(t, c, "w", proto.Act{RecID: proto.WatchPrefix + "%1"}); res.Err != "keys go only from a console" {
		t.Fatalf("err %q", res.Err)
	}
}
