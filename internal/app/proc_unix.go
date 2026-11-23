//go:build unix

package app

import (
	"os/exec"
	"syscall"
)

// ownSession runs cmd in a session of its own, with no controlling
// terminal: nothing it runs can read or draw on the console's.
func ownSession(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

// hangUp ends cmd's session, which it leads (a pty's child).
func hangUp(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGHUP) }

// killGroup kills cmd's session.
func killGroup(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
