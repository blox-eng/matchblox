package service

import (
	"math"

	"github.com/blox-eng/matchblox/internal/procfs"
)

// ttyOf is the controlling terminal of pid, from /proc/<pid>/stat.
func ttyOf(pid int) (ttyDevice, bool) {
	p, ok := (procfs.FS{Root: "/proc"}).Proc(pid)
	if !ok {
		return ttyDevice{}, false
	}
	return ttyFromNr(p.TTY)
}

// ttyFromNr maps a tty_nr to a device; 0 and anything outside 32 bits is no terminal.
func ttyFromNr(nr int) (ttyDevice, bool) {
	if nr <= 0 || int64(nr) > math.MaxUint32 {
		return ttyDevice{}, false
	}
	return ttyDev(uint32(nr)), true
}
