package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/proto"
)

const deskTTY = "/dev/pts/3"

func flipFrom(t *testing.T, m Model, tty, pane string) Model {
	t.Helper()
	next, cmd := m.Update(uv.UnknownApcEvent(keycmd.FlipAPC(tty + " " + pane)))
	drain(cmd)
	return next.(Model)
}

// jumped is the tmux argv a key ran in m: the flip's way back.
func jumped(t *testing.T, m Model, k tea.KeyPressMsg) (Model, []string) {
	t.Helper()
	var ran []string
	m.opt.Run = func(argv []string) error { ran = argv; return nil }
	next, cmd := m.Update(k)
	fed(next, cmd)
	return next.(Model), ran
}

var altS = tea.KeyPressMsg{Code: 's', Mod: tea.ModAlt}

func TestFlipFromASessionOpensItsChainsRoomOnTheNeed(t *testing.T) {
	t.Skip("replaced by TestARoomOpensOnTheNeedElseTheTalkLine (Task 12 step 1)")
}

func TestFlipInTheRoomGoesBackToWhereItCameFrom(t *testing.T) {
	m, _ := headModel(t, 160, headState(t, runningPlan(), true))
	m = flipFrom(t, m, deskTTY, "%1")
	m.marked = clientMark{deskTTY, m.now()} // the desk's key
	_, ran := jumped(t, m, altS)
	if got := strings.Join(ran, " "); !strings.Contains(got, "switch-client") || !strings.HasSuffix(got, "%1") {
		t.Fatalf("ran %q, want a switch to %%1", got)
	}
}

func TestTwoClientsFlipBackToTheirOwnSessions(t *testing.T) {
	m, _ := headModel(t, 160, headState(t, runningPlan(), true))
	m = flipFrom(t, m, deskTTY, "%1")
	m = flipFrom(t, m, "/dev/pts/9", "%8")
	if m.room.from[deskTTY] != "%1" || m.room.from["/dev/pts/9"] != "%8" {
		t.Fatalf("from %v", m.room.from)
	}
	m.marked = clientMark{"/dev/pts/9", m.now()}
	if _, ran := jumped(t, m, altS); !strings.HasSuffix(strings.Join(ran, " "), "%8") {
		t.Fatalf("the phone went back to %q", ran)
	}
}

func TestFlipInTheRoomWithNoCameFromStartsTyping(t *testing.T) {
	m, _ := roomWith(t, headState(t, runningPlan(), true), "hearth-api")
	next, _ := m.Update(altS)
	if r := next.(Model).room; r == nil || !r.typing {
		t.Fatalf("room %+v", r)
	}
}

func TestFlipBackToAGonePaneStays(t *testing.T) {
	m, _ := headModel(t, 160, headState(t, runningPlan(), true))
	m = flipFrom(t, m, deskTTY, "%1")
	m.room.from[deskTTY] = "%77"
	m.marked = clientMark{deskTTY, m.now()}
	next, ran := jumped(t, m, altS)
	if len(ran) != 0 || next.room == nil || saidNow(next) != "That session ended." {
		t.Fatalf("ran %q room %v toast %q", ran, next.room != nil, saidNow(next))
	}
}

func TestFlipFromALooseSessionOpensTheChainOfOne(t *testing.T) {
	m, _ := headModel(t, 160, headState(t, runningPlan(), true))
	m = flipFrom(t, m, deskTTY, "%2") // flint-auth
	if m.room == nil || m.room.k.id != "=%2" {
		t.Fatalf("room %+v", m.room)
	}
}

func TestFlipFromAPaneWithNoSessionSaysSo(t *testing.T) {
	m, _ := headModel(t, 160, headState(t, runningPlan(), true))
	m = flipFrom(t, m, deskTTY, "%55")
	if m.room != nil || !strings.Contains(m.flash, "no session") {
		t.Fatalf("room %+v flash %q", m.room, m.flash)
	}
}

// TestNextWalksMatchesThenTheStokerThenTheFirst: ⌥n from a match goes to
// the next one; from the last, to the chain's room; from the room, to the
// first match.
func TestNextWalksMatchesThenTheStokerThenTheFirst(t *testing.T) {
	m, _ := headModel(t, 160, headState(t, runningPlan(), true)) // c1: kindle-web %5, hearth-api %1, ember-billing %8
	next := func(m Model, from string) (Model, []string) {
		t.Helper()
		var ran []string
		m.opt.Run = func(argv []string) error { ran = argv; return nil }
		n, cmd := m.Update(uv.UnknownApcEvent(keycmd.NextAPC(deskTTY + " " + from)))
		fed(n, cmd)
		return n.(Model), ran
	}
	if _, ran := next(m, "%5"); !strings.HasSuffix(strings.Join(ran, " "), "%1") {
		t.Fatalf("from kindle-web: %q", ran)
	}
	if got, ran := next(m, "%8"); len(ran) != 0 || got.room == nil || got.room.k.id != "c1" {
		t.Fatalf("from the last match: ran %q room %+v", ran, got.room)
	}
	m = flipFrom(t, m, deskTTY, "%8")
	m.marked = clientMark{deskTTY, m.now()}
	if _, ran := jumped(t, m, tea.KeyPressMsg{Code: 'n', Mod: tea.ModAlt}); !strings.HasSuffix(strings.Join(ran, " "), "%5") {
		t.Fatalf("from the room: %q", ran)
	}
}

var _ = proto.State{}
