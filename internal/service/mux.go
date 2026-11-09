package service

import "github.com/blox-eng/matchblox/internal/mux"

// mux is the multiplexer chains and sends reach panes through.
func (s *Service) mux() mux.Mux {
	if s.Mux != nil {
		return s.Mux
	}
	return mux.Tmux{Run: s.tmuxOut, Capture: s.Capture}
}
