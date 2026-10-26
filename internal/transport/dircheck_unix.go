//go:build unix

package transport

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// CheckDir refuses a socket directory that is a symlink, that others can
// enter, or that another user owns: on a shared /tmp another user could
// make it first and serve a fake console.
func CheckDir(dir string) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s is open to other users (mode %o); it must be 0700", dir, fi.Mode().Perm())
	}
	if !mine(fi) {
		return fmt.Errorf("%s belongs to uid %d: %w", dir, fi.Sys().(*syscall.Stat_t).Uid, ErrNotYours)
	}
	return nil
}

// ErrNotYours: another user owns the directory; only they or root can
// remove it.
var ErrNotYours = errors.New("not yours")

// Mine is true for a directory of this user that is no link, and for a
// path that does not exist yet.
func Mine(path string) bool {
	fi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	return err == nil && fi.IsDir() && mine(fi)
}

func mine(fi fs.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return !ok || int(st.Uid) == os.Getuid()
}
