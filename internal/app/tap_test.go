package app

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/logos"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
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

// A focus opens the session's pane on the phone's own tmux client, once that
// client is attached, and on no other. As Enter does, it names the pane's
// agent first, so the phone's command keys light in the pane.
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
	if nm.standIn {
		t.Fatal("a session at work got the chat's keys")
	}
	if s, ok := nm.selectedSession(); !ok || s.Pane != "%1" || nm.tab != tabSessions {
		t.Fatalf("selected %q on tab %d", s.Pane, nm.tab)
	}
	msgs := runFocus(t, cmd)
	if len(ran) != 2 || ran[0] != strings.Join(remote.AgentMarkArgv("%1", "claude"), " ") || !isFocus(strings.Fields(ran[1]), "/dev/ttys004", "%1") || asks < 3 {
		t.Fatalf("ran %q after %d asks, want the mark, then the focus of /dev/ttys004 on %%1", ran, asks)
	}
	signal := slices.Index(msgs, tea.Msg(tea.RawMsg{Msg: logos.Passthrough(keycmd.PaneSignal("claude", "%1"))}))
	moved := slices.IndexFunc(msgs, func(msg tea.Msg) bool { r, ok := msg.(ranMsg); return ok && r.jumped == "%1" })
	if signal < 0 || moved < signal {
		t.Errorf("messages %v: want the pane signal before the jump", msgs)
	}
}

// runFocus runs the commands of a state update, a batch's or a sequence's
// each in turn, giving each up to one second: the focus waits for the
// phone's client. It returns the messages they gave.
func runFocus(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	out := make(chan tea.Msg, 1)
	go func() { out <- cmd() }()
	select {
	case msg := <-out:
		if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeFor[tea.Cmd]() {
			var got []tea.Msg
			for i := range v.Len() {
				got = append(got, runFocus(t, v.Index(i).Interface().(tea.Cmd))...)
			}
			return got
		}
		return []tea.Msg{msg}
	case <-time.After(time.Second):
		return nil
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
		if nm.flash != "That session ended." || nm.standIn || len(ran) != 0 || nm.tab != tabSessions {
			t.Fatalf("%+v: flash %q stand-in %v ran %q tab %d", f, nm.flash, nm.standIn, ran, nm.tab)
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
	if nm := next.(Model); nm.tab != tabGit || nm.standIn || nm.flash != "" {
		t.Fatalf("tab %d stand-in %v flash %q", nm.tab, nm.standIn, nm.flash)
	}
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
