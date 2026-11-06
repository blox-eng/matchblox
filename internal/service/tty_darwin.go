//go:build !ios

package service

import "golang.org/x/sys/unix"

// ttyOf is the controlling terminal of pid, from the kernel's process
// table: e_tdev of its kinfo_proc, NODEV (-1) for none.
func ttyOf(pid int) (ttyDevice, bool) {
	k, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || k.Eproc.Tdev == -1 {
		return ttyDevice{}, false
	}
	dev := uint64(uint32(k.Eproc.Tdev)) //nolint:gosec // dev_t is 32 bits on darwin
	return ttyDevice{major: unix.Major(dev), minor: unix.Minor(dev)}, true
}
