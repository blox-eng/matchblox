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
			m.tab, m.flash = tab, ""
		}
		return m, nil
	}
	b := m.body(w)
	i := ms.Y - chrome
	if i < 0 || i >= len(b.rows) || i >= m.height-chrome-1 || b.rows[i] < 0 {
		return m, nil
	}
	row := b.rows[i]
	if row == m.rowIndex() {
		a := m.primary()
		if a == nil {
			return m, nil
		}
		if a.nav && !a.destructive {
			return m.confirm(*a)
		}
		m.pending = a
		return m, nil
	}
	m.selectRow(row)
	m.flash = ""
	if a := m.primary(); a != nil {
		m.flash = "tap again: " + a.String()
	}
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
		return m.queueSelIndex()
	case tabPanes:
		return min(m.paneSel, max(len(m.paneRows())-1, 0))
	case tabSessions:
		return m.selIndex()
	case tabProcs:
		return m.orphanIndex()
	case tabGit:
		return min(m.gitSel, max(len(m.worktreeRows())-1, 0))
	case tabRecs:
		return min(m.recSel, max(len(m.recs)-1, 0))
	}
	return 0
}

func (m Model) rowCount() int {
	switch m.tab {
	case tabQueue:
		return len(m.queue)
	case tabPanes:
		return len(m.paneRows())
	case tabSessions:
		return len(m.snap.Sessions)
	case tabProcs:
		return len(m.snap.Orphans)
	case tabGit:
		return len(m.worktreeRows())
	case tabRecs:
		return len(m.recs)
	}
	return 0
}

func (m *Model) selectRow(i int) {
	if i < 0 || i >= m.rowCount() {
		return
	}
	switch m.tab {
	case tabQueue:
		m.queuePane = m.queue[i].Pane
	case tabPanes:
		m.paneSel = i
	case tabSessions:
		m.selPID = m.snap.Sessions[i].PID
	case tabProcs:
		m.orphanPID = m.snap.Orphans[i].PID
	case tabGit:
		m.gitSel = i
	case tabRecs:
		m.recSel = i
	}
}
