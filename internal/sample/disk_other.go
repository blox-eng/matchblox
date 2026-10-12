//go:build !unix

package sample

// statfs reads no disks where there is no statfs.
func statfs(string) (Disk, uint64, bool) { return Disk{}, 0, false }
