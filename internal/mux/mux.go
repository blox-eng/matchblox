// Package mux is what matchblox does to a pane, whatever runs the panes:
// list, start, rename, kill, respawn, type a line, press keys, paste a
// text, read screens and tell copy mode. tmux implements it; muxtest has
// a fake for tests.
package mux

import (
	"context"
	"time"
)

// Start is one session to open. Env values may sit on a tmux -e word.
// This package never writes them into an error or a log.
type Start struct {
	Name, Dir string
	Env       []string
	Argv      []string
	Detached  bool
	EmptyExit bool // stoker: remain-on-exit / exit-empty as today
}

// Mux is one multiplexer. Type, Keys, Start, Rename, Kill, Respawn, Focus and Attach give
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
	// ListStrict is List, except that a multiplexer that fails for any
	// reason but "no server" or "not installed" is an error, not an empty
	// list. Placement of a caller uses it.
	ListStrict() ([]Pane, error)
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
	// HasSession is the step that asks if the session name is on this
	// server: it fails when it is not.
	HasSession(name string) []string
	// Respawn is the step that starts the command again in pane. The pane
	// id stays: this does not open a session.
	Respawn(pane, dir string, env []string, argv []string) [][]string
	// Split opens a pane next to target running argv and returns its id.
	// It runs: the id exists only after the multiplexer made the pane.
	Split(target string, argv []string) (pane string, err error)
	// Beside is Split with the new pane to the right of target, without the
	// keys, and percent of the window's width.
	Beside(target string, percent int, argv []string) (pane string, err error)
	// SwitchClient is the step that moves the client that asked to pane.
	SwitchClient(pane string) []string
	// ViewCmd is the name a pane shows while it runs a nested client of this
	// multiplexer (a view of an agent pane).
	ViewCmd() string
	// Focus is the steps that bring pane to the front: its window, then
	// the pane. Attach is a separate step.
	Focus(pane string) [][]string
	// LastClient is the tty and the creation time of the client of session
	// that was active last. No session, or a multiplexer that errors, is
	// "" and the zero time with no error: the guard is not a wall.
	LastClient(session string) (tty string, created time.Time, err error)
	// Attach is the step that gives the terminal to session.
	Attach(session string) []string
	// Caller is the pane that this process runs in, "" when it runs in
	// none.
	Caller() string
	// ListBuffers is the paste buffers of this server, newest first. A
	// server that is not there is an error. A read ends with ctx.
	ListBuffers(ctx context.Context) ([]Buffer, error)
	// ShowBuffer is the text of buffer name. A buffer that is gone is an
	// error. A read ends with ctx.
	ShowBuffer(ctx context.Context, name string) (string, error)
	// Notice shows text for a few seconds to each client of a person on
	// this server, not to a control client. It says nothing when the
	// server has no clients or does not answer.
	Notice(ctx context.Context, text string)
	// SizedWindows is each window of this server that an older service
	// sized: its id and its mark. A server that is not there is an error.
	SizedWindows() ([]SizedWindow, error)
	// RestoreWindow puts back the window-size of a window from its mark,
	// then drops the mark.
	RestoreWindow(window, mark string) error
}

// Buffer is one line of the paste buffer list. The three fields are as
// the multiplexer prints them, so two reads of one buffer are equal.
type Buffer struct{ Name, Created, Size string }

// SizedWindow is a window an older service sized, with its mark: the
// window-size it had before ("-": none) and its size then, "latest 120x40".
type SizedWindow struct{ Window, Mark string }
