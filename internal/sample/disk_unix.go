//go:build unix

package sample

import (
	"os"
	"syscall"
)

// statfs reads the file system that holds path, and the device it is on.
func statfs(path string) (Disk, uint64, bool) {
	var fs syscall.Statfs_t
	st, err := os.Stat(path)
	if err != nil || syscall.Statfs(path, &fs) != nil {
		return Disk{}, 0, false
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return Disk{}, 0, false
	}
	bs := uint64(fs.Bsize) //nolint:gosec // a block size is positive
	total, free, avail := uint64(fs.Blocks)*bs, uint64(fs.Bfree)*bs, uint64(fs.Bavail)*bs
	return Disk{Path: path, Total: total, Used: total - free, Avail: avail}, uint64(sys.Dev), true //nolint:gosec // Dev is int32 on macOS
}
