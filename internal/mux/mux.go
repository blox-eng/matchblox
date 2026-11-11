// Package mux is what chains and sending do to a pane, whatever runs the
// panes: type a line, press keys, read screens and tell copy mode. tmux
// implements it; muxtest has a fake for tests.
package mux

// Mux is one multiplexer. Type and Keys give steps, not runs: the service
// runs each step behind its guard, as it runs a stoker step.
type Mux interface {
	// Type is the step that types text into pane as it is.
	Type(pane, text string) []string
	// Keys is the step that presses keys (key names) in pane.
	Keys(pane string, keys ...string) []string
	// Screens reads the visible text of panes; a pane that is gone is
	// missing.
	Screens(panes []string) map[string]string
	// InMode tells if pane shows copy mode or another mode, where keys
	// move a cursor instead of reaching the agent.
	InMode(pane string) (bool, error)
}
