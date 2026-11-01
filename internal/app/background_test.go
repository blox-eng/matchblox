package app

import (
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
)

// An idle session's detail names the background work it left: the
// subagents, the shells, how long the oldest has run, and each command.
// A session in a pane shows its chat there; this one runs outside tmux.
func TestTheDetailShowsTheBackgroundWork(t *testing.T) {
	st := queueState()
	pid := 0
	for i := range st.Sessions {
		if st.Sessions[i].Pane == "%1" {
			st.Sessions[i].Pane, pid = "", st.Sessions[i].PID
			st.Sessions[i].Agents = 1
			st.Sessions[i].Shells = []sample.Shell{{PID: 2240632, Started: st.At.Add(-37 * time.Hour), Command: `while pgrep -f "node walk.mjs" >/dev/null; do sleep 15; done`}}
		}
	}
	m, _ := loadedWith(t, 160, st)
	m.chose, m.selPID = true, pid
	out := screen(m)
	for _, want := range []string{"agents   1 subagent running", "shells   1 shell · oldest 1d13h", "2240632", "node walk.mjs"} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q in\n%s", want, out)
		}
	}
}

// A session working through its subagents says so in its row, wide and on
// a phone: busy, with the word agents, never idle.
func TestASessionWorkingThroughSubagentsSaysAgents(t *testing.T) {
	for _, w := range []int{120, 46} {
		st := queueState()
		for i := range st.Sessions {
			if st.Sessions[i].Pane == "%1" {
				s := &st.Sessions[i]
				s.Busy, s.Status, s.Agents, s.Idle = true, sample.StatusAgents, 2, 4*time.Minute
			}
		}
		m, _ := loadedWith(t, w, st)
		next, _ := key(m, "1")
		out := screen(next)
		var row string
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "agents") && !strings.Contains(l, "AGENTS") {
				row = l
			}
		}
		if row == "" || strings.Contains(row, "idle") || strings.Contains(row, "busy") {
			t.Fatalf("width %d: row %q in\n%s", w, row, out)
		}
	}
}
