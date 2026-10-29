package app

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/sample"
)

// laneW is a row's lane: a gap, the cells of the past, the cell of now and
// the cells ahead, then room for " 5h resets".
const laneW = 1 + lanePast + 1 + laneAhead + len(" 5h resets")

// laneMin is the narrowest list that has lanes.
const laneMin = 107

// laneCells is a lane on one time scale: the cells of the past (the last
// is the current five minutes), the column of now, blank for the rule,
// then a limit's run to its reset and the window's name. A session seen at
// the window's start began before it.
func laneCells(l [lanePast]laneState, reset time.Time, window string, now time.Time, st styles) string {
	var b strings.Builder
	for i, c := range l {
		if i == 0 && c != laneNone {
			b.WriteString(st.faint.Render("‹"))
			continue
		}
		b.WriteString(laneGlyph(c, st))
	}
	b.WriteString(" ")
	if !reset.After(now) {
		return b.String()
	}
	n := min(int((reset.Sub(now)+laneCell-1)/laneCell), laneAhead)
	b.WriteString(st.faint.Render(strings.Repeat("┄", n)))
	if window != "" {
		b.WriteString(st.muted.Render(" " + window + " resets"))
	}
	return b.String()
}

func laneGlyph(c laneState, st styles) string {
	switch c {
	case laneWork:
		return st.muted.Render("━")
	case laneWait:
		return st.warn.Render("━")
	case laneIdle:
		return st.faint.Render("╌")
	}
	return " "
}

// laneOfRow is the lane of a session's row: a session on a plan's limit
// runs to the reset of the window that stops it.
func (m Model) laneOfRow(s sample.Session) string {
	now := m.now()
	var reset time.Time
	window := ""
	if s.Stopped() {
		reset = s.Out
		for _, a := range m.limitsOf(s.Host) {
			for _, u := range a.Uses(now) {
				if window == "" && u.Resets.Equal(reset) {
					window = u.Name
				}
			}
		}
	}
	return laneCells(m.laneOf(s, now), reset, window, now, m.st)
}

// laneX is the column of a lane's first cell in a list w wide: the track
// ends a margin short of w and opens with a gap.
func (t tracks) laneX(w int) int { return w - t.lane }

// axisLines are the two lines over the lanes: the hours, and a tick at
// each hour back.
func (m Model) axisLines(t tracks, w int) []string {
	x := t.laneX(w)
	hours, ticks := []rune(strings.Repeat(" ", w)), []rune(strings.Repeat(" ", w))
	put := func(line []rune, at int, s string) {
		for i, r := range []rune(s) {
			if at+i < len(line) {
				line[at+i] = r
			}
		}
	}
	put(hours, x, "−2h")
	put(hours, x+lanePast/2, "−1h")
	put(hours, x+lanePast+2, "now")
	put(hours, x+lanePast+laneAhead-2, "+1h")
	put(ticks, x, "┊")
	put(ticks, x+lanePast/2, "┊")
	return []string{m.st.faint.Render(string(hours)), m.st.faint.Render(string(ticks))}
}

// overlayNow draws the rule of now down column x of frame, from line top
// to line bottom: it meets the hairlines and crosses the bars. Every cell
// it touches takes the accent and keeps its background.
func overlayNow(frame string, x, top, bottom int, st styles) string {
	lines := strings.Split(frame, "\n")
	w := 0
	for _, l := range lines {
		w = max(w, lipgloss.Width(l))
	}
	c := lipgloss.NewCanvas(w, len(lines))
	c.Compose(lipgloss.NewLayer(frame))
	for y := top; y <= bottom && y < len(lines); y++ {
		cell := c.CellAt(x, y)
		if cell == nil || cell.Width == 0 {
			continue // the second half of a wide glyph
		}
		next := *cell
		if g := ruleGlyph(cell.Content, y == top, y == bottom); g != "" {
			next.Content, next.Width = g, 1
		}
		next.Style.Fg = st.theme.Accent
		c.SetCell(x, y, &next)
	}
	return c.Render()
}

// ruleGlyph is what the rule makes of a glyph, or "" when the glyph keeps
// itself: a letter only takes the rule's colour.
func ruleGlyph(g string, top, bottom bool) string {
	switch g {
	case " ", "", "┊":
		return "│"
	case "─":
		switch {
		case top:
			return "┬"
		case bottom:
			return "┴"
		}
		return "┼"
	case "━":
		return "┿"
	case "╌":
		return "┼"
	}
	return ""
}
