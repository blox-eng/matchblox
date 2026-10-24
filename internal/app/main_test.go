package app

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/setup"

	"github.com/blox-eng/matchblox/internal/testhome"
)

// TestMain draws every time in UTC, as the frames in testdata/screens are.
// time.Local is set here, before any test runs: a test that wrote it would
// race the timers other tests leave running. It also stubs the pair probe so
// NewPairOffer never shells out to this machine's tailscale or sshd.
func TestMain(m *testing.M) {
	time.Local = time.UTC
	setup.SetPairProbe(setup.PairProbe{LookPath: func(string) (string, error) { return "", errors.New("not found") }})
	os.Exit(testhome.Run(m))
}
