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

func TestTheTabRowIsBoardAndMachine(t *testing.T) {
	t.Skip("needs Task 12 step 6c: the tab row is board · machine")
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

func TestACardSaysItsChainMates(t *testing.T) {
	t.Skip("needs Task 12 step 6d: the room's match rows (inbox, repo, also in) leave for the cards")
}

func TestTheMenuTakesAMatchOutOfItsChain(t *testing.T) {
	t.Skip("needs Task 12 step 6d: u leaves the room for the card's menu, which has no destructive line today")
}
