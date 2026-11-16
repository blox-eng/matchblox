package app

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/sample"
)

// besideTmux is a fake tmux that records each command and answers the
// window's width and the new pane's id.
type besideTmux struct {
	winW  string
	winH  string   // the window's height, "" when tmux does not say
	kept  string   // the window's @matchblox_beside, "" when unset
	alive bool     // the pane kept answers
	own   []string // other panes in the console's window
	using string   // the window's active pane, "" when tmux does not say
	chain string   // the window's @matchblox_chain
	calls [][]string
}

func (b *besideTmux) run(args ...string) (string, error) {
	b.calls = append(b.calls, args)
	switch args[0] {
	case "show-option":
		if args[len(args)-1] == sample.ChainOption {
			return b.chain + "\n", nil
		}
		return b.kept + "\n", nil
	case "respawn-pane":
		return b.chainList(args), nil
	case "display":
		if args[len(args)-1] == "#{window_id}" {
			if args[3] == "%1" || args[3] == "%9" || slices.Contains(b.own, args[3]) {
				return "@1\n", nil
			}
			return "@2\n", nil
		}
		if args[len(args)-1] == "#{pane_id}" {
			if !b.alive {
				return "", errors.New("can't find pane: " + args[3])
			}
			return args[3] + "\n", nil
		}
		if args[len(args)-1] == "#{window_width} #{window_height}" && b.winH != "" {
			return b.winW + " " + b.winH + "\n", nil
		}
		return b.winW + "\n", nil
	case "split-window":
		return "%9\n", nil
	case "list-panes":
		return "\n" + b.using + "\n", nil
	}
	return "", nil
}

// chainList answers a chain view's list: each split's "i %2i", and the
// window's @matchblox_chain.
func (b *besideTmux) chainList(args []string) string {
	if !slices.Contains(args, sample.ChainOption) {
		return ""
	}
	var out, ids []string
	for i, a := range args {
		if a == "-F" && strings.HasSuffix(args[i+1], " #{pane_id}") {
			n := strings.TrimSuffix(args[i+1], " #{pane_id}")
			out, ids = append(out, n+" %2"+n), append(ids, "%2"+n)
		}
	}
	b.chain = strings.Join(append([]string{args[3]}, ids...), " ")
	return strings.Join(out, "\n") + "\n"
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

// TestANarrowWindowKeepsTheAgentInUse: the window narrows while the agent
// has the keys: the agent fills it, and keeps them.
func TestANarrowWindowKeepsTheAgentInUse(t *testing.T) {
	m, tm := besideConsole(t, boardFixture(t), 40, 50, "160")
	tm.using, tm.winW = "%9", "70"
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 40, Height: 50})
	feed(t, next.(Model), cmd)
	if !tm.ran(remote.ZoomArgv("%9", true)) {
		t.Fatalf("the agent in use is not zoomed: %q", tm.lines())
	}
	if tm.ran(remote.ZoomArgv("%1", true)) {
		t.Fatalf("the list took the window from the agent in use: %q", tm.lines())
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
	if !strings.HasPrefix(r, "respawn-pane -k -t %9 -- /bin/sh") || !strings.Contains(r, " ssh ") || !strings.Contains(r, "ws-1") {
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

func TestAPeerViewWaitsWhenItEnds(t *testing.T) {
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
	m = feed(t, m, m.besideCmd())
	next, cmd := key(chooseNamed(t, m, "kindle-web"), "enter")
	next, cmd = key(afterChoice(next, cmd), "y")
	afterChoice(next, cmd)
	if r := respawned(tm); !strings.Contains(r, "-- /bin/sh -c") || !strings.Contains(r, "ctrl+q: back to the list") || !strings.Contains(r, " ssh ") {
		t.Fatalf("the peer's ssh is not wrapped: %q", r)
	}
	if r := respawned(tm); !strings.Contains(r, "session-window-changed") || !strings.Contains(r, sample.ViewOption) {
		t.Fatalf("the peer's view does not end with its agent's pane: %q", r)
	}
}

func TestTypingInTheListSaysTheAgentPaneTakesText(t *testing.T) {
	m, _ := shownAt(t, "kindle-web", "160")
	said := map[string]string{}
	for _, k := range []string{"h", "e", "l", "l", "o"} { // h, e and o are the list's own keys
		next, _ := key(m, k)
		m = next.(Model)
		said[k] = m.sideSays()
	}
	for k, v := range m.lines {
		if v != "" {
			t.Fatalf("typed into the line of %v: %q", k, v)
		}
	}
	if !strings.Contains(said["l"], "the agent pane takes text") || !strings.Contains(said["l"], "⏎ shows kindle-web") {
		t.Fatalf("l says %q", said["l"])
	}
}

func TestTwoQuickChoicesSplitOnceAndShowTheSecond(t *testing.T) {
	m, tm := besideConsole(t, boardFixture(t), 40, 50, "160") // %9 gone
	m.opt.Socket = "/tmp/tmux-1000/default"
	m.opt.Tmux = func(args ...string) (string, error) {
		if args[0] == "split-window" {
			defer func() { tm.alive = true }()
		}
		return tm.run(args...)
	}
	next, first := key(chooseNamed(t, m, "kindle-web"), "enter")
	next, second := key(chooseNamed(t, next.(Model), "flint-auth"), "enter")
	m = next.(Model)
	// The second runs first, as a race would run it.
	for _, cmd := range []tea.Cmd{second, first} {
		for _, msg := range cmdMsgs(cmd) {
			var more tea.Cmd
			next, more = m.Update(msg)
			m = afterChoice(next, more)
		}
	}
	if n := tm.count("split-window"); n != 2 {
		t.Fatalf("%d splits (one at the start): %q", n, tm.lines())
	}
	var shown []string
	for _, l := range tm.lines() {
		if strings.HasPrefix(l, "respawn-pane") {
			shown = append(shown, l)
		}
	}
	if len(shown) == 0 || !strings.Contains(shown[len(shown)-1], "flint-auth") {
		t.Fatalf("respawns %q: the last is not flint-auth", shown)
	}
	if m.showing {
		t.Fatal("a choice still runs")
	}
}

func TestAPaneOfTheConsolesWindowIsNotShown(t *testing.T) {
	m, tm := shownAt(t, "kindle-web", "160")
	s, _ := m.sideChosen()
	tm.own = []string{s.Pane}
	m = enter(t, m)
	if r := respawned(tm); r != "" {
		t.Fatalf("respawned %q", r)
	}
	if !strings.Contains(m.flash, "console's own window") || !strings.Contains(m.flash, "ctrl+o") {
		t.Fatalf("flash %q", m.flash)
	}
}

// TestTheShellKeepsTheAgentPane: the console the shell opens for this
// machine splits its window and shows views on the console's tmux socket,
// as main asked; a host's console keeps the drawn chat.
func TestTheShellKeepsTheAgentPane(t *testing.T) {
	sh := NewShell(ShellOptions{
		Base: Options{NoMotion: true, Beside: true, Socket: "/tmp/tmux-1000/default"},
		Open: func(host string) Options { return Options{Conn: newFake(), Host: host} },
	})
	here, _ := sh.open("", false)
	if o := here.cur.opt; !o.Beside || o.Socket != "/tmp/tmux-1000/default" {
		t.Fatalf("this machine's console lost the agent pane: Beside %v, Socket %q", o.Beside, o.Socket)
	}
	there, _ := sh.open("ws-1", false)
	if there.cur.opt.Beside {
		t.Fatal("a host's console splits the window")
	}
}

// chainOf is the board's fixture with the chain login made of names, each
// in a pane of its own outside the console's window: %40, %41 and on.
func chainOf(t *testing.T, names ...string) proto.State {
	t.Helper()
	st := boardFixture(t)
	var ms []chains.Member
	for n, name := range names {
		i := slices.IndexFunc(st.Sessions, func(s sample.Session) bool { return s.Name == name })
		if i < 0 {
			t.Fatalf("no %s in the fixture", name)
		}
		st.Sessions[i].Pane = "%4" + strconv.Itoa(n)
		ms = append(ms, memberOf(st.Sessions[i]))
	}
	st.Chains.Chains[0].Members = ms
	return st
}

// chainShown is a beside console that pressed g on kindle-web, a member of
// st's chain login.
func chainShown(t *testing.T, st proto.State) (Model, *besideTmux) {
	t.Helper()
	m, tm := besideConsoleWith(t, st, 40, 50, &besideTmux{winW: "160", alive: true})
	m.opt.Socket = "/tmp/tmux-1000/default"
	next, cmd := key(chooseNamed(t, m, "kindle-web"), "g")
	return afterChoice(next, cmd), tm
}

var loginNames = []string{"kindle-web", "flint-auth", "hearth-api"}

func TestGOpensTheChainBeside(t *testing.T) {
	m, tm := chainShown(t, chainOf(t, loginNames...))
	r := respawned(tm)
	if !strings.HasPrefix(r, "respawn-pane -k -t %9 -- ") || !strings.Contains(r, "set-option -w -t %9 @matchblox_chain %9") {
		t.Fatalf("respawn %q in %q (flash %q)", r, tm.lines(), m.flash)
	}
	for _, n := range loginNames {
		if !strings.Contains(r, n) {
			t.Errorf("no view of %s in %q", n, r)
		}
	}
	if n := strings.Count(strings.Join(tm.lines(), "\n"), "split-window"); n != 3 {
		t.Fatalf("%d splits (one at the start): %q", n, tm.lines())
	}
	if n := strings.Count(plainFrame(m), "▸"); n != 3 {
		t.Fatalf("%d members marked on screen:\n%s", n, plainFrame(m))
	}
}

func TestAChainViewShowsAtMostFour(t *testing.T) {
	six := []string{"kindle-web", "flint-auth", "hearth-api", "spark-search", "hearth-limits", "flint-api"}
	m, tm := chainShown(t, chainOf(t, six...))
	var drawn []string
	for _, k := range m.sideOrder() {
		for _, s := range m.sideSessions() {
			if paneOf(s) == k && slices.Contains(six, s.Name) && !slices.Contains(drawn, s.Name) {
				drawn = append(drawn, s.Name)
			}
		}
	}
	r := respawned(tm)
	for i, n := range drawn {
		if on := strings.Contains(r, "sh "+n+" "); on != (i < 4) {
			t.Errorf("%s (drawn %d) on screen: %v in %q", n, i+1, on, r)
		}
	}
	if n := strings.Count(r, "split-window"); n != 3 {
		t.Fatalf("%d splits for 4: %q", n, r)
	}
}

func TestAChainViewSkipsAnEndedMember(t *testing.T) {
	st := chainOf(t, loginNames...)
	for i := range st.Sessions {
		if st.Sessions[i].Name == "hearth-api" {
			st.Sessions[i].Pane = ""
		}
	}
	_, tm := chainShown(t, st)
	r := respawned(tm)
	if strings.Contains(r, "hearth-api") || strings.Count(r, "split-window") != 1 {
		t.Fatalf("respawn %q: want kindle-web and flint-auth, one split", r)
	}
}

func TestAChainViewSkipsAMemberInTheConsolesWindow(t *testing.T) {
	m, tm := besideConsoleWith(t, chainOf(t, loginNames...), 40, 50, &besideTmux{winW: "160", alive: true, own: []string{"%41"}})
	m.opt.Socket = "/tmp/tmux-1000/default"
	next, cmd := key(chooseNamed(t, m, "kindle-web"), "g")
	m = afterChoice(next, cmd)
	if r := respawned(tm); strings.Contains(r, "flint-auth") || strings.Count(r, "split-window") != 1 {
		t.Fatalf("respawn %q: want flint-auth left out", r)
	}
	if !strings.Contains(m.flash, "console's own window: flint-auth") {
		t.Fatalf("flash %q", m.flash)
	}
}

func TestGAgainClosesTheChainView(t *testing.T) {
	m, tm := chainShown(t, chainOf(t, loginNames...))
	next, cmd := key(m, "g")
	m = afterChoice(next, cmd)
	lines := strings.Join(tm.lines(), "\n")
	for _, want := range []string{"kill-pane -t %21", "kill-pane -t %22", "set-option -wu -t %1 @matchblox_chain"} {
		if !strings.Contains(lines, want) {
			t.Errorf("no %q in %q", want, lines)
		}
	}
	if strings.Contains(lines, "kill-pane -t %9") {
		t.Errorf("the agent pane was killed: %q", lines)
	}
	if strings.Contains(plainFrame(m), "▸") {
		t.Fatalf("a member is still marked:\n%s", plainFrame(m))
	}
}

func TestEscClosesTheChainViewOnTheChosenMember(t *testing.T) {
	m, tm := chainShown(t, chainOf(t, loginNames...))
	m = chooseNamed(t, m, "flint-auth")
	i := slices.Index(m.chain.on, m.sideAt)
	if i < 1 {
		t.Fatalf("flint-auth is view %d", i)
	}
	keep := m.chain.panes[i]
	next, cmd := key(m, "esc")
	m = afterChoice(next, cmd)
	lines := strings.Join(tm.lines(), "\n")
	if !strings.Contains(lines, "kill-pane -t %9") || strings.Contains(lines, "kill-pane -t "+keep) {
		t.Fatalf("want every pane but flint-auth's %s killed: %q", keep, lines)
	}
	if m.beside != keep || !strings.Contains(lines, "@matchblox_beside "+keep) {
		t.Fatalf("beside = %q, want %s: %q", m.beside, keep, lines)
	}
	if m.chain != nil {
		t.Fatal("the chain view is still open")
	}
}

func TestEnterOnAMemberOnScreenGivesItsPaneTheKeys(t *testing.T) {
	m, tm := chainShown(t, chainOf(t, loginNames...))
	m = chooseNamed(t, m, "hearth-api")
	pane := m.chain.panes[slices.Index(m.chain.on, m.sideAt)]
	before := len(tm.calls)
	next, cmd := key(m, "enter")
	afterChoice(next, cmd)
	got := strings.Join(tm.lines()[before:], "\n")
	if !strings.Contains(got, "select-pane -Z -t "+pane) || strings.Contains(got, "respawn-pane") || strings.Contains(got, "kill-pane") {
		t.Fatalf("after ⏎: %q", got)
	}
}

func TestBigGOpensTheRoom(t *testing.T) {
	m, tm := besideConsoleWith(t, chainOf(t, loginNames...), 40, 50, &besideTmux{winW: "160", alive: true})
	next, cmd := key(chooseNamed(t, m, "kindle-web"), "G")
	m = afterChoice(next, cmd)
	if m.room == nil || m.room.k.id != "c1" {
		t.Fatalf("room %v", m.room)
	}
	if r := respawned(tm); r != "" {
		t.Fatalf("G respawned %q", r)
	}
}

func TestAPeerChainAsksOnceBeforeItShows(t *testing.T) {
	home, ws := newFake(), newFake()
	tm := &besideTmux{winW: "160", alive: true}
	opt := Options{NoMotion: true, Conn: home, Now: func() time.Time { return demoAt }, Beside: true, Pane: "%1", Tmux: tm.run,
		Peers: []Peer{{Host: "ws-1", Conn: ws}}}
	var next tea.Model = New(opt)
	next, _ = next.Update(tea.WindowSizeMsg{Width: 40, Height: 50})
	next, _ = next.Update(stateMsg(proto.State{}))
	next, _ = next.Update(peerMsg{host: "ws-1", conn: ws, msg: stateMsg(chainOf(t, loginNames...))})
	m := next.(Model)
	m.splashDone = true
	m = feed(t, m, m.besideCmd())
	next, cmd := key(chooseNamed(t, m, "kindle-web"), "g")
	m = afterChoice(next, cmd)
	if m.pending == nil || respawned(tm) != "" {
		t.Fatalf("pending %v, respawn %q: want one consent box first", m.pending != nil, respawned(tm))
	}
	next, cmd = key(m, "y")
	m = afterChoice(next, cmd)
	r := respawned(tm)
	if n := strings.Count(r, " ssh -t "); n != 3 {
		t.Fatalf("%d ssh views in %q (flash %q)", n, r, m.flash)
	}
	if m.pending != nil {
		t.Fatal("a second consent box")
	}
}

func TestTheBesideListHintSaysEnterShowsTheAgent(t *testing.T) {
	m, _ := besideConsole(t, boardFixture(t), 40, 50, "160")
	got := m.sideKeyWords()
	if strings.Contains(got, "write") || !strings.Contains(got, "⏎ show") {
		t.Fatalf("beside list hint = %q, want ⏎ show and no write", got)
	}
}

// closedShell is a shell of this desk closed on a host's console, as q
// leaves it, restarting or not; tm is its tmux, a chain view open.
func closedShell(t *testing.T, restarting bool) *besideTmux {
	t.Helper()
	tm := &besideTmux{winW: "160", kept: "%9", alive: true, chain: "%9 %21 %22"}
	sh := NewShell(ShellOptions{
		Base: Options{NoMotion: true, Beside: true, Pane: "%1", Tmux: tm.run, Socket: "/tmp/tmux-1000/default"},
		Open: func(host string) Options { return Options{Conn: newFake(), Host: host} },
	})
	sh, _ = sh.open("ws-1", false)
	sh.cur.restarting = restarting
	tm.calls = nil
	sh.Close()
	return tm
}

func TestQuittingEndsTheAgentPanes(t *testing.T) {
	tm := closedShell(t, false)
	for _, pane := range []string{"%9", "%21", "%22"} {
		if tm.count("kill-pane -t "+pane) != 1 {
			t.Errorf("%s not killed once: %q", pane, tm.lines())
		}
	}
	for _, opt := range []string{sample.BesideOption, sample.ChainOption, sample.ConsoleOption} {
		if !tm.ran([]string{"tmux", "set-option", "-wu", "-t", "%1", opt}) {
			t.Errorf("%s not unset: %q", opt, tm.lines())
		}
	}
}

func TestARestartKeepsTheAgentPanes(t *testing.T) {
	if tm := closedShell(t, true); tm.count("kill-pane") != 0 || tm.count("set-option") != 0 {
		t.Fatalf("a restart ended the agent panes: %q", tm.lines())
	}
}

// pocketWindow is a console beside the list in a window of w by h, its list
// pane 40 by h, that Pocket alone views; tm counts the zooms of two sizes.
func pocketWindow(t *testing.T, w, h string) *besideTmux {
	t.Helper()
	rows, _ := strconv.Atoi(h)
	m, tm := besideConsoleWith(t, boardFixture(t), 40, rows, &besideTmux{winW: w, winH: h})
	pocketOnly(&m)
	tm.calls = nil
	for range 2 {
		next, cmd := m.Update(tea.WindowSizeMsg{Width: 40, Height: rows})
		m = feed(t, next.(Model), cmd)
	}
	return tm
}

func TestPocketInALandscapeWindowShowsBothPanes(t *testing.T) {
	tm := pocketWindow(t, "120", "30")
	if tm.ran(remote.ZoomArgv("%1", true)) || !tm.ran(remote.ZoomArgv("%1", false)) {
		t.Fatalf("a landscape window of 120 by 30 zoomed: %q", tm.lines())
	}
}

func TestPocketInAnUprightWindowZooms(t *testing.T) {
	tm := pocketWindow(t, "100", "60")
	if !tm.ran(remote.ZoomArgv("%1", true)) || tm.ran(remote.ZoomArgv("%1", false)) {
		t.Fatalf("an upright window of 100 by 60 did not zoom: %q", tm.lines())
	}
}
