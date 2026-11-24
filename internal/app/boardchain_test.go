package app

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/charmbracelet/x/ansi"
)

// The board's tests of the chains and their rooms (Task 12 step 0c).

// chainBoard is the console on st at w columns, on the demo's clock, with
// the keys in the list and the session named name chosen ("" for the
// list's first).
func chainBoard(t *testing.T, w int, st proto.State, name string) (Model, *fakeConn) {
	t.Helper()
	m, f := headModel(t, w, st)
	m.toList()
	if name != "" {
		m.sideAt = paneOf(matchOf(t, m, name))
	}
	return m, f
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

// TestTheTabRowIsBoardAndMachine: two tabs, the board and Machine; Git,
// History, Panes and the stoker open from Machine's rows. Tab and
// Shift+Tab turn between the two.
func TestTheTabRowIsSessionsAndMachine(t *testing.T) {
	if !slices.Equal(tabNames, []string{"sessions", "machine"}) {
		t.Fatalf("tabs %q, want sessions and machine", tabNames)
	}
	m, _ := loadedWith(t, 44, queueState()) // the phone: the desk has no tab row
	for tab, want := range map[int][]string{tabSessions: {"SESSIONS", "3", "machine"}, tabMachine: {"sessions", "3", "MACHINE"}} {
		on := m
		on.setTab(tab)
		if row := strings.Fields(strings.Split(screen(on), "\n")[1]); !slices.Equal(row, want) {
			t.Errorf("tab %d: the tab row is %q, want %q", tab, row, want)
		}
	}
	if next := pressOn(m, "tab"); next.tab != tabMachine {
		t.Fatalf("tab on Sessions opens tab %d", next.tab)
	}
	machine := pressOn(m, "tab")
	if pressOn(machine, "tab").tab != tabSessions || pressOn(machine, "shift+tab").tab != tabSessions || pressOn(m, "shift+tab").tab != tabMachine {
		t.Fatal("Tab and Shift+Tab do not turn between Sessions and Machine")
	}
	// A detached busy loop burns: the tab row says so, Machine's !, on the
	// Sessions and on a view of Machine.
	burns, _ := loadedWith(t, 44, orphanState())
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

// TestTheCountKeepsTheTalkDraft: a tap on the count closes the room, but
// the words typed to its stoker wait: the room opens again with them on
// the talk line.
func TestTheCountKeepsTheTalkDraft(t *testing.T) {
	m, _ := hearthOpen(t, 160, headState(t, runningPlan(), true))
	for i := 0; i < 12 && !strings.HasPrefix(roomRowNow(m), "talk"); i++ {
		m = pressOn(m, "down")
	}
	m = typeText(pressOn(m, "enter"), "ship the login fix").(Model)
	if m.room == nil || m.room.draft != "ship the login fix" {
		t.Fatalf("the talk line did not take the words: %+v", m.room)
	}
	if m = tapText(t, m, "need you"); m.room != nil {
		t.Fatal("the count kept the room")
	}
	m = pressOn(m, "esc", "g") // the count gave the keys to the line: esc goes to the list
	if m.room == nil || m.boardChain != "hearth" {
		t.Fatalf("g after the count: room %v, chain %q", m.room, m.boardChain)
	}
	if m.room.draft != "ship the login fix" || !strings.HasPrefix(roomRowNow(m), "talk") || !strings.Contains(plainFrame(m), "› ship the login fix") {
		t.Errorf("the room opened again with draft %q on row %q:\n%s", m.room.draft, roomRowNow(m), plainFrame(m))
	}
}

// TestASentDraftDoesNotComeBack: a draft put back on the talk line is no
// longer kept: once sent, no later open of the room shows it again.
func TestASentDraftDoesNotComeBack(t *testing.T) {
	m, f := hearthOpen(t, 160, headState(t, runningPlan(), true, mockupChain, []string{"flint-auth", "spark-docs"}))
	toTalk := func(m Model) Model {
		for i := 0; i < 12 && !strings.HasPrefix(roomRowNow(m), "talk"); i++ {
			m = pressOn(m, "down")
		}
		return m
	}
	m = typeText(pressOn(toTalk(m), "enter"), "ship the login fix").(Model)
	m = pressOn(tapText(t, m, "need you"), "esc", "g")
	if m.room == nil || m.room.draft != "ship the login fix" {
		t.Fatalf("g after the count: room %+v", m.room)
	}
	m = pressOn(m, "enter", "enter")
	if len(f.acts()) == 0 || m.room.draft != "" {
		t.Fatalf("⏎ ⏎ did not send the draft: acts %+v, draft %q", f.acts(), m.room.draft)
	}
	if m = pressOn(m, "esc", "esc", "esc", "g"); m.room == nil || m.room.draft != "" || strings.Contains(plainFrame(m), "› ship the login fix") {
		t.Errorf("a room opened again shows the sent draft: room %+v", m.room)
	}
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

	m, _ = chainBoard(t, 160, st, "")
	m = m.flipFrom("/dev/pts/2", matchOf(t, m, "hearth-api").Pane)
	if row := roomRowNow(m); row != "talk › tell hearth's stoker…" || !m.room.typing || m.boardChain != "hearth" {
		t.Errorf("a flip lands on %q, typing %v, chain %q", row, m.room.typing, m.boardChain)
	}
}

// TestTheRoomsCursorWalksItsRows: in the chat's place ↑ and ↓ walk the
// stoker's card, its steps and the talk line, and stop at their ends; ⏎
// unfolds the plan and types on the talk line; esc stops typing, then
// closes the room, and the chat comes back.
func TestTheRoomsCursorWalksItsRows(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, _ := hearthOpen(t, 160, st)
	if m.sidePlace != placeRoom || roomRowNow(m) != "talk › tell hearth's stoker…" {
		t.Fatalf("g opens place %d on %q", m.sidePlace, roomRowNow(m))
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
		t.Errorf("↑ from the card's first row: %q", roomRowNow(m))
	}
	for range 7 {
		m = pressOn(m, "down")
	}
	if roomRowNow(m) != "talk › tell hearth's stoker…" {
		t.Fatalf("↓ to the end: %q", roomRowNow(m))
	}

	m = pressOn(m, "enter")
	m = typeText(m, "ship it").(Model)
	if !m.room.typing || m.room.draft != "ship it" || !strings.Contains(plainFrame(m), "› ship it▏") {
		t.Fatalf("⏎ on the talk line: typing %v, draft %q", m.room.typing, m.room.draft)
	}
	if m = pressOn(m, "esc"); m.room == nil || m.room.typing {
		t.Fatal("the first esc did not only stop typing")
	}
	if m = pressOn(m, "esc"); m.room != nil || m.boardChain != "" || m.sidePlace != placeChat {
		t.Fatalf("the second esc kept the room: chain %q, place %d", m.boardChain, m.sidePlace)
	}

	m, _ = hearthOpen(t, 160, st)
	m = pressOn(m, "up")
	if m = pressOn(m, "enter"); !m.room.unfolded || !strings.Contains(plainFrame(m), "  step") {
		t.Error("⏎ on more does not unfold the plan")
	}
}

// TestANarrowedChainIsItsRoom: g narrows the list to the chain, and the
// room takes the chat's place: its stoker's card and steps, then the talk
// line. y on the step that waits gives its go through the consent box,
// and a tap on the talk line chooses it, a second one types.
func TestANarrowedChainIsItsRoom(t *testing.T) {
	st := headState(t, aGo(runningPlan(), "merge:#4556", ""), true)
	st.Queue = nil
	m, f := hearthOpen(t, 160, st)
	var got []string
	for _, s := range m.sideSessions() {
		if m.inBoard(s) {
			got = append(got, s.Name)
		}
	}
	slices.Sort(got)
	if want := []string{"ember-billing", "hearth-api", "kindle-web"}; !slices.Equal(got, want) {
		t.Errorf("the room's list shows %v, want %v", got, want)
	}
	lines := strings.Split(plainFrame(m), "\n")
	at := func(has string) int {
		return slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, has) })
	}
	head, step, talk := at("CHAIN hearth"), at("▸ step · waits for your go"), at("› tell hearth's stoker…")
	if head < 0 || step <= head || talk <= step {
		t.Fatalf("the room's name, its step and the talk line (head %d, step %d, talk %d):\n%s", head, step, talk, strings.Join(lines, "\n"))
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

	x := strings.Index(lines[talk], "›")
	x = len([]rune(lines[talk][:x]))
	m = mouse(t, m, tea.MouseClickMsg{X: x, Y: talk, Button: tea.MouseLeft})
	m = mouse(t, m, tea.MouseReleaseMsg{X: x, Y: talk, Button: tea.MouseLeft})
	if roomRowNow(m) != "talk › tell hearth's stoker…" || m.room.typing {
		t.Fatalf("the first tap on the talk line: %q, typing %v", roomRowNow(m), m.room.typing)
	}
	m = mouse(t, m, tea.MouseClickMsg{X: x, Y: talk, Button: tea.MouseLeft})
	m = mouse(t, m, tea.MouseReleaseMsg{X: x, Y: talk, Button: tea.MouseLeft})
	if !m.room.typing {
		t.Error("the second tap on the talk line does not type")
	}
}

// TestOnAPhoneTheRoomTakesTheScreen: on a phone the room is one pane,
// in place of the list: the stoker's card, its steps and the talk line,
// with ◂ back to the list, the room still open.
func TestOnAPhoneTheRoomTakesTheScreen(t *testing.T) {
	st := headState(t, runningPlan(), true)
	m, _ := hearthOpen(t, 44, st)
	frame := plainFrame(m)
	for _, want := range []string{"◂ CHAIN hearth", `≋ "ship the chain"`, "▸ step", "› tell hearth's stoker…"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the room's pane lacks %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "NEEDS YOU") {
		t.Errorf("the room's pane draws the list:\n%s", frame)
	}
	m = pressOn(m, "left")
	if m.room == nil || m.sidePlace != placeChat || !strings.Contains(plainFrame(m), "NEEDS YOU") {
		t.Errorf("◂ from the room: room %v, place %d:\n%s", m.room != nil, m.sidePlace, plainFrame(m))
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

func outOfChain(m Model, s sample.Session, i int) Model {
	m.chooseRow(s)
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

	// A tap on the count in the header clears the narrowing and closes
	// the room. The line goes with it.
	asks, _ := hearthOpen(t, 160, headState(t, runningPlan(), true)) // kindle-web asks: the header counts it
	if menuLine(asks, s, "out of hearth") < 0 {
		t.Fatalf("the room's menu lacks out of hearth: %q", itemLabels(asks.menuFor(s)))
	}
	wc := asks.drawScreen(160)
	if wc.waits.press == "" {
		t.Fatalf("no count in the header (%+v)", wc.waits)
	}
	for _, c := range []struct {
		name string
		m    Model
		at   tea.MouseClickMsg
	}{
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
	head := strings.Split(plainFrame(m), "\n")[0] // the desk's ⚙ opens Machine
	x := ansi.StringWidth(head[:strings.Index(head, "⚙")])
	m = mouse(t, m, tea.MouseClickMsg{X: x, Y: 0, Button: tea.MouseLeft})
	m = mouse(t, m, tea.MouseReleaseMsg{X: x, Y: 0, Button: tea.MouseLeft})
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

// TestARoomRowSaysWhyACardKeyRests: while the room has the chat's place a
// key of the list acts on nothing and says which keys the room takes.
func TestARoomRowSaysWhyACardKeyRests(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, f := hearthOpen(t, 160, st)
	if roomRowNow(m) != "talk › tell hearth's stoker…" {
		t.Fatalf("the room opens on %q", roomRowNow(m))
	}
	for _, k := range []string{"x", "p", "c"} {
		next := pressOn(m, k)
		if !strings.HasPrefix(next.noticeText, "the room takes ") || len(f.acts()) != 0 || next.pending != nil {
			t.Errorf("%s on the talk line: notice %q, acts %+v, pending %+v", k, next.noticeText, f.acts(), next.pending)
		}
	}
}

// TestAnAskingChainStokerIsANeed (ruling 39): a chain stoker whose agent
// asks is a need: its card is in NEEDS YOU and counts there, as any match
// that asks, and says the ask; on the whole board and in its chain's room.
// A stoker that does not ask is no card.
func TestAnAskingChainStokerIsANeed(t *testing.T) {
	quiet := headState(t, runningPlan(), true, []string{"hearth-api", "ember-billing"})
	m, _ := chainBoard(t, 160, quiet, "")
	if slices.ContainsFunc(m.all.sessions, func(s sample.Session) bool {
		return s.Pane == stokerPane && slices.ContainsFunc(needRows(m), func(c sample.Session) bool { return c.Pane == s.Pane })
	}) || strings.Contains(plainFrame(m), "stoker-s1") {
		t.Fatalf("a stoker that does not ask is a card:\n%s", plainFrame(m))
	}
	before := m.needCount()

	asks := headState(t, runningPlan(), true, []string{"hearth-api", "ember-billing"})
	asker(&asks)
	board, _ := chainBoard(t, 160, asks, "")
	room, _ := hearthOpen(t, 160, asks)
	for name, m := range map[string]Model{"board": board, "room": room} {
		need := needRows(m)
		if !slices.ContainsFunc(need, func(s sample.Session) bool { return s.Pane == stokerPane }) {
			t.Errorf("%s: the asking stoker is not in NEEDS YOU: %v", name, need)
		}
		if n := m.needCount(); n != before+1 {
			t.Errorf("%s: NEEDS YOU counts %d, want %d", name, n, before+1)
		}
		if !strings.Contains(plainFrame(m), "merge #812 now?") {
			t.Errorf("%s: the card does not say the ask:\n%s", name, plainFrame(m))
		}
		if f := plainFrame(m); !strings.Contains(f, "╿ hearth's stoker") || strings.Contains(f, "stoker-s1") {
			t.Errorf("%s: the card is not named hearth's stoker (ruling 42):\n%s", name, f)
		}
	}
	if f := plainFrame(room); !strings.Contains(f, "≋ hearth's stoker asks: merge #812 now?") || strings.Contains(f, "is quiet since") {
		t.Errorf("the stoker's card in the room does not say it asks:\n%s", f)
	}
}

// TestAChainStokerWhoseRunIsOffIsNoCard: the agent of a chain stoker that
// rests (its run off) is no card and no count, on the whole board and in
// its chain's room; while it does not ask, the same holds when it runs.
func TestAChainStokerWhoseRunIsOffIsNoCard(t *testing.T) {
	for name, v := range map[string]proto.StokerView{"off": {}, "on": runningPlan()} {
		st := headState(t, v, true)
		board, _ := chainBoard(t, 160, st, "")
		room, _ := hearthOpen(t, 160, st)
		for where, m := range map[string]Model{"board": board, "room": room} {
			for _, ss := range [][]sample.Session{m.sideSessions()} {
				if slices.ContainsFunc(ss, func(s sample.Session) bool { return s.Pane == stokerPane }) {
					t.Errorf("run %s, %s: the stoker's agent is a card", name, where)
				}
			}
			if f := plainFrame(m); strings.Contains(f, "stoker-s1") || strings.Contains(f, "╿ hearth's stoker") {
				t.Errorf("run %s, %s: the stoker's agent is drawn:\n%s", name, where, f)
			}
		}
	}
}

// TestAFlipIntoAnOpenRoomLandsOnTheNeed: ⌥s into the room that is open
// already lands as into a new one: on the card of the match that asks,
// though the cursor was on another card; the draft stays.
func TestAFlipIntoAnOpenRoomLandsOnTheNeed(t *testing.T) {
	m, _ := hearthOpen(t, 160, headState(t, runningPlan(), true))
	if chosenName(t, m) != "kindle-web" {
		t.Fatalf("the room opens on %s, want kindle-web", chosenName(t, m))
	}
	hearth := matchOf(t, m, "hearth-api")
	m.chooseRow(hearth)
	r := *m.room
	r.draft = "half an order"
	m.room = &r
	m = m.flipFrom("/dev/pts/2", hearth.Pane)
	if got := chosenName(t, m); got != "kindle-web" || m.room.at != -1 {
		t.Errorf("a flip into the open room chose %s (room row %d), want kindle-web", got, m.room.at)
	}
	if m.room.draft != "half an order" {
		t.Errorf("the flip lost the draft: %q", m.room.draft)
	}
}

// TestTheAskingStokersCardIsItsChainsStokerEverywhere (ruling 42): the
// card's menu and its key line's words name it as its card does.
func TestTheAskingStokersCardIsItsChainsStokerEverywhere(t *testing.T) {
	asks := headState(t, runningPlan(), true, []string{"hearth-api", "ember-billing"})
	asker(&asks)
	m, _ := chainBoard(t, 160, asks, "")
	s, ok := m.agentOf(asks.Sessions[0].Host, "s1")
	if !ok {
		t.Fatal("no stoker agent")
	}
	m.sideAt = paneOf(s)
	m = pressOn(m, "m")
	if m.menu == nil {
		t.Fatal("m opened no menu on the stoker's card")
	}
	box, _, _ := m.menuBox(160, 30)
	if got := ansi.Strip(strings.Join(box, "\n")); !strings.Contains(got, "hearth's stoker") || strings.Contains(got, "stoker-s1") {
		t.Errorf("the menu does not name hearth's stoker:\n%s", got)
	}
}

// needRows are the sessions of the list's NEEDS YOU.
func needRows(m Model) []sample.Session {
	return slices.DeleteFunc(m.sideSessions(), func(s sample.Session) bool { return m.sideStateOf(s) != sideNeed })
}

// TestLeavingTheRoomSaysGReopensIt: ◂ in the room gives the chat its place
// back and the keys to the list; the key line names g, which opens the
// room again.
func TestLeavingTheRoomSaysGReopensIt(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, _ := hearthOpen(t, 160, st)
	m = pressOn(m, "left")
	if m.sidePlace != placeChat || !m.inList() {
		t.Fatalf("◂ in the room: place %d, in the list %v", m.sidePlace, m.inList())
	}
	if keys := m.sideKeyWords(); !strings.Contains(keys, "g room") { // the keys the ? sheet and the phone's key line name
		t.Errorf("the key line %q does not say g opens the room again", keys)
	}
	if m = pressOn(m, "g"); m.sidePlace != placeRoom {
		t.Errorf("g did not open the room again: place %d", m.sidePlace)
	}
}

// reopenAct is the act ⏎ sent on the waiting row of m, and the
// conversations its Text asks recover to reopen. The Text names nothing
// else: a tmux session's name would start each agent of that session.
func reopenAct(t *testing.T, m Model) (proto.Act, []string) {
	t.Helper()
	acts := boardFake(m).acts()
	if len(acts) != 1 {
		t.Fatalf("⏎ on a waiting row sent %d acts: %+v", len(acts), acts)
	}
	var r proto.Recover
	if err := json.Unmarshal([]byte(acts[0].Text), &r); err != nil {
		t.Fatalf("the act's Text %q is no JSON: %v", acts[0].Text, err)
	}
	if want, _ := json.Marshal(proto.Recover{Conversations: r.Conversations}); string(want) != acts[0].Text {
		t.Fatalf("the act's Text is %s, want only conversations", acts[0].Text)
	}
	return acts[0], r.Conversations
}

// TestEnterReopensAWaitingMatch: ⏎ on a waiting row, on a desk or not,
// sends recover for its conversation, as JSON: never
// its name, which the record may not know. The result shows as one notice
// that names the fix.
func TestEnterReopensAWaitingMatch(t *testing.T) {
	since := time.Date(2026, 10, 1, 22, 14, 0, 0, time.Local)
	for _, c := range []struct {
		name       string
		desk, line bool
	}{{"kindle-web", false, false}, {"kindle-web", true, false}, {"kindle\"web\nx", false, false}} {
		m := pressOn(waitingKindle(t, since, 0), "down")
		for _, v := range m.chains.Mates {
			if v.Wait != nil {
				v.Wait.Name = c.name
			}
		}
		if c.desk {
			m.opt.Beside = true
		}
		if c.line {
			m.toLine()
		}
		m = pressOn(m, "enter")
		a, convs := reopenAct(t, m)
		if a.RecID != "recover" || a.Confirm != "y" || !slices.Equal(convs, []string{"c-3"}) {
			t.Errorf("desk %v, line %v, name %q: ⏎ sent %+v, conversations %q; want recover for c-3", c.desk, c.line, c.name, a, convs)
		}
	}

	m := pressOn(pressOn(waitingKindle(t, since, 0), "down"), "enter")
	for _, c := range []struct {
		row  proto.Recovered
		want string
	}{
		{proto.Recovered{Name: "kindle-web", State: proto.RecoverReopened, As: "kindle-web"}, "kindle-web reopened"},
		{proto.Recovered{Name: "kindle-web", State: proto.RecoverCannot, Why: "claude is not installed here: install it, then run matchblox recover again"},
			"kindle-web not reopened: claude is not installed here: install it, then run matchblox recover again"},
	} {
		next, _ := m.Update(resultMsg(proto.Result{ActID: "a1", Recover: []proto.Recovered{c.row}}))
		if got := next.(Model).noticeText; got != c.want {
			t.Errorf("%s: the notice is %q, want %q", c.row.State, got, c.want)
		}
	}
}

// TestEnterInTheLineOnAWaitingRowDoesNotReopen: in the line, ⏎ sends what
// was typed; on a waiting row it starts no agent. It says why, and sends
// nothing: ⏎ in the list reopens.
func TestEnterInTheLineOnAWaitingRowDoesNotReopen(t *testing.T) {
	m := pressOn(waitingKindle(t, time.Date(2026, 10, 1, 22, 14, 0, 0, time.Local), 0), "down")
	m.toLine()
	before := len(boardFake(m).allActs())
	m = pressOn(m, "enter")
	if acts := boardFake(m).allActs(); len(acts) != before {
		t.Fatalf("⏎ in the line on a waiting row sent %+v", acts[before:])
	}
	if want := "kindle-web waits for recover · matchblox recover reopens it"; m.noticeText != want {
		t.Fatalf("the notice is %q, want %q", m.noticeText, want)
	}
}

// TestTheReopenNoticeNamesTheRow: recover answers with the record's tmux
// session, work; the notice names the row the owner chose, api. A row of
// a conversation no member waits with keeps recover's name.
func TestTheReopenNoticeNamesTheRow(t *testing.T) {
	m := pressOn(waitingKindle(t, time.Date(2026, 10, 1, 22, 14, 0, 0, time.Local), 0), "down")
	for _, v := range m.chains.Mates {
		if v.Wait != nil {
			v.Wait.Name = "api"
		}
	}
	rows := []proto.Recovered{
		{Name: "work", Conversation: "c-3", State: proto.RecoverReopened, As: "work"},
		{Name: "other", Conversation: "c-9", State: proto.RecoverReady},
	}
	next, _ := m.Update(resultMsg(proto.Result{ActID: "a1", Recover: rows}))
	if got, want := next.(Model).noticeText, "api reopened · other is ready"; got != want {
		t.Fatalf("the notice is %q, want %q", got, want)
	}
}

// TestTheMenuOfAWaitingMatch: m on a waiting row opens its menu, with the
// room closed. out of login asks for a typed y, then takes the member out
// by its session id, never by its pane.
func TestTheMenuOfAWaitingMatch(t *testing.T) {
	m := pressOn(waitingKindle(t, time.Date(2026, 10, 1, 22, 14, 0, 0, time.Local), 0), "down")
	if m.room != nil {
		t.Fatal("the room is open")
	}
	m = pressOn(m, "m")
	if m.menu == nil {
		t.Fatalf("m on a waiting row opened no menu; the notice says %q", m.noticeText)
	}
	want := []string{"reopen it", "copy its name", "copy its conversation id", "out of login"}
	if got := itemLabels(m.menu.items); !slices.Equal(got, want) {
		t.Fatalf("the menu reads %q, want %q", got, want)
	}
	if !strings.Contains(plainFrame(m), "kindle-web") {
		t.Errorf("the menu's box does not name kindle-web:\n%s", plainFrame(m))
	}
	for range 3 {
		m = pressOn(m, "down")
	}
	asked := pressOn(m, "enter")
	f := boardFake(asked)
	if len(f.acts()) != 0 || asked.pending == nil || !asked.pending.destructive || asked.pending.label != "take kindle-web out of login" {
		t.Fatalf("⏎ on out of login: acts %+v, pending %+v", f.acts(), asked.pending)
	}
	pressOn(asked, "y")
	if a := f.acts(); len(a) != 1 || a[0].RecID != "chain:out:sid:sid-kindle-web@login" || a[0].Confirm != "y" {
		t.Fatalf("y: acts %+v", a)
	}

	reopen := pressOn(pressOn(waitingKindle(t, time.Date(2026, 10, 1, 22, 14, 0, 0, time.Local), 0), "down"), "m", "enter")
	if a, convs := reopenAct(t, reopen); a.RecID != "recover" || !slices.Equal(convs, []string{"c-3"}) {
		t.Errorf("reopen it sent %+v, conversations %q", a, convs)
	}
}

// TestTheReopenNoticeNamesTheRowAfterItIsLive: the reopened agent shows
// in the list before recover answers, so no row waits with its
// conversation any more. The notice still names the row the owner chose,
// not recover's tmux session.
func TestTheReopenNoticeNamesTheRowAfterItIsLive(t *testing.T) {
	m := pressOn(pressOn(waitingKindle(t, time.Date(2026, 10, 1, 22, 14, 0, 0, time.Local), 0), "down"), "enter")
	for i := range m.chains.Mates {
		m.chains.Mates[i].Wait = nil
	}
	rows := []proto.Recovered{{Name: "work", Conversation: "c-3", State: proto.RecoverReopened, As: "work"}}
	next, _ := m.Update(resultMsg(proto.Result{ActID: "a1", Recover: rows}))
	if got, want := next.(Model).noticeText, "kindle-web reopened"; got != want {
		t.Fatalf("the notice is %q, want %q", got, want)
	}
}
