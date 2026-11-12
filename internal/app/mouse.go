package app

import (
	tea "charm.land/bubbletea/v2"
)

// tap is a click or a finger on a phone. A tap on a tab opens it, on a key
// of the key line presses it, on `● N wait for you` selects the first
// session that waits (on the board `● N need you` clears its filters). A tap
// on the live view goes into the agent's pane: right of the list, or over
// the whole screen of a phone, where its title row goes back to Sessions.
// The demo's stand-in is answered with the belt: a tap on its screen
// presses nothing. A tap on a session's row selects it, and on a phone
// opens its live view; a tap on the selected row goes into its pane. A tap
// on another row selects it and shows the step a second tap runs: an answer for
// a session that waits, else the row's Enter. The second tap runs it when
// it only moves this terminal, or asks to confirm it. A tap is never
// consent to a destructive step, and it cancels a pending one. On the
// board a right-click on a card opens its menu, and while the menu is open
// it takes every tap.
func (m Model) tap(ms tea.Mouse) (tea.Model, tea.Cmd) {
	switch {
	case m.pair.owns() || m.asking(): // the pair page and an ssh prompt take keys only
		return m, nil
	case m.menu != nil && m.boardShown():
		return m.menuTap(ms)
	case ms.Button == tea.MouseRight && m.boardShown():
		return m.boardRightClick(ms)
	case ms.Button != tea.MouseLeft:
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
	if !m.shows() || m.hostCount() == 1 && m.mismatch {
		return m, nil
	}
	m = m.dragStart(ms.X, ms.Y)
	m.searching = false // keep the search; what the tap asks for shows on the key line
	w := max(m.width, minWidth)
	sc := m.draw(w)
	if m.sheet != nil {
		return m.tapSheet(ms, sc)
	}
	if m.form != nil {
		return m.tapForm(ms, sc)
	}
	if sc.placeFull && ms.Y != sc.keysAt {
		switch {
		case ms.Y == 0:
			return m.leaveStandIn(), nil
		case m.standIn:
			return m, nil
		}
		return m.intoSelected()
	}
	switch {
	case ms.Y == 0:
		if s, ok := spanAt([]span{sc.stokerAt}, ms.X); ok && s.press != "" {
			return m.openAgent()
		}
		if s, ok := spanAt([]span{sc.hostAt}, ms.X); ok && s.press != "" {
			return m.keyed(keyPress(s.press)) // the host filter: as s
		}
		if s, ok := spanAt([]span{sc.waits}, ms.X); ok && s.press != "" {
			if m.boardShown() {
				m.clearBoardFilters()
			} else {
				m.setTab(tabSessions)
				if i := m.firstWaiting(); i >= 0 {
					m.selectRow(len(m.doors) + i)
				}
			}
			m.flash = ""
		}
		return m, nil
	case ms.Y == 1:
		if s, ok := spanAt(sc.tabs, ms.X); ok {
			if next, crumb := m.boardTabTap(s.press); crumb {
				return next, nil
			}
			if t, ok := tabOfPress(s.press); ok {
				m.setTab(t)
				m.flash = ""
				return m.seeStoker()
			}
		}
		return m, nil
	case ms.Y == sc.keysAt:
		if s, ok := spanAt(sc.keys, ms.X); ok {
			m.armed = ""
			if s.press == tapBack {
				return m.leaveStandIn(), nil
			}
			return m.keyed(keyPress(s.press))
		}
		return m, nil
	case sc.stripAt > 0 && ms.Y == sc.stripAt:
		if s, ok := spanAt([]span{sc.strip}, ms.X); ok && s.press != "" {
			return m.keyed(keyPress(s.press)) // as u
		}
		return m, nil
	case m.boardShown() && ms.Y >= sc.bodyTop && ms.Y < sc.bodyTop+len(sc.rows): // on Pocket no key line closes the body
		if m.tapWaits(ms.X, ms.Y-sc.bodyTop) {
			m.tapLater = &point{ms.X, ms.Y} // a drag may start here: the tap waits for the release
			return m.holdPress(m, nil, ms)
		}
		next, cmd := m.boardTap(ms.X, ms.Y-sc.bodyTop)
		if nm, ok := next.(Model); ok && (nm.pending != nil || nm.menu != nil || !nm.peekReach()) {
			nm.dragFrom, nm.dragLines = nil, nil // the press opened a box: no drag hides it
			next = nm
		}
		return m.holdPress(next, cmd, ms)
	}
	i := ms.Y - sc.bodyTop
	if m.boardShown() || i < 0 || i >= len(sc.rows) {
		return m, nil // the board took every tap on its body above
	}
	if sc.rows[i] == headerRow {
		return m.tapHeader(ms.X)
	}
	if sc.rows[i] < 0 {
		return m, nil
	}
	picked := m.view().index != nil && m.rowIndex() == sc.rows[i]
	m.selectRow(sc.rows[i])
	if v, ok := m.selectedMachineView(); ok && m.tab == tabMachine {
		// A tap on the selected view row opens it, as Enter does.
		m.armed, m.flash = "", ""
		if picked {
			return m.openView(v).seeStoker() // taps are not wrapped in stokerSeen: the stoker's screen sees its steps here
		}
		return m, nil
	}
	if _, ok := m.selectedHostRow(); ok && m.tab == tabMachine {
		// A tap on the selected host row opens Hosts, as Enter does.
		m.armed, m.flash = "", ""
		if picked {
			return m.openHosts()
		}
		return m, nil
	}
	if host, pane := m.tapTarget(); pane != "" {
		// A tap selects the row, and a tap on it once it is selected goes
		// into its pane, as Enter does (#351, #352).
		m.armed, m.flash = "", ""
		if !picked { // the default selection is not a tap
			return m, nil
		}
		return m.goIntoAsking(host, pane)
	}
	say, run, host := m.tapAct()
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
	if _, door := m.selectedDoor(); !door && m.takeOverOn(host) {
		m.held, m.heldOn, m.flash = run, host, ""
		return m, nil
	}
	return run(m)
}

// tapAct is what a second tap on the selected row does, what the first
// one shows, and the host it acts on: the row's Enter.
func (m Model) tapAct() (string, func(Model) (tea.Model, tea.Cmd), string) {
	a := m.primary()
	if a == nil {
		return "", nil, ""
	}
	return a.String(), func(m Model) (tea.Model, tea.Cmd) {
		if a.pair {
			return m.openPairDoor()
		}
		if a.ghost {
			return m.openNewIn(m.opt.Host, a.where, ""), nil
		}
		if a.nav && !a.destructive {
			return m.confirm(*a)
		}
		m.pending = a
		return m, nil
	}, m.actHost(a)
}

// wheel is a swipe on a phone: over the live view it scrolls the agent's
// screen back; elsewhere it moves the selection, as ↑ and ↓ do.
func (m Model) wheel(ms tea.Mouse) (tea.Model, tea.Cmd) {
	if m.asking() || m.menu != nil && m.boardShown() { // the menu moves only with its keys
		return m, nil
	}
	m.held = nil // a swipe is another key: it drops the question
	if m.boardShown() && !m.standIn && !m.viewFull() {
		return m.boardWheel(ms)
	}
	if m.overView() {
		m.armed = ""
		return m.wheelView(ms.Button == tea.MouseWheelUp)
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

// boardWheel is a wheel step, or a phone's swipe, on the board: over the
// live view beside the columns it scrolls the chosen card's pane, else it
// moves the chosen card as ↑ and ↓ do, by the same keys and their rules.
func (m Model) boardWheel(ms tea.Mouse) (tea.Model, tea.Cmd) {
	up := ms.Button == tea.MouseWheelUp
	if s, ok := m.boardSelected(); ok && m.overBoardView(ms) {
		return m.onCard(s).wheelView(up)
	}
	k := m.keys.main.Down
	if up {
		k = m.keys.main.Up
	} else if ms.Button != tea.MouseWheelDown {
		return m, nil
	}
	next, cmd, _ := m.boardKey(keyPress(k.Keys()[0]))
	return next, cmd
}

// overBoardView tells if ms is over the live view beside the board, in
// the frame View drew last: a swipe's ticks draw nothing again.
func (m Model) overBoardView(ms tea.Mouse) bool {
	w := max(m.width, minWidth)
	sc, kept := m.onScreen(w)
	if !kept {
		sc = m.draw(w)
	}
	return overBoardViewOf(ms, sc)
}

// overBoardViewOf tells if ms is over the live view beside the board of
// the frame sc.
func overBoardViewOf(ms tea.Mouse, sc drawn) bool {
	v := sc.boardView
	return v.x1 > 0 && ms.X >= v.x0 && ms.X < v.x1 && ms.Y >= v.y0 && ms.Y < v.y1
}

// overView tells if the live view is under the finger: the stand-in is
// the whole screen.
func (m Model) overView() bool {
	if !m.have || m.mismatch || m.pair.owns() || m.sheet != nil || m.form != nil || m.pending != nil {
		return false
	}
	w := max(m.width, minWidth)
	sc, kept := m.onScreen(w)
	if !kept {
		sc = m.draw(w)
	}
	return sc.placeFull
}
