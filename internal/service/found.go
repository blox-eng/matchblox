package service

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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
		f.Skipped, f.SkippedMore = cached.Skipped, cached.SkippedMore
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
	same := s.foundGen > 0 && s.found.Same(f) // the first one always goes out
	s.found = f                               // always kept: a new console reads the latest Took
	if !same {
		s.foundGen++
	}
	s.mu.Unlock()
	if !same {
		s.broadcast()
	}
}

func (s *Service) kickFound() {
	select {
	case s.foundKick <- struct{}{}:
	default:
	}
}

// hungFor is how long a folder that did not answer is skipped: long
// enough not to wait on it every 30 minutes, short enough that a slow
// but live one comes back the same day.
const hungFor = 6 * time.Hour

// scanReq is a yes that waits for a walk of roots, the picked ones it
// recorded. done holds one answer, so the loop never waits on a yes
// whose console has gone.
type scanReq struct {
	roots []string
	done  chan scanOutcome
}

// scanOutcome is a walk's answer to a yes: the Found it stored, or why
// it stored none for that yes.
type scanOutcome struct {
	found gitscan.Found
	err   error
}

var (
	errScanNo         = errors.New("a no came while it ran: nothing is kept")
	errScanStopped    = errors.New("the service stopped before the scan ended")
	errScanSuperseded = errors.New("other folders were picked since")
)

// supersededError tells a yes that the roots it asked for are no longer
// the ones on record.
type supersededError struct{ roots []string }

func (e supersededError) Error() string {
	roots := strings.Join(e.roots, " ")
	if roots == "" {
		roots = "the defaults"
	}
	return errScanSuperseded.Error() + ": " + roots
}

func (e supersededError) Is(target error) bool { return target == errScanSuperseded }

// foundLoop is the only walker. It starts after the first sample, or at
// the first yes, and reads the consent before every walk: no walk runs
// without a yes on record. The yeses that queue while it walks share the
// next walk.
func (s *Service) foundLoop(ctx context.Context) {
	if s.Find == nil {
		return
	}
	defer close(s.scanEnd)
	var reqs []scanReq
	select {
	case <-ctx.Done():
		return
	case <-s.foundKick:
	case r := <-s.scanReqs:
		reqs = append(reqs, r)
	}
	t := time.NewTicker(scanEvery)
	defer t.Stop()
	for {
		for drained := false; !drained; {
			select {
			case r := <-s.scanReqs:
				reqs = append(reqs, r)
			default:
				drained = true
			}
		}
		s.walk(ctx, reqs)
		reqs = nil
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.foundKick:
		case r := <-s.scanReqs:
			reqs = append(reqs, r)
		}
	}
}

// walk scans the roots on record and answers every one of reqs. A no or
// other roots that come while it walks win, and so does the service
// stopping: its result is dropped. A yes whose roots are not the walk's
// hears that other folders were picked; the yes that picked them is
// queued and gets its own walk.
func (s *Service) walk(ctx context.Context, reqs []scanReq) {
	all := func(err error) {
		for _, r := range reqs {
			r.done <- scanOutcome{err: err} // done holds one: never blocks
		}
	}
	if ctx.Err() != nil {
		all(errScanStopped)
		return
	}
	s.consentMu.Lock()
	c, nos := s.consent(), s.nos
	s.consentMu.Unlock()
	if c.Answer != "yes" {
		all(errScanNo)
		return
	}
	roots := c.Roots
	if s.ScanRoots != nil {
		roots = s.ScanRoots(c.Roots)
	}
	start := time.Now()
	res := s.Find(ctx, roots, s.skipHung(start))
	// Folders that hung are kept even when the walk's result is dropped:
	// they hang the next walk just the same.
	for _, p := range res.Hung {
		s.hung[p] = time.Now()
	}
	if ctx.Err() != nil {
		all(errScanStopped) // a cut walk must not replace the list
		return
	}
	f := gitscan.Found{Consent: "yes", Roots: c.Roots, At: time.Now(), Took: time.Since(start),
		Repos: s.carryOver(res), Errors: res.Errors, Skipped: res.Skipped, SkippedMore: res.SkippedMore}
	f, on, err := s.keep(f, nos)
	if errors.Is(err, errScanNo) {
		all(err)
		return
	}
	for _, r := range reqs {
		if err == nil && slices.Equal(r.roots, f.Roots) {
			r.done <- scanOutcome{found: f}
		} else {
			r.done <- scanOutcome{err: supersededError{on}}
		}
	}
}

// carryOver is res's repos and, from the last scan, those under a folder
// res could not read because it did not answer: a folder that did not
// answer keeps what was found there, sorted by path as Find sorts.
func (s *Service) carryOver(res gitscan.Result) []gitscan.Checkout {
	var dark []string
	for _, p := range res.Hung {
		dark = append(dark, filepath.Clean(p))
	}
	for _, k := range res.Skipped {
		if k.Unanswered() {
			dark = append(dark, filepath.Clean(k.Path))
		}
	}
	if len(dark) == 0 {
		return res.Repos
	}
	s.mu.Lock()
	last := s.found.Repos
	s.mu.Unlock()
	out := slices.Clone(res.Repos)
	have := map[string]bool{}
	for _, c := range res.Repos {
		have[c.Path] = true
	}
	for _, c := range last {
		if !have[c.Path] && slices.ContainsFunc(dark, func(d string) bool { return under(c.Path, d) }) {
			have[c.Path] = true
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b gitscan.Checkout) int { return strings.Compare(a.Path, b.Path) })
	return out
}

// under reports whether path is dir or below it.
func under(path, dir string) bool {
	path = filepath.Clean(path)
	return path == dir || strings.HasPrefix(path, strings.TrimSuffix(dir, string(filepath.Separator))+string(filepath.Separator))
}

// keep stores f, the walk of the roots in f.Roots that started after nos
// noes, if the consent on record still asks for it. It hands back what
// it stored and the roots on record: errScanNo if a no came since, a
// supersededError if other roots were picked since.
func (s *Service) keep(f gitscan.Found, nos uint64) (gitscan.Found, []string, error) {
	s.consentMu.Lock()
	defer s.consentMu.Unlock()
	now := s.consent()
	if now.Answer != "yes" || s.nos != nos {
		return f, nil, errScanNo
	}
	if !slices.Equal(now.Roots, f.Roots) {
		return f, now.Roots, supersededError{now.Roots}
	}
	if s.ReposPath != "" {
		if err := state.WriteFound(s.ReposPath, f); err != nil {
			f.Errors = append(f.Errors, "repos.json: "+err.Error())
		}
	}
	s.setFound(f)
	return f, now.Roots, nil
}

// skipHung forgets the folders that hung more than hungFor before now,
// and hands Find a copy of the rest: its goroutines may read it after
// it returns.
func (s *Service) skipHung(now time.Time) map[string]bool {
	if s.hung == nil {
		s.hung = map[string]time.Time{}
	}
	skip := make(map[string]bool, len(s.hung))
	for p, at := range s.hung {
		if now.Sub(at) > hungFor {
			delete(s.hung, p)
		} else {
			skip[p] = true
		}
	}
	return skip
}

// scanYes records a yes, with the roots in text (one a line; none: those
// on record), and waits for a walk of them: it says how that walk went.
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

	r := scanReq{roots: roots, done: make(chan scanOutcome, 1)}
	stopped := proto.Result{Err: errScanStopped.Error()}
	select {
	case s.scanReqs <- r:
	case <-ctx.Done():
		return stopped
	case <-s.scanEnd:
		return stopped
	}
	select {
	case o := <-r.done:
		return o.result()
	case <-ctx.Done():
		return stopped
	case <-s.scanEnd:
		select { // the loop answers every yes it took before it ends
		case o := <-r.done:
			return o.result()
		default:
			return stopped
		}
	}
}

// result is what the yes hears: why nothing was kept, or the summary of
// the Found that was.
func (o scanOutcome) result() proto.Result {
	if o.err != nil {
		return proto.Result{Err: o.err.Error()}
	}
	sum := o.found.Summary()
	return proto.Result{Scan: &sum}
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
	s.nos++
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
