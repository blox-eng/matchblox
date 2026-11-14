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
	"github.com/blox-eng/matchblox/internal/doors"
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

// tapHead taps the head or the page mark of column c, or its section's
// head in the stack at Medium.
func tapHead(t *testing.T, m Model, c boardCol) Model {
	t.Helper()
	w := max(m.width, minWidth)
	d := m.boardPanel(w, m.bodyRows())
	x, y := -1, d.headAt
	if i := slices.IndexFunc(d.heads, func(h span) bool { return h.press == c.short() }); i >= 0 {
		x = d.heads[i].x0
	}
	for line, sps := range d.taps {
		if i := slices.IndexFunc(sps, func(h span) bool { return h.press == c.short() }); x < 0 && i >= 0 {
			x, y = sps[i].x0, line
		}
	}
	if x < 0 {
		t.Fatalf("no head of %s: %+v", c.name(), d.heads)
	}
	next, cmd := tap(m, x, m.draw(w).bodyTop+y)
	drain(cmd)
	return next.(Model)
}

// drawnNames are the matches whose cards the board draws.
func drawnNames(m Model) []string {
	w := max(m.width, minWidth)
	d := m.boardPanel(w, m.bodyRows())
	var names []string
	for _, ss := range m.boardIndex() {
		for _, s := range ss {
			for _, k := range d.cards {
				if k == paneOf(s) && !slices.Contains(names, s.Name) {
					names = append(names, s.Name)
				}
			}
		}
	}
	slices.Sort(names)
	return names
}

func TestATapOnAColumnNameHidesIt(t *testing.T) {
	st := boardFixture(t)
	for _, size := range [][2]int{{80, 31}, {104, 38}} {
		m := tapHead(t, boardShot(t, st, size[0], size[1], nil), colWork)
		if !m.colHidden(colWork) {
			t.Fatalf("%d: a tap on WORKING's name does not hide it", size[0])
		}
		if f := plainFrame(m); !strings.Contains(strings.Split(f, "\n")[1], "+ work 3") {
			t.Fatalf("%d: the tab row has no + work 3:\n%s", size[0], f)
		}
		m = tapText(t, m, "+ work 3")
		if m.colHidden(colWork) || m.boardAt != colWork || chosenName(t, m) != "flint-ui" {
			t.Errorf("%d: a tap on + work 3: hidden %v, open %s, chosen %s", size[0], m.colHidden(colWork), m.boardAt.name(), chosenName(t, m))
		}
	}

	m := tapHead(t, boardShot(t, st, 40, 34, nil), colNeed) // the open page
	if !m.colHidden(colNeed) {
		t.Fatal("on a phone a tap on the open page's mark does not hide it")
	}
	if f := plainFrame(m); !strings.Contains(f, "+ need 4") {
		t.Fatalf("the page marks have no + need 4:\n%s", f)
	}
	m = tapHead(t, m, colShip)
	if m.colHidden(colShip) || m.boardAt != colShip || chosenName(t, m) != "cinder-ci" {
		t.Errorf("a tap on ship's mark: hidden %v, open %s", m.colHidden(colShip), m.boardAt.name())
	}
	m = tapText(t, m, "+ need 4")
	if m.colHidden(colNeed) || m.boardAt != colNeed {
		t.Errorf("a tap on + need 4: hidden %v, open %s", m.colHidden(colNeed), m.boardAt.name())
	}
}

func TestAllHiddenSaysHowToComeBack(t *testing.T) {
	st := boardFixture(t)
	for _, size := range [][2]int{{40, 34}, {80, 31}, {104, 38}, {160, 38}} {
		m := boardShot(t, st, size[0], size[1], func(m *Model) {
			for _, c := range boardCols {
				m.flipHidden(c)
			}
		})
		f := plainFrame(m)
		if !strings.Contains(strings.Join(strings.Fields(f), " "), "Every column is hidden. → or a tap on a + name shows one.") {
			t.Errorf("%d: the body does not say how to come back:\n%s", size[0], f)
		}
		for _, mark := range []string{"+ need 4", "+ work 3", "+ ship 3", "+ quiet 5"} {
			if !strings.Contains(f, mark) {
				t.Errorf("%d: no %s to tap:\n%s", size[0], mark, f)
			}
		}
		chain := boardShot(t, st, size[0], size[1], func(m *Model) {
			m.flipHidden(colWork)
			m.boardChain = "login"
		})
		for _, f := range []Model{m, chain} {
			for i, l := range frameLines(f) {
				if w := ansi.StringWidth(l); w != size[0] {
					t.Errorf("%d: line %d is %d cells: %q", size[0], i, w, ansi.Strip(l))
				}
			}
		}
		if m = tapText(t, m, "+ ship 3"); m.colHidden(colShip) || chosenName(t, m) != "cinder-ci" {
			t.Errorf("%d: a tap on + ship 3 does not show it", size[0])
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
		m.flipHidden(colNeed)
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
	if m.colHidden(colNeed) || m.boardChain != "" || m.boardHost != "" || m.filter != "" {
		t.Fatalf("a tap on the count left a filter: hidden %v, chain %q, host %q, search %q", m.colHidden(colNeed), m.boardChain, m.boardHost, m.filter)
	}
	if m.boardAt != colNeed || chosenName(t, m) != "spark-search" {
		t.Errorf("a tap on the count chose %s in %s, want the first wait", chosenName(t, m), m.boardAt.name())
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

func TestGNarrowsToTheChain(t *testing.T) {
	now := demoAt
	m := chainsFixture(t)
	m.opt.Now = func() time.Time { return now }
	now = now.Add(time.Second)
	m = pressOn(m, "g")
	if m.boardChain != "login" || !m.peekAt.Equal(now) {
		t.Fatalf("g on flint-auth narrows to %q, peek at %v", m.boardChain, m.peekAt)
	}
	if got, want := drawnNames(m), []string{"flint-api", "flint-auth", "flint-ui"}; !slices.Equal(got, want) {
		t.Errorf("g leaves %v, want %v", got, want)
	}
	if row := strings.Split(plainFrame(m), "\n")[1]; !strings.Contains(row, "chain login · 3   esc all") {
		t.Errorf("the tab row has no crumb: %q", row)
	}
	if m = pressOn(m, "]"); m.boardChain != "docs" {
		t.Errorf("] goes to %q, want docs", m.boardChain)
	}
	if got, want := drawnNames(m), []string{"spark-docs", "spark-search"}; !slices.Equal(got, want) {
		t.Errorf("] leaves %v, want %v", got, want)
	}
	if m = pressOn(m, "["); m.boardChain != "login" {
		t.Errorf("[ goes to %q, want login", m.boardChain)
	}
	if m = pressOn(m, "esc"); m.boardChain != "" || !slices.Contains(drawnNames(m), "kindle-web") ||
		!slices.Contains(drawnNames(m), "spark-search") || !slices.Contains(drawnNames(m), "cinder-ci") { // matches of no chain, and of docs
		t.Errorf("esc keeps the chain %q: %v", m.boardChain, drawnNames(m))
	}

	searched := chainsFixture(t)
	searched.filter = "spark"
	if searched = pressOn(searched, "]"); searched.boardChain != "docs" {
		t.Errorf("] with a search of spark goes to %q, want docs: login draws no card", searched.boardChain)
	}
	if hosted := pressOn(func() Model { h := chainsFixture(t); h.boardHost = "gpu-1"; return h }(), "]"); hosted.boardChain != "" {
		t.Errorf("] on gpu-1 alone goes to %q: no chain has a match there", hosted.boardChain)
	}

	m = pressOn(m, "right")
	for chosenName(t, m) != "flint-docs" {
		m = pressOn(m, "down")
	}
	if m = pressOn(m, "g"); m.boardChain != "" || noticeRow(m) != "flint-docs is in no chain" {
		t.Errorf("g on a card in no chain: chain %q, notice %q", m.boardChain, noticeRow(m))
	}
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

func TestAHostThatIsNotReachedKeepsItsCards(t *testing.T) {
	m := lostPeer(t, 40*time.Second)
	m.splashDone = true
	m.viewPrefs.CopyCheckedPhone, m.viewPrefs.CopyCheckedDesk = true, true // the copy check has its own test
	m = upd(t, m, tea.WindowSizeMsg{Width: 40, Height: 34})
	m = openOn(m, "api-auth")
	w := 40
	d := m.boardPanel(w, m.bodyRows())
	plain := plainLines(d.lines)
	host := slices.IndexFunc(plain, func(l string) bool { return strings.HasPrefix(l, " ws-1: not reached since 14:02") })
	if host < 0 || host >= d.headAt || !strings.Contains(strings.Join(plain[host:d.headAt], " "), "⏎ try again") {
		t.Fatalf("the host line is at %d, the columns at %d:\n%s", host, d.headAt, strings.Join(plain, "\n"))
	}
	if wide := m.hostLines(80); len(wide) != 1 || strings.TrimSpace(ansi.Strip(wide[0])) != "ws-1: not reached since 14:02 · ⏎ try again" {
		t.Errorf("the host line at 80: %q", wide)
	}
	card := slices.IndexFunc(plain, func(l string) bool { return strings.Contains(l, "train-loop") })
	if card < 0 {
		t.Fatalf("the card of ws-1 is gone:\n%s", strings.Join(plain, "\n"))
	}
	if !strings.Contains(plain[card], "as of 14:02") {
		t.Errorf("the card of ws-1 says no time: %q", plain[card])
	}
	if want := m.st.faint.Render(plain[card]); !strings.Contains(d.lines[card], want) {
		t.Errorf("the card of ws-1 is not faint: %q", d.lines[card])
	}
	next, cmd := tap(m, 2, m.draw(w).bodyTop+host)
	msgs := cmdMsgs(cmd)
	if !slices.ContainsFunc(msgs, func(msg tea.Msg) bool { r, ok := msg.(peerRedialMsg); return ok && r.host == "ws-1" }) {
		t.Errorf("a tap on the host line does not redial ws-1: %v", msgs)
	}
	if f := next.(Model).flash; !strings.Contains(f, "ws-1") {
		t.Errorf("the redial says %q", f)
	}

	m = firstCard(m)
	m = pressOn(m, "up")
	if line, above := m.aboveChosen(); !above || line != "ws-1" {
		t.Fatalf("up from the first card chose %q, %v; want the host line", line, above)
	}
	next, cmd = key(m, "enter")
	if msgs := cmdMsgs(cmd); !slices.ContainsFunc(msgs, func(msg tea.Msg) bool { r, ok := msg.(peerRedialMsg); return ok && r.host == "ws-1" }) {
		t.Errorf("enter on the host line does not redial ws-1: %v", msgs)
	}
	if f := next.(Model).flash; !strings.Contains(f, "ws-1") {
		t.Errorf("the redial by enter says %q", f)
	}
}

// firstCard chooses the first card of the open column.
func firstCard(m Model) Model {
	b, _ := m.boardDrawOf()
	m.boardChooseCard(b.open, b.idx[b.open][0])
	return m
}

// TestTheLinesAboveTheColumnsTakeTheArrows: up from the first card
// chooses the doors line, then the host line; down goes back down to the
// first card. A chosen line has the wash, and no card is chosen while it
// is.
func TestTheLinesAboveTheColumnsTakeTheArrows(t *testing.T) {
	m := lostPeer(t, 40*time.Second)
	m.splashDone = true
	m.viewPrefs.CopyCheckedPhone, m.viewPrefs.CopyCheckedDesk = true, true // the copy check has its own test
	m.doors = []doors.Door{{ID: doors.Tmux, Title: "Install tmux", Term: []string{"sh", "-c", "true"}}}
	m = firstCard(upd(t, m, tea.WindowSizeMsg{Width: 80, Height: 31}))
	first := chosenName(t, m)
	chosen := func(m Model) string {
		line, above := m.aboveChosen()
		if !above {
			return "card"
		}
		return "line " + line
	}
	for _, c := range []struct{ key, want string }{
		{"up", "line "}, {"up", "line ws-1"}, {"up", "line ws-1"}, {"down", "line "}, {"down", "card"},
	} {
		if m = pressOn(m, c.key); chosen(m) != c.want {
			t.Fatalf("%s chose %s, want %s", c.key, chosen(m), c.want)
		}
	}
	if chosenName(t, m) != first {
		t.Errorf("down from the doors line chose %s, want %s", chosenName(t, m), first)
	}

	m = pressOn(m, "up", "up")
	if _, ok := m.boardSelected(); ok {
		t.Error("a card is chosen with the host line")
	}
	d := m.boardPanel(80, m.bodyRows())
	if host := slices.IndexFunc(plainLines(d.lines), func(l string) bool { return strings.Contains(l, "ws-1: not reached") }); host < 0 ||
		!strings.Contains(d.lines[host], washOn(m)) || !strings.HasPrefix(plainLines(d.lines)[host], "▌") {
		t.Errorf("the chosen host line is not drawn as a chosen row: %q", d.lines[max(host, 0)])
	}
	if m = pressOn(m, "1"); m.peekArmed != 0 || len(boardFake(m).acts()) != 0 {
		t.Errorf("a digit on the host line marked %d, sent %v", m.peekArmed, boardFake(m).acts())
	}
	if m = pressOn(m, "x"); m.pending != nil || noticeRow(m) != "no card is chosen · ↓ goes back to the cards" {
		t.Errorf("x on the host line: pending %v, notice %q", m.pending, noticeRow(m))
	}
	if m = pressOn(m, "esc"); chosen(m) != "card" || chosenName(t, m) != first {
		t.Errorf("esc on the host line: %s", chosen(m))
	}

	m = pressOn(m, "up")
	next, _ := key(m, "enter")
	if p := next.(Model).pending; p == nil || p.door != doors.Tmux {
		t.Errorf("enter on the doors line opens %+v, want the door's step", p)
	}
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
	if len(m.boardColumn(colNeed)) != 1 {
		t.Error("the card of ws-1 that asks left NEEDS YOU")
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
	sc := m.draw(80)
	if word := ansi.Strip(ansi.Cut(sc.lines[0], sc.hostAt.x0, sc.hostAt.x1)); sc.hostAt.press == "" || !strings.HasPrefix(word, "all hosts") {
		t.Fatalf("the host word is at %+v: %q", sc.hostAt, word)
	}
	next, cmd := tap(m, sc.hostAt.x0+1, 0)
	drain(cmd)
	if m = next.(Model); m.boardHost != m.nameOf(m.opt.Host) {
		t.Fatalf("a tap on the host word turns to %q", m.boardHost)
	}
	if head := ansi.Strip(m.draw(80).lines[0]); !strings.Contains(head, m.boardHost+" ▾") {
		t.Errorf("the header does not say the host: %q", head)
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

// openOn opens the QUIET page on name, a quiet match of this machine.
func openOn(m Model, name string) Model {
	chooseCard(name)(&m)
	return m
}

func TestSearchNarrowsEveryColumn(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, nil)
	before := drawnNames(m)
	m = pressOn(m, "/", "l", "o", "g")
	if got, want := drawnNames(m), []string{"flint-api", "flint-auth", "flint-ui"}; !slices.Equal(got, want) {
		t.Errorf("a search of log leaves %v, want %v", got, want)
	}
	f := plainFrame(m)
	for _, head := range []string{"NEEDS YOU · 1", "WORKING · 1", "SHIPPING · 1", "QUIET · 0"} {
		if !strings.Contains(f, head) {
			t.Errorf("no %s:\n%s", head, f)
		}
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
