package app

import (
	"fmt"
	"strings"

	"github.com/blox-eng/matchblox/internal/sample"
)

// Layout is the shape of the console for a terminal width (DESIGN.md §6).
type Layout int

const (
	// Narrow is a phone: one column, the queue first, two lines a row.
	Narrow Layout = iota
	// Medium drops columns and keeps the detail under the list.
	Medium
	// Wide shows every column.
	Wide
)

func layout(width int) Layout {
	switch {
	case width < 60:
		return Narrow
	case width < 100:
		return Medium
	}
	return Wide
}

// body is a panel: its lines, and for each line the row it shows (-1 for
// none), so a tap knows what it hit.
type body struct {
	lines []string
	rows  []int
}

func (b *body) add(row int, lines ...string) {
	for _, l := range lines {
		b.lines = append(b.lines, l)
		b.rows = append(b.rows, row)
	}
}

// addRow adds the lines of one row, fitted to w, on the selected wash when
// it is the selected row.
func (b *body) addRow(row int, selected bool, w int, st styles, lines ...string) {
	for _, l := range lines {
		l = fit(l, w)
		if selected {
			l = st.selected.Render(l)
		}
		b.add(row, l)
	}
}

func plain(lines []string) body {
	var b body
	b.add(-1, lines...)
	return b
}

// minWidth is the narrowest console drawn; a narrower terminal cuts it.
const minWidth = 30

// chrome is the lines above the body: the header, the tabs, the action
// line and a hairline.
const chrome = 4

// narrowRow is a session on a phone: the match, the state, the name and
// the context on the first line; where it runs on the second.
func (m Model) narrowRow(s sample.Session, selected bool) []string {
	st := m.st
	first := " " + m.matchCell(s) + st.text.Render(pad("busy", colSt-2)) + pad("", colIdle)
	if !s.Busy {
		first = " " + m.matchCell(s) + st.faint.Render(pad("idle", colSt-2)) + st.muted.Render(pad(sample.Human(s.Idle), colIdle))
	}
	first += st.text.Render(s.Name)
	if s.Context == "known" {
		style := st.muted
		switch {
		case s.ContextPct >= m.compactAt():
			style = st.neg
		case s.ContextPct >= 70:
			style = st.warn
		}
		first += "  " + style.Render(fmt.Sprintf("%.0f%%", s.ContextPct))
	}
	switch s.Do {
	case "compact":
		first += "  " + st.neg.Render("! compact")
	case "clear":
		first += "  " + st.warn.Render("▲ clear")
	}
	where := place(s.Target, s.Pane)
	if selected {
		where = "▌" + where
	}
	second := "   " + st.muted.Render(where) + "  " + st.muted.Render(worktree(s.Cwd))
	if s.Progress != nil {
		second += "  " + m.progressCell(s.Progress, 5)
	}
	return []string{first, second}
}

// visible is the part of the body that fits under the chrome: the lines
// above the first row stay, and the rows scroll so the selected one is on
// the screen. A tap reads the same lines the screen shows.
func (m Model) visible(w int) body {
	b := m.body(w)
	room := max(m.height-chrome, 0)
	if len(b.lines) <= room {
		return b
	}
	head := 0
	for head < len(b.rows) && b.rows[head] < 0 {
		head++
	}
	if head >= len(b.rows) || head > room/2 {
		return body{lines: b.lines[:room], rows: b.rows[:room]}
	}
	first, last := -1, -1
	sel := m.rowIndex()
	for i, r := range b.rows {
		if r == sel {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	space := room - head
	start := head
	if first >= 0 {
		start = min(max(first-space/2, head), len(b.lines)-space)
		start = max(min(start, first), last-space+1, head)
	}
	return body{
		lines: append(append([]string{}, b.lines[:head]...), b.lines[start:start+space]...),
		rows:  append(append([]int{}, b.rows[:head]...), b.rows[start:start+space]...),
	}
}

// progressCell is the bar an agent reported, in n cells and its percent.
func (m Model) progressCell(p *sample.Progress, n int) string {
	if p == nil {
		return ""
	}
	full := min(max((p.Pct*n+50)/100, 0), n)
	return m.st.text.Render(strings.Repeat("▰", full)) + m.st.faint.Render(strings.Repeat("▱", n-full)) +
		" " + m.st.muted.Render(fmt.Sprintf("%d%%", p.Pct))
}

// sessionIn is the agent session of a pane.
func (m Model) sessionIn(pane string) (sample.Session, bool) {
	for _, s := range m.all.sessions {
		if s.Pane == pane {
			return s, true
		}
	}
	return sample.Session{}, false
}
