//go:build !unix

package app

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// ownSession is a no-op where there are no sessions.
func ownSession(*exec.Cmd) {}

// hangUp ends cmd.
func hangUp(cmd *exec.Cmd) { _ = cmd.Process.Kill() }

// killGroup kills cmd.
func killGroup(cmd *exec.Cmd) { _ = cmd.Process.Kill() }

// pollable is f as it is.
func pollable(f *os.File) (*os.File, error) { return f, nil }

// ptySize sizes f, a pty's master.
func ptySize(f *os.File, w, h int) error {
	return pty.Setsize(f, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)}) //nolint:gosec // a region's size
}
