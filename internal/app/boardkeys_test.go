package app

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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
		if m = pressOn(m, "right"); m.boardAt != colQuiet {
			t.Errorf("w=%d: → at the last column moves to %v", w, m.boardAt)
		}
		if m = pressOn(m, "left"); m.boardAt != colNeed {
			t.Errorf("w=%d: ← from QUIET lands on %v", w, m.boardAt)
		}
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
	m = boardShot(t, st, 80, 31, func(m *Model) {
		chooseCard("flint-auth")(m)
		m.opt.Now = func() time.Time { return clock }
	})
	m = pressOn(m, "esc") // the first frame lands the peek
	clock = clock.Add(time.Minute)
	m = pressOn(m, "down")
	if got := chosenName(t, m); got != "flint-auth" || !m.peekAt.Equal(demoAt) {
		t.Errorf("↓ at the bottom: %s, the peek started again at %v", got, m.peekAt)
	}
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
	m := boardShot(t, boardFixture(t), 80, 31, nil)
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
		{"c", "c copies now. The stoker is on its card.", "copied: the last message of spark-search"},
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
	goAt := func(m Model) (int, int) {
		t.Helper()
		for y, l := range strings.Split(plainFrame(m), "\n") {
			if x := strings.Index(l, "y give the go"); x >= 0 {
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

	m := tapGo(goBoard(t, &now, nil))
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
		m = tapGo(goBoard(t, &now, nil))
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

func TestTheLiveViewIsSizedAsDrawn(t *testing.T) {
	st := boardFixture(t)
	m := boardShot(t, st, 160, 38, chooseCard("flint-auth"))
	cols, rows := m.viewSize()
	if _, live := boardWidths(Wide, 160, boardCols[:]); cols != live-1 || rows < 10 {
		t.Fatalf("viewSize %d x %d beside a live view %d wide", cols, rows, live)
	}
	var text strings.Builder
	for i := 1; i <= rows; i++ {
		fmt.Fprintf(&text, "row %d\n", i)
	}
	m.screen = proto.Screen{Pane: paneNamed(st, "flint-auth"), At: demoAt, Cols: cols, Rows: rows, Text: text.String()}
	f := strings.Join(plainLines(m.boardPanel(160, m.bodyRows()).lines), "\n")
	if !strings.Contains(f, " row 1 ") && !strings.HasSuffix(f, " row 1") || !strings.Contains(f, fmt.Sprintf("row %d", rows)) {
		t.Errorf("a screen of %d rows is not whole in the live view:\n%s", rows, f)
	}
	if m = boardShot(t, st, 151, 38, nil); func() bool { c, r := m.viewSize(); return c != 0 || r != 0 }() {
		t.Error("a size for a live view at 151 columns")
	}
}
