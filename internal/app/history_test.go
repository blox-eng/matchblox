package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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

// historyCame is m with a screen of the history up lines up.
func historyCame(t *testing.T, m Model, up int) Model {
	t.Helper()
	s, _ := m.selectedSession()
	next, cmd := m.Update(screenMsg{Pane: s.Pane, At: demoAt.Add(time.Duration(up) * time.Second), Text: "history", Up: up})
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

func TestTheWheelScrollsTheChatUpAndBack(t *testing.T) {
	m, f := deskChat(t)
	s, _ := m.selectedSession()
	for i, want := range []string{"up:3", "up:6"} {
		m = wheelChat(t, m, true, 1)
		if a := lastWatch(t, f); a.RecID != proto.WatchPrefix+s.Pane || a.Text != want {
			t.Fatalf("wheel up %d: watch %q %q, want %q", i+1, a.RecID, a.Text, want)
		}
	}
	for i, want := range []string{"up:3", "up:0"} {
		m = wheelChat(t, m, false, 1)
		if a := lastWatch(t, f); a.Text != want {
			t.Fatalf("wheel down %d: watch %q, want %q", i+1, a.Text, want)
		}
	}
	if s2, _ := m.selectedSession(); !m.chatKeys || s2.Pane != s.Pane {
		t.Fatalf("chatKeys %v on %q: the wheel moved off the chat", m.chatKeys, s2.Name)
	}
}

func TestAChatUpSaysSoInADimFrame(t *testing.T) {
	m, _ := deskChat(t)
	m = wheelChat(t, m, true, 2)
	sc := m.draw(m.width)
	if title := ansi.Strip(sc.lines[sc.bodyTop]); strings.Contains(title, "lines up") {
		t.Fatalf("title %q before the history came", title)
	}
	if ink := chatEdgeInk(t, m); ink != "hair" {
		t.Fatalf("a chat that waits for the history has its edge in %q", ink)
	}
	m = historyCame(t, m, 6)
	sc = m.draw(m.width)
	if title := ansi.Strip(sc.lines[sc.bodyTop]); !strings.Contains(title, "↑ 6 lines up") {
		t.Fatalf("title %q", title)
	}
	if ink := chatTitleInk(t, m); ink == "accent" {
		t.Fatal("a chat up the history has its title in the accent")
	}
	if ink := chatEdgeInk(t, m); ink != "hair" {
		t.Fatalf("a chat up the history has its edge in %q", ink)
	}
	if keys := ansi.Strip(sc.lines[sc.keysAt]); !strings.Contains(keys, "a key goes back to live") || strings.Contains(keys, "keys go to") {
		t.Fatalf("key line %q", keys)
	}
}

// Back from the history, the chat is blank until the live screen comes,
// and a key until then goes nowhere: it would answer a screen not shown.
func TestBackToLiveWaitsForTheLiveScreen(t *testing.T) {
	m, f := deskChat(t)
	s, _ := m.selectedSession()
	m = historyCame(t, wheelChat(t, m, true, 1), 3)
	m = pressKeys(t, m, "x")
	if m.screen.Text != "" || !m.notLive() {
		t.Fatalf("screen %+v after going back to live:\n%s", m.screen, screen(m))
	}
	n := len(f.acts())
	m = pressKeys(t, m, "enter")
	if got := f.acts()[n:]; len(got) != 0 || m.keyInFlight != "" {
		t.Fatalf("an Enter before the live screen sent %+v", got)
	}
	next, cmd := m.Update(screenMsg{Pane: s.Pane, At: demoAt.Add(time.Minute), Text: askScreen})
	drain(cmd)
	m = pressKeys(t, next.(Model), "enter")
	got := f.acts()[n:]
	if len(got) != 1 || got[0].Text != "Enter" || !got[0].Seen.Equal(demoAt.Add(time.Minute)) {
		t.Fatalf("an Enter on the live screen sent %+v", got)
	}
}

func TestAPhoneChatUpSaysSo(t *testing.T) {
	m := pressKeys(t, demoSized(t, 44, 31), "enter")
	m = historyCame(t, wheelAt(t, m, true, 10, 10), 3)
	if title := ansi.Strip(m.draw(44).lines[0]); !strings.Contains(title, "↑ 3 lines up") {
		t.Fatalf("title %q", title)
	}
}

func TestAKeyWhileUpOnlyGoesBackToLive(t *testing.T) {
	for _, press := range []func(Model) Model{
		func(m Model) Model { return pressKeys(t, m, "x") },
		func(m Model) Model { return pressKeys(t, m, "enter") },
		func(m Model) Model { return pressKeys(t, m, "ctrl+]") },
		func(m Model) Model {
			next, cmd := m.Update(tea.PasteMsg{Content: "hello"})
			drain(cmd)
			return next.(Model)
		},
	} {
		m, f := deskChat(t)
		m = wheelChat(t, m, true, 2)
		n := len(f.acts())
		m = press(m)
		if got := f.acts()[n:]; len(got) != 0 || m.keyInFlight != "" || len(m.keyQ) != 0 {
			t.Fatalf("sent %+v while up", got)
		}
		if m.flash != "back to live — the agent did not get that key" || !m.chatKeys {
			t.Fatalf("flash %q chatKeys %v", m.flash, m.chatKeys)
		}
		if a := lastWatch(t, f); a.Text != "up:0" {
			t.Fatalf("watch %q, want up:0", a.Text)
		}
		if strings.Contains(screen(m), "lines up") {
			t.Fatalf("still up:\n%s", screen(m))
		}
	}
}

func TestACardKeyWhileUpOnlyGoesBackToLive(t *testing.T) {
	m := pressKeys(t, demoSized(t, 132, 32), "down")
	for s, ok := m.selectedSession(); ok && s.Name != "flint-auth"; s, ok = m.selectedSession() {
		m = pressKeys(t, m, "down")
	}
	m = pressKeys(t, m, "enter")
	f := m.conn.(*fakeConn)
	if !m.chatKeys || !m.cardDrawn() {
		t.Fatal("no card on the chat")
	}
	m = wheelChat(t, m, true, 1)
	n := len(f.acts())
	m = pressKeys(t, m, "1")
	if got := f.acts()[n:]; len(got) != 0 || m.flash != "back to live — the agent did not get that key" {
		t.Fatalf("sent %+v, flash %q", got, m.flash)
	}
}

func TestATapOnTheTitleWhileUpGoesBackToLive(t *testing.T) {
	m, f := deskChat(t)
	m = wheelChat(t, m, true, 1)
	sc := m.draw(m.width)
	next, cmd := tap(m, sc.listW+4, sc.bodyTop)
	drain(cmd)
	m = next.(Model)
	if a := lastWatch(t, f); a.Text != "up:0" || !m.chatKeys {
		t.Fatalf("watch %q chatKeys %v", a.Text, m.chatKeys)
	}
	phone := pressKeys(t, demoSized(t, 44, 31), "enter")
	phone = wheelAt(t, phone, true, 10, 10)
	next, cmd = tap(phone, 3, 0)
	drain(cmd)
	if phone = next.(Model); !phone.chatKeys || phone.up != 0 {
		t.Fatalf("a phone's title tap while up: chatKeys %v up %d", phone.chatKeys, phone.up)
	}
}

func TestLeavingTheChatGoesBackToLive(t *testing.T) {
	m, f := deskChat(t)
	m = wheelChat(t, m, true, 2)
	before, _ := m.selectedSession()
	other := ""
	for _, s := range m.snap.Sessions {
		if s.Pane != "" && s.Pane != before.Pane {
			other = s.Name
			break
		}
	}
	next, cmd := tap(m, 5, sessLine(t, m, other))
	drain(cmd)
	m = next.(Model)
	if m.up != 0 {
		t.Fatalf("up %d after the selection moved", m.up)
	}
	if a := lastWatch(t, f); a.Text != "up:0" {
		t.Fatalf("watch %q %q, want up:0", a.RecID, a.Text)
	}
}

func TestTheWheelOverTheListMovesTheSelection(t *testing.T) {
	m, f := deskChat(t)
	before, _ := m.selectedSession()
	sc := m.draw(m.width)
	m = wheelAt(t, m, false, 5, sc.bodyTop+5)
	if s, _ := m.selectedSession(); s.Pane == before.Pane {
		t.Fatal("the wheel over the list did not move the selection")
	}
	if a := lastWatch(t, f); a.Text != "up:0" {
		t.Fatalf("watch %q, want up:0", a.Text)
	}
}

// A screen of the history that comes after the chat went back to live is
// not shown: it is not the live screen.
func TestAHistoryScreenAfterLiveIsIgnored(t *testing.T) {
	m, _ := deskChat(t)
	s, _ := m.selectedSession()
	m = wheelChat(t, m, true, 1)
	m = pressKeys(t, m, "x")
	next, _ := m.Update(screenMsg{Pane: s.Pane, Text: "old history", Up: 3})
	if got := next.(Model).screen; got.Up != 0 || got.Text == "old history" {
		t.Fatalf("screen %+v", got)
	}
}
