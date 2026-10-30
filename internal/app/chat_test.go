package app

import (
	"io"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/logos"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/said"
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
	m, f := inChat(t)
	for _, e := range decoded(t, panes.HomeKey) {
		next, _ := m.Update(e)
		m = next.(Model)
	}
	if m.chatKeys {
		t.Fatal("the home key left the keys in the chat")
	}
	if got := keyActs(f); len(got) != 0 {
		t.Fatalf("the home key went to the agent as %+v", got)
	}
}

// decoded is raw, the bytes a terminal sends, read in one go by the input
// decoder the console runs on, as Bubble Tea hands them to the model.
func decoded(t *testing.T, raw string) []tea.Msg {
	t.Helper()
	events := make(chan uv.Event)
	go func() {
		defer close(events)
		_ = uv.NewTerminalReader(strings.NewReader(raw), "xterm-256color").StreamEvents(t.Context(), events)
	}()
	var msgs []tea.Msg
	for e := range events {
		if k, ok := e.(uv.KeyPressEvent); ok {
			e = tea.KeyPressMsg(k)
		}
		msgs = append(msgs, e)
	}
	return msgs
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
	m := pressKeys(t, demoSized(t, 132, 32), "enter")
	if ink := chatTitleInk(t, m); ink != "accent" {
		t.Fatalf("with the keys, the chat's title is %q", ink)
	}
	if ink := chatTitleInk(t, pressKeys(t, m, "ctrl+]")); ink == "accent" {
		t.Fatal("without the keys, the chat's title is in the accent")
	}
	if ink := chatEdgeInk(t, m); ink != "accent" {
		t.Fatalf("with the keys, the chat's edge is %q", ink)
	}
	if ink := chatEdgeInk(t, pressKeys(t, m, "ctrl+]")); ink != "hair" {
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
	last := pressKeys(t, m, slices.Repeat([]string{"down"}, m.rowCount())...)
	left, _ = sides(last.draw(132), 132)
	if !slices.ContainsFunc(left, func(l string) bool { return strings.Contains(l, "RECOMMENDATIONS") }) {
		t.Fatalf("the list does not scroll to its recommendations on a desk:\n%s", strings.Join(left, "\n"))
	}
}

// flintSized is the demo machine at w by h with flint-auth, edited by
// edit, selected and its screen asking.
func flintSized(t *testing.T, w, h int, edit func(*sample.Session)) Model {
	t.Helper()
	st := screensState(t)
	for i := range st.Sessions {
		if st.Sessions[i].Name == "flint-auth" {
			edit(&st.Sessions[i])
		}
	}
	m := New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	next, _ = next.Update(stateMsg(st))
	nm := next.(Model)
	nm.splashDone = true
	for _, s := range nm.snap.Sessions {
		if s.Name == "flint-auth" {
			nm.chose, nm.selPID = true, s.PID
		}
	}
	nm.watchCmd()
	next, _ = nm.Update(screenMsg{Pane: nm.watching, At: demoAt, Text: askScreen})
	return next.(Model)
}

// everyFact gives a session an account, background work and, with its
// ship, all three facts of the chat's strip.
func everyFact(s *sample.Session) {
	s.Provider, s.Account, s.Agents = "anthropic", "work", 1
	s.Shells = []sample.Shell{{PID: 2240632, Started: demoAt.Add(-37 * time.Hour), Command: "sleep 15"}}
}

// The strip between the chat's title and its screen says the session's
// account, its PR and its background work, on one line where it fits.
// The screen gives the strip its rows.
func TestTheChatStripSaysTheSessionsFacts(t *testing.T) {
	m := flintSized(t, 200, 50, everyFact)
	sc := m.draw(200)
	_, right := sides(sc, 200)
	strip := right[sc.bodyTop+1]
	for _, want := range []string{"✻ work", "#4545", "conflicts with main", "1 subagent running", "1 shell · oldest 1d13h"} {
		if !strings.Contains(strip, want) {
			t.Fatalf("no %q in the strip %q:\n%s", want, strip, strings.Join(right, "\n"))
		}
	}
	if strings.Contains(right[sc.bodyTop+2], "work") {
		t.Fatalf("the strip takes two lines where one fits:\n%s", strings.Join(right, "\n"))
	}
	if len(sc.lines) != 50 || !slices.ContainsFunc(right, func(l string) bool { return strings.Contains(l, "╰─ ⏎") }) {
		t.Fatalf("%d lines, or the card is gone:\n%s", len(sc.lines), strings.Join(right, "\n"))
	}
}

// A session with no account, no PR and no background work has no strip:
// the screen starts under the title.
func TestTheChatStripTakesNoRowsWithoutFacts(t *testing.T) {
	m := flintSized(t, 200, 50, func(s *sample.Session) {
		s.Provider, s.Account, s.Agents, s.Shells = "", "", 0, nil
	})
	m.shipReport = nil
	s, _ := m.selectedSession()
	got := m.chatPane(s, 121, 40)
	want := m.screenLines(120, 39)
	if len(got) != 40 {
		t.Fatalf("%d lines in a pane of 40", len(got))
	}
	for i, l := range want {
		if got[i+1] != " "+l {
			t.Fatalf("line %d under the title is %q, not the screen's %q", i+1, ansi.Strip(got[i+1]), ansi.Strip(l))
		}
	}
}

// A strip too long for one line takes two, never more.
func TestTheChatStripWrapsToTwoLinesAtMost(t *testing.T) {
	m := flintSized(t, 104, 38, func(s *sample.Session) {
		everyFact(s)
		s.Account = "a-long-account-name@example.com · Max"
	})
	sc := m.draw(104)
	_, right := sides(sc, 104)
	first, second, third := right[sc.bodyTop+1], right[sc.bodyTop+2], right[sc.bodyTop+3]
	if !strings.Contains(first, "a-long-account") || !strings.Contains(second, "#4545") && !strings.Contains(second, "subagent") {
		t.Fatalf("the strip is not on two lines:\n%s", strings.Join(right, "\n"))
	}
	if strings.Contains(third, "subagent") || strings.Contains(third, "shell") {
		t.Fatalf("the strip takes a third line:\n%s", strings.Join(right, "\n"))
	}
	for _, l := range right[sc.bodyTop+1 : sc.bodyTop+3] {
		if lipgloss.Width(l) > 104-sc.listW-1 {
			t.Fatalf("a strip line is wider than the chat: %q", l)
		}
	}
}

// On a short desk the stats keep their rows and the list gives way, down
// to 3 rows and the hint under them; shorter, the machine folds, then
// usage.
func TestAShortDeskKeepsItsStats(t *testing.T) {
	m := demoSized(t, 132, 32)
	left, _ := sides(m.draw(132), 132)
	all := strings.Join(left, "\n")
	if !strings.Contains(all, "USAGE") || !strings.Contains(all, "MACHINE") || !strings.Contains(all, "↓ ") {
		t.Fatalf("132x32: the stats do not keep their rows over a cut list:\n%s", all)
	}
	folded := false
	for h := 10; h <= 40; h++ {
		m := demoSized(t, 132, h)
		sc := m.draw(132)
		left, _ := sides(sc, 132)
		hair := slices.IndexFunc(left[sc.bodyTop:], func(l string) bool { return strings.HasPrefix(l, "───") })
		if hair < 0 || hair >= len(sc.rows) {
			folded = true
			continue
		}
		if hair < 4 {
			t.Fatalf("132x%d: the list keeps %d lines over the stats:\n%s", h, hair, strings.Join(left, "\n"))
		}
	}
	if !folded {
		t.Fatal("no desk from 10 to 40 rows is short enough to fold the stats")
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
	for _, h := range []int{32, 24} { // 24: the list keeps its fewest rows
		theMoreHintNeverCoversTheSelection(t, demoSized(t, 132, h))
	}
}

func theMoreHintNeverCoversTheSelection(t *testing.T, m Model) {
	t.Helper()
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

// cardState has session a (%1) on the question tool's call id.
func cardState(id string, qs ...said.Question) proto.State {
	st := fixtureState()
	st.Sessions = []sample.Session{
		{PID: 1, Pane: "%1", Name: "a", Ask: &said.Ask{ID: id, Questions: qs}},
		{PID: 2, Pane: "%2", Name: "b"},
	}
	return st
}

// askModel is the console at w columns with session a (%1) on Claude Code's
// question tool, its screen in, and the keys in its chat.
func askModel(t *testing.T, w int, qs ...said.Question) (Model, *fakeConn) {
	t.Helper()
	m, f := loadedWith(t, w, cardState("toolu_1", qs...))
	m = onPane(t, m, "%1")
	next, _ := m.Update(screenMsg{Pane: "%1", At: time.Unix(100, 0), Text: "⏺ Which way?"})
	m = pressKeys(t, next.(Model), "enter")
	if !m.chatKeys {
		t.Fatal("enter did not give the chat the keys")
	}
	return pastQuiet(m), f
}

// pastQuiet is m with its clock stopped after the card's quiet.
func pastQuiet(m Model) Model {
	at := m.now().Add(time.Second)
	m.opt.Now = func() time.Time { return at }
	return m
}

var threeWays = said.Question{Question: "Which way do we fix the login?", Options: []said.Option{
	{Label: "Patch the token check"}, {Label: "Move to the new session store"}, {Label: "Leave it"}}}

// pickActs are the answers the console sent through the pick: act.
func pickActs(f *fakeConn) []proto.Act {
	var out []proto.Act
	for _, a := range f.acts() {
		if strings.HasPrefix(a.RecID, "pick:") {
			out = append(out, a)
		}
	}
	return out
}

func TestCardShowsTheQuestion(t *testing.T) {
	m, _ := askModel(t, 100, threeWays)
	for _, w := range []int{44, 80} {
		lines := m.chatLines(w, 20)
		if len(lines) != 20 {
			t.Fatalf("%d: %d lines", w, len(lines))
		}
		out := ansi.Strip(strings.Join(lines, "\n"))
		for _, want := range []string{"a asks", "Which way do we fix", "1 Patch the token check", "2 Move to the new", "3 Leave it", "4 Type an answer…", "⏎ send"} {
			if !strings.Contains(out, want) {
				t.Fatalf("%d: the card lacks %q:\n%s", w, want, out)
			}
		}
		if last := ansi.Strip(lines[len(lines)-1]); !strings.HasPrefix(last, "╰") {
			t.Fatalf("%d: the card is not at the foot of the chat: %q", w, last)
		}
		for i, l := range lines {
			if lw := ansi.StringWidth(l); lw > w {
				t.Fatalf("%d: line %d is %d wide: %q", w, i, lw, ansi.Strip(l))
			}
		}
	}
}

func TestCardDownDownEnterPicksTheThird(t *testing.T) {
	m, f := askModel(t, 100, threeWays)
	pressKeys(t, m, "down", "down", "enter")
	got := pickActs(f)
	if len(got) != 1 || got[0].RecID != "pick:%1" || got[0].Text != `{"id":"toolu_1","choices":[{"picks":[3]}]}` {
		t.Fatalf("picks %+v", got)
	}
	if keys := keyActs(f); len(keys) != 0 {
		t.Fatalf("the card sent keys to the agent: %+v", keys)
	}
}

func TestCardSpaceTicks(t *testing.T) {
	multi := threeWays
	multi.Multi = true
	m, f := askModel(t, 100, multi)
	m = pressKeys(t, m, "space")
	if c := m.askCard; !c.ticks[1] || c.ticks[2] {
		t.Fatalf("ticks %v", c.ticks)
	}
	if out := ansi.Strip(strings.Join(m.chatLines(60, 20), "\n")); !strings.Contains(out, "[✔] Patch the token check") || !strings.Contains(out, "␣ tick") {
		t.Fatalf("the card does not show the tick:\n%s", out)
	}
	pressKeys(t, m, "down", "down", "space", "enter")
	if got := pickActs(f); len(got) != 1 || got[0].Text != `{"id":"toolu_1","choices":[{"picks":[1,3]}]}` {
		t.Fatalf("picks %+v", got)
	}
}

func TestCardTapPicks(t *testing.T) {
	for _, size := range [][2]int{{200, 50}, {44, 31}} {
		m, f := askModel(t, size[0], threeWays)
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = next.(Model)
		sc := m.draw(size[0])
		y := -1
		for i, l := range sc.lines {
			if strings.Contains(ansi.Strip(l), "2 Move to the new") {
				y = i
			}
		}
		if y < 0 {
			t.Fatalf("%v: no option 2 on screen:\n%s", size, ansi.Strip(strings.Join(sc.lines, "\n")))
		}
		next, cmd := tap(m, sc.cardX+4, y)
		drain(cmd)
		if got := pickActs(f); len(got) != 1 || got[0].Text != `{"id":"toolu_1","choices":[{"picks":[2]}]}` {
			t.Fatalf("%v: picks %+v", size, got)
		}
		if !next.(Model).chatKeys {
			t.Fatalf("%v: the chat lost the keys", size)
		}
	}
}

func TestNoCardAtAPermissionPrompt(t *testing.T) {
	m, f := loadedWith(t, 100, queueState()) // app-feature (%1) asks for permission
	m = pressKeys(t, onPane(t, m, "%1"), "enter")
	if lines := m.cardLines(60); len(lines) != 0 {
		t.Fatalf("a card at a permission prompt: %q", lines)
	}
	pressKeys(t, m, "enter")
	if got := keyActs(f); len(got) != 1 || got[0].Text != "Enter" || len(pickActs(f)) != 0 {
		t.Fatalf("acts %+v", f.acts())
	}
}

// On a phone the chat is not on screen until Enter opens it: an Enter or a
// digit at once goes nowhere, as the builder has not read the prompt. After
// the moment they go. On a desk the chat was already beside the list.
func TestAPhoneChatIsQuietAtAPermissionPrompt(t *testing.T) {
	m, f := loadedWith(t, 44, queueState()) // app-feature (%1) asks for permission
	m, now := clocked(m)
	m = pressKeys(t, onPane(t, m, "%1"), "enter", "enter", "1")
	if !m.chatKeys || len(keyActs(f)) != 0 || m.flash != "not sent: app-feature asks something new — look again" {
		t.Fatalf("enter enter 1: chatKeys %v acts %+v flash %q", m.chatKeys, keyActs(f), m.flash)
	}
	*now = now.Add(cardQuiet)
	pressKeys(t, m, "enter")
	if got := keyActs(f); len(got) != 1 || got[0].Text != "Enter" {
		t.Fatalf("after the quiet: acts %+v", f.acts())
	}

	m, f = loadedWith(t, 100, queueState())
	m, _ = clocked(m)
	pressKeys(t, onPane(t, m, "%1"), "enter", "enter")
	if got := keyActs(f); len(got) != 1 || got[0].Text != "Enter" {
		t.Fatalf("a desk's chat was on screen: acts %+v", f.acts())
	}
}

// A card's answer and the keys around it go one at a time, in the order
// they were pressed: no key passes an answer in flight, and no answer a key.
func TestTheCardsAnswerKeepsItsPlaceAmongTheKeys(t *testing.T) {
	m, f := askModel(t, 100, threeWays)
	m = pressKeys(t, m, "1", "esc")
	if len(pickActs(f)) != 1 || len(keyActs(f)) != 0 {
		t.Fatalf("esc behind the answer: picks %+v keys %+v", pickActs(f), keyActs(f))
	}
	m = answered(t, m, "")
	if got := keyActs(f); len(got) != 1 || got[0].Text != "Escape" {
		t.Fatalf("esc after the answer: %+v", f.acts())
	}
	if m.toast != "sent to a" {
		t.Fatalf("toast %q", m.toast)
	}

	m, f = askModel(t, 100, threeWays)
	m = pressKeys(t, m, "tab", "2")
	if len(keyActs(f)) != 1 || len(pickActs(f)) != 0 {
		t.Fatalf("the answer behind tab: keys %+v picks %+v", keyActs(f), pickActs(f))
	}
	m = answered(t, m, "")
	if got := pickActs(f); len(got) != 1 || got[0].Text != `{"id":"toolu_1","choices":[{"picks":[2]}]}` {
		t.Fatalf("the answer after tab: %+v", f.acts())
	}
	m = pressKeys(t, m, "esc")
	answered(t, m, "the session in %1 no longer waits on that question")
	if len(keyActs(f)) != 1 {
		t.Fatalf("esc went after a refused answer: %+v", keyActs(f))
	}
}

// Esc, Tab, Shift+Tab and ^C go to the agent over a card: Claude Code
// needs them to leave its question.
func TestTheCardLeavesTheAgentItsOwnKeys(t *testing.T) {
	m, f := askModel(t, 100, threeWays)
	pressKeys(t, m, "esc", "tab")
	if got := keyActs(f); len(got) != 1 || got[0].Text != "Escape" {
		t.Fatalf("acts %+v", f.acts())
	}
}

// Letters type on the card's typing row, q and digits after the first
// letter too, and ⏎ sends the text as the answer.
func TestCardTypesTheAnswer(t *testing.T) {
	m, f := askModel(t, 100, threeWays)
	next := typeText(m, "no, wait 2")
	if out := ansi.Strip(strings.Join(next.(Model).chatLines(60, 20), "\n")); !strings.Contains(out, "› 4 no, wait 2▏") {
		t.Fatalf("the typing row:\n%s", out)
	}
	pressKeys(t, next.(Model), "backspace", "enter")
	if got := pickActs(f); len(got) != 1 || got[0].Text != `{"id":"toolu_1","choices":[{"text":"no, wait "}]}` {
		t.Fatalf("picks %+v", got)
	}
	if len(keyActs(f)) != 0 {
		t.Fatalf("typing went to the agent: %+v", keyActs(f))
	}
}

// Several questions go one at a time, and their answers go as one pick.
func TestCardAnswersEveryQuestion(t *testing.T) {
	checks := said.Question{Question: "Which checks?", Multi: true, Options: []said.Option{{Label: "unit"}, {Label: "e2e"}, {Label: "lint"}}}
	m, f := askModel(t, 100, threeWays, checks)
	m, now := clocked(m)
	if out := ansi.Strip(strings.Join(m.chatLines(60, 20), "\n")); !strings.Contains(out, "a asks · 1 of 2") || !strings.Contains(out, "⏎ question 2") {
		t.Fatalf("the first question:\n%s", out)
	}
	m = pressKeys(t, m, "2")
	*now = now.Add(cardQuiet) // the second question's quiet
	m = pressKeys(t, m, "space", "3")
	out := ansi.Strip(strings.Join(m.chatLines(60, 20), "\n"))
	for _, want := range []string{"2 of 2", "Which checks?", "[✔] unit", "[ ] e2e", "[✔] lint"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the second question lacks %q:\n%s", want, out)
		}
	}
	pressKeys(t, m, "enter")
	if got := pickActs(f); len(got) != 1 || got[0].Confirm != "y" || got[0].Text != `{"id":"toolu_1","choices":[{"picks":[2]},{"picks":[1,3]}]}` {
		t.Fatalf("picks %+v", got)
	}
}

// The key line says where the answer went, or that it did not go and why.
func TestCardSaysWhatWasSent(t *testing.T) {
	m, _ := askModel(t, 100, threeWays)
	next, _ := key(m, "enter")
	if nm := next.(Model); nm.toast != "sending to a…" {
		t.Fatalf("toast %q", nm.toast)
	}
	id := "a" + strconv.Itoa(next.(Model).acts)
	sent, _ := next.Update(resultMsg(proto.Result{ActID: id, Ran: [][]string{{"tmux"}}}))
	if nm := sent.(Model); nm.toast != "sent to a" || nm.flash != "" {
		t.Fatalf("toast %q flash %q", nm.toast, nm.flash)
	}
	stopped, _ := next.Update(resultMsg(proto.Result{ActID: id, Skipped: []string{"the session in %1 no longer waits for an answer"}}))
	if nm := stopped.(Model); strings.Contains(nm.toast, "sent to") || nm.flash != "not sent to a: the session in %1 no longer waits for an answer" {
		t.Fatalf("toast %q flash %q", nm.toast, nm.flash)
	}
}

// After a send the card has nothing picked: a second ⏎ (a double tap, a key
// repeat) answers nothing.
func TestASecondEnterNeverAnswersTwice(t *testing.T) {
	m, f := askModel(t, 100, threeWays)
	m = pressKeys(t, m, "enter", "enter")
	if got := pickActs(f); len(got) != 1 || m.flash != "pick an option" {
		t.Fatalf("picks %+v flash %q", got, m.flash)
	}
	if len(keyActs(f)) != 0 {
		t.Fatalf("the second enter went to the agent: %+v", keyActs(f))
	}
}

// A card's own keys change nothing on the session at night: only what goes
// to the agent asks to take over.
func TestCardKeysAskNoTakeOver(t *testing.T) {
	st := nightState()
	st.Sessions = []sample.Session{{PID: 1, Pane: "%1", Name: "a", Ask: &said.Ask{ID: "toolu_1", Questions: []said.Question{threeWays}}}}
	st.Queue = nil // the night's own fixture has %1 at a permission prompt
	m, _ := loadedWith(t, 100, st)
	m = pressKeys(t, m, "enter", "down")
	if m.held != nil || m.askCard.at != 1 {
		t.Fatalf("held %v at %d", m.held != nil, m.askCard.at)
	}
	if m = pressKeys(t, m, "esc"); m.held == nil {
		t.Fatal("esc to the agent did not ask to take over")
	}
}

func TestAOpensTheChatOnShipsAndStoker(t *testing.T) {
	st := screensState(t)
	m, _ := shipsAt(t, 86, st)
	next, _ := key(m, "down")
	next, _ = key(next, "down") // #4545 is stuck; flint-auth asks
	nm := pressKeys(t, next.(Model), "a")
	if s, ok := nm.selectedSession(); nm.tab != tabSessions || !ok || s.Pane != "%2" || !nm.chatKeys {
		t.Fatalf("a on a stuck ship: tab %d session %q chatKeys %v flash %q", nm.tab, s.Pane, nm.chatKeys, nm.flash)
	}
	if nm := pressKeys(t, m, "a"); nm.chatKeys || nm.flash == "" { // #4551 runs
		t.Fatal("a on a ship that runs opened the chat")
	}

	st = tabMorning(screensState(t))
	m, _ = loadedWith(t, 100, st)
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
	if line := actionLine(selectStep(t, m, "p5")); !strings.Contains(line, "a chat") {
		t.Fatalf("the key line does not offer a: %q", line)
	}
	nm = pressKeys(t, selectStep(t, m, "p5"), "a")
	if s, ok := nm.selectedSession(); nm.tab != tabSessions || !ok || s.Pane != pane || !nm.chatKeys {
		t.Fatalf("a on a failed step: tab %d session %q chatKeys %v flash %q", nm.tab, s.Pane, nm.chatKeys, nm.flash)
	}
	if nm := pressKeys(t, selectStep(t, m, "p2"), "a"); nm.chatKeys || !strings.Contains(nm.flash, "failed step") {
		t.Fatalf("a on a done step: chatKeys %v flash %q", nm.chatKeys, nm.flash)
	}
}

// clocked is m on a clock the test moves.
func clocked(m Model) (Model, *time.Time) {
	// From m's own time, never back: a card drawn on the wall clock stays
	// in its past whatever the day the test runs.
	now := m.now()
	m.opt.Now = func() time.Time { return now }
	return m, &now
}

// A card sends nothing for a moment after it shows: Enter Enter on a phone,
// whose first Enter opens the chat, answers nothing; nor does an Enter, a
// digit or a tap at once on a follow-up question. After the moment it
// sends.
func TestTheCardIsQuietAtFirst(t *testing.T) {
	m, f := loadedWith(t, 44, cardState("toolu_1", threeWays))
	m, now := clocked(m)
	m = pressKeys(t, onPane(t, m, "%1"), "enter", "enter")
	if !m.chatKeys || len(pickActs(f)) != 0 || m.flash != "not sent: a asks something new — look again" {
		t.Fatalf("enter enter: chatKeys %v picks %+v flash %q", m.chatKeys, pickActs(f), m.flash)
	}
	*now = now.Add(cardQuiet)
	m = pressKeys(t, m, "enter")
	if len(pickActs(f)) != 1 {
		t.Fatalf("after the quiet: picks %+v flash %q", pickActs(f), m.flash)
	}
	m = answered(t, m, "")

	// The agent asks again, with a new call: its card starts over, quiet.
	next, _ := m.Update(stateMsg(cardState("toolu_2", threeWays)))
	m = next.(Model)
	if _, c, _ := m.cardNow(); c.askID != "toolu_2" || c.at != 0 {
		t.Fatalf("the follow-up's card %+v", c)
	}
	m = pressKeys(t, m, "enter", "2")
	sc := m.draw(44)
	y := slices.IndexFunc(sc.lines, func(l string) bool { return strings.Contains(ansi.Strip(l), "3 Leave it") })
	next, cmd := tap(m, sc.cardX+4, y)
	drain(cmd)
	m = next.(Model)
	if len(pickActs(f)) != 1 || m.flash != "not sent: a asks something new — look again" {
		t.Fatalf("at once on the follow-up: picks %+v flash %q", pickActs(f), m.flash)
	}
	*now = now.Add(cardQuiet)
	pressKeys(t, m, "2")
	if got := pickActs(f); len(got) != 2 || got[1].Text != `{"id":"toolu_2","choices":[{"picks":[2]}]}` {
		t.Fatalf("after the quiet: picks %+v", got)
	}
}

// nightAsk is the console at night, at w columns, with the keys in the chat
// of a (%1) on the question tool, past the card's quiet.
func nightAsk(t *testing.T, w int) (Model, *fakeConn) {
	t.Helper()
	st := nightState()
	st.Sessions = []sample.Session{{PID: 1, Pane: "%1", Name: "a", Ask: &said.Ask{ID: "toolu_1", Questions: []said.Question{threeWays}}}}
	st.Queue = nil // the night's own fixture has %1 at a permission prompt
	m, f := loadedWith(t, w, st)
	return pastQuiet(pressKeys(t, m, "enter")), f
}

// At night the card's answers act on a session the stoker runs: Enter, a
// digit and a tap that picks ask to take over first, then answer the card
// they were pressed on. A newer question by then is not answered.
func TestTheCardsAnswerAsksToTakeOver(t *testing.T) {
	for _, k := range []string{"enter", "2"} {
		m, f := nightAsk(t, 100)
		m = pressKeys(t, m, k)
		if m.held == nil || len(pickActs(f)) != 0 {
			t.Fatalf("%s: held %v picks %+v", k, m.held != nil, pickActs(f))
		}
		pressKeys(t, m, "n")
		want := map[string]string{"enter": `{"id":"toolu_1","choices":[{"picks":[1]}]}`, "2": `{"id":"toolu_1","choices":[{"picks":[2]}]}`}[k]
		if got := pickActs(f); len(got) != 1 || got[0].Text != want {
			t.Fatalf("%s: picks %+v", k, got)
		}
	}

	m, f := nightAsk(t, 100)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)
	sc := m.draw(100)
	y := slices.IndexFunc(sc.lines, func(l string) bool { return strings.Contains(ansi.Strip(l), "3 Leave it") })
	next, cmd := tap(m, sc.cardX+4, y)
	drain(cmd)
	if m = next.(Model); m.held == nil || len(pickActs(f)) != 0 {
		t.Fatalf("a tap: held %v picks %+v", m.held != nil, pickActs(f))
	}
	pressKeys(t, m, "n")
	if got := pickActs(f); len(got) != 1 || got[0].Text != `{"id":"toolu_1","choices":[{"picks":[3]}]}` {
		t.Fatalf("a tap: picks %+v", got)
	}

	m, f = nightAsk(t, 100)
	m = pressKeys(t, m, "enter")
	st := nightState()
	st.Sessions = []sample.Session{{PID: 1, Pane: "%1", Name: "a", Ask: &said.Ask{ID: "toolu_2", Questions: []said.Question{threeWays}}}}
	st.Queue = nil
	next, _ = m.Update(stateMsg(st))
	m = pressKeys(t, next.(Model), "n")
	if len(pickActs(f)) != 0 || m.flash != "not sent: a asks something new — look again" {
		t.Fatalf("a newer question: picks %+v flash %q", pickActs(f), m.flash)
	}
}

// A permission prompt is answered with the agent's own keys, even while the
// transcript still holds a question tool call.
func TestNoCardAtAPermissionPromptWithAnAsk(t *testing.T) {
	st := queueState() // app-feature (%1) asks for permission
	for i := range st.Sessions {
		if st.Sessions[i].Pane == "%1" {
			st.Sessions[i].Ask = &said.Ask{ID: "toolu_1", Questions: []said.Question{threeWays}}
		}
	}
	m, f := loadedWith(t, 100, st)
	m = pastQuiet(pressKeys(t, onPane(t, m, "%1"), "enter"))
	if lines := m.cardLines(60); len(lines) != 0 {
		t.Fatalf("a card at a permission prompt: %q", lines)
	}
	pressKeys(t, m, "enter")
	if got := keyActs(f); len(got) != 1 || got[0].Text != "Enter" || len(pickActs(f)) != 0 {
		t.Fatalf("acts %+v", f.acts())
	}
}

// On a short screen the card keeps all its rows; the agent's screen above it
// gives way.
func TestTheCardKeepsItsRowsOnAShortScreen(t *testing.T) {
	m, _ := askModel(t, 44, threeWays)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 44, Height: 16})
	next, _ = next.Update(screenMsg{Pane: "%1", At: time.Unix(101, 0), Text: strings.Repeat("a line of the agent\n", 30) + "⏺ Which way?"})
	out := strings.Split(screen(next), "\n")
	if len(out) != 16 {
		t.Fatalf("%d lines:\n%s", len(out), strings.Join(out, "\n"))
	}
	if card := ansi.Strip(strings.Join(next.(Model).cardLines(44), "\n")); !strings.Contains(strings.Join(out, "\n"), card) {
		t.Fatalf("the card lost rows:\n%s\nwant:\n%s", strings.Join(out, "\n"), card)
	}
	if !strings.Contains(strings.Join(out, "\n"), "a line of the agent") {
		t.Fatalf("no room left for the agent's screen:\n%s", strings.Join(out, "\n"))
	}
}

// The card's next question is new to the builder: an Enter at once on it
// sends nothing.
func TestTheNextQuestionIsQuietToo(t *testing.T) {
	checks := said.Question{Question: "Which checks?", Options: []said.Option{{Label: "unit"}, {Label: "e2e"}}}
	m, f := askModel(t, 100, threeWays, checks)
	m, now := clocked(m)
	m = pressKeys(t, m, "enter", "enter")
	if len(pickActs(f)) != 0 || m.flash != "not sent: a asks something new — look again" {
		t.Fatalf("enter at once on question 2: picks %+v flash %q", pickActs(f), m.flash)
	}
	*now = now.Add(cardQuiet)
	pressKeys(t, m, "enter")
	if got := pickActs(f); len(got) != 1 || got[0].Text != `{"id":"toolu_1","choices":[{"picks":[1]},{"picks":[1]}]}` {
		t.Fatalf("after the quiet: picks %+v", got)
	}
}

// On a desk the card is on the right side before the chat has the keys: its
// quiet runs from when it was first drawn, and opening the chat does not
// start it again.
func TestADrawnCardIsNotQuietAgain(t *testing.T) {
	m, f := loadedWith(t, 200, cardState("toolu_1", threeWays))
	m, now := clocked(onPane(t, m, "%1"))
	m.cardSeen = cardSeen{}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m = next.(Model)
	if !strings.Contains(screen(m), "a asks") || m.cardSeen.askID != "toolu_1" {
		t.Fatalf("the card is not drawn on the right (seen %+v):\n%s", m.cardSeen, screen(m))
	}
	*now = now.Add(cardQuiet + 100*time.Millisecond)
	pressKeys(t, m, "enter", "down", "down", "enter")
	if got := pickActs(f); len(got) != 1 || got[0].Text != `{"id":"toolu_1","choices":[{"picks":[3]}]}` {
		t.Fatalf("picks %+v", got)
	}
}

func TestFitWrapsBetweenWords(t *testing.T) {
	got := fitScreen("⏺ The refresh endpoint is in and its tests pass.", 44, 10)
	if len(got) != 2 || ansi.Strip(got[0]) != "⏺ The refresh endpoint is in and its tests" || ansi.Strip(got[1]) != "pass." {
		t.Fatalf("wrap: %q", got)
	}
}

func TestFitBreaksALongWordHard(t *testing.T) {
	got := fitScreen("see "+strings.Repeat("x", 60), 44, 10)
	var xs string
	for _, l := range got {
		if w := ansi.StringWidth(l); w > 44 {
			t.Fatalf("line %q is %d wide", l, w)
		}
		xs += strings.ReplaceAll(strings.TrimPrefix(ansi.Strip(l), "see"), " ", "")
	}
	if xs != strings.Repeat("x", 60) {
		t.Fatalf("lost x's: %q", got)
	}
}

func TestFitWrapKeepsStyles(t *testing.T) {
	got := fitScreen("aaaa \x1b[31mredword\x1b[0m", 7, 10)
	if len(got) != 2 || !strings.Contains(got[1], "\x1b[31m") || ansi.Strip(got[1]) != "redword" {
		t.Fatalf("style lost: %q", got)
	}
}

func TestFitWrapTrimsBlanks(t *testing.T) {
	for _, l := range fitScreen("one two three four five six seven eight nine ten eleven", 12, 10) {
		if p := ansi.Strip(l); p != strings.TrimRight(p, " ") {
			t.Fatalf("trailing blank in %q", l)
		}
	}
}

func TestAgentMenuCut(t *testing.T) {
	got := agentMenuCut(askScreen)
	if !strings.Contains(got, "⏺ The refresh endpoint is in") {
		t.Fatalf("the work went: %q", got)
	}
	for _, gone := range []string{"❯ 1. Rotate it", "☐ Rotate", "Enter to select"} {
		if strings.Contains(got, gone) {
			t.Fatalf("%q stayed: %q", gone, got)
		}
	}
}

func TestNoRuleKeepsTheWholeScreen(t *testing.T) {
	if got := agentMenuCut("a\nb\n❯ 1. x"); got != "a\nb\n❯ 1. x" {
		t.Fatalf("cut %q", got)
	}
}

func TestCardDropsTheAgentsMenu(t *testing.T) {
	m, _ := askModel(t, 44, threeWays)
	next, _ := m.Update(screenMsg{Pane: "%1", At: time.Unix(200, 0), Text: askScreen})
	out := ansi.Strip(strings.Join(next.(Model).chatLines(44, 29), "\n"))
	if !strings.Contains(out, "Patch the token check") || strings.Contains(out, "❯ 1. Rotate it") {
		t.Fatalf("the question shows twice or not at all:\n%s", out)
	}
}

func TestPermissionKeepsTheAgentsMenu(t *testing.T) {
	m, _ := loadedWith(t, 44, queueState()) // app-feature (%1) asks for permission
	m = pressKeys(t, onPane(t, m, "%1"), "enter")
	next, _ := m.Update(screenMsg{Pane: "%1", At: time.Unix(200, 0), Text: askScreen})
	if out := ansi.Strip(strings.Join(next.(Model).chatLines(44, 29), "\n")); !strings.Contains(out, "❯ 1. Rotate it") {
		t.Fatalf("the agent's own menu went with no card:\n%s", out)
	}
}

func TestHistoryKeepsTheAgentsMenu(t *testing.T) {
	m, _ := askModel(t, 44, threeWays)
	m.up, m.watchUp = 6, 6 // scrolled up: a history frame is kept
	next, _ := m.Update(screenMsg{Pane: "%1", At: time.Unix(200, 0), Up: 6, Text: askScreen})
	m = next.(Model)
	if out := ansi.Strip(strings.Join(m.chatLines(44, 29), "\n")); !strings.Contains(out, "❯ 1. Rotate it") {
		t.Fatalf("history was cut:\n%s", out)
	}
}

// Claude Code draws a second rule inside its menu, over "Chat about this":
// the cut starts at the rule over the question's header, not the last one.
func TestAgentMenuCutOnARealPane(t *testing.T) {
	b, err := os.ReadFile("../../testdata/demo/panes/2.txt")
	if err != nil {
		t.Fatal(err)
	}
	got := agentMenuCut(string(b))
	if !strings.Contains(got, "wire the rotation.") {
		t.Fatalf("the work went: %q", got)
	}
	for _, gone := range []string{"☐ Refresh", "1. Rotate it", "Chat about this", "Enter to select"} {
		if strings.Contains(got, gone) {
			t.Fatalf("%q stayed: %q", gone, got)
		}
	}
}

func TestAgentMenuCutKeepsAPlainRule(t *testing.T) {
	screen := "⏺ done\n────────\n> \n────────\n  ? for shortcuts"
	if got := agentMenuCut(screen); got != screen {
		t.Fatalf("cut a prompt box: %q", got)
	}
}

// commandChat is the console with the keys in the chat of a, in %1, which
// runs agent.
func commandChat(t *testing.T, agent string) (Model, *fakeConn) {
	t.Helper()
	st := fixtureState()
	st.Sessions = []sample.Session{{PID: 1, Pane: "%1", Name: "a", Agent: agent}, {PID: 2, Pane: "%2", Name: "b"}}
	m, f := loadedWith(t, 100, st)
	m = onPane(t, m, "%1")
	next, _ := m.Update(screenMsg{Pane: m.watching, At: time.Unix(100, 0), Text: "> "})
	m = pressKeys(t, next.(Model), "enter")
	if !m.chatKeys {
		t.Fatal("enter did not give the chat the keys")
	}
	return m, f
}

// command is the command key id, as the phone sends it, read by the
// console's input decoder.
func command(t *testing.T, m Model, id string) Model {
	t.Helper()
	for _, e := range decoded(t, keycmd.Sequence(id)) {
		next, cmd := m.Update(e)
		drain(cmd)
		m = next.(Model)
	}
	return m
}

func TestCompactKeyRunsTheCompactAct(t *testing.T) {
	m, f := commandChat(t, "claude")
	m = command(t, m, "compact")
	if got := f.acts(); len(got) != 1 || got[0].RecID != "compact:%1" || got[0].Confirm != "y" || got[0].PID != 1 {
		t.Fatalf("acts %+v", got)
	}
	if m.flash != "" {
		t.Fatalf("flash %q", m.flash)
	}
}

func TestClearKeyRunsTheClearAct(t *testing.T) {
	m, f := commandChat(t, "claude")
	command(t, m, "clear")
	if got := f.acts(); len(got) != 1 || got[0].RecID != "clear:%1" || got[0].Confirm != "y" || got[0].PID != 1 {
		t.Fatalf("acts %+v", got)
	}
}

// TestTypeCommandRunsTheCommandAct: a typed command goes as one act the
// service guards (rest, no question, no draft), never as bare keys.
func TestTypeCommandRunsTheCommandAct(t *testing.T) {
	m, f := commandChat(t, "claude")
	m = command(t, m, "model")
	if got := f.acts(); len(got) != 1 || got[0].RecID != proto.CommandPrefix+"%1" || got[0].Text != "model" || got[0].PID != 1 || got[0].Confirm != "y" {
		t.Fatalf("acts %+v", got)
	}
	if m = answered(t, m, "/model not sent: the pane %1 has text you typed"); m.flash != "/model not sent: the pane %1 has text you typed" {
		t.Fatalf("flash %q", m.flash)
	}
}

func TestCommandForAnotherAgentSendsNothing(t *testing.T) {
	m, f := commandChat(t, "codex")
	m = command(t, m, "compact")
	if got := f.acts(); len(got) != 0 {
		t.Fatalf("acts %+v", got)
	}
	if want := "/compact is Claude Code's: a runs Codex"; m.flash != want {
		t.Fatalf("flash %q, want %q", m.flash, want)
	}
	m, _ = commandChat(t, "cursor-agent")
	if m = command(t, m, "codex-compact"); m.flash != "/compact is Codex's: a runs Cursor" {
		t.Fatalf("flash %q", m.flash)
	}
}

func TestCommandWithNoChatSendsNothing(t *testing.T) {
	m, f := commandChat(t, "claude")
	m = pressKeys(t, m, "ctrl+]")
	m = command(t, m, "compact")
	if got := f.acts(); len(got) != 0 {
		t.Fatalf("acts %+v", got)
	}
	if m.flash != "commands run from the chat" {
		t.Fatalf("flash %q", m.flash)
	}
}

// TestUnknownCommandSaysTheHostIsOlder: an app newer than the host sends
// an id the host's table does not have. Nothing goes, and the key line
// says what to do.
func TestUnknownCommandSaysTheHostIsOlder(t *testing.T) {
	m, f := commandChat(t, "claude")
	m = command(t, m, "nope")
	if got := f.acts(); len(got) != 0 || m.flash != "/nope is new to this console: update matchblox on the host" || !m.chatKeys {
		t.Fatalf("acts %+v flash %q chatKeys %v", got, m.flash, m.chatKeys)
	}
	if m = command(t, m, "no\x07pe"); m.flash != "/nope is new to this console: update matchblox on the host" {
		t.Fatalf("an id with a control character: flash %q", m.flash)
	}
}

// TestACommandUnderALayerSendsNothing: the layer on top holds the key
// line (here the take-over question), so the person sees what to close.
func TestACommandUnderALayerSendsNothing(t *testing.T) {
	st := nightState()
	st.Sessions = []sample.Session{{PID: 1, Pane: "%1", Name: "a", Agent: "claude"}}
	m, f := loadedWith(t, 100, st)
	m = command(t, pressKeys(t, onPane(t, m, "%1"), "enter"), "compact")
	m = command(t, m, "clear")
	if out := screen(m); len(f.acts()) != 0 || m.held == nil || !strings.Contains(out, "Take over?") {
		t.Fatalf("acts %+v, held %v\n%s", f.acts(), m.held != nil, out)
	}
}

func TestACommandAtNightAsksToTakeOver(t *testing.T) {
	st := nightState()
	st.Sessions = []sample.Session{{PID: 1, Pane: "%1", Name: "a", Agent: "claude"}}
	m, f := loadedWith(t, 100, st)
	m = pressKeys(t, onPane(t, m, "%1"), "enter")
	m = command(t, m, "compact")
	if m.held == nil || len(f.acts()) != 0 {
		t.Fatalf("held %v acts %+v", m.held != nil, f.acts())
	}
	pressKeys(t, m, "n")
	if got := f.acts(); len(got) != 2 || got[1].RecID != "compact:%1" {
		t.Fatalf("acts %+v", got)
	}
}

// cmdMsgs are the messages cmd gives, a batch's or a sequence's each in
// turn. A command that blocks (a read) gives none.
func cmdMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	out := make(chan tea.Msg, 1)
	go func() { out <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-out:
	case <-time.After(50 * time.Millisecond):
		return nil
	}
	if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeFor[tea.Cmd]() { // a batch or a sequence
		var got []tea.Msg
		for i := range v.Len() {
			got = append(got, cmdMsgs(v.Index(i).Interface().(tea.Cmd))...)
		}
		return got
	}
	return []tea.Msg{msg}
}

// raws are the strings cmd writes through Bubble Tea's raw output.
func raws(cmd tea.Cmd) []string {
	var got []string
	for _, msg := range cmdMsgs(cmd) {
		if r, ok := msg.(tea.RawMsg); ok {
			got = append(got, r.Msg.(string))
		}
	}
	return got
}

// agentRaws are the agent signals the console wrote while m took ks.
func agentRaws(t *testing.T, m Model, ks ...string) (Model, []string) {
	t.Helper()
	var got []string
	for _, k := range ks {
		next, cmd := key(m, k)
		got = append(got, raws(cmd)...)
		m = next.(Model)
	}
	return m, got
}

// agentChat is the console on a, in %1, which runs agent, and b, in %2,
// which runs other, with the chat of a on screen but not yet open.
func agentChat(t *testing.T, agent, other string) Model {
	t.Helper()
	st := fixtureState()
	st.Sessions = []sample.Session{{PID: 1, Pane: "%1", Name: "a", Agent: agent}, {PID: 2, Pane: "%2", Name: "b", Agent: other}}
	m, _ := loadedWith(t, 100, st)
	m = onPane(t, m, "%1")
	next, _ := m.Update(screenMsg{Pane: m.watching, At: time.Unix(100, 0), Text: "> "})
	return next.(Model)
}

func TestChatSendsItsAgent(t *testing.T) {
	m := agentChat(t, "claude", "codex")
	m, got := agentRaws(t, m, "enter")
	if want := logos.Passthrough(keycmd.AgentSignal("claude")); !slices.Equal(got, []string{want}) {
		t.Fatalf("open wrote %q, want %q", got, want)
	}
	if !m.chatKeys {
		t.Fatal("enter did not open the chat")
	}
	m = m.openChat()
	if _, got = agentRaws(t, m, "x", "y"); len(got) != 0 {
		t.Fatalf("the open chat wrote %q again", got)
	}
}

func TestChatCloseSendsNoAgent(t *testing.T) {
	m, _ := agentRaws(t, agentChat(t, "claude", "codex"), "enter")
	m, got := agentRaws(t, m, "ctrl+]")
	if want := logos.Passthrough(keycmd.AgentSignal("")); !slices.Equal(got, []string{want}) {
		t.Fatalf("close wrote %q, want %q", got, want)
	}
	if _, got = agentRaws(t, m, "down"); len(got) != 0 {
		t.Fatalf("the closed chat wrote %q", got)
	}
}

func TestSwitchingSessionSendsTheNewAgent(t *testing.T) {
	m, _ := agentRaws(t, agentChat(t, "claude", "cursor-agent"), "enter")
	next, cmd := m.chatOn("%2").Update(screenMsg{Pane: "%2", Text: "> "})
	if got, want := raws(cmd), logos.Passthrough(keycmd.AgentSignal("cursor")); !slices.Equal(got, []string{want}) {
		t.Fatalf("switch wrote %q, want %q", got, want)
	}
	if s, _ := next.(Model).selectedSession(); s.Pane != "%2" || !next.(Model).chatKeys {
		t.Fatalf("chat on %s, keys %v", s.Pane, next.(Model).chatKeys)
	}
}

func TestAgentSignalOutsideTmuxIsNotWrapped(t *testing.T) {
	m := agentChat(t, "opencode", "")
	m.opt.OutsideTmux = true
	if _, got := agentRaws(t, m, "enter"); !slices.Equal(got, []string{keycmd.AgentSignal("opencode")}) {
		t.Fatalf("open wrote %q", got)
	}
}

// lastUp is the lines up the last watch the console sent asks for.
func lastUp(t *testing.T, f *fakeConn) string {
	t.Helper()
	acts := f.allActs()
	for i := len(acts) - 1; i >= 0; i-- {
		if strings.HasPrefix(acts[i].RecID, proto.WatchPrefix) {
			if acts[i].RecID != proto.WatchPrefix+"%1" {
				t.Fatalf("watch %q", acts[i].RecID)
			}
			return acts[i].Text
		}
	}
	t.Fatal("no watch sent")
	return ""
}

func TestPageUpScrollsTheHistory(t *testing.T) {
	m, f := commandChat(t, "claude")
	page := m.draw(m.width).chatRows
	if page <= chatStep {
		t.Fatalf("the chat shows %d rows", page)
	}
	m = pressKeys(t, m, "pgup")
	if got, want := lastUp(t, f), proto.UpPrefix+strconv.Itoa(page); got != want || !m.chatKeys {
		t.Fatalf("pgup asked %q, want %q (keys %v)", got, want, m.chatKeys)
	}
	m = pressKeys(t, m, "pgup")
	if got := lastUp(t, f); got != proto.UpPrefix+strconv.Itoa(2*page) {
		t.Fatalf("second pgup asked %q", got)
	}
	m = pressKeys(t, m, "pgdown")
	if got := lastUp(t, f); got != proto.UpPrefix+strconv.Itoa(page) {
		t.Fatalf("pgdown asked %q", got)
	}
	m = pressKeys(t, m, "x") // any other key goes back to live and types nothing
	if m.up != 0 || len(keyActs(f)) != 0 {
		t.Fatalf("up %d, keys %+v", m.up, keyActs(f))
	}
	m = pressKeys(t, m, "pgup", "pgdown")
	if got := lastUp(t, f); got != proto.UpPrefix+"0" || m.up != 0 {
		t.Fatalf("pgdown to live asked %q", got)
	}
}

func TestOldTmuxSaysCommandsNeedIt(t *testing.T) {
	m := agentChat(t, "claude", "codex")
	m.opt.LocalTmux = "3.2a"
	m, got := agentRaws(t, m, "enter")
	if len(got) != 0 {
		t.Fatalf("an old tmux got %q", got)
	}
	if m.flash != "COMMANDS · needs tmux 3.3" || !strings.Contains(screen(m), "COMMANDS · needs tmux 3.3") {
		t.Fatalf("flash %q\n%s", m.flash, screen(m))
	}
	m, got = agentRaws(t, m, "ctrl+]", "enter")
	if len(got) != 0 || m.flash != "" {
		t.Fatalf("second open wrote %q, flash %q", got, m.flash)
	}
}

func TestQuitWithTheChatOpenSendsNoAgent(t *testing.T) {
	m, _ := agentRaws(t, agentChat(t, "claude", "codex"), "enter")
	_, cmd := m.quit()
	got := cmdMsgs(cmd)
	want := tea.RawMsg{Msg: logos.Passthrough(keycmd.AgentSignal(""))}
	if len(got) != 2 || got[0] != want || got[1] != (tea.QuitMsg{}) {
		t.Fatalf("quit gave %#v, want the empty agent, then quit", got)
	}
	if _, cmd = agentChat(t, "claude", "codex").quit(); len(raws(cmd)) != 0 {
		t.Fatalf("a quit with no chat told the app %q", raws(cmd))
	}
}

func TestBackToHostsWithTheChatOpenSendsNoAgent(t *testing.T) {
	r := newShell(t, nil, nil, "", false)
	m, _ := agentRaws(t, agentChat(t, "claude", "codex"), "enter")
	r.sh.cur = m
	next, cmd := r.sh.Update(layerMsg{})
	if got, want := raws(cmd), logos.Passthrough(keycmd.AgentSignal("")); !slices.Equal(got, []string{want}) || next.(Shell).layer != layerHosts {
		t.Fatalf("back wrote %q, want %q (layer %v)", got, want, next.(Shell).layer)
	}
}

// TestANewViewerGetsTheAgentAgain: the phone attaches to a console whose
// chat is open already (after a reconnect, or beside the desktop). It
// starts with no agent, so the console sends the chat's agent again to the
// new set of viewers, and once only.
func TestANewViewerGetsTheAgentAgain(t *testing.T) {
	m, _ := agentRaws(t, agentChat(t, "claude", "codex"), "enter")
	next, cmd := m.Update(graphicsMsg{gen: m.opt.Gen, watched: true, key: "/dev/ttys002:100@here"})
	want := logos.Passthrough(keycmd.AgentSignal("claude"))
	if got := raws(cmd); !slices.Equal(got, []string{want}) {
		t.Fatalf("the first viewers got %q, want %q", got, want)
	}
	m = next.(Model)
	if _, cmd = m.Update(graphicsMsg{gen: m.opt.Gen, watched: true, key: "/dev/ttys002:100@here"}); len(raws(cmd)) != 0 {
		t.Fatalf("the same viewers got %q again", raws(cmd))
	}
	phone := "/dev/ttys002:100@here,/dev/ttys005:105@here"
	next, cmd = m.Update(graphicsMsg{gen: m.opt.Gen, watched: true, key: phone})
	if got := raws(cmd); !slices.Equal(got, []string{want}) {
		t.Fatalf("a phone that attached got %q, want %q", got, want)
	}
	m, _ = agentRaws(t, next.(Model), "ctrl+]")
	if _, cmd = m.Update(graphicsMsg{gen: m.opt.Gen, watched: true, key: "/dev/ttys005:205@here"}); len(raws(cmd)) != 0 {
		t.Fatalf("a viewer of a closed chat got %q", raws(cmd))
	}
}

// TestAChatOnAnAgentWithNoCommandsIsStillTheChat: the app hears the chat
// is open, so its command keys go, and the console says why one does not
// run there, instead of the app saying the commands run from the chat.
func TestAChatOnAnAgentWithNoCommandsIsStillTheChat(t *testing.T) {
	m, got := agentRaws(t, agentChat(t, "aider", "codex"), "enter")
	if want := logos.Passthrough(keycmd.AgentSignal(keycmd.OtherAgent)); !slices.Equal(got, []string{want}) {
		t.Fatalf("open wrote %q, want %q", got, want)
	}
	if m = command(t, m, "compact"); m.flash != "/compact is Claude Code's: a runs aider" {
		t.Fatalf("flash %q", m.flash)
	}
}
