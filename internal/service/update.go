package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	// InstallProven also refuses a build whose provenance it did not check.
	InstallProven(tag string) (update.Proof, error)
	Managed() string
}

// updateTick is how often the update loop reads the clock. The wall clock
// decides when a look is due: a ticker of a day stops while a laptop sleeps.
const updateTick = time.Minute

// updating is true for a service that looks for releases: one with an
// updater whose own build is a release, not a build from a commit.
func (s *Service) updating() bool {
	return s.Update != nil && release.IsRelease(s.Binary) && !release.IsPseudo(s.Binary)
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
	s.loadedAt = s.now()
	s.updateMu.Unlock()
	v := state.UpdateView{Mode: s.UpdateMode, Current: s.Binary}
	if s.UpdateMode != config.UpdateOff {
		v.Latest, v.FirstSeen, v.Last = f.Latest, f.FirstSeen, f.Last
		v.Managed = s.Update.Managed()
		if l := f.Last; l != nil && l.Err == "" && l.At.Before(s.loadedAt) && release.Compare(s.Binary, l.To) < 0 {
			v.Restart = restartFix(l.To, s.Binary)
		}
	}
	s.updateMu.Lock()
	changed := s.setUpdateView(v)
	s.updateMu.Unlock()
	if changed {
		s.refreshDoors()
	}
}

// updateLoop looks for a newer release at start and then every
// updateEvery, until ctx ends. Under mode off it never looks. Under mode
// auto, each wake installs a release that is due, or says why it waits;
// one that failed is tried again after the next look.
func (s *Service) updateLoop(ctx context.Context) {
	if !s.updating() || s.UpdateMode == config.UpdateOff {
		return
	}
	ticks := s.updateTicks
	if ticks == nil {
		t := time.NewTicker(updateTick)
		defer t.Stop()
		ticks = t.C
	}
	var next time.Time
	tried := false // an auto install ran since the last look
	for {
		now := s.now()
		if !now.Before(next) {
			s.checkUpdate(now)
			next, tried = now.Add(updateEvery), false
		}
		due, waiting := false, ""
		if !tried {
			due, waiting = s.autoDue(now)
		}
		s.setWaiting(waiting)
		if tag, ok := s.newer(); ok && due && s.autoInstall(tag) {
			tried = true
		}
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
	}
}

// updateEvery is how often the service looks for a newer release.
const updateEvery = 24 * time.Hour

// autoWait is how long mode auto waits from the first sight of a release
// before it installs it.
const autoWait = 24 * time.Hour

// autoDue is true when mode auto installs the newer release now: a day
// after this machine first saw it, once, and with no work of its own in
// flight. When only that work holds it back, waiting says which. A closed
// offer does not stop it: the person closed the offer only.
func (s *Service) autoDue(now time.Time) (due bool, waiting string) {
	if s.UpdateMode != config.UpdateAuto || !s.updating() || s.Update.Managed() != "" {
		return false, ""
	}
	tag, ok := s.newer()
	if !ok {
		return false, ""
	}
	v := s.currentUpdateView()
	if l := v.Last; l != nil && l.To == tag && l.Err == "" {
		return false, "" // installed: a service that still runs the old one says so
	}
	if v.FirstSeen.IsZero() || now.Sub(v.FirstSeen) < autoWait {
		return false, ""
	}
	if why := s.busy(); why != "" {
		return false, why
	}
	return true, ""
}

// busy says what work is in flight: a stoker run, the host's or a
// chain's, or a clone. "": none.
func (s *Service) busy() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur.Stoker != nil && s.cur.Stoker.On {
		return "auto waits for the stoker run to end"
	}
	for _, c := range s.cur.Stokers {
		if c.View.On {
			return "auto waits for the stoker run to end"
		}
	}
	if _, ok := s.cloneRunsLocked(); ok {
		return "auto waits for a clone to end"
	}
	return ""
}

// cloneRunsLocked is the repo of a clone that runs. Hold s.mu.
func (s *Service) cloneRunsLocked() (repo string, ok bool) {
	for _, j := range s.clones {
		if j.end.IsZero() {
			return j.c.Repo, true
		}
	}
	return "", false
}

// setWaiting puts why auto waits in the view; an unchanged view is not
// sent again.
func (s *Service) setWaiting(why string) {
	s.updateMu.Lock()
	v := s.currentUpdateView()
	changed := false
	if v.Mode != "" && v.Waiting != why {
		v.Waiting = why
		changed = s.setUpdateView(v)
	}
	s.updateMu.Unlock()
	if changed {
		s.refreshDoors()
	}
}

// checkUpdate looks for the latest release once. A failed look keeps the
// latest release found before and says why it failed.
func (s *Service) checkUpdate(now time.Time) {
	tag, err := s.Update.Latest()
	if err == nil && !release.IsRelease(tag) {
		err = fmt.Errorf("the latest release %q is not a release tag", tag)
	}
	s.updateMu.Lock()
	v := s.currentUpdateView()
	v.CheckedAt, v.Err = now, ""
	f := &s.updates
	switch {
	case err != nil:
		v.Err = err.Error()
		if f.Failed.IsZero() {
			f.Failed = now
			s.saveUpdates()
		}
	case tag != f.Latest:
		f.Latest, f.FirstSeen = tag, now
		fallthrough
	default:
		// A first sight later than now was written by a clock that ran
		// ahead; with none, the day starts now.
		if f.FirstSeen.IsZero() || f.FirstSeen.After(now) {
			f.FirstSeen = now
		}
		f.Failed = time.Time{}
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
	if v == nil || !release.IsRelease(v.Latest) || release.IsPre(v.Latest) {
		return "", false
	}
	if release.Compare(v.Current, v.Latest) >= 0 {
		return "", false
	}
	return v.Latest, true
}

// managedProblem is why a package manager's install has no update act.
const managedProblem = "Homebrew installed matchblox. Update it with Homebrew."

// restartAfter is how long after a good install the restart may take
// before the person is asked to open matchblox again: the watch of the
// binary looks once a minute.
const restartAfter = 2 * time.Minute

// restartFix is the fix for a release installed on disk that the service
// does not run.
func restartFix(to, runs string) string {
	return fmt.Sprintf("%s is installed, but this service still runs %s. Quit matchblox and open it again.", to, runs)
}

// stuckLocked is the release this service installed when the restart for
// it has not come within restartAfter, else "". Hold updateMu.
func (s *Service) stuckLocked() string {
	if l := s.updates.Last; s.installed != "" && l != nil && l.To == s.installed && s.now().Sub(l.At) >= restartAfter {
		return s.installed
	}
	return ""
}

// oneLine is a reason of several lines on one, for a row that wraps.
func oneLine(why string) string {
	var parts []string
	for l := range strings.SplitSeq(why, "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			parts = append(parts, strings.TrimSuffix(l, "."))
		}
	}
	return strings.Join(parts, " · ")
}

// unproven is said beside an install whose build provenance was not
// checked: the service installs with no gh, or one not logged in.
const unproven = " · provenance not checked"

// restartDoor is the row of a release installed on disk that the service
// does not run. It has no action: the fix is the person's.
func restartDoor(tag, runs, fix string, last *state.UpdateRecord) doors.Door {
	if last != nil && !last.Provenance {
		fix = strings.TrimSuffix(fix, ".") + unproven
	}
	return doors.Door{ID: doors.Update, Title: "Restart for " + tag, Why: "This machine runs " + runs + ".", Sum: tag, Problem: fix}
}

var sha256Hex = regexp.MustCompile(`\b[0-9a-f]{64}\b`)

// failReason is why an install failed, as a sentence for a row: on one
// line, each checksum cut to its start, and without the words the row
// leads with.
func failReason(err string) string {
	why := oneLine(err)
	for _, said := range []string{" · Nothing was changed", ". Nothing was changed"} {
		why = strings.ReplaceAll(why, said, "")
	}
	why = sha256Hex.ReplaceAllStringFunc(why, func(h string) string { return h[:12] + "…" })
	if !strings.HasSuffix(why, "…") {
		why += "."
	}
	return why
}

// updateDoor offers the newer release, unless a person closed the offer
// of that same release. A restart that does not come is said first: under
// mode off, over a closed offer and over a newer release.
func (s *Service) updateDoor() (doors.Door, bool) {
	s.updateMu.Lock()
	closed, stuck, last := s.updates.Closed, s.stuckLocked(), s.updates.Last
	s.updateMu.Unlock()
	if stuck != "" {
		return restartDoor(stuck, s.Binary, restartFix(stuck, s.Binary), last), true
	}
	tag, ok := s.newer()
	if !ok {
		return doors.Door{}, false
	}
	v := s.currentUpdateView()
	installed := v.Last != nil && v.Last.To == tag && v.Last.Err == ""
	switch {
	case closed == tag || v.Mode == config.UpdateOff:
		return doors.Door{}, false
	case installed && v.Restart != "":
		return restartDoor(tag, v.Current, v.Restart, v.Last), true
	case installed:
		return doors.Door{}, false
	}
	d := doors.Door{ID: doors.Update, Title: "Update to " + tag, Why: "matchblox " + tag + " is out. This machine runs " + v.Current + ".", Sum: tag}
	if l := v.Last; l != nil && l.To == tag && l.Err != "" {
		// The outcome and the fix before the reason, which is long. In Why,
		// not Problem: a door with a problem has no action, and Enter is the fix.
		d.Why += " The install failed, nothing was changed. Enter tries again, or run: matchblox update. " + failReason(l.Err)
	}
	if at := v.FirstSeen.Add(autoWait); v.Mode == config.UpdateAuto && v.Managed == "" && !v.FirstSeen.IsZero() && s.now().Before(at) {
		d.Why += " Auto installs it after " + at.Local().Format("Jan 2 at 15:04") + "."
	}
	if v.Waiting != "" {
		d.Why += " " + strings.ToUpper(v.Waiting[:1]) + v.Waiting[1:] + "."
	}
	if v.Managed != "" {
		d.Preview, d.Problem = v.Managed, managedProblem
		return d, true
	}
	lines := []string{
		fmt.Sprintf("matchblox %s → %s (%s/%s)", v.Current, tag, runtime.GOOS, runtime.GOARCH),
		"  checksum    checked first",
		"  provenance  checked if gh is installed", // two lines: a phone shows 43 columns
		"              and logged in",
	}
	if s.Exe != "" {
		path, err := filepath.EvalSymlinks(s.Exe)
		if err != nil {
			path = s.Exe
		}
		lines = append(lines, "  replaces", "    "+path) // its own line: a phone still shows the file name
	}
	d.Preview = strings.Join(lines, "\n")
	return d, true
}

// staleTag is the answer to an act on a release that is no longer the latest.
const staleTag = "a newer release came out: look again"

// updateAct installs the release the person saw on the door, only while
// it is still the latest.
func (s *Service) updateAct(tag string) proto.Result {
	latest, ok := s.newer()
	switch {
	case !ok:
		return proto.Result{Err: "no newer release to install"}
	case tag != latest:
		return proto.Result{Err: staleTag}
	}
	return s.installUpdate(tag)
}

// closeUpdate closes the offer of the release the person saw, only while
// it is still the latest; a newer one opens it again. The row of a restart
// that does not come stays: it is the only word of a service that waits to
// stop.
func (s *Service) closeUpdate(tag string) proto.Result {
	s.updateMu.Lock()
	stuck := s.stuckLocked()
	s.updateMu.Unlock()
	if stuck != "" {
		return proto.Result{Err: restartFix(stuck, s.Binary)}
	}
	latest, ok := s.newer()
	switch {
	case !ok:
		return proto.Result{Err: "no newer release to close"}
	case tag != latest:
		return proto.Result{Err: staleTag}
	}
	s.updateMu.Lock()
	s.updates.Closed = tag
	s.saveUpdates()
	s.updateMu.Unlock()
	return proto.Result{Ran: [][]string{{"close", doors.Update}}}
}

// hostUpdate is a person's key press on this host's row: it looks for the
// latest release now, under mode off too, and installs it when it is newer.
func (s *Service) hostUpdate(a proto.Act) proto.Result {
	if a.Confirm != "y" {
		return proto.Result{Err: "needs a typed y"}
	}
	if !s.updating() {
		return proto.Result{Err: notARelease}
	}
	tag, err := s.lookNow()
	if err != nil {
		return proto.Result{Err: err.Error()}
	}
	if release.IsPre(tag) || release.Compare(s.Binary, tag) >= 0 {
		return proto.Result{Skipped: []string{tag + " is the latest release"}}
	}
	return s.installUpdate(tag)
}

// lookNow finds the latest release. Under mode off it keeps nothing: the
// service never looks again by itself.
func (s *Service) lookNow() (string, error) {
	if s.UpdateMode == config.UpdateOff {
		tag, err := s.Update.Latest()
		if err == nil && !release.IsRelease(tag) {
			err = fmt.Errorf("the latest release %q is not a release tag", tag)
		}
		return tag, err
	}
	if s.currentUpdateView().Mode == "" {
		s.loadUpdate() // the view keeps its mode and release
	}
	s.checkUpdate(s.now())
	v := s.currentUpdateView()
	if v.Err != "" {
		return "", errors.New(v.Err)
	}
	return v.Latest, nil
}

// notARelease is why a build from a checkout does not update itself.
const notARelease = "this matchblox is not a release build, so it does not update itself"

// installUpdate is a person's install of tag over this binary. It waits
// for a clone, which the restart would cut off; a stoker run goes on
// after the restart, so it does not hold the install. A good install
// changes the binary on disk, and the watch of it runs the new one.
func (s *Service) installUpdate(tag string) proto.Result {
	if !s.installing.CompareAndSwap(false, true) {
		return s.notAgain()
	}
	s.mu.Lock()
	repo, cloning := s.cloneRunsLocked() // no clone starts past the flag
	s.mu.Unlock()
	if cloning {
		s.installing.Store(false)
		return proto.Result{Err: "a clone of " + repo + " runs. Update when it ends."}
	}
	return s.install(tag, false)
}

// notAgain is the answer to an install while another runs, or after a
// good one: the service is about to start again on it, or the restart is
// overdue and the person has to open matchblox again.
func (s *Service) notAgain() proto.Result {
	s.updateMu.Lock()
	done, stuck := s.installed, s.stuckLocked()
	s.updateMu.Unlock()
	if stuck != "" {
		return proto.Result{Err: restartFix(stuck, s.Binary)}
	}
	if done != "" {
		return proto.Result{Skipped: []string{done + " is installed: matchblox starts again in a moment"}}
	}
	return proto.Result{Skipped: []string{"already updating"}}
}

// autoInstall is mode auto's install: it looks at the work in flight
// again once it holds the flag, which no new clone passes, and waits when
// work started since it looked.
func (s *Service) autoInstall(tag string) bool {
	if !s.installing.CompareAndSwap(false, true) {
		return false
	}
	if why := s.busy(); why != "" {
		s.installing.Store(false)
		s.setWaiting(why)
		return false
	}
	s.install(tag, true)
	return true
}

// install runs with the installing flag held. It lets go of the flag
// unless the install worked: then the service stops for the new binary,
// and nothing new may start before it does. proven is auto's install: no
// person is there, so it needs a checked provenance.
func (s *Service) install(tag string, proven bool) (res proto.Result) {
	defer func() {
		if len(res.Ran) == 0 {
			s.installing.Store(false)
		}
	}()
	if !s.updating() {
		return proto.Result{Err: notARelease}
	}
	if cmd := s.Update.Managed(); cmd != "" {
		return proto.Result{Err: managedProblem + " Run: " + cmd}
	}
	run := s.Update.Install
	if proven {
		run = s.Update.InstallProven
	}
	proof, err := run(tag)
	rec := state.UpdateRecord{From: s.Binary, To: tag, At: s.now(), Checksum: proof.Checksum, Provenance: proof.Provenance}
	if err != nil {
		rec.Err = err.Error()
	}
	s.updateMu.Lock()
	s.updates.Last = &rec
	if err == nil {
		s.installed = tag
	}
	s.saveUpdates()
	v := s.currentUpdateView()
	last := rec
	v.Last, v.Restart = &last, ""
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

// checkFailFor is how long a run of failed looks lasts before a person
// is told: one failed look on a laptop that is offline is no news. The run
// starts at its first failed look and ends at a look that works.
const checkFailFor = 3 * 24 * time.Hour

// updateCheckDoor says the looks have failed for days, and how to see why.
// It has no action; a person who closes it closes this run of failures.
func (s *Service) updateCheckDoor() (doors.Door, bool) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	return s.checkDoorLocked()
}

// checkDoorLocked is updateCheckDoor. Hold updateMu.
func (s *Service) checkDoorLocked() (doors.Door, bool) {
	if !s.updating() || s.UpdateMode == config.UpdateOff {
		return doors.Door{}, false
	}
	f, v := s.updates, s.currentUpdateView()
	if v.Err == "" || f.Failed.IsZero() || s.now().Sub(f.Failed) < checkFailFor || f.CheckClosed.Equal(f.Failed) {
		return doors.Door{}, false
	}
	why := fmt.Sprintf("Every look since %s failed: %s. Run: matchblox update.", f.Failed.Local().Format("Jan 2 at 15:04"), strings.TrimSuffix(v.Err, "."))
	return doors.Door{ID: doors.UpdateCheck, Title: "Release check fails", Why: why, Preview: "matchblox update"}, true
}

// closeUpdateCheck closes the row of this run of failed looks: a look
// that works ends the run, and three days of failures open it again.
func (s *Service) closeUpdateCheck() proto.Result {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	if _, ok := s.checkDoorLocked(); !ok {
		return proto.Result{Err: "the release check works again"}
	}
	s.updates.CheckClosed = s.updates.Failed
	s.saveUpdates()
	return proto.Result{Ran: [][]string{{"close", doors.UpdateCheck}}}
}
