package service

import (
	"strings"

	"github.com/blox-eng/matchblox/internal/panes"
)

// A console's view never sizes an agent's window: each resize made a
// full-screen agent such as Codex draw its whole screen again (#351).
// Older services sized windows and marked them.

// restoreSized puts back each window a service before this one left
// sized, on the builder's tmux server and on matchblox's own: one that
// ended without its stop.
func (s *Service) restoreSized() {
	servers := []string{""}
	if s.StokerServer != "" {
		servers = append(servers, s.StokerServer)
	}
	for _, server := range servers {
		out, err := s.tmuxOut(panes.SizedWindows(server))
		if err != nil {
			continue // no server: nothing sized
		}
		for _, line := range strings.Split(out, "\n") {
			if window, mark, ok := strings.Cut(line, "\t"); ok && mark != "" {
				_, _ = s.tmuxOut(panes.RestoreWindow(server, window, mark))
			}
		}
	}
}
