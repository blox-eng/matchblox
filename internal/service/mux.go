package service

import "github.com/blox-eng/matchblox/internal/mux"

// mux is the multiplexer chains and sends reach panes through.
func (s *Service) mux() mux.Mux {
	if s.Mux != nil {
		return s.Mux
	}
	return s.tmuxOn("")
}

// stokerMux is the multiplexer of the stoker's own server (-L), whatever
// s.Mux is: its sessions are never the builder's.
func (s *Service) stokerMux() mux.Tmux {
	server := s.StokerServer
	if server == "" {
		server = "matchblox"
	}
	return s.tmuxOn(server)
}

// tmuxOn is tmux on server ("": the builder's). A run that tests set goes
// through TmuxOut; with none, mux runs tmux itself, and bounds each read.
func (s *Service) tmuxOn(server string) mux.Tmux {
	return mux.Tmux{Server: server, Run: s.TmuxOut, RunIn: s.TmuxIn, Capture: s.Capture}
}
