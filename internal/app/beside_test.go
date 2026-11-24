package app

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// besideTmux is a fake tmux that records each command and answers which
// window a pane is in; views records the views the agent region ran.
type besideTmux struct {
	own   []string // other panes in the console's window
	calls [][]string
	views []string
}

func (b *besideTmux) run(args ...string) (string, error) {
	b.calls = append(b.calls, args)
	if args[0] == "display" && args[len(args)-1] == "#{window_id}" {
		if args[3] == "%1" || slices.Contains(b.own, args[3]) {
			return "@1\n", nil
		}
		return "@2\n", nil
	}
	return "", nil
}

// shown is the view the agent region ran last; "" none.
func (b *besideTmux) shown() string {
	if len(b.views) == 0 {
		return ""
	}
	return b.views[len(b.views)-1]
}

// besideConsoleWith is the desk at w by h on st, its agent region
// recording each view it would run into tm, and running none.
func besideConsoleWith(t *testing.T, st proto.State, w, h int, tm *besideTmux) (Model, *besideTmux) {
	t.Helper()
	old := startAgent
	startAgent = func(argv []string, _, _ int) (*agentTerm, error) {
		tm.views = append(tm.views, strings.Join(argv, " "))
		return nil, errors.New("a test runs no view")
	}
	t.Cleanup(func() { startAgent = old })
	opt := Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }, Beside: true, Pane: "%1", Tmux: tm.run}
	var next tea.Model = New(opt)
	next, _ = next.Update(tea.WindowSizeMsg{Width: w, Height: h})
	next, _ = next.Update(stateMsg(st))
	m := next.(Model)
	m.splashDone = true
	m.viewPrefs.CopyCheckedPhone, m.viewPrefs.CopyCheckedDesk = true, true
	m.toList()
	return m, tm
}

// peerDesk is the desk with the board's fixture on the peer ws-1.
func peerDesk(t *testing.T, st proto.State) (Model, *besideTmux) {
	t.Helper()
	ws := newFake()
	m, tm := besideConsoleWith(t, proto.State{}, 160, 50, &besideTmux{})
	m.opt.Peers = []Peer{{Host: "ws-1", Conn: ws}}
	m = New(m.opt)
	m = upd(t, m, tea.WindowSizeMsg{Width: 160, Height: 50})
	m = upd(t, m, stateMsg(proto.State{}))
	m = upd(t, m, peerMsg{host: "ws-1", conn: ws, msg: stateMsg(st)})
	m.splashDone = true
	m.viewPrefs.CopyCheckedPhone, m.viewPrefs.CopyCheckedDesk = true, true
	m.toList()
	return m, tm
}

func shownAt(t *testing.T, name string) (Model, *besideTmux) {
	t.Helper()
	m, tm := besideConsoleWith(t, boardFixture(t), 160, 50, &besideTmux{})
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

// afterChoice runs cmd, and gives the console the end of each choice.
func afterChoice(next tea.Model, cmd tea.Cmd) Model {
	m := next.(Model)
	for _, msg := range cmdMsgs(cmd) {
		if msg, ok := msg.(shownMsg); ok {
			next, cmd := m.Update(msg)
			m = afterChoice(next, cmd) // a choice may start the next one
		}
	}
	return m
}

// live gives m's agent region a screen that shows and runs, as a view's
// first frame leaves it.
func live(t *testing.T, m Model) Model {
	t.Helper()
	scr := newAgentScreen(m.agent.w, m.agent.h)
	t.Cleanup(func() { _ = scr.Close() })
	m.agent.term = &agentTerm{scr: scr, framed: true}
	return m
}

func TestEnterShowsTheChosenAgent(t *testing.T) {
	m, tm := shownAt(t, "kindle-web")
	m = enter(t, m)
	if r := tm.shown(); !strings.HasPrefix(r, "/bin/sh -c") || !strings.Contains(r, "matchblox-jump-") || !strings.Contains(r, "kindle-web") {
		t.Fatalf("the region ran %q (flash %q)", r, m.flash)
	}
	if !m.agent.keys || m.agent.name != "kindle-web" || m.agent.w != 119 || m.agent.h != 46 {
		t.Fatalf("the region shows %q at %d×%d, keys %v: want kindle-web at 119×46 with the keys", m.agent.name, m.agent.w, m.agent.h, m.agent.keys)
	}
}

func TestTheBarNamesTheSessionWithTheKeys(t *testing.T) {
	m, _ := shownAt(t, "kindle-web")
	m = live(t, enter(t, m))
	if got := keyLineOf(m); got != "AGENT  kindle-web" {
		t.Fatalf("with the agent's keys the bar says %q, want %q", got, "AGENT  kindle-web")
	}
	if got := keyLineOf(pressOn(m, "ctrl+q")); got != "LIST" {
		t.Fatalf("with the list's keys the bar says %q, want LIST", got)
	}
}

// rest runs cmd as the list rests after a move: the preview's timer, then
// the end of the choice it starts.
func rest(t *testing.T, next tea.Model, cmd tea.Cmd) Model {
	t.Helper()
	m := next.(Model)
	for _, msg := range cmdMsgs(cmd) {
		if msg, ok := msg.(previewMsg); ok {
			next, cmd := m.Update(msg)
			m = afterChoice(next, cmd)
		}
	}
	return m
}

// restNow makes the preview's rest 0 for the test.
func restNow(t *testing.T) {
	t.Helper()
	old := previewRest
	previewRest = 0
	t.Cleanup(func() { previewRest = old })
}

func TestArrowsPreviewTheChosenAgent(t *testing.T) {
	restNow(t)
	m, tm := shownAt(t, "hearth-limits")
	next, cmd := key(m, "up")
	m = rest(t, next, cmd)
	s, _ := m.sideChosen()
	if r := tm.shown(); r == "" || !strings.Contains(r, s.Name) || strings.Contains(r, "hearth-limits") {
		t.Fatalf("the region ran %q, want a view of %s", r, s.Name)
	}
	if m.agent.keys {
		t.Fatal("a preview took the keys")
	}
}

func TestQuickArrowsPreviewOnlyWhereTheyStop(t *testing.T) {
	restNow(t)
	m, tm := shownAt(t, "hearth-limits")
	next, first := key(m, "up")
	next, second := key(next, "down")
	next, third := key(next, "up")
	m = rest(t, next, first)
	m = rest(t, m, second)
	if r := tm.shown(); r != "" {
		t.Fatalf("a passed row was shown: %q", r)
	}
	rest(t, m, third)
	if r := tm.shown(); !strings.Contains(r, "spark-docs") {
		t.Fatalf("the region ran %q, want spark-docs", r)
	}
}

func TestEnterOnAPreviewGivesItTheKeys(t *testing.T) {
	restNow(t)
	m, tm := shownAt(t, "hearth-limits")
	next, cmd := key(m, "up")
	m = live(t, rest(t, next, cmd))
	views := len(tm.views)
	if m = enter(t, m); len(tm.views) != views || !m.agent.keys {
		t.Fatalf("want the keys and no new view: %q, keys %v", tm.views, m.agent.keys)
	}
}

func TestEnterOnAnEndedPreviewShowsItAgain(t *testing.T) {
	restNow(t)
	m, tm := shownAt(t, "hearth-limits")
	next, cmd := key(m, "up")
	m = rest(t, next, cmd) // the view did not run: nothing shows
	views := len(tm.views)
	if enter(t, m); len(tm.views) != views+1 {
		t.Fatalf("a view that does not run was not shown again: %q", tm.views)
	}
}

func TestAStokerHostDoesNotPreview(t *testing.T) {
	restNow(t)
	st := boardFixture(t)
	v := runningNight(demoAt)
	st.Stoker = &v
	m, tm := besideConsoleWith(t, st, 160, 50, &besideTmux{})
	m.opt.Socket = "/tmp/tmux-1000/default"
	m = chooseNamed(t, m, "kindle-web")
	next, cmd := key(m, "up")
	m = rest(t, next, cmd)
	if m.held != nil || tm.shown() != "" {
		t.Fatalf("held %v, ran %q: a look asked or showed", m.held != nil, tm.shown())
	}
}

func TestAPeerAgentAsksBeforeItShows(t *testing.T) {
	m, tm := peerDesk(t, boardFixture(t))
	m = chooseNamed(t, m, "kindle-web")
	if s, _ := m.sideChosen(); s.Host != "ws-1" {
		t.Fatalf("kindle-web is on %q", s.Host)
	}
	next, cmd := key(m, "enter")
	m = afterChoice(next, cmd)
	if m.pending == nil || tm.shown() != "" {
		t.Fatalf("pending %v, ran %q: want the consent box first", m.pending != nil, tm.shown())
	}
	next, cmd = key(m, "y")
	afterChoice(next, cmd)
	if r := tm.shown(); !strings.HasPrefix(r, "/bin/sh") || !strings.Contains(r, " ssh ") || !strings.Contains(r, "ws-1") {
		t.Fatalf("the region ran %q", r)
	}
}

// TestAnOldPeerTmuxSaysTheFix: a peer's view tells its tmux -T RGB
// (tmux 3.2). A peer whose hello says an older tmux runs no view, and
// the console names the fix.
func TestAnOldPeerTmuxSaysTheFix(t *testing.T) {
	m, tm := peerDesk(t, boardFixture(t))
	m.peers[m.peerAt("ws-1")].hello.Tmux = "3.1"
	next, cmd := key(chooseNamed(t, m, "kindle-web"), "enter")
	next, cmd = key(afterChoice(next, cmd), "y")
	m = afterChoice(next, cmd)
	const want = "refused: tmux 3.1 on ws-1 is older than 3.2: update tmux there, then open the host again"
	if r := tm.shown(); r != "" || !strings.Contains(m.flash+m.noticeText+m.sideSays(), want) {
		t.Fatalf("the region ran %q; the console says %q / %q, want %q", r, m.flash, m.sideSays(), want)
	}
}

func TestAPeerPreviewsOnlyAfterItsConsent(t *testing.T) {
	restNow(t)
	m, tm := peerDesk(t, boardFixture(t))
	m = chooseNamed(t, m, "kindle-web")
	next, cmd := key(m, "down")
	m = rest(t, next, cmd)
	if m.pending != nil || tm.shown() != "" {
		t.Fatalf("pending %v, ran %q: a look at a peer asked or showed", m.pending != nil, tm.shown())
	}
	m = chooseNamed(t, m, "kindle-web")
	next, cmd = key(m, "enter")
	next, cmd = key(afterChoice(next, cmd), "y")
	m = afterChoice(next, cmd)
	next, cmd = key(m, "down")
	m = rest(t, next, cmd)
	s, _ := m.sideChosen()
	if r := tm.shown(); m.pending != nil || !strings.Contains(r, " ssh ") || !strings.Contains(r, s.Name) {
		t.Fatalf("after the consent: pending %v, ran %q", m.pending != nil, r)
	}
}

func TestAPeerViewWaitsWhenItEnds(t *testing.T) {
	m, tm := peerDesk(t, boardFixture(t))
	next, cmd := key(chooseNamed(t, m, "kindle-web"), "enter")
	next, cmd = key(afterChoice(next, cmd), "y")
	afterChoice(next, cmd)
	r := tm.shown()
	if !strings.HasPrefix(r, "/bin/sh -c") || !strings.Contains(r, "ctrl+q: back to the list") || !strings.Contains(r, " ssh ") {
		t.Fatalf("the peer's ssh is not wrapped: %q", r)
	}
	if !strings.Contains(r, "session-window-changed") || !strings.Contains(r, sample.ViewOption) {
		t.Fatalf("the peer's view does not end with its agent's pane: %q", r)
	}
}

func TestAStokerHostAsksToTakeOver(t *testing.T) {
	st := boardFixture(t)
	v := runningNight(demoAt)
	st.Stoker = &v
	m, tm := besideConsoleWith(t, st, 160, 50, &besideTmux{})
	m.opt.Socket = "/tmp/tmux-1000/default"
	m = chooseNamed(t, m, "kindle-web")
	next, cmd := key(m, "enter")
	m = afterChoice(next, cmd)
	if m.held == nil || tm.shown() != "" {
		t.Fatalf("held %v, ran %q: want take over? first", m.held != nil, tm.shown())
	}
	next, cmd = key(m, "n")
	afterChoice(next, cmd)
	if tm.shown() == "" {
		t.Fatal("no view after the answer")
	}
}

func TestALocalViewNeedsTheSocket(t *testing.T) {
	m, tm := shownAt(t, "kindle-web")
	m.opt.Socket = ""
	m = enter(t, m)
	if tm.shown() != "" || !strings.Contains(m.flash, "socket") {
		t.Fatalf("ran %q, flash %q", tm.shown(), m.flash)
	}
}

// TestAClickOnARowKeepsTheKeysInTheList: a click on a row shows its agent
// at once, and the keys stay in the list, so ↑ and ↓ go on choosing.
func TestAClickOnARowKeepsTheKeysInTheList(t *testing.T) {
	m, tm := shownAt(t, "kindle-web")
	next, cmd := tapRow(t, m, "flint-auth")
	m = afterChoice(next, cmd)
	if r := tm.shown(); r == "" || !strings.Contains(r, "flint-auth") {
		t.Fatalf("the region ran %q", r)
	}
	if m.agent.keys {
		t.Fatal("the click gave the agent the keys")
	}
	if s, _ := m.sideChosen(); s.Name != "flint-auth" {
		t.Fatalf("the list chose %s, want flint-auth", s.Name)
	}
}

// TestAClickOnTheShownRowKeepsTheKeysInTheList: a click on the row the
// agent region shows already leaves the keys in the list.
func TestAClickOnTheShownRowKeepsTheKeysInTheList(t *testing.T) {
	m, tm := shownAt(t, "kindle-web")
	m = live(t, enter(t, m))
	m.agent.keys = false
	views := len(tm.views)
	next, cmd := tapRow(t, m, "kindle-web")
	if m = afterChoice(next, cmd); m.agent.keys || len(tm.views) != views {
		t.Fatalf("want no new view and the keys in the list: %q, keys %v", tm.views, m.agent.keys)
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

func TestTypingInTheListSaysTheAgentTakesText(t *testing.T) {
	m, _ := shownAt(t, "kindle-web")
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
	if !strings.Contains(said["l"], "the agent takes text") || !strings.Contains(said["l"], "⏎ goes into kindle-web") {
		t.Fatalf("l says %q", said["l"])
	}
}

func TestTwoQuickChoicesShowTheSecond(t *testing.T) {
	m, tm := shownAt(t, "kindle-web")
	next, first := key(m, "enter")
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
	if r := tm.shown(); !strings.Contains(r, "flint-auth") {
		t.Fatalf("the region ran %q last, want flint-auth", tm.views)
	}
	if m.showing {
		t.Fatal("a choice still runs")
	}
}

func TestAPaneOfTheConsolesWindowIsNotShown(t *testing.T) {
	m, tm := shownAt(t, "kindle-web")
	s, _ := m.sideChosen()
	tm.own = []string{s.Pane}
	m = enter(t, m)
	if r := tm.shown(); r != "" {
		t.Fatalf("the region ran %q", r)
	}
	if !strings.Contains(m.flash, "console's own window") || !strings.Contains(m.flash, "ctrl+o") {
		t.Fatalf("flash %q", m.flash)
	}
}

// TestTheShellKeepsTheAgentRegion: the console the shell opens for this
// machine shows views on the console's tmux socket, as main asked; a
// host's console keeps the drawn chat.
func TestTheShellKeepsTheAgentRegion(t *testing.T) {
	sh := NewShell(ShellOptions{
		Base: Options{NoMotion: true, Beside: true, Socket: "/tmp/tmux-1000/default"},
		Open: func(host string) Options { return Options{Conn: newFake(), Host: host} },
	})
	here, _ := sh.open("", false)
	if o := here.cur.opt; !o.Beside || o.Socket != "/tmp/tmux-1000/default" {
		t.Fatalf("this machine's console lost the agent region: Beside %v, Socket %q", o.Beside, o.Socket)
	}
	there, _ := sh.open("ws-1", false)
	if there.cur.opt.Beside {
		t.Fatal("a host's console shows agents' own screens")
	}
}

// TestGShowsOnlyTheChainOnTheDesk: with the agent's screen on the desk, g
// shows only the chosen session's chain in the list, as everywhere, and
// runs no view.
func TestGShowsOnlyTheChainOnTheDesk(t *testing.T) {
	m, tm := besideConsoleWith(t, chainOf(t, "flint-auth", "kindle-web"), 160, 50, &besideTmux{})
	next, cmd := key(chooseNamed(t, m, "kindle-web"), "g")
	if m = afterChoice(next, cmd); m.boardChain == "" || tm.shown() != "" {
		t.Fatalf("g: the list shows chain %q, the region ran %q", m.boardChain, tm.shown())
	}
}

// TestBigGIsNoKey: g alone shows the chain; G, which did the same, is
// gone, and says what a letter does in the list beside the agent.
func TestBigGIsNoKey(t *testing.T) {
	m, tm := besideConsoleWith(t, chainOf(t, "flint-auth", "kindle-web"), 160, 50, &besideTmux{})
	next, cmd := key(chooseNamed(t, m, "kindle-web"), "G")
	m = afterChoice(next, cmd)
	if m.room != nil || m.boardChain != "" || tm.shown() != "" {
		t.Fatalf("G: room %v, chain %q, the region ran %q", m.room, m.boardChain, tm.shown())
	}
	if !strings.HasPrefix(m.noticeText, "the agent takes text") {
		t.Fatalf("G says %q", m.noticeText)
	}
}

// chainOf is the board's fixture with the chain login made of names.
func chainOf(t *testing.T, names ...string) proto.State {
	t.Helper()
	st := boardFixture(t)
	var ms []chains.Member
	for _, name := range names {
		i := slices.IndexFunc(st.Sessions, func(s sample.Session) bool { return s.Name == name })
		if i < 0 {
			t.Fatalf("no %s in the fixture", name)
		}
		ms = append(ms, memberOf(st.Sessions[i]))
	}
	st.Chains.Chains[0].Members = ms
	return st
}

func TestANameEndingInASemicolonShowsAsItsPane(t *testing.T) {
	m, _ := shownAt(t, "kindle-web")
	s, ok := m.sideChosen()
	if !ok {
		t.Fatal("nothing chosen")
	}
	for i := range m.all.sessions {
		if m.all.sessions[i].Pane == s.Pane {
			m.all.sessions[i].Name = "kindle-web;"
		}
	}
	if got := m.besideName(s.Host, s.Pane); got != s.Pane {
		t.Fatalf("besideName = %q, want the pane %s", got, s.Pane)
	}
}

func TestAResizeToThePhoneClosesTheSheet(t *testing.T) {
	m, _ := shownAt(t, "kindle-web")
	if m = pressOn(m, "?"); m.over != "keys" {
		t.Fatal("? opened no sheet on the desk")
	}
	m = upd(t, m, tea.WindowSizeMsg{Width: 50, Height: 50})
	if m.desk() || m.over != "" {
		t.Fatalf("on the phone (desk %v) the sheet %q stays open, unseen", m.desk(), m.over)
	}
	before := chosenName(t, m)
	if m = pressOn(m, "down"); chosenName(t, m) == before {
		t.Fatalf("on the phone ↓ did not move the list from %s: a sheet took it", before)
	}
}

func TestPocketHeldUprightClosesTheSheet(t *testing.T) {
	m, _ := shownAt(t, "kindle-web")
	m = upd(t, m, tea.WindowSizeMsg{Width: 160, Height: 80}) // tall, and still a desk until Pocket answers
	if m = pressOn(m, "?"); m.over != "keys" {
		t.Fatal("? opened no sheet on the desk")
	}
	pocketOnly(&m)
	before := chosenName(t, m)
	if m = pressOn(m, "down"); m.desk() || m.over != "" || chosenName(t, m) == before {
		t.Fatalf("Pocket upright (desk %v): the sheet %q stays, and ↓ left the list on %s", m.desk(), m.over, chosenName(t, m))
	}
}

// TestPocketsBeltActsOnTheListBesideTheAgent: Pocket held sideways draws
// the desk, and the belt's taps stay the list's keys while the agent has
// the keyboard's, as they were beside the old split: none types into the
// agent.
func TestPocketsBeltActsOnTheListBesideTheAgent(t *testing.T) {
	m, _ := shownAt(t, "kindle-web")
	pocketOnly(&m)
	m = live(t, enter(t, m))
	if !m.desk() || m.deskMode() != "AGENT" {
		t.Fatalf("Pocket sideways: desk %v, mode %s; want the desk with the agent's keys", m.desk(), m.deskMode())
	}
	typed := make(chan string, 64)
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := m.agent.term.scr.Read(buf)
			if n > 0 {
				typed <- string(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	acts := map[string]func(Model) bool{
		"m": func(n Model) bool { return n.menu != nil },
		"/": func(n Model) bool { return n.searching },
		"x": func(n Model) bool { return n.pending != nil },
		"p": func(n Model) bool { return n.pending != nil },
		"N": func(n Model) bool { return n.form != nil || strings.HasPrefix(n.flash, "no agent to start here") },
	}
	for key, acted := range acts {
		next, _ := m.Update(tapFrom(key, beltPhone))
		if !acted(next.(Model)) {
			t.Errorf("the belt's %s did not act on the list (notice %q)", key, next.(Model).noticeText)
		}
		select {
		case b := <-typed:
			t.Errorf("the belt's %s typed %q into the agent", key, b)
		case <-time.After(20 * time.Millisecond):
		}
	}
}
