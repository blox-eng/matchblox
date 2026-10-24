package service

import "github.com/blox-eng/matchblox/internal/state"

// loadUsed reads the record of where sessions started, so the first
// snapshot carries it. It needs no scan consent: it is matchblox's own use.
func (s *Service) loadUsed() {
	var u state.Used
	if s.UsedPath != "" {
		u = state.ReadUsed(s.UsedPath)
	}
	s.mu.Lock()
	s.used = u
	s.usedGen++
	s.mu.Unlock()
	s.broadcast()
}

// addUsed records that sessions started in paths, now.
func (s *Service) addUsed(paths ...string) {
	s.usedMu.Lock()
	s.mu.Lock()
	u := s.used.Add(s.now(), paths...)
	s.used = u
	s.usedGen++
	s.mu.Unlock()
	if s.UsedPath != "" {
		_ = state.WriteUsed(s.UsedPath, u) // best effort: the list only ranks
	}
	s.usedMu.Unlock()
	s.broadcast()
}
