package app

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/logos"
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

// rigged is m inside tmux, in the console pane, with the clients of list
// attached and /dev/pts/7 an iPad's.
func rigged(m Model, list string) (Model, *[][]string) {
	m.opt.Pane, m.opt.ClientList = console, clients(list)
	var ttys []string
	for _, c := range clientsOf(list, "") {
		ttys = append(ttys, c.tty)
	}
	m.opt.Clients = func() ([]string, error) { return ttys, nil }
	m.opt.Pair = &PairHost{DeviceTTYs: func() map[string]string { return map[string]string{"/dev/pts/7": "iPad-id"} }}
	return m, recordRuns(&m)
}

func jumpRig(t *testing.T, list string) (Model, *[][]string) {
	t.Helper()
	return rigged(loaded(t, 100), list)
}

const deskAndPad = "1794089228 /dev/pts/7 %99\n1794089220 /dev/pts/1 %99"

// fed runs cmd and hands each message it gives back to m.
func fed(m tea.Model, cmd tea.Cmd) tea.Model {
	for _, msg := range cmdMsgs(cmd) {
		m, _ = m.Update(msg)
	}
	return m
}

func ranFocus(ran [][]string, tty, pane string) bool {
	return slices.ContainsFunc(ran, func(a []string) bool { return slices.Equal(a, remote.FocusArgv(tty, pane)) })
}

func TestAMarkedEnterMovesOnlyThatClient(t *testing.T) {
	m, ran := jumpRig(t, deskAndPad)
	_, cmd := key(marked(m, "/dev/pts/7"), "enter")
	drain(cmd)
	if !ranFocus(*ran, "/dev/pts/7", "%1") || ranFocus(*ran, "/dev/pts/1", "%1") {
		t.Errorf("ran %q, want FocusArgv for /dev/pts/7 only", *ran)
	}
}

func TestAnUnmarkedEnterMovesTheDesk(t *testing.T) {
	m, ran := jumpRig(t, deskAndPad)
	_, cmd := key(m, "enter")
	drain(cmd)
	if !ranFocus(*ran, "/dev/pts/1", "%1") || ranFocus(*ran, "/dev/pts/7", "%1") {
		t.Errorf("ran %q, want FocusArgv for the desk, /dev/pts/1", *ran)
	}
}

func TestAMarkedKeyOfAGoneClientIsRefused(t *testing.T) {
	m, ran := jumpRig(t, deskAndPad)
	m.opt.Clients = func() ([]string, error) { return []string{"/dev/pts/1"}, nil }
	next, cmd := key(marked(m, "/dev/pts/7"), "enter")
	next = fed(next, cmd)
	if ranFocus(*ran, "/dev/pts/7", "%1") || ranFocus(*ran, "/dev/pts/1", "%1") {
		t.Errorf("ran %q, want no jump", *ran)
	}
	if got := next.(Model).flash; got != "this device is not attached: reconnect" {
		t.Errorf("flash %q", got)
	}
}

func TestATmuxErrorJumpsAsBefore(t *testing.T) {
	m, ran := jumpRig(t, deskAndPad)
	m.opt.ClientList = func() (string, error) { return "", errors.New("no server running") }
	_, cmd := key(m, "enter")
	drain(cmd)
	if want := [][]string{{"tmux", "switch-client", "-t", "%1"}}; !slices.EqualFunc(*ran, want, slices.Equal) {
		t.Errorf("ran %q, want %q", *ran, want)
	}
}

func TestAJumpToAGonePaneSaysSo(t *testing.T) {
	m, _ := jumpRig(t, deskAndPad)
	m.opt.Run = func(argv []string) error {
		if slices.Equal(argv, remote.FocusArgv("/dev/pts/1", "%1")) {
			return errors.New("can't find pane: %1")
		}
		return nil
	}
	next, cmd := key(m, "enter")
	next = fed(next, cmd)
	nm := next.(Model)
	if !strings.HasPrefix(nm.flash, "failed: tmux switch-client -c /dev/pts/1 -t %1") {
		t.Errorf("flash %q", nm.flash)
	}
	if s, _ := nm.selectedSession(); s.Pane != "%1" {
		t.Errorf("the selection moved to %q", s.Pane)
	}
}

// The phone asks, the desk answers take over?: the phone still goes in. A
// tap, which asks with no key held, does the same.
func TestTakeOverAnsweredFromTheDeskJumpsThePhone(t *testing.T) {
	m, ran := rigged(sized(t, 100, 30, stokerState(runningNight(time.Now()))), deskAndPad)
	held, _ := key(marked(m, "/dev/pts/7"), "enter")
	if held.(Model).held == nil || len(*ran) != 0 {
		t.Fatalf("held %v ran %q: want take over? first", held.(Model).held != nil, *ran)
	}
	_, cmd := key(held, "n")
	drain(cmd)
	if !ranFocus(*ran, "/dev/pts/7", "%1") || ranFocus(*ran, "/dev/pts/1", "%1") {
		t.Errorf("Enter: ran %q, want FocusArgv for /dev/pts/7 only", *ran)
	}

	m, ran = rigged(sized(t, 100, 30, stokerState(runningNight(time.Now()))), deskAndPad)
	m.tab = tabSessions
	y := sessLine(t, m, "app-feature")
	next, cmd := tap(m, 5, y)
	drain(cmd)
	held, _ = tap(marked(next, "/dev/pts/7"), 5, y)
	if held.(Model).held == nil || len(*ran) != 0 {
		t.Fatalf("tap: held %v ran %q: want take over? first", held.(Model).held != nil, *ran)
	}
	_, cmd = key(held, "n")
	drain(cmd)
	if !ranFocus(*ran, "/dev/pts/7", "%1") || ranFocus(*ran, "/dev/pts/1", "%1") {
		t.Errorf("tap: ran %q, want FocusArgv for /dev/pts/7 only", *ran)
	}
}

func TestTheSelectionMovesOnWhenNoOneElseWatches(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	next, _ := m.Update(ranMsg{cmd: "tmux switch-client -c /dev/pts/7 -t %1", jumped: "%1", tty: "/dev/pts/7"})
	if s, _ := next.(Model).selectedSession(); s.Pane != "%2" {
		t.Errorf("the selection is %q, want the next that waits, %%2", s.Pane)
	}
}

func TestTheSelectionStaysWhileTheDeskWatches(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	next, _ := m.Update(ranMsg{cmd: "tmux switch-client -c /dev/pts/7 -t %1", jumped: "%1", tty: "/dev/pts/7", watched: true})
	if s, _ := next.(Model).selectedSession(); s.Pane != "%1" {
		t.Errorf("the selection moved to %q while the desk watches", s.Pane)
	}
	for list, want := range map[string]bool{deskAndPad: true, "1794089228 /dev/pts/7 %99\n1794089220 /dev/pts/1 %4": false} {
		m, _ := jumpRig(t, list)
		a := m.jump("%1")
		a.tty = "/dev/pts/7"
		if got := m.intoCmd(*a)().(ranMsg); got.watched != want || got.err != nil {
			t.Errorf("%q: watched %v (err %v), want %v", list, got.watched, got.err, want)
		}
	}
}

func TestTheSignalNamesThePaneBeforeTheJump(t *testing.T) {
	m, ran := jumpRig(t, deskAndPad)
	_, cmd := key(m, "enter")
	msgs := cmdMsgs(cmd)
	want := tea.RawMsg{Msg: logos.Passthrough(keycmd.PaneSignal("claude", "%1"))}
	if len(msgs) == 0 || msgs[0] != want {
		t.Fatalf("the first message is %v, want %q", msgs, want.Msg)
	}
	if jump := [][]string{remote.AgentMarkArgv("%1", "claude"), remote.FocusArgv("/dev/pts/1", "%1")}; !slices.EqualFunc(*ran, jump, slices.Equal) {
		t.Errorf("ran %q, want %q", *ran, jump)
	}
}

func TestTheConsoleAnnouncesTheMarks(t *testing.T) {
	marks := logos.Passthrough(keycmd.MarksSignal())
	for _, start := range []string{"", "here"} {
		r := newShell(t, []string{"here"}, nil, start, false)
		if got := raws(r.sh.Init()); !slices.Equal(got, []string{marks}) {
			t.Errorf("start %q wrote %q, want %q", start, got, marks)
		}
	}
	m := loaded(t, 100)
	next, cmd := m.Update(graphicsMsg{gen: m.opt.Gen, watched: true, key: "/dev/pts/1:100@here"})
	if got := raws(cmd); !slices.Contains(got, marks) {
		t.Errorf("the first viewers got %q, want %q", got, marks)
	}
	if _, cmd = next.Update(graphicsMsg{gen: m.opt.Gen, watched: true, key: "/dev/pts/1:100@here"}); slices.Contains(raws(cmd), marks) {
		t.Errorf("the same viewers got %q again", raws(cmd))
	}
	if _, cmd = next.Update(graphicsMsg{gen: m.opt.Gen, watched: true, key: "/dev/pts/1:100@here,/dev/pts/7:105@here"}); !slices.Contains(raws(cmd), marks) {
		t.Errorf("a phone that attached got %q, want %q", raws(cmd), marks)
	}
}
