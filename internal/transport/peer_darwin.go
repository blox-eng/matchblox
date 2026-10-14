package transport

import (
	"net"

	"golang.org/x/sys/unix"
)

// PeerPID is the pid of the process at the other end of a unix socket, as
// the kernel saw it connect (LOCAL_PEERPID). False: not known.
func PeerPID(c net.Conn) (int, bool) {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return 0, false
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0, false
	}
	pid, perr := 0, error(nil)
	if err := raw.Control(func(fd uintptr) {
		pid, perr = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
	}); err != nil || perr != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}
