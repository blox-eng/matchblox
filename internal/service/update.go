package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/release"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/update"
)

// UpdateSource finds the latest release and installs one.
type UpdateSource interface {
	Latest() (string, error)
	Install(tag string) (update.Proof, error)
	Managed() string
}

// updateTick is how often the update loop reads the clock. The wall clock
// decides when a look is due: a ticker of a day stops while a laptop sleeps.
const updateTick = time.Minute

// updating is true for a service that looks for releases: one with an
// updater whose own build is a release.
func (s *Service) updating() bool {
	return s.Update != nil && release.IsRelease(s.Binary)
}

// loadUpdate puts the update view in the state, before the first sample,
// with what update.json kept: a console knows this host has the update
// act from its first snapshot.
func (s *Service) loadUpdate() {
	if !s.updating() {
		return
	}
	s.updateMu.Lock()
	if s.UpdatePath != "" {
		s.updates = state.ReadUpdate(s.UpdatePath)
	}
	f := s.updates
	s.updateMu.Unlock()
	v := state.UpdateView{Mode: s.UpdateMode, Current: s.Binary}
	if s.UpdateMode != config.UpdateOff {
		v.Latest, v.FirstSeen, v.Last = f.Latest, f.FirstSeen, f.Last
		v.Managed = s.Update.Managed()
	}
	s.updateMu.Lock()
	changed := s.setUpdateView(v)
	s.updateMu.Unlock()
	if changed {
		s.refreshDoors()
	}
}

// updateLoop looks for a newer release at start and then every
// UpdateEvery, until ctx ends. Under mode off it never looks.
func (s *Service) updateLoop(ctx context.Context) {
	if !s.updating() || s.UpdateMode == config.UpdateOff {
		return
	}
	every := s.UpdateEvery
	if every <= 0 {
		every = 24 * time.Hour
	}
	ticks := s.updateTicks
	if ticks == nil {
		t := time.NewTicker(updateTick)
		defer t.Stop()
		ticks = t.C
	}
	var next time.Time
	for {
		if now := s.now(); !now.Before(next) {
			s.checkUpdate(now)
			next = now.Add(every)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
	}
}

// checkUpdate looks for the latest release once. A failed look keeps the
// latest release found before and says why it failed.
func (s *Service) checkUpdate(now time.Time) {
	tag, err := s.Update.Latest()
	s.updateMu.Lock()
	v := s.currentUpdateView()
	v.CheckedAt, v.Err = now, ""
	if err != nil {
		v.Err = err.Error()
	} else if tag != s.updates.Latest {
		s.updates.Latest, s.updates.FirstSeen = tag, now
		s.saveUpdates()
	}
	v.Latest, v.FirstSeen = s.updates.Latest, s.updates.FirstSeen
	changed := s.setUpdateView(v)
	s.updateMu.Unlock()
	if changed {
		s.refreshDoors()
	}
}

// currentUpdateView is a copy of the view in the state.
func (s *Service) currentUpdateView() state.UpdateView {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur.Update == nil {
		return state.UpdateView{}
	}
	return *s.cur.Update
}

// setUpdateView puts v in the state and reports whether it changed. Hold
// updateMu, so two writers of the view do not drop each other's change;
// then, with updateMu unlocked, refreshDoors sends it with the doors it
// opens or closes. The view is replaced, never changed in place: a
// snapshot sent before holds the old one.
func (s *Service) setUpdateView(v state.UpdateView) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur.Update != nil && *s.cur.Update == v {
		return false
	}
	s.cur.Update, s.gen = &v, s.gen+1
	return true
}

// saveUpdates writes update.json. Hold updateMu.
func (s *Service) saveUpdates() {
	if s.UpdatePath == "" {
		return
	}
	if err := state.WriteUpdate(s.UpdatePath, s.updates); err != nil {
		fmt.Fprintln(os.Stderr, "matchblox: update.json:", err)
	}
}

// newer is the latest release when it is newer than this build. A
// pre-release is never offered.
func (s *Service) newer() (tag string, ok bool) {
	s.mu.Lock()
	v := s.cur.Update
	s.mu.Unlock()
	if v == nil || !release.IsRelease(v.Latest) || strings.Contains(v.Latest, "-") {
		return "", false
	}
	if release.Compare(v.Current, v.Latest) >= 0 {
		return "", false
	}
	return v.Latest, true
}

// managedProblem is why a package manager's install has no update act.
const managedProblem = "Homebrew installed matchblox. Update it with Homebrew."

// updateDoor offers the newer release, unless a person closed the offer
// of that same release.
func (s *Service) updateDoor() (doors.Door, bool) {
	tag, ok := s.newer()
	if !ok {
		return doors.Door{}, false
	}
	s.updateMu.Lock()
	closed := s.updates.Closed
	s.updateMu.Unlock()
	v := s.currentUpdateView()
	if closed == tag || v.Mode == config.UpdateOff {
		return doors.Door{}, false
	}
	d := doors.Door{ID: doors.Update, Title: "matchblox " + tag + " is out", Why: "This machine runs " + v.Current + ".", Sum: tag}
	if v.Managed != "" {
		d.Preview, d.Problem = v.Managed, managedProblem
		return d, true
	}
	lines := []string{
		fmt.Sprintf("matchblox %s → %s (%s/%s)", v.Current, tag, runtime.GOOS, runtime.GOARCH),
		"  checksum    checked before anything changes",
		"  provenance  checked when gh can verify it",
	}
	if s.Exe != "" {
		path, err := filepath.EvalSymlinks(s.Exe)
		if err != nil {
			path = s.Exe
		}
		lines = append(lines, "  replaces    "+path)
	}
	d.Preview = strings.Join(lines, "\n")
	return d, true
}

// updateAct installs the release the person saw on the door, only while
// it is still the latest.
func (s *Service) updateAct(tag string) proto.Result {
	latest, ok := s.newer()
	switch {
	case !ok:
		return proto.Result{Err: "no newer release to install"}
	case tag != latest:
		return proto.Result{Err: "a newer release came out: look again"}
	}
	return s.installUpdate(tag)
}

// closeUpdate closes the offer of the latest release; a newer one opens
// it again.
func (s *Service) closeUpdate() proto.Result {
	tag, ok := s.newer()
	if !ok {
		return proto.Result{Err: "no newer release to close"}
	}
	s.updateMu.Lock()
	s.updates.Closed = tag
	s.saveUpdates()
	s.updateMu.Unlock()
	return proto.Result{Ran: [][]string{{"close", doors.Update}}}
}

// installUpdate installs tag over this binary and records how it went. A
// good install changes the binary on disk, and the watch of it stops the
// service (ErrReplaced): the next console starts the new one.
func (s *Service) installUpdate(tag string) proto.Result {
	if !s.installing.CompareAndSwap(false, true) {
		return proto.Result{Skipped: []string{"already updating"}}
	}
	defer s.installing.Store(false)
	if !s.updating() {
		return proto.Result{Err: "this matchblox is not a release build, so it does not update itself"}
	}
	if cmd := s.Update.Managed(); cmd != "" {
		return proto.Result{Err: managedProblem + " Run: " + cmd}
	}
	proof, err := s.Update.Install(tag)
	rec := state.UpdateRecord{From: s.Binary, To: tag, At: s.now(), Checksum: proof.Checksum, Provenance: proof.Provenance}
	if err != nil {
		rec.Err = err.Error()
	}
	s.updateMu.Lock()
	s.updates.Last = &rec
	s.saveUpdates()
	v := s.currentUpdateView()
	last := rec
	v.Last = &last
	changed := s.setUpdateView(v)
	s.updateMu.Unlock()
	if changed {
		s.refreshDoors()
	}
	if err != nil {
		return proto.Result{Err: err.Error()}
	}
	return proto.Result{Ran: [][]string{{"install", tag}}}
}
