package app

import (
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

func TestFitKeepsTheBottomAndCutsRules(t *testing.T) {
	prose := ""
	for i := 0; i < 180; i++ {
		prose += string(rune('a' + i%26))
	}
	var lines []string
	for i := 0; i < 39; i++ {
		if i%2 == 0 {
			lines = append(lines, strings.Repeat("─", 200))
		} else {
			lines = append(lines, prose)
		}
	}
	lines = append(lines, "> ")
	got := fitScreen(strings.Join(lines, "\n"), 44, 20)
	if len(got) != 20 {
		t.Fatalf("got %d lines, want 20", len(got))
	}
	full := false
	for i, l := range got {
		w := ansi.StringWidth(l)
		if w > 44 || strings.Contains(l, "─") && w != 44 {
			t.Fatalf("line %d is %d wide", i, w)
		}
		full = full || w == 44
	}
	if !full {
		t.Fatal("no line is 44 wide")
	}
	if strings.TrimRight(got[19], " ") != ">" {
		t.Fatalf("last line = %q", got[19])
	}
	// The last prose line wraps whole: 44+44+44+44+4.
	tail := strings.Join(got[13:18], "")
	if tail != prose {
		t.Fatalf("prose %q, want %q", tail, prose)
	}
}

func TestFitTrimsTrailingBlanks(t *testing.T) {
	got := fitScreen("hi"+strings.Repeat(" ", 100), 40, 5)
	if len(got) != 1 || got[0] != "hi" {
		t.Fatalf("got %q", got)
	}
}

func TestFitKeepsStyles(t *testing.T) {
	got := fitScreen("\x1b[31mred\x1b[0m   ", 40, 5)
	if len(got) != 1 || !strings.Contains(got[0], "\x1b[31m") || ansi.Strip(got[0]) != "red" {
		t.Fatalf("got %q", got)
	}
}

func chatModel(t *testing.T) (Model, *fakeConn) {
	t.Helper()
	st := fixtureState()
	st.Sessions = []sample.Session{
		{PID: 1, Pane: "%1", Name: "a"},
		{PID: 2, Pane: "%2", Name: "b"},
	}
	return loadedWith(t, 100, st)
}

func TestScreenOfAnotherPaneIsIgnored(t *testing.T) {
	m, _ := chatModel(t)
	next, _ := m.Update(screenMsg{Pane: m.watching, Text: "mine"})
	m = next.(Model)
	next, _ = m.Update(screenMsg{Pane: "%9", Text: "other"})
	if got := next.(Model).screen; got.Text != "mine" {
		t.Fatalf("screen = %+v", got)
	}
}

func TestSelectionChangeWatches(t *testing.T) {
	m, f := chatModel(t)
	first, _ := m.selected()
	next, cmd := key(m, "down")
	drain(cmd)
	second, _ := next.(Model).selected()
	if first.Pane == second.Pane {
		t.Fatal("down did not move")
	}
	acts := f.allActs()
	if last := acts[len(acts)-1]; last.RecID != proto.WatchPrefix+second.Pane {
		t.Fatalf("last act = %q, want watch:%s", last.RecID, second.Pane)
	}
}

func TestSelectionChangeDropsTheOldScreen(t *testing.T) {
	m, _ := chatModel(t)
	sel, _ := m.selected()
	next, _ := m.Update(screenMsg{Pane: sel.Pane, Text: "old"})
	next, _ = key(next, "down")
	if got := next.(Model).screen; got.Text != "" {
		t.Fatalf("screen of the old pane stayed: %+v", got)
	}
}

func TestLostConnectionForgetsTheWatch(t *testing.T) {
	m, _ := chatModel(t)
	next, _ := m.Update(lostMsg{err: io.EOF, conn: m.conn})
	if next.(Model).watching != "" {
		t.Fatal("still watching after the connection went")
	}
}

func TestChatLinesWhenTheSessionEnded(t *testing.T) {
	m, _ := chatModel(t)
	sel, _ := m.selected()
	next, _ := m.Update(screenMsg{Pane: sel.Pane, Gone: true})
	lines := next.(Model).chatLines(40, 5)
	if len(lines) != 5 || strings.TrimSpace(lines[2]) != "That session ended." {
		t.Fatalf("lines = %q", lines)
	}
}

func TestChatLinesFillTheViewFromTheBottom(t *testing.T) {
	m, _ := chatModel(t)
	sel, _ := m.selected()
	next, _ := m.Update(screenMsg{Pane: sel.Pane, Text: "one\ntwo"})
	lines := next.(Model).chatLines(40, 5)
	if len(lines) != 5 || lines[3] != "one" || lines[4] != "two" || lines[0] != "" {
		t.Fatalf("lines = %q", lines)
	}
	next, _ = m.Update(screenMsg{Pane: sel.Pane, Text: "   \n\n"})
	if lines := next.(Model).chatLines(40, 5); len(lines) != 5 {
		t.Fatalf("a blank screen gave %d lines", len(lines))
	}
}

func TestRefusedWatchRetriesOnTheNextSnapshotAndSaysNothing(t *testing.T) {
	m, f := chatModel(t)
	sel, _ := m.selected()
	next, cmd := m.Update(resultMsg(proto.Result{ActID: "watch:" + sel.Pane, Err: "no agent session in " + sel.Pane + " any more"}))
	drain(cmd)
	mm := next.(Model)
	if mm.flash != "" || mm.watching != "" {
		t.Fatalf("flash %q watching %q", mm.flash, mm.watching)
	}
	next, cmd = mm.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	drain(cmd)
	n := len(f.allActs())
	next, cmd = next.Update(stateMsg(fixtureStateWith(sel)))
	drain(cmd)
	acts := f.allActs()
	if len(acts) != n+1 || acts[n].RecID != proto.WatchPrefix+sel.Pane || next.(Model).flash != "" {
		t.Fatalf("acts %+v", acts[n:])
	}
}

func fixtureStateWith(ss ...sample.Session) proto.State {
	st := fixtureState()
	st.Sessions = ss
	return st
}

func TestChatLinesTakeNoNegativeHeight(t *testing.T) {
	m, _ := chatModel(t)
	sel, _ := m.selected()
	next, _ := m.Update(screenMsg{Pane: sel.Pane, Text: "one"})
	if lines := next.(Model).chatLines(40, -1); len(lines) != 0 {
		t.Fatalf("lines = %q", lines)
	}
}

func TestRefusalOfAnOldWatchHoldsNothing(t *testing.T) {
	m, _ := chatModel(t)
	a, _ := m.selected()
	next, cmd := key(m, "down")
	drain(cmd)
	b, _ := next.(Model).selected()
	next, _ = next.Update(screenMsg{Pane: b.Pane, Text: "kept"})
	next, _ = next.Update(resultMsg(proto.Result{ActID: "watch:" + a.Pane, Err: "no agent session in " + a.Pane + " any more"}))
	mm := next.(Model)
	if mm.watchHeld != "" || mm.watching != b.Pane || mm.screen.Text != "kept" {
		t.Fatalf("held %q watching %q screen %q", mm.watchHeld, mm.watching, mm.screen.Text)
	}
}

// inChat is the console with the first session's screen and the keys in
// its chat.
func inChat(t *testing.T) (Model, *fakeConn) {
	t.Helper()
	m, f := chatModel(t)
	m = onPane(t, m, "%1")
	next, _ := m.Update(screenMsg{Pane: m.watching, At: time.Unix(100, 0), Text: "> "})
	next, _ = key(next, "enter")
	m = next.(Model)
	if !m.chatKeys {
		t.Fatal("enter did not give the chat the keys")
	}
	return m, f
}

// onPane is m with the session in pane selected.
func onPane(t *testing.T, m Model, pane string) Model {
	t.Helper()
	for _, k := range []string{"up", "down", "down"} {
		if s, _ := m.selected(); s.Pane == pane {
			return m
		}
		next, _ := key(m, k)
		m = next.(Model)
	}
	t.Fatalf("no session in %s to select", pane)
	return m
}

// keyActs are the keys the console sent, in order.
func keyActs(f *fakeConn) []proto.Act {
	var out []proto.Act
	for _, a := range f.acts() {
		if strings.HasPrefix(a.RecID, proto.KeysPrefix) {
			out = append(out, a)
		}
	}
	return out
}

// pressKeys is ks on m, with its commands run.
func pressKeys(t *testing.T, m Model, ks ...string) Model {
	t.Helper()
	for _, k := range ks {
		next, cmd := key(m, k)
		drain(cmd)
		m = next.(Model)
	}
	return m
}

// answered is the result of the key in flight, with the next one sent.
func answered(t *testing.T, m Model, err string) Model {
	t.Helper()
	if m.keyInFlight == "" {
		t.Fatal("no key in flight")
	}
	next, cmd := m.Update(resultMsg(proto.Result{ActID: m.keyInFlight, Err: err}))
	drain(cmd)
	return next.(Model)
}

func TestEnterGivesTheChatTheKeys(t *testing.T) {
	for _, k := range []string{"enter", "a", "right"} {
		m, f := chatModel(t)
		s, _ := m.selected()
		m = pressKeys(t, m, k)
		if !m.chatKeys {
			t.Fatalf("%s: the chat has no keys", k)
		}
		line, _ := m.keyLine(100)
		if want := "keys go to " + s.Name + " · ctrl+] back · ctrl+o into tmux"; !strings.Contains(ansi.Strip(line), want) {
			t.Fatalf("%s: key line %q, want %q", k, ansi.Strip(line), want)
		}
		if len(f.acts()) != 0 {
			t.Fatalf("%s: sent %+v", k, f.acts())
		}
	}
}

func TestEnterOnASessionOutsideTmuxSaysWhy(t *testing.T) {
	st := fixtureState()
	st.Sessions = []sample.Session{{PID: 1, Name: "out"}}
	m, _ := loadedWith(t, 100, st)
	m = pressKeys(t, m, "enter")
	if m.chatKeys || !strings.Contains(m.flash, "runs outside tmux") {
		t.Fatalf("chatKeys=%v flash=%q", m.chatKeys, m.flash)
	}
}

func TestCtrlBracketGivesKeysBack(t *testing.T) {
	m, f := inChat(t)
	m = pressKeys(t, m, "ctrl+]")
	if m.chatKeys || len(f.acts()) != 0 {
		t.Fatalf("chatKeys=%v acts=%+v", m.chatKeys, f.acts())
	}
}

func TestChatSendsKeysInOrder(t *testing.T) {
	m, f := inChat(t)
	m = pressKeys(t, m, "down", "down", "enter")
	if got := keyActs(f); len(got) != 1 || got[0].Text != "Down" || got[0].RecID != proto.KeysPrefix+"%1" {
		t.Fatalf("in flight: %+v", got)
	}
	m = answered(t, m, "")
	if got := keyActs(f); len(got) != 2 || got[1].Text != "Down" {
		t.Fatalf("second: %+v", got)
	}
	m = answered(t, m, "")
	got := keyActs(f)
	if len(got) != 3 || got[2].Text != "Enter" || !got[2].Seen.Equal(m.screen.At) || got[2].PID != 1 {
		t.Fatalf("third: %+v", got)
	}
	m = answered(t, m, "")
	if m.keyInFlight != "" || len(m.keyQ) != 0 {
		t.Fatalf("left: %q %+v", m.keyInFlight, m.keyQ)
	}
}

func TestTypedTextCoalesces(t *testing.T) {
	m, f := inChat(t)
	m = pressKeys(t, m, "down", "y", "e", "s")
	answered(t, m, "")
	if got := keyActs(f); len(got) != 2 || got[1].Text != "lit:yes" {
		t.Fatalf("acts: %+v", got)
	}
}

func TestKeyNames(t *testing.T) {
	for k, want := range map[string]string{
		"up": "Up", "down": "Down", "left": "Left", "right": "Right", "enter": "Enter", "tab": "Tab",
		"shift+tab": "BTab", "esc": "Escape", "ctrl+c": "C-c", "backspace": "BSpace", "space": "lit: ", "y": "lit:y", "Y": "lit:Y",
	} {
		if got, ok := keyName(keyPress(k)); !ok || got != want {
			t.Errorf("%s = %q %v, want %q", k, got, ok, want)
		}
	}
	if got, ok := keyName(keyPress("ctrl+x")); ok {
		t.Errorf("ctrl+x = %q, want no key", got)
	}
}

func TestQueuedKeysStayWithTheirPane(t *testing.T) {
	m, f := inChat(t)
	m = pressKeys(t, m, "down", "enter", "ctrl+]")
	m = onPane(t, m, "%2")
	answered(t, m, "")
	if got := keyActs(f); len(got) != 2 || got[1].RecID != proto.KeysPrefix+"%1" || got[1].Text != "Enter" {
		t.Fatalf("acts: %+v", got)
	}

	m, f = inChat(t)
	m = pressKeys(t, m, "down", "enter")
	st := fixtureState()
	st.Sessions = []sample.Session{{PID: 2, Pane: "%2", Name: "b"}}
	next, _ := m.Update(stateMsg(st))
	m = next.(Model)
	if len(m.keyQ) != 0 || m.flash != "not sent: a ended" {
		t.Fatalf("queue %+v flash %q", m.keyQ, m.flash)
	}
	m = answered(t, m, "")
	if got := keyActs(f); len(got) != 1 {
		t.Fatalf("acts: %+v", got)
	}
}

func TestChatLeavesWhenItsSessionEnds(t *testing.T) {
	m, _ := inChat(t)
	st := fixtureState()
	st.Sessions = []sample.Session{{PID: 2, Pane: "%2", Name: "b"}}
	next, _ := m.Update(stateMsg(st))
	if next.(Model).chatKeys {
		t.Fatal("the chat kept the keys of a session that ended")
	}
}

func TestCtrlCTwiceIsHeld(t *testing.T) {
	m, f := inChat(t)
	now := time.Unix(1000, 0)
	m.opt.Now = func() time.Time { return now }
	m = pressKeys(t, m, "ctrl+c")
	if m.quitting {
		t.Fatal("ctrl+c quit the console")
	}
	m = answered(t, m, "")
	now = now.Add(500 * time.Millisecond)
	m = pressKeys(t, m, "ctrl+c")
	if got := keyActs(f); len(got) != 1 || got[0].Text != "C-c" {
		t.Fatalf("acts: %+v", got)
	}
	if m.flash != "^C held: one went 0.5 s ago, ^C again stops the agent" {
		t.Fatalf("flash %q", m.flash)
	}
	now = now.Add(300 * time.Millisecond)
	m = pressKeys(t, m, "ctrl+c")
	if got := keyActs(f); len(got) != 2 || got[1].Text != "C-c" {
		t.Fatalf("acts: %+v", got)
	}
}

func TestCtrlOGoesIntoThePane(t *testing.T) {
	var ran []string
	m, _ := inChat(t)
	m.opt.Run = func(argv []string) error { ran = argv; return nil }
	m = pressKeys(t, m, "ctrl+o")
	if m.pending == nil || m.pending.String() != "tmux switch-client -t %1" {
		t.Fatalf("pending %v", m.pending)
	}
	_, cmd := key(m, "enter")
	drain(cmd)
	if strings.Join(ran, " ") != "tmux switch-client -t %1" {
		t.Fatalf("ran %q", ran)
	}
}

func TestKeysWithNoConnection(t *testing.T) {
	m, f := inChat(t)
	m.lost = true
	m = pressKeys(t, m, "down")
	if len(m.keyQ) != 0 || m.keyInFlight != "" || len(keyActs(f)) != 0 || m.flash != "not sent, no connection" {
		t.Fatalf("queue %+v flight %q flash %q", m.keyQ, m.keyInFlight, m.flash)
	}
}

func TestRefusalShowsAndClearsQueue(t *testing.T) {
	m, f := inChat(t)
	m = pressKeys(t, m, "down", "down", "enter")
	const why = "not sent: a asks something new — look again"
	m = answered(t, m, why)
	if m.flash != why || len(m.keyQ) != 0 || m.keyInFlight != "" || len(keyActs(f)) != 1 {
		t.Fatalf("flash %q queue %+v flight %q", m.flash, m.keyQ, m.keyInFlight)
	}
}

func TestEscGoesToTheAgent(t *testing.T) {
	m, f := inChat(t)
	m = pressKeys(t, m, "esc")
	if got := keyActs(f); len(got) != 1 || got[0].Text != "Escape" || !m.chatKeys {
		t.Fatalf("acts %+v chatKeys %v", got, m.chatKeys)
	}
}

func TestHomeKeyGivesKeysBack(t *testing.T) {
	if homeKey != panes.HomeKey {
		t.Fatalf("homeKey %q, the app sends %q", homeKey, panes.HomeKey)
	}
	m, _ := inChat(t)
	next, _ := m.Update(uv.UnknownApcEvent(panes.HomeKey))
	if next.(Model).chatKeys {
		t.Fatal("the home key left the keys in the chat")
	}
}

func TestClicksMoveTheKeys(t *testing.T) {
	m, _ := chatModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m = next.(Model)
	sc := m.draw(200)
	if sc.listW >= 200 {
		t.Fatal("no right side at 200 columns")
	}
	right := tea.Mouse{X: sc.listW + 5, Y: sc.bodyTop + 1, Button: tea.MouseLeft}
	next, _ = m.Update(tea.MouseClickMsg(right))
	m = next.(Model)
	if !m.chatKeys {
		t.Fatal("a click on the right did not give the chat the keys")
	}
	left := tea.Mouse{X: 2, Y: sc.bodyTop + 1, Button: tea.MouseLeft}
	next, _ = m.Update(tea.MouseClickMsg(left))
	if next.(Model).chatKeys {
		t.Fatal("a click on the left left the keys in the chat")
	}
}

func TestTheChatsFirstKeyAsksToTakeOverAndCtrlCStays(t *testing.T) {
	m, f := loadedWith(t, 120, nightState())
	m = pressKeys(t, m, "enter")
	if !m.chatKeys || m.held != nil {
		t.Fatalf("opening the chat: chatKeys %v, held %v", m.chatKeys, m.held != nil)
	}
	next, cmd := key(m, "ctrl+c")
	if quits(cmd) || next.(Model).quitting || next.(Model).held == nil {
		t.Fatal("ctrl+c in the chat at night quit, or did not ask to take over")
	}
	pressKeys(t, next.(Model), "n")
	if got := keyActs(f); len(got) != 1 || got[0].Text != "C-c" {
		t.Fatalf("the held ^C did not go: %+v", f.acts())
	}
}

// nightChat is the console at night with the keys in %1's chat.
func nightChat(t *testing.T) (Model, *fakeConn) {
	t.Helper()
	st := nightState()
	st.Sessions = []sample.Session{{PID: 1, Pane: "%1", Name: "a"}, {PID: 2, Pane: "%2", Name: "b"}}
	m, f := loadedWith(t, 100, st)
	m = pressKeys(t, onPane(t, m, "%1"), "enter")
	if !m.chatKeys {
		t.Fatal("enter did not give the chat the keys")
	}
	return m, f
}

func TestAHeldKeyGoesToTheAgentItWasPressedFor(t *testing.T) {
	m, f := nightChat(t)
	m = pressKeys(t, m, "down")
	if m.held == nil {
		t.Fatal("the first key to the agent did not ask to take over")
	}
	m.selPID = 2 // the selection moves to b while the question waits
	m = pressKeys(t, m, "n")
	if got := keyActs(f); len(got) != 1 || got[0].RecID != proto.KeysPrefix+"%1" || got[0].PID != 1 || got[0].Text != "Down" {
		t.Fatalf("acts %+v", got)
	}

	m, f = nightChat(t)
	m = pressKeys(t, m, "down")
	st := nightState()
	st.Sessions = []sample.Session{{PID: 2, Pane: "%2", Name: "b"}}
	next, _ := m.Update(stateMsg(st))
	m = pressKeys(t, next.(Model), "n")
	if got := keyActs(f); len(got) != 0 || m.flash != "not sent: a ended" {
		t.Fatalf("acts %+v flash %q", got, m.flash)
	}
}

func TestASearchKeepsQueuedKeys(t *testing.T) {
	m, f := inChat(t)
	m = pressKeys(t, m, "down", "enter", "ctrl+]", "/", "b")
	if !m.searching || slices.ContainsFunc(m.snap.Sessions, func(s sample.Session) bool { return s.Pane == "%1" }) {
		t.Fatalf("the search did not hide a: searching %v", m.searching)
	}
	if len(m.keyQ) != 1 || m.flash != "" {
		t.Fatalf("queue %+v flash %q", m.keyQ, m.flash)
	}
	answered(t, m, "")
	if got := keyActs(f); len(got) != 2 || got[1].RecID != proto.KeysPrefix+"%1" || got[1].Text != "Enter" {
		t.Fatalf("acts %+v", got)
	}
}

func TestCtrlCHoldIsPerAgentAndFromWhenItWent(t *testing.T) {
	m, f := inChat(t)
	now := time.Unix(1000, 0)
	m.opt.Now = func() time.Time { return now }
	m = pressKeys(t, m, "ctrl+c", "ctrl+]")
	m = pressKeys(t, onPane(t, m, "%2"), "enter")
	now = now.Add(300 * time.Millisecond)
	m = pressKeys(t, m, "ctrl+c")
	if m.flash != "" {
		t.Fatalf("a ^C to b was held: %q", m.flash)
	}
	m = answered(t, m, "")
	if got := keyActs(f); len(got) != 2 || got[0].RecID != proto.KeysPrefix+"%1" || got[1].RecID != proto.KeysPrefix+"%2" {
		t.Fatalf("acts %+v", got)
	}

	// A ^C still in the queue went no time ago: the next one waits.
	m = answered(t, m, "")
	now = now.Add(2 * time.Second)
	m = pressKeys(t, m, "down", "ctrl+c")
	if m.flash != "" || len(m.keyQ) != 1 {
		t.Fatalf("a ^C 2 s after the last was held: %q", m.flash)
	}
	now = now.Add(2 * time.Second)
	m = pressKeys(t, m, "ctrl+c")
	if m.flash != "^C held: one waits to go, ^C again stops the agent" {
		t.Fatalf("flash %q", m.flash)
	}
}

func TestAPasteGoesAsTextAndEnter(t *testing.T) {
	m, f := inChat(t)
	long := strings.Repeat("x", 250)
	next, cmd := m.Update(tea.PasteMsg{Content: "yes\r\nno\x07" + long})
	drain(cmd)
	m = next.(Model)
	for range 3 {
		m = answered(t, m, "")
	}
	var got []string
	for _, a := range keyActs(f) {
		if a.RecID != proto.KeysPrefix+"%1" {
			t.Fatalf("act %+v", a)
		}
		got = append(got, a.Text)
	}
	want := []string{"lit:yes", "Enter", "lit:no" + long[:198], "lit:" + long[:52]}
	if !slices.Equal(got, want) {
		t.Fatalf("texts %q, want %q", got, want)
	}
}

func TestPasteNewlines(t *testing.T) {
	for in, want := range map[string][]string{
		"a\rb":   {"lit:a", "Enter", "lit:b"},
		"a\r\nb": {"lit:a", "Enter", "lit:b"},
		"a\nb":   {"lit:a", "Enter", "lit:b"},
		"a\n\nb": {"lit:a", "Enter", "Enter", "lit:b"},
		"a\tb":   {"lit:ab"},
	} {
		if got := pasteTexts(in); !slices.Equal(got, want) {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}

func TestAPasteTouchesTheStokersRest(t *testing.T) {
	m, f := inChat(t)
	var v proto.StokerView
	v.On, v.Rests = true, m.now()
	m.stoker = &v
	if !m.resting() || m.night() {
		t.Fatal("the stoker does not rest")
	}
	next, cmd := m.Update(tea.PasteMsg{Content: "hi"})
	drain(cmd)
	if next.(Model).touched.IsZero() || !slices.ContainsFunc(f.acts(), func(a proto.Act) bool { return a.RecID == "stoker:touch" }) {
		t.Fatalf("acts %+v", f.acts())
	}
}

// demoSized is the demo machine at w by h, the selected session's screen
// a question with three options.
func demoSized(t *testing.T, w, h int) Model {
	t.Helper()
	m := New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	next, _ = next.Update(stateMsg(screensState(t)))
	nm := next.(Model)
	nm.splashDone = true
	next, _ = nm.Update(screenMsg{Pane: nm.watching, At: demoAt, Text: askScreen})
	return next.(Model)
}

// askLast is the last line of askScreen.
const askLast = "Enter to select · ↑/↓ to navigate · Esc to cancel"

// sides are the lines of a frame cut at the list's edge: the list, and
// what is right of its hairline.
func sides(sc drawn, w int) (left, right []string) {
	for _, l := range sc.lines {
		plain := ansi.Strip(l)
		left = append(left, ansi.Cut(plain, 0, sc.listW))
		right = append(right, ansi.Cut(plain, sc.listW+1, w))
	}
	return left, right
}

func TestWideStacksStatsLeft(t *testing.T) {
	m := demoSized(t, 200, 50)
	sc := m.draw(200)
	if sc.listW > 200*2/5 {
		t.Fatalf("the left column is %d of 200 columns", sc.listW)
	}
	if sc.dockAt >= 0 {
		t.Fatal("the dock is drawn on a desk")
	}
	left, right := sides(sc, 200)
	at := map[string]int{}
	for y, l := range left {
		for _, title := range []string{"SESSIONS", "USAGE", "MACHINE"} {
			if _, seen := at[title]; !seen && strings.Contains(l, title) {
				at[title] = y
			}
		}
	}
	if len(at) != 3 || at["SESSIONS"] >= at["USAGE"] || at["USAGE"] >= at["MACHINE"] {
		t.Fatalf("titles at %v:\n%s", at, strings.Join(left, "\n"))
	}
	if !slices.ContainsFunc(right, func(l string) bool { return strings.Contains(l, askLast) }) {
		t.Fatalf("the right side has no screen:\n%s", strings.Join(right, "\n"))
	}
}

func TestMediumSplitsListAndChat(t *testing.T) {
	m := demoSized(t, 86, 30)
	sc := m.draw(86)
	if sc.listW != (86-1)/2 {
		t.Fatalf("the list is %d of 86 columns", sc.listW)
	}
	if sc.dockAt < 0 {
		t.Fatal("no dock on an opened phone")
	}
	_, right := sides(sc, 86)
	if !slices.ContainsFunc(right, func(l string) bool { return strings.Contains(l, "Esc to") }) {
		t.Fatalf("the right half has no screen:\n%s", strings.Join(right, "\n"))
	}
}

func TestNarrowSwapsToChat(t *testing.T) {
	list := demoSized(t, 44, 31)
	m := pressKeys(t, list, "enter")
	sc := m.draw(44)
	if got := strings.TrimSpace(ansi.Strip(sc.lines[0])); got != "‹ Sessions · spark-docs · done" {
		t.Fatalf("title row %q", got)
	}
	out := ansi.Strip(strings.Join(sc.lines, "\n"))
	if len(sc.lines) != 31 || strings.Contains(out, "WAITS FOR YOU") || !strings.Contains(out, "Esc to") {
		t.Fatalf("not the chat alone, %d lines:\n%s", len(sc.lines), out)
	}
	next, _ := m.Update(tea.MouseClickMsg(tea.Mouse{X: 3, Y: 0, Button: tea.MouseLeft}))
	if m = next.(Model); m.chatKeys || screen(m) != screen(list) {
		t.Fatalf("a tap on ‹ Sessions: chatKeys %v, not the list as before:\n%s", m.chatKeys, screen(m))
	}
	tall := pressKeys(t, demoSized(t, 44, 50), "enter")
	next, _ = tall.Update(tea.MouseClickMsg(tea.Mouse{X: 3, Y: 0, Button: tea.MouseLeft}))
	if sc := next.(Model).draw(44); sc.dockAt < 0 {
		t.Fatalf("no dock under the list:\n%s", screen(next))
	}
}

func TestChatFrameShowsWhoHasKeys(t *testing.T) {
	titleInk := func(m Model) string {
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
	m := pressKeys(t, demoSized(t, 132, 32), "enter")
	if ink := titleInk(m); ink != "accent" {
		t.Fatalf("with the keys, the chat's title is %q", ink)
	}
	if ink := titleInk(pressKeys(t, m, "ctrl+]")); ink == "accent" {
		t.Fatal("without the keys, the chat's title is in the accent")
	}
	edgeInk := func(m Model) string {
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
	if ink := edgeInk(m); ink != "accent" {
		t.Fatalf("with the keys, the chat's edge is %q", ink)
	}
	if ink := edgeInk(pressKeys(t, m, "ctrl+]")); ink != "hair" {
		t.Fatalf("without the keys, the chat's edge is %q", ink)
	}
}

// A step that asks to confirm shows over a phone's chat, so the next key
// never answers what the builder cannot see.
func TestAPhoneChatShowsWhatAsksToConfirm(t *testing.T) {
	m := pressKeys(t, demoSized(t, 44, 31), "enter", "ctrl+o")
	if out := screen(m); m.pending == nil || !strings.Contains(out, "tmux switch-client") || !strings.Contains(out, "⏎ run") {
		t.Fatalf("no jump box over the chat:\n%s", out)
	}
	st := nightState()
	st.Sessions = []sample.Session{{PID: 1, Pane: "%1", Name: "a"}, {PID: 2, Pane: "%2", Name: "b"}}
	night, _ := loadedWith(t, 44, st)
	night = pressKeys(t, night, "enter", "x")
	if out := screen(night); night.held == nil || !strings.Contains(out, "The stoker is on. Take over?") {
		t.Fatalf("no take-over box over the chat:\n%s", out)
	}
}

func TestAPhoneChatFitsAShortScreen(t *testing.T) {
	m := pressKeys(t, demoSized(t, 44, 3), "enter")
	if n := len(strings.Split(m.render(), "\n")); !m.chatKeys || n > 3 {
		t.Fatalf("chatKeys %v, %d lines on a 3-line screen", m.chatKeys, n)
	}
}

func TestATapOnAPhoneRowOpensItsChat(t *testing.T) {
	m := demoSized(t, 44, 31)
	next, _ := tap(m, 5, sessLine(t, m, "flint-auth"))
	m = next.(Model)
	if s, _ := m.selectedSession(); !m.chatKeys || s.Name != "flint-auth" {
		t.Fatalf("chatKeys %v on %q", m.chatKeys, s.Name)
	}
	if got := strings.TrimSpace(ansi.Strip(m.draw(44).lines[0])); got != "‹ Sessions · flint-auth · asks" {
		t.Fatalf("title row %q", got)
	}
}

func TestACutListSaysHowManyAreBelow(t *testing.T) {
	m := demoSized(t, 132, 32)
	sc := m.draw(132)
	left, _ := sides(sc, 132)
	if !slices.ContainsFunc(left, func(l string) bool {
		return strings.HasPrefix(strings.TrimSpace(l), "↓ ") && strings.Contains(l, " more")
	}) {
		t.Fatalf("the cut list does not say what is below:\n%s", strings.Join(left, "\n"))
	}
	if !slices.ContainsFunc(left, func(l string) bool { return strings.Contains(l, "RECOMMENDATIONS") }) {
		t.Fatalf("no recommendations on a desk:\n%s", strings.Join(left, "\n"))
	}
}

func TestAGoneScreenSaysTheSessionEnded(t *testing.T) {
	m, _ := inChat(t)
	next, _ := m.Update(screenMsg{Pane: "%1", Gone: true})
	if m = next.(Model); m.chatKeys || m.flash != "That session ended." {
		t.Fatalf("chatKeys %v, flash %q", m.chatKeys, m.flash)
	}
}

// The hint under a cut list never covers the selection: every row the
// selection moves through is drawn, selected, and a tap on it hits it.
func TestTheMoreHintNeverCoversTheSelection(t *testing.T) {
	m := demoSized(t, 132, 32)
	for range m.rowCount() {
		sc := m.draw(132)
		lines := strings.Split(ansi.Strip(strings.Join(sc.lines, "\n")), "\n")
		at := slices.Index(sc.rows, m.rowIndex())
		if at < 0 || !strings.HasPrefix(lines[sc.bodyTop+at], "▌") {
			t.Fatalf("row %d is not drawn as selected:\n%s", m.rowIndex(), strings.Join(lines, "\n"))
		}
		for i, l := range lines {
			if strings.Contains(l, "↓ ") && strings.Contains(l, " more") && i-sc.bodyTop >= 0 && i-sc.bodyTop < len(sc.rows) && sc.rows[i-sc.bodyTop] >= 0 {
				t.Fatalf("the hint is on row %d:\n%s", sc.rows[i-sc.bodyTop], strings.Join(lines, "\n"))
			}
		}
		next, _ := tap(m, 3, sc.bodyTop+at)
		if next.(Model).rowIndex() != m.rowIndex() {
			t.Fatalf("a tap on the selected row %d selected %d", m.rowIndex(), next.(Model).rowIndex())
		}
		m = pressKeys(t, m, "down")
	}
}
