package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
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

func matchOf(t *testing.T, m Model, name string) sample.Session {
	t.Helper()
	for _, s := range m.all.sessions {
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
// on a row it parked frees it; each is the service's park:<pane>. A park
// by you is in HELD, and its row says so.
func TestZParksAndFreesTheChosenCard(t *testing.T) {
	m := portShot(t, 120, 40, "kindle-web", nil)
	s := matchOf(t, m, "kindle-web")
	if acts := boardFake(pressOn(m, "z")).acts(); len(acts) != 1 || acts[0].RecID != "park:"+s.Pane {
		t.Fatalf("z sent %+v, want park:%s", acts, s.Pane)
	}
	held := portShot(t, 120, 40, "kindle-legacy", nil)
	legacy := matchOf(t, held, "kindle-legacy")
	if h := held.sideStateOf(legacy); h != sideHeld {
		t.Errorf("a match parked by you is in %s", h.head())
	}
	if word, _ := held.cardSays(legacy); word != "⏸ parked by you" {
		t.Errorf("the parked card says %q", word)
	}
	if acts := boardFake(pressOn(held, "z")).acts(); len(acts) != 1 || acts[0].RecID != "park:"+legacy.Pane {
		t.Fatalf("z on the parked card sent %+v, want park:%s", acts, legacy.Pane)
	}
}

// TestTheStokersAgentIsNoCard (#81): the stoker's own agent is no row and
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
		if strings.Contains(plainFrame(m), "ember-reports") {
			t.Errorf("width %d: the stoker's agent is a row:\n%s", w, plainFrame(m))
		}
		if n := len(m.sideSessions()); n != len(m.all.sessions)-1 {
			t.Errorf("width %d: %d rows for %d matches and the stoker's agent", w, n, len(m.all.sessions))
		}
		m = portShot(t, w, 40, "", edit(true))
		if h := ansi.Strip(m.head(w)); !strings.Contains(h, "stoker needs you") && !strings.Contains(h, "stoker asks") {
			t.Errorf("width %d: the header does not say the stoker needs you: %q", w, h)
		}
	}
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

// TestEnterOnAnotherHostsCardGoesIntoItsPane (R3.9): ⏎ on a card of
// another host goes into its pane over that host's ssh, behind ⏎; with
// the home service lost, a card of another host still acts on its host.
func TestCtrlOOnAnotherHostsRowGoesIntoItsPane(t *testing.T) {
	m := portShot(t, 120, 40, "ember-billing", nil)
	s := matchOf(t, m, "ember-billing")
	if s.Host != "gpu-1" {
		t.Fatalf("ember-billing is on %q", s.Host)
	}
	if p := pressOn(m, "ctrl+o").pending; p == nil || !p.remote || p.host != "gpu-1" || p.pane != s.Pane {
		t.Fatalf("ctrl+o on gpu-1's row: %+v, want the jump into %s on gpu-1", p, s.Pane)
	}
	m.lost = true
	if p := pressOn(m, "x").pending; p == nil || p.host != "gpu-1" {
		t.Fatalf("x on gpu-1's row with home lost: %+v, want its kill on gpu-1", p)
	}
}

// TestATakeOverIsAskedFromTheList (R3.11): while the stoker runs the
// night, ctrl+o and x on a session of its host in the list ask "Take
// over?" first; n keeps the stoker and the act still runs, on the session
// the key was pressed on. A session of a host with no stoker acts at once;
// on another host the question and its answer are that host's.
func TestATakeOverIsAskedFromTheList(t *testing.T) {
	night := func() Model {
		v := runningNight(demoAt)
		return portShot(t, 120, 40, "kindle-web", func(st *proto.State) { st.Stoker = &v })
	}
	m := night()
	s := matchOf(t, m, "kindle-web")
	ran := recordRuns(&m)
	held, _ := key(m, "ctrl+o")
	if held.(Model).held == nil || len(*ran) != 0 {
		t.Fatalf("ctrl+o: held %v ran %q, want Take over? first", held.(Model).held != nil, *ran)
	}
	_, cmd := key(held, "n")
	drain(cmd)
	if len(*ran) != 1 || (*ran)[0][1] != "switch-client" || (*ran)[0][len((*ran)[0])-1] != s.Pane {
		t.Errorf("after n ctrl+o ran %q, want the jump into %s", *ran, s.Pane)
	}

	m = night()
	m.selPID, m.selHost, m.chose = matchOf(t, m, "flint-auth").PID, "", true // Sessions' own row is another match
	held, _ = key(m, "x")
	if held.(Model).held == nil {
		t.Fatal("x on the row does not ask Take over? first")
	}
	next, cmd := key(held, "n")
	drain(cmd)
	if p := next.(Model).pending; p == nil || p.rec != "kill:"+s.Pane {
		t.Errorf("after n x is %+v, want the kill of kindle-web (%s)", p, s.Pane)
	}

	m = night() // gpu-1 runs the night too
	billing := matchOf(t, m, "ember-billing")
	m.chooseRow(billing)
	held, _ = key(m, "x")
	if held.(Model).held == nil || held.(Model).heldOn != "gpu-1" {
		t.Fatalf("x on gpu-1's row: held %v on %q, want gpu-1's question", held.(Model).held != nil, held.(Model).heldOn)
	}
	if p := pressOn(held.(Model), "n").pending; p == nil || p.host != "gpu-1" || p.rec != "kill:"+billing.Pane {
		t.Errorf("after n x on gpu-1's row is %+v, want its kill on gpu-1", p)
	}

	hosts, _, conns := stokedHosts(t, nil, nightView(), nil)
	hosts.splashDone = true
	hosts.toList()
	hosts.chooseRow(matchOf(t, hosts, "home-1"))
	if next := pressOn(hosts, "x"); next.held != nil || next.pending == nil {
		t.Errorf("x on a row of a host with no stoker: held %v pending %+v, want its box at once", next.held != nil, next.pending)
	}
	hosts.chooseRow(matchOf(t, hosts, "ws-run"))
	pressAll(hosts, "ctrl+o", "y")
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
