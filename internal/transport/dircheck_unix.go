//go:build unix

package transport

import (
	"fmt"
	"os"
	"syscall"
)

// checkDir refuses a socket directory that is a symlink, that others can
// enter, or that another user owns: on a shared /tmp another user could
// make it first and serve a fake console.
func checkDir(dir string) error {
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
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("%s belongs to uid %d, not to you", dir, st.Uid)
	}
	return nil
}
