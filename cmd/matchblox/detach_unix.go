//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

// detach puts the service in its own session: no controlling terminal, so
// closing the console's terminal does not stop it.
func detach(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
