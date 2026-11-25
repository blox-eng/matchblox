//go:build unix

package app

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// ownSession runs cmd in a session of its own, with no controlling
// terminal: nothing it runs can read or draw on the console's.
func ownSession(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

// hangUp ends cmd's session, which it leads (a pty's child).
func hangUp(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGHUP) }

// killGroup kills cmd's session.
func killGroup(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }

// pollable is f, a pty's master, handed to Go's poller, so its Close ends
// a Write the client never takes. pty's own File is blocking: Close
// cannot end a write that waits on it.
func pollable(f *os.File) (*os.File, error) {
	defer f.Close()
	fd, err := unix.FcntlInt(f.Fd(), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), f.Name()), nil
}

// ptySize sizes f, a pty's master, as pty.Setsize does, but without
// f.Fd, which would make f blocking again.
func ptySize(f *os.File, w, h int) error {
	c, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var sizeErr error
	if err := c.Control(func(fd uintptr) {
		sizeErr = unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(w), Row: uint16(h)}) //nolint:gosec // a region's size
	}); err != nil {
		return err
	}
	return sizeErr
}
