//go:build unix && !ios

package service

import "golang.org/x/sys/unix"

// devOf is the device a terminal path names.
func devOf(path string) (ttyDevice, bool) {
	var st unix.Stat_t
	if unix.Stat(path, &st) != nil {
		return ttyDevice{}, false
	}
	dev := uint64(st.Rdev) //nolint:gosec,unconvert // Rdev is int32 on darwin
	return ttyDevice{major: unix.Major(dev), minor: unix.Minor(dev)}, true
}
