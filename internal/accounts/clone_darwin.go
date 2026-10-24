package accounts

import (
	"errors"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

// refusals are the errors of a file system that cannot clone a file.
var refusals = []error{unix.ENOTSUP, unix.EXDEV, unix.EINVAL, unix.EOPNOTSUPP}

// cloneFile clones from to the new file to, copy-on-write, with mode perm.
func cloneFile(from, to string, perm fs.FileMode) error {
	if err := unix.Clonefile(from, to, unix.CLONE_NOFOLLOW); err != nil {
		return &fs.PathError{Op: "clonefile", Path: to, Err: err}
	}
	if err := os.Chmod(to, perm); err != nil {
		return errors.Join(err, os.Remove(to))
	}
	return nil
}
