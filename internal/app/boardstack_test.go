package app

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

func TestStackRowsGivesShippingItsShips(t *testing.T) {
	tests := []struct {
		name                          string
		need, ships, rows             int
		needs, work, ship             bool
		needH, workH, shipH, wantGaps int
	}{
		{"shipping takes a row a ship", 9, 3, 21, true, true, true, 10, 5, 4, 2},
		{"shipping stops at 3 rows", 9, 6, 21, true, true, true, 10, 5, 4, 2},
		{"one ship takes one row", 9, 1, 21, true, true, true, 10, 7, 2, 2},
		{"an empty shipping keeps its head and a row", 9, 0, 21, true, true, true, 10, 7, 2, 2},
		{"needs you that does not fit leaves two rows each", 30, 3, 21, true, true, true, 17, 2, 2, 0},
		{"one shown section takes the rest", 9, 0, 21, true, true, false, 10, 10, 0, 1},
		{"only shipping shown takes the rest", 9, 3, 21, true, false, true, 10, 0, 10, 1},
		{"hidden needs you gives its rows to working", 0, 3, 21, false, true, true, 0, 16, 4, 1},
		{"neither section shown leaves needs you the rows", 30, 0, 21, true, false, false, 21, 0, 0, 0},
		{"one shown section keeps two rows when needs you is long", 30, 0, 21, true, true, false, 19, 2, 0, 0},
		{"short of rows each section keeps what it can", 9, 3, 5, true, true, true, 2, 2, 1, 0},
		{"a narrowed empty needs you keeps its head alone", 0, 3, 21, true, true, true, 1, 14, 4, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needH, workH, shipH, gaps := stackRows(tt.need, tt.ships, tt.rows, tt.needs, tt.work, tt.ship)
			if needH != tt.needH || workH != tt.workH || shipH != tt.shipH || gaps != tt.wantGaps {
				t.Fatalf("stackRows(%d, %d, %d, %v, %v, %v) = %d, %d, %d, %d; want %d, %d, %d, %d",
					tt.need, tt.ships, tt.rows, tt.needs, tt.work, tt.ship, needH, workH, shipH, gaps,
					tt.needH, tt.workH, tt.shipH, tt.wantGaps)
			}
			if sum := needH + workH + shipH + gaps; (tt.work || tt.ship) && sum != tt.rows {
				t.Errorf("the sections take %d of %d rows", sum, tt.rows)
			}
		})
	}
}

func TestStackRowsMovesShippingOnlyUnderFourShips(t *testing.T) {
	_, _, want, _ := stackRows(9, 3, 21, true, true, true)
	for _, ships := range []int{4, 5, 13} {
		if _, _, shipH, _ := stackRows(9, ships, 21, true, true, true); shipH != want {
			t.Errorf("%d ships take %d rows, want %d as 3 do", ships, shipH, want)
		}
	}
}

// sectionHeads is the line of each section's head in the board's body at
// w by h, -1 for a section with no head.
func sectionHeads(t *testing.T, m Model) (d boardDrawn, plain []string, at map[boardCol]int) {
	t.Helper()
	d = m.boardPanel(max(m.width, minWidth), m.bodyRows())
	plain = plainLines(d.lines)
	at = map[boardCol]int{colNeed: -1, colWork: -1, colShip: -1}
	for i, l := range plain {
		for c := range at {
			if strings.HasPrefix(l, " "+c.name()+" · ") && strings.HasSuffix(strings.TrimRight(l, " "), "  −") {
				at[c] = i
			}
		}
	}
	return d, plain, at
}

func TestMediumStacksTheSections(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, nil)
	d, plain, at := sectionHeads(t, m)
	body := strings.Join(plain, "\n")
	need, work, ship := at[colNeed], at[colWork], at[colShip]
	if need < 0 || work < 0 || ship < 0 || !(need < work && work < ship) {
		t.Fatalf("the sections are not NEEDS YOU, WORKING, SHIPPING from the top (%d, %d, %d):\n%s", need, work, ship, body)
	}
	for c, want := range map[boardCol]string{colNeed: "NEEDS YOU · 4  −", colWork: "WORKING · 3  −", colShip: "SHIPPING · 3  −"} {
		if got := strings.TrimSpace(plain[at[c]]); got != want {
			t.Errorf("the head of %s is %q, want it alone on its line as %q", c.name(), got, want)
		}
		if sp, ok := spanAt(d.taps[at[c]], 1); !ok || sp.x0 != 0 || sp.press != c.short() {
			t.Errorf("a tap on the head of %s does not press %q: %+v", c.name(), c.short(), d.taps[at[c]])
		}
	}
	for _, name := range []string{"spark-search", "kindle-web", "hearth-api", "flint-auth"} {
		if !slices.ContainsFunc(plain[need+1:work], func(l string) bool { return strings.Contains(l, name) }) {
			t.Errorf("the wait %s is not between NEEDS YOU and WORKING:\n%s", name, body)
		}
	}
	for _, name := range []string{"flint-ui", "ember-billing", "spark-docs"} {
		if !slices.ContainsFunc(plain[work+1:ship], func(l string) bool { return strings.Contains(l, name) }) {
			t.Errorf("%s is not in WORKING:\n%s", name, body)
		}
	}
	if row := plain[work+1]; !strings.Contains(row, "busy Writing the test for a token") || !strings.Contains(row, "ctx 40%") {
		t.Errorf("a working match's row does not say what it does at the full width: %q", row)
	}
}

// fewerWaits is the fixed fleet with only its first n waits: the others
// leave with their queue items and ships.
func fewerWaits(st proto.State, n int) proto.State {
	gone := []string{"flint-auth", "kindle-web", "hearth-api", "spark-search"}[n:]
	out := func(name string) bool { return slices.Contains(gone, name) }
	st.Sessions = slices.DeleteFunc(slices.Clone(st.Sessions), func(s sample.Session) bool { return out(s.Name) })
	st.Queue = slices.DeleteFunc(slices.Clone(st.Queue), func(it queue.Item) bool { return out(it.Name) })
	ships := *st.Ships
	ships.Ships = slices.DeleteFunc(slices.Clone(ships.Ships), func(sh proto.Ship) bool { return out(sh.Session.Name) })
	st.Ships = &ships
	return st
}

func TestMediumShowsEveryWait(t *testing.T) {
	st := boardFixture(t)
	for i := range st.Sessions {
		if st.Sessions[i].Name == "flint-auth" { // a quote of three lines over its two options
			st.Sessions[i].LastLine = "I can fix it in two ways. The token check is a small patch that keeps the old store and its tests. " +
				"The new session store is the bigger move: it changes the login, the refresh and the logout, and it needs a migration."
		}
	}
	names := []string{"flint-auth", "kindle-web", "hearth-api", "spark-search"}
	for n := 1; n <= 4; n++ {
		for _, chosen := range names[:n] {
			m := boardShot(t, fewerWaits(st, n), 80, 31, chooseCard(chosen))
			_, plain, at := sectionHeads(t, m)
			body := strings.Join(plain, "\n")
			if at[colNeed] < 0 || at[colWork] < 0 {
				t.Fatalf("%d waits, %s chosen: no NEEDS YOU or WORKING head:\n%s", n, chosen, body)
			}
			need := plain[at[colNeed]+1 : at[colWork]]
			if slices.ContainsFunc(need, func(l string) bool { return strings.Contains(l, "↓") }) {
				t.Errorf("%d waits, %s chosen: NEEDS YOU ends in ↓ N more:\n%s", n, chosen, body)
			}
			for _, name := range names[:n] {
				if !slices.ContainsFunc(need, func(l string) bool { return strings.Contains(l, name) }) {
					t.Errorf("%d waits, %s chosen: %s is not on the screen:\n%s", n, chosen, name, body)
				}
			}
			if chosen == "flint-auth" && !slices.ContainsFunc(need, func(l string) bool { return strings.Contains(l, "2. Move to the new session store") }) {
				t.Errorf("%d waits: the chosen flint-auth is not whole:\n%s", n, body)
			}
		}
	}
}

// TestMediumHeadsHoldStill (owner, 2026-11-14): WORKING's head moves only
// with NEEDS YOU's rows; SHIPPING's head moves with them and with the
// count of ships, and only then.
func TestMediumHeadsHoldStill(t *testing.T) {
	base := boardFixture(t)
	heads := func(st proto.State, edit func(*Model)) map[boardCol]int {
		_, _, at := sectionHeads(t, boardShot(t, st, 80, 31, edit))
		return at
	}
	edit := func(f func(s *sample.Session)) proto.State {
		st := base
		st.Sessions = slices.Clone(base.Sessions)
		for i := range st.Sessions {
			f(&st.Sessions[i])
		}
		return st
	}
	want := heads(base, nil)
	says := edit(func(s *sample.Session) {
		if s.Name == "flint-ui" {
			s.LastLine = "Reading the refresh path again."
		}
	})
	ships := edit(func(s *sample.Session) {
		if s.Name == "spark-docs" {
			s.Busy, s.Status = false, "idle"
		}
	})
	sh := *ships.Ships
	sh.Ships = append(slices.Clone(sh.Ships), shipFor(paneSession(ships, "spark-docs"), 4570, shipRunning, "CI · 2 of 9 jobs", 1, time.Minute))
	ships.Ships = &sh
	more := base
	extra := boardMatch("hearth-queue", time.Hour)
	extra.Busy, extra.Status, extra.LastLine, extra.Context, extra.ContextPct = true, "busy", "Draining the retry queue.", "known", 28
	more.Sessions = append(slices.Clone(base.Sessions), extra)
	fewer := base
	fewer.Sessions = slices.DeleteFunc(slices.Clone(base.Sessions), func(s sample.Session) bool { return s.Name == "spark-docs" })
	for _, c := range []struct {
		name string
		st   proto.State
	}{
		{"a working match's text changes", says},
		{"WORKING gains a match", more},
		{"WORKING loses a match", fewer},
	} {
		if got := heads(c.st, nil); !maps.Equal(got, want) {
			t.Errorf("%s: the heads moved from %v to %v", c.name, want, got)
		}
	}
	if got := heads(ships, nil); !maps.Equal(got, want) { // the fourth ship: SHIPPING keeps its 3 rows
		t.Errorf("a match moves from WORKING to SHIPPING: the heads moved from %v to %v", want, got)
	}
	two := base
	two.Sessions = slices.DeleteFunc(slices.Clone(base.Sessions), func(s sample.Session) bool { return s.Name == "hearth-limits" })
	sh2 := *base.Ships
	sh2.Ships = slices.DeleteFunc(slices.Clone(sh2.Ships), func(sh proto.Ship) bool { return sh.Session.Name == "hearth-limits" })
	two.Ships = &sh2
	if got := heads(two, nil); got[colNeed] != want[colNeed] || got[colWork] != want[colWork] || got[colShip] != want[colShip]+1 {
		t.Errorf("a ship goes, 3 to 2: the heads are %v, want %v with SHIPPING a row down", got, want)
	}
	five := base // more than 3 ships: SHIPPING keeps its rows whatever WORKING holds
	sh5 := *base.Ships
	for i, name := range []string{"ember-tax", "wick-push"} {
		ship := boardMatch(name, time.Hour)
		ship.Context, ship.ContextPct = "known", 30
		five.Sessions = append(slices.Clone(five.Sessions), ship)
		sh5.Ships = append(slices.Clone(sh5.Ships), shipFor(ship, 4571+i, shipRunning, "CI · 1 of 9 jobs", 1, time.Minute))
	}
	five.Ships = &sh5
	fiveMore, fiveFewer := five, five
	fiveMore.Sessions = append(slices.Clone(five.Sessions), extra)
	fiveFewer.Sessions = slices.DeleteFunc(slices.Clone(five.Sessions), func(s sample.Session) bool { return s.Name == "spark-docs" })
	wantFive := heads(five, nil)
	if _, plain, _ := sectionHeads(t, boardShot(t, five, 80, 31, nil)); !strings.HasPrefix(strings.TrimSpace(plain[wantFive[colShip]]), "SHIPPING · 5") {
		t.Fatalf("the fleet of five ships has %q", plain[wantFive[colShip]])
	}
	for name, st := range map[string]proto.State{"gains": fiveMore, "loses": fiveFewer} {
		if got := heads(st, nil); !maps.Equal(got, wantFive) {
			t.Errorf("5 ships, WORKING %s a match: the heads moved from %v to %v", name, wantFive, got)
		}
	}
	if a, b := heads(base, chooseCard("flint-ui")), heads(base, chooseCard("spark-docs")); !maps.Equal(a, b) {
		t.Errorf("the selection moved in WORKING: the heads moved from %v to %v", a, b)
	}
	if a, b := heads(base, chooseCard("cinder-ci")), heads(base, chooseCard("flint-api")); !maps.Equal(a, b) {
		t.Errorf("the selection moved in SHIPPING: the heads moved from %v to %v", a, b)
	}
}

// paneSession is the match named name in st.
func paneSession(st proto.State, name string) sample.Session {
	return st.Sessions[slices.IndexFunc(st.Sessions, func(s sample.Session) bool { return s.Name == name })]
}

// busyFleet is a fleet at a busy hour: two waits, flint-auth's question
// and kindle-web's merge; thirteen matches at work; three ships.
func busyFleet(t testing.TB) proto.State {
	st := boardFixture(t)
	busy := map[string]string{
		"hearth-api":   "Migrating the session table to the new key.",
		"spark-search": "Running 412 tests in internal/search.",
	}
	for i, s := range st.Sessions {
		if line, ok := busy[s.Name]; ok {
			s.Busy, s.Status, s.Permits, s.Idle, s.LastLine = true, "busy", false, 0, line
			st.Sessions[i] = s
		}
	}
	st.Queue = slices.DeleteFunc(slices.Clone(st.Queue), func(it queue.Item) bool { return it.Name == "hearth-api" })
	ships := *st.Ships
	ships.Ships = slices.DeleteFunc(slices.Clone(ships.Ships), func(sh proto.Ship) bool { return sh.Session.Name == "spark-search" })
	st.Ships = &ships
	for i, w := range []struct {
		name, agent, line string
		ctx               float64
	}{
		{"hearth-db", "claude", "Adding the index on orders.created.", 18},
		{"flint-admin", "claude", "Reading the admin routes for the audit.", 33},
		{"spark-mail", "claude", "Writing the bounce handler.", 47},
		{"kindle-cache", "claude", "Profiling the cache misses on the product page.", 88},
		{"ember-tax", "claude", "Reading the VAT rules for the EU.", 29},
		{"wick-push", "opencode", "Wiring push tokens to the API.", 51},
		{"cinder-deploy", "codex", "Updating the deploy runbook.", 12},
		{"hearth-search", "claude", "Tuning the ranking weights.", 39},
	} {
		s := boardMatch(w.name, time.Duration(i+1)*20*time.Minute)
		s.Agent, s.Busy, s.Status, s.Idle, s.LastLine, s.Context, s.ContextPct = w.agent, true, "busy", 0, w.line, "known", w.ctx
		st.Sessions = append(st.Sessions, s)
	}
	return st
}

func TestMediumBusyFleet(t *testing.T) {
	m := boardShot(t, busyFleet(t), 80, 31, chooseCard("flint-auth"))
	_, plain, at := sectionHeads(t, m)
	body := strings.Join(plain, "\n")
	if got := strings.TrimSpace(plain[max(at[colWork], 0)]); got != "WORKING · 13  −" {
		t.Fatalf("WORKING's head is %q, want 13 at work:\n%s", got, body)
	}
	need := plain[at[colNeed]+1 : at[colWork]]
	for _, want := range []string{"flint-auth", "1. Patch the token check", "2. Move to the new session store", "kindle-web"} {
		if !slices.ContainsFunc(need, func(l string) bool { return strings.Contains(l, want) }) {
			t.Errorf("NEEDS YOU has no %q: a wait is not whole:\n%s", want, body)
		}
	}
	work := plain[at[colWork]+1 : at[colShip]]
	last := strings.TrimSpace(work[len(work)-1])
	for last == "" && len(work) > 1 {
		work = work[:len(work)-1]
		last = strings.TrimSpace(work[len(work)-1])
	}
	if !strings.HasPrefix(last, "↓ ") || !strings.HasSuffix(last, " more") {
		t.Errorf("WORKING does not end in ↓ N more:\n%s", body)
	}
	golden(t, m, "board-busy", 80, 31)
}

func TestMediumOneRowSectionShowsACard(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, chooseCard("flint-auth")) // the open question leaves WORKING two rows
	d, plain, at := sectionHeads(t, m)
	work := at[colWork]
	if work < 0 || strings.TrimSpace(plain[work]) != "WORKING · 3  ↓ 2  −" {
		t.Fatalf("WORKING's head does not count the cards that do not show:\n%s", strings.Join(plain, "\n"))
	}
	if !strings.Contains(plain[work+1], "flint-ui") || d.cards[boardSpot{work + 1, colWork}] != paneOf(paneSession(boardFixture(t), "flint-ui")) {
		t.Errorf("WORKING's one row is not its first card: %q", plain[work+1])
	}
	if sp, ok := spanAt(d.taps[work], lipgloss.Width(" WORKING · 3  ↓ 2  ")); !ok || sp.press != colWork.short() {
		t.Errorf("a tap on WORKING's − does not hide it: %+v", d.taps[work])
	}
	if m = tapHead(t, m, colWork); !m.colHidden(colWork) {
		t.Error("a tap on WORKING's head does not hide it")
	}

	m = boardShot(t, boardFixture(t), 80, 24, chooseCard("spark-docs")) // a short screen leaves WORKING two rows
	_, plain, at = sectionHeads(t, m)
	if work = at[colWork]; work < 0 || strings.TrimSpace(plain[work]) != "WORKING · 3  ↓ 2  −" {
		t.Fatalf("WORKING's head does not count the cards that do not show:\n%s", strings.Join(plain, "\n"))
	}
	if row := plain[work+1]; !strings.HasPrefix(row, "▌✦ spark-docs") || !strings.Contains(row, "busy Running the suite.") {
		t.Errorf("WORKING's one row is not the chosen card's row: %q", row)
	}
}

// TestMediumShortScreensKeepTheNames sweeps the heights at 80 columns: every
// shown section with cards keeps its name on the screen, whatever is
// chosen and whether QUIET is open, and every line is the screen's width.
func TestMediumShortScreensKeepTheNames(t *testing.T) {
	st := boardFixture(t)
	for h := 6; h <= 31; h++ {
		for _, chosen := range []string{"spark-search", "kindle-web", "hearth-api", "flint-auth", "wick-mobile"} {
			m := boardShot(t, st, 80, h, chooseCard(chosen))
			lines := frameLines(m)
			if len(lines) != h {
				t.Errorf("80x%d, %s chosen: %d lines", h, chosen, len(lines))
			}
			for i, l := range lines {
				if lipgloss.Width(l) != 80 {
					t.Errorf("80x%d, %s chosen: line %d is %d cells", h, chosen, i, lipgloss.Width(l))
				}
			}
			if m.bodyRows() < 1 {
				continue // no body row: the console's least frame
			}
			f := plainFrame(m)
			for _, c := range []boardCol{colNeed, colWork, colShip} {
				if !strings.Contains(f, c.name()+" · ") {
					t.Errorf("80x%d, %s chosen: no %s on the screen:\n%s", h, chosen, c.name(), f)
				}
			}
		}
	}
}
