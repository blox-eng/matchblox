package app

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/logos"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

// pressOn is pressAll on the board's console.
func pressOn(m Model, keys ...string) Model { return pressAll(m, keys...).(Model) }

func boardFake(m Model) *fakeConn { return m.conn.(*fakeConn) }

func chosenName(t *testing.T, m Model) string {
	t.Helper()
	s, ok := m.boardSelected()
	if !ok {
		t.Fatal("no card is chosen")
	}
	return s.Name
}

// newBoardState gives the console a new state of this machine: st's
// matches, queue and ships that are not gpu-1's, as boardShot splits them.
func newBoardState(t *testing.T, m Model, st proto.State) Model {
	t.Helper()
	home := st
	home.Sessions = slices.DeleteFunc(slices.Clone(st.Sessions), func(s sample.Session) bool { return s.Host == "gpu-1" })
	home.Queue = slices.DeleteFunc(slices.Clone(st.Queue), func(it queue.Item) bool { return it.Host == "gpu-1" })
	home.Ships = &proto.Ships{At: demoAt}
	if st.Ships != nil {
		home.Ships.Ships = slices.DeleteFunc(slices.Clone(st.Ships.Ships), func(sh proto.Ship) bool { return sh.Host == "gpu-1" })
	}
	next, _ := m.Update(stateMsg(home))
	return next.(Model)
}

// noticeRow is the row over the key line's hairline.
func noticeRow(m Model) string {
	lines := strings.Split(plainFrame(m), "\n")
	return strings.TrimSpace(lines[len(lines)-3])
}

func mergeOf(t *testing.T, st proto.State) proto.Ship {
	t.Helper()
	for _, sh := range st.Ships.Ships {
		if sh.Number == 4556 {
			return sh
		}
	}
	t.Fatal("no ship #4556")
	return proto.Ship{}
}

func TestArrowsSkipHiddenAndEmptyColumns(t *testing.T) {
	st := boardFixture(t)
	for _, w := range []int{80, 104} {
		m := boardShot(t, st, w, 38, func(m *Model) { m.flipHidden(colWork) })
		m = pressOn(m, "right")
		if m.boardAt != colShip || chosenName(t, m) != "cinder-ci" {
			t.Errorf("w=%d: → from NEEDS YOU with WORKING hidden: %v, %s", w, m.boardAt, chosenName(t, m))
		}

		noShips := st
		noShips.Ships = &proto.Ships{At: demoAt}
		m = boardShot(t, noShips, w, 38, func(m *Model) { m.flipHidden(colWork) })
		if n := len(m.boardIndex()[colShip]); n != 0 {
			t.Fatalf("SHIPPING has %d cards", n)
		}
		m = pressOn(m, "right")
		if m.boardAt != colQuiet {
			t.Errorf("w=%d: → over a hidden and an empty column lands on %v", w, m.boardAt)
		}
		if m = pressOn(m, "right"); m.boardAt != colWork || m.colHidden(colWork) {
			t.Errorf("w=%d: → at the last column does not show the hidden WORKING: %v", w, m.boardAt)
		}
		if m = pressOn(m, "left"); m.boardAt != colNeed {
			t.Errorf("w=%d: ← from QUIET lands on %v", w, m.boardAt)
		}
	}
}

// TestAKeyShowsAHiddenColumn: a keyboard with no mouse brings a hidden
// column back: → past the last shown column shows the first hidden one,
// and with every column hidden → shows NEEDS YOU.
func TestAKeyShowsAHiddenColumn(t *testing.T) {
	st := boardFixture(t)
	m := boardShot(t, st, 104, 38, nil)
	for range boardCols {
		m = pressOn(m, "m")
		for m.menu != nil && !strings.HasPrefix(m.menu.items[m.menu.at].label, "hide") {
			m = pressOn(m, "down")
		}
		m = pressOn(m, "enter")
	}
	if _, shown := m.boardDrawOf(); len(shown) != 0 {
		t.Fatalf("the menu left %v shown", shown)
	}
	if f := strings.Join(strings.Fields(plainFrame(m)), " "); !strings.Contains(f, "→ or a tap on a + name shows one") {
		t.Errorf("the body does not name the key:\n%s", plainFrame(m))
	}
	m = pressOn(m, "right")
	if s, ok := m.boardSelected(); m.colHidden(colNeed) || m.boardAt != colNeed || !ok || m.columnOf(s) != colNeed {
		t.Errorf("→ with every column hidden: need hidden %v, open %s, a card of it chosen %v", m.colHidden(colNeed), m.boardAt.name(), ok)
	}

	m = pressOn(boardShot(t, st, 104, 38, func(m *Model) { m.flipHidden(colWork) }), "right", "right")
	if m.boardAt != colQuiet {
		t.Fatalf("→ → from NEEDS YOU lands on %s", m.boardAt.name())
	}
	if m = pressOn(m, "right"); m.colHidden(colWork) || m.boardAt != colWork {
		t.Errorf("→ past QUIET: work hidden %v, open %s", m.colHidden(colWork), m.boardAt.name())
	}
}

// TestAKeyOnAnEmptySearchSaysEscClears: a card key with no card says the
// fix that is true: esc clears a search, s turns a host, and a + name
// shows a column only when one is hidden.
func TestAKeyOnAnEmptySearchSaysEscClears(t *testing.T) {
	st := boardFixture(t)
	m := typeText(pressOn(boardShot(t, st, 104, 38, nil), "/"), "zzzz").(Model)
	m = pressOn(m, "enter")
	for _, k := range []string{"x", "p", "m"} {
		if got := noticeRow(pressOn(m, k)); got != `nothing matches "zzzz" · esc clears` {
			t.Errorf("%s on an empty search says %q", k, got)
		}
	}
	m = boardShot(t, st, 104, 38, func(m *Model) { m.boardHost = "macbook" })
	if got := noticeRow(pressOn(m, "x")); got != "no match on macbook · s turns the host" {
		t.Errorf("x on a host with no match says %q", got)
	}
	m = boardShot(t, st, 104, 38, func(m *Model) {
		for _, c := range boardCols {
			m.flipHidden(c)
		}
	})
	if got := noticeRow(pressOn(m, "x")); got != "no card is chosen · tap a + name to show a column" {
		t.Errorf("x with every column hidden says %q", got)
	}
}

func TestTheChoiceFollowsItsMatch(t *testing.T) {
	st := boardFixture(t)
	m := pressOn(boardShot(t, st, 80, 31, nil), "down")
	if got := chosenName(t, m); got != "kindle-web" {
		t.Fatalf("↓ chose %s, want the second card, kindle-web", got)
	}
	if m.peekAt != demoAt {
		t.Errorf("the move did not start the peek: %v", m.peekAt)
	}

	older := boardMatch("tinder-auth", time.Hour)
	older.Permits, older.LastLine = true, "May I delete the old token table?"
	st.Sessions = append(st.Sessions, older)
	st.Queue = append(st.Queue, waitFor(older, queue.StatePermission, 3*time.Hour))
	m = newBoardState(t, m, st)
	if need := m.boardIndex()[colNeed]; need[0].Name != "tinder-auth" {
		t.Fatalf("the new wait is not above: %s", need[0].Name)
	}
	if got := chosenName(t, m); got != "kindle-web" {
		t.Errorf("after a new state the choice is %s, want kindle-web", got)
	}

	m = pressOn(m, "up", "up", "up")
	if got := chosenName(t, m); got != "tinder-auth" {
		t.Errorf("↑ at the top: %s", got)
	}
	clock := demoAt
	// ↓ at the bottom chooses the ghost row (step 0b), and ↑ comes back. At
	// Medium the bottom is SHIPPING's last card: ↓ goes on through the sections.
	m = pressOn(boardShot(t, st, 80, 31, chooseCard("hearth-limits")), "esc")
	if m = pressOn(m, "down"); reachOf(m) != "ghost" {
		t.Errorf("↓ at the bottom chose %s, want the ghost row", reachOf(m))
	}
	if m = pressOn(m, "up"); chosenName(t, m) != "hearth-limits" {
		t.Errorf("↑ from the ghost row chose %s, want hearth-limits", chosenName(t, m))
	}
	m = boardShot(t, st, 80, 31, func(m *Model) {
		chooseCard("flint-auth")(m)
		m.opt.Now = func() time.Time { return clock }
	})
	m = pressOn(m, "esc") // the first frame lands the peek
	clock = clock.Add(time.Minute)
	m = pressOn(m, "up", "down")
	if m.peekAsk != "toolu_board" {
		t.Errorf("the peek of flint-auth does not keep its question: %q", m.peekAsk)
	}
}

func TestAfterAnActTheFirstWaitIsChosen(t *testing.T) {
	st := boardFixture(t)
	m := pressOn(boardShot(t, st, 80, 31, chooseCard("kindle-web")), "p", "y")
	if len(boardFake(m).acts()) != 1 {
		t.Fatalf("the merge was not sent: %+v", boardFake(m).acts())
	}
	if got := chosenName(t, m); m.boardAt != colNeed || got != "spark-search" {
		t.Errorf("after a merge the choice is %s in %v, want spark-search", got, m.boardAt)
	}

	m = boardShot(t, st, 80, 31, chooseCard("hearth-api"))
	m.opt.Run = func([]string) error { return nil }
	if m = pressOn(m, "enter"); chosenName(t, m) != "spark-search" {
		t.Errorf("after ⏎ the choice is %s, want spark-search", chosenName(t, m))
	}

	alone := st
	alone.Sessions = slices.DeleteFunc(slices.Clone(st.Sessions), func(s sample.Session) bool {
		return slices.Contains([]string{"flint-auth", "hearth-api", "spark-search"}, s.Name)
	})
	m = pressOn(boardShot(t, alone, 80, 31, chooseCard("kindle-web")), "p", "y")
	if got := chosenName(t, m); m.boardAt != colNeed || got != "kindle-web" {
		t.Errorf("with no other wait the choice moved to %s in %v", got, m.boardAt)
	}
}

func TestTheKeyLineDropsWholeKeysFromTheRight(t *testing.T) {
	st := boardFixture(t)
	m := boardShot(t, st, 80, 31, chooseCard("kindle-web"))
	hints := m.boardHints(80)
	if hints[0].key != "p" || !slices.ContainsFunc(hints, func(h hint) bool { return h.key == "tab" }) {
		t.Fatalf("the board's keys: %+v", hints)
	}
	line := strings.TrimSpace(keyText(m.draw(80)))
	var parts []string
	whole := -1
	for i, h := range hints {
		parts = append(parts, h.key+" "+h.label)
		if strings.Join(parts, "   ") == line {
			whole = i
		}
	}
	if whole < 0 || whole == len(hints)-1 {
		t.Errorf("at 80 the key line does not end at a whole key of %d: %q", len(hints), line)
	}
	if strings.Contains(line, "sessions") || strings.Contains(line, "rename") {
		t.Errorf("the key line has the keys of Sessions: %q", line)
	}

	m = boardShot(t, st, 40, 34, nil)
	if got := strings.TrimSpace(keyText(m.draw(40))); got != "c copy   v one line   s hosts   u strip" {
		t.Errorf("at 40 the key line is %q", got)
	}
}

func TestPMergesThroughTheConsentBox(t *testing.T) {
	st := boardFixture(t)
	sh := mergeOf(t, st)
	m := pressOn(boardShot(t, st, 80, 31, chooseCard("kindle-web")), "p")
	if !m.consenting() || !strings.Contains(consentText(m), strings.Join(sh.Merge, " ")) {
		t.Fatalf("p does not show the merge: %q", consentText(m))
	}
	m = pressOn(m, "enter")
	if m.pending != nil || len(boardFake(m).acts()) != 0 {
		t.Fatalf("⏎ ran the merge: %+v", boardFake(m).acts())
	}
	m = pressOn(m, "p", "y")
	want := proto.Act{RecID: "ship:" + sh.Repo + "#4556", Which: "secondary", Confirm: "y"}
	if acts := boardFake(m).acts(); len(acts) != 1 || acts[0] != want {
		t.Fatalf("y sent %+v, want %+v", acts, want)
	}

	m = pressOn(boardShot(t, st, 80, 31, chooseCard("spark-search")), "p", "p")
	if m.pending != nil || len(boardFake(m).acts()) != 0 {
		t.Errorf("p on a stuck ship acted: %+v", boardFake(m).acts())
	}
	if got := noticeRow(m); got != "no ship of spark-search waits for a merge" {
		t.Errorf("the notice: %q", got)
	}
}

func TestXOnlyEverKills(t *testing.T) {
	st := boardFixture(t)
	m := boardShot(t, st, 80, 31, chooseCard("kindle-web"))
	pane := paneNamed(st, "kindle-web")
	m = pressOn(m, "x")
	box := consentText(m)
	if !strings.Contains(box, "tmux kill-pane -t "+pane) || strings.Contains(box, "merge") {
		t.Fatalf("x shows %q", box)
	}
	m = pressOn(m, "y", "y")
	if acts := boardFake(m).acts(); len(acts) != 1 || acts[0].RecID != "kill:"+pane {
		t.Fatalf("x sent %+v", acts)
	}
}

func TestTheBoxKeepsWhatItShowed(t *testing.T) {
	st := boardFixture(t)
	sh := mergeOf(t, st)
	m := pressOn(boardShot(t, st, 80, 31, chooseCard("kindle-web")), "p")
	shown := consentText(m)

	merged := st
	merged.Ships = &proto.Ships{At: demoAt}
	for _, s := range st.Ships.Ships {
		if s.Number == 4556 {
			s.Kind, s.Note, s.Stage, s.Merge = shipRunning, "merged · deploying", 3, nil
		}
		merged.Ships.Ships = append(merged.Ships.Ships, s)
	}
	m = newBoardState(t, m, merged)
	if got := consentText(m); got != shown {
		t.Fatalf("the box changed with the state:\n%q\n%q", shown, got)
	}
	m = pressOn(m, "y")
	want := proto.Act{RecID: "ship:" + sh.Repo + "#4556", Which: "secondary", Confirm: "y"}
	if acts := boardFake(m).acts(); len(acts) != 1 || acts[0] != want {
		t.Fatalf("y sent %+v, want %+v", acts, want)
	}

	two := st
	two.Ships = &proto.Ships{At: demoAt, Ships: slices.Clone(st.Ships.Ships)}
	kindle := st.Sessions[slices.IndexFunc(st.Sessions, func(s sample.Session) bool { return s.Name == "kindle-web" })]
	stuck := shipFor(kindle, 4557, shipStuck, "CI failed · tests", 1, time.Hour)
	two.Ships.Ships = append(two.Ships.Ships, stuck)
	m = boardShot(t, two, 80, 31, chooseCard("kindle-web"))
	if lead := m.shipsOf(kindle)[0]; lead.Number != 4557 {
		t.Fatalf("the leading ship is #%d", lead.Number)
	}
	lines, _ := m.boardCard(kindle, 38, cardOpts{sel: true, need: true})
	if f := strings.Join(plainLines(lines), "\n"); strings.Contains(f, "merge #") {
		t.Errorf("a card whose leading ship is stuck offers the merge of the second:\n%s", f)
	}
	if m = pressOn(m, "p"); m.pending != nil {
		t.Errorf("p merged a ship that does not lead: %v", m.pending)
	}
}

func TestATapNeverRunsADestructiveAct(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, chooseCard("kindle-web")) // at Medium only the chosen wait shows its keys
	lines := strings.Split(plainFrame(m), "\n")
	y := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "  p merge #4556") })
	if y < 0 {
		t.Fatalf("no p merge on the frame:\n%s", strings.Join(lines, "\n"))
	}
	next, cmd := tap(m, 3, y)
	drain(cmd)
	m = next.(Model)
	if !m.consenting() || chosenName(t, m) != "kindle-web" || !strings.Contains(consentText(m), "gh pr merge 4556") {
		t.Fatalf("a tap on p merge: consent %v, chosen %s", m.consenting(), chosenName(t, m))
	}
	next, cmd = tap(m, 40, 15)
	drain(cmd)
	m = next.(Model)
	if m.pending != nil || len(boardFake(m).acts()) != 0 {
		t.Errorf("a tap on the box did not cancel it: %v %+v", m.pending, boardFake(m).acts())
	}
}

func TestAChangedKeySaysSoOnce(t *testing.T) {
	dir := t.TempDir()
	var clipped []string
	now := demoAt
	open := func() Model {
		return boardShot(t, boardFixture(t), 80, 31, func(m *Model) {
			m.opt.StateDir, m.viewPrefs = dir, loadViewPrefs(dir)
			m.opt.Clip = func(s string) { clipped = append(clipped, s) }
			m.opt.Now = func() time.Time { return now }
		})
	}
	m := open()
	for _, c := range []struct{ key, once, then string }{
		{"c", "c copies now. The stoker is on its card.", "copied: last message of spark-search · not checked here"},
		{"v", "v changes the cards now: verbose, or one line.", ""},
		{"g", "g shows one chain now. esc shows all.", "spark-search is in no chain"},
		{"u", "u turns the strip now.", ""},
		{"p", "p merges now. The ships are on their cards.", "no ship of spark-search waits for a merge"},
		{"1", "A digit marks an answer now. Press it again to send.", ""},
		{"s", "s changes the host now. The order of a column is fixed.", ""}, // last: it turns to the next host
	} {
		m = pressOn(m, c.key)
		if got := noticeRow(m); got != c.once {
			t.Errorf("the first %s says %q, want %q", c.key, got, c.once)
		}
		now = now.Add(noticeFor) // the first notice ends
		m = pressOn(m, c.key)
		if got := noticeRow(m); got == c.once || c.then != "" && got != c.then {
			t.Errorf("the second %s says %q, want %q", c.key, got, c.then)
		}
	}
	if want := "Lint fails on a file I did not change. I tried twice and stopped."; len(clipped) != 2 || clipped[0] != want {
		t.Errorf("c copied %q", clipped)
	}

	m = open()
	for _, k := range []string{"c", "v", "s", "g", "u", "p", "1"} {
		m = pressOn(m, k)
		if got := noticeRow(m); strings.Contains(got, " now") {
			t.Errorf("a new console says %q again on %s", got, k)
		}
	}

	m = pressOn(open(), "down", "x")
	if got := noticeRow(m); got != "x kills. p merges #4556." {
		t.Errorf("the first x on a card whose ship waits says %q", got)
	}
}

func TestEscNeverLeavesTheConsole(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, func(m *Model) {
		m.opt.Layered = true
		m.filter = "flint"
		m.refresh()
	})
	next, cmd := key(m, "esc")
	if m = next.(Model); m.filter != "" || slices.ContainsFunc(cmdMsgs(cmd), func(msg tea.Msg) bool { _, ok := msg.(layerMsg); return ok }) {
		t.Fatalf("esc does not clear the search first: %q", m.filter)
	}
	next, cmd = key(m, "esc")
	if slices.ContainsFunc(cmdMsgs(cmd), func(msg tea.Msg) bool { _, ok := msg.(layerMsg); return ok }) || !next.(Model).boardShown() {
		t.Error("esc left the board")
	}
}

// TestAPhoneStartsVerboseInNeedsYouOnly: until v is pressed on a phone,
// NEEDS YOU is verbose and every other page one line; the first v stores a
// choice for every page. A desk starts verbose.
func TestAPhoneStartsVerboseInNeedsYouOnly(t *testing.T) {
	dir := t.TempDir()
	phone := boardShot(t, boardFixture(t), 40, 34, func(m *Model) { m.opt.StateDir, m.viewPrefs = dir, loadViewPrefs(dir) })
	if phone.oneLineIn(colNeed) || !phone.oneLineIn(colWork) || !phone.oneLineIn(colQuiet) {
		t.Fatalf("a fresh phone: NEEDS YOU one line %v, WORKING %v", phone.oneLineIn(colNeed), phone.oneLineIn(colWork))
	}
	page := func(m Model) string { return strings.Join(plainLines(m.boardPanel(40, m.bodyRows()).lines), "\n") }
	if f := page(pressOn(phone, "right")); strings.Contains(f, "ctx 22%") {
		t.Errorf("a card of WORKING that is not chosen is not one line on a fresh phone:\n%s", f)
	}
	phone = pressOn(phone, "v")
	if !phone.oneLineIn(colNeed) || !phone.oneLineIn(colWork) {
		t.Error("the first v does not make every page one line")
	}
	phone = pressOn(phone, "v")
	if phone.oneLineIn(colNeed) || phone.oneLineIn(colWork) {
		t.Error("the second v does not make every page verbose")
	}
	if p := loadViewPrefs(dir); p.OneLinePhone == nil || *p.OneLinePhone {
		t.Errorf("the phone's choice is not kept: %v", p.OneLinePhone)
	}
	if desk := boardShot(t, boardFixture(t), 104, 38, nil); desk.oneLineIn(colWork) || desk.oneLineIn(colNeed) {
		t.Error("a desk does not start verbose")
	}
}

func TestTheNoticeIsGoneAfterItsTime(t *testing.T) {
	now := demoAt
	m := boardShot(t, boardFixture(t), 40, 34, func(m *Model) { m.opt.Now = func() time.Time { return now } })
	tick := m.notice("not copied: tmux drops it · tmux set -s set-clipboard on")
	lines := strings.Split(plainFrame(m), "\n")
	two := strings.TrimSpace(lines[len(lines)-4]) + " " + strings.TrimSpace(lines[len(lines)-3])
	if two != "not copied: tmux drops it · tmux set -s set-clipboard on" {
		t.Errorf("a long notice on a phone is not two whole lines: %q", two)
	}
	if !strings.Contains(ansi.Strip(m.render()), "● 4 need you") {
		t.Error("the header changed for a notice")
	}
	msg := tick() // the tick ends the notice; it waits noticeFor
	if _, ok := msg.(noticeMsg); !ok {
		t.Fatalf("the notice's tick is %T", msg)
	}
	now = now.Add(noticeFor)
	next, _ := m.Update(msg)
	if f := plainFrame(next.(Model)); strings.Contains(f, "not copied") {
		t.Errorf("the notice stays after %v:\n%s", noticeFor, f)
	}
}

func TestARefusedMergeKeepsItsReason(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, func(m *Model) {
		m.opt.StateDir, m.viewPrefs = t.TempDir(), viewPrefs{} // the first p ever
		chooseCard("kindle-web")(m)
	})
	next, _ := m.Update(lostMsg{err: io.EOF, conn: m.conn})
	m = pressOn(next.(Model), "p")
	if m.pending != nil || !strings.Contains(m.flash, "is not connected") || !strings.Contains(noticeRow(m), "is not connected") {
		t.Errorf("the first p on a lost host: pending %v, flash %q, notice row %q", m.pending, m.flash, noticeRow(m))
	}
}

// goBoard is the board with a stoker step that waits for a go on
// hearth-api's pane.
func goBoard(t *testing.T, now *time.Time, edit func(*Model)) Model {
	t.Helper()
	st := boardFixture(t)
	hearth := st.Sessions[slices.IndexFunc(st.Sessions, func(s sample.Session) bool { return s.Name == "hearth-api" })]
	st.Stoker = &proto.StokerView{Mode: proto.StokerMode{On: true}, Run: []proto.StokerEntry{goFor(hearth, 2*time.Minute)}}
	return boardShot(t, st, 80, 31, func(m *Model) {
		m.opt.Now = func() time.Time { return *now }
		if edit != nil {
			edit(m)
		}
	})
}

var grantAct = proto.Act{RecID: "stoker:grant", Which: "primary", Confirm: "y", Text: "go-hearth-api"}

func TestATapOnTheGoArmsItAndASecondGivesIt(t *testing.T) {
	now := demoAt
	onHearth := chooseCard("hearth-api") // at Medium only the chosen wait shows its keys
	goAt := func(m Model) (int, int) {
		t.Helper()
		for y, l := range strings.Split(plainFrame(m), "\n") {
			if x := strings.Index(l, "y give the go"); x >= 0 {
				return x, y
			}
			if x := strings.Index(l, "y again: merge #4556"); x >= 0 { // the armed card says what the go does
				return x, y
			}
		}
		t.Fatalf("no y on a card:\n%s", plainFrame(m))
		return 0, 0
	}
	tapGo := func(m Model) Model {
		x, y := goAt(m)
		next, cmd := tap(m, x, y)
		drain(cmd)
		return next.(Model)
	}

	m := tapGo(goBoard(t, &now, onHearth))
	if len(boardFake(m).acts()) != 0 || noticeRow(m) != "tap y again to give the go" || chosenName(t, m) != "hearth-api" {
		t.Fatalf("the first tap: acts %+v, notice %q", boardFake(m).acts(), noticeRow(m))
	}
	if m = tapGo(m); !slices.Equal(boardFake(m).acts(), []proto.Act{grantAct}) {
		t.Fatalf("the second tap sent %+v", boardFake(m).acts())
	}

	for name, between := range map[string]func(Model) Model{
		"a move":           func(m Model) Model { return pressOn(m, "up", "down") },
		"esc":              func(m Model) Model { return pressOn(m, "esc") },
		"the notice's end": func(m Model) Model { now = now.Add(noticeFor); return m },
	} {
		now = demoAt
		m = tapGo(goBoard(t, &now, onHearth))
		if m = tapGo(between(m)); len(boardFake(m).acts()) != 0 {
			t.Errorf("a tap after %s gave the go: %+v", name, boardFake(m).acts())
		}
	}

	now = demoAt
	m = pressOn(goBoard(t, &now, chooseCard("hearth-api")), "y")
	if !slices.Equal(boardFake(m).acts(), []proto.Act{grantAct}) {
		t.Errorf("a typed y sent %+v", boardFake(m).acts())
	}
}

// TestTheBeltsGoArmsThenGives (owner, 2026-11-13: "Arm then send"): on
// Pocket the belt's y is a tap, so the first y arms the go and the card
// says what it does; a second y gives it, and any other key between them
// disarms it.
func TestTheBeltsGoArmsThenGives(t *testing.T) {
	now := demoAt
	pocket := func(m *Model) { chooseCard("hearth-api")(m); pocketOnly(m) }

	m := pressOn(goBoard(t, &now, pocket), "y")
	if !m.onPocket() {
		t.Fatal("the board is not on Pocket")
	}
	if len(boardFake(m).acts()) != 0 || m.pending != nil {
		t.Fatalf("one belt y gave the go: acts %+v, pending %v", boardFake(m).acts(), m.pending)
	}
	if f := plainFrame(m); !strings.Contains(f, "y again: merge #4556") {
		t.Errorf("the armed card does not say what the go does:\n%s", f)
	}
	if v := m.beltValue(); !strings.Contains(v, "y*") {
		t.Errorf("the belt does not light the armed y: %q", v)
	}
	if m = pressOn(m, "y"); !slices.Equal(boardFake(m).acts(), []proto.Act{grantAct}) {
		t.Fatalf("the second y sent %+v", boardFake(m).acts())
	}

	answered := func(m Model, k string) Model { // the app answers each new board= value
		m = pressOn(m, k)
		m.belt.pocket[0].n = m.belt.n
		return m
	}
	for _, between := range []string{"v", "c", "u", "esc"} {
		now = demoAt
		m = answered(answered(answered(goBoard(t, &now, pocket), "y"), between), "y")
		if !m.onPocket() {
			t.Fatalf("y %s y: the board left Pocket", between)
		}
		if len(boardFake(m).acts()) != 0 {
			t.Errorf("y %s y gave the go: %+v", between, boardFake(m).acts())
		}
	}
}

// TestAnyTapDisarmsTheBeltsGo: on Pocket a tap anywhere disarms an armed
// go, not only a tap on a card; and y on a card with no go arms nothing.
func TestAnyTapDisarmsTheBeltsGo(t *testing.T) {
	now := demoAt
	pocket := func(m *Model) { chooseCard("hearth-api")(m); pocketOnly(m) }
	for name, tapAway := range map[string]func(Model) Model{
		"a column's name": func(m Model) Model { return tapHead(t, m, colShip) },
		"the header":      func(m Model) Model { next, _ := tap(m, m.width-1, 0); return next.(Model) },
		"a blank row":     func(m Model) Model { next, _ := tap(m, m.width-1, m.height-2); return next.(Model) },
	} {
		now = demoAt
		m := pressOn(goBoard(t, &now, pocket), "y")
		if m.goArmed == (paneKey{}) {
			t.Fatalf("%s: the first y armed nothing", name)
		}
		m = tapAway(m)
		m.belt.pocket[0].n = m.belt.n // the app answers the new board= value
		if m.goArmed != (paneKey{}) {
			t.Errorf("a tap on %s kept the go armed", name)
		}
		if m = pressOn(m, "y"); len(boardFake(m).acts()) != 0 {
			t.Errorf("y, a tap on %s, y gave the go: %+v", name, boardFake(m).acts())
		}
	}

	now = demoAt
	m := pressOn(goBoard(t, &now, func(m *Model) { chooseCard("kindle-web")(m); pocketOnly(m) }), "y")
	if m.goArmed != (paneKey{}) || m.noticeText != "no step waits for your go on kindle-web" {
		t.Errorf("y on a card with no go: armed %v, notice %q", m.goArmed, m.noticeText)
	}
}

func TestCtrlOGoesIntoTheChosenCard(t *testing.T) {
	st := boardFixture(t)
	var ran []string
	m := boardShot(t, st, 80, 31, func(m *Model) {
		chooseCard("hearth-api")(m)
		m.opt.Run = func(argv []string) error { ran = argv; return nil }
	})
	next, cmd := key(m, "ctrl+o")
	fed(next, cmd)
	if want := "tmux switch-client -t " + paneNamed(st, "hearth-api"); strings.Join(ran, " ") != want {
		t.Errorf("ctrl+o ran %q, want %q", ran, want)
	}
}

func TestNoCardChosenActsOnNothing(t *testing.T) {
	st := boardFixture(t)
	st.Agents = []string{"claude"}
	m := boardShot(t, st, 80, 31, func(m *Model) {
		for _, c := range boardCols {
			m.flipHidden(c)
		}
		m.opt.Run = func(argv []string) error { t.Errorf("ran %v", argv); return nil }
	})
	if _, ok := m.boardSelected(); ok {
		t.Fatal("a card is chosen with every column hidden")
	}
	for _, k := range []string{"enter", "a", "ctrl+o", "z", "r", "A", "d", "h", "f", "F", "n", "o", "x", "p", "y"} {
		next, cmd := key(m, k)
		drain(cmd)
		nm := next.(Model)
		if nm.pending != nil || nm.form != nil || nm.tab != tabSessions || len(boardFake(nm).acts()) != 0 {
			t.Errorf("%s acted with no card: pending %v, form %v, tab %d, acts %+v", k, nm.pending, nm.form != nil, nm.tab, boardFake(nm).acts())
		}
		if got := noticeRow(nm); got != "no card is chosen · tap a + name to show a column" {
			t.Errorf("%s says %q", k, got)
		}
	}
	if nm := pressOn(m, "N"); nm.form == nil {
		t.Errorf("N opens no form with no card: %q", nm.flash)
	}
	if nm := pressOn(m, "/"); !nm.searching {
		t.Error("/ does not search with no card")
	}
}

// The live view beside the board crops the chosen card's full-screen agent
// as main's live view does: its lines are cut at the view's edge with a ›,
// its last rows show, and its window keeps its own size (#351). At 151
// columns there is no live view, and no screen is watched.
func TestTheLiveViewCropsTheChosenCardsScreen(t *testing.T) {
	st := boardFixture(t)
	watched := func(w int) Model {
		next, _ := boardShot(t, st, w, 38, chooseCard("flint-auth")).Update(tea.WindowSizeMsg{Width: w, Height: 38})
		return next.(Model)
	}
	m := watched(160)
	if m.watching != paneNamed(st, "flint-auth") {
		t.Fatalf("the board watches %q, not flint-auth", m.watching)
	}
	_, live := boardWidths(Wide, 160, boardCols[:])
	var text strings.Builder
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&text, "row %d %s\n", i, strings.Repeat("=", 2*live))
	}
	m.screen = proto.Screen{Pane: paneNamed(st, "flint-auth"), At: demoAt, Alt: true, Text: text.String()}
	f := strings.Join(plainLines(m.boardPanel(160, m.bodyRows()).lines), "\n")
	if !strings.Contains(f, "row 60 =") || strings.Contains(f, "row 1 =") || !strings.Contains(f, "=›") {
		t.Errorf("the live view does not show the last rows of flint-auth, cut with a ›:\n%s", f)
	}
	for _, l := range plainLines(m.boardPanel(160, m.bodyRows()).lines) {
		if n := ansi.StringWidth(l); n > 160 {
			t.Errorf("a line %d wide at 160 columns: %q", n, l)
		}
	}
	if m = watched(151); m.watching != "" {
		t.Errorf("at 151 columns the board watches %q", m.watching)
	}
}

// pocketOnly makes the app the only viewer, one that answered the board's
// keys with a mark.
func pocketOnly(m *Model) {
	m.viewers, m.belt.value, m.belt.n = "/dev/pts/7:105@here", ";", 1
	m.belt.pocket = []pocketClient{{"/dev/pts/7:105", 1}}
}

// TestPocketSeesANewerService: Pocket has no key line, so the strip's row
// says that the service is newer, and a tap on it starts the console again.
func TestPocketSeesANewerService(t *testing.T) {
	st := boardFixture(t)
	for _, w := range []int{40, 44} {
		m := boardShot(t, st, w, 34, func(m *Model) {
			pocketOnly(m)
			m.newer, m.startsAgain = "R start again · service v0.43.0, console v0.42.0", true
			m.opt.Restart = func() error { return nil }
		})
		if !m.onPocket() {
			t.Fatal("the board is not on Pocket")
		}
		if f := strings.Join(strings.Fields(plainFrame(m)), " "); !strings.Contains(f, "R start again · service v0.43.0, console v0.42.0") {
			t.Fatalf("%d: Pocket does not say the service is newer:\n%s", w, plainFrame(m))
		}
		next := tapText(t, m, "R start again")
		if !next.restarting {
			t.Errorf("%d: a tap on the notice did not start the console again", w)
		}
	}
}

// boardSaid are the board= values in what cmd writes to the terminal.
func boardSaid(cmd tea.Cmd) []string {
	var got []string
	for _, r := range raws(cmd) {
		if _, v, ok := strings.Cut(r, "matchblox;board="); ok {
			got = append(got, v[:strings.Index(v, "\x1b")])
		}
	}
	return got
}

// TestPocketHasNoKeyLine (spec "The belt on Pocket"): on Pocket the strip
// is the last line and the cards take the key line's two rows; a frame
// that is not Pocket's, the menu and the consent box keep the key line.
func TestPocketHasNoKeyLine(t *testing.T) {
	st := boardFixture(t)
	desk := boardShot(t, st, 40, 34, chooseCard("kindle-web"))
	pocket := boardShot(t, st, 40, 34, func(m *Model) { chooseCard("kindle-web")(m); pocketOnly(m) })
	if desk.onPocket() || !pocket.onPocket() {
		t.Fatalf("onPocket: desk %v, pocket %v", desk.onPocket(), pocket.onPocket())
	}
	ds, ps := desk.draw(40), pocket.draw(40)
	if ds.keysAt != len(ds.lines)-1 || !strings.Contains(ansi.Strip(ds.lines[ds.keysAt]), "copy") {
		t.Errorf("the desk's last line is %q, want the key line", ansi.Strip(ds.lines[len(ds.lines)-1]))
	}
	if ps.keysAt != -1 || ps.stripAt != len(ps.lines)-1 {
		t.Errorf("Pocket: key line at %d, strip at %d of %d lines; want no key line and the strip last", ps.keysAt, ps.stripAt, len(ps.lines))
	}
	if strings.Contains(ansi.Strip(ps.lines[len(ps.lines)-2]), "─────") {
		t.Errorf("Pocket keeps the key line's hairline: %q", ansi.Strip(ps.lines[len(ps.lines)-2]))
	}
	if pocket.bodyRows() != desk.bodyRows()+2 {
		t.Errorf("Pocket's body has %d rows, the desk's %d: want two more", pocket.bodyRows(), desk.bodyRows())
	}
	if n := len(frameLines(pocket)); n != 34 {
		t.Errorf("Pocket's frame has %d lines, want 34", n)
	}
	for name, m := range map[string]Model{"the menu": pressOn(pocket, "m"), "the consent box": pressOn(pocket, "p")} {
		if sc := m.draw(40); sc.keysAt != len(sc.lines)-1 || m.bodyRows() != desk.bodyRows() {
			t.Errorf("%s on Pocket: key line at %d of %d lines, body %d rows; want the key line kept", name, sc.keysAt, len(sc.lines), m.bodyRows())
		}
	}
	both := pocket
	both.viewers = "/dev/pts/2:90@here,/dev/pts/7:105@here"
	if both.onPocket() || both.draw(40).keysAt < 0 {
		t.Error("with a desk also viewing, the key line goes")
	}
	off := pocket
	off.tab = tabHistory
	if off.draw(40).keysAt < 0 {
		t.Error("with the board off, Pocket loses the key line")
	}
}

// TestPocketIsTheAppThatAnswered (controller ruling 21): onPocket holds
// while every viewer of the session answered the board's keys with the
// belt's answer (BeltKey, which tmux hands on with its tty) since it attached. Any other mark
// is no answer: an old app keeps its key line. A desk that joins, a
// terminal that comes, a new client on the same tty, or a value left
// unanswered ends it.
func TestPocketIsTheAppThatAnswered(t *testing.T) {
	st := boardFixture(t)
	clock := demoAt
	m := boardShot(t, st, 40, 34, func(m *Model) { m.opt.Now = func() time.Time { return clock } })
	update := func(msgs ...tea.Msg) {
		t.Helper()
		for _, msg := range msgs {
			next, _ := m.Update(msg)
			m = next.(Model)
		}
	}
	mark := func(tty string) tea.Msg { return uv.UnknownApcEvent(keycmd.MarkedPrefix + tty + "\x1b\\") }
	answer := uv.UnknownApcEvent(keycmd.BeltFromPrefix + "/dev/pts/7\x1b\\")
	viewers := func(key string) tea.Msg { return graphicsMsg{gen: m.opt.Gen, watched: true, key: key} }

	update(viewers("/dev/pts/7:105@here"))
	if m.onPocket() {
		t.Fatal("Pocket before the app answered")
	}
	update(mark("/dev/pts/7"), keyPress("down"))
	if m.onPocket() {
		t.Fatal("a mark alone (an old app's key) made the console Pocket")
	}
	update(uv.UnknownApcEvent(keycmd.BeltKey))
	if m.onPocket() {
		t.Fatal("the app's answer with no client named made the console Pocket")
	}
	update(answer)
	if !m.onPocket() {
		t.Fatal("the app answered the board's keys: not on Pocket")
	}

	// A new value: the answer to the one before holds for beltAnswerWait.
	update(keyPress("m"))
	if m.belt.value != "menu" || !m.onPocket() {
		t.Fatalf("value %q: the key line comes back while the app's answer is on its way", m.belt.value)
	}
	clock = clock.Add(beltAnswerWait)
	if m.onPocket() {
		t.Error("the app did not answer the menu's value, yet the console stays on Pocket")
	}
	update(answer)
	if !m.onPocket() {
		t.Error("the app answered again: not on Pocket")
	}
	update(keyPress("esc"), keyPress("m"))
	if m.onPocket() {
		t.Error("two values went unanswered, yet the console stays on Pocket")
	}
	update(answer)

	update(uv.UnknownApcEvent(keycmd.ViewerKey))
	if m.onPocket() {
		t.Error("a terminal came, yet the console stays on Pocket before the app answers again")
	}
	update(answer)
	if !m.onPocket() {
		t.Error("the app answered after a terminal came: not on Pocket")
	}
	update(viewers("/dev/pts/2:90@here,/dev/pts/7:105@here"), answer) // the app answers the value sent again
	if m.onPocket() {
		t.Error("a desk joined, yet the console is on Pocket")
	}
	update(viewers("/dev/pts/7:105@away"))
	if !m.onPocket() {
		t.Error("the desk left and the app stays: not on Pocket")
	}
	update(viewers("/dev/pts/7:200@here"))
	if m.onPocket() {
		t.Error("a new client on the app's tty is on Pocket before it answered")
	}

	off := boardShot(t, st, 40, 34, func(m *Model) { m.tab = tabHistory })
	next, _ := off.Update(viewers("/dev/pts/7:105@here"))
	next, _ = next.Update(answer)
	if next.(Model).onPocket() {
		t.Error("an answer while the board is off answered no board signal")
	}

	old := boardShot(t, st, 40, 34, func(m *Model) { m.opt.LocalTmux = "3.2" })
	next, _ = old.Update(viewers("/dev/pts/7:105@here"))
	next, _ = next.Update(answer)
	if next.(Model).onPocket() {
		t.Error("below tmux 3.3 the signal does not pass, yet the console is on Pocket")
	}
}

func beltFaces(ks []beltKey) []string {
	var out []string
	for _, k := range ks {
		out = append(out, k.Face)
	}
	return out
}

func beltKeyOf(t *testing.T, ks []beltKey, face string) beltKey {
	t.Helper()
	for _, k := range ks {
		if k.Face == face {
			return k
		}
	}
	t.Fatalf("no %s on the belt: %v", face, beltFaces(ks))
	return beltKey{}
}

// TestBeltKeysFollowTheChosenCard (design decision 10): home is ◂, the
// chosen card's keys, ↑ ↓ ⏎; the tail has every other key of the board.
func TestBeltKeysFollowTheChosenCard(t *testing.T) {
	tail := []string{"c", "m", "g", "v", "s", "u", "⇥", "x", "←", "→", "]", "[", "/", "z"}
	st := boardFixture(t)

	m, _ := flintBoard(t, 80, 34)
	ks := m.beltKeys()
	if want := append([]string{"◂", "1", "2", "↑", "↓", "⏎"}, tail...); !slices.Equal(beltFaces(ks), want) {
		t.Fatalf("flint-auth's belt is %v, want %v", beltFaces(ks), want)
	}
	for i, n := range []string{"1", "2"} {
		if k := ks[1+i]; k != (beltKey{Key: n, Face: n, Word: "answer", On: true, Act: true}) {
			t.Errorf("key %d is %+v, want %s answer in the accent", 1+i, k, n)
		}
	}
	if g := beltKeyOf(t, ks, "g"); !g.On || g.Word != "chain" {
		t.Errorf("flint-auth is in chain login, yet g is %+v", g)
	}
	for _, k := range ks[3:] {
		if k.Act {
			t.Errorf("%s is in the accent: only the card's keys are", k.Face)
		}
	}

	kindle := boardShot(t, st, 80, 34, chooseCard("kindle-web"))
	ks = kindle.beltKeys()
	if want := append([]string{"◂", "p", "↑", "↓", "⏎"}, tail...); !slices.Equal(beltFaces(ks), want) {
		t.Fatalf("kindle-web's belt is %v, want %v", beltFaces(ks), want)
	}
	if p := ks[1]; p != (beltKey{Key: "p", Face: "p", Word: "merge", On: true, Act: true}) {
		t.Errorf("kindle-web's ship waits, yet its key is %+v", p)
	}
	if g := beltKeyOf(t, ks, "g"); g.On {
		t.Errorf("kindle-web is in no chain, yet g is %+v", g)
	}
	words := map[string]string{"c": "copy", "m": "menu", "g": "chain", "v": "one line", "s": "hosts", "u": "strip", "⇥": "machine", "x": "kill", "z": "park"}
	for face, word := range words {
		if k := beltKeyOf(t, ks, face); k.Word != word {
			t.Errorf("%s says %q, want %q", face, k.Word, word)
		}
	}

	box := pressOn(kindle, "p")
	want := []beltKey{{Face: "◂", On: true}, {Key: "y", Face: "y", Word: "runs", On: true, Act: true}, {Key: "esc", Face: "esc", Word: "cancels", On: true}}
	if got := box.beltKeys(); !slices.Equal(got, want) {
		t.Errorf("with the consent box up the belt is %+v, want %+v", got, want)
	}
}

// TestTheBeltSignalSaysTheBoardsKeys: the console tells the app the
// chosen card's keys, an armed one with *, and the dim keys after ;; box
// and menu while they show; "" when it leaves the board. It says a value
// once, and again to new viewers.
func TestTheBeltSignalSaysTheBoardsKeys(t *testing.T) {
	st := boardFixture(t)
	value := func(m Model) string { t.Helper(); return m.beltValue() }

	empty := st
	empty.Sessions, empty.Queue, empty.Ships, empty.Chains = nil, nil, &proto.Ships{At: demoAt}, nil
	if got := value(boardShot(t, empty, 80, 34, nil)); got != ";c,m,g,s,x,],[,z" {
		t.Errorf("no card chosen: %q", got)
	}

	m, clock := flintBoard(t, 80, 34)
	if got := value(m); got != "1,2;" {
		t.Errorf("flint-auth: %q, want %q", got, "1,2;")
	}
	m = pressOn(m, "2")
	*clock = clock.Add(time.Millisecond)
	if got := value(m); got != "1,2*;" {
		t.Errorf("flint-auth with 2 armed: %q, want %q", got, "1,2*;")
	}

	kindle := boardShot(t, st, 80, 34, chooseCard("kindle-web"))
	if got := value(kindle); got != "p;g" {
		t.Errorf("kindle-web, its ship waits and it is in no chain: %q", got)
	}
	if got := value(pressOn(kindle, "m")); got != "menu" {
		t.Errorf("the menu open: %q", got)
	}
	if got := value(pressOn(kindle, "p")); got != "box" {
		t.Errorf("the consent box up: %q", got)
	}
	// Only the box that takes a typed y and esc is box; the take-over
	// question and the scan's consent are home with no card keys, and keep
	// the key line on Pocket.
	held := kindle
	held.held = func(m Model) (tea.Model, tea.Cmd) { return m, nil }
	scan := kindle
	scan.scanAsk, scan.found = &scanAsk{}, &proto.Found{}
	for name, c := range map[string]Model{"the take-over question": held, "the scan's consent": scan} {
		if !c.consenting() {
			t.Fatalf("%s does not show", name)
		}
		if got := value(c); got != ";c,m,g,v,s,u,tab,x,left,right,],[,/,z" {
			t.Errorf("%s: %q, want the home with no card keys", name, got)
		}
		if ks := c.beltKeys(); slices.ContainsFunc(ks, func(k beltKey) bool { return k.Act }) {
			t.Errorf("%s: card keys on the belt: %v", name, beltFaces(ks))
		}
		pocketOnly(&c)
		if c.draw(80).keysAt < 0 {
			t.Errorf("%s on Pocket: no key line", name)
		}
	}

	// A card on another host takes no digit: its digits are dim.
	far := st
	far.Sessions = slices.Clone(st.Sessions)
	far.Queue = slices.Clone(st.Queue)
	for i := range far.Sessions {
		if far.Sessions[i].Name == "flint-auth" {
			far.Sessions[i].Host = "gpu-1"
		}
	}
	for i := range far.Queue {
		if far.Queue[i].Name == "flint-auth" {
			far.Queue[i].Host = "gpu-1"
		}
	}
	if got := value(boardShot(t, far, 80, 34, chooseCard("flint-auth"))); !strings.HasPrefix(got, ";1,2") {
		t.Errorf("flint-auth on gpu-1: %q, want its digits dim", got)
	}

	off := kindle
	off.tab = tabHistory
	if got := value(off); got != "" {
		t.Errorf("the board off: %q", got)
	}

	// Sent once for each change, through the agent signal's way.
	next, cmd := kindle.Update(noticeMsg{})
	if got := raws(cmd); !slices.Contains(got, logos.Passthrough(keycmd.BoardSignal("p;g"))) {
		t.Fatalf("the first frame wrote %q, want board=p;g through tmux", got)
	}
	next, cmd = next.Update(noticeMsg{})
	if got := boardSaid(cmd); len(got) != 0 {
		t.Errorf("an unchanged value went again: %q", got)
	}
	next, cmd = next.Update(graphicsMsg{gen: kindle.opt.Gen, watched: true, key: "/dev/pts/7:105@here"})
	if got := boardSaid(cmd); !slices.Equal(got, []string{"p;g"}) {
		t.Errorf("new viewers got %q, want p;g", got)
	}
	next, cmd = next.Update(uv.UnknownApcEvent(keycmd.ViewerKey))
	if got := boardSaid(cmd); !slices.Equal(got, []string{"p;g"}) {
		t.Errorf("a terminal that came got %q, want p;g", got)
	}
	nm := next.(Model)
	nm.tab = tabHistory
	next, cmd = nm.Update(noticeMsg{})
	if got := boardSaid(cmd); !slices.Equal(got, []string{""}) {
		t.Errorf("leaving the board wrote %q, want one empty value", got)
	}
	for range 2 {
		if next, cmd = next.Update(noticeMsg{}); len(boardSaid(cmd)) != 0 {
			t.Errorf("the board is off, yet the console wrote %q", boardSaid(cmd))
		}
	}
	// Every console opens on the board now (Task 12 step 1): none is
	// without one.
}

// TestTheBeltValueKeepsItsLimit: a value never passes beltMax bytes; the
// dim keys leave whole, from the right, and the card's keys stay.
func TestTheBeltValueKeepsItsLimit(t *testing.T) {
	ks := []beltKey{{Face: "◂", On: true}, {Key: "1", Act: true, On: true}, {Key: "2", Act: true, On: true}, {Key: "3", Act: true, On: true}}
	for i := range 20 {
		ks = append(ks, beltKey{Key: fmt.Sprintf("long%02d", i)})
	}
	got := beltWord(ks, "3")
	if len(got) > beltMax {
		t.Fatalf("%q has %d bytes, more than %d", got, len(got), beltMax)
	}
	card, dims, ok := strings.Cut(got, ";")
	if !ok || card != "1,2,3*" {
		t.Fatalf("%q: want the card's keys 1,2,3* before ;", got)
	}
	for i, d := range strings.Split(dims, ",") {
		if d != fmt.Sprintf("long%02d", i) {
			t.Fatalf("dim key %d is %q in %q: want whole keys, in order", i, d, got)
		}
	}
	if beltWord(ks, "") == beltWord(ks, "3") {
		t.Error("the armed key does not show")
	}
}

// TestPocketExpiresWhenTheBeltLeavesBoard (controller ruling 22): while
// on Pocket, each viewers poll sends the value again as a new value; an
// app that answers keeps Pocket, one whose belt left Board stops
// answering and loses it within one poll plus beltAnswerWait, with no
// state change. With a desk viewing nothing is sent again.
func TestPocketExpiresWhenTheBeltLeavesBoard(t *testing.T) {
	st := boardFixture(t)
	clock := demoAt
	m := boardShot(t, st, 40, 34, func(m *Model) { m.opt.Now = func() time.Time { return clock } })
	var cmd tea.Cmd
	update := func(msgs ...tea.Msg) {
		t.Helper()
		for _, msg := range msgs {
			var next tea.Model
			next, cmd = m.Update(msg)
			m = next.(Model)
		}
	}
	phone := "/dev/pts/7:105@here"
	poll := func(key string) tea.Msg { return graphicsMsg{gen: m.opt.Gen, watched: true, key: key} }
	answer := uv.UnknownApcEvent(keycmd.BeltFromPrefix + "/dev/pts/7\x1b\\")
	update(poll(phone), answer)
	if !m.onPocket() {
		t.Fatal("the app answered: not on Pocket")
	}
	value := m.belt.value

	for range 3 { // an app on Board answers every poll's value
		clock = clock.Add(graphicsEvery)
		update(poll(phone))
		if got := boardSaid(cmd); !slices.Equal(got, []string{value}) {
			t.Fatalf("a poll on Pocket wrote %q, want %q again", got, value)
		}
		update(answer)
		if !m.onPocket() {
			t.Fatal("the app answers each value, yet the console left Pocket")
		}
	}

	// The belt leaves Board: no answer, and nothing on the board changes.
	clock = clock.Add(graphicsEvery)
	update(poll(phone))
	if len(boardSaid(cmd)) != 1 || !m.onPocket() {
		t.Fatalf("the poll wrote %q; on Pocket %v while the answer may come", boardSaid(cmd), m.onPocket())
	}
	clock = clock.Add(beltAnswerWait)
	if m.onPocket() {
		t.Fatal("no answer for a poll plus the grace, yet the console stays on Pocket")
	}
	clock = clock.Add(graphicsEvery)
	if update(poll(phone)); len(boardSaid(cmd)) != 0 {
		t.Errorf("off Pocket, a poll sent the unchanged value: %q", boardSaid(cmd))
	}

	// A late answer to a value counts for the next, but not past one poll.
	update(answer)
	if !m.onPocket() {
		t.Fatal("the app answered again: not on Pocket")
	}
	clock = clock.Add(graphicsEvery)
	update(poll(phone)) // sent again: the app is gone from Board and says nothing
	clock = clock.Add(beltAnswerWait / 2)
	update(answer) // late: it answers the value before, and counts for this one
	clock = clock.Add(graphicsEvery)
	update(poll(phone))
	clock = clock.Add(beltAnswerWait)
	if m.onPocket() {
		t.Error("a late answer held Pocket past the next poll")
	}

	desk := boardShot(t, st, 40, 34, func(m *Model) { m.opt.Now = func() time.Time { return clock } })
	next, _ := desk.Update(poll("/dev/pts/2:90@here," + phone))
	next, _ = next.Update(answer)
	clock = clock.Add(graphicsEvery)
	if _, c := next.Update(poll("/dev/pts/2:90@here," + phone)); len(boardSaid(c)) != 0 {
		t.Errorf("with a desk viewing, a poll sent the unchanged value: %q", boardSaid(c))
	}
}

// TestATerminalThatComesWaitsForTheViewers: on ViewerKey the console asks
// who views it at once, and counts no answer until that reply came; then
// every viewer must answer.
func TestATerminalThatComesWaitsForTheViewers(t *testing.T) {
	st := boardFixture(t)
	viewers := "/dev/pts/7:105@here"
	m := boardShot(t, st, 40, 34, func(m *Model) {
		m.opt.Graphics = func() (bool, string, bool) { return false, viewers, true }
	})
	answer := uv.UnknownApcEvent(keycmd.BeltFromPrefix + "/dev/pts/7\x1b\\")
	update := func(msg tea.Msg) tea.Cmd {
		t.Helper()
		next, cmd := m.Update(msg)
		m = next.(Model)
		return cmd
	}
	update(graphicsMsg{gen: m.opt.Gen, watched: true, key: viewers})
	update(answer)
	if !m.onPocket() {
		t.Fatal("the app answered: not on Pocket")
	}

	viewers = "/dev/pts/2:90@here,/dev/pts/7:105@here" // a desk attaches
	cmd := update(uv.UnknownApcEvent(keycmd.ViewerKey))
	var asked []graphicsMsg
	for _, msg := range cmdMsgs(cmd) {
		if g, ok := msg.(graphicsMsg); ok {
			asked = append(asked, g)
		}
	}
	if len(asked) != 1 {
		t.Fatalf("a terminal came, yet the console did not ask who views it: %v", cmdMsgs(cmd))
	}
	update(answer) // the app answers at once, before the viewers are known
	if m.onPocket() {
		t.Fatal("the app answered before the viewers were known: on Pocket with a desk there")
	}
	update(asked[0])
	update(answer)
	if m.onPocket() || m.draw(40).keysAt < 0 {
		t.Fatal("the desk views the console, yet the key line is gone")
	}

	viewers = "/dev/pts/7:105@here" // the desk leaves; the app came again
	cmd = update(uv.UnknownApcEvent(keycmd.ViewerKey))
	for _, msg := range cmdMsgs(cmd) {
		if g, ok := msg.(graphicsMsg); ok {
			update(g)
		}
	}
	update(answer)
	if !m.onPocket() {
		t.Error("only the app views the console and it answered: not on Pocket")
	}
}

// remoteAuth is the fixture with flint-auth, and its question, on gpu-1.
func remoteAuth(t *testing.T) proto.State {
	t.Helper()
	st := boardFixture(t)
	for i, s := range st.Sessions {
		if s.Name == "flint-auth" {
			st.Sessions[i].Host = "gpu-1"
			for j, it := range st.Queue {
				if it.Pane == s.Pane {
					st.Queue[j].Host = "gpu-1"
				}
			}
		}
	}
	return st
}

func TestARemoteCardOffersNoDigits(t *testing.T) {
	for _, size := range [][2]int{{40, 34}, {80, 31}, {104, 38}} {
		m := boardShot(t, remoteAuth(t), size[0], size[1], chooseCard("flint-auth"))
		if s, ok := m.boardSelected(); !ok || s.Name != "flint-auth" || s.Host != "gpu-1" {
			t.Fatalf("%dx%d: the chosen card is %s on %q", size[0], size[1], s.Name, s.Host)
		}
		frame := plainFrame(m)
		for _, no := range []string{"1-2", "1. Patch the token check", "2. Move to the new session store"} {
			if strings.Contains(frame, no) {
				t.Errorf("%dx%d: a card on gpu-1 offers %q:\n%s", size[0], size[1], no, frame)
			}
		}
		if keys := keyLineOf(m); size[0] > 40 && !strings.HasPrefix(keys, "⏎ go in") {
			t.Errorf("%dx%d: the key line of a card on gpu-1 is %q", size[0], size[1], keys)
		}
		if !strings.Contains(frame, "⏎ go in") {
			t.Errorf("%dx%d: the card on gpu-1 shows no ⏎ go in:\n%s", size[0], size[1], frame)
		}
	}
}

func TestEveryEnterHintSaysGoIn(t *testing.T) {
	for _, size := range [][2]int{{40, 34}, {80, 31}, {104, 38}, {160, 38}} {
		m := boardShot(t, boardFixture(t), size[0], size[1], nil)
		if frame := plainFrame(m); strings.Contains(frame, "⏎ answer") {
			t.Errorf("%dx%d: a card says ⏎ answer:\n%s", size[0], size[1], frame)
		}
	}

	// A tap on the key row of a card that is not chosen chooses it, and
	// never goes in.
	m, _, _ := menuBoard(t, 40, 34, nil)
	lines := strings.Split(plainFrame(m), "\n")
	_, ny := cardOn(t, m, "flint-auth")
	ky := -1
	for y := ny + 1; y < len(lines); y++ {
		if strings.Contains(lines[y], "⏎ ") {
			ky = y
			break
		}
	}
	if ky < 0 {
		t.Fatalf("flint-auth draws no key row:\n%s", plainFrame(m))
	}
	kx := ansi.StringWidth(lines[ky][:strings.Index(lines[ky], "⏎")])
	next, cmd := m.Update(tea.MouseClickMsg{X: kx, Y: ky, Button: tea.MouseLeft})
	m = next.(Model)
	if wentIn(cmdMsgs(cmd), paneNamed(boardFixture(t), "flint-auth")) || chosenName(t, m) != "flint-auth" {
		t.Errorf("a tap on flint-auth's ⏎ go in: chosen %s, went in %v", chosenName(t, m), cmdMsgs(cmd))
	}
}

func TestADigitThatCannotActSaysWhy(t *testing.T) {
	for _, c := range []struct{ card, digit, said string }{
		{"hearth-api", "1", "hearth-api asks in its pane · ⏎ goes in"},
		{"flint-auth", "9", "1-2 only"},
		{"flint-auth", "0", "1-2 only"},
		{"flint-ui", "1", "flint-ui asks no question"},
	} {
		m, _, clock := menuBoard(t, 80, 31, chooseCard(c.card))
		*clock = clock.Add(time.Second) // past the dead time
		m = pressOn(m, c.digit)
		if got := noticeRow(m); got != c.said {
			t.Errorf("%s on %s says %q, want %q", c.digit, c.card, got, c.said)
		}
		if armed, _ := m.peekMarks(matchNamed(t, m, c.card)); armed != 0 {
			t.Errorf("%s on %s armed %d", c.digit, c.card, armed)
		}
	}
}

// TestATapOnPocketChoosesTheCard: Pocket is worked by touch alone, so a
// tap on a card chooses it there as on a desk.
func TestATapOnPocketChoosesTheCard(t *testing.T) {
	for _, w := range []int{40, 132} {
		m := boardShot(t, boardFixture(t), w, 34, pocketOnly)
		if !m.beltOnly() {
			t.Fatalf("%d: the fixture is not on Pocket", w)
		}
		lines := strings.Split(plainFrame(m), "\n")
		y := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, " flint-auth ") })
		if y < 0 || chosenName(t, m) == "flint-auth" {
			t.Fatalf("%d: flint-auth is not an unchosen card on the frame:\n%s", w, strings.Join(lines, "\n"))
		}
		x := strings.Index(lines[y], "flint-auth")
		next, cmd := tap(m, len([]rune(lines[y][:x]))+1, y)
		drain(cmd)
		if got := chosenName(t, next.(Model)); got != "flint-auth" {
			t.Errorf("%d: a tap on flint-auth chose %s", w, got)
		}
	}
}

// TestATapBelowTheBoardsBodyChoosesNoCard: a tap under the body (the key
// line's hairline, the key line off its keys, the strip off its words,
// past the frame where the belt is) chooses no card and opens nothing, on
// a desk and on Pocket, at 40 and 132 columns.
func TestATapBelowTheBoardsBodyChoosesNoCard(t *testing.T) {
	for _, w := range []int{40, 132} {
		for _, pocket := range []bool{false, true} {
			var edit func(*Model)
			if pocket {
				edit = pocketOnly
			}
			m := boardShot(t, boardFixture(t), w, 34, edit)
			ran := recordRuns(&m)
			sc := m.draw(w)
			chosen := chosenName(t, m)
			ys := []int{len(sc.lines), len(sc.lines) + 3}
			if sc.keysAt >= 0 {
				ys = append(ys, sc.keysAt-1, sc.keysAt)
			}
			if sc.stripAt > 0 {
				ys = append(ys, sc.stripAt)
			}
			for _, y := range ys {
				for _, x := range []int{0, w / 2, w - 1} {
					if _, onKey := spanAt(sc.keys, x); y == sc.keysAt && onKey {
						continue
					}
					if _, onStrip := spanAt([]span{sc.strip}, x); y == sc.stripAt && onStrip {
						continue
					}
					next, cmd := tap(m, x, y)
					drain(cmd)
					nm := next.(Model)
					if s, ok := nm.boardSelected(); !ok || s.Name != chosen {
						t.Errorf("w %d pocket %v: a tap at %d,%d chose %s", w, pocket, x, y, s.Name)
					}
					if nm.pending != nil || nm.menu != nil || nm.form != nil || nm.sheet != nil || nm.standIn || nm.room != nil ||
						len(boardFake(nm).acts()) != 0 || len(*ran) != 0 {
						t.Errorf("w %d pocket %v: a tap at %d,%d opened or sent something", w, pocket, x, y)
					}
				}
			}
		}
	}
}

// TestATabsSpanPressesNoKey: a tap on a tab's name opens the tab, and the
// span says so without a key: digits answer a card's question on the
// board (ruling 37), so no tab span may press one.
func TestATabsSpanPressesNoKey(t *testing.T) {
	m := boardShot(t, boardFixture(t), 132, 34, nil)
	sc := m.draw(132)
	machine := -1
	for i, s := range sc.tabs {
		if len(s.press) == 1 && s.press >= "0" && s.press <= "9" {
			t.Errorf("tab span %d presses the digit %q", i, s.press)
		}
		if s.press == tabPress(tabMachine) {
			machine = i
		}
	}
	if machine < 0 {
		t.Fatalf("no span opens machine: %+v", sc.tabs)
	}
	next, cmd := tap(m, sc.tabs[machine].x0, 1)
	drain(cmd)
	if nm := next.(Model); nm.tab != tabMachine || len(boardFake(nm).acts()) != 0 {
		t.Errorf("a tap on machine: tab %d, sent %+v", nm.tab, boardFake(nm).acts())
	}
}

func TestMediumDownCrossesIntoTheNextSection(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, chooseCard("flint-auth")) // the last wait
	if m = pressOn(m, "down"); chosenName(t, m) != "flint-ui" || m.boardAt != colWork {
		t.Fatalf("↓ from the last wait chooses %s in %s, want flint-ui in WORKING", chosenName(t, m), m.boardAt.name())
	}
	if m = pressOn(m, "up"); chosenName(t, m) != "flint-auth" || m.boardAt != colNeed {
		t.Errorf("↑ from the first working match chooses %s in %s, want flint-auth in NEEDS YOU", chosenName(t, m), m.boardAt.name())
	}
	m = pressOn(boardShot(t, boardFixture(t), 80, 31, chooseCard("spark-docs")), "down")
	if chosenName(t, m) != "cinder-ci" || m.boardAt != colShip {
		t.Errorf("↓ from the last working match chooses %s, want cinder-ci", chosenName(t, m))
	}
	m = pressOn(boardShot(t, boardFixture(t), 80, 31, func(m *Model) { m.flipHidden(colWork); chooseCard("flint-auth")(m) }), "down")
	if chosenName(t, m) != "cinder-ci" {
		t.Errorf("↓ from the last wait with WORKING hidden chooses %s, want cinder-ci", chosenName(t, m))
	}

	w := pressOn(boardShot(t, boardFixture(t), 104, 38, chooseCard("flint-auth")), "down")
	if w.boardAt != colNeed || w.boardBelow != 1 {
		t.Errorf("on Wide ↓ from a column's last card leaves its column: open %s, below %d", w.boardAt.name(), w.boardBelow)
	}
}

func TestMediumDownReachesEachCardOfAOneRowSection(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 24, chooseCard("flint-ui")) // a short screen leaves WORKING one row
	for _, want := range []string{"ember-billing", "spark-docs"} {
		m = pressOn(m, "down")
		_, plain, at := sectionHeads(t, m)
		if chosenName(t, m) != want || !strings.Contains(plain[at[colWork]+1], want) {
			t.Fatalf("↓ in the one-row WORKING chooses %s, row %q; want %s", chosenName(t, m), plain[at[colWork]+1], want)
		}
	}
}

func TestMediumArrowsMoveBetweenSections(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, nil)
	if m = pressOn(m, "right"); m.boardAt != colWork {
		t.Errorf("→ from NEEDS YOU opens %s, want WORKING", m.boardAt.name())
	}
	if m = pressOn(m, "left"); m.boardAt != colNeed {
		t.Errorf("← from WORKING opens %s, want NEEDS YOU", m.boardAt.name())
	}
	m = boardShot(t, boardFixture(t), 80, 31, func(m *Model) { m.flipHidden(colWork) })
	if m = pressOn(m, "right"); m.boardAt != colShip {
		t.Errorf("→ with WORKING hidden opens %s, want SHIPPING", m.boardAt.name())
	}
}

func TestMediumTapOnASectionHidesIt(t *testing.T) {
	for _, c := range []boardCol{colNeed, colWork, colShip} {
		m := tapHead(t, boardShot(t, boardFixture(t), 80, 31, nil), c)
		if !m.colHidden(c) {
			t.Errorf("a tap on the head of %s does not hide it", c.name())
		}
		if c == colWork && !strings.Contains(strings.Split(plainFrame(m), "\n")[1], "+ work 3") {
			t.Errorf("the tab row has no + work 3:\n%s", plainFrame(m))
		}
	}
}

func TestMediumKeyLineSaysSections(t *testing.T) {
	if got := keyLineOf(boardShot(t, boardFixture(t), 80, 31, nil)); !strings.Contains(got, "←→ sections") {
		t.Errorf("the key line at 80x31 is %q, want ←→ sections", got)
	}
	if got := keyLineOf(boardShot(t, boardFixture(t), 104, 38, nil)); !strings.Contains(got, "←→ columns") {
		t.Errorf("the key line at 104x38 is %q, want ←→ columns", got)
	}
}
