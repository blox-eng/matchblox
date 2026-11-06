package service

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// stokerUnit is one stoker of the family: the host stoker, or the stoker
// of one chain. Each has its store, its tmux session and its driven panes.
type stokerUnit struct {
	id    string
	link  stoker.Link // Chain is empty for the host
	store *stoker.Store
	// startedAt is when matchblox started the unit's agent; zero once it
	// is ready, or gave up on it. Service.mu guards it.
	startedAt time.Time
	// driven are the panes the unit drove, for the merge guard.
	driven drivenState
}

// stokerFamily holds the units. The host unit wraps Service.Stoker, so
// what sets that field sets the host's store.
type stokerFamily struct {
	mu     sync.Mutex
	host   stokerUnit
	chains []*stokerUnit // by id
}

// hostUnit is the unit of the host stoker. Nil: the service keeps no
// stoker.
func (s *Service) hostUnit() *stokerUnit {
	if s.Stoker == nil {
		return nil
	}
	f := &s.fam
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.host.store != s.Stoker { // written only on a change: readers hold no lock
		f.host.store = s.Stoker
	}
	return &f.host
}

// family is every unit: the host first, then the chain units by id.
func (s *Service) family() []*stokerUnit {
	var out []*stokerUnit
	if h := s.hostUnit(); h != nil {
		out = append(out, h)
	}
	s.fam.mu.Lock()
	defer s.fam.mu.Unlock()
	return append(out, s.fam.chains...)
}

// unitByID is the unit of a stoker id. Nil: none is loaded.
func (s *Service) unitByID(id string) *stokerUnit {
	if id == stoker.HostID {
		return s.hostUnit()
	}
	s.fam.mu.Lock()
	defer s.fam.mu.Unlock()
	for _, u := range s.fam.chains {
		if u.id == id {
			return u
		}
	}
	return nil
}

// unitOfChain is the unit of a chain's stoker. Nil: the chain has none.
func (s *Service) unitOfChain(chainID string) *stokerUnit {
	if chainID == "" {
		return nil
	}
	s.fam.mu.Lock()
	defer s.fam.mu.Unlock()
	for _, u := range s.fam.chains {
		if u.link.Chain == chainID {
			return u
		}
	}
	return nil
}

// unitOf is the unit whose tmux session the act was typed in. stokerPane
// tells if the pane is on the stoker's tmux server at all: the server
// decides, not the agent list, and the builder's pane of the same id never
// is. A stoker pane with no unit is an unknown stoker session, and fails
// closed: a session the sample does not name yet, while chain units run,
// or a session no loaded unit runs.
func (s *Service) unitOf(a proto.Act) (u *stokerUnit, stokerPane bool) {
	if s.StokerServer == "" || a.From == "" {
		return nil, false
	}
	if server, _ := sample.SplitPane(a.From); server != s.StokerServer {
		return nil, false
	}
	name, known := s.tmuxSessionOf(a.From)
	if !known {
		s.fam.mu.Lock()
		chains := len(s.fam.chains)
		s.fam.mu.Unlock()
		if chains > 0 {
			return nil, true
		}
		return s.hostUnit(), true
	}
	id, ok := stoker.IDOfSession(name)
	if !ok {
		return nil, true
	}
	return s.unitByID(id), true
}

// tmuxSessionOf is the tmux session of a pane, as the last sample listed
// it. False: the sample names no session for the pane.
func (s *Service) tmuxSessionOf(pane string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := ""
	for _, p := range s.cur.Panes {
		if p.ID == pane {
			name, _, _ = strings.Cut(p.Target, ":")
		}
	}
	if x, ok := s.sessionAtLocked(pane); ok && name == "" {
		name = x.TmuxSession()
	}
	return name, name != ""
}

// loadFamily reads the chain units from stokers/ in the host's state dir:
// each store and its link.json. A unit already loaded keeps its memory. A
// unit that does not read is left out, and its error returned.
func (s *Service) loadFamily() error {
	h := s.hostUnit()
	if h == nil {
		return nil
	}
	state := h.store.Dir()
	ids, err := stoker.FamilyIDs(state)
	if err != nil {
		return err
	}
	var errs []error
	units := make([]*stokerUnit, 0, len(ids))
	for _, id := range ids {
		if u := s.unitByID(id); u != nil {
			units = append(units, u)
			continue
		}
		dir := stoker.FamilyDir(state, id)
		st, err := stoker.Open(dir)
		if err != nil {
			errs = append(errs, fmt.Errorf("stoker %s: %w", id, err))
			continue
		}
		l, err := stoker.ReadLink(dir)
		if err != nil {
			errs = append(errs, fmt.Errorf("stoker %s: %w", id, err))
			continue
		}
		units = append(units, &stokerUnit{id: id, link: l, store: st})
	}
	s.fam.mu.Lock()
	s.fam.chains = units
	s.fam.mu.Unlock()
	return errors.Join(errs...)
}
