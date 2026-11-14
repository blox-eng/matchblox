package app

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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
		next, _ = key(next, "1")
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

// TestOldSessionsSinkEvenWhenQueued: a queue item a day old does not lift a
// cold session over a fresh one: age outranks waiting (found live: without
// hooks the queue holds a "done" item for nearly every idle session).
func TestOldSessionsSinkEvenWhenQueued(t *testing.T) {
	st := agingState()
	st.Queue = append(st.Queue, queue.Item{Pane: "%delta", Name: "delta", State: queue.StateFinished, Since: st.At.Add(-30 * time.Hour)})
	m, _ := loadedWith(t, 120, st)
	next, _ := key(m, "1")
	if got := order(next); got != "charlie bravo alpha delta echo" {
		t.Fatalf("order %q", got)
	}
}

// TestTheFixFitsTheParent: zombies of PID 1 need the system manager, not
// the user one; one agent is "agent".
func TestTheFixFitsTheParent(t *testing.T) {
	st := fixtureState()
	st.Exited = []sample.Exited{{PID: 9001, Parent: 1, ParentComm: "systemd"}}
	m, _ := loadedWith(t, 120, st)
	next, _ := key(m, "1")
	if out := screen(next); !strings.Contains(out, "1 exited agent · e shows it") {
		t.Fatalf("singular:\n%s", out)
	}
	next, _ = key(next, "e")
	out := screen(next)
	if !strings.Contains(out, "sudo systemctl daemon-reexec reaps them") || strings.Contains(out, "--user") {
		t.Fatalf("pid 1 fix:\n%s", out)
	}
}

// A session the plan's limit stops says until when, and is quiet: nothing
// to do there before its reset.
func TestAnOutSessionSaysUntilWhen(t *testing.T) {
	st := fixtureState()
	back := st.At.Add(2 * time.Hour)
	out := ""
	for i := range st.Sessions {
		if !st.Sessions[i].Busy {
			st.Sessions[i].Out, st.Sessions[i].Idle = back, 10*time.Minute
			out = st.Sessions[i].Pane
			break
		}
	}
	m, _ := loadedWith(t, 160, st)
	m = selectPane(t, m, out).(Model) // the chosen card says what the row said
	at := back.Local().Format("15:04")
	if out := ansi.Strip(m.render()); !strings.Contains(out, "· out") || !regexp.MustCompile(`✻ +`+at).MatchString(out) { // the card's word and its age
		t.Fatalf("no out %s in\n%s", at, out)
	}
	if g := m.groupOf(sample.Session{Out: back}); g != groupQuiet {
		t.Fatalf("an out session is in group %d", g)
	}
}

func TestStatusCountsTheOutSessions(t *testing.T) {
	st := fixtureState()
	n := 0
	for i := range st.Sessions {
		if !st.Sessions[i].Busy && n == 0 {
			st.Sessions[i].Out, st.Sessions[i].Idle, n = st.At.Add(time.Hour), 10*time.Minute, 1
		}
	}
	if out := Summary(st.Doc); !strings.Contains(out, ", out 1)") {
		t.Fatalf("%s", out)
	}
}
