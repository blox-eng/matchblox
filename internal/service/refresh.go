package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// refreshRun is a record refresh in progress: the pane waits for its
// agent's task record. skip is closed by the builder's early switch.
type refreshRun struct {
	info proto.Refresh
	skip chan struct{}
}

// switching reports whether pane is in a record refresh. The guards ask it
// first and refuse at once: they never wait on the pane's hold.
func (s *Service) switching(pane string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshes[pane] != nil
}

// switchingErr is the refusal of other input to a switching pane, or nil.
// own is the refresh that types its own request, which passes; nil for
// any other input.
func (s *Service) switchingErr(pane string, own *refreshRun) error {
	s.mu.Lock()
	r := s.refreshes[pane]
	s.mu.Unlock()
	if r == nil || r == own {
		return nil
	}
	return errors.New(r.info.Name + " switches agents now: send again after")
}

// startRefresh marks x's pane as switching until the returned func runs,
// which removes the mark and tells the consoles. It returns nil, and marks
// nothing, when the pane is switching already.
func (s *Service) startRefresh(x sample.Session, until time.Time) (*refreshRun, func()) {
	r := &refreshRun{
		info: proto.Refresh{Pane: x.Pane, Name: x.Name, Agent: x.Agent, Since: s.now(), Until: until},
		skip: make(chan struct{}),
	}
	s.mu.Lock()
	if s.refreshes[x.Pane] != nil {
		s.mu.Unlock()
		return nil, nil
	}
	if s.refreshes == nil {
		s.refreshes = map[string]*refreshRun{}
	}
	s.refreshes[x.Pane] = r
	s.mu.Unlock()
	s.broadcast()
	return r, func() {
		s.mu.Lock()
		if s.refreshes[x.Pane] == r {
			delete(s.refreshes, x.Pane)
		}
		s.mu.Unlock()
		s.broadcast()
	}
}

// refreshList is the refreshes in progress, by pane, for the snapshot of
// each console. s.mu is held.
func (s *Service) refreshList() []proto.Refresh {
	var out []proto.Refresh
	for _, r := range s.refreshes {
		out = append(out, r.info)
	}
	slices.SortFunc(out, func(a, b proto.Refresh) int { return strings.Compare(a.Pane, b.Pane) })
	return out
}

// refreshLimit is how long a switch waits for the record, from the request.
var refreshLimit = 3 * time.Minute

// refreshPoll is how often the wait reads the session.
var refreshPoll = 250 * time.Millisecond

// refreshEnd is how the wait of a record refresh ended.
type refreshEnd int

const (
	refreshNotRun    refreshEnd = iota // no request went
	refreshTurnEnded                   // the request's turn ended: a turn after it is the builder's
	refreshCut                         // the limit or esc ended the wait: the request's turn may still run
)

// refreshRecord asks x, the idle agent a switch replaces, to write its task
// record, and waits for that turn to end, up to refreshLimit. The pane is
// marked as switching for the wait. note is the capture's word on the
// record: written for this switch, or not refreshed and why. It is "" for
// a switch with no handoff, which captures nothing. err refuses the switch.
func (s *Service) refreshRecord(ctx context.Context, x sample.Session, n proto.SwitchAgent, word string) (note string, end refreshEnd, err error) {
	if n.Handoff == "" {
		return "", refreshNotRun, nil
	}
	record := s.managedRecord(x)
	why := ""
	switch {
	case word != "":
		why = x.Name + " " + word
	case x.Stopped():
		why = "out of usage until " + x.Out.Local().Format("15:04")
	case x.Agent != "claude" && x.Agent != "codex" || x.Transcript == "":
		why = x.Agent + " gives no turn end"
	case record == "":
		why = "no managed task record"
	case n.NoRefresh:
		why = "skipped with --no-refresh"
	}
	if why != "" {
		return "not refreshed: " + why, refreshNotRun, nil
	}
	sent := s.now()
	run, release := s.startRefresh(x, sent.Add(refreshLimit))
	if run == nil {
		return "", refreshNotRun, errors.New("switching already")
	}
	defer release()
	limit := time.NewTimer(refreshLimit)
	defer limit.Stop()
	before := modTime(record)
	res, _, _, _ := s.typeLine(ctx, x, refreshPrompt(record), refreshLead, refreshLead, run)
	if ctx.Err() != nil {
		return "", refreshNotRun, ctx.Err()
	}
	if res.Err != "" {
		return "not refreshed: the request did not reach " + x.Name + ": " + strings.TrimSuffix(res.Err, peerRetry), refreshNotRun, nil
	}
	tick := time.NewTicker(refreshPoll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", refreshNotRun, ctx.Err()
		case <-run.skip:
			return "not refreshed: skipped with esc", refreshCut, nil
		case <-limit.C:
			return "not refreshed: the refresh timed out after " + waitText(refreshLimit), refreshCut, nil
		case <-tick.C:
		}
		cur, ok := s.sessionAt(x.Pane)
		if !ok || cur.PID != x.PID || cur.Start != x.Start {
			return "", refreshNotRun, errors.New("the session changed while this switch read it: switch again")
		}
		if cur.TurnEnded.After(sent) && cur.Resting() && !s.waitsOn(x.Pane, cur) {
			break
		}
	}
	if modTime(record).After(before) {
		return "record written for this switch", refreshTurnEnded, nil
	}
	return "not refreshed: it ended its turn without writing the record", refreshTurnEnded, nil
}

// managedRecord is the task record of x's match, in a bundle this service
// made, or "".
func (s *Service) managedRecord(x sample.Session) string {
	if x.MatchContext == "" {
		return ""
	}
	record := filepath.Join(filepath.Dir(x.MatchContext), "handover.md")
	if rel, err := filepath.Rel(s.matchContextRoot(), record); err != nil || !filepath.IsLocal(rel) {
		return ""
	}
	return record
}

// modTime is when path was last written, or zero when it cannot be read.
func modTime(path string) time.Time {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// waitText is d as minutes and seconds: 3:00.
func waitText(d time.Duration) string {
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}
