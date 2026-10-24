//go:build !darwin && !linux

package accounts

import (
	"errors"
	"io/fs"
)

// refusals are the errors of a file system that cannot clone a file.
var refusals = []error{errors.ErrUnsupported}

// cloneFile is refused here: the copy is byte by byte.
func cloneFile(from, to string, _ fs.FileMode) error {
	return &fs.PathError{Op: "clone", Path: to, Err: errors.ErrUnsupported}
}
