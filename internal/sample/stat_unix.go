//go:build unix

package sample

import (
	"os"
	"syscall"
)

// inodeOf is the file's inode: a rename over it makes a new one, even in
// the clock tick of the last write.
func inodeOf(st os.FileInfo) uint64 {
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		return uint64(sys.Ino) //nolint:unconvert // Ino is not uint64 on every unix
	}
	return 0
}
