package app

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
)

// queueState is the fixture machine with two sessions that wait: app-feature
// at a permission prompt, app-review done with its turn.
func queueState() proto.State {
	st := fixtureState()
	for i, s := range st.Sessions {
		switch s.Pane {
		case "%1":
			st.Sessions[i].Busy, st.Sessions[i].Status, st.Sessions[i].Idle = false, "idle", 3*time.Minute
		case "%2":
			st.Sessions[i].Idle = time.Minute
		}
	}
	st.Queue = []queue.Item{
		{SessionID: "a", Pane: "%1", Target: "work:1.1", Name: "app-feature", State: queue.StatePermission,
			Since: st.At.Add(-3 * time.Minute), LastLine: "Claude needs your permission to use Bash"},
		{SessionID: "b", Pane: "%2", Target: "work:2.1", Name: "app-review", State: queue.StateFinished,
			Since: st.At.Add(-time.Minute), Estimated: true},
	}
	return st
}

func typeText(m tea.Model, s string) tea.Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyPressMsg{Text: string(r), Code: r})
	}
	return m
}

// TestSessionsIsTheDefaultTab: the console opens on Sessions, on the first
// session that waits; its detail says what it asked.
func TestSessionsIsTheDefaultTab(t *testing.T) {
	m, _ := loadedWith(t, 132, queueState())
	out := ansi.Strip(m.render())
	if m.tab != tabSessions {
		t.Fatalf("the console opens on tab %d", m.tab)
	}
	for _, want := range []string{"SESSIONS 3", "WAITS FOR YOU · 2", "asks", "app-feature", "Claude needs your permission to use Bash", "panes"} {
		if !strings.Contains(out, want) {
			t.Fatalf("view lacks %q:\n%s", want, out)
		}
	}
}

func TestEnterOnQueueRowSwitches(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	var ran []string
	m.opt.Run = func(argv []string) error { ran = argv; return nil }
	next, _ := key(m, "enter")
	next, cmd := key(next, "enter")
	if cmd == nil {
		t.Fatal("no command")
	}
	next.Update(cmd())
	if strings.Join(ran, " ") != "tmux switch-client -t %1" {
		t.Fatalf("ran %v", ran)
	}
}

func TestEnterOutsideTmuxAttaches(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	m.opt.OutsideTmux = true
	var ran [][]string
	m.opt.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	next, _ := key(m, "enter")
	p := next.(Model).pending
	want := [][]string{
		{"tmux", "select-window", "-t", "%1"},
		{"tmux", "select-pane", "-t", "%1"},
		{"tmux", "attach-session", "-t", "%1"},
	}
	if p == nil || !reflect.DeepEqual(p.steps, want) || !p.attach {
		t.Fatalf("pending %+v", p)
	}
	for _, step := range want {
		if !remote.NavAllowed(step) {
			t.Fatalf("nav refuses %q", step)
		}
	}
	_, cmd := key(next, "enter")
	if cmd == nil {
		t.Fatal("no command")
	}
	// The attach runs as a process the console gives the terminal to, and
	// comes back from when it exits; the moves before it run first.
	if len(ran) != 2 || ran[1][1] != "select-pane" {
		t.Fatalf("ran %q", ran)
	}
}

// TestThePeekSendsOnEnter: a shows what the session said and a typing
// line; Enter sends the line to the pane named in the title with no
// second confirm, the toast says where it went, and the peek moves to the
// next session that waits.
func TestThePeekSendsOnEnter(t *testing.T) {
	m, f := loadedWith(t, 100, queueState())
	m, _ = func() (Model, tea.Cmd) { n, c := key(m, "down"); return n.(Model), c }() // app-review finished its turn
	next, _ := key(m, "a")
	next = typeText(next, "yes, open it")
	out := ansi.Strip(next.(Model).render())
	for _, want := range []string{"app-review said", "› yes, open it▏", "⏎ send", "⇥ next (1)", "esc"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the peek lacks %q:\n%s", want, out)
		}
	}
	next, cmd := key(next, "enter")
	if cmd == nil {
		t.Fatal("enter sent nothing")
	}
	drain(cmd)
	acts := f.acts()
	if len(acts) != 1 || acts[0] != (proto.Act{RecID: "answer:%2", Which: "secondary", Confirm: "y", Text: "yes, open it"}) {
		t.Fatalf("acts %+v", acts)
	}
	nm := next.(Model)
	if nm.toast != "sent to app-review" || nm.peek == nil || nm.peek.pane != "%1" {
		t.Fatalf("toast %q peek %+v: the peek moves to app-feature", nm.toast, nm.peek)
	}
}

// Review 1: Enter at a permission prompt approves it; a typed answer never
// goes there. Its peek shows the prompt and opens the pane.
func TestAnswerOnPermissionGoesToPane(t *testing.T) {
	m, f := loadedWith(t, 100, queueState()) // app-feature asks for permission
	var ran []string
	m.opt.Run = func(argv []string) error { ran = argv; return nil }
	next, _ := key(m, "a")
	out := ansi.Strip(next.(Model).render())
	if !strings.Contains(out, "app-feature asks") || !strings.Contains(out, "Claude needs your permission") ||
		!strings.Contains(out, "⏎ open") || strings.Contains(out, "▏") {
		t.Fatalf("the peek of a permission prompt:\n%s", out)
	}
	next = typeText(next, "yes")
	next, cmd := key(next, "enter")
	if cmd == nil {
		t.Fatal("enter did not open the pane")
	}
	next.Update(cmd())
	if strings.Join(ran, " ") != "tmux switch-client -t %1" || len(f.acts()) != 0 {
		t.Fatalf("ran %q acts %+v", ran, f.acts())
	}
}

// TestThePeekAnswersTheQuestionTool: the options of the question tool's
// call, numbered with the typing option last; a digit picks, several
// questions go one at a time, several choices tick with space and go on
// Enter, and the answers go to the service as one pick.
func TestThePeekAnswersTheQuestionTool(t *testing.T) {
	st := queueState()
	for i := range st.Sessions {
		if st.Sessions[i].Pane == "%2" {
			st.Sessions[i].Said = "Task 10 is done. Two ways on from here."
			st.Sessions[i].Ask = &said.Ask{ID: "toolu_1", Questions: []said.Question{
				{Question: "Which next?", Options: []said.Option{{Label: "Start task 11"}, {Label: "Fix the flake"}}},
				{Question: "Which checks?", Multi: true, Options: []said.Option{{Label: "unit"}, {Label: "e2e"}, {Label: "lint"}}},
			}}
		}
	}
	for _, w := range []int{44, 86} {
		m, f := loadedWith(t, w, st)
		m, _ = func() (Model, tea.Cmd) { n, c := key(m, "down"); return n.(Model), c }()
		next, _ := key(m, "a")
		out := ansi.Strip(next.(Model).render())
		for _, want := range []string{"app-review asks · 1 of 2", "Task 10 is done", "Which next?", "› 1 Start task 11", "2 Fix the flake", "3 Type an answer…"} {
			if !strings.Contains(out, want) {
				t.Fatalf("width %d: the peek lacks %q:\n%s", w, want, out)
			}
		}
		next, _ = key(next, "2")
		next, _ = key(next, " ") // the cursor's option: unit
		next, _ = key(next, "3")
		out = ansi.Strip(next.(Model).render())
		for _, want := range []string{"2 of 2", "Which checks?", "[✔] unit", "[ ] e2e", "[✔] lint", "␣ tick"} {
			if !strings.Contains(out, want) {
				t.Fatalf("width %d: the second question lacks %q:\n%s", w, want, out)
			}
		}
		_, cmd := key(next, "enter")
		drain(cmd)
		acts := f.acts()
		want := `{"id":"toolu_1","choices":[{"picks":[2]},{"picks":[1,3]}]}`
		if len(acts) != 1 || acts[0].RecID != "pick:%2" || acts[0].Confirm != "y" || acts[0].Text != want {
			t.Fatalf("width %d: acts %+v", w, acts)
		}
	}
}

// At 80 columns every tab is named, and the alert marker fits.
func TestTabsFit80WithAlert(t *testing.T) {
	st := queueState()
	st.Alerts = []sample.Alert{{Key: "load", Level: "warn", Title: "load"}}
	m, _ := loadedWith(t, 80, st)
	line, _ := m.tabsLine(80)
	line = ansi.Strip(line)
	for _, want := range []string{"SESSIONS", "ships", "machine", "git", "history", "panes", "▲ 1 alert"} {
		if !strings.Contains(line, want) || strings.Contains(line, "…") {
			t.Fatalf("tab line %q lacks %q", line, want)
		}
	}
}

func TestAnswerEscCancels(t *testing.T) {
	m0, f := loadedWith(t, 100, queueState())
	down, _ := key(m0, "down") // app-review finished its turn
	m := down.(Model)
	next, _ := key(m, "a")
	next = typeText(next, "no")
	next, _ = key(next, "esc")
	if next.(Model).peek != nil || next.(Model).pending != nil || len(f.acts()) != 0 {
		t.Fatal("esc did not cancel")
	}
	// q inside the peek is text, not quit.
	next, _ = key(m, "a")
	next, cmd := key(next, "q")
	if cmd != nil || next.(Model).quitting {
		t.Fatal("q quit while typing")
	}
}

func TestSessionMatchGlyphs(t *testing.T) {
	m := loaded(t, 120) // no motion: each state is one still glyph
	m.opt.CompactAt = 85
	for _, tc := range []struct {
		busy bool
		pct  float64
		want matchKind
		ch   string
		word string
	}{
		{true, 50, matchLit, "✦", "busy"},
		{false, 50, matchUnlit, "╿", "idle"},
		{true, 90, matchBurnt, "│", "busy"},
		{false, 90, matchBurnt, "│", "idle"},
	} {
		s := sample.Session{Pane: "%9", Target: "lab:1.9", Name: "api", Busy: tc.busy, Context: "known", ContextPct: tc.pct, Tokens: int(tc.pct * 2000)}
		if !tc.busy {
			s.Status = "idle"
		}
		if got := m.matchOf(s); got != tc.want {
			t.Errorf("busy=%v %v%%: %v, want %v", tc.busy, tc.pct, got, tc.want)
		}
		plain := ansi.Strip(strings.Join(m.sessionRow(s, false, 78), "\n"))
		if !strings.Contains(plain, tc.ch+" ") || !strings.Contains(plain, tc.word) {
			t.Errorf("busy=%v %v%%: row %q lacks %q or the word %q", tc.busy, tc.pct, plain, tc.ch, tc.word)
		}
	}
}

// animated is the console with motion on, past the start screen, on a
// clock the test moves.
func animated(t *testing.T, st proto.State, now *time.Time) Model {
	t.Helper()
	m := New(Options{Conn: newFake(), CompactAt: 85, Now: func() time.Time { return *now }})
	m.splashDone = true
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	next, _ = next.Update(stateMsg(st))
	return next.(Model)
}

// sessionState is one session in pane %7, busy or idle, at 50 % context.
func sessionState(busy bool) proto.State {
	st := fixtureState()
	s := sample.Session{PID: 77, Pane: "%7", Target: "lab:1.7", Name: "api", Busy: busy, Context: "known", ContextPct: 50}
	if !busy {
		s.Status = "idle"
	}
	st.Sessions, st.Queue = []sample.Session{s}, nil
	return st
}

type seen struct {
	ch string
	c  string
}

func glyphAt(m Model, pane string) seen {
	for _, s := range m.snap.Sessions {
		if s.Pane == pane {
			ch, c := m.matchFor(s)
			return seen{ch, fmt.Sprint(c)}
		}
	}
	return seen{}
}

func TestBurningFlickers(t *testing.T) {
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	m := animated(t, sessionState(true), &now)
	now = now.Add(2 * time.Second) // past any strike
	chars, colours := map[string]bool{}, map[string]bool{}
	for range 16 {
		g := glyphAt(m, "%7")
		chars[g.ch], colours[g.c] = true, true
		now = now.Add(200 * time.Millisecond)
	}
	if !chars["✦"] || !chars["✧"] || len(colours) < 3 {
		t.Fatalf("a burning match must flicker: chars %v, %d colours", chars, len(colours))
	}
}

func TestAsksBreathes(t *testing.T) {
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	st := sessionState(false)
	st.Queue = []queue.Item{{SessionID: "a", Pane: "%7", State: queue.StatePermission, Since: now}}
	m := animated(t, st, &now)
	asks := map[string]bool{}
	for range 24 {
		g := glyphAt(m, "%7")
		if g.ch != "╿" {
			t.Fatalf("asks shows %q", g.ch)
		}
		asks[g.c] = true
		now = now.Add(100 * time.Millisecond)
	}
	if len(asks) < 3 {
		t.Fatalf("an asking match must breathe: %d colours", len(asks))
	}
	st.Queue[0].State = queue.StateFinished // done: it waits, but it does not ask
	next, _ := m.Update(stateMsg(st))
	idle := map[string]bool{}
	for range 24 {
		idle[glyphAt(next.(Model), "%7").c] = true
		now = now.Add(100 * time.Millisecond)
	}
	if len(idle) != 1 {
		t.Fatalf("an idle match must be still: %d colours", len(idle))
	}
}

func TestStrikeWhenWorkStarts(t *testing.T) {
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	m := animated(t, sessionState(false), &now)
	next, cmd := m.Update(stateMsg(sessionState(true)))
	m = next.(Model)
	if cmd == nil {
		t.Fatal("no tick for the strike")
	}
	for _, step := range []struct {
		after time.Duration
		ch    string
	}{{0, "╿"}, {300 * time.Millisecond, "·"}, {600 * time.Millisecond, "✧"}} {
		at := now
		now = now.Add(step.after)
		if g := glyphAt(m, "%7"); g.ch != step.ch {
			t.Fatalf("strike at +%v: %q, want %q", step.after, g.ch, step.ch)
		}
		now = at
	}
	now = now.Add(900 * time.Millisecond)
	if g := glyphAt(m, "%7"); g.ch != "✦" && g.ch != "✧" {
		t.Fatalf("after the strike it burns: %q", g.ch)
	}
}

func TestFlameGoesOut(t *testing.T) {
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	m := animated(t, sessionState(true), &now)
	now = now.Add(2 * time.Second)
	next, _ := m.Update(stateMsg(sessionState(false)))
	m = next.(Model)
	now = now.Add(100 * time.Millisecond)
	if g := glyphAt(m, "%7"); g.ch != "✧" {
		t.Fatalf("the flame dies down first: %q", g.ch)
	}
	now = now.Add(500 * time.Millisecond)
	if g := glyphAt(m, "%7"); g.ch != "╿" {
		t.Fatalf("then the match rests: %q", g.ch)
	}
}

func TestNoMotionIsStill(t *testing.T) {
	m, _ := loadedWith(t, 100, sessionState(false))
	next, cmd := m.Update(stateMsg(sessionState(true)))
	if cmd == nil {
		t.Fatal("the receive loop must go on")
	}
	nm := next.(Model)
	if nm.animating {
		t.Fatal("no motion, yet the console ticks")
	}
	if g := glyphAt(nm, "%7"); g.ch != "✦" {
		t.Fatalf("no motion shows the burning match at once: %q", g.ch)
	}
}

// The console wakes only while something moves: fire, a session that asks,
// or a change that is still playing.
func TestTicksOnlyWhileSomethingMoves(t *testing.T) {
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	m := animated(t, sessionState(true), &now)
	if !m.animating {
		t.Fatal("a burning match must tick")
	}
	quiet := sessionState(false)
	next, _ := m.Update(stateMsg(quiet))
	now = now.Add(5 * time.Second) // the flame has gone out
	next, cmd := next.Update(animMsg{})
	if cmd != nil || next.(Model).animating {
		t.Fatal("nothing moves, yet the console keeps ticking")
	}
}

// TestAWaitingSessionsMatch: a session that waits is unlit, or burnt when
// its context is full.
func TestAWaitingSessionsMatch(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	m.opt.CompactAt = 85
	s, _ := m.selectedSession()
	if m.groupOf(s) != groupWaits {
		t.Fatalf("%s does not wait", s.Name)
	}
	s.Context, s.ContextPct = "known", 50
	if got := m.matchOf(s); got != matchUnlit {
		t.Fatalf("a waiting session is unlit, got %v", got)
	}
	s.ContextPct = 90
	if got := m.matchOf(s); got != matchBurnt {
		t.Fatalf("a full waiting session is burnt, got %v", got)
	}
}

func TestPanesTabListsEveryPane(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	next, _ := key(m, "6")
	out := ansi.Strip(next.(Model).render())
	for _, want := range []string{"work:1.1", "notes:1.1", "zsh", "/work/notes"} {
		if !strings.Contains(out, want) {
			t.Fatalf("panes tab lacks %q:\n%s", want, out)
		}
	}
}

// TestAnAgentWithoutHooksIsReadFromThePane: a Codex session waits with the
// question from its pane, says where that came from, shows "not measured"
// for context use, and takes an answer like any other.
func TestAnAgentWithoutHooksIsReadFromThePane(t *testing.T) {
	st := fixtureState()
	q := "Would you like to run the following command?"
	st.Sessions = append(st.Sessions, sample.Session{PID: 900, Pane: "%9", Target: "api:1.1", Tab: "api", Name: "codex",
		Status: "idle", Idle: time.Minute, Context: "unmeasured", FromPane: true, Permits: true, LastLine: q, Procs: 1})
	st.Queue = []queue.Item{{Pane: "%9", Target: "api:1.1", Name: "codex", State: queue.StatePermission,
		Since: st.At.Add(-time.Minute), LastLine: q, Estimated: true, FromPane: true}}
	for _, w := range []int{132, 50} {
		m, _ := loadedWith(t, w, st)
		out := ansi.Strip(m.render())
		for _, want := range []string{"codex", "asks", "Would you like to run"} {
			if !strings.Contains(out, want) {
				t.Fatalf("width %d: the session lacks %q:\n%s", w, want, out)
			}
		}
	}
	// An approval menu is answered in its pane: Enter there takes the
	// selected choice, so the console never types a line into it.
	m, f := loadedWith(t, 100, st)
	m.selPID, m.chose = 900, true
	next, _ := key(m, "a")
	next = typeText(next, "y")
	if out := ansi.Strip(next.(Model).render()); !strings.Contains(out, "⏎ open") || strings.Contains(out, "› y") || len(f.acts()) != 0 {
		t.Fatalf("a on an approval menu: acts %+v\n%s", f.acts(), out)
	}
	m.tab, m.selPID, m.width = tabSessions, 900, 132
	out := ansi.Strip(m.render())
	if strings.Contains(out, "1 processes") {
		t.Fatalf("one process in the plural:\n%s", out)
	}
	for _, want := range []string{"not measured", q, "no hooks for codex: busy and idle are", "1 process"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Sessions lacks %q:\n%s", want, out)
		}
	}
}

// TestSessionsSayWhatTheQueueSays: a session that waits shows the queue's
// word in Sessions, not idle; a turn that ended under 100% progress, with
// no question, is paused on both tabs.
func TestSessionsSayWhatTheQueueSays(t *testing.T) {
	st := fixtureState()
	st.Sessions = append(st.Sessions,
		sample.Session{PID: 901, Pane: "%8", Target: "app:1.1", Name: "slice-one", Status: "idle", Idle: time.Minute, Context: "fresh",
			Progress: &sample.Progress{Pct: 60, Step: "wiring"}},
		sample.Session{PID: 902, Pane: "%9", Target: "app:2.1", Name: "slice-two", Status: "idle", Idle: time.Minute, Context: "fresh"})
	st.Queue = []queue.Item{
		{Pane: "%8", Target: "app:1.1", Name: "slice-one", State: queue.StateFinished, Since: st.At.Add(-time.Minute)},
		{Pane: "%9", Target: "app:2.1", Name: "slice-two", State: queue.StateQuestion, Since: st.At.Add(-time.Minute), LastLine: "Shall I open the PR?"},
	}
	m, _ := loadedWith(t, 100, st)
	rowOf := func(out, name string) string {
		for _, l := range strings.Split(out, "\n") {
			if l = listOf(l, m.width); strings.Contains(l, name) {
				return l
			}
		}
		t.Fatalf("no row for %s:\n%s", name, out)
		return ""
	}
	q := ansi.Strip(m.render())
	if r := rowOf(q, "slice-one"); !strings.Contains(r, "paused") {
		t.Fatalf("queue row %q", r)
	}
	m.tab = tabSessions
	for _, w := range []int{100, 50} {
		m.width = w
		out := ansi.Strip(m.render())
		for name, word := range map[string]string{"slice-one": "paused", "slice-two": "waits"} {
			if r := rowOf(out, name); !strings.Contains(r, word) || strings.Contains(r, "idle") {
				t.Fatalf("width %d: %s row %q, want %q", w, name, r, word)
			}
		}
	}
}

// TestAJumpMovesTheCursorToTheNextThatWaits: after Enter goes to an agent,
// the console selects the next one that waits, so the way back lands on it.
func TestAJumpMovesTheCursorToTheNextThatWaits(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	m.opt.Run = func([]string) error { return nil }
	next, cmd := key(m, "enter")
	if it, _ := m.selectedSession(); it.Pane != "%1" {
		t.Fatalf("starts on %s", it.Pane)
	}
	if cmd == nil {
		next, cmd = key(next, "enter")
	}
	next, _ = next.Update(cmd())
	if it, _ := next.(Model).selectedSession(); it.Pane != "%2" {
		t.Fatalf("after the jump to %%1 the queue selects %s, want %%2", it.Pane)
	}

	st := queueState()
	m, _ = loadedWith(t, 100, st)
	m.opt.Run = func([]string) error { return nil }
	m.tab = tabSessions
	from, _ := m.selected()
	armed, cmd := key(m, "enter")
	if cmd == nil {
		armed, cmd = key(armed, "enter")
	}
	if cmd == nil {
		t.Fatal("no jump")
	}
	nm, _ := armed.Update(cmd())
	s, _ := nm.(Model).selected()
	if s.PID == from.PID || s.Busy {
		t.Fatalf("after the jump to %s Sessions selects %s (busy %v)", from.Name, s.Name, s.Busy)
	}
	waits := false
	for _, it := range nm.(Model).queue {
		waits = waits || it.Pane == s.Pane
	}
	if !waits {
		t.Fatalf("Sessions selects %s, which does not wait", s.Name)
	}
}

// TestTwoSessionsOutsideTmuxKeepTheirOwnWords: a session with no pane does
// not take the queue word of another session with no pane.
func TestTwoSessionsOutsideTmuxKeepTheirOwnWords(t *testing.T) {
	st := fixtureState()
	st.Sessions = append(st.Sessions, sample.Session{PID: 960, Name: "loose", Status: "idle", Idle: time.Minute, Context: "fresh"})
	st.Queue = []queue.Item{{SessionID: "x", Name: "other", State: queue.StateQuestion, Since: st.At}}
	m, _ := loadedWith(t, 100, st)
	for _, s := range m.snap.Sessions {
		if s.PID == 960 && m.stateWord(s) != "idle" {
			t.Fatalf("a session with no pane took %q", m.stateWord(s))
		}
	}
}

// TestAnEstimatedRowShowsWhatTheAgentAsked: a session without hooks that
// waits on a question shows the question, so `a` answers what it reads.
func TestAnEstimatedRowShowsWhatTheAgentAsked(t *testing.T) {
	st := queueState()
	q := "Should a refresh also rotate the token?"
	st.Queue[1].State, st.Queue[1].LastLine = queue.StateQuestion, q
	for _, w := range []int{132, 44} {
		m, _ := loadedWith(t, w, st)
		next := selectPane(t, m, "%2")
		if out := strings.Join(strings.Fields(ansi.Strip(next.(Model).render())), " "); !strings.Contains(out, q) {
			t.Fatalf("width %d: the session lacks the question:\n%s", w, ansi.Strip(next.(Model).render()))
		}
	}
}

// A finished turn of a session the plan's limit stops says out on its
// queue row: there is nothing to answer before the reset.
func TestAQueueRowOfAnOutSessionSaysOut(t *testing.T) {
	st := fixtureState()
	st.Sessions = append(st.Sessions, sample.Session{PID: 901, Pane: "%8", Target: "app:1.1", Name: "slice-x",
		Status: "idle", Idle: time.Minute, Context: "fresh", Out: st.At.Add(time.Hour)})
	st.Queue = []queue.Item{{Pane: "%8", Target: "app:1.1", Name: "slice-x", State: queue.StateFinished, Since: st.At.Add(-time.Minute)}}
	m, _ := loadedWith(t, 100, st)
	for _, l := range strings.Split(ansi.Strip(m.render()), "\n") {
		if strings.Contains(l, "slice-x") {
			if !regexp.MustCompile(`\bout\b`).MatchString(l) || strings.Contains(l, "done") {
				t.Fatalf("queue row %q", l)
			}
			return
		}
	}
	t.Fatal("no queue row")
}

// A tap on an option of the peek picks it, even while the typing line has
// the cursor: it never types the digit.
func TestATapOnAnOptionPicksIt(t *testing.T) {
	st := screensState(t)
	m, f := loadedWith(t, 44, st)
	for _, s := range m.snap.Sessions {
		if s.Name == "flint-auth" {
			m.chose, m.selPID = true, s.PID
		}
	}
	next, _ := key(m, "a")
	next = typeText(next, "x")
	out := strings.Split(ansi.Strip(next.(Model).render()), "\n")
	y := -1
	for i, l := range out {
		if strings.Contains(l, "2 Rotate, keep") {
			y = i
		}
	}
	if y < 0 {
		t.Fatalf("no option 2:\n%s", strings.Join(out, "\n"))
	}
	next, _ = tap(next, 10, y)
	if p := next.(Model).peek; p == nil || p.q != 1 || len(p.choices) != 1 || p.choices[0].Picks[0] != 2 || len(f.acts()) != 0 {
		t.Fatalf("peek %+v", next.(Model).peek)
	}
}
