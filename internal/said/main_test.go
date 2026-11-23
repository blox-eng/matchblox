//go:build canary

package said

import (
	"os"
	"testing"

	"github.com/blox-eng/matchblox/internal/testhome"
)

// TestMain keeps the canary's tmux servers off the builder's own.
func TestMain(m *testing.M) {
	os.Exit(testhome.Run(m))
}
