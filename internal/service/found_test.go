package service

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
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
	s.Find = func(_ context.Context, roots []string, _ map[string]bool) gitscan.Result {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.roots = append(f.roots, roots)
		return gitscan.Result{Repos: []gitscan.Checkout{{Path: "/h/a", Branch: "main"}}}
	}
	return s, f
}

func foundReady(s *Service) {
	s.loadFound()
	s.publish(sample.Snapshot{At: time.Now()}, func(state.Doc) {})
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
	foundReady(s)
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
	foundReady(s)
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
	foundReady(s)
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
	if res := s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", From: "%3", Console: true}); res.Err != scanRefused {
		t.Fatalf("scan:yes from a console hello in an agent's pane: %q", res.Err)
	}
	if n := len(f.calls()); n != 0 {
		t.Fatalf("scanned %d times", n)
	}
}

// blockingFind holds each walk until release, and finds roots[0]/r.
func blockingFind(s *Service) (started, release chan struct{}) {
	started, release = make(chan struct{}, 2), make(chan struct{})
	s.Find = func(_ context.Context, roots []string, _ map[string]bool) gitscan.Result {
		started <- struct{}{}
		<-release
		return gitscan.Result{Repos: []gitscan.Checkout{{Path: roots[0] + "/r"}}}
	}
	return started, release
}

func TestNoDuringScanWins(t *testing.T) {
	s, _ := foundTest(t)
	foundReady(s)
	started, release := blockingFind(s)
	done := make(chan struct{})
	go func() { s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true}); close(done) }()
	<-started
	if res := s.act(t.Context(), proto.Act{RecID: "scan:no"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	close(release)
	<-done
	if _, err := os.Stat(s.ReposPath); !os.IsNotExist(err) {
		t.Fatalf("repos.json written after a no: %v", err)
	}
	if st, _ := s.stateFor(&client{}); st.Found == nil || st.Found.Consent != "no" || len(st.Found.Repos) != 0 {
		t.Fatalf("Found = %+v", st.Found)
	}
}

func TestNarrowedRootsDuringScanWin(t *testing.T) {
	s, _ := foundTest(t)
	state.WriteConsent(s.ConsentPath, state.Consent{Answer: "yes", Roots: []string{"/wide"}})
	foundReady(s)
	started, release := blockingFind(s)
	first := make(chan struct{})
	go func() { s.scan(t.Context()); close(first) }()
	<-started
	second := make(chan struct{})
	go func() {
		s.act(t.Context(), proto.Act{RecID: "scan:yes", Confirm: "y", Console: true, Text: "/narrow"})
		close(second)
	}()
	deadline := time.Now().Add(waitLimit)
	for !slices.Equal(state.ReadConsent(s.ConsentPath).Roots, []string{"/narrow"}) {
		if time.Now().After(deadline) {
			t.Fatal("the narrowed roots were never recorded")
		}
		time.Sleep(time.Millisecond)
	}
	release <- struct{}{}
	<-first
	if r := state.ReadFound(s.ReposPath); len(r.Repos) != 0 {
		t.Fatalf("the wide walk was kept after the roots narrowed: %+v", r)
	}
	if st, _ := s.stateFor(&client{}); st.Found != nil && len(st.Found.Repos) != 0 {
		t.Fatalf("the wide walk went to consoles: %+v", st.Found)
	}
	<-started
	release <- struct{}{}
	<-second
	r := state.ReadFound(s.ReposPath)
	if len(r.Repos) != 1 || r.Repos[0].Path != "/narrow/r" || !slices.Equal(r.Roots, []string{"/narrow"}) {
		t.Fatalf("cache = %+v, want only the narrowed walk", r)
	}
}

func TestCacheSentBeforeFirstScan(t *testing.T) {
	s, _ := foundTest(t)
	state.WriteConsent(s.ConsentPath, state.Consent{Answer: "yes"})
	state.WriteFound(s.ReposPath, gitscan.Found{Consent: "yes", Repos: []gitscan.Checkout{{Path: "/h/cached"}}})
	s.Find = func(ctx context.Context, _ []string, _ map[string]bool) gitscan.Result {
		<-ctx.Done()
		return gitscan.Result{}
	}
	run(t, s)
	if got := firstFound(t, s); got == nil || len(got.Repos) != 1 || got.Repos[0].Path != "/h/cached" {
		t.Fatalf("first Found = %+v, want the cache", got)
	}
}

func TestCancelledScanKeepsCache(t *testing.T) {
	s, _ := foundTest(t)
	state.WriteConsent(s.ConsentPath, state.Consent{Answer: "yes"})
	state.WriteFound(s.ReposPath, gitscan.Found{Consent: "yes", Repos: []gitscan.Checkout{{Path: "/h/cached"}}})
	s.Find = func(context.Context, []string, map[string]bool) gitscan.Result { return gitscan.Result{} }
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s.scan(ctx)
	if r := state.ReadFound(s.ReposPath); len(r.Repos) != 1 || r.Repos[0].Path != "/h/cached" {
		t.Fatalf("a scan cut short by the service stopping replaced the cache: %+v", r)
	}
}

func TestFoundOncePerConsole(t *testing.T) {
	s, _ := foundTest(t)
	foundReady(s)
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
	foundReady(s)
	if st, ok := s.stateFor(&client{}); !ok || st.Found != nil {
		t.Fatalf("Found = %+v with no Find", st.Found)
	}
}
