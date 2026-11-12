package app

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/logos"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/sample"
)

func recordRuns(m *Model) *[][]string {
	var ran [][]string
	m.opt.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	return &ran
}

func TestEveryWayInJumps(t *testing.T) {
	t.Skip("replaced by TestCtrlOGoesIntoTheChosenCard (Task 12 step 1)")
	for _, k := range []string{"enter", "right", "a", "ctrl+o"} {
		m := loaded(t, 100)
		ran := recordRuns(&m)
		next, cmd := key(m, k)
		drain(cmd)
		if nm := next.(Model); nm.standIn || nm.pending != nil {
			t.Errorf("%s: stand-in %v, pending %v: want the jump at once", k, nm.standIn, nm.pending)
		}
		if len(*ran) != 1 || strings.Join((*ran)[0], " ") != "tmux switch-client -t %1" {
			t.Errorf("%s ran %q", k, *ran)
		}
	}
}

// A phone (owner, 2026-11-09, #351 and #352): on one column the first tap
// on a row goes into its pane, as Enter does. A split screen shows the view
// beside the list: the first tap selects the row, and a second tap on it
// goes in.
func TestATapOnARowJumps(t *testing.T) {
	t.Skip("replaced by TestOnlyACardYouChoseGoesIn (Task 12 step 1)")
	m, _ := loadedWith(t, 44, fixtureState())
	ran := recordRuns(&m)
	_, cmd := tap(m, 5, sessLine(t, m, "app-review"))
	drain(cmd)
	if len(*ran) != 1 || strings.Join((*ran)[0], " ") != "tmux switch-client -t %2" {
		t.Fatalf("44 wide: the first tap ran %q, want the jump into %%2", *ran)
	}

	m, _ = loadedWith(t, 100, fixtureState())
	ran = recordRuns(&m)
	next, cmd := tap(m, 5, sessLine(t, m, "app-review"))
	drain(cmd)
	if s, _ := next.(Model).selectedSession(); s.Pane != "%2" || len(*ran) != 0 {
		t.Fatalf("100 wide: the first tap selected %q, ran %q", s.Pane, *ran)
	}
	_, cmd = tap(next, 5, sessLine(t, next.(Model), "app-review"))
	drain(cmd)
	if len(*ran) != 1 || strings.Join((*ran)[0], " ") != "tmux switch-client -t %2" {
		t.Errorf("100 wide: the second tap ran %q", *ran)
	}
}

func TestAPanelessRowSaysSo(t *testing.T) {
	t.Skip("replaced by TestTheCardSaysWhatTheRowSaid (Task 12 step 1)")
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
	t.Skip("dropped (Task 12 step 1): the Ships tab goes (step 6a)")
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
	if next.(Model).standIn || len(*ran) != 2 || !isFocus((*ran)[1], "/dev/ttys004", "%2") {
		t.Errorf("ran %q, stand-in %v: want FocusArgv for the phone", *ran, next.(Model).standIn)
	}
}

// openViewCmd is the selected session as the console shows it once
// selected: on a phone, its row opened in place (#351: no view takes the
// screen).
func openViewCmd(m Model) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	return next.(Model), cmd
}

// keyOrOpen presses k; an Enter on a session, which would go into its pane,
// leaves the console as the selection shows it instead.
func keyOrOpen(m tea.Model, k string) (tea.Model, tea.Cmd) {
	if nm, ok := m.(Model); ok && k == "enter" && !nm.standIn && nm.focus(keyPress(k)) == layerMain {
		if s, ok := nm.selectedSession(); ok && s.Pane != "" {
			return openViewCmd(nm)
		}
	}
	return key(m, k)
}

func openView(t *testing.T, m Model) Model {
	t.Helper()
	m, cmd := openViewCmd(m)
	drain(cmd)
	return m
}

func TestCtrlOInTheViewGoesIntoThePane(t *testing.T) {
	m, _ := loadedWith(t, 44, fixtureState())
	m = openView(t, m)
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
	if len(*ran) != 1 || next.(Model).standIn {
		t.Errorf("ran %q, stand-in %v", *ran, next.(Model).standIn)
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

// settled is fed that also runs what m does with each message: a jump's
// result sends the acts that follow it.
func settled(m tea.Model, cmd tea.Cmd) tea.Model {
	for _, msg := range cmdMsgs(cmd) {
		var then tea.Cmd
		m, then = m.Update(msg)
		drain(then)
	}
	return m
}

func ranFocus(ran [][]string, tty, pane string) bool {
	return slices.ContainsFunc(ran, func(a []string) bool { return isFocus(a, tty, pane) })
}

// isFocus tells if argv is a FocusArgv that moves tty to pane.
func isFocus(argv []string, tty, pane string) bool {
	t, p, ok := remote.FocusOf(argv)
	return ok && t == tty && p == pane
}

// ranMarkThenFocus tells if ran is mark, then the focus of tty to pane.
func ranMarkThenFocus(ran [][]string, mark []string, tty, pane string) bool {
	return len(ran) == 2 && slices.Equal(ran[0], mark) && isFocus(ran[1], tty, pane)
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
	m.opt.ClientList = clients("1794089220 /dev/pts/1 %99") // the iPad left after its mark
	next, cmd := key(marked(m, "/dev/pts/7"), "enter")
	next = fed(next, cmd)
	if ranFocus(*ran, "/dev/pts/7", "%1") || ranFocus(*ran, "/dev/pts/1", "%1") {
		t.Errorf("ran %q, want no jump", *ran)
	}
	if got := next.(Model).flash; got != "this device is not attached: reconnect" {
		t.Errorf("flash %q", got)
	}
}

// A jump whose clients tmux does not list moves no screen: a bare
// switch-client would move the one tmux picks, often the phone.
func TestATmuxErrorMovesNoScreen(t *testing.T) {
	m, ran := jumpRig(t, deskAndPad)
	m.opt.ClientList = func() (string, error) { return "", errors.New("no server running") }
	next, cmd := key(m, "enter")
	next = fed(next, cmd)
	if len(*ran) != 0 {
		t.Errorf("ran %q", *ran)
	}
	if got := next.(Model).flash; got != errNoClient.Error() {
		t.Errorf("flash %q", got)
	}
	m.marked = clientMark{"/dev/pts/7", time.Now()} // a marked key: the same words, not "reconnect"
	next, cmd = key(m, "enter")
	if got := fed(next, cmd).(Model).flash; got != errNoClient.Error() {
		t.Errorf("a marked key's flash %q", got)
	}
}

// A mark names a client that does not show the console: nothing moves it,
// though it is attached to this tmux.
func TestAMarkOfAClientElsewhereMovesNothing(t *testing.T) {
	m, ran := jumpRig(t, deskAndPad+"\n1794089229 /dev/pts/9 %5")
	m.marked = clientMark{"/dev/pts/9", time.Now()}
	next, cmd := key(m, "enter")
	next = fed(next, cmd)
	if len(*ran) != 0 {
		t.Errorf("ran %q", *ran)
	}
	if got := next.(Model).flash; !strings.Contains(got, errNotAttached.Error()) {
		t.Errorf("flash %q", got)
	}
}

func TestAJumpToAGonePaneSaysSo(t *testing.T) {
	m, _ := jumpRig(t, deskAndPad)
	m.opt.Run = func(argv []string) error {
		if isFocus(argv, "/dev/pts/1", "%1") {
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
	t.Skip("replaced by TestAfterAnActTheFirstWaitIsChosen (Task 12 step 1)")
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
	_, cmd := key(marked(m, "/dev/pts/7"), "enter")
	msgs := cmdMsgs(cmd)
	want := tea.RawMsg{Msg: logos.Passthrough(keycmd.PaneSignal("claude", "%1"))}
	if len(msgs) == 0 || msgs[0] != want {
		t.Fatalf("the first message is %v, want %q", msgs, want.Msg)
	}
	if !ranMarkThenFocus(*ran, remote.AgentMarkArgv("%1", "claude"), "/dev/pts/7", "%1") {
		t.Errorf("ran %q, want the mark, then the focus of /dev/pts/7 on %%1", *ran)
	}
}

// The signal reaches every screen on the console, so a jump of the desk, an
// unmarked key, sends none: the phone would take the desk's pane as its own.
func TestTheDesksJumpNamesNoPane(t *testing.T) {
	m, ran := jumpRig(t, deskAndPad)
	_, cmd := key(m, "enter")
	for _, msg := range cmdMsgs(cmd) {
		if r, ok := msg.(tea.RawMsg); ok && strings.Contains(r.Msg.(string), "pane=") {
			t.Fatalf("the desk's jump wrote %q", r.Msg)
		}
	}
	if !ranFocus(*ran, "/dev/pts/1", "%1") {
		t.Errorf("ran %q, want the desk's jump", *ran)
	}
}

// A jump into a pane of an agent with no command keys takes the old mark
// off: the phone's command keys type no other agent's command there.
func TestAJumpToAnAgentWithNoKeysUnmarksThePane(t *testing.T) {
	m, ran := jumpRig(t, deskAndPad)
	i := slices.IndexFunc(m.all.sessions, func(s sample.Session) bool { return s.Pane == "%1" })
	m.all.sessions[i].Agent = "zsh"
	_, cmd := key(m, "enter")
	drain(cmd)
	if !ranMarkThenFocus(*ran, remote.AgentUnmarkArgv("%1"), "/dev/pts/1", "%1") {
		t.Errorf("ran %q, want the unmark, then the focus of /dev/pts/1 on %%1", *ran)
	}
}

// The jump after a switch names the agent the switch started, before the
// jump and on the pane: the sessions still name the one it replaced.
func TestTheJumpAfterASwitchNamesItsNewAgent(t *testing.T) {
	m, ran := jumpRig(t, deskAndPad)
	m.jumpAfter.act, m.jumpAfter.host, m.jumpAfter.pane, m.jumpAfter.done, m.jumpAfter.tty, m.jumpAfter.agent = "a9", m.opt.Host, "%1", "switched app to codex", "/dev/pts/7", "codex"
	_, jump := m.switched(proto.Result{ActID: "a9", Ran: [][]string{{"tmux"}}})
	msgs := cmdMsgs(jump)
	want := tea.RawMsg{Msg: logos.Passthrough(keycmd.PaneSignal("codex", "%1"))}
	if len(msgs) == 0 || msgs[0] != want {
		t.Fatalf("the first message is %v, want %q", msgs, want.Msg)
	}
	if !ranMarkThenFocus(*ran, remote.AgentMarkArgv("%1", "codex"), "/dev/pts/7", "%1") {
		t.Errorf("ran %q, want the codex mark, then the focus of /dev/pts/7 on %%1", *ran)
	}
}

// A jump that waited for its confirm, Enter on Panes, names its pane first too.
func TestAConfirmedJumpNamesThePaneBeforeTheJump(t *testing.T) {
	m, ran := jumpRig(t, deskAndPad)
	m.tab = tabPanes
	next, _ := key(marked(m, "/dev/pts/7"), "enter") // the phone's: the pending jump keeps its tty
	a := next.(Model).pending
	if a == nil || !a.nav || a.pane == "" {
		t.Fatalf("Enter on Panes is pending %+v, want a jump", a)
	}
	_, cmd := key(next, "enter")
	msgs := append(cmdMsgs(cmd), nil)
	if r, ok := msgs[0].(tea.RawMsg); !ok || !strings.Contains(r.Msg.(string), ";pane="+a.pane) || len(*ran) == 0 {
		t.Errorf("the first message is %v, ran %q: want the signal of %s before the jump", msgs, *ran, a.pane)
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
	// tmux tells the console the moment a terminal comes to show it.
	if _, cmd = next.Update(uv.UnknownApcEvent(keycmd.ViewerKey)); !slices.Contains(raws(cmd), marks) { // with the board's keys
		t.Errorf("a terminal that came got %q, want %q at once", raws(cmd), marks)
	}
}

// Building a jump asks tmux nothing, even while a stoker rests and every
// key touches it: the desk is read once, when the jump runs.
func TestOnlyTheRunningJumpReadsTheClients(t *testing.T) {
	st := stokerState(runningNight(time.Now()))
	st.Stoker.Rests = st.At
	m, _ := rigged(sized(t, 100, 30, st), deskAndPad)
	m.tab = tabSessions
	reads := 0
	m.opt.ClientList = func() (string, error) { reads++; return deskAndPad, nil }
	next, _ := key(m, "down")
	next, _ = key(next, "up")
	if reads != 0 || next.(Model).primary() == nil || reads != 0 {
		t.Fatalf("building the action read the clients %d times", reads)
	}
	_, cmd := key(next, "enter")
	if reads != 0 {
		t.Fatalf("Enter read the clients %d times on the update loop", reads)
	}
	drain(cmd)
	if reads != 1 {
		t.Errorf("the jump read the clients %d times, want once", reads)
	}
}

// The phone asks for an agent switch: the jump after it moves the phone.
func TestASwitchJumpsTheClientThatAsked(t *testing.T) {
	next, f := switchState(t, lifeState())
	m, ran := rigged(next.(Model), deskAndPad)
	next, _ = key(marked(m, "/dev/pts/7"), "A")
	next, _ = key(marked(next, "/dev/pts/7"), "enter")
	next, cmd := key(marked(next, "/dev/pts/7"), "y")
	drain(cmd)
	if acts := f.acts(); len(acts) != 1 || acts[0].RecID != "switch:%1" {
		t.Fatalf("sent %+v", acts)
	}
	_, cmd = next.Update(resultMsg(proto.Result{ActID: next.(Model).jumpAfter.act, Ran: [][]string{{"tmux"}}}))
	drain(cmd)
	if !ranFocus(*ran, "/dev/pts/7", "%1") || ranFocus(*ran, "/dev/pts/1", "%1") {
		t.Errorf("ran %q, want FocusArgv for /dev/pts/7 only", *ran)
	}
	var sw proto.SwitchAgent
	if err := json.Unmarshal([]byte(f.acts()[0].Text), &sw); err != nil || !slices.ContainsFunc(*ran, func(a []string) bool { return slices.Equal(a, remote.AgentMarkArgv("%1", sw.Agent)) }) {
		t.Errorf("ran %q, want the pane marked with the new agent %q (%v)", *ran, sw.Agent, err)
	}
}

// intoActs are the take-over acts the console sent.
func intoActs(f *fakeConn) []string {
	var out []string
	for _, id := range recIDs(f.acts()) {
		if strings.HasPrefix(id, proto.IntoPrefix) {
			out = append(out, id)
		}
	}
	return out
}

func TestGoingInWhileTheStokerRestsTakesTheSessionOver(t *testing.T) {
	m, f := loadedWith(t, 100, restingState())
	ran := recordRuns(&m)
	s, _ := m.selectedSession()
	next, cmd := key(m, "enter")
	settled(next, cmd)
	if got := intoActs(f); len(got) != 1 || got[0] != proto.IntoPrefix+s.Pane {
		t.Fatalf("into acts %q, want into:%s", got, s.Pane)
	}
	if len(*ran) != 1 || (*ran)[0][1] != "switch-client" {
		t.Errorf("ran %q, want the jump", *ran)
	}
}

func TestGoingInWhileTheStokerIsInChargeLeavesItTheSession(t *testing.T) {
	st := nightState()
	st.Stoker.Asked = true // n kept it on: Enter asks no more
	m, f := loadedWith(t, 100, st)
	ran := recordRuns(&m)
	next, cmd := key(m, "enter")
	settled(next, cmd)
	if got := intoActs(f); len(got) != 0 {
		t.Fatalf("into acts %q while the stoker is in charge", got)
	}
	if len(*ran) != 1 {
		t.Errorf("ran %q, want the jump", *ran)
	}
}

func TestTakingOverThenGoingInFreesTheSession(t *testing.T) {
	m, f := loadedWith(t, 100, nightState())
	recordRuns(&m)
	s, _ := m.selectedSession()
	next, _ := key(m, "enter")
	next, cmd := key(next, "y")
	settled(next, cmd)
	got := recIDs(f.acts())
	if i, j := slices.Index(got, "stoker:rest"), slices.Index(got, proto.IntoPrefix+s.Pane); i < 0 || j < i {
		t.Fatalf("acts %q, want stoker:rest, then into:%s", got, s.Pane)
	}
}

// A jump that did not go frees nothing: the session stays the stoker's
// while no one is in it.
func TestARefusedJumpFreesNothing(t *testing.T) {
	m, f := loadedWith(t, 100, restingState())
	m.opt.Run = func([]string) error { return errors.New("can't find pane") }
	next, cmd := key(m, "enter")
	settled(next, cmd)
	if got := intoActs(f); len(got) != 0 {
		t.Fatalf("into acts %q for a jump that failed", got)
	}
}

// The signal goes raw outside tmux, through passthrough inside, and not at
// all through a tmux too old to pass it.
func TestTheSignalGoesTheWayTheTerminalReadsIt(t *testing.T) {
	say := keycmd.MarksSignal()
	if got := sayAgent(say, Options{OutsideTmux: true})(); got != (tea.RawMsg{Msg: say}) {
		t.Errorf("outside tmux wrote %v", got)
	}
	if sayAgent(say, Options{LocalTmux: "3.2a"}) != nil {
		t.Error("tmux 3.2a got the signal")
	}
	if got := sayAgent(say, Options{LocalTmux: "3.4"})(); got != (tea.RawMsg{Msg: logos.Passthrough(say)}) {
		t.Errorf("tmux 3.4 wrote %v", got)
	}
}

// A jump that leaves no screen on the console stops the watch: the service
// reads no hidden pane while the person types in the agent's. The watch
// comes back when a screen shows the console again, or the person acts there.
func TestAJumpWithNoOneLeftStopsTheWatch(t *testing.T) {
	m, f := loadedWith(t, 160, fixtureState()) // the board watches beside its live view
	m, _ = rigged(m, "1794089220 /dev/pts/1 %99")
	s, _ := m.selectedSession()
	next, cmd := key(m, "enter")
	next = settled(next, cmd)
	if a := lastWatch(t, f); a.RecID != proto.WatchPrefix {
		t.Fatalf("after the jump the watch is %q, want none", a.RecID)
	}
	_, cmd = next.Update(uv.UnknownApcEvent(keycmd.ViewerKey))
	drain(cmd)
	if c, ok := next.(Model).boardSelected(); ok {
		s = c // after the jump the board chose the first wait (R3.2)
	}
	if a := lastWatch(t, f); a.RecID != proto.WatchPrefix+s.Pane {
		t.Errorf("back on the console the watch is %q, want %s", a.RecID, s.Pane)
	}

	m, f = loadedWith(t, 160, fixtureState()) // the board watches beside its live view
	m, _ = rigged(m, deskAndPad)
	next, cmd = key(m, "enter")
	settled(next, cmd)
	if slices.ContainsFunc(f.allActs(), func(a proto.Act) bool { return a.RecID == proto.WatchPrefix }) {
		t.Error("the watch stopped with the iPad still on the console")
	}
}
