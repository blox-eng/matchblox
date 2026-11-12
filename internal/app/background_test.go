package app

import (
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
)

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
