package app

import (
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
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
	return loadedWith(t, 100, st)
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
	m, f := viewModel(t)
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
	m, _ := viewModel(t)
	sel, _ := m.selected()
	next, _ := m.Update(screenMsg{Pane: sel.Pane, Text: "one"})
	if lines := next.(Model).viewLines(40, -1); len(lines) != 0 {
		t.Fatalf("lines = %q", lines)
	}
}

func TestRefusalOfAnOldWatchHoldsNothing(t *testing.T) {
	m, _ := viewModel(t)
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
	if sc.listW != 38 {
		t.Fatalf("the list is %d of 86 columns", sc.listW)
	}
	if sc.dockAt >= 0 {
		t.Fatal("a dock under the chat: usage goes under the list")
	}
	_, right := sides(sc, 86)
	if !slices.ContainsFunc(right, func(l string) bool { return strings.Contains(l, "Esc to") }) {
		t.Fatalf("the right half has no screen:\n%s", strings.Join(right, "\n"))
	}
}

func TestNarrowSwapsToChat(t *testing.T) {
	list := demoSized(t, 44, 31)
	m := openView(t, list)
	sc := m.draw(44)
	if got := strings.TrimSpace(ansi.Strip(sc.lines[0])); got != "‹ Sessions · spark-docs · done" {
		t.Fatalf("title row %q", got)
	}
	out := ansi.Strip(strings.Join(sc.lines, "\n"))
	if len(sc.lines) != 31 || strings.Contains(out, "WAITS FOR YOU") || !strings.Contains(out, "Esc to") {
		t.Fatalf("not the chat alone, %d lines:\n%s", len(sc.lines), out)
	}
	next, _ := m.Update(tea.MouseClickMsg(tea.Mouse{X: 3, Y: 0, Button: tea.MouseLeft}))
	if m = next.(Model); m.standIn || screen(m) != screen(list) {
		t.Fatalf("a tap on ‹ Sessions: stand-in %v, not the list as before:\n%s", m.standIn, screen(m))
	}
	tall := openView(t, fullView(demoSized(t, 44, 50)))
	next, _ = tall.Update(tea.MouseClickMsg(tea.Mouse{X: 3, Y: 0, Button: tea.MouseLeft}))
	if sc := next.(Model).draw(44); sc.dockAt < 0 {
		t.Fatalf("no dock under the list:\n%s", screen(next))
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
	if len(sc.lines) != 50 || !slices.ContainsFunc(right, func(l string) bool { return strings.Contains(l, "Enter to select") }) {
		t.Fatalf("%d lines, or the agent's menu is gone:\n%s", len(sc.lines), strings.Join(right, "\n"))
	}
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

// On a short desk the machine folds before the list is cut; usage keeps
// its rows and the list gives way to it alone, down to 3 rows and the
// hint under them; shorter, usage folds too.
func TestAShortDeskKeepsItsStats(t *testing.T) {
	m := demoSized(t, 132, 32)
	left, _ := sides(m.draw(132), 132)
	all := strings.Join(left, "\n")
	if !strings.Contains(all, "USAGE") || strings.Contains(all, "MACHINE") || !strings.Contains(all, "↓ ") {
		t.Fatalf("132x32: want usage over a cut list, the machine folded:\n%s", all)
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

// inView is a phone with the first session's screen in its live view.
func inView(t *testing.T) Model {
	t.Helper()
	m, _ := viewModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 44, Height: 30})
	m = onPane(t, next.(Model), "%1")
	next, _ = m.Update(screenMsg{Pane: m.watching, At: time.Unix(100, 0), Text: "> "})
	m = openView(t, next.(Model))
	if !m.viewFull() {
		t.Fatal("the view does not take the screen")
	}
	return m
}

func TestAGoneScreenSaysTheSessionEnded(t *testing.T) {
	next, _ := inView(t).Update(screenMsg{Pane: "%1", Gone: true})
	if m := next.(Model); m.viewOpen || m.flash != "That session ended." {
		t.Fatalf("view %v, flash %q: want the list, and why", m.viewOpen, m.flash)
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
	if !m.viewOpen || m.screen.Text != "> " || !strings.Contains(m.flash, "could not read") {
		t.Fatalf("view %v screen %+v flash %q", m.viewOpen, m.screen, m.flash)
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

// moveDown presses down once.
func moveDown(m Model) Model {
	next, cmd := key(m, "down")
	drain(cmd)
	return next.(Model)
}

// watches tells if the last watch act named pane.
func (f *fakeConn) watches(pane string) bool {
	acts := f.allActs()
	for i := len(acts) - 1; i >= 0; i-- {
		if w, ok := strings.CutPrefix(acts[i].RecID, proto.WatchPrefix); ok {
			return w == pane
		}
	}
	return false
}

// sessionPane is the selected session's pane.
func (m Model) sessionPane() string {
	s, _ := m.selectedSession()
	return s.Pane
}

// T26 (owner, 2026-11-08): on one column the first tap opens the view, the second jumps.
func TestOneColumnShowsTheLiveViewThenJumps(t *testing.T) {
	m, f := loadedWith(t, 44, fixtureState()) // a phone: one column
	var ran [][]string
	m.opt.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	m = moveDown(m) // select the second session: the view follows the selection
	if !f.watches(m.sessionPane()) || len(ran) != 0 {
		t.Fatalf("selecting: watch %v, ran %q; want its live view and no jump", f.watches(m.sessionPane()), ran)
	}
	sc := m.draw(44)
	i := slices.Index(sc.rows, m.rowIndex())
	if i < 0 {
		t.Fatal("the selected row is not on the screen")
	}
	_, cmd := tap(m, 2, sc.bodyTop+i) // the selected row: go into its pane
	drain(cmd)
	// No client asked here: the jump moves the desk (Task 4), not a FocusArgv.
	if len(ran) != 1 || !remote.NavAllowed(ran[0]) || ran[0][3] != m.sessionPane() {
		t.Fatalf("second tap ran %q, want a jump into %s", ran, m.sessionPane())
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

func TestTheLiveViewNamesTheWayIn(t *testing.T) {
	m := loaded(t, 100)
	if out := ansi.Strip(m.render()); !strings.Contains(out, "enter goes into app-feature") {
		t.Errorf("no way in on the key line:\n%s", out)
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
