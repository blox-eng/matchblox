package service

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/state"
)

// scanEvery is how often the host is scanned again after a yes.
const scanEvery = 30 * time.Minute

const scanRefused = "only you say yes to a scan, at a console or in your own shell"

// loadFound reads the consent and, after a yes, the last scan, so the
// first snapshot carries them before any scan ends.
func (s *Service) loadFound() {
	if s.Find == nil {
		return
	}
	c := s.consent()
	f := gitscan.Found{Consent: c.Answer, Roots: c.Roots}
	if c.Answer == "yes" && s.ReposPath != "" {
		cached := state.ReadFound(s.ReposPath)
		f.At, f.Took, f.Repos, f.Errors = cached.At, cached.Took, cached.Repos, cached.Errors
	}
	s.setFound(f)
}

func (s *Service) consent() state.Consent {
	if s.ConsentPath == "" {
		s.mu.Lock()
		defer s.mu.Unlock()
		return state.Consent{Answer: s.found.Consent, Roots: s.found.Roots}
	}
	return state.ReadConsent(s.ConsentPath)
}

func (s *Service) setFound(f gitscan.Found) {
	s.mu.Lock()
	s.found = f
	s.foundGen++
	s.mu.Unlock()
	s.broadcast()
}

func (s *Service) kickFound() {
	select {
	case s.foundKick <- struct{}{}:
	default:
	}
}

// foundLoop starts after the first sample and reads the consent before
// every scan: no scan runs without a yes on record.
func (s *Service) foundLoop(ctx context.Context) {
	if s.Find == nil {
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-s.foundKick:
	}
	t := time.NewTicker(scanEvery)
	defer t.Stop()
	for {
		if s.consent().Answer == "yes" {
			s.scan(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.foundKick:
		}
	}
}

// scan walks the roots on record. A no or other roots that come while it
// walks win, and so does the service stopping: its result is dropped.
func (s *Service) scan(ctx context.Context) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	c := s.consent()
	if c.Answer != "yes" {
		return
	}
	roots := c.Roots
	if s.ScanRoots != nil {
		roots = s.ScanRoots(c.Roots)
	}
	start := time.Now()
	res := s.Find(ctx, roots, nil)
	if ctx.Err() != nil {
		return // the service stops: a cut walk must not replace the list
	}
	f := gitscan.Found{Consent: "yes", Roots: c.Roots, At: time.Now(), Took: time.Since(start),
		Repos: res.Repos, Errors: res.Errors, Skipped: res.Skipped, SkippedMore: res.SkippedMore}
	s.consentMu.Lock()
	defer s.consentMu.Unlock()
	if now := s.consent(); now.Answer != "yes" || !slices.Equal(now.Roots, c.Roots) {
		return
	}
	if s.ReposPath != "" {
		if err := state.WriteFound(s.ReposPath, f); err != nil {
			f.Errors = append(f.Errors, "repos.json: "+err.Error())
		}
	}
	s.setFound(f)
}

// scanYes records a yes, with the roots in text (one a line; none: those
// on record), and scans now.
func (s *Service) scanYes(ctx context.Context, text string) proto.Result {
	if s.Find == nil {
		return proto.Result{Err: "this service finds no repositories"}
	}
	var roots []string
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			roots = append(roots, l)
		}
	}
	s.consentMu.Lock()
	if len(roots) == 0 {
		roots = s.consent().Roots
	}
	c := state.Consent{Answer: "yes", Roots: roots, At: s.now()}
	if err := s.writeConsent(c); err != nil {
		s.consentMu.Unlock()
		return proto.Result{Err: "scan.json: " + err.Error()}
	}
	s.mu.Lock()
	f := s.found
	s.mu.Unlock()
	f.Consent, f.Roots = "yes", roots
	s.setFound(f)
	s.consentMu.Unlock()
	s.scan(ctx)
	return proto.Result{}
}

// scanNo records a no and forgets what was found.
func (s *Service) scanNo() proto.Result {
	if s.Find == nil {
		return proto.Result{Err: "this service finds no repositories"}
	}
	s.consentMu.Lock()
	defer s.consentMu.Unlock()
	if err := s.writeConsent(state.Consent{Answer: "no", At: s.now()}); err != nil {
		return proto.Result{Err: "scan.json: " + err.Error()}
	}
	s.setFound(gitscan.Found{Consent: "no"})
	if s.ReposPath != "" {
		if err := os.Remove(s.ReposPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return proto.Result{Err: "repos.json: " + err.Error()}
		}
	}
	return proto.Result{}
}

func (s *Service) writeConsent(c state.Consent) error {
	if s.ConsentPath == "" {
		return nil
	}
	return state.WriteConsent(s.ConsentPath, c)
}
