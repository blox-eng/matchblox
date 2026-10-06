//go:build unix

package transport

import (
	"errors"
	"os"
	"syscall"
)

// lock takes an exclusive lock that the kernel drops when the process dies,
// so a crashed service never leaves a lock behind.
func lock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // our own runtime dir
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil { //nolint:gosec // fd fits int
		if errors.Is(err, syscall.EWOULDBLOCK) {
			err = ErrInUse
		}
		return nil, errors.Join(err, f.Close())
	}
	return f, nil
}
