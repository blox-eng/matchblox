package app

import (
	"maps"
	"slices"
	"strings"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/sample"
)

// A chain's room is the board narrowed to the chain (spec "Chains"): the
// stoker's card above the columns, the talk line under them, and on a phone
// the stoker as the first page. m.room keeps the room's state; its cursor
// (room.at) is on a line of the card, on the talk line, or -1: on the
// board, whose own choice holds.

// roomChosen is the row of the room the cursor is on, when the room draws
// it: a line of the stoker's card, or the talk line. false: no room is
// open, or the cursor is on the board.
func (m Model) roomChosen() (roomRow, bool) {
	if m.room == nil || !m.boardShown() {
		return roomRow{}, false
	}
	rows := m.roomRows(m.room.k)
	if at := min(m.room.at, len(rows)-1); at >= 0 {
		return rows[at], true
	}
	return roomRow{}, false
}

// keepDraft keeps the open room's words typed to its stoker and not sent,
// for the room to show them when it opens again.
func (m *Model) keepDraft() {
	r := m.room
	if r == nil {
		return
	}
	m.drafts = maps.Clone(m.drafts)
	if r.draft == "" {
		delete(m.drafts, r.k)
		return
	}
	if m.drafts == nil {
		m.drafts = map[chainKey]string{}
	}
	m.drafts[r.k] = r.draft
}

// closeRoom closes the room: the board shows every chain again. Words
// typed to its stoker and not sent wait for the room to open again.
func (m Model) closeRoom() Model {
	m.keepDraft()
	m.room = nil
	m.boardChain, m.peekAt = "", m.now()
	return m
}

// roomDrawn is one line the room draws on the board, and the row of the
// room it is (-1 for none).
type roomDrawn struct {
	text string
	sel  bool
	at   int
}

// roomDraws are the room's own lines w wide, top down: each line of the
// stoker's card, then the talk line, the cursor's row with its mark.
func (m Model) roomDraws(w int) (card, talk []roomDrawn) {
	if m.room == nil {
		return nil, nil
	}
	rows := m.roomRows(m.room.k)
	for i, r := range rows {
		sel := i == m.room.at
		for _, l := range m.roomLine(r, w-1, sel) {
			d := roomDrawn{fit(m.chosenMark(sel)+l, w), sel, i}
			if r.kind == roomTalk {
				talk = append(talk, d)
			} else {
				card = append(card, d)
			}
		}
	}
	return card, talk
}

// roomFolds is how far a short screen folds the stoker's card (roomSplit).
const roomFolds = 4

// roomSplit are the room's lines above the columns of a mid screen and a
// desk, the card and a hairline under it, and under the columns, the talk
// line. On a phone they are the stoker's page. A short screen folds the card
// fold steps: 1 keeps its active step, 2 its first line, 3 drops the
// hairline, 4 the card; the cursor's row always stays.
func (m Model) roomSplit(w, fold int) (above, below []roomDrawn) {
	if m.room == nil || m.layoutAt(w) == Narrow {
		return nil, nil
	}
	above, below = m.roomDraws(w)
	if fold > 0 && len(above) > 0 {
		rows := m.roomRows(m.room.k)
		keep := above[0].at
		if fold == 1 {
			if i := slices.IndexFunc(above, func(d roomDrawn) bool {
				return rows[d.at].kind == roomStep && strings.HasPrefix(rows[d.at].text, "▸")
			}); i >= 0 {
				keep = above[i].at
			}
		}
		if fold == roomFolds {
			keep = -1
		}
		above = slices.DeleteFunc(above, func(d roomDrawn) bool { return d.at != keep && !d.sel })
	}
	if len(above) > 0 && fold < 3 {
		above = append(above, roomDrawn{text: m.st.hair.Render(strings.Repeat("─", w)), at: -1})
	}
	return above, below
}

// stokerPage: on a phone the stoker's page is open, the room's row chosen.
func (m Model) stokerPage(w int) bool {
	_, own := m.roomChosen()
	return own && m.layoutAt(w) == Narrow
}

// roomPage draws the stoker's page of a phone into d: the card, a blank
// line, then the talk line.
func (m Model) roomPage(d *boardDrawn, w int) {
	card, talk := m.roomDraws(w)
	for _, l := range card {
		d.roomAt[len(d.lines)] = l.at
		d.addRow(-1, l.sel, w, m.st, l.text)
	}
	if len(card) > 0 {
		d.add(-1, strings.Repeat(" ", w))
	}
	for _, l := range talk {
		d.roomAt[len(d.lines)] = l.at
		d.addRow(-1, l.sel, w, m.st, l.text)
	}
}

// roomOrder are the room's own rows in the cursor's order, split at the
// board's cards: on a desk the card's rows come before the cards and the
// talk line after them; on a phone the stoker's page, card and talk line,
// is before the cards.
func (m Model) roomOrder() (before, after []int) {
	rows := m.roomRows(m.room.k)
	for i, r := range rows {
		switch {
		case r.kind == roomTalk && m.layoutAt(max(m.width, minWidth)) != Narrow:
			after = append(after, i)
		default:
			before = append(before, i)
		}
	}
	return before, after
}

// roomOn puts the room's cursor on its row at, off the board's lines.
func (m *Model) roomOn(at int) {
	r := *m.room
	r.at = at
	m.room = &r
	m.boardAbove, m.boardBelow, m.recPick, m.ghostPick = 0, 0, "", 0
}

// roomOff gives the cursor back to the board: the room's cursor is on no
// row of its own.
func (m *Model) roomOff() {
	r := *m.room
	r.at = -1
	m.room = &r
}

// roomToCard gives the cursor to the open column's first (d 1) or last
// (d -1) card. false: the column has no card.
func (m *Model) roomToCard(d int) bool {
	b, _ := m.boardDrawOf()
	ss := b.idx[b.open]
	if len(ss) == 0 {
		return false
	}
	m.roomOff()
	s := ss[0]
	if d < 0 {
		s = ss[len(ss)-1]
	}
	m.boardChooseCard(b.open, s)
	m.chooseByHand()
	return true
}

// roomMove is ↑ (d -1) or ↓ (d 1) on the room's own row at: through the
// card's rows and the talk line, and from their ends onto the board.
func (m *Model) roomMove(at, d int) {
	before, after := m.roomOrder()
	in, list := slices.Index(before, at), before
	if in < 0 {
		in, list = slices.Index(after, at), after
	}
	switch next := in + d; {
	case next >= 0 && next < len(list):
		m.roomOn(list[next])
	case d < 0 && slices.Equal(list, after) && m.roomToCard(-1):
	case d < 0 && slices.Equal(list, after) && len(before) > 0:
		m.roomOn(before[len(before)-1])
	case d < 0 && len(m.aboveLines()) > 0:
		m.roomOff()
		m.boardAbove = 1
	case d > 0 && slices.Equal(list, before) && m.roomToCard(1):
	case d > 0 && slices.Equal(list, before) && len(after) > 0:
		m.roomOn(after[0])
	case d > 0 && len(m.belowLines()) > 0:
		m.roomOff()
		m.boardBelow = 1
	}
}

// roomEdge is ↑ (d -1) or ↓ (d 1) from the board's side into the room's
// own rows: up from the first card, or down from the line above the
// columns next to them, to the card's rows; down from the last card, or up
// from the ghost row, to the talk line. false: the board moves as it does
// with no room.
func (m *Model) roomEdge(d int) bool {
	before, after := m.roomOrder()
	_, above := m.aboveChosen()
	_, below := m.belowChosen()
	b, _ := m.boardDrawOf()
	ss := b.idx[b.open]
	i := slices.IndexFunc(ss, func(s sample.Session) bool { return b.picked && paneOf(s) == paneOf(b.chosen) })
	switch {
	case above && d > 0 && m.boardAbove == 1 && len(before) > 0:
		m.roomOn(before[0])
	case below && d < 0 && m.boardBelow == 1 && len(after) > 0:
		m.roomOn(after[len(after)-1])
	case above || below:
		return false
	case d < 0 && i <= 0 && len(before) > 0:
		m.roomOn(before[len(before)-1])
	case d > 0 && (!b.picked || i == len(ss)-1) && len(after) > 0:
		m.roomOn(after[0])
	default:
		return false
	}
	return true
}

// roomBoardKey is k on the board with a room open: the talk line types,
// the room's own keys keep their jobs, and ↑ ↓ walk the card's rows, the
// cards and the talk line. false: the board takes k.
func (m Model) roomBoardKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	rk, mk, bk := m.keys.room, m.keys.main, m.keys.board
	if m.room.typing || bkey.Matches(k, rk.Flip, rk.Walk) {
		m.flash = ""
		next, cmd := m.roomKey(k)
		return next, cmd, true
	}
	if bkey.Matches(k, rk.Next, rk.Prev) { // the chains the board draws, as on the board
		m.flash = ""
		step := 1
		if bkey.Matches(k, rk.Prev) {
			step = -1
		}
		m.turnChain(step)
		return m, nil, true
	}
	_, own := m.roomChosen()
	m.flash = ""
	var cmd tea.Cmd
	switch {
	case own && bkey.Matches(k, mk.Up):
		m.roomMove(m.room.at, -1)
	case own && bkey.Matches(k, mk.Down):
		m.roomMove(m.room.at, 1)
	case own && bkey.Matches(k, rk.Open, rk.Give, rk.Back):
		next, cmd := m.roomKey(k)
		return next, cmd, true
	case own && bkey.Matches(k, bk.Left, bk.Right):
		m.roomToCard(1) // back to the open column
	case own && (isDigit(k) || bkey.Matches(k, bk.Merge, bk.Menu, bk.Copy, mk.Second, mk.Into, mk.Jump, bk.Chain,
		mk.Park, mk.Rename, mk.Switch, mk.Dismiss, mk.Fold, mk.Stoke, mk.StokeChain, mk.Night, mk.Order)):
		// The room's row is chosen, not a card: never a match the person cannot see.
		cmd = m.notice("no card is chosen · ↓ goes back to the cards")
		if before, _ := m.roomOrder(); !slices.Contains(before, m.room.at) {
			cmd = m.notice("no card is chosen · ↑ goes back to the cards")
		}
	case own:
		return m, nil, false
	case bkey.Matches(k, mk.Up) && m.roomEdge(-1), bkey.Matches(k, mk.Down) && m.roomEdge(1):
	default:
		return m, nil, false
	}
	return m, cmd, true
}

// roomHints are the keys of the room's row on the key line.
func (m Model) roomHints(row roomRow) []hint {
	rk := m.keys.room
	var hs []hint
	switch row.kind {
	case roomStep:
		if _, ok := m.goOf(m.room.k, row); ok {
			hs = append(hs, hintFor(rk.Give, "give"))
		}
	case roomMore:
		hs = append(hs, hintFor(rk.Open, "unfold"))
	case roomTalk:
		hs = append(hs, hintFor(rk.Open, "talk"))
	case roomCard:
		switch row.id {
		case "quiet":
			hs = append(hs, hintFor(rk.Open, "ask"))
		case roomEmpty:
			hs = append(hs, hintFor(rk.Open, "new session"))
		}
	}
	return append(hs, hintFor(rk.Next, "next chain"), hintFor(rk.Back, "all"))
}

// roomTap is a tap on the room's row at: the first chooses it, a second
// acts as ⏎ does, or on a step that waits for a go as y does, which asks
// for a typed y in the consent box.
func (m Model) roomTap(at int) (tea.Model, tea.Cmd) {
	row, own := m.roomChosen()
	switch {
	case !own || m.room.at != at:
		m.roomOn(at)
		return m, nil
	case row.kind == roomStep:
		return m.roomKey(keyPress(m.keys.room.Give.Keys()[0]))
	}
	return m.roomKey(keyPress(m.keys.room.Open.Keys()[0]))
}

// stokerMark is the press of a tap on the stoker's page mark.
const stokerMark = "stoker"

// openStokerPage opens the stoker's page of a phone: the room's cursor on
// the need's step, else on the talk line.
func (m Model) openStokerPage() Model {
	at := m.roomLanding(m.room.k)
	if at < 0 { // the need is on a card: the page opens on the talk line
		at = len(m.roomRows(m.room.k)) - 1
	}
	m.roomOn(at)
	return m
}
