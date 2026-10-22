//go:build !unix

package gitscan

// statDev knows no devices here: the walk crosses mounts.
func statDev(string) (uint64, bool) { return 0, false }
