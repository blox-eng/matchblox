package app

import (
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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

func viewModel(t *testing.T) (Model, *fakeConn) {
	t.Helper()
	st := fixtureState()
	st.Sessions = []sample.Session{
		{PID: 1, Pane: "%1", Name: "a"},
		{PID: 2, Pane: "%2", Name: "b"},
	}
	return loadedWith(t, 160, st) // the board watches its chosen card beside its live view
}

func TestScreenOfAnotherPaneIsIgnored(t *testing.T) {
	m, _ := viewModel(t)
	next, _ := m.Update(screenMsg{Pane: m.watching, Text: "mine"})
	m = next.(Model)
	next, _ = m.Update(screenMsg{Pane: "%9", Text: "other"})
	if got := next.(Model).screen; got.Text != "mine" {
		t.Fatalf("screen = %+v", got)
	}
}

func TestSelectionChangeWatches(t *testing.T) {
	// the two matches are in two columns of the board: → moves between them
	m, f := viewModel(t)
	first, _ := m.selected()
	next, cmd := key(m, "right")
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
	m, _ := viewModel(t)
	sel, _ := m.selected()
	next, _ := m.Update(screenMsg{Pane: sel.Pane, Text: "old"})
	next, _ = key(next, "down")
	if got := next.(Model).screen; got.Text != "" {
		t.Fatalf("screen of the old pane stayed: %+v", got)
	}
}

func TestLostConnectionForgetsTheWatch(t *testing.T) {
	m, _ := viewModel(t)
	next, _ := m.Update(lostMsg{err: io.EOF, conn: m.conn})
	if next.(Model).watching != "" {
		t.Fatal("still watching after the connection went")
	}
}

func TestChatLinesWhenTheSessionEnded(t *testing.T) {
	m, _ := viewModel(t)
	sel, _ := m.selected()
	next, _ := m.Update(screenMsg{Pane: sel.Pane, Gone: true})
	lines := next.(Model).viewLines(40, 5)
	if len(lines) != 5 || strings.TrimSpace(lines[2]) != "That session ended." {
		t.Fatalf("lines = %q", lines)
	}
}

func TestChatLinesFillTheViewFromTheBottom(t *testing.T) {
	m, _ := viewModel(t)
	sel, _ := m.selected()
	next, _ := m.Update(screenMsg{Pane: sel.Pane, Text: "one\ntwo"})
	lines := next.(Model).viewLines(40, 5)
	if len(lines) != 5 || lines[3] != "one" || lines[4] != "two" || lines[0] != "" {
		t.Fatalf("lines = %q", lines)
	}
	next, _ = m.Update(screenMsg{Pane: sel.Pane, Text: "   \n\n"})
	if lines := next.(Model).viewLines(40, 5); len(lines) != 5 {
		t.Fatalf("a blank screen gave %d lines", len(lines))
	}
}

func TestRefusedWatchRetriesOnTheNextSnapshotAndSaysNothing(t *testing.T) {
	m, f := viewModel(t)
	sel, _ := m.selected()
	next, cmd := m.Update(resultMsg(proto.Result{ActID: "watch:" + sel.Pane, Err: "no agent session in " + sel.Pane + " any more"}))
	drain(cmd)
	mm := next.(Model)
	if mm.flash != "" || mm.watching != "" {
		t.Fatalf("flash %q watching %q", mm.flash, mm.watching)
	}
	next, cmd = mm.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
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
	m, _ := viewModel(t)
	sel, _ := m.selected()
	next, _ := m.Update(screenMsg{Pane: sel.Pane, Text: "one"})
	if lines := next.(Model).viewLines(40, -1); len(lines) != 0 {
		t.Fatalf("lines = %q", lines)
	}
}

func TestRefusalOfAnOldWatchHoldsNothing(t *testing.T) {
	// the two matches are in two columns of the board: → moves between them
	m, _ := viewModel(t)
	a, _ := m.selected()
	next, cmd := key(m, "right")
	drain(cmd)
	b, _ := next.(Model).selected()
	next, _ = next.Update(screenMsg{Pane: b.Pane, Text: "kept"})
	next, _ = next.Update(resultMsg(proto.Result{ActID: "watch:" + a.Pane, Err: "no agent session in " + a.Pane + " any more"}))
	mm := next.(Model)
	if mm.watchHeld != "" || mm.watching != b.Pane || mm.screen.Text != "kept" {
		t.Fatalf("held %q watching %q screen %q", mm.watchHeld, mm.watching, mm.screen.Text)
	}
}

// onPane is m with the session in pane selected.
func onPane(t *testing.T, m Model, pane string) Model {
	t.Helper()
	next, cmd := selectPane(t, m, pane).Update(noticeMsg{}) // the board watches its chosen card
	drain(cmd)
	return next.(Model)
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

func TestKeyNames(t *testing.T) {
	for k, want := range map[string]string{
		"up": "Up", "down": "Down", "left": "Left", "right": "Right", "enter": "Enter", "tab": "Tab",
		"shift+tab": "BTab", "esc": "Escape", "ctrl+c": "C-c", "ctrl+a": "C-a", "backspace": "BSpace", "space": "lit: ", "y": "lit:y", "Y": "lit:Y",
	} {
		if got, ok := keyName(keyPress(k)); !ok || got != want {
			t.Errorf("%s = %q %v, want %q", k, got, ok, want)
		}
	}
	for _, l := range "bdefgklnprtuvwxy" {
		k := "ctrl+" + string(l)
		if got, ok := keyName(keyPress(k)); !ok || got != "C-"+string(l) {
			t.Errorf("%s = %q %v, want C-%c", k, got, ok, l)
		}
	}
	// ctrl+z would stop the agent; ctrl+s and ctrl+q stop and start the
	// terminal; ctrl+h, i, j and m are Backspace, Tab and Enter, which have
	// their own names; ctrl+o is the console's own.
	for _, k := range []string{"ctrl+z", "ctrl+s", "ctrl+q", "ctrl+h", "ctrl+i", "ctrl+j", "ctrl+m", "ctrl+o"} {
		if got, ok := keyName(keyPress(k)); ok {
			t.Errorf("%s = %q, want no key", k, got)
		}
	}
}

// demoSized is the demo machine at w by h, the selected session's screen
// a question with three options.
func demoSized(t testing.TB, w, h int) Model {
	t.Helper()
	m := New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	next, _ = next.Update(stateMsg(screensState(t)))
	nm := next.(Model)
	nm.splashDone = true
	next, _ = nm.Update(screenMsg{Pane: nm.watching, At: demoAt, Text: askScreen})
	return next.(Model)
}

func TestACutListSaysHowManyAreBelow(t *testing.T) {
	m := demoSized(t, 132, 32)
	sc := m.draw(132)
	left := plainLines(sc.lines)
	if !slices.ContainsFunc(left, func(l string) bool {
		return strings.HasPrefix(strings.TrimSpace(l), "↓ ") && strings.Contains(l, " more")
	}) {
		t.Fatalf("the cut list does not say what is below:\n%s", strings.Join(left, "\n"))
	}
	last := pressKeys(t, m, slices.Repeat([]string{"down"}, m.rowCount())...)
	left = plainLines(last.draw(132).lines)
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

// A session with no account, no PR and no background work has no strip:
// the screen starts under the title.
func TestTheChatStripTakesNoRowsWithoutFacts(t *testing.T) {
	m := flintSized(t, 200, 50, func(s *sample.Session) {
		s.Provider, s.Account, s.Agents, s.Shells = "", "", 0, nil
	})
	m.shipReport, m.allShips = nil, nil
	s, _ := m.selectedSession()
	got := m.viewPane(s, 121, 40)
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

// inView is a phone with the first session's screen in its live view.
func inView(t *testing.T) Model {
	t.Helper()
	m, _ := viewModel(t)
	m = onPane(t, m, "%1")
	next, _ := m.Update(screenMsg{Pane: m.watching, At: time.Unix(100, 0), Text: "> "})
	return next.(Model)
}

func TestAGoneScreenSaysTheSessionEnded(t *testing.T) {
	next, _ := inView(t).Update(screenMsg{Pane: "%1", Gone: true})
	m := next.(Model)
	if out := ansi.Strip(screen(m)); !strings.Contains(out, "That session ended.") {
		t.Fatalf("the view does not say the session ended:\n%s", out)
	}
}

var threeWays = said.Question{Question: "Which way do we fix the login?", Options: []said.Option{
	{Label: "Patch the token check"}, {Label: "Move to the new session store"}, {Label: "Leave it"}}}

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

// A screen the service could not read is not a session that ended: the
// view stays open on the last screen, and the key line says why.
func TestAFailedReadKeepsTheChat(t *testing.T) {
	next, _ := inView(t).Update(screenMsg{Pane: "%1", At: time.Unix(101, 0), Err: "could not read %1: tmux did not answer"})
	m := next.(Model)
	if m.screen.Text != "> " || !strings.Contains(m.flash, "could not read") {
		t.Fatalf("screen %+v flash %q", m.screen, m.flash)
	}
}

// An agent's screen reaches the console's terminal with its colours only:
// a hyperlink (OSC 8), any other OSC, an APC or a DCS goes, its text stays.
func TestTheScreenDropsLinksAndOtherControls(t *testing.T) {
	text := "see \x1b]8;;https://evil.example/\x1b\\the docs\x1b]8;;\x1b\\ \x1b[1mbold\x1b[0m" +
		" \x1b]0;title\x07\x1b_apc\x1b\\\x1bPdcs\x1b\\end"
	out := strings.Join(fitScreen(text, 80, 5), "\n")
	for _, bad := range []string{"\x1b]", "\x1b_", "\x1bP", "evil"} {
		if strings.Contains(out, bad) {
			t.Errorf("%q kept %q", out, bad)
		}
	}
	if ansi.Strip(out) != "see the docs bold end" || !strings.Contains(out, "\x1b[1m") {
		t.Errorf("text or colours lost: %q", out)
	}
}

// Only colours reach the console's terminal from an agent's screen: any
// other escape sequence (a cursor move, a screen clear, a mode, a reset),
// a C1 control and any other C0 control goes, in both fits, its text stays.
func TestOnlyColoursReachTheConsole(t *testing.T) {
	text := "a\x1b[2Jb\x1b[10;10Hc\x1b[?1049hd\x1bce\x1b7f\u009b31mg\u009dh\x07i\x08j \x1b[38;5;1mred\x1b[m"
	for name, out := range map[string]string{
		"fitScreen":  strings.Join(fitScreen(text, 80, 5), "\n"),
		"cropScreen": strings.Join(cropScreen(text, 80, 5), "\n"),
	} {
		if plain := ansi.Strip(out); plain != "abcdefghij red" {
			t.Errorf("%s: text %q, want abcdefghij red (%q)", name, plain, out)
		}
		for _, bad := range []string{"\x1b[2J", "\x1b[10;10H", "\x1b[?1049h", "\x1bc", "\x1b7", "\u009b", "\u009d", "\x07", "\x08"} {
			if strings.Contains(out, bad) {
				t.Errorf("%s kept %q: %q", name, bad, out)
			}
		}
		if !strings.Contains(out, "\x1b[38;5;1m") {
			t.Errorf("%s lost the colour: %q", name, out)
		}
	}
}

func TestTheLiveViewTakesNoKeys(t *testing.T) { // T6
	for _, w := range []int{100, 44} {
		m, f := loadedWith(t, w, fixtureState())
		if w == 44 {
			m = openView(t, m)
		}
		for _, k := range []string{"2", "y", "esc", "tab", "backspace", "space", "ctrl+a", "m", "up", "down"} {
			next, cmd := key(m, k)
			drain(cmd)
			m = next.(Model)
		}
		for _, a := range f.acts() {
			if strings.HasPrefix(a.RecID, proto.KeysPrefix) {
				t.Errorf("width %d: a key reached the agent: %+v", w, a)
			}
		}
	}
}

func TestTheStandInIsTheDemosOnly(t *testing.T) { // T7
	m, f := loadedWith(t, 100, fixtureState())
	m.tab = tabSessions
	if nm := m.openStandIn("%1"); nm.standIn {
		t.Fatal("the stand-in opened outside the demo")
	}
	m.standIn = true // even forced, it forwards nothing
	_, cmd := m.standInKey(keyPress("2"))
	drain(cmd)
	if len(f.acts()) != 0 {
		t.Errorf("the stand-in sent %+v outside the demo", f.acts())
	}
}
