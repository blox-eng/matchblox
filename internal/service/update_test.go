package service

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/update"
)

type fakeUpdates struct {
	mu      sync.Mutex
	latest  string
	err     error
	calls   int
	managed string
}

func (f *fakeUpdates) Latest() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.latest, f.err
}

func (f *fakeUpdates) Install(string) (update.Proof, error) { return update.Proof{}, nil }
func (f *fakeUpdates) Managed() string                      { return f.managed }

func (f *fakeUpdates) set(latest string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.latest, f.err = latest, err
}

func (f *fakeUpdates) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *fakeClock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

var updateT0 = time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)

func updateTest(t *testing.T, current, mode, path string) (*Service, *fakeUpdates, *fakeClock) {
	t.Helper()
	s := newTest(t, time.Hour)
	src := &fakeUpdates{latest: "v0.43.0"}
	clk := &fakeClock{at: updateT0}
	s.Binary, s.UpdateMode, s.UpdatePath = current, mode, path
	s.Update, s.Now = src, clk.now
	return s, src, clk
}

// startUpdates runs the update loop on ticks the test sends; tick returns
// once the loop took it, so the tick before it is fully handled.
func startUpdates(t *testing.T, s *Service) (tick func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)
	s.updateTicks = ticks
	s.loadUpdate()
	done := make(chan struct{})
	go func() { s.updateLoop(ctx); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(waitLimit):
			t.Errorf("the update loop did not stop in %v", waitLimit)
		}
	})
	return func() {
		for range 2 {
			select {
			case ticks <- time.Time{}:
			case <-done:
				return
			}
		}
	}
}

func updateView(s *Service) *state.UpdateView { return s.State().Update }

func TestUpdateOffMakesNoRequest(t *testing.T) {
	s, src, clk := updateTest(t, "v0.42.0", config.UpdateOff, filepath.Join(t.TempDir(), "update.json"))
	tick := startUpdates(t, s)
	for range 3 {
		tick()
		clk.add(25 * time.Hour)
	}
	tick()
	if n := src.count(); n != 0 {
		t.Fatalf("Latest calls %d, want 0", n)
	}
	if v := updateView(s); v == nil || *v != (state.UpdateView{Mode: "off", Current: "v0.42.0"}) {
		t.Fatalf("view %+v", v)
	}
}

func TestUpdateCheckAtStartAndDaily(t *testing.T) {
	s, src, clk := updateTest(t, "v0.42.0", config.UpdateOffer, filepath.Join(t.TempDir(), "update.json"))
	tick := startUpdates(t, s)
	tick()
	if n := src.count(); n != 1 {
		t.Fatalf("calls at start %d, want 1", n)
	}
	clk.add(23 * time.Hour)
	tick()
	if n := src.count(); n != 1 {
		t.Fatalf("calls after 23h %d, want 1", n)
	}
	clk.add(time.Hour)
	tick()
	if n := src.count(); n != 2 {
		t.Fatalf("calls after 24h %d, want 2", n)
	}
	if v := updateView(s); v.Mode != "offer" || !v.CheckedAt.Equal(updateT0.Add(24*time.Hour)) {
		t.Fatalf("view %+v", v)
	}
}

func TestUpdateViewNamesANewerRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	s, src, clk := updateTest(t, "v0.42.0", config.UpdateOffer, path)
	s.loadUpdate()
	s.checkUpdate(clk.now())
	v := updateView(s)
	if v.Latest != "v0.43.0" || !v.FirstSeen.Equal(updateT0) || v.Current != "v0.42.0" {
		t.Fatalf("first view %+v", v)
	}
	if tag, ok := s.newer(); !ok || tag != "v0.43.0" {
		t.Fatalf("newer %q %v", tag, ok)
	}
	clk.add(24 * time.Hour)
	s.checkUpdate(clk.now())
	if v := updateView(s); !v.FirstSeen.Equal(updateT0) {
		t.Fatalf("first seen moved on a second check: %v", v.FirstSeen)
	}

	again, _, _ := updateTest(t, "v0.42.0", config.UpdateOffer, path)
	again.loadUpdate()
	if v := updateView(again); v.Latest != "v0.43.0" || !v.FirstSeen.Equal(updateT0) {
		t.Fatalf("view after a restart, before a check %+v", v)
	}
	again.checkUpdate(updateT0.Add(48 * time.Hour))
	if v := updateView(again); !v.FirstSeen.Equal(updateT0) {
		t.Fatalf("first seen moved after a restart: %v", v.FirstSeen)
	}

	src.set("v0.44.0", nil)
	s.checkUpdate(updateT0.Add(72 * time.Hour))
	if v := updateView(s); v.Latest != "v0.44.0" || !v.FirstSeen.Equal(updateT0.Add(72*time.Hour)) {
		t.Fatalf("a new latest keeps the old first seen: %+v", v)
	}
	if f := state.ReadUpdate(path); f.Latest != "v0.44.0" || !f.FirstSeen.Equal(updateT0.Add(72*time.Hour)) {
		t.Fatalf("update.json %+v", f)
	}
}

func TestUpdateCheckFailureIsSaid(t *testing.T) {
	s, src, clk := updateTest(t, "v0.42.0", config.UpdateOffer, filepath.Join(t.TempDir(), "update.json"))
	tick := startUpdates(t, s)
	tick()
	src.set("", errors.New("could not reach GitHub to find the latest release: no route to host"))
	clk.add(24 * time.Hour)
	tick()
	v := updateView(s)
	if v.Err != "could not reach GitHub to find the latest release: no route to host" {
		t.Fatalf("err %q", v.Err)
	}
	if v.Latest != "v0.43.0" || !v.CheckedAt.Equal(updateT0.Add(24*time.Hour)) {
		t.Fatalf("view after a failure %+v", v)
	}
	clk.add(23 * time.Hour)
	tick()
	if n := src.count(); n != 2 {
		t.Fatalf("calls before a period after the failure %d, want 2", n)
	}
	src.set("v0.43.0", nil)
	clk.add(time.Hour)
	tick()
	if n := src.count(); n != 3 {
		t.Fatalf("calls a period after the failure %d, want 3", n)
	}
	if v := updateView(s); v.Err != "" {
		t.Fatalf("err kept after a good check: %q", v.Err)
	}
}

func TestNoOfferForSameOlderOrPrerelease(t *testing.T) {
	for _, latest := range []string{"v0.42.0", "v0.41.0", "v0.43.0-rc.1"} {
		s, src, clk := updateTest(t, "v0.42.0", config.UpdateOffer, filepath.Join(t.TempDir(), "update.json"))
		src.set(latest, nil)
		s.loadUpdate()
		s.checkUpdate(clk.now())
		if tag, ok := s.newer(); ok {
			t.Errorf("latest %s: newer %q", latest, tag)
		}
	}
}

func TestDevBuildMakesNoRequest(t *testing.T) {
	s, src, clk := updateTest(t, "dev", config.UpdateOffer, filepath.Join(t.TempDir(), "update.json"))
	tick := startUpdates(t, s)
	tick()
	clk.add(25 * time.Hour)
	tick()
	if n := src.count(); n != 0 {
		t.Fatalf("Latest calls %d, want 0", n)
	}
	if v := updateView(s); v != nil {
		t.Fatalf("a dev build sent a view %+v", v)
	}
}
