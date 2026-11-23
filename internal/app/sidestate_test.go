package app

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

func TestSideStateOf(t *testing.T) {
	type edit = func(*proto.State, *sample.Session)
	merge := func(st *proto.State, s *sample.Session) {
		sh := shipFor(*s, 4556, shipWaits, "green · waits for your merge", 2, time.Hour)
		sh.Merge = []string{"gh", "pr", "merge", "4556"}
		st.Ships.Ships = append(st.Ships.Ships, sh)
	}
	for _, c := range []struct {
		name string
		set  []edit
		want sideState
		why  string
	}{
		{"flint-auth", []edit{func(st *proto.State, s *sample.Session) {
			st.Queue = append(st.Queue, waitFor(*s, queue.StatePermission, 4*time.Minute))
		}}, sideNeed, ""},
		{"hearth-api", []edit{func(_ *proto.State, s *sample.Session) { s.Busy, s.Status, s.Idle = true, "busy", 0 }}, sideWork, ""},
		{"kindle-web", []edit{func(st *proto.State, s *sample.Session) {
			st.Holds = append(st.Holds, proto.Hold{SessionID: s.SessionID, Pane: s.Pane, Name: s.Name, By: holdByYou})
		}}, sideHeld, "parked by you · z lets it go"},
		{"spark-docs", []edit{func(st *proto.State, s *sample.Session) {
			st.Holds = append(st.Holds, proto.Hold{SessionID: s.SessionID, Pane: s.Pane, Name: s.Name, By: "stoker", Why: "not the priority"})
		}}, sideHeld, "held by the stoker · not the priority"},
		{"spark-docs", []edit{func(st *proto.State, s *sample.Session) {
			st.Holds = append(st.Holds, proto.Hold{SessionID: s.SessionID, Pane: s.Pane, Name: s.Name, By: "stoker"})
			s.Busy, s.Status, s.Idle = true, "busy", 0
		}}, sideHeld, "held by the stoker"},
		{"cinder-ci", []edit{func(_ *proto.State, s *sample.Session) { s.Idle = 30 * time.Hour }}, sideIdle, ""},
		{"cinder-ci", []edit{func(_ *proto.State, s *sample.Session) { s.Status = "stopped" }}, sideIdle, ""},
		{"ember-billing", []edit{merge}, sideNeed, ""},
		{"wick-mobile", []edit{func(st *proto.State, s *sample.Session) {
			st.Ships.Ships = append(st.Ships.Ships, shipFor(*s, 4556, shipRunning, "CI · 2 of 5 jobs", 1, time.Hour))
		}}, sideIdle, ""},
	} {
		t.Run(c.name+"/"+c.want.head(), func(t *testing.T) {
			s := boardMatch(c.name, time.Hour)
			st := boardState()
			for _, f := range c.set {
				f(&st, &s)
			}
			st.Sessions = []sample.Session{s}
			m := boardModel(t, st)
			got := m.all.sessions[0]
			if state := m.sideStateOf(got); state != c.want {
				t.Fatalf("state %s, want %s", state.head(), c.want.head())
			}
			if why := m.heldWhy(got); why != c.why {
				t.Fatalf("heldWhy %q, want %q", why, c.why)
			}
		})
	}
}

// TestTheListEndsInHeld: a session the builder parked sits below the idle
// ones, at the end of the list (the owner, 2026-11-20, #211).
func TestTheListEndsInHeld(t *testing.T) {
	idle, parked := boardMatch("cinder-ci", time.Hour), boardMatch("ember-billing", time.Hour)
	idle.Status = "stopped"
	st := boardState()
	st.Sessions = []sample.Session{parked, idle}
	st.Holds = []proto.Hold{{SessionID: parked.SessionID, Pane: parked.Pane, Name: parked.Name, By: holdByYou}}
	lines, _, _ := boardModel(t, st).sideList(80, 30)
	text := strings.Join(lines, "\n")
	i, h := strings.Index(text, "IDLE"), strings.Index(text, "HELD")
	if i < 0 || h < 0 || h < i {
		t.Fatalf("IDLE at %d, HELD at %d, want HELD last:\n%s", i, h, text)
	}
}

// doneMatch is a match at rest whose turn ended ago, an hour after the
// owner last opened it.
func doneMatch(name string, ago time.Duration) sample.Session {
	s := boardMatch(name, 2*time.Hour)
	s.TurnEndedAt, s.SeenAt = boardAt.Add(-ago), boardAt.Add(-time.Hour)
	return s
}

func seenActs(f *fakeConn) []string {
	var out []string
	for _, a := range f.acts() {
		if strings.HasPrefix(a.RecID, proto.SeenPrefix) {
			out = append(out, a.RecID)
		}
	}
	return out
}

func TestDoneHoldsATurnTheOwnerHasNotSeen(t *testing.T) {
	a := doneMatch("flint-auth", 13*time.Minute)
	b := doneMatch("hearth-api", 13*time.Minute)
	b.SeenAt = boardAt.Add(-11 * time.Minute)
	c := doneMatch("kindle-web", 13*time.Minute)
	st := boardState(a, b, c)
	st.Queue = []queue.Item{waitFor(c, queue.StateQuestion, time.Minute)}
	m := boardModel(t, st)
	for i, want := range []sideState{sideDone, sideIdle, sideNeed} {
		if got := m.sideStateOf(m.all.sessions[i]); got != want {
			t.Errorf("%s is in %s, want %s", m.all.sessions[i].Name, got.head(), want.head())
		}
	}
}

func TestTheGroupsReadInOrder(t *testing.T) {
	need, work, idle, held := boardMatch("flint-auth", time.Hour), boardMatch("hearth-api", time.Hour), boardMatch("cinder-ci", time.Hour), boardMatch("ember-billing", time.Hour)
	work.Busy, work.Status, work.Idle = true, "busy", 0
	st := boardState(held, idle, work, doneMatch("kindle-web", time.Minute), need)
	st.Queue = []queue.Item{waitFor(need, queue.StatePermission, time.Minute)}
	st.Holds = []proto.Hold{{SessionID: held.SessionID, Pane: held.Pane, Name: held.Name, By: holdByYou}}
	lines, _, _ := boardModel(t, st).sideList(80, 30)
	var heads []string
	for _, l := range plainLines(lines) {
		if name, _, ok := strings.Cut(strings.TrimSpace(l), " · 1"); ok && name == strings.ToUpper(name) {
			heads = append(heads, name)
		}
	}
	if want := []string{"NEEDS YOU", "DONE", "WORKING", "IDLE", "HELD"}; !slices.Equal(heads, want) {
		t.Fatalf("heads %q, want %q:\n%s", heads, want, strings.Join(plainLines(lines), "\n"))
	}
}

// TestScrollingDoesNotSee: ↑↓ only choose a row; the agent getting the
// keys is the open (design review D5), and the row it opened stays where
// it was until the choice moves.
func TestScrollingDoesNotSee(t *testing.T) {
	st := boardState(doneMatch("flint-auth", 30*time.Minute), doneMatch("hearth-api", 20*time.Minute),
		doneMatch("kindle-web", 10*time.Minute), boardMatch("cinder-ci", time.Hour))
	m := pressOn(boardModel(t, st), "down", "down")
	f := boardFake(m)
	done := func(m Model) int { g, _ := groupOfState(m.sideGroups(), sideDone); return g.count }
	if got := seenActs(f); len(got) != 0 || done(m) != 3 || chosenName(t, m) != "kindle-web" {
		t.Fatalf("↓↓ sent %q, DONE counts %d, chosen %s; want nothing sent, 3, kindle-web", got, done(m), chosenName(t, m))
	}
	m = pressOn(m, "enter")
	_ = m.View()
	if got := seenActs(f); !slices.Equal(got, []string{proto.SeenPrefix + "%kindle-web"}) {
		t.Fatalf("⏎ sent %q, want one seen of kindle-web", got)
	}
	st.Sessions[2].SeenAt = boardAt
	m = newBoardState(t, m, st)
	at := slices.Index(m.sideOrder(), paneKey{"", "%kindle-web"})
	if done(m) != 3 || at != 2 {
		t.Fatalf("seen: DONE counts %d, kindle-web at %d; want it in its place until the choice moves", done(m), at)
	}
	m = pressOn(m, "down")
	if got := m.sideStateOf(m.all.sessions[2]); got != sideIdle || done(m) != 2 || len(seenActs(f)) != 1 {
		t.Fatalf("after ↓ kindle-web is in %s, DONE counts %d, %d seen sent; want IDLE, 2, 1", got.head(), done(m), len(seenActs(f)))
	}
}

// TestDoneLeavesWhenTheAgentGetsTheKeys: ⌃o, and ⏎ beside the list, give
// the agent the keys: each sends one seen.
func TestDoneLeavesWhenTheAgentGetsTheKeys(t *testing.T) {
	st := boardState(doneMatch("flint-auth", 10*time.Minute))
	m := boardModel(t, st)
	m.opt.Run = func([]string) error { return nil }
	if got := seenActs(boardFake(pressOn(m, "ctrl+o"))); !slices.Equal(got, []string{proto.SeenPrefix + "%flint-auth"}) {
		t.Errorf("⌃o sent %q, want one seen of flint-auth", got)
	}
	b, _ := besideConsole(t, st, 80, 30, "160")
	if got := seenActs(boardFake(pressOn(b, "enter"))); !slices.Equal(got, []string{proto.SeenPrefix + "%flint-auth"}) {
		t.Errorf("⏎ beside sent %q, want one seen of flint-auth", got)
	}
}

// TestDoneOutlastsTheColdRule: a turn the owner never opened stays in
// DONE however long ago it ended (the spec's anti-goal).
func TestDoneOutlastsTheColdRule(t *testing.T) {
	s := doneMatch("flint-auth", 30*time.Hour)
	s.Idle, s.SeenAt = 30*time.Hour, time.Time{}
	m := boardModel(t, boardState(s))
	if got := m.sideStateOf(m.all.sessions[0]); got != sideDone {
		t.Fatalf("a turn ended 30h ago, never seen, is in %s, want DONE", got.head())
	}
}

// TestDoneKeepEndsWhenAnotherRowIsClicked: ⏎ keeps a done row in its
// place; a click on another row moves the choice, so it goes to IDLE, and
// the click itself opens nothing.
func TestDoneKeepEndsWhenAnotherRowIsClicked(t *testing.T) {
	st := boardState(doneMatch("flint-auth", 30*time.Minute), doneMatch("hearth-api", 20*time.Minute))
	m, _, _ := sideShotOf(t, st, 132, 32)
	m.toList()
	m = pressOn(m, "enter")
	f := boardFake(m)
	st.Sessions[0].SeenAt = boardAt
	m = newBoardState(t, m, st)
	if got := m.sideStateOf(m.all.sessions[0]); got != sideDone {
		t.Fatalf("after ⏎ flint-auth is in %s, want DONE until the choice moves", got.head())
	}
	m = tapOn(t, m, listRow(t, m, "hearth-api"))
	if got := m.sideStateOf(m.all.sessions[0]); got != sideIdle || chosenName(t, m) != "hearth-api" {
		t.Fatalf("after a click on hearth-api flint-auth is in %s, chosen %s; want IDLE, hearth-api", got.head(), chosenName(t, m))
	}
	if got := seenActs(f); len(got) != 1 {
		t.Fatalf("seen sent %q, want only ⏎'s: a click on a row opens nothing", got)
	}
}

// TestDoneNeedsAPaneToBeKept: a session with no key of its own is not the
// kept row.
func TestDoneNeedsAPaneToBeKept(t *testing.T) {
	s := boardMatch("flint-auth", time.Hour)
	s.Pane, s.PID, s.SessionID = "", 0, ""
	m := boardModel(t, boardState(s))
	if got := m.sideStateOf(m.all.sessions[0]); got != sideIdle {
		t.Fatalf("a rest with no pane, pid or sid is in %s, want IDLE", got.head())
	}
}

// TestDoneSeenOnAnOldServiceSaysNothing: a service that does not know
// seen: answers unknown action; the owner opened a session, so nothing
// went wrong for them to read.
func TestDoneSeenOnAnOldServiceSaysNothing(t *testing.T) {
	m, ws := twoHosts(t, hostState(), hostState(sess(2, "train-loop", "%2", func(s *sample.Session) { s.TurnEndedAt = boardAt })))
	m.toList()
	m = pressOn(m, "enter")
	var id string
	ws.mu.Lock()
	for _, e := range ws.sent {
		var a proto.Act
		if e.Kind == proto.KindAct && json.Unmarshal(e.Body, &a) == nil && a.RecID == proto.SeenPrefix+"%2" {
			id = e.ID
		}
	}
	ws.mu.Unlock()
	if id == "" {
		t.Fatal("⏎ sent no seen to the peer")
	}
	next, cmd := m.Update(peerMsg{host: "ws-1", conn: ws, msg: resultMsg(proto.Result{ActID: id, Err: "unknown action"})})
	drain(cmd)
	if frame := plainFrame(next.(Model)); strings.Contains(frame, "unknown action") {
		t.Fatalf("an old service's answer to seen shows:\n%s", frame)
	}
}

// TestDoneOnAPhone: a phone draws the DONE group whole, one row per line.
func TestDoneOnAPhone(t *testing.T) {
	st := boardState(doneMatch("flint-auth", 30*time.Minute), doneMatch("hearth-api", 20*time.Minute), boardMatch("cinder-ci", time.Hour))
	m, _, _ := sideShotOf(t, st, 44, 31)
	frame := plainFrame(m)
	for _, want := range []string{"DONE · 2", "flint-auth", "hearth-api", "IDLE · 1", "cinder-ci"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("no %q on a phone:\n%s", want, frame)
		}
	}
	for _, l := range strings.Split(frame, "\n") {
		if w := ansi.StringWidth(l); w > 44 {
			t.Fatalf("a line of %d cells on a 44-cell phone: %q", w, l)
		}
	}
}
