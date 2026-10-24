//go:build unix

package clone

import (
	"errors"
	"os/exec"
	"syscall"
)

// ownGroup runs cmd in a process group of its own, and a stop kills the
// whole group: git's children (ssh, remote helpers) go with it.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		return nil
	}
}
