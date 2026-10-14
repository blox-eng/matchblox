package panes

import (
	"slices"

	"github.com/blox-eng/matchblox/internal/sample"
)

// RenameSteps name the session in pane: a Claude Code agent's /rename
// first (two steps, the text and Enter), then the window, then its tmux
// session when the agent is that session's one window.
func RenameSteps(pane, agent, name string, one bool) [][]string {
	var steps [][]string
	if agent == "claude" {
		steps = Send(pane, "/rename "+name)
	}
	tmux, id := sample.Tmux(pane)
	steps = append(steps, slices.Concat(tmux, []string{"rename-window", "-t", id, name}))
	if one {
		steps = append(steps, slices.Concat(tmux, []string{"rename-session", "-t", id, name}))
	}
	return steps
}
