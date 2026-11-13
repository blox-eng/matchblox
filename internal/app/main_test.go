package app

import (
	"errors"
	"os"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/setup"

	"github.com/blox-eng/matchblox/internal/testhome"
)

// TestMain draws every time in UTC, as the frames in testdata/screens are.
// time.Local is set here, before any test runs: a test that wrote it would
// race the timers other tests leave running. It also stubs the pair probe so
// NewPairOffer never shells out to this machine's tailscale or sshd. A
// notice's end comes at once: its life runs on each test's clock, and a
// test that runs the command would otherwise wait noticeFor of wall time.
func TestMain(m *testing.M) {
	time.Local = time.UTC
	setup.SetPairProbe(setup.PairProbe{LookPath: func(string) (string, error) { return "", errors.New("not found") }})
	noticeEnds = func(time.Duration) tea.Cmd { return func() tea.Msg { return noticeMsg{} } }
	os.Exit(testhome.Run(m))
}
