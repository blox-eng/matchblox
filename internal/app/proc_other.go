//go:build !unix

package app

import "os/exec"

// ownSession is a no-op where there are no sessions.
func ownSession(*exec.Cmd) {}

// hangUp ends cmd.
func hangUp(cmd *exec.Cmd) { _ = cmd.Process.Kill() }

// killGroup kills cmd.
func killGroup(cmd *exec.Cmd) { _ = cmd.Process.Kill() }
