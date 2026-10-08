package app

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/sample"
)

func queueState() proto.State {
	st := fixtureState()
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

func TestQueueIsDefaultTab(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	out := ansi.Strip(m.render())
	for _, want := range []string{"1 QUEUE", "2 WAITING FOR YOU", "asks", "app-feature", "Claude needs your permission to use Bash", "estimated", "7 PANES"} {
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

func TestAnswerNeedsConfirm(t *testing.T) {
	m, f := loadedWith(t, 100, queueState())
	m, _ = func() (Model, tea.Cmd) { n, c := key(m, "down"); return n.(Model), c }() // app-review finished its turn
	next, _ := key(m, "a")
	next = typeText(next, "yes, open it")
	out := ansi.Strip(next.(Model).render())
	if !strings.Contains(out, "yes, open it") {
		t.Fatalf("the input is not shown:\n%s", out)
	}
	next, _ = key(next, "enter")
	p := next.(Model).pending
	if p == nil || !p.destructive || p.rec != "answer:%2" || p.text != "yes, open it" {
		t.Fatalf("pending %+v", p)
	}
	if _, cmd := key(next, "enter"); cmd != nil || len(f.acts()) != 0 {
		t.Fatal("enter alone sent the answer")
	}
	next, _ = key(m, "a")
	next = typeText(next, "yes")
	next, _ = key(next, "enter")
	_, cmd := key(next, "y")
	if cmd == nil {
		t.Fatal("no command")
	}
	cmd()
	acts := f.acts()
	if len(acts) != 1 || acts[0] != (proto.Act{RecID: "answer:%2", Which: "secondary", Confirm: "y", Text: "yes"}) {
		t.Fatalf("acts %+v", acts)
	}
}

// Review 1: Enter at a permission prompt approves it; a typed answer never
// goes there. The console sends the person to the pane instead.
func TestAnswerOnPermissionGoesToPane(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState()) // app-feature asks for permission
	next, _ := key(m, "a")
	nm := next.(Model)
	if nm.input != nil || !strings.Contains(nm.flash, "Enter goes there") {
		t.Fatalf("input %+v flash %q", nm.input, nm.flash)
	}
}

// The Sessions tab answers only a session that waits for an answer.
func TestAnswerFromSessionsNeedsAWait(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	m.tab = tabSessions // the first session, app-feature, is in the queue at a permission prompt
	if next, _ := key(m, "a"); next.(Model).input != nil {
		t.Fatal("a opened an answer for a permission prompt")
	}
}

// Review 7: at 80 columns all seven tabs and the alert marker fit.
func TestTabsFit80WithAlert(t *testing.T) {
	st := queueState()
	st.Alerts = []sample.Alert{{Key: "load", Level: "warn", Title: "load"}}
	m, _ := loadedWith(t, 80, st)
	line := ansi.Strip(m.tabs(80))
	if !strings.Contains(line, "7 ") || !strings.Contains(line, "▲ 1 alert") || strings.Contains(line, "…") {
		t.Fatalf("tab line %q", line)
	}
}

func TestAnswerEscCancels(t *testing.T) {
	m0, f := loadedWith(t, 100, queueState())
	down, _ := key(m0, "down") // app-review finished its turn
	m := down.(Model)
	next, _ := key(m, "a")
	next = typeText(next, "no")
	next, _ = key(next, "esc")
	if next.(Model).input != nil || next.(Model).pending != nil || len(f.acts()) != 0 {
		t.Fatal("esc did not cancel")
	}
	// q inside the input is text, not quit.
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
		s := sample.Session{Pane: "%9", Target: "lab:1.9", Name: "api", Busy: tc.busy, Context: "known", ContextPct: tc.pct}
		if !tc.busy {
			s.Status = "idle"
		}
		if got := m.matchOf(s); got != tc.want {
			t.Errorf("busy=%v %v%%: %v, want %v", tc.busy, tc.pct, got, tc.want)
		}
		plain := ansi.Strip(m.row(s, false, 120, 0, 20))
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

func TestQueueRowMatch(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	m.opt.CompactAt = 85
	it := queueState().Queue[0]
	m.snap.Sessions[0].Context, m.snap.Sessions[0].ContextPct = "known", 50
	if got := m.queueMatch(it); got != matchUnlit {
		t.Fatalf("a waiting session is unlit, got %v", got)
	}
	m.snap.Sessions[0].Context, m.snap.Sessions[0].ContextPct = "known", 90
	if got := m.queueMatch(it); got != matchBurnt {
		t.Fatalf("a full waiting session is burnt, got %v", got)
	}
}

func TestPanesTabListsEveryPane(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	next, _ := key(m, "7")
	out := ansi.Strip(next.(Model).render())
	for _, want := range []string{"work:1.1", "notes:1.1", "zsh", "/work/notes"} {
		if !strings.Contains(out, want) {
			t.Fatalf("panes tab lacks %q:\n%s", want, out)
		}
	}
}

// The person confirms the steps the service runs.
func TestAnswerPreviewIsWhatRuns(t *testing.T) {
	if got, want := answerSteps("%3", "-y; ls"), panes.Send("%3", "-y; ls"); !reflect.DeepEqual(got, want) {
		t.Fatalf("console %q, service %q", got, want)
	}
}
