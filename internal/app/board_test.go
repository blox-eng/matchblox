package app

import (
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

func TestBoardWidths(t *testing.T) {
	all := boardCols[:]
	for _, c := range []struct {
		l     Layout
		w     int
		shown []boardCol
		cols  []int
		live  int
	}{
		{Medium, 80, all, nil, 0}, // the sections stack at the full width (TestMediumStacksTheSections)
		{Wide, 104, all, []int{36, 24, 24, 17}, 0},
		{Wide, 151, all, []int{53, 35, 35, 25}, 0},
		{Wide, 152, all, []int{36, 25, 25, 18}, 44},
		{Wide, 160, all, []int{36, 25, 25, 18}, 52},
		{Wide, 160, []boardCol{colNeed, colShip, colQuiet}, []int{36, 25, 18}, 78},
		{Narrow, 40, all, []int{40}, 0},
	} {
		cols, live := boardWidths(c.l, c.w, c.shown)
		if !slices.Equal(cols, c.cols) || live != c.live {
			t.Errorf("boardWidths(%v, %d, %v) = %v, %d; want %v, %d", c.l, c.w, c.shown, cols, live, c.cols, c.live)
		}
		if c.cols == nil {
			continue
		}
		sum := live
		for _, x := range cols {
			sum += x
		}
		gaps := len(cols) - 1
		if live > 0 {
			gaps++
		}
		if sum+gaps != c.w {
			t.Errorf("boardWidths(%v, %d, %v): %v and %d with %d gaps do not fill the width", c.l, c.w, c.shown, cols, live, gaps)
		}
	}
}

// boardFixture is the board's machine as the sketch draws it: fifteen
// matches of the demo fleet, four that need you, three at work, three that
// ship and five quiet; flint-auth, flint-ui and flint-api in the chain
// login; ember-billing, cinder-ci, hearth-notes and tinder-scratch on a
// second host, gpu-1. A match of screensState keeps its pane, agent and
// folder.
func boardFixture(t testing.TB) proto.State {
	st := screensState(t)
	demo := map[string]sample.Session{}
	for _, s := range st.Sessions {
		demo[s.Name] = s
	}
	match := func(name, host, agent string, ctx float64, idle time.Duration, edit func(*sample.Session)) sample.Session {
		s := boardMatch(name, 3*time.Hour)
		if d, ok := demo[name]; ok {
			d.Busy, d.Status, d.Out, d.Started = false, "idle", time.Time{}, s.Started
			d.Shells, d.Agents, d.Asks, d.Permits, d.Ask, d.Progress, d.Do, d.Said = nil, 0, false, false, nil, nil, "", ""
			s = d
		}
		s.Host, s.Agent, s.Context, s.ContextPct, s.Idle = host, agent, "known", ctx, idle
		if edit != nil {
			edit(&s)
		}
		return s
	}
	busy := func(line string) func(*sample.Session) {
		return func(s *sample.Session) { s.Busy, s.Status, s.LastLine = true, "busy", line }
	}
	says := func(line string) func(*sample.Session) {
		return func(s *sample.Session) { s.LastLine = line }
	}
	auth := match("flint-auth", "", "claude", 62, 2*time.Minute, func(s *sample.Session) {
		s.LastLine = "I can fix it in two ways. The token check is a small patch."
		s.Ask = &said.Ask{ID: "toolu_board", Questions: []said.Question{{Question: "Which way do we fix the login?",
			Options: []said.Option{{Label: "Patch the token check"}, {Label: "Move to the new session store"}}}}}
	})
	kindle := match("kindle-web", "", "claude", 48, 9*time.Minute, says("Every check passed on #4556. I did not merge: that is yours."))
	hearth := match("hearth-api", "", "claude", 31, 4*time.Minute, func(s *sample.Session) {
		s.Permits, s.LastLine = true, "The plan changes 2 records and removes none."
	})
	search := match("spark-search", "", "claude", 77, 31*time.Minute, says("Lint fails on a file I did not change. I tried twice and stopped."))
	ui := match("flint-ui", "", "claude", 40, 0, busy("Writing the test for a token that expires mid-request."))
	billing := match("ember-billing", "gpu-1", "claude", 71, 0, func(s *sample.Session) {
		s.Busy, s.Status, s.Agents, s.LastLine = true, sample.StatusAgents, 3, "Three subagents read the invoice paths."
	})
	docs := match("spark-docs", "", "claude", 22, 0, busy("Running the suite."))
	limits := match("hearth-limits", "", "claude", 55, 4*time.Minute, says("Pushed. CI runs."))
	api := match("flint-api", "", "claude", 44, 40*time.Minute, says("Opened #4560 and asked for a review."))
	cinder := match("cinder-ci", "gpu-1", "codex", 37, 2*time.Hour, says("Merged. The deploy runs."))
	notes := match("hearth-notes", "gpu-1", "claude", 18, time.Hour, says("Done."))
	reports := match("ember-reports", "", "cursor", 26, 3*time.Hour, says("Nothing left on my list."))
	wick := match("wick-mobile", "", "opencode", 33, 50*time.Minute, func(s *sample.Session) {
		s.Out, s.LastLine = demoAt.Add(time.Hour), "The plan limit stopped me."
	})
	legacy := match("kindle-legacy", "", "claude", 52, 5*time.Hour, says("Parked."))
	scratch := match("tinder-scratch", "gpu-1", "aider", 9, 2*24*time.Hour, says("Idle since Sunday."))
	ui.Started, billing.Started, docs.Started = demoAt.Add(-3*time.Hour), demoAt.Add(-2*time.Hour), demoAt.Add(-time.Hour)

	st.Sessions = []sample.Session{auth, kindle, hearth, search, ui, billing, docs, limits, api, cinder, notes, reports, wick, legacy, scratch}
	st.Children, st.IdlePanes, st.Exited = nil, nil, nil
	st.Queue = []queue.Item{waitFor(auth, queue.StateQuestion, 2*time.Minute), waitFor(hearth, queue.StatePermission, 4*time.Minute)}
	ship := func(s sample.Session, n int, kind, note string, stage int, ago time.Duration) proto.Ship {
		sh := shipFor(s, n, kind, note, stage, ago)
		sh.Host = s.Host
		return sh
	}
	waits := ship(kindle, 4556, shipWaits, "green · waits for your merge", 2, 9*time.Minute)
	waits.Merge = []string{"gh", "pr", "merge", "4556", "--squash", "-R", waits.Repo}
	st.Ships = &proto.Ships{At: demoAt, Ships: []proto.Ship{
		waits,
		ship(search, 4548, shipStuck, "CI failed · lint, twice", 1, 31*time.Minute),
		ship(limits, 4551, shipRunning, "CI · 6 of 9 jobs", 1, 4*time.Minute),
		ship(api, 4560, shipRunning, "waits for review", 1, 40*time.Minute),
		ship(cinder, 4549, shipRunning, "deploying", 3, 2*time.Hour),
	}}
	st.Holds = []proto.Hold{{SessionID: legacy.SessionID, Pane: legacy.Pane, Name: legacy.Name, By: holdByYou}}
	st.Chains = &state.ChainsView{Chains: []chains.Chain{{ID: "c1", Name: "login", Repo: "/home/user/src/hearth",
		Members: []chains.Member{memberOf(auth), memberOf(ui), memberOf(api)}}}}
	return st
}

// boardShot is the console at w by h with the board on, on the clock of
// demoAt: st's matches of this machine, and gpu-1's when st has some. edit
// changes the model before the frame is drawn.
func boardShot(t testing.TB, st proto.State, w, h int, edit func(*Model)) Model {
	t.Helper()
	home, gpu := st, st
	home.Sessions, home.Queue, gpu.Sessions, gpu.Queue = nil, nil, nil, nil
	home.Ships, gpu.Ships = &proto.Ships{At: demoAt}, &proto.Ships{At: demoAt}
	gpu.Holds, gpu.Chains, gpu.Recommendations = nil, nil, nil
	for _, s := range st.Sessions {
		if s.Host == "gpu-1" {
			s.Host = ""
			gpu.Sessions = append(gpu.Sessions, s)
		} else {
			home.Sessions = append(home.Sessions, s)
		}
	}
	for _, it := range st.Queue {
		if it.Host == "gpu-1" {
			it.Host = ""
			gpu.Queue = append(gpu.Queue, it)
		} else {
			home.Queue = append(home.Queue, it)
		}
	}
	if st.Ships != nil {
		for _, sh := range st.Ships.Ships {
			if sh.Host == "gpu-1" {
				sh.Host = ""
				gpu.Ships.Ships = append(gpu.Ships.Ships, sh)
			} else {
				home.Ships.Ships = append(home.Ships.Ships, sh)
			}
		}
	}
	opt := Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }}
	peer := newFake()
	if len(gpu.Sessions) > 0 {
		opt.Peers = []Peer{{Host: "gpu-1", Conn: peer}}
	}
	var next tea.Model = New(opt)
	next, _ = next.Update(tea.WindowSizeMsg{Width: w, Height: h})
	next, _ = next.Update(stateMsg(home))
	if len(gpu.Sessions) > 0 {
		next, _ = next.Update(peerMsg{host: "gpu-1", conn: peer, msg: stateMsg(gpu)})
	}
	m := next.(Model)
	m.splashDone = true
	// The screens are checked: the copy check's door has its own test.
	m.viewPrefs.CopyCheckedPhone, m.viewPrefs.CopyCheckedDesk = true, true
	if edit != nil {
		edit(&m)
	}
	return m
}

// chooseCard picks the card of the match named name, in its column.
func chooseCard(name string) func(*Model) {
	return func(m *Model) {
		idx := m.boardIndex()
		for _, c := range boardCols {
			for _, s := range idx[c] {
				if s.Name == name {
					m.boardAt, m.boardPick = c, map[boardCol]paneKey{c: paneOf(s)}
					return
				}
			}
		}
		panic(name + " is on no column")
	}
}

func oneLineBoard(m *Model) {
	yes := true
	m.viewPrefs.OneLinePhone, m.viewPrefs.OneLineDesk = &yes, true
}

func frameLines(m Model) []string { return strings.Split(m.render(), "\n") }

func plainFrame(m Model) string { return ansi.Strip(m.render()) }

func TestBoardFitsItsScreen(t *testing.T) {
	st := boardFixture(t)
	for _, size := range [][2]int{{40, 34}, {80, 31}, {104, 38}, {160, 38}} {
		for _, one := range []bool{false, true} {
			var edit func(*Model)
			if one {
				edit = oneLineBoard
			}
			m := boardShot(t, st, size[0], size[1], edit)
			lines := frameLines(m)
			if len(lines) != size[1] {
				t.Errorf("%dx%d, one line %v: %d lines", size[0], size[1], one, len(lines))
			}
			for i, l := range lines {
				if lipgloss.Width(l) != size[0] {
					t.Errorf("%dx%d, one line %v: line %d is %d cells: %q", size[0], size[1], one, i, lipgloss.Width(l), ansi.Strip(l))
				}
			}
		}
	}
}

// twelveWaits is a machine where twelve matches wait, the oldest first.
func twelveWaits() proto.State {
	names := []string{"hearth-api", "flint-auth", "spark-docs", "kindle-web", "ember-billing", "cinder-ci",
		"wick-mobile", "hearth-limits", "flint-ui", "spark-search", "ember-reports", "kindle-legacy"}
	var ss []sample.Session
	var q []queue.Item
	for i, n := range names {
		s := boardMatch(n, time.Hour)
		s.Permits, s.LastLine = true, "May I run the migration on the test database?"
		ss = append(ss, s)
		q = append(q, waitFor(s, queue.StatePermission, time.Duration(20-i)*time.Minute))
	}
	st := boardState(ss...)
	st.Queue = q
	return st
}

func TestBoardKeepsTheChosenCardWhole(t *testing.T) {
	m := boardShot(t, twelveWaits(), 40, 34, chooseCard("flint-ui")) // the 9th
	d := m.boardPanel(40, m.bodyRows())
	plain := plainLines(d.lines)
	col := slices.Index(plain, strings.Repeat("─", 40)) + 1 // under the page marks
	if col == 0 {
		t.Fatalf("no hairline under the page marks:\n%s", strings.Join(plain, "\n"))
	}
	shown := map[paneKey]int{}
	var order []paneKey
	for i := range plain {
		if k, ok := d.cards[boardSpot{i, colNeed}]; ok {
			if shown[k] == 0 {
				order = append(order, k)
			}
			shown[k]++
		}
	}
	idx := m.boardIndex()[colNeed]
	chosen := paneOf(idx[8])
	lines, _ := m.boardCard(idx[8], 40, cardOpts{sel: true, need: true})
	if shown[chosen] != len(lines) {
		t.Errorf("the chosen card shows %d of its %d lines:\n%s", shown[chosen], len(lines), strings.Join(plain, "\n"))
	}
	first := slices.IndexFunc(idx, func(s sample.Session) bool { return paneOf(s) == order[0] })
	last := slices.IndexFunc(idx, func(s sample.Session) bool { return paneOf(s) == order[len(order)-1] })
	if want := "  ↑ " + strconv.Itoa(first) + " more"; first == 0 || strings.TrimRight(plain[col], " ") != want {
		t.Errorf("the column starts %q, want %q", plain[col], want)
	}
	end := col
	for i := col; i < len(plain); i++ {
		if strings.HasPrefix(plain[i], "  ↓ ") {
			end = i
		}
	}
	if want := "  ↓ " + strconv.Itoa(len(idx)-1-last) + " more"; last == len(idx)-1 || strings.TrimRight(plain[end], " ") != want {
		t.Errorf("the column ends %q, want %q", plain[end], want)
	}
	for i := end + 1; i < len(plain); i++ {
		if _, ok := d.cards[boardSpot{i, colNeed}]; ok {
			t.Errorf("a card under the ↓ line: %q", plain[i])
		}
	}
}

func TestEmptyNeedsYouSaysSo(t *testing.T) {
	st := boardFixture(t)
	noNeed, noWork := st, st
	noNeed.Sessions = slices.DeleteFunc(slices.Clone(st.Sessions), func(s sample.Session) bool {
		return slices.Contains([]string{"flint-auth", "kindle-web", "hearth-api", "spark-search"}, s.Name)
	})
	noWork.Sessions = slices.DeleteFunc(slices.Clone(st.Sessions), func(s sample.Session) bool {
		return slices.Contains([]string{"flint-ui", "ember-billing", "spark-docs"}, s.Name)
	})
	for _, w := range []int{80, 104} {
		if f := plainFrame(boardShot(t, noNeed, w, 38, nil)); !strings.Contains(f, "nothing needs you") || !strings.Contains(f, "NEEDS YOU · 0") {
			t.Errorf("w=%d: an empty NEEDS YOU does not say so:\n%s", w, f)
		}
		m := boardShot(t, noWork, w, 38, nil)
		if m.layoutAt(w) == Medium { // the stack: WORKING keeps its head and its rows
			_, plain, at := sectionHeads(t, m)
			_, _, full := sectionHeads(t, boardShot(t, st, w, 38, nil))
			if at[colWork] < 0 || !strings.HasPrefix(plain[at[colWork]], " WORKING · 0  −") || !maps.Equal(at, full) {
				t.Errorf("w=%d: an empty WORKING does not keep its head and rows: %v, want %v\n%s", w, at, full, strings.Join(plain, "\n"))
			}
			continue
		}
		d := m.boardPanel(w, m.bodyRows())
		head := ansi.Strip(d.lines[d.headAt])
		if !strings.Contains(head, "WORKING · 0") {
			t.Errorf("w=%d: an empty WORKING has no head: %q", w, head)
		}
		cols, _ := boardWidths(m.layoutAt(w), w, boardCols[:])
		if len(d.heads) < 2 || d.heads[1].x0 != cols[0]+1 || d.heads[1].x1-d.heads[1].x0 != cols[1] {
			t.Errorf("w=%d: an empty WORKING does not keep its width %d: %+v", w, cols[1], d.heads)
		}
	}
}

func TestTheBoardsStates(t *testing.T) {
	st := boardFixture(t)
	t.Run("before the first state", func(t *testing.T) {
		var next tea.Model = New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
		next, _ = next.Update(tea.WindowSizeMsg{Width: 80, Height: 31})
		m := next.(Model)
		m.splashDone = true
		// The tab row and the key line are not the board's yet: its body.
		f := strings.Join(plainLines(m.boardPanel(80, m.bodyRows()).lines), "\n")
		if !strings.Contains(f, "reading your matches…") || !strings.Contains(f, "NEEDS YOU") {
			t.Errorf("no heads and no reading line:\n%s", f)
		}
		if strings.Contains(f, "0") || strings.Contains(f, "nothing needs you") {
			t.Errorf("a count before the first state:\n%s", f)
		}
		golden(t, m, "boardreading", 80, 31)
	})
	t.Run("no session", func(t *testing.T) {
		none := st
		none.Sessions, none.Queue, none.Ships, none.Holds, none.Chains = nil, nil, nil, nil, nil
		m := boardShot(t, none, 80, 31, nil)
		d := m.boardPanel(80, m.bodyRows())
		plain := plainLines(d.lines)
		if f := strings.Join(plain, "\n"); strings.Contains(f, "NEEDS YOU") || strings.Contains(f, "WORKING") {
			t.Errorf("columns with no session:\n%s", f)
		}
		if !strings.HasPrefix(plain[0], "▌+ start a new session") {
			t.Errorf("the ghost row does not lead with the cursor: %q", plain[0])
		}
		golden(t, m, "boardnone", 80, 31)
	})
	t.Run("the service is lost", func(t *testing.T) {
		m := boardShot(t, st, 80, 31, nil)
		if n := m.boardStateNotice(); n != "" {
			t.Errorf("a notice with the service up: %q", n)
		}
		next, _ := m.Update(lostMsg{err: io.EOF, conn: m.conn})
		m = next.(Model)
		if n := m.boardStateNotice(); n != "no service · it redials" {
			t.Errorf("the notice: %q", n)
		}
		f := plainFrame(m)
		if !strings.Contains(f, "spark-search") {
			t.Errorf("the board does not stay:\n%s", f)
		}
		if lines := m.boardPanel(80, m.bodyRows()).lines; strings.Contains(strings.Join(lines, "\n"), m.st.accent.Render("▌")) {
			t.Errorf("a lost board keeps its accent")
		}
		golden(t, m, "boardlost", 80, 31)
	})
	t.Run("a search with no match", func(t *testing.T) {
		m := boardShot(t, st, 80, 31, func(m *Model) {
			m.filter = "redis"
			m.refresh()
		})
		d := m.boardPanel(80, m.bodyRows())
		f := strings.Join(plainLines(d.lines), "\n")
		if !strings.Contains(f, `nothing matches "redis" · esc clears`) {
			t.Errorf("no line for the search:\n%s", f)
		}
		for _, c := range []boardCol{colNeed, colWork, colShip} {
			if !strings.Contains(f, c.name()+" · 0") {
				t.Errorf("%s does not keep its name with · 0:\n%s", c.name(), f)
			}
		}
		if strings.Contains(f, "nothing needs you") {
			t.Errorf("a search says nothing needs you:\n%s", f)
		}
		golden(t, m, "boardsearch", 80, 31)
	})
}

func TestNoRepeatBesideTheLiveView(t *testing.T) {
	st := boardFixture(t)
	for _, c := range []struct {
		w      int
		repeat bool
	}{{160, false}, {151, true}} {
		m := boardShot(t, st, c.w, 38, chooseCard("flint-auth"))
		d := m.boardPanel(c.w, m.bodyRows())
		chosen := paneOf(m.boardIndex()[colNeed][3])
		var card []string
		for i, l := range plainLines(d.lines) {
			if d.cards[boardSpot{i, colNeed}] == chosen {
				card = append(card, l)
			}
		}
		f := strings.Join(card, "\n")
		if got := strings.Contains(f, "“I can fix it") && strings.Contains(f, "1. Patch the token check"); got != c.repeat {
			t.Errorf("w=%d: the chosen card has its quote and options: %v, want %v\n%s", c.w, got, c.repeat, f)
		}
		if !strings.Contains(f, "flint-auth") {
			t.Errorf("w=%d: flint-auth is not the chosen card:\n%s", c.w, f)
		}
	}
}

func TestQuietIsADrawerOnMedium(t *testing.T) {
	st := boardFixture(t)
	m := boardShot(t, st, 80, 31, nil)
	plain := plainLines(m.boardPanel(80, m.bodyRows()).lines)
	var at []int
	for i, l := range plain {
		if strings.Contains(l, "hearth-notes") || strings.Contains(l, "tinder-scratch") {
			at = append(at, i)
		}
	}
	if len(at) != 1 || !strings.HasPrefix(plain[at[0]], " QUIET · 5 ") {
		t.Errorf("QUIET is not one line with the names:\n%s", strings.Join(plain, "\n"))
	}

	m = boardShot(t, st, 80, 31, chooseCard("ember-reports"))
	plain = plainLines(m.boardPanel(80, m.bodyRows()).lines)
	head := slices.IndexFunc(plain, func(l string) bool { return strings.HasPrefix(l, " QUIET · 5  −") })
	ghost := slices.IndexFunc(plain, func(l string) bool { return strings.Contains(l, "+ start a new session") })
	if head < 0 || ghost-head-1 != 6 {
		t.Fatalf("the open drawer is not six rows under its head:\n%s", strings.Join(plain, "\n"))
	}
	for _, name := range []string{"ember-reports", "wick-mobile", "kindle-legacy", "hearth-notes", "tinder-scratch"} {
		if !slices.ContainsFunc(plain[head+1:ghost], func(l string) bool { return strings.Contains(l, name) }) {
			t.Errorf("the open drawer has no %s:\n%s", name, strings.Join(plain[head:ghost], "\n"))
		}
	}
	if _, _, at := sectionHeads(t, m); slices.Contains(slices.Collect(maps.Values(at)), -1) {
		t.Errorf("with the drawer open a section has no head: %v\n%s", at, strings.Join(plain, "\n"))
	}
}

func TestTheLiveViewFollowsTheChosenCard(t *testing.T) {
	st := boardFixture(t)
	screen := func(name string) func(*Model) {
		return func(m *Model) {
			chooseCard(name)(m)
			m.screen = proto.Screen{Pane: paneNamed(st, "flint-auth"), At: demoAt, Text: "the pane of flint-auth\n"}
		}
	}
	m := boardShot(t, st, 160, 38, screen("flint-auth"))
	cols, live := boardWidths(Wide, 160, boardCols[:])
	right := 160 - live
	d := m.boardPanel(160, m.bodyRows())
	side := func(d boardDrawn) string {
		var out []string
		for _, l := range plainLines(d.lines) {
			if r := []rune(l); len(r) > right {
				out = append(out, string(r[right:]))
			}
		}
		return strings.Join(out, "\n")
	}
	s := m.boardIndex()[colNeed][3]
	if r := side(d); !strings.Contains(r, m.viewTitle(s)) || !strings.Contains(r, "the pane of flint-auth") {
		t.Errorf("the live view does not show flint-auth:\n%s", r)
	}

	// The screen comes from the service's watch, which follows the chosen
	// card and not the selection of Sessions.
	m = boardShot(t, st, 160, 38, chooseCard("flint-auth"))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 38})
	next, _ = next.Update(screenMsg{Pane: paneNamed(st, "flint-auth"), At: demoAt, Text: "the pane of flint-auth\n"})
	m = next.(Model)
	if r := side(m.boardPanel(160, m.bodyRows())); m.watching != paneNamed(st, "flint-auth") || !strings.Contains(r, "the pane of flint-auth") {
		t.Errorf("the live view watches %q, not flint-auth:\n%s", m.watching, r)
	}

	m = boardShot(t, st, 160, 38, screen("hearth-api")) // ↓ from flint-auth
	d = m.boardPanel(160, m.bodyRows())
	s = m.boardIndex()[colNeed][2]
	if r := side(d); !strings.Contains(r, m.viewTitle(s)) || strings.Contains(r, "the pane of flint-auth") || strings.Contains(r, "flint-auth ·") {
		t.Errorf("the live view does not follow the card to hearth-api:\n%s", r)
	}

	m = boardShot(t, st, 151, 38, screen("flint-auth"))
	d = m.boardPanel(151, m.bodyRows())
	if f := strings.Join(plainLines(d.lines), "\n"); strings.Contains(f, "the pane of flint-auth") || strings.Contains(f, "│") {
		t.Errorf("a live view at 151 columns:\n%s", f)
	}
	if n := len(d.heads); n != 4 || d.heads[n-1].x1 != 151 {
		t.Errorf("at 151 columns the columns do not take the width: %+v (of %v)", d.heads, cols)
	}
}

// needStack draws NEEDS YOU of boardFixture, 38 wide and h tall, with
// name chosen: its lines, and the lines of each card in the column.
func needStack(t *testing.T, name string, h int) (plain []string, cards [][]string) {
	t.Helper()
	m := boardShot(t, boardFixture(t), 80, 31, chooseCard(name))
	b := boardDraw{idx: m.boardIndex()}
	m.boardChoose(&b, boardCols[:])
	for _, s := range b.idx[colNeed] {
		bl := m.boardBlockOf(&b, colNeed, s, 38, false)
		cards = append(cards, plainLines(bl.lines[:bl.n]))
	}
	d := boardDrawn{cards: map[boardSpot]paneKey{}, taps: map[int][]span{}}
	m.boardStack(&d, &b, colNeed, 0, 0, 38, h, false)
	return plainLines(d.lines), cards
}

func TestTheLastCardTakesNoBlankRow(t *testing.T) {
	_, cards := needStack(t, "spark-search", 30)
	h := len(cards) - 1 // a blank between two cards
	for _, c := range cards {
		h += len(c)
	}
	plain, _ := needStack(t, "spark-search", h)
	f := strings.Join(plain, "\n")
	if strings.Contains(f, "↓") || !strings.Contains(f, strings.Join(cards[len(cards)-1], "\n")) {
		t.Errorf("a last card that fits exactly is not whole in %d rows:\n%s", h, f)
	}

	// flint-auth, the last, chosen: the ↑ line, hearth-api and a blank, flint-auth.
	_, cards = needStack(t, "flint-auth", 30)
	last := len(cards) - 1
	h = 1 + len(cards[last-1]) + 1 + len(cards[last])
	plain, _ = needStack(t, "flint-auth", h)
	if got := strings.TrimRight(plain[0], " "); got != "  ↑ 2 more" {
		t.Errorf("the chosen last card scrolls to %q, want ↑ 2 more:\n%s", got, strings.Join(plain, "\n"))
	}
	if f := strings.Join(plain, "\n"); !strings.Contains(f, strings.Join(cards[last], "\n")) || strings.Contains(f, "↓") {
		t.Errorf("the chosen last card is not whole:\n%s", f)
	}
}

func TestASearchIsSaidAsTyped(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, func(m *Model) {
		m.filter = `say "hi"`
		m.refresh()
	})
	f := strings.Join(plainLines(m.boardPanel(80, m.bodyRows()).lines), "\n")
	if !strings.Contains(f, `nothing matches "say "hi"" · esc clears`) {
		t.Errorf("the search is not said as typed:\n%s", f)
	}
}

func TestAHiddenColumnGivesItsRoom(t *testing.T) {
	for _, size := range [][2]int{{80, 31}, {160, 38}} {
		w := size[0]
		m := boardShot(t, boardFixture(t), w, size[1], func(m *Model) { m.flipHidden(colWork) })
		d := m.boardPanel(w, m.bodyRows())
		for i, l := range d.lines {
			if lipgloss.Width(l) != w {
				t.Errorf("w=%d: line %d is %d cells: %q", w, i, lipgloss.Width(l), ansi.Strip(l))
			}
		}
		f := strings.Join(plainLines(d.lines), "\n")
		for _, name := range []string{"WORKING", "flint-ui", "spark-docs", "ember-billing"} {
			if strings.Contains(f, name) {
				t.Errorf("w=%d: the hidden column draws %s:\n%s", w, name, f)
			}
		}
		if m.layoutAt(w) == Medium { // the stack: SHIPPING takes WORKING's rows, and every ship shows
			_, plain, at := sectionHeads(t, m)
			if at[colShip] < 0 || at[colWork] >= 0 {
				t.Fatalf("w=%d: the heads are %v:\n%s", w, at, f)
			}
			for _, name := range []string{"cinder-ci", "flint-api", "hearth-limits"} {
				if !slices.ContainsFunc(plain[at[colShip]:], func(l string) bool { return strings.Contains(l, name) }) {
					t.Errorf("w=%d: SHIPPING does not take the room: no %s\n%s", w, name, f)
				}
			}
			continue
		}
		shown := []boardCol{colNeed, colShip, colQuiet}
		cols, live := boardWidths(m.layoutAt(w), w, shown)
		full, _ := boardWidths(m.layoutAt(w), w, boardCols[:])
		if len(d.heads) != len(cols) || d.heads[0].x1 != cols[0] {
			t.Errorf("w=%d: heads %+v, want widths %v", w, d.heads, cols)
		}
		if live == 0 && cols[0] <= full[0] {
			t.Errorf("w=%d: NEEDS YOU does not take the room: %v, was %v", w, cols, full)
		}
	}
}

func TestTheEmptyBoardOffersOnlyWhatActs(t *testing.T) {
	none := boardFixture(t)
	none.Sessions, none.Queue, none.Ships, none.Holds, none.Chains = nil, nil, nil, nil, nil
	for _, size := range [][2]int{{40, 34}, {80, 31}, {160, 38}} {
		m := boardShot(t, none, size[0], size[1], nil)
		if got := keyLineOf(m); got != "⏎ new session   u strip   tab next" {
			t.Errorf("%dx%d: the empty board offers %q", size[0], size[1], got)
		}
	}
}

func TestTheLiveViewSaysTheCardsWord(t *testing.T) {
	m := boardShot(t, boardFixture(t), 160, 38, chooseCard("spark-search"))
	word, _ := m.cardSays(matchNamed(t, m, "spark-search"))
	if word != "stuck" {
		t.Fatalf("the card of spark-search says %q, want stuck", word)
	}
	head := strings.Split(plainFrame(m), "\n")[3]
	view := strings.TrimSpace(head[strings.LastIndex(head, "│")+len("│"):])
	if view != "spark-search · stuck" {
		t.Errorf("the live view's head is %q, want the card's word: spark-search · stuck", view)
	}
}
