//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// detach starts the service in its own process group with no console.
func detach(c *exec.Cmd) {
	const createNewProcessGroup, detachedProcess = 0x00000200, 0x00000008
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | detachedProcess}
}
