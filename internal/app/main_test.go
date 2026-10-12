package app

import (
	"os"
	"testing"
	"time"
)

// TestMain draws every time in UTC, as the frames in testdata/screens are.
// time.Local is set here, before any test runs: a test that wrote it would
// race the timers other tests leave running.
func TestMain(m *testing.M) {
	time.Local = time.UTC
	os.Exit(m.Run())
}
