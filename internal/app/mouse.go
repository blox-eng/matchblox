package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// tap is a click or a finger on a phone. A tap on a tab opens it. A tap on
// a row selects it and shows the step a second tap runs; the second tap
// runs it when it only moves this terminal, or asks to confirm it. A tap is
// never consent to a destructive step, and it cancels a pending one.
func (m Model) tap(ms tea.Mouse) (tea.Model, tea.Cmd) {
	if ms.Button != tea.MouseLeft {
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
	w := max(m.width, minWidth)
	if ms.Y == 1 {
		if tab, ok := m.tabAt(w, ms.X); ok {
			m.tab, m.flash, m.armed = tab, "", ""
		}
		return m, nil
	}
	b := m.visible(w)
	i := ms.Y - chrome
	if i < 0 || i >= len(b.rows) || b.rows[i] < 0 {
		return m, nil
	}
	m.selectRow(b.rows[i])
	a := m.primary()
	if a == nil {
		m.armed, m.flash = "", "nothing to do here"
		return m, nil
	}
	// The second tap runs what the first one showed, and only that: a row
	// that moved under the finger shows its own step first.
	if m.armed != a.String() {
		m.armed, m.flash = a.String(), "tap again: "+a.String()
		return m, nil
	}
	m.armed = ""
	if a.nav && !a.destructive {
		return m.confirm(*a)
	}
	m.pending = a
	return m, nil
}

// wheel is a swipe on a phone: it moves the selection, as ↑ and ↓ do.
func (m Model) wheel(ms tea.Mouse) (tea.Model, tea.Cmd) {
	switch ms.Button {
	case tea.MouseWheelUp:
		m.move(-1)
	case tea.MouseWheelDown:
		m.move(1)
	}
	m.armed = ""
	return m, nil
}

// tabAt is the tab under column x of the tab line.
func (m Model) tabAt(w, x int) (int, bool) {
	t := m.tabTier(w)
	start := 1
	for i, p := range m.tabParts(t) {
		pw := ansi.StringWidth(p)
		if x >= start && x < start+pw {
			return i, true
		}
		start += pw + len(t.gap)
	}
	return 0, false
}

// rowIndex is the selected row of the open tab.
func (m Model) rowIndex() int {
	switch m.tab {
	case tabQueue:
		return m.queueIndex()
	case tabPanes:
		return min(m.paneSel, max(len(m.paneRows())-1, 0))
	case tabSessions:
		return m.selIndex()
	case tabProcs:
		return m.orphanIndex()
	case tabGit:
		return min(m.gitSel, max(len(m.worktreeRows())-1, 0))
	}
	return 0
}

func (m Model) rowCount() int {
	switch m.tab {
	case tabQueue:
		return len(m.queue) + len(m.recs)
	case tabPanes:
		return len(m.paneRows())
	case tabSessions:
		return len(m.snap.Sessions)
	case tabProcs:
		return len(m.snap.Orphans)
	case tabGit:
		return len(m.worktreeRows())
	}
	return 0
}

func (m *Model) selectRow(i int) {
	if i < 0 || i >= m.rowCount() {
		return
	}
	switch m.tab {
	case tabQueue:
		if i < len(m.queue) {
			m.queuePane, m.recPick = m.queue[i].Pane, ""
		} else {
			m.recPick = m.recs[i-len(m.queue)].ID
		}
	case tabPanes:
		m.paneSel = i
	case tabSessions:
		m.selPID = m.snap.Sessions[i].PID
	case tabProcs:
		m.orphanPID = m.snap.Orphans[i].PID
	case tabGit:
		m.gitSel = i
	}
}
