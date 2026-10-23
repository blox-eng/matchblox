//go:build !windows

package setup

import (
	"os/exec"
	"syscall"
)

func ownGroup(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
