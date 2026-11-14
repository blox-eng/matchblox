package guard

import (
	"os"
	"strconv"

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
	unlock := 0
	if err = unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, unlock); err == nil {
		var n int
		if n, err = unix.IoctlGetInt(fd, unix.TIOCGPTN); err == nil {
			slave, err = os.OpenFile("/dev/pts/"+strconv.Itoa(n), os.O_RDWR|unix.O_NOCTTY, 0)
		}
	}
	if err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	return master, slave, nil
}
