package app

import (
	"maps"

	tea "charm.land/bubbletea/v2"
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
	if m.sidePlace == placeRoom {
		m.sidePlace = placeChat
	}
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

// roomOn puts the room's cursor on its row at.
func (m *Model) roomOn(at int) {
	r := *m.room
	r.at = at
	m.room = &r
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
	return append(hs, hintFor(rk.Back, "all"))
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
