package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/proto"
)

// chatTitleInk is the ink of the session's name on the chat's title row.
func chatTitleInk(t *testing.T, m Model) string {
	t.Helper()
	sc := m.draw(m.width)
	s, _ := m.selectedSession()
	x := 0
	for _, sp := range parsed(t, m.render())[sc.bodyTop] {
		if x > sc.listW && strings.Contains(sp.Text, s.Name) {
			return sp.FG
		}
		x += lipgloss.Width(sp.Text)
	}
	t.Fatalf("no title on the chat:\n%s", screen(m))
	return ""
}

// chatEdgeInk is the ink of the line between the list and the chat.
func chatEdgeInk(t *testing.T, m Model) string {
	t.Helper()
	sc := m.draw(m.width)
	x := 0
	for _, sp := range parsed(t, m.render())[sc.bodyTop+1] {
		if x == sc.listW && strings.HasPrefix(sp.Text, "│") {
			return sp.FG
		}
		x += lipgloss.Width(sp.Text)
	}
	t.Fatalf("no edge at column %d:\n%s", sc.listW, screen(m))
	return ""
}

// deskChat is the demo desk with the chat holding the keys, and its fake.
func deskChat(t *testing.T) (Model, *fakeConn) {
	t.Helper()
	m := pressKeys(t, demoSized(t, 132, 32), "enter")
	return m, m.conn.(*fakeConn)
}

// wheelAt turns the wheel once at x, y.
func wheelAt(t *testing.T, m Model, up bool, x, y int) Model {
	t.Helper()
	b := tea.MouseWheelDown
	if up {
		b = tea.MouseWheelUp
	}
	next, cmd := m.Update(tea.MouseWheelMsg{X: x, Y: y, Button: b})
	drain(cmd)
	return next.(Model)
}

// wheelChat turns the wheel over the chat n times.
func wheelChat(t *testing.T, m Model, up bool, n int) Model {
	t.Helper()
	sc := m.draw(m.width)
	for range n {
		m = wheelAt(t, m, up, sc.listW+10, sc.bodyTop+5)
	}
	return m
}

// lastWatch is the text of the last watch act sent.
func lastWatch(t *testing.T, f *fakeConn) proto.Act {
	t.Helper()
	acts := f.allActs()
	for i := len(acts) - 1; i >= 0; i-- {
		if strings.HasPrefix(acts[i].RecID, proto.WatchPrefix) {
			return acts[i]
		}
	}
	t.Fatal("no watch act")
	return proto.Act{}
}

func TestTheWheelOverTheListMovesTheSelection(t *testing.T) {
	m, f := deskChat(t)
	before, _ := m.selectedSession()
	sc := m.draw(m.width)
	m = wheelAt(t, m, false, 5, sc.bodyTop+5)
	if s, _ := m.selectedSession(); s.Pane == before.Pane {
		t.Fatal("the wheel over the list did not move the selection")
	}
	if a := lastWatch(t, f); a.Text != "" {
		t.Fatalf("watch text %q", a.Text)
	}
}

// The chat is a window on the pane: the wheel goes to the agent, and the
// console keeps no history of its own (#288).
func TestTheWheelOverTheChatGoesToTheAgent(t *testing.T) {
	m, f := deskChat(t)
	before := len(keyActs(f))
	m = wheelChat(t, m, true, 1)
	got := keyActs(f)[before:]
	sel, _ := m.selectedSession()
	if len(got) != 1 || got[0].Text != proto.WheelUp || got[0].RecID != proto.KeysPrefix+sel.Pane {
		t.Fatalf("sent %+v", got)
	}
	if !m.chatKeys {
		t.Fatal("the chat gave the keys back")
	}
}

func TestTheWheelDownGoesToTheAgent(t *testing.T) {
	m, f := deskChat(t)
	before := len(keyActs(f))
	wheelChat(t, m, false, 1)
	if got := keyActs(f)[before:]; len(got) != 1 || got[0].Text != proto.WheelDown {
		t.Fatalf("sent %+v", got)
	}
}

// A flick must stop when it stops: steps never pile up behind the agent.
func TestAFlickKeepsOneWheelStepWaiting(t *testing.T) {
	m, _ := deskChat(t)
	m = wheelChat(t, m, true, 20)
	if len(m.keyQ) != 1 || m.keyQ[0].text != proto.WheelUp {
		t.Fatalf("queue %+v", m.keyQ)
	}
}

func TestPageKeysGoToTheAgent(t *testing.T) {
	m, f := deskChat(t)
	before := len(keyActs(f))
	pressKeys(t, m, "pgup")
	if got := keyActs(f)[before:]; len(got) != 1 || got[0].Text != "PPage" {
		t.Fatalf("sent %+v", got)
	}
}

func TestAWatchAsksForTheLiveScreenOnly(t *testing.T) {
	m, f := deskChat(t)
	wheelChat(t, m, true, 3)
	if a := lastWatch(t, f); a.Text != "" {
		t.Fatalf("watch text %q", a.Text)
	}
}

// With the stoker in charge the wheel writes to no pane, and asks nothing:
// looking around never asks. The line says how to take over.
func TestTheWheelWritesNothingWhileTheStokerIsInCharge(t *testing.T) {
	m, f := nightChat(t)
	m = wheelChat(t, m, true, 3)
	if got := keyActs(f); len(got) != 0 || m.held != nil {
		t.Fatalf("sent %+v, held %v", got, m.held != nil)
	}
	if m.flash != "not scrolled: the stoker is in charge — press a key in the chat to take over" {
		t.Fatalf("flash %q", m.flash)
	}
}

// A swipe that opens the chat of a stoker's pane looks and writes nothing.
func TestAWheelThatOpensAStokerChatWritesNothing(t *testing.T) {
	m, f := nightChat(t)
	m = pressKeys(t, m, "ctrl+a", "m")
	m = wheelChat(t, m, true, 1)
	if got := keyActs(f); len(got) != 0 || m.held != nil {
		t.Fatalf("sent %+v, held %v", got, m.held != nil)
	}
}

// PgUp is a key: with the stoker in charge it asks to take over, as any key.
func TestPgUpAsksToTakeOverWhileTheStokerIsInCharge(t *testing.T) {
	m, f := nightChat(t)
	m = pressKeys(t, m, "pgup")
	if got := keyActs(f); len(got) != 0 || m.held == nil {
		t.Fatalf("sent %+v, held %v", got, m.held != nil)
	}
}
