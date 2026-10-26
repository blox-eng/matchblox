//go:build unix

package app

import (
	"os/exec"
	"syscall"
)

// ownSession runs cmd in a session of its own, with no controlling
// terminal: nothing it runs can read or draw on the console's.
func ownSession(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
