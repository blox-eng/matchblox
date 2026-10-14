//go:build !windows

package setup

import (
	"os"
	"path/filepath"
	"syscall"
)

// withKeysLock runs fn while this process alone changes authorized_keys: two
// matchblox processes (a gate pair and a devices remove) never read, edit
// and replace the file at the same time.
func withKeysLock(home string, fn func() error) error {
	dir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, ".matchblox.lock"), os.O_CREATE|os.O_RDONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	return fn()
}
