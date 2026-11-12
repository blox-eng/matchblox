// Package filestamp tells if a file is still the one a process saw: a
// binary installed over, or a link repointed to a new version.
package filestamp

import "os"

// Of is the file at p, nil when p is "" or cannot be read.
func Of(p string) os.FileInfo {
	if p == "" {
		return nil
	}
	fi, _ := os.Stat(p)
	return fi
}

// Changed tells if the file at p is no longer was: removed, replaced, or
// written over. A nil was never changes.
func Changed(was os.FileInfo, p string) bool {
	if was == nil {
		return false
	}
	now := Of(p)
	return now == nil || !os.SameFile(was, now) || now.Size() != was.Size() || !now.ModTime().Equal(was.ModTime())
}
