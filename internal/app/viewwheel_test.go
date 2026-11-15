package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
)

// deskView is the demo desk, the list beside the chat, and its fake.
func deskView(t *testing.T) (Model, *fakeConn) {
	t.Helper()
	m := demoSized(t, 160, 32)
	m.sideOn = true
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
