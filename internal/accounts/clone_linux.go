package accounts

import (
	"errors"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

// refusals are the errors of a file system that cannot clone a file.
var refusals = []error{unix.ENOTSUP, unix.EXDEV, unix.EINVAL, unix.EOPNOTSUPP}

// cloneFile clones from to the new file to, copy-on-write (FICLONE), with
// mode perm, as darwin does.
func cloneFile(from, to string, perm fs.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if err := unix.IoctlFileClone(int(out.Fd()), int(in.Fd())); err != nil { //nolint:gosec // a file descriptor fits an int
		return errors.Join(&fs.PathError{Op: "ficlone", Path: to, Err: err}, out.Close(), os.Remove(to))
	}
	if err := out.Chmod(perm); err != nil {
		return errors.Join(err, out.Close(), os.Remove(to))
	}
	if err := out.Close(); err != nil {
		return errors.Join(err, os.Remove(to))
	}
	return nil
}
