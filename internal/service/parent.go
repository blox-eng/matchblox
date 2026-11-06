//go:build !ios

package service

import (
	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/unix"

	"github.com/blox-eng/matchblox/internal/procfs"
)

// parentOf is the parent pid of pid: from /proc, else from the system.
func parentOf(pid int) (int, bool) {
	if p, ok := (procfs.FS{Root: "/proc"}).Proc(pid); ok {
		return p.PPID, true
	}
	p, err := process.NewProcess(int32(pid)) //nolint:gosec // a pid fits in int32
	if err != nil {
		return 0, false
	}
	pp, err := p.Ppid()
	if err != nil {
		return 0, false
	}
	return int(pp), true
}

// ttyOf is the controlling terminal of pid, as major and minor numbers.
func ttyOf(pid int) (major, minor uint32, ok bool) {
	p, ok := (procfs.FS{Root: "/proc"}).Proc(pid)
	if !ok || p.TTY == 0 {
		return 0, 0, false
	}
	major, minor = ttyDev(uint32(p.TTY)) //nolint:gosec // tty_nr is a 32-bit device number
	return major, minor, true
}

// devOf is the device a terminal path names, as major and minor numbers.
func devOf(path string) (major, minor uint32, ok bool) {
	var st unix.Stat_t
	if unix.Stat(path, &st) != nil {
		return 0, 0, false
	}
	return unix.Major(uint64(st.Rdev)), unix.Minor(uint64(st.Rdev)), true //nolint:unconvert // Rdev is int32 on some systems
}
