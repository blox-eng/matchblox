package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"slices"
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
	if err := r.c.Send(proto.KindAct, id, proto.Act{RecID: recID, Text: text}); err != nil {
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
	// history is how many lines each pane holds above its screen; ups are
	// the offsets asked of captureUp.
	history int
	ups     []int
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

// captureUp serves "up <n>" for a pane with a screen, n stopped at the
// history's top.
func (f *captureOf) captureUp(pane string, up int) (string, int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.ups = append(f.ups, up)
	if _, ok := f.screens[pane]; !ok {
		return "", 0, false
	}
	up = min(up, f.history)
	return fmt.Sprintf("up %d\n", up), up, true
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
	f := &captureOf{screens: screens, history: 1000}
	s.Capture, s.CaptureUp = f.capture, f.captureUp
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

func TestWatchPaneGone(t *testing.T) {
	s, _, ctx := watchTest(t, map[string]string{}, "%1")
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
	if res.Err != "keys go only from a console" {
		t.Fatalf("err %q", res.Err)
	}
}

func TestWatchStopsWhenTheSessionLeaves(t *testing.T) {
	s, f, ctx := watchTest(t, map[string]string{"%1": idleScreen})
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

func TestWatchUpSendsTheLinesUp(t *testing.T) {
	s, f, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	c := connectAs(t, ctx, s, proto.ClientConsole)
	r := read(c)
	if res := r.actText(t, "w", proto.WatchPrefix+"%1", "up:30"); res.Err != "" {
		t.Fatal(res.Err)
	}
	sc := r.screenThat(t, func(sc proto.Screen) bool { return sc.Up > 0 })
	if sc.Pane != "%1" || sc.Up != 30 || sc.Text != "up 30\n" {
		t.Fatalf("screen %+v", sc)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !slices.Contains(f.ups, 30) {
		t.Fatalf("offsets asked %v, want 30", f.ups)
	}
}

func TestWatchUpZeroIsLive(t *testing.T) {
	s, _, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	c := connectAs(t, ctx, s, proto.ClientConsole)
	r := read(c)
	r.actText(t, "w", proto.WatchPrefix+"%1", "up:6")
	r.screenThat(t, func(sc proto.Screen) bool { return sc.Up == 6 })
	if res := r.actText(t, "w2", proto.WatchPrefix+"%1", "up:0"); res.Err != "" {
		t.Fatal(res.Err)
	}
	if sc := r.screenThat(t, func(sc proto.Screen) bool { return sc.Text == idleScreen }); sc.Up != 0 {
		t.Fatalf("screen %+v", sc)
	}
}

func TestWatchUpStopsAtTheTopOfTheHistory(t *testing.T) {
	s, f, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	f.history = 10
	c := connectAs(t, ctx, s, proto.ClientConsole)
	r := read(c)
	r.actText(t, "w", proto.WatchPrefix+"%1", "up:30")
	if sc := r.screenThat(t, func(sc proto.Screen) bool { return sc.Up > 0 }); sc.Up != 10 || sc.Text != "up 10\n" {
		t.Fatalf("screen %+v, want up 10", sc)
	}
}

func TestWatchUpRefusesABadOffset(t *testing.T) {
	s, _, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	c := connectAs(t, ctx, s, proto.ClientConsole)
	r := read(c)
	for i, text := range []string{"up:2001", "up:-1", "up:x", "down:3"} {
		if res := r.actText(t, fmt.Sprint("w", i), proto.WatchPrefix+"%1", text); res.Err == "" {
			t.Fatalf("%q was taken", text)
		}
	}
}

// A screen of the history is not the one an Enter answers: an Enter seen
// on it is refused, and one seen on the live screen goes.
func TestEnterSeenOnTheHistoryIsRefused(t *testing.T) {
	s, _, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	c := connectAs(t, ctx, s, proto.ClientConsole)
	r := read(c)
	r.actText(t, "w", proto.WatchPrefix+"%1", "up:3")
	old := r.screenThat(t, func(sc proto.Screen) bool { return sc.Up > 0 })
	enter := func(seen time.Time) proto.Result {
		return s.act(ctx, proto.Act{Console: true, RecID: proto.KeysPrefix + "%1", Text: "Enter", PID: 100, Seen: seen})
	}
	if res := enter(old.At); res.Err == "" || len(res.Ran) != 0 {
		t.Fatalf("enter on the history: %+v", res)
	}
	r.actText(t, "w2", proto.WatchPrefix+"%1", "up:0")
	live := r.screenThat(t, func(sc proto.Screen) bool { return sc.Up == 0 && sc.Text == idleScreen })
	if res := enter(live.At); res.Err != "" || len(res.Ran) != 1 {
		t.Fatalf("enter on the live screen: %+v", res)
	}
}

// Every screen of the history since the last live one stays refused, however
// many came, and the live screen after them is not.
func TestEnterSeenOnAnyOfManyHistoryScreensIsRefused(t *testing.T) {
	s, _, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	c := connectAs(t, ctx, s, proto.ClientConsole)
	r := read(c)
	var ups []proto.Screen
	for n := 1; n <= 40; n++ {
		r.actText(t, fmt.Sprint("w", n), proto.WatchPrefix+"%1", fmt.Sprint("up:", n))
		ups = append(ups, r.screenThat(t, func(sc proto.Screen) bool { return sc.Up == n }))
	}
	enter := func(seen time.Time) proto.Result {
		return s.act(ctx, proto.Act{Console: true, RecID: proto.KeysPrefix + "%1", Text: "Enter", PID: 100, Seen: seen})
	}
	for _, sc := range []proto.Screen{ups[0], ups[20], ups[39]} {
		if res := enter(sc.At); res.Err == "" {
			t.Fatalf("enter on the history screen up %d went", sc.Up)
		}
	}
	r.actText(t, "live", proto.WatchPrefix+"%1", "up:0")
	live := r.screenThat(t, func(sc proto.Screen) bool { return sc.Up == 0 && sc.At.After(ups[39].At) })
	if res := enter(live.At); res.Err != "" || len(res.Ran) != 1 {
		t.Fatalf("enter on the live screen: %+v", res)
	}
}

// Stamps taken in turn by a console up the history and one live never put
// the live one inside the history's run.
func TestALiveStampIsNeverInAHistoryRun(t *testing.T) {
	var u upScreens
	at := time.Unix(1000, 0)
	still := func() time.Time { return at } // one clock reading for all: the stamps still rise
	var lives, ups []time.Time
	for range 5 {
		ups = append(ups, u.stamp("%1", still, true))
		lives = append(lives, u.stamp("%1", still, false))
	}
	ups = append(ups, u.stamp("%1", still, true), u.stamp("%1", still, true))
	for _, l := range lives {
		if u.has("%1", l) {
			t.Fatalf("live stamp %v refused", l)
		}
	}
	for _, h := range ups[len(ups)-2:] {
		if !u.has("%1", h) {
			t.Fatalf("history stamp %v not refused", h)
		}
	}
}

// Two consoles on one pane, one up the history: the live one's Enter goes.
func TestTheLiveConsoleEnterGoesWhileAnotherIsUp(t *testing.T) {
	s, f, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	ra := read(connectAs(t, ctx, s, proto.ClientConsole))
	rb := read(connectAs(t, ctx, s, proto.ClientConsole))
	rb.act(t, "w", proto.WatchPrefix+"%1")
	for n := 1; n <= 10; n++ {
		ra.actText(t, fmt.Sprint("a", n), proto.WatchPrefix+"%1", fmt.Sprint("up:", n))
		f.set("%1", fmt.Sprintf("live %d\n", n))
		rb.screenThat(t, func(sc proto.Screen) bool { return sc.Text == fmt.Sprintf("live %d\n", n) })
		ra.screenThat(t, func(sc proto.Screen) bool { return sc.Up == n })
	}
	g := rb.got()
	live := g[len(g)-1]
	if res := s.act(ctx, proto.Act{Console: true, RecID: proto.KeysPrefix + "%1", Text: "Enter", PID: 100, Seen: live.At}); res.Err != "" || len(res.Ran) != 1 {
		t.Fatalf("the live console's Enter: %+v", res)
	}
}
