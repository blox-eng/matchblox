package app

import (
	tea "charm.land/bubbletea/v2"
)

// tap is a click or a finger on a phone. A tap on a tab opens it, on a key
// of the key line, of an opened row or under the detail presses it, on `● N wait for you` selects the first
// session that waits, on the dock's `machine ›` switches the panel. A tap
// right of the list gives the chat the keys, and so does a tap on a
// session's row on a phone; a tap on the list, or on the title row of a
// phone's chat, gives them back to Sessions. A tap on an option of the
// chat's question card picks it. A tap on a row selects it and shows the step a second tap runs: an answer for
// a session that waits, else the row's Enter. The second tap runs it when
// it only moves this terminal, or asks to confirm it. A tap is never
// consent to a destructive step, and it cancels a pending one.
func (m Model) tap(ms tea.Mouse) (tea.Model, tea.Cmd) {
	if ms.Button != tea.MouseLeft || m.pair.owns() || m.asking() { // the pair page and an ssh prompt take keys only
		return m, nil
	}
	if m.held != nil { // a tap is another key: it drops the question
		m.held, m.flash = nil, ""
		return m, nil
	}
	if m.scanShown() { // a tap is never an answer
		return m.closeScan(), nil
	}
	if m.splashing() {
		m.splashDone = true
		return m, nil
	}
	if m.pending != nil {
		m.flash, m.pending = "cancelled: "+m.pending.String(), nil
		return m, nil
	}
	if !m.have || m.mismatch {
		return m, nil
	}
	m.searching = false // keep the search; what the tap asks for shows on the key line
	w := max(m.width, minWidth)
	sc := m.draw(w)
	if m.sheet != nil {
		return m.tapSheet(ms, sc)
	}
	if m.form != nil {
		return m.tapForm(ms, sc)
	}
	ci, dx := ms.Y-sc.cardAt, ms.X-sc.cardX
	onCard := ci >= 0 && ci < len(sc.cardRows) && dx >= 0 && dx < sc.cardW
	if m.up > 0 && m.chatKeys {
		switch {
		case onCard:
			return m.backToLive(), nil
		case sc.chatFull && ms.Y == 0, !sc.chatFull && ms.Y == sc.bodyTop && ms.X > sc.listW:
			m.up = 0 // the title: back to live, the keys stay
			return m, nil
		}
	}
	if onCard {
		return m.tapCard(ci, dx, sc)
	}
	if sc.chatFull && ms.Y != sc.keysAt {
		if ms.Y == 0 {
			m.chatKeys = false
		}
		return m, nil
	}
	switch {
	case ms.Y == 0:
		if s, ok := spanAt([]span{sc.stokerAt}, ms.X); ok && s.press != "" {
			return m.openAgent()
		}
		if s, ok := spanAt([]span{sc.waits}, ms.X); ok && s.press != "" {
			m.setTab(tabSessions)
			if i := m.firstWaiting(); i >= 0 {
				m.selectRow(len(m.doors) + i)
			}
			m.flash = ""
		}
		return m, nil
	case ms.Y == 1:
		if s, ok := spanAt(sc.tabs, ms.X); ok {
			m.setTab(int(s.press[0] - '1'))
			m.flash = ""
			return m.seeStoker()
		}
		return m, nil
	case ms.Y == sc.keysAt:
		if s, ok := spanAt(sc.keys, ms.X); ok {
			m.armed = ""
			return m.keyed(keyPress(s.press))
		}
		return m, nil
	case sc.dockAt >= 0 && ms.Y >= sc.dockAt && ms.Y < sc.dockAt+sc.dockH:
		if x0, x1 := m.dockToggle(w); ms.Y == sc.dockAt && m.phoneDock(w) && ms.X >= x0 && ms.X < x1 {
			m.cycleDock()
			return m, nil
		}
		if ms.X >= sc.stx0 && ms.X < sc.stx1 {
			return m.openStoker().seeStoker()
		}
		return m, nil
	}
	if ms.Y == sc.actKeysAt {
		if s, ok := spanAt(sc.actKeys, ms.X); ok {
			m.armed = ""
			return m.keyed(keyPress(s.press))
		}
	}
	i := ms.Y - sc.bodyTop
	if i < 0 || i >= len(sc.rows) {
		return m, nil
	}
	if ms.X >= sc.listW {
		if m.tab == tabSessions {
			return m.openChat(), nil
		}
		return m, nil
	}
	back := m.chatKeys // the tap gives the keys back, and opens nothing
	m.chatKeys = false
	if sc.rows[i] == stokerPanelRow {
		return m.openStoker().seeStoker()
	}
	if sc.rows[i] == headerRow {
		return m.tapHeader(ms.X)
	}
	if sc.rows[i] == recsLabelRow {
		m.foldRecs()
		return m, nil
	}
	if _, ok := m.selectedRec(); ok && sc.rows[i] == m.rowIndex() && ms.X >= sc.listW-m.dismissWidth() {
		m.dismissRec()
		return m, nil
	}
	if sc.rows[i] < 0 {
		return m, nil
	}
	picked := m.view().index != nil && m.rowIndex() == sc.rows[i] && (m.tab != tabSessions || m.chose)
	m.selectRow(sc.rows[i])
	if s, ok := m.selectedSession(); ok && s.Pane != "" && m.tab == tabSessions && m.layoutAt(w) == Narrow {
		m.armed = "" // a phone's session opens in its chat, as Enter does
		return m.openChat(), nil
	}
	if m.tab == tabShips {
		// A tap on the selected ship goes to its session, as Enter does.
		m.armed, m.flash = "", ""
		if picked {
			return m.toDriver(), nil
		}
		return m, nil
	}
	if pane, _ := m.chatTarget(); pane != "" {
		// A session opens in place with its keys; a tap on it once it is
		// selected gives the keys to its chat.
		m.armed, m.flash = "", ""
		if !picked || back {
			return m, nil // the default selection is not a tap
		}
		return m.chatOn(pane), nil
	}
	say, run := m.tapAct()
	if run == nil {
		m.armed, m.flash = "", "nothing to do here"
		return m, nil
	}
	// The second tap runs what the first one showed, and only that: a row
	// that moved under the finger shows its own step first.
	if m.armed != say {
		m.armed, m.flash = say, "tap again: "+say
		return m, nil
	}
	m.armed = ""
	if _, door := m.selectedDoor(); !door && m.night() && !m.stoker.Asked {
		m.held, m.flash = run, ""
		return m, nil
	}
	return run(m)
}

// tapAct is what a second tap on the selected row does, and what the
// first one shows: the row's Enter.
func (m Model) tapAct() (string, func(Model) (tea.Model, tea.Cmd)) {
	a := m.primary()
	if a == nil {
		return "", nil
	}
	return a.String(), func(m Model) (tea.Model, tea.Cmd) {
		if a.pair {
			return m.openPairDoor()
		}
		if a.ghost {
			return m.openNewIn(a.where, ""), nil
		}
		if a.nav && !a.destructive {
			return m.confirm(*a)
		}
		m.pending = a
		return m, nil
	}
}

// wheel is a swipe on a phone: over the chat it scrolls the agent's
// history, elsewhere it moves the selection, as ↑ and ↓ do.
func (m Model) wheel(ms tea.Mouse) (tea.Model, tea.Cmd) {
	if m.asking() {
		return m, nil
	}
	m.held = nil // a swipe is another key: it drops the question
	if m.overChat(ms) {
		m.armed = ""
		return m.scrollChat(ms.Button == tea.MouseWheelUp), nil
	}
	switch ms.Button {
	case tea.MouseWheelUp:
		m.move(-1)
	case tea.MouseWheelDown:
		m.move(1)
	}
	m.armed = ""
	return m, nil
}

// overChat tells if ms is over the chat: the whole screen of a phone's
// chat, or the chat beside the list.
func (m Model) overChat(ms tea.Mouse) bool {
	if !m.have || m.mismatch || m.pair.owns() || m.sheet != nil || m.form != nil || m.pending != nil {
		return false
	}
	w := max(m.width, minWidth)
	sc := m.draw(w)
	if sc.chatFull {
		return true
	}
	_, detailW := m.panes(w)
	_, chatting := m.chatShown(detailW)
	return chatting && ms.X > sc.listW && ms.Y >= sc.bodyTop && ms.Y < sc.bodyTop+len(sc.rows)
}
