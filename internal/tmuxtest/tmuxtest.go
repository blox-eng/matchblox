// Package tmuxtest gives tests tmux servers of their own: a socket name
// no builder uses, and a cleanup that ends the server and removes its
// socket, so no test leaves one behind.
package tmuxtest

import (
	"fmt"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"

	"github.com/blox-eng/matchblox/internal/sample"
)

var n atomic.Int64

// Server is the socket name (tmux -L) of a server for this test alone.
// The test starts it; when the test ends it is killed and its socket
// removed. Without tmux the test is skipped.
func Server(t testing.TB) string {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	name := fmt.Sprintf("matchblox-test-%d-%d", os.Getpid(), n.Add(1))
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", name, "kill-server").Run() //nolint:gosec // a test's own socket name
		_ = os.Remove(sample.SocketPath(name))
	})
	return name
}
