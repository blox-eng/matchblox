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

// pocketOnly makes the app the only viewer, one that answered the board's
// keys with a mark.
func pocketOnly(m *Model) {
	m.viewers, m.belt.value, m.belt.n = "/dev/pts/7:105@here", ";", 1
	m.belt.pocket = []pocketClient{{"/dev/pts/7:105", 1}}
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
	off.board = false
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

	off := boardShot(t, st, 40, 34, func(m *Model) { m.board = false })
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
	off.board = false
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
	nm.board = false
	next, cmd = nm.Update(noticeMsg{})
	if got := boardSaid(cmd); !slices.Equal(got, []string{""}) {
		t.Errorf("leaving the board wrote %q, want one empty value", got)
	}
	for range 2 {
		if next, cmd = next.Update(noticeMsg{}); len(boardSaid(cmd)) != 0 {
			t.Errorf("the board is off, yet the console wrote %q", boardSaid(cmd))
		}
	}
	never := boardShot(t, st, 80, 34, func(m *Model) { m.board = false })
	if _, cmd := never.Update(graphicsMsg{gen: never.opt.Gen, watched: true, key: "/dev/pts/7:105@here"}); len(boardSaid(cmd)) != 0 {
		t.Errorf("a console with no board wrote %q", boardSaid(cmd))
	}
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
