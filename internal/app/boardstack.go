package app

import (
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
)

// stackRows splits the body rows of the stacked board (owner,
// 2026-11-14). NEEDS YOU takes its cards and its head first, but leaves 2
// rows to each other section shown; short of rows each section keeps its
// head before NEEDS YOU keeps a card. A blank row sits between sections and
// is the first to go when rows are short. SHIPPING sits at the bottom with
// its head and a row for each of its ships, 3 at most. WORKING
// takes the rest, so its head moves only with NEEDS YOU, and SHIPPING's
// only with NEEDS YOU and a count of ships under 4.
func stackRows(need, ships, rows int, needs, work, ship bool) (needH, workH, shipH, gaps int) {
	shown := 0
	if work {
		shown++
	}
	if ship {
		shown++
	}
	if needs {
		if needH = min(need+1, rows-2*shown); needH < 2 {
			needH = max(min(need+1, 2, rows-shown), min(rows, 1)) // the heads first, then a card
		}
	}
	sections := shown
	if needH > 0 {
		sections++
	}
	gaps = max(0, min(min(sections-1, 2), rows-needH-2*shown))
	rest := max(0, rows-needH-gaps)
	switch {
	case work && ship && rest < 4:
		workH = max(min(rest-1, 2), 0)
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
	// Short of rows the heads of the sections with cards come first, then the
	// chosen card's head, then the heads of empty sections, then the drawer,
	// whose foot is cut.
	stack := slices.DeleteFunc(slices.Clone(shown), func(c boardCol) bool { return c == colQuiet })
	if rows-len(talk) <= len(stack) {
		stack = slices.DeleteFunc(stack, func(c boardCol) bool { return len(b.idx[c]) == 0 })
	}
	reserve := len(stack)
	if b.picked && b.open != colQuiet {
		reserve++
	}
	drawer = min(drawer, max(rows-len(talk)-reserve, 0))
	sec := max(rows-drawer-len(talk), 0)
	if sec < len(stack) {
		m.stackNames(d, b, stack, w, sec)
		m.stackFoot(d, b, w, talk, drawer)
		return
	}
	need := 0
	switch ss := b.idx[colNeed]; {
	case len(ss) > 0:
		for _, s := range ss {
			need += len(m.boardBlockOf(b, colNeed, s, w, false).lines)
		}
	case m.filter == "" && m.boardChain == "" && m.boardHost == "":
		need = 2 // a blank row, and that nothing needs you
	case m.boardWords(b) != "":
		need = 1 // the row that says why there is no card
	}
	needH, workH, shipH, gaps := stackRows(need, len(b.idx[colShip]), sec, slices.Contains(stack, colNeed), slices.Contains(stack, colWork), slices.Contains(stack, colShip))
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
		if sc.h == 1 {
			more = len(ss) // the head alone: it counts every card
		}
		opened, row := 0, 0
		if sc.c != colNeed {
			opened, row = m.stackNeeds(b, sc.c, w)
		}
		switch {
		case sc.h == 2 && len(ss) > 1, sc.h > 2 && row > sc.h-1: // one row: the chosen card, else the first, and the head counts the rest
			one := *b
			card := ss[0]
			if b.picked && b.open == sc.c {
				card = b.chosen
			}
			one.idx[sc.c] = []sample.Session{card}
			one.oneRow[sc.c] = true
			on, more = &one, len(ss)-1
		case sc.h > 1 && opened > sc.h-1: // the chosen card opened would push a card out uncounted: it is its row
			one := *b
			one.oneRow[sc.c] = true
			on = &one
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
	m.stackFoot(d, b, w, talk, drawer)
}

// stackNeeds are the rows the chosen card of section c needs, with the ↑
// and ↓ marks that count the cards above and under it: opened, and as its
// row. 0, 0: the section holds no chosen card.
func (m Model) stackNeeds(b *boardDraw, c boardCol, w int) (opened, row int) {
	ss := b.idx[c]
	sel := slices.IndexFunc(ss, func(s sample.Session) bool { return b.picked && b.open == c && paneOf(s) == paneOf(b.chosen) })
	if sel < 0 {
		return 0, 0
	}
	bl := m.boardBlockOf(b, c, ss[sel], w, false)
	opened, row = bl.n, 1 // the last card has no blank row under it
	if sel < len(ss)-1 {
		opened, row = len(bl.lines)+1, row+1
	}
	if sel > 0 {
		opened, row = opened+1, row+1
	}
	return opened, row
}

// stackFoot is what closes the stack: the room's talk lines, then the
// QUIET drawer.
func (m Model) stackFoot(d *boardDrawn, b *boardDraw, w int, talk []roomDrawn, drawer int) {
	for _, t := range talk {
		d.roomAt[len(d.lines)] = t.at
		d.addRow(-1, t.sel, w, m.st, t.text)
	}
	if drawer > 0 {
		m.boardDrawer(d, b, w)
	}
}

// stackNames is the stack in fewer rows than it has heads: one line names
// each section of stack with its count, as the column heads did, and a tap
// on a name hides it.
func (m Model) stackNames(d *boardDrawn, b *boardDraw, stack []boardCol, w, rows int) {
	if rows <= 0 {
		return
	}
	var parts []string
	x := 0
	for _, c := range stack {
		text := " " + c.name()
		if m.have {
			text += " · " + strconv.Itoa(len(b.idx[c]))
		}
		cw := lipgloss.Width(text + "  −")
		parts = append(parts, m.boardHead(c, len(b.idx[c]), 0, cw, c == b.open))
		d.heads = append(d.heads, span{min(x, w), min(x+cw, w), c.short()})
		x += cw + 1
	}
	d.headAt = len(d.lines)
	d.add(-1, fit(strings.Join(parts, " "), w))
	for range rows - 1 {
		d.add(-1, strings.Repeat(" ", w))
	}
	d.colsTo = len(d.lines)
}
