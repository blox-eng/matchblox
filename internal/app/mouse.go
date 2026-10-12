package app

import (
	tea "charm.land/bubbletea/v2"
)

// tap is a click or a finger on a phone. A tap on a tab opens it, on a key
// of the key line presses it, on `● N wait for you` selects the first
// session that waits, on the dock's `machine ›` switches the panel. A tap
// on a row selects it and shows the step a second tap runs: an answer for
// a session that waits, else the row's Enter. The second tap runs it when
// it only moves this terminal, or asks to confirm it. A tap is never
// consent to a destructive step, and it cancels a pending one.
func (m Model) tap(ms tea.Mouse) (tea.Model, tea.Cmd) {
	if ms.Button != tea.MouseLeft {
		return m, nil
	}
	if m.held != nil { // a tap is another key: it drops the question
		m.held, m.flash = nil, ""
		return m, nil
	}
	if m.splashing() {
		m.splashDone = true
		return m, nil
	}
	if m.pending != nil {
		m.flash, m.pending = "cancelled: "+m.pending.String(), nil
		return m, nil
	}
	if m.input != nil || !m.have || m.mismatch {
		return m, nil
	}
	m.searching = false // keep the search; what the tap asks for shows on the key line
	w := max(m.width, minWidth)
	sc := m.draw(w)
	switch {
	case ms.Y == 0:
		if s, ok := spanAt([]span{sc.waits}, ms.X); ok && s.press != "" {
			m.setTab(tabSessions)
			if i := m.firstWaiting(); i >= 0 {
				m.selectRow(len(m.doors) + len(m.recs) + i)
			}
			m.flash = ""
		}
		return m, nil
	case ms.Y == 1:
		if s, ok := spanAt(sc.tabs, ms.X); ok {
			m.setTab(int(s.press[0] - '1'))
			m.flash = ""
		}
		return m, nil
	case ms.Y == sc.keysAt:
		if s, ok := spanAt(sc.keys, ms.X); ok {
			m.armed = ""
			return m.key(s.press)
		}
		return m, nil
	case ms.Y == sc.dockAt && layout(w) == Narrow:
		if x0, x1 := m.dockToggle(w); ms.X >= x0 && ms.X < x1 {
			m.dockMachine = !m.dockMachine
		}
		return m, nil
	}
	i := ms.Y - sc.bodyTop
	if i < 0 || i >= len(sc.rows) || ms.X >= sc.listW {
		return m, nil
	}
	if sc.rows[i] == headerRow {
		return m.tapHeader(ms.X)
	}
	if sc.rows[i] < 0 {
		return m, nil
	}
	m.selectRow(sc.rows[i])
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
// first one shows: a session that waits for an answer gets the answer
// line, any other row its Enter.
func (m Model) tapAct() (string, func(Model) (tea.Model, tea.Cmd)) {
	if pane, _ := m.answerTarget(); pane != "" {
		s, _ := m.selectedSession()
		return "answer " + s.Name, func(m Model) (tea.Model, tea.Cmd) {
			m.input = &answerInput{pane: pane}
			return m, nil
		}
	}
	a := m.primary()
	if a == nil {
		return "", nil
	}
	return a.String(), func(m Model) (tea.Model, tea.Cmd) {
		if a.nav && !a.destructive {
			return m.confirm(*a)
		}
		m.pending = a
		return m, nil
	}
}

// wheel is a swipe on a phone: it moves the selection, as ↑ and ↓ do.
func (m Model) wheel(ms tea.Mouse) (tea.Model, tea.Cmd) {
	m.held = nil // a swipe is another key: it drops the question
	switch ms.Button {
	case tea.MouseWheelUp:
		m.move(-1)
	case tea.MouseWheelDown:
		m.move(1)
	}
	m.armed = ""
	return m, nil
}
