//go:build !linux && !darwin && !freebsd

package gitscan

// readMounts has no table here: no folder is skipped by mount.
func readMounts() mountTable { return mountTable{} }
