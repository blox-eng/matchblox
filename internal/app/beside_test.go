package app

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/sample"
)

// besideTmux is a fake tmux that records each command and answers the
// window's width and the new pane's id.
type besideTmux struct {
	winW  string
	kept  string // the window's @matchblox_beside, "" when unset
	alive bool   // the pane kept answers
	calls [][]string
}

func (b *besideTmux) run(args ...string) (string, error) {
	b.calls = append(b.calls, args)
	switch args[0] {
	case "show-option":
		return b.kept + "\n", nil
	case "display":
		if args[len(args)-1] == "#{pane_id}" {
			if !b.alive {
				return "", errors.New("can't find pane: " + args[3])
			}
			return args[3] + "\n", nil
		}
		return b.winW + "\n", nil
	case "split-window":
		return "%9\n", nil
	}
	return "", nil
}

func (b *besideTmux) lines() []string {
	var out []string
	for _, c := range b.calls {
		out = append(out, strings.Join(c, " "))
	}
	return out
}

func (b *besideTmux) ran(argv []string) bool {
	return slices.Contains(b.lines(), strings.Join(argv[1:], " "))
}

func (b *besideTmux) count(prefix string) (n int) {
	for _, l := range b.lines() {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	return n
}

// besideConsole is the console at w by h in a window of winW columns, its
// split done.
func besideConsole(t *testing.T, st proto.State, w, h int, winW string) (Model, *besideTmux) {
	t.Helper()
	return besideConsoleWith(t, st, w, h, &besideTmux{winW: winW})
}

func besideConsoleWith(t *testing.T, st proto.State, w, h int, tm *besideTmux) (Model, *besideTmux) {
	t.Helper()
	opt := Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }, Beside: true, Pane: "%1", Tmux: tm.run}
	var next tea.Model = New(opt)
	next, _ = next.Update(tea.WindowSizeMsg{Width: w, Height: h})
	next, _ = next.Update(stateMsg(st))
	m := next.(Model)
	m.splashDone = true
	m.viewPrefs.CopyCheckedPhone, m.viewPrefs.CopyCheckedDesk = true, true
	m = feed(t, m, m.besideCmd())
	return m, tm
}

// feed runs cmd and gives its message, and what follows, to the console.
func feed(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if _, batch := msg.(tea.BatchMsg); batch {
		return m // timers and the like: nothing for tmux
	}
	next, cmd := m.Update(msg)
	return feed(t, next.(Model), cmd)
}

func TestTheConsoleSplitsItsWindowOnceAtStart(t *testing.T) {
	m, tm := besideConsole(t, boardFixture(t), 40, 50, "160")
	if n := tm.count("split-window -h -d -t %1"); n != 1 {
		t.Fatalf("%d splits: %q", n, tm.lines())
	}
	joined := strings.Join(tm.lines(), "\n")
	for _, want := range []string{"set-option -w -t %1 @matchblox_console %1", "set-option -w -t %1 @matchblox_beside %9", "pane-active-border-style fg=#c3a56a"} {
		if !strings.Contains(joined, want) {
			t.Errorf("no %q in %q", want, joined)
		}
	}
	if m.beside != "%9" {
		t.Fatalf("beside = %q", m.beside)
	}
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 40, Height: 50})
	feed(t, next.(Model), cmd)
	if n := tm.count("split-window"); n != 1 {
		t.Fatalf("a second size made %d splits", n)
	}
}

func TestABesideConsoleDrawsOnlyTheList(t *testing.T) {
	m, _ := besideConsole(t, boardFixture(t), 40, 50, "160")
	frame := plainFrame(m)
	if !strings.Contains(frame, "kindle-web") {
		t.Fatalf("no list rows in\n%s", frame)
	}
	for _, not := range []string{"send to", "ctrl+o expand"} {
		if strings.Contains(frame, not) {
			t.Fatalf("%q drawn in\n%s", not, frame)
		}
	}
}

func TestTheZoomFollowsTheWindowWidth(t *testing.T) {
	for _, c := range []struct {
		winW string
		zoom bool
	}{{"160", false}, {"70", true}} {
		m, tm := besideConsole(t, boardFixture(t), 40, 50, c.winW)
		if !tm.ran(remote.ZoomArgv("%1", c.zoom)) {
			t.Errorf("window %s: no zoom=%v in %q", c.winW, c.zoom, tm.lines())
		}
		if tm.ran(remote.ZoomArgv("%1", !c.zoom)) {
			t.Errorf("window %s: zoom=%v too", c.winW, !c.zoom)
		}
		if c.zoom && tm.count("resize-pane -t") != 0 {
			t.Errorf("window %s: a resize would end the zoom: %q", c.winW, tm.lines())
		}
		if got := m.onePane(40); got != c.zoom {
			t.Errorf("window %s: onePane = %v", c.winW, got)
		}
	}
}

func TestTheListKeepsItsWidth(t *testing.T) {
	for _, c := range []struct {
		winW, height string
		w            int
	}{{"200", "50", 40}, {"110", "60", 32}} {
		h := 50
		if c.height == "60" {
			h = 60
		}
		_, tm := besideConsole(t, boardFixture(t), 40, h, c.winW)
		if want := remote.ListWidthArgv("%1", c.w); !tm.ran(want) {
			t.Errorf("window %s: no %v in %q", c.winW, want, tm.lines())
		}
	}
}

func TestTheDemoKeepsTheDrawnChat(t *testing.T) {
	tm := &besideTmux{}
	opt := Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }, Pane: "%1", Tmux: tm.run}
	var next tea.Model = New(opt)
	next, _ = next.Update(tea.WindowSizeMsg{Width: 160, Height: 38})
	next, _ = next.Update(stateMsg(boardFixture(t)))
	m := next.(Model)
	m.splashDone = true
	if m.besideCmd() != nil {
		t.Fatal("a console without Beside runs tmux")
	}
	if !strings.Contains(plainFrame(m), "│") {
		t.Fatalf("no chat place in\n%s", plainFrame(m))
	}
	if len(tm.calls) != 0 {
		t.Fatalf("tmux calls: %q", tm.lines())
	}
}

func TestAFailedSplitKeepsTheDrawnChat(t *testing.T) {
	tm := &besideTmux{winW: "160"}
	opt := Options{NoMotion: true, Conn: newFake(), Beside: true, Pane: "%1", Tmux: func(args ...string) (string, error) {
		if args[0] == "split-window" {
			return "", errors.New("no server")
		}
		return tm.run(args...)
	}}
	m := New(opt)
	m = feed(t, m, m.besideCmd())
	if m.beside != "" || !strings.Contains(m.flash, "the agent pane did not open") {
		t.Fatalf("beside %q, flash %q", m.beside, m.flash)
	}
}

func TestARestartedConsoleAdoptsItsAgentPane(t *testing.T) {
	m, tm := besideConsoleWith(t, boardFixture(t), 40, 50, &besideTmux{winW: "160", kept: "%7", alive: true})
	if n := tm.count("split-window"); n != 0 {
		t.Fatalf("a restart split again: %q", tm.lines())
	}
	if m.beside != "%7" {
		t.Fatalf("beside = %q", m.beside)
	}
	if got := strings.Join(tm.lines(), "\n"); !strings.Contains(got, "set-option -w -t %1 "+sample.ConsoleOption+" %1") {
		t.Fatalf("the console pane is not current in %q", got)
	}
	if !strings.Contains(strings.Join(tm.lines(), "\n"), "show-option -wqv -t %1 "+sample.BesideOption) {
		t.Fatalf("the option was not read: %q", tm.lines())
	}
}

func TestADeadAgentPaneIsMadeAgain(t *testing.T) {
	m, tm := besideConsoleWith(t, boardFixture(t), 40, 50, &besideTmux{winW: "160", kept: "%7"})
	if n := tm.count("split-window"); n != 1 {
		t.Fatalf("%d splits: %q", n, tm.lines())
	}
	if m.beside != "%9" {
		t.Fatalf("beside = %q", m.beside)
	}
}

// shownAt is a beside console whose list chooses name, its agent pane alive.
func shownAt(t *testing.T, name, winW string) (Model, *besideTmux) {
	t.Helper()
	m, tm := besideConsoleWith(t, boardFixture(t), 40, 50, &besideTmux{winW: winW, alive: true})
	m.opt.Socket = "/tmp/tmux-1000/default"
	return chooseNamed(t, m, name), tm
}

// chooseNamed makes the list choose the session called name.
func chooseNamed(t *testing.T, m Model, name string) Model {
	t.Helper()
	i := slices.IndexFunc(m.sideSessions(), func(s sample.Session) bool { return s.Name == name })
	if i < 0 {
		t.Fatalf("no %s in the list", name)
	}
	m.sideAt = paneOf(m.sideSessions()[i])
	return m
}

// enter presses ⏎ and runs what follows.
func enter(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := key(m, "enter")
	return afterChoice(next, cmd)
}

// afterChoice runs cmd, and gives the console the end of a choice.
func afterChoice(next tea.Model, cmd tea.Cmd) Model {
	m := next.(Model)
	for _, msg := range cmdMsgs(cmd) {
		if msg, ok := msg.(shownMsg); ok {
			next, _ := m.Update(msg)
			m = next.(Model)
		}
	}
	return m
}

func respawned(tm *besideTmux) string {
	for _, l := range tm.lines() {
		if strings.HasPrefix(l, "respawn-pane") {
			return l
		}
	}
	return ""
}

func TestEnterShowsTheChosenAgentBeside(t *testing.T) {
	m, tm := shownAt(t, "kindle-web", "160")
	m = enter(t, m)
	r := respawned(tm)
	if !strings.HasPrefix(r, "respawn-pane -k -t %9 -- /bin/sh") || !strings.Contains(r, "matchblox-jump-") || !strings.Contains(r, "kindle-web") {
		t.Fatalf("respawn %q in %q (flash %q)", r, tm.lines(), m.flash)
	}
	lines := tm.lines()
	at := slices.Index(lines, r)
	if sel := slices.Index(lines, "select-pane -Z -t %9"); sel < at {
		t.Fatalf("the agent pane is not selected after the respawn: %q", lines)
	}
	if tm.count("split-window") != 1 {
		t.Fatalf("a live agent pane was made again: %q", lines)
	}
}

func TestArrowsDoNotRespawn(t *testing.T) {
	m, tm := shownAt(t, "kindle-web", "160")
	next, cmd := key(m, "down")
	next, cmd2 := key(afterChoice(next, cmd), "down")
	afterChoice(next, cmd2)
	if r := respawned(tm); r != "" {
		t.Fatalf("an arrow respawned: %q", r)
	}
}

func TestAPeerAgentAsksBeforeItShows(t *testing.T) {
	home, ws := newFake(), newFake()
	tm := &besideTmux{winW: "160", alive: true}
	opt := Options{NoMotion: true, Conn: home, Now: func() time.Time { return demoAt }, Beside: true, Pane: "%1", Tmux: tm.run,
		Peers: []Peer{{Host: "ws-1", Conn: ws}}}
	var next tea.Model = New(opt)
	next, _ = next.Update(tea.WindowSizeMsg{Width: 40, Height: 50})
	next, _ = next.Update(stateMsg(proto.State{}))
	next, _ = next.Update(peerMsg{host: "ws-1", conn: ws, msg: stateMsg(boardFixture(t))})
	m := next.(Model)
	m.splashDone = true
	m.viewPrefs.CopyCheckedPhone, m.viewPrefs.CopyCheckedDesk = true, true
	m = feed(t, m, m.besideCmd())
	m = chooseNamed(t, m, "kindle-web")
	if s, _ := m.sideChosen(); s.Host != "ws-1" {
		t.Fatalf("kindle-web is on %q", s.Host)
	}
	next, cmd := key(m, "enter")
	m = afterChoice(next, cmd)
	if m.pending == nil || respawned(tm) != "" {
		t.Fatalf("pending %v, respawn %q: want the consent box first", m.pending != nil, respawned(tm))
	}
	next, cmd = key(m, "y")
	afterChoice(next, cmd)
	r := respawned(tm)
	if !strings.HasPrefix(r, "respawn-pane -k -t %9 -- ssh") || !strings.Contains(r, "ws-1") {
		t.Fatalf("respawn %q in %q", r, tm.lines())
	}
}

func TestAStokerHostAsksToTakeOver(t *testing.T) {
	st := boardFixture(t)
	v := runningNight(demoAt)
	st.Stoker = &v
	m, tm := besideConsoleWith(t, st, 40, 50, &besideTmux{winW: "160", alive: true})
	m.opt.Socket = "/tmp/tmux-1000/default"
	m = chooseNamed(t, m, "kindle-web")
	next, cmd := key(m, "enter")
	m = afterChoice(next, cmd)
	if m.held == nil || respawned(tm) != "" {
		t.Fatalf("held %v, respawn %q: want take over? first", m.held != nil, respawned(tm))
	}
	next, cmd = key(m, "n")
	afterChoice(next, cmd)
	if respawned(tm) == "" {
		t.Fatalf("no respawn after the answer: %q", tm.lines())
	}
}

func TestAChoiceRemakesAMissingAgentPane(t *testing.T) {
	m, tm := besideConsole(t, boardFixture(t), 40, 50, "160") // %9 does not answer
	m.opt.Socket = "/tmp/tmux-1000/default"
	m = enter(t, chooseNamed(t, m, "kindle-web"))
	lines := tm.lines()
	r := respawned(tm)
	at := slices.Index(lines, r)
	split := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "split-window") {
			split = i // the last split
		}
	}
	if r == "" || split < 0 || split > at || tm.count("split-window") != 2 {
		t.Fatalf("want a second split before the respawn: %q", lines)
	}
	if m.beside != "%9" {
		t.Fatalf("beside = %q", m.beside)
	}
}

func TestOnePaneZoomsTheAgent(t *testing.T) {
	m, tm := shownAt(t, "kindle-web", "70")
	enter(t, m)
	if !tm.ran(remote.ZoomArgv("%9", true)) {
		t.Fatalf("no zoom of the agent in %q", tm.lines())
	}
}

func TestALocalViewNeedsTheSocket(t *testing.T) {
	m, tm := shownAt(t, "kindle-web", "160")
	m.opt.Socket = ""
	m = enter(t, m)
	if respawned(tm) != "" || !strings.Contains(m.flash, "socket") {
		t.Fatalf("respawn %q, flash %q", respawned(tm), m.flash)
	}
}

func TestAFailedRespawnSaysWhy(t *testing.T) {
	tm := &besideTmux{winW: "160", alive: true}
	m, _ := besideConsoleWith(t, boardFixture(t), 40, 50, &besideTmux{winW: "160", alive: true})
	m.opt.Tmux = func(args ...string) (string, error) {
		if args[0] == "respawn-pane" {
			return "", errors.New("no space for new pane")
		}
		return tm.run(args...)
	}
	m.opt.Socket = "/tmp/tmux-1000/default"
	m = enter(t, chooseNamed(t, m, "kindle-web"))
	if !strings.Contains(m.flash, "kindle-web") || !strings.Contains(m.flash, "no space for new pane") {
		t.Fatalf("flash %q", m.flash)
	}
}

func TestATapOnARowShowsItsAgent(t *testing.T) {
	m, tm := shownAt(t, "kindle-web", "160")
	next, cmd := tapRow(t, m, "flint-auth")
	afterChoice(next, cmd)
	if r := respawned(tm); r == "" || !strings.Contains(r, "flint-auth") {
		t.Fatalf("respawn %q", r)
	}
}

// tapRow taps the list's row of the session called name.
func tapRow(t *testing.T, m Model, name string) (tea.Model, tea.Cmd) {
	t.Helper()
	want := chooseNamed(t, m, name).sideAt
	box := m.sideBoxAt(max(m.width, minWidth))
	_, hits, _ := m.sideList(box.listW, m.bodyRows())
	for y, h := range hits {
		if h.k == want {
			return m.sideTap(1, y)
		}
	}
	t.Fatalf("no row of %s", name)
	return nil, nil
}
