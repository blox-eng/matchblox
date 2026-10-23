package gitscan

import "os"

// readMounts is the host's mount table, empty when unreadable.
func readMounts() mountTable {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return mountTable{}
	}
	defer f.Close()
	return parseMountinfo(f)
}
