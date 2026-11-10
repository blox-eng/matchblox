package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
)

// deskView is the demo desk with the live view beside the list, and its fake.
func deskView(t *testing.T) (Model, *fakeConn) {
	t.Helper()
	m := demoSized(t, 132, 32)
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

// turnWheel turns the wheel over the chat n times.
func turnWheel(t *testing.T, m Model, up bool, n int) Model {
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
	m, f := deskView(t)
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

// The view is a window on the pane: the wheel scrolls it back through the
// service, and the console keeps no history of its own (#288). T14.
func TestTheWheelScrollsTheLiveViewBack(t *testing.T) {
	m, f := loadedWith(t, 100, fixtureState())
	m.tab = tabSessions
	before := len(keyActs(f))
	m = turnWheel(t, m, true, 1)
	got := keyActs(f)[before:]
	if len(got) != 1 || got[0].Text != proto.WheelUp || got[0].RecID != proto.KeysPrefix+"%1" {
		t.Fatalf("sent %+v, want one keys:%%1 %s", got, proto.WheelUp)
	}
	if m.standIn {
		t.Fatal("the wheel opened the view")
	}
}

func TestTheWheelDownScrollsTheLiveViewOn(t *testing.T) {
	m, f := deskView(t)
	before := len(keyActs(f))
	turnWheel(t, m, false, 1)
	if got := keyActs(f)[before:]; len(got) != 1 || got[0].Text != proto.WheelDown {
		t.Fatalf("sent %+v", got)
	}
}

func TestAWatchAsksForTheLiveScreenOnly(t *testing.T) {
	m, f := deskView(t)
	turnWheel(t, m, true, 3)
	for _, a := range f.allActs() {
		if strings.HasPrefix(a.RecID, proto.WatchPrefix) && a.Text != "" {
			t.Fatalf("watch text %q", a.Text)
		}
	}
}
