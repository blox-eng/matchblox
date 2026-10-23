//go:build darwin || freebsd

package gitscan

import "golang.org/x/sys/unix"

// readMounts is the host's mount table, empty on error. MNT_NOWAIT reads
// the kernel's cache and never touches a dead server.
func readMounts() mountTable {
	t := mountTable{}
	n, err := unix.Getfsstat(nil, unix.MNT_NOWAIT)
	if err != nil || n <= 0 {
		return t
	}
	buf := make([]unix.Statfs_t, n)
	n, err = unix.Getfsstat(buf, unix.MNT_NOWAIT)
	if err != nil {
		return t
	}
	for _, s := range buf[:n] {
		t[unix.ByteSliceToString(s.Mntonname[:])] = unix.ByteSliceToString(s.Fstypename[:])
	}
	return t
}
