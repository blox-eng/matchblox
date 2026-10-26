//go:build unix

package clone

import (
	"errors"
	"os/exec"
	"syscall"
)

// ownGroup runs cmd in a session of its own, with no controlling
// terminal, so ssh cannot ask on one. The session's process group is
// cmd's pid, and a stop kills the whole group: git's children (ssh,
// remote helpers) go with it. Setsid alone: with Setpgid too, the start
// fails.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		return nil
	}
}

// Alive tells if a process with this pid runs. A process we may not signal
// runs all the same.
func Alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || !errors.Is(err, syscall.ESRCH)
}
