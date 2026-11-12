package service

import "github.com/blox-eng/matchblox/internal/sample"

// markMatches marks each agent pane the sampler sees, and takes the mark
// off a pane whose agent left: the flip key (⌥s) acts only on a marked
// pane, and every other pane gets the key as typed. A stoker's pane is no
// match. Only the sample loop calls it, so flipMarked needs no lock; a pane
// is marked once, not on every sample.
func (s *Service) markMatches(ss []sample.Session) {
	if !s.MarkMatches {
		return
	}
	want := map[string]bool{}
	for _, x := range ss {
		if x.Pane != "" && !s.onStokerServer(x.Pane) {
			want[x.Pane] = true
		}
	}
	if s.flipMarked == nil {
		s.flipMarked = map[string]bool{}
	}
	mx := s.mux()
	for p := range want {
		if !s.flipMarked[p] && mx.Mark(p, true) == nil {
			s.flipMarked[p] = true
		}
	}
	for p := range s.flipMarked {
		if !want[p] {
			_ = mx.Mark(p, false) // a pane that closed has no mark to take off
			delete(s.flipMarked, p)
		}
	}
}
