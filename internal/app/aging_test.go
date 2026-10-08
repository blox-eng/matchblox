package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
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

func agingState() proto.State {
	st := sortFixture()
	st.Sessions = append(st.Sessions, st.Sessions[0], st.Sessions[0])
	st.Sessions[3].PID, st.Sessions[3].Name, st.Sessions[3].Pane, st.Sessions[3].Idle = 4, "delta", "%delta", 30*time.Hour
	st.Sessions[4].PID, st.Sessions[4].Name, st.Sessions[4].Pane, st.Sessions[4].Idle = 5, "echo", "%echo", 8*24*time.Hour
	// charlie (idle 1h) waits for the person: it goes above alpha (10 min).
	st.Queue = []queue.Item{{Pane: "%charlie", Name: "charlie", State: queue.StateQuestion, Since: st.At.Add(-time.Hour)}}
	return st
}

// TestIdleSessionsAge: busy, then who waits for the person, then idle by
// the newest; a day makes a session cold, a week makes it stale.
func TestIdleSessionsAge(t *testing.T) {
	for _, w := range []int{50, 120} {
		m, _ := loadedWith(t, w, agingState())
		next, _ := key(m, "2")
		if got := order(next); got != "bravo charlie alpha delta echo" {
			t.Fatalf("width %d: order %q", w, got)
		}
		lines := strings.Split(screen(next), "\n")
		for name, word := range map[string]string{"alpha": "idle", "delta": "cold", "echo": "stale"} {
			if l := lines[lineOf(t, next, name)]; !strings.Contains(l, word) {
				t.Fatalf("width %d: %s is not %s: %q", w, name, word, l)
			}
		}
	}
}
