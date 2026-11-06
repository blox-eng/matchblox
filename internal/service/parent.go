//go:build !ios

package service

import (
	"github.com/shirou/gopsutil/v4/process"

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
