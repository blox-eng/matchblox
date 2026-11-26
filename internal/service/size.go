package service

import "github.com/blox-eng/matchblox/internal/mux"

// A console's view never sizes an agent's window: each resize made a
// full-screen agent such as Codex draw its whole screen again (#351).
// Older services sized windows and marked them.

// restoreSized puts back each window a service before this one left
// sized, on the builder's multiplexer and on matchblox's own: one that
// ended without its stop.
func (s *Service) restoreSized() {
	servers := []mux.Mux{s.mux()}
	if s.StokerServer != "" {
		servers = append(servers, s.tmuxOn(s.StokerServer))
	}
	for _, m := range servers {
		windows, err := m.SizedWindows()
		if err != nil {
			continue // no server: nothing sized
		}
		for _, w := range windows {
			_ = m.RestoreWindow(w.Window, w.Mark) // best effort: the next start tries again
		}
	}
}
