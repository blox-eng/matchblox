package app

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// The board's tests of the rules the Sessions list kept (Task 12 step 0c,
// the test map's "to write" list).

// portShot is the board fixture's console at w by h, edit run on its state
// first, with the agents to start; the card of the match named name is
// chosen ("" for the board's own choice).
func portShot(t *testing.T, w, h int, name string, edit func(*proto.State)) Model {
	t.Helper()
	st := boardFixture(t)
	st.Agents = []string{"claude", "codex"}
	if edit != nil {
		edit(&st)
	}
	return boardShot(t, st, w, h, func(m *Model) {
		if name != "" {
			chooseCard(name)(m)
		}
	})
}

// matchOf is the match named name on the board.
func matchOf(t *testing.T, m Model, name string) sample.Session {
	t.Helper()
	for _, s := range append(slices.Clone(m.all.sessions), m.shipCards()...) {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no match %s", name)
	return sample.Session{}
}

// TestNRAOpenTheirFormOnTheChosenCard (R3.7): N opens New session where
// the chosen match works, r opens Rename on it, and A Switch agent.
func TestNRAOpenTheirFormOnTheChosenCard(t *testing.T) {
	m := portShot(t, 120, 40, "kindle-web", func(st *proto.State) {
		for i := range st.Sessions {
			if st.Sessions[i].Name == "kindle-web" {
				st.Sessions[i].Cwd = "/home/user/src/kindle"
			}
		}
	})
	s := matchOf(t, m, "kindle-web")
	n := pressOn(m, "N")
	if f := n.form; f == nil || f.rename || f.swap || n.whereSelected().path() != "/home/user/src/kindle" {
		t.Errorf("N opened %+v, want New session where kindle-web works", f)
	}
	if f := pressOn(m, "r").form; f == nil || !f.rename || f.pane != s.Pane {
		t.Errorf("r opened %+v, want Rename on %s", f, s.Pane)
	}
	if f := pressOn(m, "A").form; f == nil || !f.swap || f.pane != s.Pane {
		t.Errorf("A opened %+v, want Switch agent on %s", f, s.Pane)
	}
}

// TestZParksAndFreesTheChosenCard (R3.8): z parks the chosen match, and z
// on a card it parked frees it; each is the service's park:<pane>. A park
// by you is in QUIET, and its card says so.
func TestZParksAndFreesTheChosenCard(t *testing.T) {
	m := portShot(t, 120, 40, "kindle-web", nil)
	s := matchOf(t, m, "kindle-web")
	if acts := boardFake(pressOn(m, "z")).acts(); len(acts) != 1 || acts[0].RecID != "park:"+s.Pane {
		t.Fatalf("z sent %+v, want park:%s", acts, s.Pane)
	}
	held := portShot(t, 120, 40, "kindle-legacy", nil)
	legacy := matchOf(t, held, "kindle-legacy")
	if c := held.columnOf(legacy); c != colQuiet {
		t.Errorf("a match parked by you is in %s", c.name())
	}
	if word, _ := held.cardSays(legacy); word != "⏸ parked by you" {
		t.Errorf("the parked card says %q", word)
	}
	if acts := boardFake(pressOn(held, "z")).acts(); len(acts) != 1 || acts[0].RecID != "park:"+legacy.Pane {
		t.Fatalf("z on the parked card sent %+v, want park:%s", acts, legacy.Pane)
	}
}

// TestEShowsAndHidesTheExitedOnTheBoard (R3.10): the agents that exited
// are one faint line under the columns; e shows each parent with its fix
// and the pids, and e again hides them.
func TestEShowsAndHidesTheExitedOnTheBoard(t *testing.T) {
	m := portShot(t, 120, 40, "", func(st *proto.State) {
		st.Exited = []sample.Exited{{PID: 4242, Parent: 1, ParentComm: "systemd"}, {PID: 4243, Parent: 1, ParentComm: "systemd"}}
	})
	if out := plainFrame(m); !strings.Contains(out, "2 exited agents · e shows them") {
		t.Fatalf("the board does not say the exited agents:\n%s", out)
	}
	m = pressOn(m, "e")
	out := plainFrame(m)
	for _, want := range []string{"2 EXITED AGENTS", "systemd (pid 1) has not reaped them", "sudo systemctl daemon-reexec reaps them", "pids 4242, 4243"} {
		if !strings.Contains(out, want) {
			t.Fatalf("e does not show %q:\n%s", want, out)
		}
	}
	if out := plainFrame(pressOn(m, "e")); strings.Contains(out, "EXITED AGENTS") || !strings.Contains(out, "2 exited agents") {
		t.Errorf("e again does not hide them:\n%s", out)
	}
}

// TestTheCardSaysWhatTheRowSaid: the card's word is the row's: the
// service's idle reason in place of idle, the queue's paused and waits,
// an estimated wait's question, and out for a finished turn of a session
// the limit stops. A card outside tmux says so, and A r z x ⏎ on it open
// nothing and send nothing.
func TestTheCardSaysWhatTheRowSaid(t *testing.T) {
	st := func(st *proto.State) {
		for i, s := range st.Sessions {
			switch s.Name {
			case "ember-reports":
				st.Reasons = map[string]proto.IdleReason{s.Pane: {Word: "#4556 waits your merge", Tone: "accent"}}
			case "spark-docs":
				st.Sessions[i].Busy, st.Sessions[i].Status, st.Sessions[i].Idle = false, "idle", time.Minute
				st.Sessions[i].Progress = &sample.Progress{Pct: 60, Step: "wiring"}
				st.Queue = append(st.Queue, queue.Item{Pane: s.Pane, Target: s.Target, Name: s.Name, State: queue.StateFinished, Since: demoAt.Add(-time.Minute)})
			case "hearth-limits":
				st.Queue = append(st.Queue, queue.Item{Pane: s.Pane, Target: s.Target, Name: s.Name, State: queue.StateQuestion,
					Since: demoAt.Add(-time.Minute), LastLine: "Should a refresh also rotate the token?", Estimated: true})
			case "wick-mobile":
				st.Queue = append(st.Queue, queue.Item{Pane: s.Pane, Target: s.Target, Name: s.Name, State: queue.StateFinished, Since: demoAt.Add(-time.Minute)})
			}
		}
	}
	for _, w := range []int{44, 132, 200} {
		m := portShot(t, w, 40, "", st)
		for name, want := range map[string]string{"ember-reports": "#4556 waits your merge", "spark-docs": "paused", "hearth-limits": "waits", "wick-mobile": "out"} {
			if word, _ := m.cardSays(matchOf(t, m, name)); word != want {
				t.Errorf("width %d: %s's card says %q, want %q", w, name, word, want)
			}
		}
		if _, says := m.cardSays(matchOf(t, m, "hearth-limits")); says != "Should a refresh also rotate the token?" {
			t.Errorf("width %d: the estimated wait's card says %q", w, says)
		}
		if w == 132 {
			continue // QUIET's cards are one line beside the others: no word
		}
		m = portShot(t, w, 40, "ember-reports", st)
		m.boardShowCol(colQuiet)
		if out := plainFrame(m); !strings.Contains(out, "#4556 waits your merge") {
			t.Errorf("width %d: the reason is not on the card, or the live view's head:\n%s", w, out)
		}
	}

	loose := portShot(t, 120, 40, "flint-ui", func(st *proto.State) {
		for i := range st.Sessions {
			if st.Sessions[i].Name == "flint-ui" {
				st.Sessions[i].Pane = ""
			}
		}
	})
	if out := plainFrame(loose); !strings.Contains(out, "outside tmux") {
		t.Fatalf("the card outside tmux does not say so:\n%s", out)
	}
	for _, k := range []string{"A", "r", "z", "x"} {
		if nm := pressOn(loose, k); nm.form != nil || nm.pending != nil {
			t.Errorf("%s on a card outside tmux opened %+v %+v", k, nm.form, nm.pending)
		}
	}
	for _, k := range []string{"enter", "a", "ctrl+o"} {
		next, cmd := key(loose, k)
		drain(cmd)
		if nm := next.(Model); nm.standIn || nm.pending != nil || !strings.Contains(plainFrame(nm), "flint-ui runs outside tmux: no pane to go into") {
			t.Errorf("%s on a card outside tmux: %q", k, noticeRow(nm))
		}
	}
	if acts := boardFake(loose).acts(); len(acts) != 0 {
		t.Errorf("a card outside tmux sent %+v", acts)
	}
}

// TestTheStokersAgentIsNoCard (#81): the stoker's own agent is no card and
// no count, and the header still says the stoker needs you.
func TestTheStokersAgentIsNoCard(t *testing.T) {
	for _, w := range []int{44, 132} {
		edit := func(asks bool) func(*proto.State) {
			return func(st *proto.State) {
				st.StokerServer = "mbtest"
				for i, s := range st.Sessions {
					if s.Name == "ember-reports" {
						st.Sessions[i].Pane, st.Sessions[i].Target, st.Sessions[i].Asks = s.Pane+"@mbtest", "stoker:1.0", asks
					}
				}
				if asks {
					st.Stoker = &proto.StokerView{Mode: proto.StokerMode{On: true}}
				}
			}
		}
		m := portShot(t, w, 40, "", edit(false))
		m.boardShowCol(colQuiet)
		if strings.Contains(plainFrame(m), "ember-reports") {
			t.Errorf("width %d: the stoker's agent is a card:\n%s", w, plainFrame(m))
		}
		idx := m.boardIndex()
		if n := len(idx[colNeed]) + len(idx[colWork]) + len(idx[colShip]) + len(idx[colQuiet]); n != len(m.all.sessions)-1 {
			t.Errorf("width %d: %d cards for %d matches and the stoker's agent", w, n, len(m.all.sessions))
		}
		m = portShot(t, w, 40, "", edit(true))
		if h := ansi.Strip(m.head(w)); !strings.Contains(h, "stoker needs you") && !strings.Contains(h, "stoker asks") {
			t.Errorf("width %d: the header does not say the stoker needs you: %q", w, h)
		}
	}
}

// TestATapEndsTheSearchsTyping: a tap ends the typing of a search, so the
// box the tapped key asks for shows; the search stays.
func TestATapEndsTheSearchsTyping(t *testing.T) {
	m := pressOn(portShot(t, 120, 40, "", nil), "/")
	m = typeText(m, "kindle").(Model)
	if !m.searching {
		t.Fatal("/ does not type a search")
	}
	b, _ := m.boardDrawOf()
	if !b.picked || b.chosen.Name != "kindle-web" {
		t.Fatalf("the search chose %+v", b.chosen.Name)
	}
	x, y := pressOn2(t, m, "p")
	next, _ := tap(m, x, y)
	nm := next.(Model)
	if nm.searching || nm.filter != "kindle" {
		t.Fatalf("a tap: searching %v, filter %q", nm.searching, nm.filter)
	}
	if nm.pending == nil || !strings.Contains(actionLine(nm), "y") {
		t.Errorf("the box of the tapped key does not show: %q", actionLine(nm))
	}
}

// pressOn2 is the frame's cell of key k on the chosen card.
func pressOn2(t *testing.T, m Model, k string) (x, y int) {
	t.Helper()
	w := max(m.width, minWidth)
	sc := m.drawScreen(w)
	d := m.boardPanel(w, m.bodyRows())
	b, _ := m.boardDrawOf()
	for y, sps := range d.taps {
		if d.cards[boardSpot{y, b.open}] != paneOf(b.chosen) {
			continue
		}
		for _, sp := range sps {
			if sp.press == k {
				return sp.x0, y + sc.bodyTop
			}
		}
	}
	t.Fatalf("no %s on a card:\n%s", k, plainFrame(m))
	return 0, 0
}

// TestTheWheelMovesTheChosenCard: the wheel moves the choice as ↓ and ↑ do.
func TestTheWheelMovesTheChosenCard(t *testing.T) {
	m := portShot(t, 120, 40, "", nil)
	first := chosenName(t, m)
	down := chosenName(t, pressOn(m, "down"))
	next, _ := m.Update(tea.MouseWheelMsg{X: 5, Y: 8, Button: tea.MouseWheelDown})
	if got := chosenName(t, next.(Model)); got != down {
		t.Fatalf("the wheel down chose %s, want %s", got, down)
	}
	next, _ = next.Update(tea.MouseWheelMsg{X: 5, Y: 8, Button: tea.MouseWheelUp})
	if got := chosenName(t, next.(Model)); got != first {
		t.Fatalf("the wheel up chose %s, want %s", got, first)
	}
}

// TestATapOnABoardKeyPressesIt: each key of the board's key line is a tap
// target, and a tap on the header's count of who waits chooses the first
// wait.
func TestATapOnABoardKeyPressesIt(t *testing.T) {
	m := portShot(t, 120, 40, "kindle-web", nil)
	lines := strings.Split(plainFrame(m), "\n")
	y, line := len(lines)-1, lines[len(lines)-1]
	at := func(word string) int {
		i := strings.Index(line, word)
		if i < 0 {
			t.Fatalf("no %q on the key line %q", word, line)
		}
		return ansi.StringWidth(line[:i])
	}
	if next, _ := tap(m, at("p merge"), y); next.(Model).pending == nil || !next.(Model).pending.destructive {
		t.Errorf("a tap on p merge: %+v", next.(Model).pending)
	}
	if next, _ := tap(m, at("v one line"), y); !next.(Model).oneLine() {
		t.Error("a tap on v one line did not change the density")
	}
	ran := recordRuns(&m)
	next, cmd := tap(m, at("⏎ go in"), y)
	drain(cmd)
	if len(*ran) != 1 || next.(Model).standIn {
		t.Errorf("a tap on ⏎ go in ran %q", *ran)
	}
	head := ansi.Strip(strings.Split(m.render(), "\n")[0])
	i := strings.Index(head, "●")
	if i < 0 {
		t.Fatalf("no count in the header %q", head)
	}
	next, _ = tap(m, ansi.StringWidth(head[:i]), 0)
	if got, want := chosenName(t, next.(Model)), m.boardColumn(colNeed)[0].Name; got != want {
		t.Errorf("a tap on who waits chose %s, want %s", got, want)
	}
}

// TestEnterOnAnotherHostsCardGoesIntoItsPane (R3.9): ⏎ on a card of
// another host goes into its pane over that host's ssh, behind ⏎; with
// the home service lost, a card of another host still acts on its host.
func TestEnterOnAnotherHostsCardGoesIntoItsPane(t *testing.T) {
	m := portShot(t, 120, 40, "ember-billing", nil)
	s := matchOf(t, m, "ember-billing")
	if s.Host != "gpu-1" {
		t.Fatalf("ember-billing is on %q", s.Host)
	}
	if p := pressOn(m, "enter").pending; p == nil || !p.remote || p.host != "gpu-1" || p.pane != s.Pane {
		t.Fatalf("⏎ on gpu-1's card: %+v, want the jump into %s on gpu-1", p, s.Pane)
	}
	m.lost = true
	if p := pressOn(m, "x").pending; p == nil || p.host != "gpu-1" {
		t.Fatalf("x on gpu-1's card with home lost: %+v, want its kill on gpu-1", p)
	}
}

// TestATakeOverIsAskedFromTheBoard (R3.11): while the stoker runs the
// night, ⏎, x and a second tap on a card of its host ask "Take over?"
// first; n keeps the stoker and the act still runs, on the card the key was
// pressed on. A card of a host with no stoker acts at once; on another
// host the question and its answer are that host's.
func TestATakeOverIsAskedFromTheBoard(t *testing.T) {
	night := func() Model {
		v := runningNight(demoAt)
		return portShot(t, 120, 40, "kindle-web", func(st *proto.State) { st.Stoker = &v })
	}
	m := night()
	s := matchOf(t, m, "kindle-web")
	ran := recordRuns(&m)
	held, _ := key(m, "enter")
	if held.(Model).held == nil || len(*ran) != 0 {
		t.Fatalf("⏎: held %v ran %q, want Take over? first", held.(Model).held != nil, *ran)
	}
	_, cmd := key(held, "n")
	drain(cmd)
	if len(*ran) != 1 || (*ran)[0][1] != "switch-client" || (*ran)[0][len((*ran)[0])-1] != s.Pane {
		t.Errorf("after n ⏎ ran %q, want the jump into %s", *ran, s.Pane)
	}

	m = night()
	m.selPID, m.selHost, m.chose = matchOf(t, m, "flint-auth").PID, "", true // Sessions' own row is another match
	held, _ = key(m, "x")
	if held.(Model).held == nil {
		t.Fatal("x on the card does not ask Take over? first")
	}
	next, cmd := key(held, "n")
	drain(cmd)
	if p := next.(Model).pending; p == nil || p.rec != "kill:"+s.Pane {
		t.Errorf("after n x is %+v, want the kill of kindle-web (%s)", p, s.Pane)
	}

	m = night()
	ran = recordRuns(&m)
	x, y := pressOn2(t, m, "enter")
	next, cmd = tap(m, x, y)
	drain(cmd)
	next, cmd = tap(next, x, y)
	drain(cmd)
	if next.(Model).held == nil || len(*ran) != 0 {
		t.Errorf("a second tap: held %v ran %q, want Take over? first", next.(Model).held != nil, *ran)
	}

	if p := pressOn(night(), "right", "right").pending; p != nil {
		t.Errorf("a move asked %+v", p)
	}
	m = night() // gpu-1 runs the night too
	billing := matchOf(t, m, "ember-billing")
	m.boardChooseCard(colWork, billing)
	held, _ = key(m, "x")
	if held.(Model).held == nil || held.(Model).heldOn != "gpu-1" {
		t.Fatalf("x on gpu-1's card: held %v on %q, want gpu-1's question", held.(Model).held != nil, held.(Model).heldOn)
	}
	if p := pressOn(held.(Model), "n").pending; p == nil || p.host != "gpu-1" || p.rec != "kill:"+billing.Pane {
		t.Errorf("after n x on gpu-1's card is %+v, want its kill on gpu-1", p)
	}

	hosts, _, conns := stokedHosts(t, nil, nightView(), nil)
	hosts.splashDone = true
	hosts.boardChooseCard(hosts.columnOf(matchOf(t, hosts, "home-1")), matchOf(t, hosts, "home-1"))
	if next := pressOn(hosts, "x"); next.held != nil || next.pending == nil {
		t.Errorf("x on a card of a host with no stoker: held %v pending %+v, want its box at once", next.held != nil, next.pending)
	}
	hosts.boardChooseCard(colQuiet, matchOf(t, hosts, "ws-run"))
	if hosts.columnOf(matchOf(t, hosts, "ws-run")) != colQuiet {
		t.Fatalf("ws-run is in %s", hosts.columnOf(matchOf(t, hosts, "ws-run")).name())
	}
	pressAll(hosts, "enter", "y")
	if got := stokerActs(conns["ws-1"]); len(got) != 1 || got[0] != "stoker:rest" {
		t.Errorf("ws-1 got %v, want its stoker to rest", got)
	}
	if got := stokerActs(conns["gpu-1"]); len(got) != 0 {
		t.Errorf("gpu-1 got %v", got)
	}
}

// TestAFocusChoosesItsCard: a focus (a tap on the island, a buzz) chooses
// its match's card and moves only the phone's client; a focus of a gone
// session says "That session ended.", shows the board and drops a box
// armed elsewhere, so the next y runs nothing.
func TestAFocusChoosesItsCard(t *testing.T) {
	m := portShot(t, 120, 40, "flint-auth", nil)
	ran := recordRuns(&m)
	m.opt.Clients = func() ([]string, error) { return []string{"/dev/ttys004"}, nil }
	s := matchOf(t, m, "spark-docs")
	st := boardFixture(t)
	st.Agents = []string{"claude", "codex"}
	st.Focus = &state.Focus{SessionID: s.SessionID, Pane: s.Pane, Client: "/dev/ttys004", At: demoAt}
	m = newBoardState(t, m, st)
	if got := chosenName(t, m); got != "spark-docs" {
		t.Fatalf("the focus chose %s, want spark-docs", got)
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q before the focus's command", *ran)
	}

	m = portShot(t, 120, 40, "flint-auth", nil)
	m.setTab(tabMachine)
	m.pending = &action{label: "x", steps: [][]string{{"x"}}}
	st.Focus = &state.Focus{SessionID: "gone-session", Gone: true, At: demoAt.Add(time.Second)}
	m = newBoardState(t, m, st)
	if !m.boardShown() || m.pending != nil || m.flash != focusGone {
		t.Fatalf("a gone focus: board %v pending %+v flash %q", m.boardShown(), m.pending, m.flash)
	}
	if acts := boardFake(pressOn(m, "y")).acts(); len(acts) != 0 {
		t.Errorf("y after a gone focus sent %+v", acts)
	}
}

// TestTheChoiceStaysWhileTheDeskWatches: a jump while another screen
// watches leaves the choice where it is.
func TestTheChoiceStaysWhileTheDeskWatches(t *testing.T) {
	m := portShot(t, 120, 40, "kindle-web", nil)
	s := matchOf(t, m, "kindle-web")
	next, _ := m.Update(ranMsg{cmd: "tmux switch-client -c /dev/pts/7 -t " + s.Pane, jumped: s.Pane, tty: "/dev/pts/7", watched: true})
	if got := chosenName(t, next.(Model)); got != "kindle-web" {
		t.Errorf("the choice moved to %s while the desk watches", got)
	}
}

// TestAClickOnTheLiveViewGoesIn: a click on the live view beside the
// board goes into the chosen card's pane.
func TestAClickOnTheLiveViewGoesIn(t *testing.T) {
	m := portShot(t, 200, 40, "kindle-web", nil)
	ran := recordRuns(&m)
	d := m.boardPanel(200, m.bodyRows())
	if d.live == 0 {
		t.Fatal("no live view at 200 columns")
	}
	sc := m.drawScreen(200)
	next, cmd := m.Update(tea.MouseClickMsg(tea.Mouse{X: 200 - d.live + 5, Y: sc.bodyTop + 3, Button: tea.MouseLeft}))
	drain(cmd)
	if len(*ran) != 1 || next.(Model).standIn {
		t.Errorf("a click on the live view ran %q", *ran)
	}
}

// TestTheLiveViewBesideTheBoardHasRoomForAMenu: the live view beside the
// board always has menuLeast rows.
func TestTheLiveViewBesideTheBoardHasRoomForAMenu(t *testing.T) {
	for _, w := range []int{152, 160, 200} {
		for h := 8; h <= 34; h++ {
			m := portShot(t, w, h, "kindle-web", nil)
			d := m.boardPanel(w, m.bodyRows())
			if d.live == 0 {
				continue
			}
			if rows := d.colsTo - d.headAt; rows < menuLeast {
				t.Fatalf("%dx%d: the live view beside the board has %d rows:\n%s", w, h, rows, plainFrame(m))
			}
		}
	}
}

// TestBoardFrameCostGrowsWithRows: a frame's work grows with its cards,
// not with their square, and chains are worked out once in refresh, not
// at each card.
func TestBoardFrameCostGrowsWithRows(t *testing.T) {
	many := func(n int, chained bool) Model {
		var ss []func(sample.Session) sample.Session
		for i := range n {
			ss = append(ss, sess(i+1, fmt.Sprintf("hearth-%d", i), fmt.Sprintf("%%%d", i+1), nil))
		}
		st := hostState(ss...)
		if chained {
			st.Chains = &state.ChainsView{}
			for i := 0; i+2 < n; i += 3 {
				c := chains.Chain{ID: fmt.Sprintf("c%d", i)}
				for _, s := range st.Sessions[i : i+3] {
					c.Members = append(c.Members, memberOf(s))
				}
				st.Chains.Chains = append(st.Chains.Chains, c)
			}
		}
		m, _ := loadedWith(t, 200, st)
		m.splashDone = true
		m.viewPrefs.CopyCheckedPhone, m.viewPrefs.CopyCheckedDesk = true, true
		return m
	}
	cost := func(n int) float64 {
		m := many(n, false)
		return testing.AllocsPerRun(1, func() { _ = m.draw(200) })
	}
	if small, big := cost(300), cost(600); big > 3*small {
		t.Fatalf("600 cards cost %.0f allocations, 300 cost %.0f: more than twice", big, small)
	}
	frame := func(chained bool) time.Duration {
		m := many(900, chained)
		best := time.Hour
		for range 5 {
			start := time.Now()
			_ = m.draw(200)
			best = min(best, time.Since(start))
		}
		return best
	}
	if plain, chained := frame(false), frame(true); chained > 20*plain+100*time.Millisecond {
		t.Fatalf("a frame with chains took %v, one without %v", chained, plain)
	}
}
