package app

import (
	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/sample"
)

// Layout is the shape of the console for a terminal (DESIGN.md §6).
type Layout int

const (
	// Narrow is a phone: the board one column, a page at a time.
	Narrow Layout = iota
	// Medium is a phone opened up: the board's columns, QUIET a drawer.
	Medium
	// Wide is a desk: every column.
	Wide
)

// layout picks the shape from the screen's shape and width. A cell is
// about twice as tall as it is wide, so a screen is portrait when
// rows*2 >= cols: a phone held upright, whatever its font, keeps its
// layout when its keyboard shrinks the screen. Upright, 90 columns and
// more are Medium; fewer are Narrow.
// A landscape screen goes by its width.
func layout(cols, rows int) Layout {
	if portrait(cols, rows) {
		if cols >= 90 {
			return Medium
		}
		return Narrow
	}
	return byWidth(cols)
}

// portrait is a screen taller than it is wide. Zero rows is an unknown
// height: not portrait.
func portrait(cols, rows int) bool { return rows > 0 && rows*2 >= cols }

func byWidth(cols int) Layout {
	switch {
	case cols < 60:
		return Narrow
	case cols < 100:
		return Medium
	}
	return Wide
}

// layoutAt is the shape a panel of w columns draws in: the screen's, or
// narrower when the panel is.
func (m Model) layoutAt(w int) Layout {
	if m.width == 0 {
		return byWidth(w)
	}
	return min(layout(m.width, m.shapeRows()), byWidth(w))
}

// shapeRows is the height the screen's shape goes by: the tallest seen at
// its width, so a keyboard that shrinks the screen never changes the
// layout; a new width decides afresh (design/0006 §4).
func (m Model) shapeRows() int {
	if m.tallW == m.width {
		return max(m.tallH, m.height)
	}
	return m.height
}

// upright is a portrait screen: its boxes sit at the bottom, as on a phone.
func (m Model) upright() bool { return portrait(m.width, m.shapeRows()) }

// body is a panel: its lines, and for each line the row it shows (-1 for
// none), so a tap knows what it hit. Machine's heads are the line of each
// host's section head, by host.
type body struct {
	lines []string
	rows  []int
	heads map[string]int
}

func (b *body) add(row int, lines ...string) {
	for _, l := range lines {
		b.lines = append(b.lines, l)
		b.rows = append(b.rows, row)
	}
}

// addRow adds the lines of one row, fitted to w, on the selected wash when
// it is the selected row: every cell of it, the styled ones too.
func (b *body) addRow(row int, selected bool, w int, st styles, lines ...string) {
	for _, l := range lines {
		l = fit(l, w)
		if selected {
			l = paint(st.selected.Render(l), st.theme.AccentWash, w)
		}
		b.add(row, l)
	}
}

// ink is the name of a row: the accent on the selected row, muted on every
// other, so the cursor is the brightest thing in a list.
func (st styles) ink(selected bool) lipgloss.Style {
	if selected {
		return st.accent
	}
	return st.muted
}

func plain(lines []string) body {
	var b body
	b.add(-1, lines...)
	return b
}

const (
	colPane = 11 // session:window.pane
	colName = 17
)

// minWidth is the narrowest console drawn; a narrower terminal cuts it.
const minWidth = 30

// sessionIn is the agent session of a pane of host.
func (m Model) sessionIn(host, pane string) (sample.Session, bool) {
	if pane == "" {
		return sample.Session{}, false // outside tmux: no pane tells sessions apart
	}
	for _, s := range m.all.sessions {
		if s.Host == host && s.Pane == pane {
			return s, true
		}
	}
	return sample.Session{}, false
}

// busyWord is the state of a busy session: a turn runs, or only its
// subagents work and their results start its next turn.
func busyWord(s sample.Session) string {
	if s.Status == sample.StatusAgents {
		return "agents"
	}
	return "busy"
}
