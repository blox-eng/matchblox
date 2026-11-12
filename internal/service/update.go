package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/config"
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
	s.setUpdateView(v)
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
	defer s.updateMu.Unlock()
	s.mu.Lock()
	var v state.UpdateView
	if s.cur.Update != nil {
		v = *s.cur.Update
	}
	s.mu.Unlock()
	v.CheckedAt, v.Err = now, ""
	if err != nil {
		v.Err = err.Error()
	} else if tag != s.updates.Latest {
		s.updates.Latest, s.updates.FirstSeen = tag, now
		if s.UpdatePath != "" {
			if err := state.WriteUpdate(s.UpdatePath, s.updates); err != nil {
				fmt.Fprintln(os.Stderr, "matchblox: update.json:", err)
			}
		}
	}
	v.Latest, v.FirstSeen = s.updates.Latest, s.updates.FirstSeen
	s.setUpdateView(v)
}

// setUpdateView sends v to every console when it changed. The view is
// replaced, never changed in place: a snapshot sent before holds the old one.
func (s *Service) setUpdateView(v state.UpdateView) {
	s.mu.Lock()
	if s.cur.Update != nil && *s.cur.Update == v {
		s.mu.Unlock()
		return
	}
	s.cur.Update, s.gen = &v, s.gen+1
	s.mu.Unlock()
	s.broadcast()
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
