//go:build !linux && !darwin

package guard

import (
	"errors"
	"os"
)

// Run runs no command here: the guard needs a Unix terminal pair.
func Run([]string, *os.File, *os.File) (int, error) {
	return 0, errors.New("the redraw guard runs on Linux and macOS only")
}
