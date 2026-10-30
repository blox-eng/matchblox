package app

import (
	"strconv"
	"strings"

	bkey "charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

// tabView is one tab: its rows and selection, what Enter and x do on the
// selected row, its keys and its body. The selection itself stays in the
// Model, so it lives across tab changes and new states.
type tabView struct {
	name      string
	keys      func(m Model) []hint // the key line, for the selected row
	count     func(m Model) int
	index     func(m Model) int
	pick      func(m *Model, i int)
	primary   func(m Model) *action // Enter: never destructive
	secondary func(m Model) *action // x: destructive, behind a typed y
	body      func(m Model, w int) body
}

// hint is a key on the key line: the key, its word, and the key a tap on
// it presses ("" for none).
type hint struct{ key, label, press string }

// hintFor is a binding's hint: its help key, the label (the binding's own
// word when ""), and its first key as the press. A disabled binding is the
// zero hint, which a key line skips.
func hintFor(b bkey.Binding, label string) hint {
	if !b.Enabled() {
		return hint{}
	}
	if label == "" {
		label = b.Help().Desc
	}
	return hint{b.Help().Key, label, b.Keys()[0]}
}

// tabHint is the hint of tab i: its own digit, as the tabs binding has it.
func tabHint(b bkey.Binding, i int, label string) hint {
	if !b.Enabled() || i >= len(b.Keys()) {
		return hint{}
	}
	return hint{b.Keys()[i], label, b.Keys()[i]}
}

// noTap is the hint without the key a tap presses: a key the line shows
// but does not take.
func noTap(h hint) hint {
	h.press = ""
	return h
}

// plainHints is hints as one line of plain text, key and word each, two
// spaces between: for a line drawn in a single style.
func plainHints(hs ...hint) string {
	var parts []string
	for _, h := range hs {
		if h == (hint{}) {
			continue
		}
		parts = append(parts, strings.TrimSpace(h.key+" "+h.label))
	}
	return strings.Join(parts, "  ")
}

// keyWord is a binding's help key in its style, then rest in the word style:
// the key and a phrase of prose after it.
func keyWord(b bkey.Binding, key, word lipgloss.Style, rest string) string {
	return key.Render(b.Help().Key) + word.Render(rest)
}

// footKey is a binding's part of a box footer that no tap presses, its key
// in the given style.
func (st styles) footKey(b bkey.Binding, label, sep string, key lipgloss.Style) footPart {
	if !b.Enabled() {
		return footPart{}
	}
	t := key.Render(b.Help().Key)
	if label != "" {
		t += st.muted.Render(" " + label)
	}
	return footPart{t, "", sep}
}

// footFor is a binding's part of a box footer: the same rules as hintFor,
// the key in the accent and its word muted.
func (st styles) footFor(b bkey.Binding, label, sep string) footPart {
	h := hintFor(b, label)
	if h == (hint{}) {
		return footPart{}
	}
	t := st.accent.Render(h.key)
	if h.label != "" {
		t += st.muted.Render(" " + h.label)
	}
	return footPart{t, h.press, sep}
}

const (
	tabSessions = iota
	tabShips
	tabMachine
	tabGit
	tabHistory
	tabPanes
	tabStoker
)

var views = [...]tabView{
	tabSessions: {
		name:  "sessions",
		keys:  Model.sessionKeys,
		count: func(m Model) int { return len(m.doors) + len(m.snap.Sessions) + len(m.ghosts()) + m.recRows() },
		index: Model.sessIndex,
		pick: func(m *Model, i int) {
			nd, ns, ng := len(m.doors), len(m.snap.Sessions), len(m.ghosts())
			switch {
			case i < nd:
				m.doorPick, m.doorAt, m.recPick, m.ghostPick = m.doors[i].ID, i, "", 0
			case i < nd+ns:
				m.doorPick, m.recPick, m.ghostPick, m.chose, m.selPID = "", "", 0, true, m.snap.Sessions[i-nd].PID
			case i < nd+ns+ng:
				m.doorPick, m.recPick, m.ghostPick = "", "", i-nd-ns+1
			default:
				m.doorPick, m.recPick, m.ghostPick = "", m.recs[i-nd-ns-ng].ID, 0
			}
		},
		primary: func(m Model) *action {
			if d, ok := m.selectedDoor(); ok {
				return m.doorAction(d)
			}
			if r, ok := m.selectedRec(); ok {
				return fromAdvice(r, "primary")
			}
			if g, ok := m.selectedGhost(); ok {
				return ghostAction(g)
			}
			if s, ok := m.selectedSession(); ok && s.Pane != "" {
				return m.jump(s.Pane)
			}
			return nil
		},
		secondary: func(m Model) *action {
			if d, ok := m.selectedDoor(); ok {
				return closeDoor(d)
			}
			if r, ok := m.selectedRec(); ok {
				return fromAdvice(r, "secondary")
			}
			if s, ok := m.selectedSession(); ok {
				return m.killAction(s)
			}
			return nil
		},
		body: Model.sessionsPanel,
	},
	tabShips: {
		name: "ships",
		keys: func(m Model) []hint {
			mk := m.keys.main
			if sh, ok := m.selectedShip(); ok {
				return append(m.shipKeys(sh), hintFor(mk.Stoke, "stoker"))
			}
			return []hint{tabHint(mk.Tabs, tabSessions, "sessions"), hintFor(mk.Stoke, "stoker")}
		},
		count:     func(m Model) int { return len(m.ships()) },
		index:     func(m Model) int { return min(m.shipSel, max(len(m.ships())-1, 0)) },
		pick:      func(m *Model, i int) { m.shipSel = i },
		secondary: Model.mergeShip,
		body:      Model.shipsPanel,
	},
	tabMachine: {
		name: "machine",
		keys: func(m Model) []hint {
			mk := m.keys.main
			stoke := hintFor(mk.Stoke, "stoker")
			if r, ok := m.selectedDevice(); ok {
				if r.plus {
					return []hint{hintFor(mk.Open, "pair"), stoke}
				}
				return []hint{hintFor(mk.Second, "remove"), stoke}
			}
			if len(m.snap.Orphans) == 0 {
				return []hint{hintFor(mk.Ships, "ships"), stoke}
			}
			return []hint{hintFor(mk.Open, "jump"), hintFor(mk.Second, "kill"), stoke}
		},
		count: Model.machineRows,
		index: Model.machineIndex,
		pick:  (*Model).machinePick,
		primary: func(m Model) *action {
			if r, ok := m.selectedDevice(); ok {
				if r.plus {
					return &action{label: "pair a device", say: "pair a device: show a one-time code", pair: true}
				}
				return nil
			}
			if o, ok := m.selectedOrphan(); ok && o.PaneAlive {
				return m.jump(o.Pane)
			}
			return nil
		},
		secondary: func(m Model) *action {
			if r, ok := m.selectedDevice(); ok {
				if r.plus {
					return nil
				}
				return m.removeDevice(r)
			}
			if o, ok := m.selectedOrphan(); ok {
				return &action{label: "kill", steps: [][]string{o.Kill}, destructive: true,
					rec: "orphan:" + strconv.Itoa(o.PID), which: "secondary"}
			}
			return nil
		},
		body: Model.machinePanel,
	},
	tabGit: {
		name: "git",
		keys: func(m Model) []hint {
			mk := m.keys.main
			return []hint{hintFor(mk.Open, "shell"), hintFor(mk.Mark, ""), hintFor(mk.MarkSafe, ""), hintFor(mk.Second, "remove"), hintFor(mk.Rename, "rescan")}
		},
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
		name: "history",
		keys: func(m Model) []hint {
			return []hint{hintFor(m.keys.main.Ships, "ships"), hintFor(m.keys.main.Stoke, "stoker")}
		},
		body: func(m Model, w int) body { return plain(m.historyPanel(w)) },
	},
	tabPanes: {
		name:  "panes",
		keys:  func(m Model) []hint { return []hint{hintFor(m.keys.main.Open, "go"), hintFor(m.keys.main.Sort, "")} },
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
	tabStoker: {
		name:  "stoker",
		keys:  Model.stokerKeys,
		count: func(m Model) int { return len(m.stokerRows()) },
		index: func(m Model) int { return min(m.stokerSel, max(len(m.stokerRows())-1, 0)) },
		pick:  func(m *Model, i int) { m.stokerSel = i },
		body:  Model.stokerBody,
	},
}

// tabNames names the tabs, in the order of their digits.
var tabNames = func() []string {
	var names []string
	for _, v := range views {
		names = append(names, v.name)
	}
	return names
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
