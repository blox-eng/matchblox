package accounts

import (
	"os"
	"testing"

	"github.com/blox-eng/matchblox/internal/testhome"
)

// TestMain keeps the tests off the builder's own home folder.
func TestMain(m *testing.M) {
	os.Exit(testhome.Run(m))
}
