package guard

import (
	"bytes"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// openPTY opens a terminal pair: the guard keeps master, the agent runs on
// slave.
func openPTY() (master, slave *os.File, err error) {
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	master = os.NewFile(uintptr(fd), "/dev/ptmx") //nolint:gosec // a file descriptor fits
	name := make([]byte, 128)                     // the size TIOCPTYGNAME writes into
	for _, req := range []struct {
		code uint
		arg  uintptr
	}{{unix.TIOCPTYGRANT, 0}, {unix.TIOCPTYUNLK, 0}, {unix.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))}} {
		if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req.code), req.arg); errno != 0 { //nolint:gosec // a file descriptor fits
			_ = master.Close()
			return nil, nil, errno
		}
	}
	end := bytes.IndexByte(name, 0)
	if end < 0 {
		end = len(name)
	}
	slave, err = os.OpenFile(string(name[:end]), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	return master, slave, nil
}
