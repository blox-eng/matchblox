package panes

import "github.com/blox-eng/matchblox/internal/mux"

// RenameSteps name the session in pane: a Claude Code agent's /rename
// first (two steps, the text and Enter), then the window, then its tmux
// session when the agent is that session's one window.
func RenameSteps(pane, agent, name string, one bool) [][]string {
	var steps [][]string
	if agent == "claude" {
		steps = Send(pane, "/rename "+name)
	}
	m := mux.Tmux{}
	steps = append(steps, m.Rename(pane, name)...)
	if one {
		steps = append(steps, m.RenameSession(pane, name))
	}
	return steps
}
