// Package testhome keeps a test binary off the builder's own folders and
// tmux server.
package testhome

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// Run runs the tests with HOME at a new empty folder and without the
// agents' config folder variables, so code that falls back to the home
// folder (os.UserHomeDir) or to an agent's default folder writes nothing
// of the builder's: not ~/.matchblox, ~/.claude, ~/.codex or ~/.cursor.
// It also clears $TMUX and $TMUX_PANE, so a tmux that a test runs with no
// socket does not reach the builder's server. It returns the exit code for os.Exit.
func Run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "matchblox-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "testhome:", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }() // a temp folder: what is left the system clears
	if err := os.Setenv("HOME", dir); err != nil {
		fmt.Fprintln(os.Stderr, "testhome:", err)
		return 1
	}
	for _, v := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME", "CURSOR_CONFIG_DIR", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME"} {
		_ = os.Unsetenv(v)
	}
	// A tmux with no socket goes to the server $TMUX names: the builder's,
	// with every agent in it (#215). No test or its subprocess inherits it.
	BuildersTmux = socketOf(os.Getenv("TMUX"))
	_ = os.Unsetenv("TMUX")
	_ = os.Unsetenv("TMUX_PANE")
	return m.Run()
}

// BuildersTmux is the socket of the tmux server the builder ran the tests
// in, from the $TMUX that Run clears, or "" outside tmux. Only a test that
// reads the builder's session names uses it, with tmux -S.
var BuildersTmux string

// socketOf is the socket path in a $TMUX value: socket,pid,session. The
// path itself can hold a comma.
func socketOf(tmux string) string {
	i := strings.LastIndex(tmux, ",")
	if i < 0 {
		return ""
	}
	j := strings.LastIndex(tmux[:i], ",")
	if j < 0 {
		return ""
	}
	return tmux[:j]
}
