package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/state"
)

// focused is the fixture's state with a focus on the session in pane.
func focused(pane string, at time.Time) proto.State {
	st := fixtureState()
	for _, s := range st.Sessions {
		if s.Pane == pane {
			st.Focus = &state.Focus{SessionID: s.SessionID, Pane: s.Pane, At: at}
		}
	}
	return st
}

// A tap on the island or a buzz for a session that waits opens its chat,
// from any tab, once for each focus.
func TestFocusOpensTheChatOfAWait(t *testing.T) {
	m, _ := loadedWith(t, 100, fixtureState())
	m.setTab(tabGit)
	at := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	next, _ := m.Update(stateMsg(focused("%2", at)))
	nm := next.(Model)
	if s, ok := nm.selectedSession(); nm.tab != tabSessions || !ok || s.Pane != "%2" || !nm.chatKeys {
		t.Fatalf("tab %d selected %q chatKeys %v: want the chat of app-review", nm.tab, s.Pane, nm.chatKeys)
	}
	// The next state carries the same focus: the chat given back stays so.
	nm.chatKeys = false
	next, _ = nm.Update(stateMsg(focused("%2", at)))
	if next.(Model).chatKeys {
		t.Fatal("the same focus opened the chat again")
	}
	// A new focus opens it again.
	next, _ = next.Update(stateMsg(focused("%2", at.Add(time.Second))))
	if !next.(Model).chatKeys {
		t.Fatal("a new focus did not open the chat")
	}
}

// A session at work has no answer to give: the focus opens its pane on the
// phone's own tmux client, once that client is attached, and on no other.
func TestFocusOpensThePaneOfABurn(t *testing.T) {
	m, _ := loadedWith(t, 100, fixtureState())
	var ran []string
	m.opt.Run = func(argv []string) error { ran = append(ran, strings.Join(argv, " ")); return nil }
	asks := 0
	m.opt.Clients = func() ([]string, error) {
		asks++
		if asks < 3 { // the phone's client attaches a moment after the act
			return []string{"/dev/ttys001"}, nil
		}
		return []string{"/dev/ttys001", "/dev/ttys004"}, nil
	}
	st := focused("%1", time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC))
	st.Focus.Client = "/dev/ttys004"
	next, cmd := m.Update(stateMsg(st))
	nm := next.(Model)
	if nm.chatKeys {
		t.Fatal("a session at work got the chat's keys")
	}
	if s, ok := nm.selectedSession(); !ok || s.Pane != "%1" || nm.tab != tabSessions {
		t.Fatalf("selected %q on tab %d", s.Pane, nm.tab)
	}
	runFocus(t, cmd)
	if len(ran) != 1 || ran[0] != strings.Join(remote.FocusArgv("/dev/ttys004", "%1"), " ") || asks < 3 {
		t.Fatalf("ran %q after %d asks", ran, asks)
	}
}

// runFocus runs the commands of a state update, giving each up to one
// seconds: the focus waits for the phone's client.
func runFocus(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	out := make(chan tea.Msg, 1)
	go func() { out <- cmd() }()
	select {
	case msg := <-out:
		if b, ok := msg.(tea.BatchMsg); ok {
			for _, c := range b {
				runFocus(t, c)
			}
		}
	case <-time.After(time.Second):
	}
}

// A console whose tmux has no such client, a focus without one, a remote
// console and one outside tmux never move a terminal: the chat is shared,
// a jump is not.
func TestFocusMovesOnlyThePhonesClient(t *testing.T) {
	old := focusWait
	focusWait = 300 * time.Millisecond
	t.Cleanup(func() { focusWait = old })
	at := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name   string
		client string
		opt    func(*Options)
	}{
		{"another server", "/dev/ttys004", func(*Options) {}},
		{"no client", "", func(*Options) {}},
		{"remote", "/dev/ttys004", func(o *Options) { o.Host = "ws-1" }},
		{"outside tmux", "/dev/ttys004", func(o *Options) { o.OutsideTmux = true }},
	} {
		m, _ := loadedWith(t, 100, fixtureState())
		var ran []string
		m.opt.Run = func(argv []string) error { ran = append(ran, strings.Join(argv, " ")); return nil }
		m.opt.Exec = func(argv []string, done func(error) tea.Msg) tea.Cmd {
			ran = append(ran, strings.Join(argv, " "))
			return nil
		}
		m.opt.Clients = func() ([]string, error) { return []string{"/dev/ttys001"}, nil }
		if c.name != "another server" {
			m.opt.Clients = func() ([]string, error) { return []string{"/dev/ttys001", "/dev/ttys004"}, nil }
		}
		c.opt(&m.opt)
		st := focused("%1", at)
		st.Focus.Client = c.client
		next, cmd := m.Update(stateMsg(st))
		runFocus(t, cmd)
		if s, ok := next.(Model).selectedSession(); !ok || s.Pane != "%1" || len(ran) != 0 {
			t.Errorf("%s: selected %q, ran %q", c.name, s.Pane, ran)
		}
	}
}

// A number whose session ended says so, and nothing else moves.
func TestFocusOfAGoneSessionSaysSo(t *testing.T) {
	m, _ := loadedWith(t, 100, fixtureState())
	var ran []string
	m.opt.Run = func(argv []string) error { ran = append(ran, strings.Join(argv, " ")); return nil }
	m.setTab(tabGit)
	at := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	for _, f := range []*state.Focus{
		{SessionID: "gone-session", Gone: true, At: at},
		// The service saw it, but this console's state no longer has it.
		{SessionID: "22222222-bbbb-4000-8000-000000000009", Pane: "%9", At: at.Add(time.Second)},
	} {
		st := fixtureState()
		st.Focus = f
		next, cmd := m.Update(stateMsg(st))
		nm := next.(Model)
		drain(cmd)
		if nm.flash != "That session ended." || nm.chatKeys || len(ran) != 0 || nm.tab != tabSessions {
			t.Fatalf("%+v: flash %q chatKeys %v ran %q tab %d", f, nm.flash, nm.chatKeys, ran, nm.tab)
		}
		nm.setTab(tabGit)
		m = nm
	}
}

// Without a focus nothing changes: the console stays where it is.
func TestNoFocusLeavesTheConsoleAlone(t *testing.T) {
	m, _ := loadedWith(t, 100, fixtureState())
	m.setTab(tabGit)
	next, _ := m.Update(stateMsg(fixtureState()))
	if nm := next.(Model); nm.tab != tabGit || nm.chatKeys || nm.flash != "" {
		t.Fatalf("tab %d chatKeys %v flash %q", nm.tab, nm.chatKeys, nm.flash)
	}
}

// withFocus is st with a focus on the session in pane.
func withFocus(st proto.State, pane string, at time.Time) proto.State {
	for _, s := range st.Sessions {
		if s.Pane == pane {
			st.Focus = &state.Focus{SessionID: s.SessionID, Pane: s.Pane, At: at}
		}
	}
	return st
}

// A confirm armed on another tab is dropped when a gone focus moves to
// Sessions: the next y must not run an action whose preview is gone.
func TestFocusOfAGoneSessionDropsAConfirm(t *testing.T) {
	m, f := loadedWith(t, 100, fixtureState())
	m.setTab(tabGit)
	m.pending = &action{label: "x", steps: [][]string{{"x"}}}
	st := fixtureState()
	st.Focus = &state.Focus{SessionID: "gone-session", Gone: true, At: time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)}
	next, _ := m.Update(stateMsg(st))
	if nm := next.(Model); nm.tab != tabSessions || nm.pending != nil {
		t.Fatalf("tab %d pending %+v: want Sessions and no confirm", nm.tab, nm.pending)
	}
	_, cmd := key(next, "y")
	drain(cmd)
	if acts := f.acts(); len(acts) != 0 {
		t.Fatalf("y ran %+v", acts)
	}
}

// A focus does not move the chat off an agent the person is typing to: not
// while keys wait to go, nor for a moment after the last one. The key line
// names the agent that asked instead. After the moment the focus moves it.
func TestFocusKeepsAChatBeingTyped(t *testing.T) {
	st := queueState() // app-feature (%1) asks for permission
	m, f := loadedWith(t, 100, st)
	m, now := clocked(m)
	m = pressKeys(t, onPane(t, m, "%2"), "enter")
	*now = now.Add(time.Second)
	m = pressKeys(t, m, "t", "h", "e", "n")
	if len(keyActs(f)) == 0 {
		t.Fatal("no key went")
	}
	at := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	next, _ := m.Update(stateMsg(withFocus(st, "%1", at)))
	m = next.(Model)
	if s, _ := m.selectedSession(); s.Pane != "%2" || !m.chatKeys || !strings.Contains(m.flash, "app-feature") {
		t.Fatalf("a focus while typing moved the chat to %s (chatKeys %v), flash %q", s.Pane, m.chatKeys, m.flash)
	}
	m.keyInFlight, m.keyQ = "", nil // the keys went
	*now = now.Add(chatTypingHold)
	next, _ = m.Update(stateMsg(withFocus(st, "%1", at.Add(time.Second))))
	if s, _ := next.(Model).selectedSession(); s.Pane != "%1" {
		t.Fatalf("a focus after the typing stopped stays on %s", s.Pane)
	}
}

// The home console is shared: an Enter on its way at the desk when the
// phone's tap lands must not answer a question nobody read. A focus starts
// the menu's quiet, on its own session too.
func TestFocusStartsTheMenusQuiet(t *testing.T) {
	st := queueState()
	for i := range st.Sessions {
		if st.Sessions[i].Pane == "%2" {
			st.Sessions[i].Ask = &said.Ask{ID: "toolu_1", Questions: []said.Question{
				{Question: "Which next?", Options: []said.Option{{Label: "Start task 11"}, {Label: "Fix the flake"}}},
			}}
		}
	}
	m, f := loadedWith(t, 100, st)
	next, _ := m.Update(stateMsg(withFocus(st, "%2", time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC))))
	if s, _ := next.(Model).selectedSession(); s.Pane != "%2" || !next.(Model).chatKeys {
		t.Fatalf("the focus opened %q, keys %v", s.Pane, next.(Model).chatKeys)
	}
	next, cmd := key(next, "enter")
	drain(cmd)
	if acts := f.acts(); len(keyActs(f)) != 0 || next.(Model).flash != askedAgain(sessionName(next.(Model))) {
		t.Fatalf("an Enter at once sent %+v (flash %q)", acts, next.(Model).flash)
	}
}

func sessionName(m Model) string {
	s, _ := m.selectedSession()
	return s.Name
}
