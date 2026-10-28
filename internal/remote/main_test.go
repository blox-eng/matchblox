package remote

import (
	"os"
	"testing"

	"github.com/blox-eng/matchblox/internal/testhome"
)

func TestMain(m *testing.M) {
	os.Exit(testhome.Run(m))
}
