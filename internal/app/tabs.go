package app

import "strconv"

// tabView is one tab: its rows and selection, what Enter and x do on the
// selected row, its keys and its body. The selection itself stays in the
// Model, so it lives across tab changes and new states.
type tabView struct {
	name, short string
	keys        string
	count       func(m Model) int
	index       func(m Model) int
	pick        func(m *Model, i int)
	primary     func(m Model) *action // Enter: never destructive
	secondary   func(m Model) *action // x: destructive, behind a typed y
	body        func(m Model, w int) body
}

const (
	tabQueue = iota
	tabSessions
	tabMachine
	tabProcs
	tabGit
	tabHistory
	tabPanes
)

var views = [...]tabView{
	tabQueue: {
		name: "queue", short: "queue", keys: "↑↓ select  ⏎ go  a answer",
		count: func(m Model) int { return len(m.doors) + len(m.queue) + len(m.recs) },
		index: Model.queueIndex,
		pick: func(m *Model, i int) {
			nd := len(m.doors)
			switch {
			case i < nd:
				m.doorPick, m.doorAt, m.queuePane, m.recPick = m.doors[i].ID, i, "", ""
			case i < nd+len(m.queue):
				m.doorPick, m.queuePane, m.recPick = "", m.queue[i-nd].Pane, ""
			default:
				m.doorPick, m.recPick = "", m.recs[i-nd-len(m.queue)].ID
			}
		},
		primary: func(m Model) *action {
			if d, ok := m.selectedDoor(); ok {
				return m.doorAction(d)
			}
			if it, ok := m.selectedQueue(); ok && it.Pane != "" {
				return m.jump(it.Pane)
			}
			if r, ok := m.selectedRec(); ok {
				return fromAdvice(r, "primary")
			}
			return nil
		},
		secondary: func(m Model) *action {
			if d, ok := m.selectedDoor(); ok && d.ID != stokedRow {
				return closeDoor(d)
			}
			if r, ok := m.selectedRec(); ok {
				return fromAdvice(r, "secondary")
			}
			return nil
		},
		body: Model.queuePanel,
	},
	tabSessions: {
		name: "sessions", short: "sess", keys: "↑↓ select  ⏎ jump  a answer  s sort",
		count: func(m Model) int { return len(m.snap.Sessions) },
		index: Model.selIndex,
		pick:  func(m *Model, i int) { m.selPID = m.snap.Sessions[i].PID },
		primary: func(m Model) *action {
			if s, ok := m.selected(); ok && s.Pane != "" {
				return m.jump(s.Pane)
			}
			return nil
		},
		secondary: func(m Model) *action {
			if s, ok := m.selected(); ok && s.Age() == "stale" {
				return &action{label: "end", steps: [][]string{{"kill", strconv.Itoa(s.PID)}}, destructive: true,
					rec: "session:" + strconv.Itoa(s.PID), which: "secondary"}
			}
			return nil
		},
		body: func(m Model, w int) body { return m.sessions(w, m.height-chrome-1) },
	},
	tabMachine: {
		name: "machine", short: "mach",
		body: func(m Model, w int) body { return plain(m.machine(w)) },
	},
	tabProcs: {
		name: "procs", short: "procs", keys: "↑↓ select  ⏎ jump  x kill",
		count: func(m Model) int { return len(m.snap.Orphans) },
		index: Model.orphanIndex,
		pick:  func(m *Model, i int) { m.orphanPID = m.snap.Orphans[i].PID },
		primary: func(m Model) *action {
			if o, ok := m.selectedOrphan(); ok && o.PaneAlive {
				return m.jump(o.Pane)
			}
			return nil
		},
		secondary: func(m Model) *action {
			if o, ok := m.selectedOrphan(); ok {
				return &action{label: "kill", steps: [][]string{o.Kill}, destructive: true,
					rec: "orphan:" + strconv.Itoa(o.PID), which: "secondary"}
			}
			return nil
		},
		body: Model.procs,
	},
	tabGit: {
		name: "git", short: "git", keys: "↑↓ select  ⏎ shell  space mark  X all safe  x remove  r rescan",
		count: func(m Model) int { return len(m.worktreeRows()) },
		index: func(m Model) int { return min(m.gitSel, max(len(m.worktreeRows())-1, 0)) },
		pick:  func(m *Model, i int) { m.gitSel = i },
		primary: func(m Model) *action {
			if wt, ok := m.selectedWorktree(); ok {
				return &action{label: "shell", nav: true, steps: [][]string{{"tmux", "new-window", "-c", wt.Path}}}
			}
			return nil
		},
		secondary: func(m Model) *action {
			if a := m.removeMarked(); a != nil {
				return a
			}
			if wt, ok := m.selectedWorktree(); ok && wt.Safe {
				return &action{label: "remove", steps: [][]string{wt.Remove}, destructive: true,
					rec: "worktree:" + wt.Path, which: "secondary"}
			}
			return nil
		},
		body: Model.gitPanel,
	},
	tabHistory: {
		name: "history", short: "hist",
		body: func(m Model, w int) body { return plain(m.historyPanel(w)) },
	},
	tabPanes: {
		name: "panes", short: "panes", keys: "↑↓ select  ⏎ go  s sort",
		count: func(m Model) int { return len(m.paneRows()) },
		index: func(m Model) int { return min(m.paneSel, max(len(m.paneRows())-1, 0)) },
		pick:  func(m *Model, i int) { m.paneSel = i },
		primary: func(m Model) *action {
			if p, ok := m.selectedPane(); ok && p.id != "" {
				return m.jump(p.id)
			}
			return nil
		},
		body: Model.panesPanel,
	},
}

// tabNames and tabShort name the tabs, long and short.
var tabNames, tabShort = func() ([]string, []string) {
	var long, short []string
	for _, v := range views {
		long, short = append(long, v.name), append(short, v.short)
	}
	return long, short
}()

func (m Model) view() tabView { return views[m.tab] }

// rowIndex is the selected row of the open tab.
func (m Model) rowIndex() int {
	if v := m.view(); v.index != nil {
		return v.index(m)
	}
	return 0
}

func (m Model) rowCount() int {
	if v := m.view(); v.count != nil {
		return v.count(m)
	}
	return 0
}

func (m *Model) selectRow(i int) {
	if v := m.view(); v.pick != nil && i >= 0 && i < m.rowCount() {
		v.pick(m, i)
	}
}

func (m Model) primary() *action {
	if v := m.view(); v.primary != nil {
		return m.forHost(v.primary(m))
	}
	return nil
}

func (m Model) secondary() *action {
	if v := m.view(); v.secondary != nil {
		return m.forHost(v.secondary(m))
	}
	return nil
}

func (m Model) body(w int) body { return m.view().body(m, w) }
