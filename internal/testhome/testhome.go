// Package testhome keeps a test binary off the builder's own folders and
// tmux server.
package testhome

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Run runs the tests with HOME at a new empty folder and without the
// agents' config folder variables, so code that falls back to the home
// folder (os.UserHomeDir) or to an agent's default folder writes nothing
// of the builder's: not ~/.matchblox, ~/.claude, ~/.codex or ~/.cursor.
// It also clears $TMUX and $TMUX_PANE and sets $TMUX_TMPDIR to a new empty
// folder, so a tmux that a test runs with no socket reaches no server of the
// builder's. It returns the exit code for os.Exit.
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
	// A tmux with no socket goes to the server $TMUX names, or else to the
	// default server in $TMUX_TMPDIR: the builder's, with every agent in it
	// (#215). A test and its subprocesses get neither.
	tmuxDir := os.Getenv("TMUX_TMPDIR")
	if tmuxDir == "" {
		tmuxDir = "/tmp"
	}
	BuildersTmux = []string{filepath.Join(tmuxDir, fmt.Sprintf("tmux-%d", os.Getuid()), "default")}
	if sock := socketOf(os.Getenv("TMUX")); sock != "" {
		BuildersTmux = append(BuildersTmux, sock)
	}
	own, err := os.MkdirTemp("/tmp", "mbt") // short, not macOS $TMPDIR: a socket path in it must fit sun_path
	if err != nil {
		fmt.Fprintln(os.Stderr, "testhome:", err)
		return 1
	}
	defer func() { endServers(own); _ = os.RemoveAll(own) }()
	if err := os.Setenv("TMUX_TMPDIR", own); err != nil {
		fmt.Fprintln(os.Stderr, "testhome:", err)
		return 1
	}
	_ = os.Unsetenv("TMUX")
	_ = os.Unsetenv("TMUX_PANE")
	return m.Run()
}

// BuildersTmux are the sockets of the builder's tmux servers that Run
// keeps the tests off: the default one, and the one $TMUX named. Only a
// test that reads the builder's session names uses them, with tmux -S.
var BuildersTmux []string

// endServers ends each tmux server whose socket is in dir: one that a test
// left. With its socket removed, no tmux could reach it any more.
func endServers(dir string) {
	socks, _ := filepath.Glob(filepath.Join(dir, "tmux-*", "*"))
	for _, sock := range socks {
		_ = exec.Command("tmux", "-S", sock, "kill-server").Run() //nolint:gosec // a socket of this test binary's own folder
	}
}

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
