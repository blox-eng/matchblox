package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
)

// liveFrom is the narrowest board with the chosen match's live view at its
// right: the four columns at their base widths, the gaps, and viewLeast.
const liveFrom = 152

// boardBase is each column's width: of 80 on Medium, where QUIET is the
// drawer, and of 104 on Wide.
var boardBase = map[Layout][4]int{Medium: {38, 20, 20, 0}, Wide: {36, 25, 25, 18}}

// boardWidths is the width of each shown column at w, with one cell
// between two, and the live view's at the right (0 for none). A hidden
// column gives its room to the others in their proportions; on Wide from
// liveFrom the columns keep their base widths and the live view takes it.
func boardWidths(l Layout, w int, shown []boardCol) (cols []int, live int) {
	if l == Narrow {
		return []int{w}, 0
	}
	sum := 0
	for _, c := range shown {
		if x := boardBase[l][c]; x > 0 {
			cols, sum = append(cols, x), sum+x
		}
	}
	if l == Wide && w >= liveFrom {
		return cols, w - sum - len(cols)
	}
	if len(cols) == 0 {
		return nil, 0
	}
	room := w - (len(cols) - 1)
	left := room
	for i := range cols {
		cols[i] = cols[i] * room / sum
		left -= cols[i]
	}
	cols[0] += left
	return cols, 0
}

// boardDrawn is the board's body and where its parts are, so a tap finds
// what it hit: the line of the column heads (or of the page marks) and a
// span on it for each column, which presses its short name; the card on
// each line of each column; the tap targets of a line, in the board's
// columns.
type boardDrawn struct {
	body
	headAt int
	heads  []span
	cards  map[boardSpot]paneKey
	taps   map[int][]span
}

// boardSpot is a line of one column.
type boardSpot struct {
	line int
	col  boardCol
}

// boardDraw is what one draw of the board works out once: the columns of
// every match, the open column and its chosen card.
type boardDraw struct {
	idx    [4][]sample.Session
	open   boardCol
	chosen sample.Session
	picked bool
	live   int
}

// empty: no column has a card.
func (b boardDraw) empty() bool {
	return !slices.ContainsFunc(b.idx[:], func(ss []sample.Session) bool { return len(ss) > 0 })
}

// boardShown: the board is drawn in Sessions' place.
func (m Model) boardShown() bool { return m.board && m.tab == tabSessions }

// boardPanel is the board in w by h (the spec's "Layout"): the setup doors
// above, the columns, the ghost row and the recommendations under them,
// each folded to one line.
func (m Model) boardPanel(w, h int) boardDrawn {
	d := boardDrawn{headAt: -1, cards: map[boardSpot]paneKey{}, taps: map[int][]span{}}
	b := boardDraw{idx: m.boardIndex()}
	if m.filter != "" {
		found := map[paneKey]bool{}
		for _, s := range m.snap.Sessions {
			found[paneOf(s)] = true
		}
		for c := range b.idx {
			b.idx[c] = slices.DeleteFunc(b.idx[c], func(s sample.Session) bool { return !found[paneOf(s)] })
		}
	}
	shown := slices.DeleteFunc(slices.Clone(boardCols[:]), m.colHidden)
	m.boardChoose(&b, shown)

	st := m.st
	if len(m.doors) > 0 {
		d.add(-1, st.label.Render(fit(fmt.Sprintf(" SET UP · %d", len(m.doors)), w)))
	}
	none := m.have && m.filter == "" && b.empty()
	var under []string
	if !none {
		under = append(under, m.ghostRow(m.ghosts()[0], false, w))
	}
	if len(m.recs) > 0 {
		under = append(under, st.label.Render(fit(fmt.Sprintf(" RECOMMENDATIONS · %d", len(m.recs)), w)))
	}
	rows := max(h-len(d.lines)-len(under), 0)
	top := len(d.lines)
	switch l := m.layoutAt(w); {
	case none:
		// No session at all: no columns, the ghost row leads with the cursor.
		for i, g := range m.ghosts() {
			d.addRow(-1, i == 0, w, st, m.ghostRow(g, i == 0, w))
		}
	case len(shown) == 0:
		d.add(-1, "", fit("  "+st.faint.Render("Every column is hidden. Tap a + name to show it."), w))
	case l == Narrow:
		m.boardPage(&d, &b, shown, w, rows)
	default:
		m.boardColumns(&d, &b, l, shown, w, rows)
	}
	for len(d.lines) < top+rows {
		d.add(-1, strings.Repeat(" ", w))
	}
	d.lines, d.rows = d.lines[:top+rows], d.rows[:top+rows]
	d.add(-1, under...)
	return d
}

// boardChoose finds the open column and its chosen card: the one the
// person chose, else the first card of the first shown column that has one.
func (m Model) boardChoose(b *boardDraw, shown []boardCol) {
	b.open = m.boardAt
	if !slices.Contains(shown, b.open) && len(shown) > 0 {
		b.open = shown[0]
	}
	if len(b.idx[b.open]) == 0 {
		if i := slices.IndexFunc(shown, func(c boardCol) bool { return len(b.idx[c]) > 0 }); i >= 0 {
			b.open = shown[i]
		}
	}
	ss := b.idx[b.open]
	if len(ss) == 0 {
		return
	}
	i := 0
	if k, ok := m.boardPick[b.open]; ok {
		i = max(slices.IndexFunc(ss, func(s sample.Session) bool { return paneOf(s) == k }), 0)
	}
	b.chosen, b.picked = ss[i], true
}

// boardWords is the line the body says in place of the cards: before the
// first state, and for a search that matches nothing.
func (m Model) boardWords(b *boardDraw) string {
	switch {
	case !m.have:
		return "reading your matches…"
	case m.filter != "" && b.empty():
		return `nothing matches "` + m.filter + `" · ` + m.keys.main.Back.Help().Key + " clears"
	}
	return ""
}

// boardStateNotice is what the notice row says of the board's state: the
// service lost, or nothing.
func (m Model) boardStateNotice() string {
	if m.lost && m.have {
		return "no service · it redials"
	}
	return ""
}

// boardPage is the board on a phone: the page marks, a hairline, and the
// open column.
func (m Model) boardPage(d *boardDrawn, b *boardDraw, shown []boardCol, w, rows int) {
	st := m.st
	line, x := " ", 1
	mark := func(text string, ink lipgloss.Style, c boardCol) {
		if x > 1 {
			line, x = line+"  ", x+2
		}
		d.heads = append(d.heads, span{x, min(x+lipgloss.Width(text), w), c.short()})
		line, x = line+ink.Render(text), x+lipgloss.Width(text)
	}
	count := func(c boardCol) string {
		if !m.have {
			return "" // before the first state, never a zero
		}
		return " " + strconv.Itoa(len(b.idx[c]))
	}
	for _, c := range shown {
		if c == b.open {
			mark(c.name()+count(c)+" −", st.text.Bold(true), c)
		} else {
			mark(c.short()+count(c), st.faint, c)
		}
	}
	for _, c := range boardCols {
		if !slices.Contains(shown, c) {
			mark("+ "+c.short()+count(c), st.faint, c)
		}
	}
	d.headAt = len(d.lines)
	d.add(-1, fit(line, w), st.hair.Render(strings.Repeat("─", w)))
	m.boardStack(d, b, b.open, len(d.lines), 0, w, rows-2, false)
	if words := m.boardWords(b); words != "" && rows > 2 {
		d.lines[d.headAt+2] = fit("  "+st.faint.Render(words), w)
	}
}

// boardColumns is the board on a mid screen and a desk: the heads, the
// columns side by side, from liveFrom the live view at the right, and on
// Medium the QUIET drawer under them.
func (m Model) boardColumns(d *boardDrawn, b *boardDraw, l Layout, shown []boardCol, w, rows int) {
	st := m.st
	var drawn []boardCol
	for _, c := range shown {
		if l == Wide || c != colQuiet {
			drawn = append(drawn, c)
		}
	}
	widths, live := boardWidths(l, w, shown)
	b.live = live
	drawer := 0
	if l == Medium && slices.Contains(shown, colQuiet) {
		drawer = 2
		if b.open == colQuiet {
			drawer = 2 + quietRows
		}
	}
	colH := max(rows-1-drawer, 0)

	var heads []string
	x := 0
	for i, c := range drawn {
		heads = append(heads, m.boardHead(c, len(b.idx[c]), widths[i], c == b.open))
		d.heads = append(d.heads, span{x, x + widths[i], c.short()})
		x += widths[i] + 1
	}
	d.headAt = len(d.lines)
	d.add(-1, strings.Join(heads, " "))
	first := len(d.lines)
	x = 0
	for i, c := range drawn {
		m.boardStack(d, b, c, first, x, widths[i], colH, l == Wide && c == colQuiet)
		x += widths[i] + 1
	}
	if words := m.boardWords(b); words != "" && colH > 0 {
		d.lines[first] = fit("  "+st.faint.Render(words), x-1)
	}
	if live > 0 {
		var view []string
		if b.picked {
			v := m
			v.chose, v.selPID, v.selHost = true, b.chosen.PID, b.chosen.Host
			view = v.viewPane(b.chosen, live, colH+1)
		}
		for i := d.headAt; i < first+colH; i++ {
			right := ""
			if j := i - d.headAt; j < len(view) {
				right = view[j]
			}
			d.lines[i] = fit(d.lines[i], w-live-1) + st.hair.Render("│") + fit(right, live)
		}
	}
	if drawer > 0 {
		m.boardDrawer(d, b, w)
	}
}

// quietRows is how many rows the QUIET drawer opens to.
const quietRows = 6

// boardDrawer is QUIET on Medium, under the columns: one line with the
// names, which opens to quietRows rows when the selection is in it.
func (m Model) boardDrawer(d *boardDrawn, b *boardDraw, w int) {
	st := m.st
	ss := b.idx[colQuiet]
	d.add(-1, st.hair.Render(strings.Repeat("─", w)))
	if b.open == colQuiet {
		d.add(-1, m.boardHead(colQuiet, len(ss), w, true))
		m.boardStack(d, b, colQuiet, len(d.lines), 0, w, quietRows, true)
		return
	}
	line := " " + colQuiet.name()
	if m.have {
		line += " · " + strconv.Itoa(len(ss))
	}
	if len(ss) > 0 {
		names := make([]string, len(ss))
		for i, s := range ss {
			names[i] = s.Name
		}
		line += "   " + strings.Join(names, " · ")
	}
	d.taps[len(d.lines)] = []span{{0, w, colQuiet.short()}}
	d.add(-1, spread(st.faint.Render(line+"   ▴"), st.faint.Render("−")+"  ", w))
}

// boardHead is a column's head: its name, its count once the first state
// came, and the mark that hides it. The open column's name is fg bold.
func (m Model) boardHead(c boardCol, n, w int, open bool) string {
	name := m.st.faint
	if open {
		name = m.st.text.Bold(true)
	}
	count := ""
	if m.have {
		count = " · " + strconv.Itoa(n)
	}
	return fit(" "+name.Render(c.name())+m.st.faint.Render(count+"  −"), w)
}

// boardBlock is one card in its column, and the blank line under it.
type boardBlock struct {
	key   paneKey
	lines []string
	taps  [][]span
	n     int // the card's own lines
}

// boardStack draws column c's cards into h lines of d from line y, at x
// and w cells wide: in its order, scrolled so the chosen card is whole,
// with how many cards are above and under the column.
func (m Model) boardStack(d *boardDrawn, b *boardDraw, c boardCol, y, x, w, h int, quietRow bool) {
	st := m.st
	ss := b.idx[c]
	h = max(h, 0)
	for len(d.lines) < y+h {
		d.add(-1, "")
	}
	col := make([]string, h)
	defer func() {
		for i, l := range col {
			if x > 0 {
				l = fit(d.lines[y+i], x-1) + " " + l
			}
			d.lines[y+i] = l
		}
	}()
	set := func(i int, l string, key *paneKey, taps []span) {
		if i >= h {
			return
		}
		col[i] = fit(l, w)
		if key != nil {
			d.cards[boardSpot{y + i, c}] = *key
		}
		if len(taps) > 0 {
			d.taps[y+i] = append(d.taps[y+i], shift(taps, x)...)
		}
	}
	for i := range h {
		set(i, "", nil, nil)
	}
	if len(ss) == 0 {
		if c == colNeed && m.have && m.filter == "" {
			set(1, "  "+st.faint.Render("nothing needs you"), nil, nil)
		}
		return
	}
	blocks := make([]boardBlock, len(ss))
	sel := -1
	for i, s := range ss {
		blocks[i] = m.boardBlockOf(b, c, s, w, quietRow)
		if b.picked && c == b.open && paneOf(s) == paneOf(b.chosen) {
			sel = i
		}
	}
	if last := &blocks[len(blocks)-1]; len(last.lines) > last.n { // a blank row is between two cards only
		last.lines, last.taps = last.lines[:last.n], last.taps[:last.n]
	}
	tall := func(a, z int) int {
		n := 0
		for _, bl := range blocks[a : z+1] {
			n += len(bl.lines)
		}
		if a > 0 {
			n++
		}
		if z < len(blocks)-1 {
			n++
		}
		return n
	}
	start := 0
	for start < sel && tall(start, sel) > h {
		start++
	}
	at := 0
	if start > 0 {
		set(at, "  "+st.faint.Render("↑ "+strconv.Itoa(start)+" more"), nil, nil)
		at++
	}
	i := start
	for ; i < len(blocks); i++ {
		bl := blocks[i]
		reserve := 0
		if i < len(blocks)-1 {
			reserve = 1
		}
		if at+len(bl.lines)+reserve > h && !(i == sel && i == start) {
			break
		}
		for j, l := range bl.lines {
			var key *paneKey
			if j < bl.n {
				key = &bl.key
			}
			set(at, l, key, bl.taps[j])
			at++
		}
	}
	if i < len(blocks) {
		set(at, "  "+st.faint.Render("↓ "+strconv.Itoa(len(blocks)-i)+" more"), nil, nil)
	}
}

// boardBlockOf is the card of s in column c, dimmed while the service is
// lost or its host is not reached, with a blank line under a whole card
// (boardStack drops it under the last).
func (m Model) boardBlockOf(b *boardDraw, c boardCol, s sample.Session, w int, quietRow bool) boardBlock {
	o := cardOpts{
		sel:       b.picked && c == b.open && paneOf(s) == paneOf(b.chosen),
		oneLine:   m.oneLine(),
		need:      c == colNeed,
		quietRow:  quietRow,
		otherHost: m.hostCount() > 1 && s.Host != m.opt.Host,
		bare:      b.live > 0,
	}
	if k, ok := m.chainKeyOf(s); ok && b.picked {
		if ck, ok := m.chainKeyOf(b.chosen); ok && ck == k {
			o.lit = true
		}
	}
	lines, taps := m.boardCard(s, w, o)
	if m.lost && m.have || m.stale(s.Host) {
		for i, l := range lines {
			lines[i] = m.st.faint.Render(ansi.Strip(l))
		}
		lines = m.washed(lines, w, o)
	}
	bl := boardBlock{key: paneOf(s), lines: lines, taps: taps, n: len(lines)}
	if !quietRow && (o.sel || !o.oneLine) {
		bl.lines, bl.taps = append(bl.lines, ""), append(bl.taps, nil)
	}
	return bl
}
