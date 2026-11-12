package service

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/update"
)

type fakeUpdates struct {
	mu      sync.Mutex
	latest  string
	err     error
	calls   int
	managed string
	// Install: the tags it was asked for, what it answers, and, when
	// gate is set, it says it began on began and waits for gate.
	installs   []string
	proof      update.Proof
	installErr error
	began      chan struct{}
	gate       chan struct{}
}

func (f *fakeUpdates) Latest() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.latest, f.err
}

func (f *fakeUpdates) Install(tag string) (update.Proof, error) {
	f.mu.Lock()
	f.installs = append(f.installs, tag)
	began, gate := f.began, f.gate
	f.mu.Unlock()
	if gate != nil {
		began <- struct{}{}
		<-gate
	}
	return f.proof, f.installErr
}

func (f *fakeUpdates) Managed() string { return f.managed }

func (f *fakeUpdates) installed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.installs)
}

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
	for _, current := range []string{"dev", "v0.42.1-0.20261112093000-f15b80c1abcd"} {
		t.Run(current, func(t *testing.T) {
			s, src, clk := updateTest(t, current, config.UpdateAuto, filepath.Join(t.TempDir(), "update.json"))
			tick := startUpdates(t, s)
			tick()
			clk.add(25 * time.Hour)
			tick()
			if n := src.count(); n != 0 {
				t.Fatalf("Latest calls %d, want 0", n)
			}
			if v := updateView(s); v != nil {
				t.Fatalf("a build that is not a release sent a view %+v", v)
			}
		})
	}
}

func TestEmptyOrBadLatestIsAFailedLook(t *testing.T) {
	for _, bad := range []string{"", "nightly"} {
		path := filepath.Join(t.TempDir(), "update.json")
		s, src, clk := updateTest(t, "v0.42.0", config.UpdateOffer, path)
		s.loadUpdate()
		s.checkUpdate(clk.now())
		src.set(bad, nil)
		s.checkUpdate(clk.now().Add(24 * time.Hour))
		v := updateView(s)
		if v.Err == "" || v.Latest != "v0.43.0" || !v.FirstSeen.Equal(updateT0) {
			t.Errorf("latest %q: view %+v", bad, v)
		}
		if f := state.ReadUpdate(path); f.Latest != "v0.43.0" || !f.FirstSeen.Equal(updateT0) {
			t.Errorf("latest %q: update.json %+v", bad, f)
		}
	}
}

// offerTest is a service on v0.42.0 that found v0.43.0.
func offerTest(t *testing.T, mode string) (*Service, *fakeUpdates, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "update.json")
	s, src, clk := updateTest(t, "v0.42.0", mode, path)
	s.loadUpdate()
	if mode != config.UpdateOff {
		s.checkUpdate(clk.now())
	}
	return s, src, path
}

func updateDoorOf(s *Service) (doors.Door, bool) {
	for _, d := range s.doors() {
		if d.ID == doors.Update {
			return d, true
		}
	}
	return doors.Door{}, false
}

func updateAct(s *Service, which, confirm, text string) proto.Result {
	return s.act(context.Background(), proto.Act{RecID: "door:" + doors.Update, Which: which, Confirm: confirm, Text: text, Console: true})
}

func TestUpdateDoorShowsForANewerRelease(t *testing.T) {
	s, _, _ := offerTest(t, config.UpdateOffer)
	exe := filepath.Join(t.TempDir(), "matchblox")
	s.Exe = exe
	d, ok := updateDoorOf(s)
	if !ok {
		t.Fatalf("no update door in %+v", s.doors())
	}
	if d.Title != "matchblox v0.43.0 is out" || d.Why != "This machine runs v0.42.0." || d.Sum != "v0.43.0" || d.Problem != "" {
		t.Fatalf("door %+v", d)
	}
	for _, want := range []string{"matchblox v0.42.0 → v0.43.0 (", "checksum", "provenance", "\n  replaces\n    " + exe} {
		if !strings.Contains(d.Preview, want) {
			t.Errorf("preview lacks %q:\n%s", want, d.Preview)
		}
	}
	if st := s.State(); !slices.ContainsFunc(st.Doors, func(d doors.Door) bool { return d.ID == doors.Update }) {
		t.Fatalf("the update view changed but the state has no update door: %+v", st.Doors)
	}
}

func TestUpdateDoorAbsentWhenUpToDate(t *testing.T) {
	s, src, clk := updateTest(t, "v0.42.0", config.UpdateOffer, filepath.Join(t.TempDir(), "update.json"))
	src.set("v0.42.0", nil)
	s.loadUpdate()
	s.checkUpdate(clk.now())
	if d, ok := updateDoorOf(s); ok {
		t.Fatalf("door on the latest release: %+v", d)
	}
}

func TestUpdateDoorAbsentWhenOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	if err := state.WriteUpdate(path, state.UpdateFile{Latest: "v0.43.0", FirstSeen: updateT0}); err != nil {
		t.Fatal(err)
	}
	s, _, _ := updateTest(t, "v0.42.0", config.UpdateOff, path)
	s.loadUpdate()
	if d, ok := updateDoorOf(s); ok {
		t.Fatalf("door under mode off: %+v", d)
	}
}

func TestUpdateDoorClosedForThisReleaseOnly(t *testing.T) {
	s, src, path := offerTest(t, config.UpdateOffer)
	if r := updateAct(s, "secondary", "y", "v0.43.0"); r.Err != "" {
		t.Fatalf("close: %+v", r)
	}
	if d, ok := updateDoorOf(s); ok {
		t.Fatalf("door after a close: %+v", d)
	}
	if f := state.ReadUpdate(path); f.Closed != "v0.43.0" {
		t.Fatalf("update.json %+v", f)
	}
	again, _, _ := updateTest(t, "v0.42.0", config.UpdateOffer, path)
	again.loadUpdate()
	if d, ok := updateDoorOf(again); ok {
		t.Fatalf("door after a close and a restart: %+v", d)
	}
	src.set("v0.44.0", nil)
	s.checkUpdate(updateT0.Add(24 * time.Hour))
	if d, ok := updateDoorOf(s); !ok || d.Sum != "v0.44.0" {
		t.Fatalf("no door for the next release: %+v %v", d, ok)
	}
	// Within the act's dedupe window: a close of another release is a new close.
	if r := updateAct(s, "secondary", "y", "v0.44.0"); r.Err != "" || len(r.Skipped) > 0 {
		t.Fatalf("close of the next release: %+v", r)
	}
	if f := state.ReadUpdate(path); f.Closed != "v0.44.0" {
		t.Fatalf("update.json after the second close %+v", f)
	}
}

func TestUpdateCloseRefusesAStaleTag(t *testing.T) {
	s, src, path := offerTest(t, config.UpdateOffer)
	src.set("v0.44.0", nil)
	s.checkUpdate(updateT0.Add(24 * time.Hour))
	if r := updateAct(s, "secondary", "y", "v0.43.0"); r.Err != "a newer release came out: look again" {
		t.Fatalf("result %+v", r)
	}
	if d, ok := updateDoorOf(s); !ok || d.Sum != "v0.44.0" {
		t.Fatalf("a stale close closed the newer release: %+v %v", d, ok)
	}
	if f := state.ReadUpdate(path); f.Closed != "" {
		t.Fatalf("update.json %+v", f)
	}
}

func TestUpdateActNeedsATypedY(t *testing.T) {
	s, src, _ := offerTest(t, config.UpdateOffer)
	for _, which := range []string{"primary", "secondary"} {
		if r := updateAct(s, which, "", "v0.43.0"); r.Err == "" {
			t.Fatalf("%s without a typed y: %+v", which, r)
		}
	}
	if n := len(src.installed()); n != 0 {
		t.Fatalf("installs without a typed y: %d", n)
	}
	if _, ok := updateDoorOf(s); !ok {
		t.Fatal("a close without a typed y closed the door")
	}
}

func TestUpdateActInstallsTheTagTheDoorShowed(t *testing.T) {
	s, src, path := offerTest(t, config.UpdateOffer)
	src.proof = update.Proof{Checksum: true, Provenance: true}
	d, _ := updateDoorOf(s)
	r := updateAct(s, "primary", "y", d.Sum)
	if r.Err != "" || len(r.Ran) != 1 || !slices.Equal(r.Ran[0], []string{"install", "v0.43.0"}) {
		t.Fatalf("result %+v", r)
	}
	if got := src.installed(); !slices.Equal(got, []string{"v0.43.0"}) {
		t.Fatalf("installed %v", got)
	}
	want := state.UpdateRecord{From: "v0.42.0", To: "v0.43.0", At: updateT0, Checksum: true, Provenance: true}
	if v := updateView(s); v.Last == nil || *v.Last != want {
		t.Fatalf("view last %+v", v.Last)
	}
	if f := state.ReadUpdate(path); f.Last == nil || *f.Last != want {
		t.Fatalf("update.json last %+v", f.Last)
	}
	if d, ok := updateDoorOf(s); ok {
		t.Fatalf("the door lingers after a good install: %+v", d)
	}
}

func TestASkippedUpdateActRunsAgainAtOnce(t *testing.T) {
	s, src, _ := offerTest(t, config.UpdateOffer)
	gate := make(chan struct{})
	src.began, src.gate = make(chan struct{}), gate
	first := make(chan proto.Result)
	go func() { first <- s.installUpdate("v0.43.0") }()
	<-src.began
	if r := updateAct(s, "primary", "y", "v0.43.0"); !slices.Equal(r.Skipped, []string{"already updating"}) {
		t.Fatalf("act while an install runs: %+v", r)
	}
	src.mu.Lock()
	src.gate, src.installErr = nil, errors.New("could not download matchblox-linux-amd64 v0.43.0: GitHub answered 502. Nothing was changed")
	src.mu.Unlock()
	close(gate)
	<-first
	r := updateAct(s, "primary", "y", "v0.43.0")
	if len(r.Skipped) > 0 || len(src.installed()) != 2 {
		t.Fatalf("the retry did not run: %+v, installs %v", r, src.installed())
	}
}

func TestUpdateActRefusesAStaleTag(t *testing.T) {
	s, src, _ := offerTest(t, config.UpdateOffer)
	src.set("v0.44.0", nil)
	s.checkUpdate(updateT0.Add(24 * time.Hour))
	r := updateAct(s, "primary", "y", "v0.43.0")
	if r.Err != "a newer release came out: look again" {
		t.Fatalf("result %+v", r)
	}
	if n := len(src.installed()); n != 0 {
		t.Fatalf("installs of a stale tag: %d", n)
	}
}

func TestUpdateInstallFailureKeepsTheOffer(t *testing.T) {
	s, src, path := offerTest(t, config.UpdateOffer)
	var stopped error
	s.mu.Lock()
	s.stop = func(err error) { stopped = err }
	s.mu.Unlock()
	src.installErr = errors.New("checksum mismatch for matchblox-linux-amd64. Nothing was changed")
	r := updateAct(s, "primary", "y", "v0.43.0")
	if !strings.Contains(r.Err, "checksum mismatch") {
		t.Fatalf("result %+v", r)
	}
	if _, ok := updateDoorOf(s); !ok {
		t.Fatal("the door went after a failed install")
	}
	if v := updateView(s); v.Last == nil || !strings.Contains(v.Last.Err, "checksum mismatch") || v.Last.To != "v0.43.0" {
		t.Fatalf("view last %+v", v.Last)
	}
	if f := state.ReadUpdate(path); f.Last == nil || f.Last.Err == "" {
		t.Fatalf("update.json last %+v", f.Last)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if stopped != nil {
		t.Fatalf("a failed install stopped the service: %v", stopped)
	}
}

func TestTwoUpdateActsRunOneInstall(t *testing.T) {
	s, src, _ := offerTest(t, config.UpdateOffer)
	src.began, src.gate = make(chan struct{}), make(chan struct{})
	first := make(chan proto.Result)
	go func() { first <- s.installUpdate("v0.43.0") }()
	<-src.began
	if r := s.installUpdate("v0.43.0"); !slices.Equal(r.Skipped, []string{"already updating"}) {
		t.Fatalf("second while the first runs: %+v", r)
	}
	close(src.gate)
	if r := <-first; r.Err != "" {
		t.Fatalf("first %+v", r)
	}
	if n := len(src.installed()); n != 1 {
		t.Fatalf("Install calls %d, want 1", n)
	}
}

func TestManagedInstallOffersTheManagersCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	s, src, clk := updateTest(t, "v0.42.0", config.UpdateOffer, path)
	src.managed = "brew upgrade matchblox"
	s.loadUpdate()
	s.checkUpdate(clk.now())
	d, ok := updateDoorOf(s)
	if !ok {
		t.Fatal("no door for a managed install")
	}
	if d.Preview != "brew upgrade matchblox" || d.Problem != "Homebrew installed matchblox. Update it with Homebrew." {
		t.Fatalf("door %+v", d)
	}
	if r := updateAct(s, "primary", "y", d.Sum); r.Err == "" {
		t.Fatalf("a managed install took a primary: %+v", r)
	}
	if r := s.installUpdate("v0.43.0"); r.Err == "" {
		t.Fatalf("installUpdate replaced a managed install: %+v", r)
	}
	if n := len(src.installed()); n != 0 {
		t.Fatalf("Install calls %d, want 0", n)
	}
}
