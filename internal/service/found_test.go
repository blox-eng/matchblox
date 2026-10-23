package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// finds records each scan's roots.
type finds struct {
	mu    sync.Mutex
	roots [][]string
}

func (f *finds) calls() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.roots)
}

func foundTest(t *testing.T) (*Service, *finds) {
	t.Helper()
	s := newTest(t, time.Hour)
	dir := t.TempDir()
	s.ConsentPath, s.ReposPath = filepath.Join(dir, "scan.json"), filepath.Join(dir, "repos.json")
	s.ScanRoots = func(picked []string) []string {
		if len(picked) == 0 {
			return []string{"/default"}
		}
		return picked
	}
	f := &finds{}
	s.Find = func(_ context.Context, roots []string, _, _ map[string]bool) gitscan.Result {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.roots = append(f.roots, roots)
		return gitscan.Result{Repos: []gitscan.Checkout{{Path: "/h/a", Branch: "main"}}}
	}
	return s, f
}

// foundReady loads the consent, takes a first sample and starts the
// scan loop, which ends with the test.
func foundReady(t *testing.T, s *Service) {
	t.Helper()
	s.loadFound()
	s.publish(sample.Snapshot{At: time.Now()}, func(state.Doc) {})
	scanLoop(t, s, context.Background())
}

// scanLoop runs foundLoop under parent until the test ends, or until
// stop, which waits for the loop to end.
func scanLoop(t *testing.T, s *Service, parent context.Context) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() { s.foundLoop(ctx); close(done) }()
	stop = func() {
		cancel()
		within(t, done, "end of the scan loop")
	}
	t.Cleanup(stop)
	return stop
}

func firstFound(t *testing.T, s *Service) *proto.Found {
	t.Helper()
	deadline := time.Now().Add(waitLimit)
	for time.Now().Before(deadline) {
		if st, ok := s.stateFor(&client{}); ok {
			return st.Found
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no snapshot")
	return nil
}

func TestNoScanWithoutYes(t *testing.T) {
	s, f := foundTest(t)
	run(t, s)
	got := firstFound(t, s)
	if got == nil || got.Consent != "" || len(got.Repos) != 0 {
		t.Fatalf("first Found = %+v, want never asked", got)
	}
	time.Sleep(50 * time.Millisecond)
	if n := len(f.calls()); n != 0 {
		t.Fatalf("scanned %d times with no yes on record", n)
	}
}

func TestNoScanAfterNo(t *testing.T) {
	s, f := foundTest(t)
	state.WriteConsent(s.ConsentPath, state.Consent{Answer: "no"})
	run(t, s)
	firstFound(t, s)
	time.Sleep(50 * time.Millisecond)
	if n := len(f.calls()); n != 0 {
		t.Fatalf("scanned %d times after a no", n)
	}
}

func TestScanYesScansAndRecords(t *testing.T) {
	s, f := foundTest(t)
	foundReady(t, s)
	if res := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true}); res.Err != "" {
		t.Fatalf("scan:yes: %s", res.Err)
	}
	if got := f.calls(); len(got) != 1 || !slices.Equal(got[0], []string{"/default"}) {
		t.Fatalf("scans = %v, want one from the defaults", got)
	}
	if c := state.ReadConsent(s.ConsentPath); c.Answer != "yes" {
		t.Fatalf("consent = %+v", c)
	}
	if r := state.ReadFound(s.ReposPath); len(r.Repos) != 1 || r.Repos[0].Path != "/h/a" {
		t.Fatalf("cache = %+v", r)
	}
	if st, _ := s.stateFor(&client{}); st.Found == nil || st.Found.Consent != "yes" || len(st.Found.Repos) != 1 {
		t.Fatalf("Found = %+v", st.Found)
	}
}

func TestScanYesKeepsPickedRoots(t *testing.T) {
	s, f := foundTest(t)
	state.WriteConsent(s.ConsentPath, state.Consent{Answer: "yes", Roots: []string{"/w"}})
	foundReady(t, s)
	s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true})
	s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true, Text: "/a\n\n/b\n"})
	got := f.calls()
	if len(got) != 2 || !slices.Equal(got[0], []string{"/w"}) || !slices.Equal(got[1], []string{"/a", "/b"}) {
		t.Fatalf("scans = %v, want [/w] then [/a /b]", got)
	}
	if c := state.ReadConsent(s.ConsentPath); !slices.Equal(c.Roots, []string{"/a", "/b"}) {
		t.Fatalf("recorded roots = %v", c.Roots)
	}
}

func TestScanNoDropsCache(t *testing.T) {
	s, _ := foundTest(t)
	foundReady(t, s)
	s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true})
	if res := s.act(t.Context(), proto.Act{RecID: "scan:no"}); res.Err != "" {
		t.Fatalf("scan:no: %s", res.Err)
	}
	if _, err := os.Stat(s.ReposPath); !os.IsNotExist(err) {
		t.Fatalf("repos.json is still there: %v", err)
	}
	if c := state.ReadConsent(s.ConsentPath); c.Answer != "no" {
		t.Fatalf("consent = %+v", c)
	}
	if st, _ := s.stateFor(&client{}); st.Found == nil || st.Found.Consent != "no" || len(st.Found.Repos) != 0 {
		t.Fatalf("Found = %+v", st.Found)
	}
}

func TestScanYesRefusedFromAgentPane(t *testing.T) {
	s, f := foundTest(t)
	s.loadFound()
	s.publish(sample.Snapshot{At: time.Now(), Sessions: []sample.Session{{Agent: "claude", Pane: "%3", Name: "other", Status: "busy"}}}, func(state.Doc) {})
	scanLoop(t, s, context.Background())
	if res := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", From: "%3"}); res.Err != scanRefused {
		t.Fatalf("scan:yes from an agent's pane: %q", res.Err)
	}
	if n := len(f.calls()); n != 0 {
		t.Fatalf("scanned %d times", n)
	}
	if c := state.ReadConsent(s.ConsentPath); c.Answer != "" {
		t.Fatalf("consent = %+v, want none", c)
	}
	if res := s.act(t.Context(), proto.Act{RecID: "scan:no", From: "%3"}); res.Err != "" {
		t.Fatalf("scan:no from an agent's pane: %q", res.Err)
	}
	if res := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y"}); res.Err != "" {
		t.Fatalf("scan:yes from the builder's shell: %q", res.Err)
	}
}

func TestScanYesRefusedFromAgentPaneClaimingConsole(t *testing.T) {
	s, f := foundTest(t)
	s.loadFound()
	s.publish(sample.Snapshot{At: time.Now(), Sessions: []sample.Session{{Agent: "claude", Pane: "%3", Name: "other", Status: "busy"}}}, func(state.Doc) {})
	scanLoop(t, s, context.Background())
	if res := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", From: "%3", Console: true}); res.Err != scanRefused {
		t.Fatalf("scan:yes from a console hello in an agent's pane: %q", res.Err)
	}
	if n := len(f.calls()); n != 0 {
		t.Fatalf("scanned %d times", n)
	}
}

// blockingFind holds each walk until release, records its roots in f,
// and finds roots[0]/r.
func blockingFind(s *Service, f *finds) (started, release chan struct{}) {
	started, release = make(chan struct{}, 8), make(chan struct{})
	s.Find = func(_ context.Context, roots []string, _, _ map[string]bool) gitscan.Result {
		f.mu.Lock()
		f.roots = append(f.roots, roots)
		f.mu.Unlock()
		started <- struct{}{}
		<-release
		return gitscan.Result{Repos: []gitscan.Checkout{{Path: roots[0] + "/r"}}}
	}
	return started, release
}

// yesAsync sends a yes with text and hands back its result.
func yesAsync(ctx context.Context, s *Service, text string) <-chan proto.Result {
	res := make(chan proto.Result, 1)
	go func() { res <- s.act(ctx, proto.Act{RecID: "scan:yes", Confirm: "y", Console: true, Text: text}) }()
	return res
}

func waitRoots(t *testing.T, s *Service, roots []string) {
	t.Helper()
	deadline := time.Now().Add(waitLimit)
	for !slices.Equal(state.ReadConsent(s.ConsentPath).Roots, roots) {
		if time.Now().After(deadline) {
			t.Fatalf("the roots %v were never recorded", roots)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestScanYesReportsNoMidWalk(t *testing.T) {
	s, f := foundTest(t)
	started, release := blockingFind(s, f)
	foundReady(t, s)
	res := yesAsync(t.Context(), s, "")
	within(t, started, "walk")
	if r := s.act(t.Context(), proto.Act{RecID: "scan:no"}); r.Err != "" {
		t.Fatal(r.Err)
	}
	close(release)
	if r := within(t, res, "yes result"); r.Err != "a no came while it ran: nothing is kept" || r.Scan != nil {
		t.Fatalf("yes after a no mid-walk = %+v", r)
	}
	if _, err := os.Stat(s.ReposPath); !os.IsNotExist(err) {
		t.Fatalf("repos.json written after a no: %v", err)
	}
	if st, _ := s.stateFor(&client{}); st.Found == nil || st.Found.Consent != "no" || len(st.Found.Repos) != 0 {
		t.Fatalf("Found = %+v", st.Found)
	}
}

func TestScanYesSuperseded(t *testing.T) {
	s, f := foundTest(t)
	started, release := blockingFind(s, f)
	foundReady(t, s)
	first := yesAsync(t.Context(), s, "/a")
	within(t, started, "walk of /a")
	second := yesAsync(t.Context(), s, "/b\n/c")
	waitRoots(t, s, []string{"/b", "/c"})
	close(release)
	if r := within(t, first, "first result"); r.Err != "other folders were picked since: /b /c" {
		t.Fatalf("first yes = %+v", r)
	}
	r := within(t, second, "second result")
	if r.Err != "" || r.Scan == nil || r.Scan.Repos != 1 {
		t.Fatalf("second yes = %+v", r)
	}
	if got := f.calls(); len(got) != 2 {
		t.Fatalf("scans = %v, want two", got)
	}
	if c := state.ReadFound(s.ReposPath); len(c.Repos) != 1 || c.Repos[0].Path != "/b/r" {
		t.Fatalf("cache = %+v, want only the walk of /b /c", c)
	}
}

func TestScanRequestsCoalesce(t *testing.T) {
	s, f := foundTest(t)
	state.WriteConsent(s.ConsentPath, state.Consent{Answer: "yes"})
	started, release := blockingFind(s, f)
	foundReady(t, s)
	s.kickFound()
	within(t, started, "the kicked walk")
	var res []<-chan proto.Result
	for range 3 {
		res = append(res, yesAsync(t.Context(), s, ""))
	}
	deadline := time.Now().Add(waitLimit)
	for len(s.scanReqs) < 3 {
		if time.Now().After(deadline) {
			t.Fatal("the three yeses never queued")
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	for i, ch := range res {
		if r := within(t, ch, "result"); r.Err != "" || r.Scan == nil {
			t.Fatalf("yes %d = %+v", i, r)
		}
	}
	if n := len(f.calls()); n > 2 {
		t.Fatalf("%d walks for one running and three queued yeses, want at most 2", n)
	}
}

func TestScanStopped(t *testing.T) {
	s, f := foundTest(t)
	s.loadFound()
	started := make(chan struct{}, 1)
	s.Find = func(ctx context.Context, roots []string, _, _ map[string]bool) gitscan.Result {
		f.mu.Lock()
		f.roots = append(f.roots, roots)
		f.mu.Unlock()
		started <- struct{}{}
		<-ctx.Done()
		return gitscan.Result{Repos: []gitscan.Checkout{{Path: "/cut"}}}
	}
	stop := scanLoop(t, s, context.Background())
	res := yesAsync(t.Context(), s, "")
	within(t, started, "walk")
	stop()
	if r := within(t, res, "yes result"); r.Err != "the service stopped before the scan ended" {
		t.Fatalf("yes cut by the service stopping = %+v", r)
	}
	if c := state.ReadFound(s.ReposPath); len(c.Repos) != 0 {
		t.Fatalf("a cut walk was kept: %+v", c)
	}
	// The loop is gone: a later yes is told so, not left waiting.
	if r := within(t, yesAsync(t.Context(), s, ""), "yes after the stop"); r.Err != "the service stopped before the scan ended" {
		t.Fatalf("yes after the loop ended = %+v", r)
	}
}

func TestHungFolderRemembered(t *testing.T) {
	s, _ := foundTest(t)
	var mu sync.Mutex
	var skips []map[string]bool
	s.Find = func(_ context.Context, _ []string, skip, _ map[string]bool) gitscan.Result {
		mu.Lock()
		defer mu.Unlock()
		skips = append(skips, skip)
		if len(skips) == 1 {
			return gitscan.Result{Hung: []string{"/x", "/y"}}
		}
		return gitscan.Result{}
	}
	s.hung = map[string]time.Time{"/old": time.Now().Add(-7 * time.Hour)}
	foundReady(t, s)
	for range 2 {
		if r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true}); r.Err != "" {
			t.Fatal(r.Err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(skips) != 2 {
		t.Fatalf("%d walks, want 2", len(skips))
	}
	if skips[0]["/old"] {
		t.Fatal("a folder that hung more than 6 hours ago is still skipped")
	}
	if !skips[1]["/x"] || !skips[1]["/y"] {
		t.Fatalf("second walk's skip = %v, want /x and /y", skips[1])
	}
}

func TestScanWaiterGoneDoesNotBlock(t *testing.T) {
	s, f := foundTest(t)
	started, release := blockingFind(s, f)
	foundReady(t, s)
	ctx, cancel := context.WithCancel(t.Context())
	gone := yesAsync(ctx, s, "")
	within(t, started, "walk")
	cancel()
	within(t, gone, "the gone waiter's result")
	close(release)
	if r := within(t, yesAsync(t.Context(), s, ""), "next yes"); r.Err != "" {
		t.Fatalf("next yes = %+v", r)
	}
}

func TestNarrowedRootsDuringScanWin(t *testing.T) {
	s, f := foundTest(t)
	state.WriteConsent(s.ConsentPath, state.Consent{Answer: "yes", Roots: []string{"/wide"}})
	started, release := blockingFind(s, f)
	foundReady(t, s)
	s.kickFound()
	within(t, started, "the wide walk")
	second := yesAsync(t.Context(), s, "/narrow")
	waitRoots(t, s, []string{"/narrow"})
	release <- struct{}{}
	within(t, started, "the narrow walk")
	if r := state.ReadFound(s.ReposPath); len(r.Repos) != 0 {
		t.Fatalf("the wide walk was kept after the roots narrowed: %+v", r)
	}
	if st, _ := s.stateFor(&client{}); st.Found != nil && len(st.Found.Repos) != 0 {
		t.Fatalf("the wide walk went to consoles: %+v", st.Found)
	}
	release <- struct{}{}
	if res := within(t, second, "narrow result"); res.Err != "" {
		t.Fatal(res.Err)
	}
	r := state.ReadFound(s.ReposPath)
	if len(r.Repos) != 1 || r.Repos[0].Path != "/narrow/r" || !slices.Equal(r.Roots, []string{"/narrow"}) {
		t.Fatalf("cache = %+v, want only the narrowed walk", r)
	}
}

func TestCacheSentBeforeFirstScan(t *testing.T) {
	s, _ := foundTest(t)
	state.WriteConsent(s.ConsentPath, state.Consent{Answer: "yes"})
	state.WriteFound(s.ReposPath, gitscan.Found{Consent: "yes", Repos: []gitscan.Checkout{{Path: "/h/cached"}}})
	s.Find = func(ctx context.Context, _ []string, _, _ map[string]bool) gitscan.Result {
		<-ctx.Done()
		return gitscan.Result{}
	}
	run(t, s)
	if got := firstFound(t, s); got == nil || len(got.Repos) != 1 || got.Repos[0].Path != "/h/cached" {
		t.Fatalf("first Found = %+v, want the cache", got)
	}
}

func TestCacheKeepsSkipped(t *testing.T) {
	s, _ := foundTest(t)
	state.WriteConsent(s.ConsentPath, state.Consent{Answer: "yes"})
	skipped := []gitscan.Skip{{Path: "/h/Desktop", Why: "macOS asks first"}}
	state.WriteFound(s.ReposPath, gitscan.Found{Consent: "yes", Repos: []gitscan.Checkout{{Path: "/h/cached"}}, Skipped: skipped, SkippedMore: 3})
	s.loadFound()
	s.mu.Lock()
	f := s.found
	s.mu.Unlock()
	if !slices.Equal(f.Skipped, skipped) || f.SkippedMore != 3 {
		t.Fatalf("after a restart Found = %+v, want the cached skipped list", f)
	}
}

func TestCancelledScanKeepsCache(t *testing.T) {
	s, _ := foundTest(t)
	state.WriteConsent(s.ConsentPath, state.Consent{Answer: "yes"})
	state.WriteFound(s.ReposPath, gitscan.Found{Consent: "yes", Repos: []gitscan.Checkout{{Path: "/h/cached"}}})
	s.loadFound()
	started := make(chan struct{}, 1)
	s.Find = func(ctx context.Context, _ []string, _, _ map[string]bool) gitscan.Result {
		started <- struct{}{}
		<-ctx.Done()
		return gitscan.Result{}
	}
	stop := scanLoop(t, s, context.Background())
	s.kickFound()
	within(t, started, "walk")
	stop()
	if r := state.ReadFound(s.ReposPath); len(r.Repos) != 1 || r.Repos[0].Path != "/h/cached" {
		t.Fatalf("a scan cut short by the service stopping replaced the cache: %+v", r)
	}
}

func TestFoundOncePerConsole(t *testing.T) {
	s, _ := foundTest(t)
	foundReady(t, s)
	cl := &client{}
	if st, _ := s.stateFor(cl); st.Found == nil {
		t.Fatal("the first snapshot has no Found")
	}
	if st, _ := s.stateFor(cl); st.Found != nil {
		t.Fatalf("the second snapshot repeats Found: %+v", st.Found)
	}
	s.act(t.Context(), proto.Act{RecID: "scan:no"})
	if st, _ := s.stateFor(cl); st.Found == nil || st.Found.Consent != "no" {
		t.Fatalf("after a no: %+v", st.Found)
	}
}

func TestNoFoundWithoutFind(t *testing.T) {
	s := newTest(t, time.Hour)
	foundReady(t, s)
	if st, ok := s.stateFor(&client{}); !ok || st.Found != nil {
		t.Fatalf("Found = %+v with no Find", st.Found)
	}
}

func TestUnchangedRescanSendsNothing(t *testing.T) {
	s, _ := foundTest(t)
	foundReady(t, s)
	s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true})
	gen := func() (uint64, time.Time, time.Duration) {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.foundGen, s.found.At, s.found.Took
	}
	g1, at1, _ := gen()
	cl := &client{}
	if st, _ := s.stateFor(cl); st.Found == nil {
		t.Fatal("no first Found")
	}
	for !time.Now().After(at1) { // the rescan's At must be later, on any clock
		time.Sleep(time.Millisecond)
	}
	if res := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true}); res.Err != "" {
		t.Fatal(res.Err)
	}
	g2, at2, took2 := gen()
	if g2 != g1 {
		t.Fatalf("foundGen %d -> %d on an unchanged rescan", g1, g2)
	}
	if !at2.After(at1) {
		t.Fatal("the second scan was not stored")
	}
	if st, _ := s.stateFor(cl); st.Found != nil {
		t.Fatalf("an unchanged rescan sent Found: %+v", st.Found)
	}
	st, _ := s.stateFor(&client{})
	if st.Found == nil || st.Found.Took != took2 {
		t.Fatalf("a new console's Found = %+v, want Took %v", st.Found, took2)
	}
}

func TestScanSupersededIsTyped(t *testing.T) {
	err := error(supersededError{roots: []string{"/a", "/b"}})
	if !errors.Is(err, errScanSuperseded) || errors.Is(err, errScanNo) {
		t.Fatalf("errors.Is(%v): superseded %v, no %v", err, errors.Is(err, errScanSuperseded), errors.Is(err, errScanNo))
	}
	if got := err.Error(); got != "other folders were picked since: /a /b" {
		t.Fatalf("Error() = %q", got)
	}
}

// waitQueued waits until n yeses wait in the loop's queue.
func waitQueued(t *testing.T, s *Service, n int) {
	t.Helper()
	deadline := time.Now().Add(waitLimit)
	for len(s.scanReqs) < n {
		if time.Now().After(deadline) {
			t.Fatalf("%d yeses never queued", n)
		}
		time.Sleep(time.Millisecond)
	}
}

// A yes whose roots are not the walk's hears so from that walk, even when
// its roots are picked again while the walk runs: it never waits for a
// second walk.
func TestScanSupersededWithinOneWalk(t *testing.T) {
	s, f := foundTest(t)
	state.WriteConsent(s.ConsentPath, state.Consent{Answer: "yes", Roots: []string{"/a"}})
	started, release := blockingFind(s, f)
	foundReady(t, s)
	s.kickFound()
	within(t, started, "walk 0")
	y := yesAsync(t.Context(), s, "/b")
	waitRoots(t, s, []string{"/b"})
	waitQueued(t, s, 1)
	z := yesAsync(t.Context(), s, "/a")
	waitRoots(t, s, []string{"/a"})
	waitQueued(t, s, 2)
	release <- struct{}{}
	within(t, started, "walk 1, of /a, for both queued yeses")
	c := yesAsync(t.Context(), s, "/b")
	waitRoots(t, s, []string{"/b"})
	release <- struct{}{}
	// Walk 2 is held: these answers can only come from walk 1.
	if r := within(t, y, "the /b yes"); r.Err != "other folders were picked since: /b" {
		t.Fatalf("yes /b queued before walk 1 = %+v", r)
	}
	if r := within(t, z, "the /a yes"); !strings.HasPrefix(r.Err, "other folders were picked since: /b") {
		t.Fatalf("yes /a = %+v", r)
	}
	within(t, started, "walk 2, of /b")
	release <- struct{}{}
	if r := within(t, c, "the last /b yes"); r.Err != "" || r.Scan == nil {
		t.Fatalf("last yes = %+v", r)
	}
	if got := f.calls(); len(got) != 3 {
		t.Fatalf("scans = %v, want three", got)
	}
}

// A no and then a yes for the same roots while a walk runs: the walk
// began under the earlier yes, so it is dropped.
func TestScanNoThenYesMidWalk(t *testing.T) {
	s, _ := foundTest(t)
	started, release := make(chan struct{}, 4), make(chan struct{})
	var n atomic.Int32
	s.Find = func(context.Context, []string, map[string]bool, map[string]bool) gitscan.Result {
		i := n.Add(1)
		started <- struct{}{}
		<-release
		return gitscan.Result{Repos: []gitscan.Checkout{{Path: fmt.Sprintf("/walk%d", i)}}}
	}
	foundReady(t, s)
	first := yesAsync(t.Context(), s, "")
	within(t, started, "walk 1")
	if r := s.act(t.Context(), proto.Act{RecID: "scan:no"}); r.Err != "" {
		t.Fatal(r.Err)
	}
	second := yesAsync(t.Context(), s, "")
	waitQueued(t, s, 1)
	close(release)
	if r := within(t, first, "first yes"); r.Err != "a no came while it ran: nothing is kept" {
		t.Fatalf("yes before the no = %+v", r)
	}
	if r := within(t, second, "second yes"); r.Err != "" || r.Scan == nil {
		t.Fatalf("yes after the no = %+v", r)
	}
	if n.Load() != 2 {
		t.Fatalf("%d walks, want 2", n.Load())
	}
	if c := state.ReadFound(s.ReposPath); len(c.Repos) != 1 || c.Repos[0].Path != "/walk2" {
		t.Fatalf("cache = %+v, want only the walk after the no", c)
	}
}

// scripted returns the results in order, the last one again after.
func scripted(s *Service, results ...gitscan.Result) {
	var mu sync.Mutex
	n := 0
	s.Find = func(context.Context, []string, map[string]bool, map[string]bool) gitscan.Result {
		mu.Lock()
		defer mu.Unlock()
		r := results[min(n, len(results)-1)]
		n++
		return r
	}
}

func repoPaths(cs []gitscan.Checkout) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Path)
	}
	return out
}

func TestRescanKeepsReposOfAFolderThatDidNotAnswer(t *testing.T) {
	s, _ := foundTest(t)
	scripted(s,
		gitscan.Result{Repos: []gitscan.Checkout{{Path: "/default/a"}, {Path: "/default/b"}, {Path: "/other/c"}, {Path: "/defaultx/d"}}},
		gitscan.Result{Repos: []gitscan.Checkout{{Path: "/other/e"}}, Hung: []string{"/default"},
			Skipped: []gitscan.Skip{{Path: "/default", Why: "it did not answer"}}},
		gitscan.Result{Repos: []gitscan.Checkout{{Path: "/other/e"}},
			Skipped: []gitscan.Skip{{Path: "/default", Why: "it did not answer before"}}},
	)
	foundReady(t, s)
	yes := func() proto.Result {
		r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true})
		if r.Err != "" {
			t.Fatal(r.Err)
		}
		return r
	}
	yes()
	want := []string{"/default/a", "/default/b", "/other/e"}
	for i := range 2 {
		r := yes()
		if r.Scan.Repos != 3 || len(r.Scan.Skipped) != 1 || r.Scan.Skipped[0].Path != "/default" {
			t.Fatalf("walk %d: scan %+v: want the kept repos counted and /default named", i+2, r.Scan)
		}
		if got := repoPaths(firstFound(t, s).Repos); !slices.Equal(got, want) {
			t.Fatalf("walk %d: console repos %v, want %v", i+2, got, want)
		}
		if got := repoPaths(state.ReadFound(s.ReposPath).Repos); !slices.Equal(got, want) {
			t.Fatalf("walk %d: repos.json %v, want %v", i+2, got, want)
		}
	}
}

func TestRescanDropsReposOfAFolderThatAnswered(t *testing.T) {
	s, _ := foundTest(t)
	scripted(s,
		gitscan.Result{Repos: []gitscan.Checkout{{Path: "/default/a"}, {Path: "/default/b"}}},
		gitscan.Result{Repos: []gitscan.Checkout{{Path: "/default/a"}}},
	)
	foundReady(t, s)
	for range 2 {
		if r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true}); r.Err != "" {
			t.Fatal(r.Err)
		}
	}
	if got := repoPaths(firstFound(t, s).Repos); !slices.Equal(got, []string{"/default/a"}) {
		t.Fatalf("console repos %v: a repo gone from a folder that answered stays gone", got)
	}
}

func TestNamedFolderClearsHungMemory(t *testing.T) {
	s, _ := foundTest(t)
	var mu sync.Mutex
	var skips []map[string]bool
	s.Find = func(_ context.Context, _ []string, skip, _ map[string]bool) gitscan.Result {
		mu.Lock()
		defer mu.Unlock()
		skips = append(skips, skip)
		return gitscan.Result{}
	}
	now := time.Now()
	s.hung = map[string]time.Time{"/p": now, "/p/sub": now, "/q": now, "/px": now}
	foundReady(t, s)
	if r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Text: "/p", Console: true}); r.Err != "" {
		t.Fatal(r.Err)
	}
	if r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true}); r.Err != "" {
		t.Fatal(r.Err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(skips) != 2 {
		t.Fatalf("%d walks, want 2", len(skips))
	}
	if skips[0]["/p"] || skips[0]["/p/sub"] || !skips[0]["/q"] || !skips[0]["/px"] {
		t.Fatalf("skip %v: a folder the builder names is never skipped; others still are", skips[0])
	}
	if !skips[1]["/q"] {
		t.Fatalf("skip %v: a yes that names no folder clears nothing", skips[1])
	}
}

func TestScanRan(t *testing.T) {
	for _, err := range []error{errScanNo, errScanStopped, supersededError{[]string{"/a"}}, supersededError{}} {
		if !ScanRan(err.Error()) {
			t.Errorf("ScanRan(%q) = false", err.Error())
		}
	}
	for _, msg := range []string{scanRefused, "scan.json: denied", "this service finds no repositories", ""} {
		if ScanRan(msg) {
			t.Errorf("ScanRan(%q) = true", msg)
		}
	}
}

func TestRescanKeepsReposOfASymlinkedRootThatDidNotAnswer(t *testing.T) {
	s, _ := foundTest(t)
	scripted(s,
		gitscan.Result{Repos: []gitscan.Checkout{{Path: "/real/a"}, {Path: "/other/c"}},
			Resolved: map[string]string{"/default": "/real"}},
		gitscan.Result{Hung: []string{"/default"}, Skipped: []gitscan.Skip{{Path: "/default", Why: "it did not answer"}}},
	)
	foundReady(t, s)
	for range 2 {
		if r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true}); r.Err != "" {
			t.Fatal(r.Err)
		}
	}
	if got := repoPaths(firstFound(t, s).Repos); !slices.Equal(got, []string{"/real/a"}) {
		t.Fatalf("console repos %v: a hung link keeps the repos under where it last led", got)
	}
}

func TestRescanDropsReposUnderAWalkedRootBelowOneThatDidNotAnswer(t *testing.T) {
	s, _ := foundTest(t)
	s.ScanRoots = func([]string) []string { return []string{"/default", "/default/dev"} }
	scripted(s,
		gitscan.Result{Repos: []gitscan.Checkout{{Path: "/default/dev/a"}, {Path: "/default/dev/deleted"}, {Path: "/default/other/b"}},
			Resolved: map[string]string{"/default": "/default", "/default/dev": "/default/dev"}},
		gitscan.Result{Repos: []gitscan.Checkout{{Path: "/default/dev/a"}}, Hung: []string{"/default"},
			Skipped:  []gitscan.Skip{{Path: "/default", Why: "it did not answer"}},
			Resolved: map[string]string{"/default/dev": "/default/dev"}},
	)
	foundReady(t, s)
	for range 2 {
		if r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true}); r.Err != "" {
			t.Fatal(r.Err)
		}
	}
	if got, want := repoPaths(firstFound(t, s).Repos), []string{"/default/dev/a", "/default/other/b"}; !slices.Equal(got, want) {
		t.Fatalf("console repos %v, want %v: a root that answered below one that did not says what is gone", got, want)
	}
}

func TestRescanKeepsNoReposOutsideThePickedRoots(t *testing.T) {
	s, _ := foundTest(t)
	s.ScanRoots = func(picked []string) []string {
		if len(picked) == 0 {
			return []string{"/home/me"}
		}
		return picked
	}
	homeHung := gitscan.Result{Repos: []gitscan.Checkout{{Path: "/srv/code/x"}}, Hung: []string{"/home/me"},
		Skipped:  []gitscan.Skip{{Path: "/home/me", Why: "it did not answer"}},
		Resolved: map[string]string{"/srv/code": "/srv/code"}}
	scripted(s,
		gitscan.Result{Repos: []gitscan.Checkout{{Path: "/home/me/old"}}, Resolved: map[string]string{"/home/me": "/home/me"}},
		homeHung, homeHung)
	foundReady(t, s)
	if r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true}); r.Err != "" {
		t.Fatal(r.Err)
	}
	for i := range 2 {
		if r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Text: "/srv/code", Console: true}); r.Err != "" {
			t.Fatal(r.Err)
		}
		if got, want := repoPaths(firstFound(t, s).Repos), []string{"/srv/code/x"}; !slices.Equal(got, want) {
			t.Fatalf("walk %d: console repos %v, want %v: a hung lookup outside the picked roots keeps nothing", i+2, got, want)
		}
	}
}

func TestNamedFolderClearsHungMemoryCanonically(t *testing.T) {
	was := foldsCase
	t.Cleanup(func() { foldsCase = was })
	for _, fold := range []bool{true, false} {
		foldsCase = func() bool { return fold }
		s, _ := foundTest(t)
		var mu sync.Mutex
		var skips []map[string]bool
		s.Find = func(_ context.Context, _ []string, skip, _ map[string]bool) gitscan.Result {
			mu.Lock()
			defer mu.Unlock()
			skips = append(skips, skip)
			return gitscan.Result{}
		}
		now := time.Now()
		s.hung = map[string]time.Time{"/real/p/sub": now, "/Users/x/Code/sub": now, "/q": now}
		s.rootsResolved = map[string]string{"/p": "/real/p"}
		foundReady(t, s)
		if r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Text: "/p\n/Users/x/code", Console: true}); r.Err != "" {
			t.Fatal(r.Err)
		}
		mu.Lock()
		skip := skips[0]
		mu.Unlock()
		if skip["/real/p/sub"] || !skip["/q"] || skip["/Users/x/Code/sub"] != !fold {
			t.Fatalf("fold %v: skip %v: a named root clears its hung folders as they were stored", fold, skip)
		}
	}
}

// A picked folder that is not there fails the yes, with the folder as
// typed: scan.json keeps the roots it had, no walk runs for it, and the
// next walk is of the roots on record.
func TestScanYesMissingFolderKeepsTheRoots(t *testing.T) {
	s, f := foundTest(t)
	s.ScanRoots = func(picked []string) []string {
		if len(picked) == 0 {
			return []string{"/default"}
		}
		var out []string
		for _, p := range picked {
			out = append(out, "/h/"+strings.TrimPrefix(p, "~/"))
		}
		return out
	}
	var asked [][]string
	s.Missing = func(roots []string) []string {
		asked = append(asked, roots)
		var out []string
		for _, r := range roots {
			if r == "/h/dve" {
				out = append(out, r)
			}
		}
		return out
	}
	foundReady(t, s)
	if r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true, Text: "~/dev\n~/dve"}); r.Err != "no folder ~/dve" || r.Scan != nil {
		t.Fatalf("a yes for a missing folder before any answer = %+v", r)
	}
	if c := state.ReadConsent(s.ConsentPath); c.Answer != "" || len(c.Roots) != 0 {
		t.Fatalf("consent = %+v, want still never asked", c)
	}
	if len(asked) != 1 || !slices.Equal(asked[0], []string{"/h/dev", "/h/dve"}) {
		t.Fatalf("Missing asked %v, want the picked folders as walked", asked)
	}
	if r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true}); r.Err != "" {
		t.Fatalf("yes: %s", r.Err)
	}
	if r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true, Text: "~/dve"}); r.Err != "no folder ~/dve" {
		t.Fatalf("a yes for a missing folder = %+v", r)
	}
	if c := state.ReadConsent(s.ConsentPath); c.Answer != "yes" || len(c.Roots) != 0 {
		t.Fatalf("consent = %+v, want yes to the defaults", c)
	}
	if st, _ := s.stateFor(&client{}); st.Found == nil || len(st.Found.Roots) != 0 {
		t.Fatalf("Found = %+v, want the defaults' roots", st.Found)
	}
	if got := f.calls(); len(got) != 1 {
		t.Fatalf("scans = %v, want only the defaults' walk", got)
	}
	if r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true}); r.Err != "" {
		t.Fatalf("yes: %s", r.Err)
	}
	if got := f.calls(); len(got) != 2 || !slices.Equal(got[1], []string{"/default"}) {
		t.Fatalf("scans = %v, want the defaults again", got)
	}
}

// A walk names to Find the roots the builder picked, as walked, and only
// those.
func TestScanWalkNamesPickedRoots(t *testing.T) {
	s, _ := foundTest(t)
	s.ScanRoots = func(picked []string) []string {
		if len(picked) == 0 {
			return []string{"/default", "/conf"}
		}
		return append(slices.Clone(picked), "/conf")
	}
	var mu sync.Mutex
	var named []map[string]bool
	s.Find = func(_ context.Context, _ []string, _, n map[string]bool) gitscan.Result {
		mu.Lock()
		defer mu.Unlock()
		named = append(named, n)
		return gitscan.Result{}
	}
	foundReady(t, s)
	for _, text := range []string{"", "/a\n/b"} {
		if r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true, Text: text}); r.Err != "" {
			t.Fatalf("yes %q: %s", text, r.Err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(named) != 2 || len(named[0]) != 0 || len(named[1]) != 2 || !named[1]["/a"] || !named[1]["/b"] {
		t.Fatalf("named = %v, want none for the defaults, then /a /b", named)
	}
}

// A walk a bound cut says so to the yes, and in the cache; one that ran
// to the end does not, whatever it could not read.
func TestScanYesSaysStopped(t *testing.T) {
	s, _ := foundTest(t)
	scripted(s, gitscan.Result{Stopped: true, Errors: []string{"stopped after 30s: some repositories may be missing"}},
		gitscan.Result{Errors: []string{"cannot read /h/lost+found: permission denied"}})
	foundReady(t, s)
	r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true})
	if r.Err != "" || r.Scan == nil || !r.Scan.Stopped {
		t.Fatalf("a cut walk's yes = %+v", r)
	}
	if c := state.ReadFound(s.ReposPath); !c.Stopped {
		t.Fatalf("cache = %+v, want stopped", c)
	}
	r = s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true})
	if r.Err != "" || r.Scan == nil || r.Scan.Stopped || len(r.Scan.Errors) != 1 {
		t.Fatalf("a full walk's yes = %+v", r)
	}
}

// A walk's unreadable count reaches the yes; a cache that could not be
// written is said apart from the folders that could not be read.
func TestScanYesSaysUnreadableAndUnsaved(t *testing.T) {
	s, _ := foundTest(t)
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s.ReposPath = filepath.Join(file, "repos.json")
	scripted(s, gitscan.Result{Unreadable: 2, Errors: []string{"cannot read /a: permission denied", "cannot read /b: permission denied"}})
	foundReady(t, s)
	r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true})
	if r.Err != "" || r.Scan == nil || r.Scan.Unreadable != 2 || r.Scan.Unsaved == "" {
		t.Fatalf("yes = %+v", r)
	}
	if strings.HasPrefix(r.Scan.Unsaved, "repos.json") {
		t.Fatalf("Unsaved = %q, want the error alone", r.Scan.Unsaved)
	}
}

// A pick with several missing folders names them all, as typed.
func TestScanYesNamesEveryMissingFolder(t *testing.T) {
	s, f := foundTest(t)
	s.Missing = func(roots []string) []string {
		var out []string
		for _, r := range roots {
			if r != "/dev" {
				out = append(out, r)
			}
		}
		return out
	}
	foundReady(t, s)
	if r := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true, Text: "/dve\n/dev\n/sr c"}); r.Err != "no folder /dve, /sr c" {
		t.Fatalf("yes = %+v, want both missing folders", r)
	}
	if n := len(f.calls()); n != 0 {
		t.Fatalf("%d walks for a refused pick", n)
	}
}
