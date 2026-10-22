//go:build unix

package gitscan

import (
	"os"
	"syscall"
)

// statDev is the device path is on, not following a symlink.
func statDev(path string) (uint64, bool) {
	fi, err := os.Lstat(path)
	if err != nil {
		return 0, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true //nolint:unconvert // int32 on darwin
}
