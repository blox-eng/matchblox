package app

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
)

// stackRows splits the body rows of the stacked board (owner,
// 2026-11-14). NEEDS YOU takes its cards and its head first, but leaves 2
// rows to each other section shown. A blank row sits between sections and
// is the first to go when rows are short. SHIPPING sits at the bottom with
// its head and a row for each of its ships matches, 3 at most. WORKING
// takes the rest, so its head moves only with NEEDS YOU, and SHIPPING's
// only with NEEDS YOU and a count of ships under 4.
func stackRows(need, ships, rows int, work, ship bool) (needH, workH, shipH, gaps int) {
	shown := 0
	if work {
		shown++
	}
	if ship {
		shown++
	}
	if need > 0 {
		needH = max(min(need+1, rows-2*shown), min(need+1, rows, 2)) // short of rows, NEEDS YOU keeps its head and a row
	}
	sections := shown
	if needH > 0 {
		sections++
	}
	gaps = max(0, min(min(sections-1, 2), rows-needH-2*shown))
	rest := max(0, rows-needH-gaps)
	switch {
	case work && ship && rest < 4:
		workH = min(rest, 2)
		shipH = rest - workH
	case work && ship:
		shipH = min(1+max(min(ships, 3), 1), rest-2)
		workH = rest - shipH
	case work:
		workH = rest
	case ship:
		shipH = rest
	}
	return needH, workH, shipH, gaps
}

// boardStacked is the board at Medium: NEEDS YOU, WORKING and SHIPPING
// from the top, each the full width under a head of its own line, then the
// room's talk lines and the QUIET drawer. A tap on a head presses its
// column's short name, as on a column's head.
func (m Model) boardStacked(d *boardDrawn, b *boardDraw, shown []boardCol, w, rows int, talk []roomDrawn) {
	st := m.st
	b.stacked = true
	drawer := 0
	if slices.Contains(shown, colQuiet) {
		drawer = 2
		if b.open == colQuiet {
			drawer = 2 + quietRows
		}
		if b.open != colQuiet && rows-drawer-len(talk) < 2 {
			drawer = 0 // the chosen card's head outranks the shelf
		}
	}
	sec := max(rows-drawer-len(talk), 0)
	need := 0
	if slices.Contains(shown, colNeed) {
		need = 2 // a blank row, and that nothing needs you
		if ss := b.idx[colNeed]; len(ss) > 0 {
			need = 0
			for _, s := range ss {
				need += len(m.boardBlockOf(b, colNeed, s, w, false).lines)
			}
		}
	}
	needH, workH, shipH, gaps := stackRows(need, len(b.idx[colShip]), sec, slices.Contains(shown, colWork), slices.Contains(shown, colShip))
	for _, c := range []boardCol{colWork, colShip} {
		b.nameW[c] = 14
		for _, s := range b.idx[c] {
			b.nameW[c] = max(b.nameW[c], min(lipgloss.Width(m.cardName(s)+m.hostTag(s, cardOpts{otherHost: m.otherHost(s)})), w/3))
		}
	}
	top := len(d.lines)
	for _, sc := range []struct {
		c boardCol
		h int
	}{{colNeed, needH}, {colWork, workH}, {colShip, shipH}} {
		if sc.h <= 0 {
			continue
		}
		if len(d.lines) > top && gaps > 0 {
			d.add(-1, strings.Repeat(" ", w))
			gaps--
		}
		ss := b.idx[sc.c]
		on, more := b, 0
		if sc.h == 2 && len(ss) > 1 { // one row: the chosen card, else the first, and the head counts the rest
			one := *b
			card := ss[0]
			if b.picked && b.open == sc.c {
				card = b.chosen
			}
			one.idx[sc.c] = []sample.Session{card}
			one.oneRow[sc.c] = true
			on, more = &one, len(ss)-1
		}
		if d.headAt < 0 {
			d.headAt = len(d.lines)
		}
		head := m.boardHead(sc.c, len(ss), more, w, sc.c == b.open)
		d.taps[len(d.lines)] = []span{{0, lipgloss.Width(strings.TrimRight(ansi.Strip(head), " ")), sc.c.short()}} // the name, the count and the −
		d.add(-1, head)
		m.boardStack(d, on, sc.c, len(d.lines), 0, w, sc.h-1, false)
	}
	for len(d.lines) < top+sec {
		d.add(-1, strings.Repeat(" ", w))
	}
	d.colsTo = top + sec
	if words := m.boardWords(b); words != "" && d.headAt >= 0 && sec > 1 {
		d.lines[d.headAt+1] = fit("  "+st.faint.Render(words), w)
	}
	for _, t := range talk {
		d.roomAt[len(d.lines)] = t.at
		d.addRow(-1, t.sel, w, st, t.text)
	}
	if drawer > 0 {
		m.boardDrawer(d, b, w)
	}
}
