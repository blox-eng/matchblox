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

// TestXEndsAStaleSession: x on a stale session ends its agent after a
// typed y; on any other session it says when x works.
func TestXEndsAStaleSession(t *testing.T) {
	m, f := loadedWith(t, 120, agingState())
	var next tea.Model = m
	next, _ = key(next, "2")
	next, _ = key(next, "x") // bravo: busy
	if next.(Model).pending != nil || !strings.Contains(actionLine(next), "x ends a stale session") {
		t.Fatalf("x on a busy session: %q", actionLine(next))
	}
	for range 4 {
		next, _ = key(next, "down")
	}
	if !strings.Contains(actionLine(next), "x end") {
		t.Fatalf("the keys of a stale session lack x: %q", actionLine(next))
	}
	next, _ = key(next, "x")
	if line := actionLine(next); !strings.Contains(line, "RUN kill 5") || !strings.Contains(line, "y run") {
		t.Fatalf("x on the stale session: %q", line)
	}
	_, cmd := key(next, "y")
	if cmd == nil {
		t.Fatal("y sent nothing")
	}
	cmd()
	if acts := f.acts(); len(acts) != 1 || acts[0] != (proto.Act{RecID: "session:5", Which: "secondary", Confirm: "y"}) {
		t.Fatalf("sent %+v", acts)
	}
}

// TestOldSessionsSinkEvenWhenQueued: a queue item a day old does not lift a
// cold session over a fresh one: age outranks waiting (found live: without
// hooks the queue holds a "done" item for nearly every idle session).
func TestOldSessionsSinkEvenWhenQueued(t *testing.T) {
	st := agingState()
	st.Queue = append(st.Queue, queue.Item{Pane: "%delta", Name: "delta", State: queue.StateFinished, Since: st.At.Add(-30 * time.Hour)})
	m, _ := loadedWith(t, 120, st)
	next, _ := key(m, "2")
	if got := order(next); got != "bravo charlie alpha delta echo" {
		t.Fatalf("order %q", got)
	}
}

// TestTheFixFitsTheParent: zombies of PID 1 need the system manager, not
// the user one; one agent is "agent".
func TestTheFixFitsTheParent(t *testing.T) {
	st := fixtureState()
	st.Exited = []sample.Exited{{PID: 9001, Parent: 1, ParentComm: "systemd"}}
	m, _ := loadedWith(t, 120, st)
	next, _ := key(m, "2")
	if out := screen(next); !strings.Contains(out, "1 exited agent · e shows it") {
		t.Fatalf("singular:\n%s", out)
	}
	next, _ = key(next, "e")
	out := screen(next)
	if !strings.Contains(out, "sudo systemctl daemon-reexec reaps them") || strings.Contains(out, "--user") {
		t.Fatalf("pid 1 fix:\n%s", out)
	}
}
