package app

import (
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/sample"
)

// cellOf is the cell where sub starts on the plain line l, or -1.
func cellOf(l, sub string) int {
	i := strings.Index(l, sub)
	if i < 0 {
		return -1
	}
	return ansi.StringWidth(l[:i])
}

// tapText taps the first cell of the first line of the frame that has sub.
func tapText(t *testing.T, m Model, sub string) Model {
	t.Helper()
	for y, l := range strings.Split(plainFrame(m), "\n") {
		if x := cellOf(l, sub); x >= 0 {
			next, cmd := tap(m, x, y)
			drain(cmd)
			return next.(Model)
		}
	}
	t.Fatalf("no %q on the frame:\n%s", sub, plainFrame(m))
	return m
}

func TestAChainKeepsTheBoardsWidth(t *testing.T) {
	st := boardFixture(t)
	for _, size := range [][2]int{{40, 34}, {80, 31}, {104, 38}, {160, 38}} {
		m := boardShot(t, st, size[0], size[1], func(m *Model) { m.boardChain = "login" })
		for i, l := range frameLines(m) {
			if w := ansi.StringWidth(l); w != size[0] {
				t.Errorf("%d: line %d is %d cells: %q", size[0], i, w, ansi.Strip(l))
			}
		}
	}
}

func TestTheCountSeesThroughEveryFilter(t *testing.T) {
	st := boardFixture(t)
	for i, s := range st.Sessions {
		if s.Name == "hearth-notes" { // on gpu-1
			st.Sessions[i].Permits, st.Sessions[i].LastLine, st.Sessions[i].Idle = true, "May I drop the old index?", time.Minute
		}
	}
	m := boardShot(t, st, 80, 45, func(m *Model) {
		m.boardChain, m.boardHost, m.filter = "login", "ws-1", "flint"
	})
	f := plainFrame(m)
	if head := strings.Split(f, "\n")[0]; !strings.Contains(head, "● 5 need you") {
		t.Fatalf("the header does not count every wait: %q", head)
	}
	if strings.Contains(f, "hearth-notes") {
		t.Fatalf("the filters show hearth-notes:\n%s", f)
	}
	sc := m.draw(80)
	next, cmd := tap(m, sc.waits.x0, 0)
	drain(cmd)
	m = next.(Model)
	if m.boardChain != "" || m.boardHost != "" || m.filter != "" {
		t.Fatalf("a tap on the count left a filter: chain %q, host %q, search %q", m.boardChain, m.boardHost, m.filter)
	}
	if f := plainFrame(m); !strings.Contains(f, "hearth-notes") {
		t.Errorf("the match on gpu-1 that asks is not on the screen:\n%s", f)
	}
}

// chainsFixture is the board's fixture with a second chain, docs
// (spark-search and spark-docs), and flint-docs, at work in no chain.
func chainsFixture(t *testing.T) Model {
	st := boardFixture(t)
	flintDocs := boardMatch("flint-docs", time.Hour)
	flintDocs.Busy, flintDocs.Status, flintDocs.Agent, flintDocs.LastLine = true, "busy", "claude", "Writing the guide."
	st.Sessions = append(st.Sessions, flintDocs)
	var search, docs sample.Session
	for _, s := range st.Sessions {
		switch s.Name {
		case "spark-search":
			search = s
		case "spark-docs":
			docs = s
		}
	}
	st.Chains.Chains = append(slices.Clone(st.Chains.Chains), chains.Chain{ID: "c2", Name: "docs", Repo: "/home/user/src/spark",
		Members: []chains.Member{memberOf(search), memberOf(docs)}})
	return boardShot(t, st, 80, 31, chooseCard("flint-auth"))
}

func TestSTurnsThroughTheHosts(t *testing.T) {
	m := boardShot(t, boardFixture(t), 40, 34, chooseCard("ember-billing"))
	gpuCard := func(m Model) string {
		for _, l := range strings.Split(plainFrame(m), "\n") {
			if strings.Contains(l, "ember-billing") {
				return l
			}
		}
		return ""
	}
	if l := gpuCard(m); !strings.Contains(l, "@gpu-1") {
		t.Fatalf("with all hosts the card of gpu-1 has no @gpu-1: %q", l)
	}
	home := m.nameOf(m.opt.Host)
	for _, want := range []string{home, "gpu-1", ""} {
		m = pressOn(m, "s")
		if m.boardHost != want {
			t.Fatalf("s turns to %q, want %q", m.boardHost, want)
		}
		switch l := gpuCard(m); want {
		case home:
			if l != "" {
				t.Errorf("the board of %s shows gpu-1's card: %q", home, l)
			}
		case "gpu-1":
			if l == "" || strings.Contains(l, "@gpu-1") {
				t.Errorf("the board of gpu-1 alone: %q, want the card with no @gpu-1", l)
			}
		default:
			if !strings.Contains(l, "@gpu-1") {
				t.Errorf("all hosts again: %q, want @gpu-1", l)
			}
		}
	}
}

// TestAHostThatIsNotReachedKeepsItsRows: its rows stay, faint, and the
// foot names it; ⏎ on the foot's part, then ⏎ in its place, dials it
// again, and a tap on the part opens the place too.
func TestAHostThatIsNotReachedKeepsItsRows(t *testing.T) {
	m := consoleOn(lostPeer(t, 40*time.Second))
	m = upd(t, m, tea.WindowSizeMsg{Width: 160, Height: 34})
	m.toList()
	if !strings.Contains(strings.Join(frameOf(m), "\n"), "train-loop") {
		t.Fatalf("the row of ws-1 is gone:\n%s", strings.Join(frameOf(m), "\n"))
	}
	m.footAt = len(m.footParts())
	opened := pressOn(m, "enter")
	if f := strings.Join(frameOf(opened), "\n"); !strings.Contains(f, "ws-1: not reached since 14:02.") || !strings.Contains(f, "⏎ try again") {
		t.Fatalf("the host's place:\n%s", f)
	}
	next, cmd := key(opened, "enter")
	if msgs := cmdMsgs(cmd); !slices.ContainsFunc(msgs, func(msg tea.Msg) bool { r, ok := msg.(peerRedialMsg); return ok && r.host == "ws-1" }) {
		t.Errorf("⏎ in the host's place does not redial ws-1: %v", msgs)
	}
	if f := next.(Model).flash; !strings.Contains(f, "ws-1") {
		t.Errorf("the redial says %q", f)
	}
	if tapped := tapText(t, m, "! ws-1 not reached"); tapped.sidePlace != placeHost {
		t.Errorf("a tap on the part opens place %d", tapped.sidePlace)
	}
}

// firstCard chooses the first session the list draws, the keys in the
// list.
func firstCard(m Model) Model {
	if order := m.sideOrder(); len(order) > 0 {
		m.sideAt = order[0]
	}
	m.toList()
	return m
}

func TestAWaitOnAHostNotReachedIsNotCounted(t *testing.T) {
	now := lostAt
	m, _, conns := hostsAt(t, &now, "", 104, 30, "ws-1")
	m.splashDone = true
	home := hostState(sess(1, "api-auth", "%1", nil))
	asks := hostState(sess(2, "train-loop", "%2", func(s *sample.Session) { s.Permits = true }))
	m = upd(t, m, stateMsg(stateAt(home, now)))
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(asks)})
	if n := m.needCount(); n != 1 {
		t.Fatalf("with ws-1 reached the count is %d, want 1", n)
	}
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: lostMsg{err: io.EOF, conn: conns["ws-1"]}})
	now = lostAt.Add(40 * time.Second)
	m = upd(t, m, stateMsg(stateAt(home, now)))
	if n := m.needCount(); n != 0 {
		t.Errorf("with ws-1 not reached the count is %d, want 0", n)
	}
	if s := matchOf(t, m, "train-loop"); m.sideStateOf(s) != sideNeed {
		t.Error("the row of ws-1 that asks left NEEDS YOU")
	}
}

func TestEscClearsTheSearchThenTheChain(t *testing.T) {
	now := demoAt
	m := boardShot(t, boardFixture(t), 80, 31, func(m *Model) {
		m.opt.Now = func() time.Time { return now }
		m.boardChain, m.filter = "login", "flint"
		m.refresh()
	})
	for i, want := range []struct {
		chain string
		peek  bool
	}{{"login", true}, {"", true}, {"", false}} {
		was := m.peekAt
		now = now.Add(time.Second)
		m = pressOn(m, "esc")
		if m.filter != "" || m.boardChain != want.chain {
			t.Fatalf("esc %d left search %q and chain %q, want chain %q", i+1, m.filter, m.boardChain, want.chain)
		}
		if moved := !m.peekAt.Equal(was); moved != want.peek || moved && !m.peekAt.Equal(now) {
			t.Errorf("esc %d: the peek is at %v (was %v, now %v)", i+1, m.peekAt, was, now)
		}
	}
}

// TestTheHomeKeyActsAsTheBoardsEsc: outside a stand-in the home key goes
// back as esc does (#360); on the board that clears the search, then the
// chain, and never leaves the board.
func TestTheHomeKeyActsAsTheBoardsEsc(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, func(m *Model) {
		m.opt.Layered = true
		m.boardChain, m.filter = "login", "flint"
		m.refresh()
	})
	home := func(m Model) (Model, []tea.Msg) {
		next, cmd := m.Update(uv.UnknownApcEvent(homeKey))
		return next.(Model), cmdMsgs(cmd)
	}
	left := func(msgs []tea.Msg) bool {
		return slices.ContainsFunc(msgs, func(msg tea.Msg) bool { _, ok := msg.(layerMsg); return ok })
	}
	for i, chain := range []string{"login", "", ""} {
		var msgs []tea.Msg
		m, msgs = home(m)
		if m.filter != "" || m.boardChain != chain {
			t.Fatalf("home key %d left search %q and chain %q, want chain %q", i+1, m.filter, m.boardChain, chain)
		}
		if left(msgs) || !m.boardShown() {
			t.Fatalf("home key %d left the board", i+1)
		}
	}
}

func TestATapOnTheHostWordTurnsTheHost(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, nil)
	sc := m.draw(80) // on the desk the host word is the bar's s▸ (owner, 2026-11-24)
	i := slices.IndexFunc(sc.keys, func(s span) bool { return s.press == "s" })
	if i < 0 || !strings.HasPrefix(ansi.Strip(ansi.Cut(sc.lines[sc.keysAt], sc.keys[i].x0, sc.keys[i].x1)), "s▸") {
		t.Fatalf("no s▸ on the bar: %q", ansi.Strip(sc.lines[sc.keysAt]))
	}
	next, cmd := tap(m, sc.keys[i].x0, sc.keysAt)
	drain(cmd)
	if m = next.(Model); m.boardHost != m.nameOf(m.opt.Host) {
		t.Fatalf("a tap on the host word turns to %q", m.boardHost)
	}
	if bar := barLine(m); !strings.HasPrefix(strings.TrimSpace(bar[strings.Index(bar, "   "):]), m.boardHost+" cpu") || strings.Contains(bar, "hosts s▸") || !strings.Contains(bar, "s▸") {
		t.Errorf("the bar does not say the host: %q", barLine(m))
	}
}

func TestAStaleHomeSaysNoAsOf(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, nil)
	next, _ := m.Update(lostMsg{err: io.EOF, conn: m.conn})
	m = next.(Model)
	m.opt.Now = func() time.Time { return demoAt.Add(time.Minute) }
	if !m.stale(m.opt.Host) {
		t.Fatal("home is not stale")
	}
	if f := plainFrame(m); strings.Contains(f, "as of") {
		t.Errorf("a card of this machine says as of:\n%s", f)
	}
}

func TestSearchNarrowsTheList(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, nil)
	before := drawnNames(m)
	m = pressOn(m, "/", "l", "o", "g")
	if got, want := drawnNames(m), []string{"flint-api", "flint-auth", "flint-ui"}; !slices.Equal(got, want) {
		t.Errorf("a search of log leaves %v, want %v", got, want)
	}
	m = pressOn(m, "enter", "esc")
	if m.filter != "" || !slices.Equal(drawnNames(m), before) {
		t.Errorf("esc keeps the search %q: %v, want %v", m.filter, drawnNames(m), before)
	}
}

// TestTheRoomTurnsTheBoardsChains: ] and [ in a room walk the chains the
// board draws a card of, as on the board, not every room there is.
func TestTheRoomTurnsTheBoardsChains(t *testing.T) {
	m := pressOn(chainsFixture(t), "g")
	if m.room == nil || m.boardChain != "login" {
		t.Fatalf("g on flint-auth: room %v, chain %q", m.room, m.boardChain)
	}
	m.filter = "flint" // docs draws no card now
	for _, k := range []string{"]", "["} {
		if next := pressOn(m, k); next.boardChain != "login" {
			t.Errorf("%s in the room goes to %q: docs draws no card", k, next.boardChain)
		}
	}
}

// drawnNames are the sessions the list draws, as the filters leave them.
func drawnNames(m Model) []string {
	var names []string
	for _, s := range m.sideSessions() {
		if m.inView(s.Host) && m.inBoard(s) && !slices.Contains(names, s.Name) {
			names = append(names, s.Name)
		}
	}
	slices.Sort(names)
	return names
}
