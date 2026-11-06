package service

import "github.com/blox-eng/matchblox/internal/procfs"

// ttyOf is the controlling terminal of pid, from /proc/<pid>/stat.
func ttyOf(pid int) (ttyDevice, bool) {
	p, ok := (procfs.FS{Root: "/proc"}).Proc(pid)
	if !ok || p.TTY == 0 {
		return ttyDevice{}, false
	}
	return ttyDev(p.TTY), true
}
