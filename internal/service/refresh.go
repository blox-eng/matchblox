package service

import (
	"errors"
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
func (s *Service) switchingErr(pane string) error {
	s.mu.Lock()
	r := s.refreshes[pane]
	s.mu.Unlock()
	if r == nil {
		return nil
	}
	return errors.New(r.info.Name + " switches agents now: send again after")
}

// startRefresh marks x's pane as switching until the returned func runs,
// which removes the mark and tells the consoles.
func (s *Service) startRefresh(x sample.Session, until time.Time) (*refreshRun, func()) {
	r := &refreshRun{
		info: proto.Refresh{Pane: x.Pane, Name: x.Name, Agent: x.Agent, Since: s.now(), Until: until},
		skip: make(chan struct{}),
	}
	s.mu.Lock()
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
