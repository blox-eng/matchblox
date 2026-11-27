package app

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
)

// busyThenIdle samples a working session, then an idle one: the last turn
// ended. edit changes the idle sample.
func busyThenIdle(t *testing.T, edit func(*proto.State, *sample.Session)) Model {
	t.Helper()
	s := boardMatch("hearth-api", time.Hour)
	s.Busy, s.Status, s.Idle, s.LastLine = true, "busy", 0, "I rewrote the router."
	m := boardModel(t, boardState(s))
	idle := m.all.sessions[0]
	idle.Busy, idle.Status, idle.Idle = false, "idle", 5*time.Second
	st := boardState(idle)
	if edit != nil {
		edit(&st, &idle)
		st.Sessions = []sample.Session{idle}
	}
	next, _ := m.Update(stateMsg(st))
	return next.(Model)
}

func askingIdle(_ *proto.State, s *sample.Session) {
	s.Ask = &said.Ask{ID: "q1", Questions: []said.Question{{Question: "Ship the tokens?"}}}
	s.Asks, s.Reported, s.Status = true, true, "waiting"
}

func doneRow(m Model) sample.Session { return m.all.sessions[0] }

func doneLines(m Model, s sample.Session) []string {
	return plainLines(m.sideRow(s, 50, false, sideLink{}))
}

func withBeside(m Model) Model {
	m.opt.Beside = true
	m.opt.Pane = "%1"
	m.opt.Socket = "/tmp/tmux-test"
	m.opt.Tmux = func(args ...string) (string, error) { return "\n", nil }
	return m
}

func keysActs(m Model) []proto.Act {
	var out []proto.Act
	for _, a := range boardFake(m).acts() {
		if strings.HasPrefix(a.RecID, proto.KeysPrefix) {
			out = append(out, a)
		}
	}
	return out
}

func applyKeysAt(m Model, s sample.Session, at time.Time) Model {
	st := boardState(s)
	st.KeysAt = map[string]time.Time{s.SessionID: at}
	next, _ := m.Update(stateMsg(st))
	return next.(Model)
}

// TestAskBeatsDone: a turn that ends with an ask sits in NEEDS YOU, not
// DONE. A wait must stay on the screen (hostile: agent's tools / DONE hides a wait).
func TestAskBeatsDone(t *testing.T) {
	m := busyThenIdle(t, askingIdle)
	s := doneRow(m)
	if h := m.sideStateOf(s); h == sideDone {
		t.Fatal("DONE hid a wait")
	}
	if h := m.sideStateOf(s); h != sideNeed {
		t.Fatalf("group %s, want NEEDS YOU", h.head())
	}
}

func TestBusyToIdleWithNoAskIsDone(t *testing.T) {
	m := busyThenIdle(t, nil)
	s := doneRow(m)
	if h := m.sideStateOf(s); h != sideDone {
		t.Fatalf("group %s, want DONE", h.head())
	}
	g, ok := groupOfState(m.sideGroups(), sideDone)
	if !ok || g.count != 1 {
		t.Fatalf("DONE %+v, want one session", g)
	}
	got := doneLines(m, s)
	if len(got) == 0 || !strings.Contains(got[0], "now") {
		t.Fatalf("row %q, want time now", got)
	}
	if len(got) < 2 || !strings.HasPrefix(strings.TrimSpace(got[1]), "said ") {
		t.Fatalf("row %q, want a second line that starts said ", got)
	}
}

func TestEnterLeavesDone(t *testing.T) {
	m := withBeside(busyThenIdle(t, nil))
	s := doneRow(m)
	if n := m.doneCount(); n != 1 {
		t.Fatalf("✓ %d done, want 1", n)
	}
	next, cmd := key(m, "enter")
	drain(cmd)
	m = next.(Model)
	want := proto.KeysPrefix + s.SessionID
	got := keysActs(m)
	if len(got) != 1 || got[0].RecID != want || got[0].Text != "" {
		t.Fatalf("acts %+v, want one %s with no text", got, want)
	}
	m = applyKeysAt(m, s, boardAt)
	if h := m.sideStateOf(doneRow(m)); h != sideIdle {
		t.Fatalf("after keys: group %s, want IDLE", h.head())
	}
	if n := m.doneCount(); n != 0 {
		t.Fatalf("✓ %d done, want 0", n)
	}
}

func TestArrowsDoNotLeaveDone(t *testing.T) {
	m := withBeside(busyThenIdle(t, nil))
	s := doneRow(m)
	for _, k := range []string{"up", "down"} {
		next, cmd := key(m, k)
		drain(cmd)
		m = next.(Model)
		if h := m.sideStateOf(s); h != sideDone {
			t.Fatalf("%s: group %s, want DONE", k, h.head())
		}
	}
	if got := keysActs(m); len(got) != 0 {
		t.Fatalf("arrows sent keys: %+v", got)
	}
}

func TestStuckPRStaysNeedsYou(t *testing.T) {
	for _, note := range []string{"CI failed · tests", "conflicts with main", "changes requested"} {
		m := busyThenIdle(t, func(st *proto.State, s *sample.Session) {
			st.Ships.Ships = append(st.Ships.Ships, shipFor(*s, 4556, shipStuck, note, 1, time.Hour))
		})
		if h := m.sideStateOf(doneRow(m)); h != sideNeed {
			t.Fatalf("%s: group %s, want NEEDS YOU", note, h.head())
		}
	}
}

func TestChainDoneMarkDoesNotCount(t *testing.T) {
	m := loginChain(t, func(_ *proto.State, ss map[string]*sample.Session) {
		s := ss["kindle-web"]
		s.Tokens, s.ContextPct, s.Context = 0, 0, ""
	})
	need, ok := groupOfState(m.sideGroups(), sideNeed)
	if !ok || len(need.units) != 1 {
		t.Fatalf("NEEDS YOU %+v, want the chain", need)
	}
	var finished sample.Session
	for _, s := range need.units[0].members {
		if s.Name == "kindle-web" {
			finished = s
		}
	}
	if m.sideStateOf(finished) != sideDone {
		t.Fatalf("nested member %s, want DONE", m.sideStateOf(finished).head())
	}
	if n := m.doneCount(); n != 0 {
		t.Fatalf("✓ %d done counted a nested finished member", n)
	}
	if g := ansi.Strip(m.sideMatch(finished, sideDone)); g != "✓" {
		t.Fatalf("nested mark %q, want ✓", g)
	}
}

func TestParkedAskStaysHeld(t *testing.T) {
	m := busyThenIdle(t, func(st *proto.State, s *sample.Session) {
		st.Holds = append(st.Holds, proto.Hold{SessionID: s.SessionID, Pane: s.Pane, Name: s.Name, By: holdByYou})
		askingIdle(st, s)
	})
	if h := m.sideStateOf(doneRow(m)); h != sideHeld {
		t.Fatalf("group %s, want HELD (park beats ask)", h.head())
	}
}
