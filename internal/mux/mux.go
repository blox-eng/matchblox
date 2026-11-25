// Package mux is what matchblox does to a pane, whatever runs the panes:
// list, start, rename, kill, respawn, type a line, press keys, paste a
// text, read screens and tell copy mode. tmux implements it; muxtest has
// a fake for tests.
package mux

import "time"

// Start is one session to open. Env values may sit on a tmux -e word.
// This package never writes them into an error or a log.
type Start struct {
	Name, Dir string
	Env       []string
	Argv      []string
	Detached  bool
	EmptyExit bool // stoker: remain-on-exit / exit-empty as today
}

// Mux is one multiplexer. Type, Keys, Start, Rename, Kill and Respawn give
// steps, not runs: the service runs each step behind its guard.
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
	// TypedAt is the last key of a person in pane: the latest key of a
	// terminal that shows it. Zero when no terminal shows it, or the
	// multiplexer cannot tell.
	TypedAt(pane string) time.Time
	// Load puts text into the paste buffer name on the server of pane. The
	// text goes on stdin: no argv word and no shell holds it.
	Load(pane, name, text string) error
	// NoBracketedPaste tells if the program in pane said it takes no
	// bracketed paste: tmux would send each line of a paste with its own
	// Enter. False when the multiplexer cannot tell.
	NoBracketedPaste(pane string) bool
	// Paste is the step that pastes buffer name into pane as one bracketed
	// paste, then deletes the buffer.
	Paste(pane, name string) []string
	// Drop deletes buffer name on the server of pane: a paste that did not
	// run leaves no text behind.
	Drop(pane, name string) error
	// Mark sets, or with on false takes off, the mark of a pane whose
	// agent the service samples: the flip key acts only on a marked pane.
	Mark(pane string, on bool) error
	// List is every pane on this multiplexer. A machine with no tmux, or
	// a tmux that exits, is an empty list.
	List() ([]Pane, error)
	// Start is the steps that open a session. The service runs them.
	Start(Start) [][]string
	// Rename is the steps that name the window in pane. The agent is not
	// known here, so this does not type the agent's own rename, and it
	// does not rename the session: that waits until the caller knows the
	// session has one window.
	Rename(pane, name string) [][]string
	// Kill is the steps that end pane, then TERM pid when pid is greater
	// than zero. The TERM names that pid and no other.
	Kill(pane string, pid int) [][]string
	// KillSession is the step that ends the session name on this server.
	KillSession(name string) []string
	// Respawn is the step that starts the command again in pane. The pane
	// id stays: this does not open a session.
	Respawn(pane, dir string, env []string, argv []string) [][]string
}
