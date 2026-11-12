package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// The board's tests of the chains and their rooms (Task 12 step 0c).

// chainBoard is the board of st at w columns, on the demo's clock, with
// the card of the match named name chosen ("" for the board's choice).
func chainBoard(t *testing.T, w int, st proto.State, name string) (Model, *fakeConn) {
	t.Helper()
	m, f := headModel(t, w, st)
	m = boardOn(m)
	if name != "" {
		s := matchOf(t, m, name)
		m.boardChooseCard(m.columnOf(s), s)
	}
	return m, f
}

// stubOf is the chain's stub on the card of the match named name, ""
// for none.
func stubOf(t *testing.T, m Model, name string) string {
	t.Helper()
	lines, _ := m.boardCard(matchOf(t, m, name), 40, cardOpts{})
	for _, l := range plainLines(lines) {
		if i := strings.IndexAny(l, "─━"); i >= 0 && strings.ContainsAny(l, "╴╸") {
			return strings.TrimSpace(l[i:])
		}
	}
	return ""
}

// TestACardOfAChainOfOneHasNoStub: a chain with one match here draws no
// chain mark; with two, each card has its stub.
func TestACardOfAChainOfOneHasNoStub(t *testing.T) {
	st := fuseState(t, mockupChain)
	left := func(gone ...string) proto.State {
		out := st
		out.Sessions = slices.DeleteFunc(slices.Clone(st.Sessions), func(s sample.Session) bool { return slices.Contains(gone, s.Name) })
		return out
	}
	m, _ := chainBoard(t, 160, left("hearth-api", "ember-billing"), "")
	if stub := stubOf(t, m, "kindle-web"); stub != "" {
		t.Errorf("a chain of one has the stub %q", stub)
	}
	if names := m.boardChains(); len(names) != 0 {
		t.Errorf("a chain of one is a chain of the board: %v", names)
	}
	m, _ = chainBoard(t, 160, left("ember-billing"), "")
	for _, name := range []string{"kindle-web", "hearth-api"} {
		if stub := stubOf(t, m, name); stub == "" {
			t.Errorf("%s's card in a chain of two has no stub", name)
		}
	}
}

// TestFOnACardOfABareChainOpensTheSheet: F on a card of a chain with no
// stoker opens its start sheet; on a card in no chain it opens nothing;
// at night it says the night is sealed and sends nothing.
func TestFOnACardOfABareChainOpensTheSheet(t *testing.T) {
	m, _ := chainBoard(t, 160, fuseState(t, mockupChain), "hearth-api")
	if s := plainFrame(sendKey(m, "F").(Model)); !strings.Contains(s, "Stoke the chain kindle-web (3 matches)?") {
		t.Fatalf("F on a card of a bare chain opened no sheet:\n%s", s)
	}
	m, _ = chainBoard(t, 160, fuseState(t, mockupChain), "flint-auth")
	if next := sendKey(m, "F").(Model); next.sheet != nil {
		t.Error("F opened a sheet on a card in no chain")
	}

	st := fuseState(t, mockupChain)
	host := proto.StokerView{}
	host.On, host.Nightly = true, true
	st.Stoker = &host
	m, f := chainBoard(t, 160, st, "hearth-api")
	if next := sendKey(m, "F").(Model); next.sheet != nil || next.flash != sealedFamily || len(f.acts()) != 0 {
		t.Errorf("F at night: sheet %v, flash %q, acts %+v", next.sheet, next.flash, f.acts())
	}
}

// TestTheRoomCardCountsTheOrdersSteps: the room's stoker card counts the
// steps of the order in flight from its go: a step done before the go is
// no step of it.
func TestTheRoomCardCountsTheOrdersSteps(t *testing.T) {
	marks := func(v proto.StokerView) string {
		m, k := roomOf(t, headState(t, v, true), "kindle-web")
		var out []string
		for _, r := range m.cardRows(k) {
			if r.kind == roomStep {
				out = append(out, r.id+r.text[:strings.Index(r.text, " ")])
			}
		}
		return strings.Join(out, " ")
	}
	v := runningPlan() // steps 1 and 2 done, an hour ago
	if got := marks(v); got != "2✓ 3▸ 4" {
		t.Errorf("the card's steps: %q, want 2✓ 3▸ 4", got)
	}
	v = runningPlan()
	v.Order.Went = demoAt.Add(-30 * time.Minute) // the order went again after them
	if got := marks(v); got != "1▸ 2" {
		t.Errorf("after a new go the card's steps: %q, want the first step active", got)
	}
}

// The tests the map names that need a later step of Task 12.

// TestTheConsoleOpensOnTheBoard: the console's first frame is the board,
// on the first match that waits, and the first tab's keys are the board's.
func TestTheConsoleOpensOnTheBoard(t *testing.T) {
	m, _ := loadedWith(t, 160, fuseState(t, mockupChain))
	if m.tab != tabSessions || !m.boardShown() {
		t.Fatalf("the console opens on tab %d, board %v", m.tab, m.boardShown())
	}
	if got := chosenName(t, m); got != "kindle-web" {
		t.Errorf("the console opens on %s, want kindle-web: it waits", got)
	}
	frame := plainFrame(m)
	for _, want := range []string{"NEEDS YOU · 1", "WORKING · 2", "kindle-web", "flint-auth"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the first frame lacks %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "WAITS FOR YOU") {
		t.Errorf("the first frame draws the Sessions list:\n%s", frame)
	}
	if got := plainHints(m.view().keys(m)...); got != plainHints(m.boardHints(160)...) {
		t.Errorf("the first tab's keys are %q, want the board's", got)
	}
}

// TestTheTabRowIsBoardAndMachine: two tabs, the board and Machine; Git,
// History, Panes and the stoker open from Machine's rows. Tab and
// Shift+Tab turn between the two.
func TestTheTabRowIsBoardAndMachine(t *testing.T) {
	if !slices.Equal(tabNames, []string{"board", "machine"}) {
		t.Fatalf("tabs %q, want board and machine", tabNames)
	}
	m, _ := loadedWith(t, 100, queueState())
	for tab, want := range map[int][]string{tabSessions: {"BOARD", "3", "machine"}, tabMachine: {"board", "3", "MACHINE"}} {
		on := m
		on.setTab(tab)
		if row := strings.Fields(strings.Split(screen(on), "\n")[1]); !slices.Equal(row, want) {
			t.Errorf("tab %d: the tab row is %q, want %q", tab, row, want)
		}
	}
	if next := pressOn(m, "tab"); next.tab != tabMachine {
		t.Fatalf("tab on the board opens tab %d", next.tab)
	}
	machine := pressOn(m, "tab")
	if pressOn(machine, "tab").tab != tabSessions || pressOn(machine, "shift+tab").tab != tabSessions || pressOn(m, "shift+tab").tab != tabMachine {
		t.Fatal("Tab and Shift+Tab do not turn between the board and Machine")
	}
	// A detached busy loop burns: the tab row says so, Machine's !, on the
	// board and on a view of Machine.
	burns, _ := loadedWith(t, 100, orphanState())
	stoker := burns
	stoker.setTab(tabStoker)
	for _, on := range []Model{burns, stoker} {
		if row := strings.Split(screen(on), "\n")[1]; !strings.Contains(row, "machine !") && !strings.Contains(row, "MACHINE !") {
			t.Errorf("tab %d: no busy loop on the tab row %q", on.tab, row)
		}
	}
	if row := strings.Split(screen(m), "\n")[1]; strings.Contains(row, "!") {
		t.Errorf("no busy loop, and the tab row says one burns: %q", row)
	}
}

// roomRowNow is the room's row the cursor is on, "" when it is on the
// board: its kind's name and its words.
func roomRowNow(m Model) string {
	row, ok := m.roomChosen()
	if !ok {
		return ""
	}
	return map[roomKind]string{roomCard: "card", roomStep: "step", roomMore: "more", roomTalk: "talk"}[row.kind] + " " + row.text
}

// hearthOpen is the board at w with the room of chain hearth open, as g
// on hearth-api opens it, and its conn.
func hearthOpen(t *testing.T, w int, st proto.State) (Model, *fakeConn) {
	t.Helper()
	m, f := chainBoard(t, w, st, "hearth-api")
	m = pressOn(m, "g")
	if m.room == nil || m.boardChain != "hearth" {
		t.Fatalf("g on hearth-api: room %v, chain %q", m.room, m.boardChain)
	}
	return m, f
}

// TestARoomOpensOnTheNeedElseTheTalkLine: the room opens on what waits:
// the card of a match that asks, the step whose go it waits for; with
// nothing, on the talk line, and a flip lands typing there.
func TestARoomOpensOnTheNeedElseTheTalkLine(t *testing.T) {
	m, _ := hearthOpen(t, 160, headState(t, runningPlan(), true))
	if row := roomRowNow(m); row != "" || chosenName(t, m) != "kindle-web" {
		t.Errorf("with kindle-web asking the room opens on %q, card %s", row, chosenName(t, m))
	}

	st := headState(t, aGo(runningPlan(), "merge:#4556", ""), true)
	st.Queue = nil
	m, _ = hearthOpen(t, 160, st)
	if row := roomRowNow(m); row != "step ▸ step · waits for your go" {
		t.Errorf("with a go waiting the room opens on %q", row)
	}
	if !strings.Contains(plainFrame(m), "▌  ▸ step · waits for your go") {
		t.Errorf("the waiting step is not chosen on the frame:\n%s", plainFrame(m))
	}

	st = headState(t, runningPlan(), true)
	st.Queue = nil
	m, _ = hearthOpen(t, 160, st)
	if row := roomRowNow(m); row != "talk › tell hearth's stoker…" || m.room.typing {
		t.Errorf("with nothing waiting the room opens on %q, typing %v", row, m.room.typing)
	}
	if !strings.Contains(plainFrame(m), "▌› tell hearth's stoker…") {
		t.Errorf("the talk line is not chosen on the frame:\n%s", plainFrame(m))
	}
	if _, ok := m.boardSelected(); ok {
		t.Error("a card is chosen while the talk line is")
	}

	m, _ = chainBoard(t, 160, st, "")
	m = m.flipFrom("/dev/pts/2", matchOf(t, m, "hearth-api").Pane)
	if row := roomRowNow(m); row != "talk › tell hearth's stoker…" || !m.room.typing || m.boardChain != "hearth" {
		t.Errorf("a flip lands on %q, typing %v, chain %q", row, m.room.typing, m.boardChain)
	}
}

// TestTheRoomsCursorWalksCardColumnsAndTalk: ↑ from the first card goes up
// the stoker's card, ↓ from the last card to the talk line, then to the
// ghost row and the recommendations; ⏎ unfolds the plan and types on the
// talk line; esc stops typing, then closes the room.
func TestTheRoomsCursorWalksCardColumnsAndTalk(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, _ := hearthOpen(t, 160, st)
	m = pressOn(m, "up")
	if chosenName(t, m) != "hearth-api" || roomRowNow(m) != "" {
		t.Fatalf("↑ from the talk line chose %q, room row %q", chosenName(t, m), roomRowNow(m))
	}
	var walked []string
	for range 6 {
		m = pressOn(m, "up")
		walked = append(walked, roomRowNow(m))
	}
	want := []string{"more ↓ 2 more", "step   step", "step ▸ step", "step ✓ step", "card ≋ hearth's stoker is quiet since 21:50 · ⏎ asks it", `card ≋ "ship the chain"`}
	if !slices.Equal(walked, want) {
		t.Errorf("↑ walks %q, want %q", walked, want)
	}
	if m = pressOn(m, "up"); roomRowNow(m) != `card ≋ "ship the chain"` {
		t.Errorf("↑ from the card's first row with no line above: %q", roomRowNow(m))
	}
	for range 6 {
		m = pressOn(m, "down")
	}
	if roomRowNow(m) != "" || chosenName(t, m) != "hearth-api" {
		t.Fatalf("↓ from the card's last row: room row %q", roomRowNow(m))
	}
	if m = pressOn(m, "down"); roomRowNow(m) != "talk › tell hearth's stoker…" {
		t.Fatalf("↓ from the last card: %q", roomRowNow(m))
	}
	if m = pressOn(m, "down"); roomRowNow(m) != "" {
		t.Fatalf("↓ from the talk line stays on %q", roomRowNow(m))
	}
	if line, ok := m.belowChosen(); !ok || line != "" {
		t.Fatalf("↓ from the talk line chose %q, %v, want the ghost row", line, ok)
	}
	if m = pressOn(m, "down"); func() bool { line, ok := m.belowChosen(); return !ok || line == "" }() {
		t.Fatal("↓ from the ghost row chose no recommendation")
	}
	if m = pressOn(m, "up", "up"); roomRowNow(m) != "talk › tell hearth's stoker…" {
		t.Fatalf("↑ from the ghost row: %q", roomRowNow(m))
	}

	m = pressOn(m, "enter")
	m = typeText(m, "ship it").(Model)
	if !m.room.typing || m.room.draft != "ship it" || !strings.Contains(plainFrame(m), "› ship it▏") {
		t.Fatalf("⏎ on the talk line: typing %v, draft %q", m.room.typing, m.room.draft)
	}
	if m = pressOn(m, "esc"); m.room == nil || m.room.typing {
		t.Fatal("the first esc did not only stop typing")
	}
	if m = pressOn(m, "esc"); m.room != nil || m.boardChain != "" {
		t.Fatalf("the second esc kept the room: chain %q", m.boardChain)
	}

	m, _ = hearthOpen(t, 160, st)
	m = pressOn(m, "up", "up")
	if m = pressOn(m, "enter"); !m.room.unfolded || !strings.Contains(plainFrame(m), "  step") {
		t.Error("⏎ on more does not unfold the plan")
	}
}

// TestANarrowedChainIsItsRoom: the room is the board narrowed to the
// chain, with the stoker's card above the columns and the talk line under
// them; y on the step that waits gives its go through the consent box, and
// a tap on the talk line chooses it, a second one types.
func TestANarrowedChainIsItsRoom(t *testing.T) {
	st := headState(t, aGo(runningPlan(), "merge:#4556", ""), true)
	st.Queue = nil
	m, f := hearthOpen(t, 160, st)
	got := drawnNames(m)
	slices.Sort(got)
	if want := []string{"ember-billing", "hearth-api", "kindle-web"}; !slices.Equal(got, want) {
		t.Errorf("the room's columns show %v, want %v", got, want)
	}
	lines := strings.Split(plainFrame(m), "\n")
	at := func(has string) int {
		return slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, has) })
	}
	step, heads, talk := at("▸ step · waits for your go"), at("NEEDS YOU ·"), at("› tell hearth's stoker…")
	if step < 0 || heads < 0 || talk < 0 || step >= heads || heads >= talk || at("chain hearth · 3   esc all") != 1 {
		t.Fatalf("the card above the columns and the talk line under them (step %d, heads %d, talk %d):\n%s", step, heads, talk, strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[len(lines)-1], "y give") {
		t.Errorf("the key line has no y give: %q", lines[len(lines)-1])
	}
	m = pressOn(m, "y")
	if m.pending == nil || !m.pending.destructive || m.pending.rec != "stoker:grant" || len(f.acts()) != 0 {
		t.Fatalf("y on the waiting step: pending %+v, acts %+v", m.pending, f.acts())
	}
	m = pressOn(m, "y")
	if a := f.acts(); len(a) != 1 || a[0].RecID != "stoker:grant" || a[0].Text != "s1 e9" {
		t.Fatalf("the typed y sent %+v", a)
	}

	m = mouse(t, m, tea.MouseClickMsg{X: 5, Y: talk, Button: tea.MouseLeft})
	m = mouse(t, m, tea.MouseReleaseMsg{X: 5, Y: talk, Button: tea.MouseLeft})
	if roomRowNow(m) != "talk › tell hearth's stoker…" || m.room.typing {
		t.Fatalf("the first tap on the talk line: %q, typing %v", roomRowNow(m), m.room.typing)
	}
	m = mouse(t, m, tea.MouseClickMsg{X: 5, Y: talk, Button: tea.MouseLeft})
	m = mouse(t, m, tea.MouseReleaseMsg{X: 5, Y: talk, Button: tea.MouseLeft})
	if !m.room.typing {
		t.Error("the second tap on the talk line does not type")
	}
}

// TestOnAPhoneTheStokerIsAPage: on a phone the room's stoker is its first
// page, with the card and the talk line; the columns' pages have no card
// above them.
func TestOnAPhoneTheStokerIsAPage(t *testing.T) {
	st := headState(t, runningPlan(), true)
	ember := slices.IndexFunc(st.Sessions, func(s sample.Session) bool { return s.Name == "ember-billing" })
	st.Ships = &proto.Ships{At: demoAt, Ships: []proto.Ship{shipFor(st.Sessions[ember], 4556, shipRunning, "CI · 6 of 9 jobs", 1, time.Hour)}}
	m, _ := hearthOpen(t, 44, st)
	marks := func(m Model) string { return strings.TrimSpace(strings.Split(plainFrame(m), "\n")[3]) }
	if got := marks(m); !strings.HasPrefix(got, "stoker NEEDS YOU 1 −") {
		t.Fatalf("the room opens on kindle-web's page: marks %q", got)
	}
	if strings.Contains(plainFrame(m), "tell hearth's stoker") || strings.Contains(plainFrame(m), "▸ step") {
		t.Errorf("a column's page has the card above it:\n%s", plainFrame(m))
	}
	m = pressOn(m, "up")
	if got := marks(m); !strings.HasPrefix(got, "STOKER  need 1  work 1  ship 1") {
		t.Errorf("↑ from the first card: marks %q, want the stoker's page", got)
	}
	frame := plainFrame(m)
	for _, want := range []string{`≋ "ship the chain"`, "▸ step", "▌› tell hearth's stoker…"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the stoker's page lacks %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "kindle-web") {
		t.Errorf("the stoker's page draws a card:\n%s", frame)
	}
	if m = pressOn(m, "down"); chosenName(t, m) != "kindle-web" || !strings.HasPrefix(marks(m), "stoker NEEDS YOU 1 −") {
		t.Errorf("↓ from the talk line: marks %q", marks(m))
	}
	m = mouse(t, m, tea.MouseClickMsg{X: 2, Y: 3, Button: tea.MouseLeft})
	m = mouse(t, m, tea.MouseReleaseMsg{X: 2, Y: 3, Button: tea.MouseLeft})
	if got := marks(m); !strings.HasPrefix(got, "STOKER") {
		t.Errorf("a tap on the stoker's mark: marks %q", got)
	}
}

// itemLabels are the words of a menu's lines.
func itemLabels(items []menuItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.label)
	}
	return out
}

// menuLine is the index of the line of s's menu that says label, -1 for
// none.
func menuLine(m Model, s sample.Session, label string) int {
	return slices.IndexFunc(m.menuFor(s), func(it menuItem) bool { return it.label == label })
}

// outOfChain runs line i of s's menu: the card chosen, the menu open, ↓ to
// it, ⏎.
func outOfChain(m Model, s sample.Session, i int) Model {
	m.boardChooseCard(m.columnOf(s), s)
	m = m.openMenu(s, 0, 5)
	for range i {
		m = pressOn(m, "down")
	}
	return pressOn(m, "enter")
}

// TestTheMenuTakesAMatchOutOfItsChain: in a chain's room the card's menu
// has "out of <chain>" (the owner, 2026-11-12: "Menu, in the room only").
// ⏎ on it asks for a typed y, the act named, and sends nothing before it.
// The full board's menu has no such line (TestTheMenuHasNoDestructiveLine),
// and nor has a chain of one's room.
func TestTheMenuTakesAMatchOutOfItsChain(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, f := hearthOpen(t, 160, st)
	s := matchOf(t, m, "ember-billing")
	i := menuLine(m, s, "out of hearth")
	if i < 0 {
		t.Fatalf("the room's menu lacks out of hearth: %q", itemLabels(m.menuFor(s)))
	}
	if full := m.closeRoom(); slices.ContainsFunc(full.menuFor(s), func(it menuItem) bool { return strings.HasPrefix(it.label, "out of") }) {
		t.Fatalf("the full board's menu takes a match out: %q", itemLabels(full.menuFor(s)))
	}
	asked := outOfChain(m, s, i)
	if len(f.acts()) != 0 || asked.pending == nil || !asked.pending.destructive || asked.pending.label != "take ember-billing out of hearth" {
		t.Fatalf("⏎ on out of hearth: acts %+v, pending %+v", f.acts(), asked.pending)
	}
	pressOn(asked, "y")
	if a := f.acts(); len(a) != 1 || a[0].RecID != "chain:out:%8@hearth" || a[0].Confirm != "y" {
		t.Fatalf("y: acts %+v", a)
	}

	// A tap that clears the narrowing closes the room: esc all on the tab
	// row, and the count in the header. The line goes with it.
	asks, _ := hearthOpen(t, 160, headState(t, runningPlan(), true)) // kindle-web asks: the header counts it
	if menuLine(asks, s, "out of hearth") < 0 {
		t.Fatalf("the room's menu lacks out of hearth: %q", itemLabels(asks.menuFor(s)))
	}
	sc, wc := m.drawScreen(160), asks.drawScreen(160)
	crumb := slices.IndexFunc(sc.tabs, func(sp span) bool { return sp.press == m.keys.main.Back.Help().Key })
	if crumb < 0 || wc.waits.press == "" {
		t.Fatalf("no esc all on the tab row (%+v) or no count in the header (%+v)", sc.tabs, wc.waits)
	}
	for _, c := range []struct {
		name string
		m    Model
		at   tea.MouseClickMsg
	}{
		{"esc all", m, tea.MouseClickMsg{X: sc.tabs[crumb].x0, Y: 1, Button: tea.MouseLeft}},
		{"the count", asks, tea.MouseClickMsg{X: wc.waits.x0, Y: 0, Button: tea.MouseLeft}},
	} {
		name, at := c.name, c.at
		cleared := mouse(t, c.m, at)
		cleared = mouse(t, cleared, tea.MouseReleaseMsg{X: at.X, Y: at.Y, Button: tea.MouseLeft})
		if cleared.boardChain != "" || cleared.room != nil {
			t.Fatalf("a tap on %s: chain %q, room %v", name, cleared.boardChain, cleared.room)
		}
		if j := menuLine(cleared, s, "out of hearth"); j >= 0 {
			t.Fatalf("a tap on %s left the line: %q", name, itemLabels(cleared.menuFor(s)))
		}
		if run := outOfChain(cleared, s, i); run.pending != nil && run.pending.destructive {
			t.Fatalf("a tap on %s: the menu's line %d still asks %+v", name, i, run.pending)
		}
	}
}

// TestATabLeavesTheRoom: off the board the room is only the board's filter.
// A tap on a tab or Tab draws that tab, not the room, and the room's keys
// rest until the board is back.
func TestATabLeavesTheRoom(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, _ := hearthOpen(t, 160, st)
	if roomRowNow(m) != "talk › tell hearth's stoker…" {
		t.Fatalf("the room's talk line is not chosen: %q", roomRowNow(m))
	}
	if m = pressOn(m, "tab"); m.tab == tabSessions {
		t.Fatal("tab on the room's talk line stays on the board")
	}
	back := pressOn(m, "shift+tab")
	if back.tab != tabSessions || back.room == nil || back.boardChain != "hearth" {
		t.Errorf("shift+tab back: tab %d, room %v, chain %q", back.tab, back.room, back.boardChain)
	}

	m, f := hearthOpen(t, 160, st)
	heads := strings.Split(plainFrame(m), "\n")[1]
	x := strings.Index(heads, "machine")
	m = mouse(t, m, tea.MouseClickMsg{X: x + 1, Y: 1, Button: tea.MouseLeft})
	m = mouse(t, m, tea.MouseReleaseMsg{X: x + 1, Y: 1, Button: tea.MouseLeft})
	if m.tab != tabMachine {
		t.Fatalf("a tap on machine: tab %d", m.tab)
	}
	if frame := plainFrame(m); strings.Contains(frame, "‹ ≋") || strings.Contains(frame, "tell hearth's stoker") {
		t.Errorf("the machine tab draws the room:\n%s", frame)
	}
	if m = pressOn(m, "y", "y"); len(f.acts()) != 0 || m.pending != nil {
		t.Errorf("the room's y acts off the board: acts %+v, pending %+v", f.acts(), m.pending)
	}
}

// TestARoomRowSaysWhyACardKeyRests: on the room's own rows a card's key
// acts on nothing and says so.
func TestARoomRowSaysWhyACardKeyRests(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, f := hearthOpen(t, 160, st)
	if roomRowNow(m) != "talk › tell hearth's stoker…" {
		t.Fatalf("the room opens on %q", roomRowNow(m))
	}
	for _, k := range []string{"x", "p", "c"} {
		next := pressOn(m, k)
		if !strings.HasPrefix(next.noticeText, "no card is chosen") || len(f.acts()) != 0 || next.pending != nil {
			t.Errorf("%s on the talk line: notice %q, acts %+v, pending %+v", k, next.noticeText, f.acts(), next.pending)
		}
	}
}
