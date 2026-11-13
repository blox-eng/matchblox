//go:build !linux && !darwin

package guard

import (
	"errors"
	"os"
	"os/exec"
)

// Main runs argv on the pane itself: there is no guard here.
func Main(argv []string, _ string, in, out *os.File) int {
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // the caller's own command, never a shell string
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, out
	var exit *exec.ExitError
	if err := cmd.Run(); errors.As(err, &exit) {
		return exit.ExitCode()
	} else if err != nil {
		return 1
	}
	return 0
}

// Run runs no command here: the guard needs a Unix terminal pair.
func Run([]string, *os.File, *os.File) (int, error) {
	return 0, errors.New("the redraw guard runs on Linux and macOS only")
}
