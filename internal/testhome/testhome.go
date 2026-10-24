// Package testhome keeps a test binary off the builder's own folders.
package testhome

import (
	"fmt"
	"os"
	"testing"
)

// Run runs the tests with HOME at a new empty folder and without the
// agents' config folder variables, so code that falls back to the home
// folder (os.UserHomeDir) or to an agent's default folder writes nothing
// of the builder's: not ~/.matchblox, ~/.claude, ~/.codex or ~/.cursor.
// It returns the exit code for os.Exit.
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
	return m.Run()
}
