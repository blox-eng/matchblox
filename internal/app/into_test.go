package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/remote"
)

func recordRuns(m *Model) *[][]string {
	var ran [][]string
	m.opt.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	return &ran
}

func TestEveryWayInJumps(t *testing.T) {
	for _, k := range []string{"enter", "right", "a", "ctrl+o"} {
		m := loaded(t, 100)
		ran := recordRuns(&m)
		next, cmd := key(m, k)
		drain(cmd)
		if nm := next.(Model); nm.chatKeys || nm.pending != nil {
			t.Errorf("%s: chat %v, pending %v: want the jump at once", k, nm.chatKeys, nm.pending)
		}
		if len(*ran) != 1 || strings.Join((*ran)[0], " ") != "tmux switch-client -t %1" {
			t.Errorf("%s ran %q", k, *ran)
		}
	}
}

// A phone keeps the live view: the first tap on a row selects it, a tap on
// the selected row goes into its pane. A split screen does the same.
func TestATapOnARowJumps(t *testing.T) {
	for _, w := range []int{44, 100} {
		m, _ := loadedWith(t, w, fixtureState())
		ran := recordRuns(&m)
		next, cmd := tap(m, 5, sessLine(t, m, "app-review"))
		drain(cmd)
		if s, _ := next.(Model).selectedSession(); s.Pane != "%2" || len(*ran) != 0 || next.(Model).chatKeys {
			t.Fatalf("width %d: the first tap selected %q, ran %q, chat %v", w, s.Pane, *ran, next.(Model).chatKeys)
		}
		next, cmd = tap(next, 5, sessLine(t, next.(Model), "app-review"))
		drain(cmd)
		if len(*ran) != 1 || strings.Join((*ran)[0], " ") != "tmux switch-client -t %2" || next.(Model).chatKeys {
			t.Errorf("width %d: the second tap ran %q, chat %v", w, *ran, next.(Model).chatKeys)
		}
	}
}

func TestAPanelessRowSaysSo(t *testing.T) {
	m, _ := loadedWith(t, 100, outsideTmux(fixtureState()))
	m.tab = tabSessions
	s, _ := m.selectedSession()
	for _, k := range []string{"enter", "a", "right", "ctrl+o"} {
		next, _ := key(m, k)
		if got, want := next.(Model).flash, s.Name+" runs outside tmux: no pane to go into"; got != want {
			t.Errorf("%s: flash %q, want %q", k, got, want)
		}
	}
}

func TestEnterOnAStokerHostAsksToTakeOver(t *testing.T) {
	m := sized(t, 100, 30, stokerState(runningNight(time.Now())))
	ran := recordRuns(&m)
	next, _ := key(m, "enter")
	if nm := next.(Model); nm.held == nil || len(*ran) != 0 {
		t.Fatalf("held %v ran %q: want take over? first", nm.held != nil, *ran)
	}
	_, cmd := key(next, "n") // keep the stoker: the jump still runs
	drain(cmd)
	if len(*ran) != 1 || (*ran)[0][1] != "switch-client" {
		t.Errorf("after the answer ran %q", *ran)
	}
}

func TestAOnAStuckShipJumps(t *testing.T) {
	m, _ := shipsAt(t, 86, screensState(t))
	ran := recordRuns(&m)
	next, _ := key(m, "down")
	next, _ = key(next, "down") // #4545 is stuck; flint-auth asks
	_, cmd := key(next, "a")
	drain(cmd)
	if len(*ran) != 1 || strings.Join((*ran)[0], " ") != "tmux switch-client -t %2" {
		t.Fatalf("a on a stuck ship ran %q", *ran)
	}
	next, cmd = key(m, "a") // #4551 runs
	drain(cmd)
	if nm := next.(Model); len(*ran) != 1 || nm.flash != "a goes into the session of a stuck ship" {
		t.Errorf("a on a ship that runs: ran %q, flash %q", *ran, nm.flash)
	}
}

func TestAOnAFailedStepJumps(t *testing.T) {
	m, _ := loadedWith(t, 100, tabMorning(screensState(t)))
	m.setTab(tabStoker)
	pane := ""
	for _, s := range m.all.sessions {
		if s.Name == "kindle-web" {
			pane = s.Pane
		}
	}
	for i, e := range m.stoker.Run {
		if e.Step == 5 {
			m.stoker.Run[i].Pane = pane
		}
	}
	ran := recordRuns(&m)
	if line := actionLine(selectStep(t, m, "p5")); !strings.Contains(line, "a go in") {
		t.Errorf("the key line does not offer a: %q", line)
	}
	_, cmd := key(selectStep(t, m, "p5"), "a")
	drain(cmd)
	if len(*ran) != 1 || strings.Join((*ran)[0], " ") != "tmux switch-client -t "+pane {
		t.Fatalf("a on a failed step ran %q", *ran)
	}
	next, _ := key(selectStep(t, m, "p2"), "a")
	if nm := next.(Model); len(*ran) != 1 || nm.flash != "a goes into the session of a failed step" {
		t.Errorf("a on a done step: ran %q, flash %q", *ran, nm.flash)
	}
}

func TestAFocusOnAWaitGoesIntoThePane(t *testing.T) {
	m, _ := loadedWith(t, 100, fixtureState())
	ran := recordRuns(&m)
	m.opt.Clients = func() ([]string, error) { return []string{"/dev/ttys004"}, nil }
	st := focused("%2", time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC))
	st.Focus.Client = "/dev/ttys004"
	next, cmd := m.Update(stateMsg(st))
	runFocus(t, cmd)
	if next.(Model).chatKeys || len(*ran) != 1 || !slices.Equal((*ran)[0], remote.FocusArgv("/dev/ttys004", "%2")) {
		t.Errorf("ran %q, chat %v: want FocusArgv for the phone", *ran, next.(Model).chatKeys)
	}
}

// openChatCmd gives the keys to the chat of the selected session, as the
// demo does until the live view replaces it, with the commands that opens it.
func openChatCmd(m Model) (Model, tea.Cmd) {
	s, _ := m.selectedSession()
	next, cmd := m.chatOn(s.Pane).Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	return next.(Model), cmd
}

// keyOrOpen presses k; an Enter on a session that would have opened its chat
// gives the chat the keys instead.
func keyOrOpen(m tea.Model, k string) (tea.Model, tea.Cmd) {
	if nm, ok := m.(Model); ok && k == "enter" && !nm.chatKeys && nm.focus(keyPress(k)) == layerMain {
		if s, ok := nm.selectedSession(); ok && s.Pane != "" {
			return openChatCmd(nm)
		}
	}
	return key(m, k)
}

func openChat(t *testing.T, m Model) Model {
	t.Helper()
	if m.chatKeys {
		t.Error("the chat already has the keys: Enter goes to its agent")
	}
	m, cmd := openChatCmd(m)
	drain(cmd)
	return m
}

func TestCtrlOInTheChatGoesIntoThePane(t *testing.T) {
	m, _ := inChat(t)
	ran := recordRuns(&m)
	_, cmd := key(m, "ctrl+o")
	drain(cmd)
	if len(*ran) != 1 || strings.Join((*ran)[0], " ") != "tmux switch-client -t %1" {
		t.Fatalf("ran %q", *ran)
	}
}

func TestAClickRightOfTheListGoesIn(t *testing.T) {
	m := loaded(t, 200)
	ran := recordRuns(&m)
	sc := m.draw(200)
	next, cmd := m.Update(tea.MouseClickMsg(tea.Mouse{X: sc.listW + 5, Y: sc.bodyTop + 1, Button: tea.MouseLeft}))
	drain(cmd)
	if len(*ran) != 1 || next.(Model).chatKeys {
		t.Errorf("ran %q, chat %v", *ran, next.(Model).chatKeys)
	}
}

// On a host the stoker drives, every tap that goes in asks to take over first.
func TestATapIntoAStokerHostAsksFirst(t *testing.T) {
	night := func(w int) (Model, *[][]string) {
		m := sized(t, w, 30, stokerState(runningNight(time.Now())))
		m.tab = tabSessions
		return m, recordRuns(&m)
	}
	asked := func(t *testing.T, name string, next tea.Model, ran *[][]string, cmd tea.Cmd) {
		t.Helper()
		drain(cmd)
		if next.(Model).held == nil || len(*ran) != 0 {
			t.Fatalf("%s: held %v ran %q: want take over? first", name, next.(Model).held != nil, *ran)
		}
		_, cmd = key(next, "n")
		drain(cmd)
		if len(*ran) != 1 {
			t.Errorf("%s: after the answer ran %q", name, *ran)
		}
	}

	m, ran := night(100)
	y := sessLine(t, m, "app-feature")
	next, cmd := tap(m, 5, y)
	drain(cmd)
	next, cmd = tap(next, 5, y)
	asked(t, "second tap on the selected row", next, ran, cmd)

	m, ran = night(200)
	sc := m.draw(200)
	next, cmd = m.Update(tea.MouseClickMsg(tea.Mouse{X: sc.listW + 5, Y: sc.bodyTop + 1, Button: tea.MouseLeft}))
	asked(t, "tap right of the list", next, ran, cmd)

	st := cardState("toolu_1", threeWays)
	st.Stoker = stokerState(runningNight(time.Now())).Stoker
	m, _ = loadedWith(t, 140, st)
	m = onPane(t, m, "%1")
	next, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	next, _ = next.Update(screenMsg{Pane: "%1", At: m.now(), Text: permitScreen})
	m = pastQuiet(next.(Model))
	ran = recordRuns(&m)
	next, cmd = tap(m, 110, rowWith(t, m, "2. Yes, and don't ask again"))
	asked(t, "tap on a menu option", next, ran, cmd)
}

func TestAOnAStuckShipAtNightAsksToTakeOver(t *testing.T) {
	st := stokerState(runningNight(time.Now()))
	full := screensState(t)
	st.Ships, st.Sessions = full.Ships, full.Sessions
	m := sized(t, 86, 30, st)
	ran := recordRuns(&m)
	next, _ := key(m, "p")
	next, _ = key(next, "down")
	next, _ = key(next, "down") // #4545 is stuck
	asked, cmd := key(next, "a")
	drain(cmd)
	if asked.(Model).held == nil || len(*ran) != 0 {
		t.Fatalf("held %v ran %q: want take over? first", asked.(Model).held != nil, *ran)
	}
	_, cmd = key(asked, "n")
	drain(cmd)
	if len(*ran) != 1 {
		t.Errorf("after the answer ran %q", *ran)
	}
}
