package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/sample"
)

// TestExitedAgentsStayQuiet: exited agents are one faint line under the
// sessions; e lists them with the parent that keeps them and the fix.
func TestExitedAgentsStayQuiet(t *testing.T) {
	for _, w := range []int{50, 120} {
		st := fixtureState()
		for pid := 7000; pid < 7137; pid++ {
			st.Exited = append(st.Exited, sample.Exited{PID: pid, Parent: 3214249, ParentComm: "systemd"})
		}
		m, _ := loadedWith(t, w, st)
		var next tea.Model = m
		next, _ = key(next, "2")
		out := screen(next)
		if !strings.Contains(out, "137 exited agents · e shows them") || strings.Contains(out, "7000") {
			t.Fatalf("width %d: the exited agents are not one quiet line:\n%s", w, out)
		}
		next, _ = key(next, "e")
		out = screen(next)
		for _, want := range []string{"137 EXITED AGENTS", "systemd (pid 3214249) has not reaped them", "systemctl --user daemon-reexec", "7000"} {
			if !strings.Contains(out, want) {
				t.Fatalf("width %d: expanded lacks %q:\n%s", w, want, out)
			}
		}
		fitsWidth(t, next.(Model), w)
		next, _ = key(next, "e")
		if strings.Contains(screen(next), "7000") {
			t.Fatal("e did not fold them again")
		}
	}
}
