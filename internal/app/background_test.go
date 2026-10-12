package app

import (
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
)

// An idle session's detail names the background work it left: the
// subagents, the shells, how long the oldest has run, and each command.
func TestTheDetailShowsTheBackgroundWork(t *testing.T) {
	st := queueState()
	for i := range st.Sessions {
		if st.Sessions[i].Pane == "%1" {
			st.Sessions[i].Agents = 1
			st.Sessions[i].Shells = []sample.Shell{{PID: 2240632, Started: st.At.Add(-37 * time.Hour), Command: `while pgrep -f "node walk.mjs" >/dev/null; do sleep 15; done`}}
		}
	}
	m, _ := loadedWith(t, 120, st)
	next, _ := key(m, "2")
	for s, ok := next.(Model).selected(); ok && s.Pane != "%1"; s, ok = next.(Model).selected() {
		next, _ = key(next, "down")
	}
	out := screen(next)
	for _, want := range []string{"BACKGROUND 1 agent · 1 shell · oldest 1d13h", "2240632", "node walk.mjs"} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q in\n%s", want, out)
		}
	}
}
