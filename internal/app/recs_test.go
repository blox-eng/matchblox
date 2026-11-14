package app

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/history"
)

// TestHistoryWithAFlatSeries: a value that never changes (swap at 0 GB)
// draws a flat chart; a zero range must not crash the console.
func TestHistoryWithAFlatSeries(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	at := time.Unix(1_800_000_000, 0)
	for i := range 10 {
		m.hist.Points = append(m.hist.Points, history.Point{At: at.Add(time.Duration(i) * 30 * time.Second), CPU: float64(i)})
	}
	m.tab = tabHistory
	if out := ansi.Strip(m.render()); !strings.Contains(out, "SWAP") {
		t.Fatalf("no swap chart:\n%s", out)
	}
}
