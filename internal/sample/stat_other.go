//go:build !unix

package sample

import "os"

// inodeOf is 0 where the system gives no inode.
func inodeOf(os.FileInfo) uint64 { return 0 }
