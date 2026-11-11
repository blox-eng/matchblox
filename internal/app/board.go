package app

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

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
// columns; the host of each host line, which a tap dials again.
type boardDrawn struct {
	body
	headAt int
	heads  []span
	cards  map[boardSpot]paneKey
	taps   map[int][]span
	hostAt map[int]string
	// tabRow and tabSpans are the tab row while the board shows.
	tabRow   string
	tabSpans []span
	// stripAt is the strip's first line (-1: none), and strip its tap.
	stripAt int
	strip   span
	// doorsAt is the doors line (-1: none).
	doorsAt int
	// colsTo is the line under the columns side by side, and live the
	// width of the live view at their right (0: none).
	colsTo, live int
}

// boardSpot is a line of one column.
type boardSpot struct {
	line int
	col  boardCol
}

// boardDraw is what one draw of the board works out once: the columns of
// the matches the filters show, the open column and its chosen card.
// none: no match at all, whatever the filters.
type boardDraw struct {
	idx    [4][]sample.Session
	open   boardCol
	chosen sample.Session
	picked bool
	live   int
	none   bool
}

// empty: no column has a card.
func (b boardDraw) empty() bool {
	return !slices.ContainsFunc(b.idx[:], func(ss []sample.Session) bool { return len(ss) > 0 })
}

// boardShown: the board is drawn in Sessions' place.
func (m Model) boardShown() bool { return m.board && m.tab == tabSessions }

// boardPanel is the board in w by h (the spec's "Layout"): the lines of
// the hosts that are not reached and the setup doors above, the columns,
// the ghost row and the recommendations under them, each folded to one
// line. It draws the tab row too, from the same index.
func (m Model) boardPanel(w, h int) boardDrawn {
	d := boardDrawn{headAt: -1, stripAt: -1, doorsAt: -1, cards: map[boardSpot]paneKey{}, taps: map[int][]span{}, hostAt: map[int]string{}}
	b, shown := m.boardDrawOf()
	d.tabRow, d.tabSpans = m.boardTabRow(w, &b)

	st := m.st
	chosen, above := m.aboveChosen()
	for _, h := range m.unreached() {
		sel := above && chosen == h
		for _, l := range m.hostLine(h, w, sel) {
			d.hostAt[len(d.lines)] = h
			d.addRow(-1, sel, w, st, l)
		}
	}
	if ds := m.boardDoors(); len(ds) > 0 {
		sel, i := above && chosen == "", m.doorIndex(ds)
		count := fmt.Sprintf("SET UP · %d", len(ds))
		if len(ds) > 1 {
			count = fmt.Sprintf("SET UP · %d of %d", i+1, len(ds))
		}
		d.doorsAt = len(d.lines)
		d.addRow(-1, sel, w, st, m.chosenMark(sel)+st.label.Render(count)+st.muted.Render(" · "+ds[i].Title))
	}
	none := m.have && b.none
	var under []string
	if !none {
		under = append(under, m.ghostRow(m.ghosts()[0], false, w))
	}
	if len(m.recs) > 0 {
		under = append(under, st.label.Render(fit(fmt.Sprintf(" RECOMMENDATIONS · %d", len(m.recs)), w)))
	}
	strip, tap, _ := m.strip(w)
	rows := m.boardRowsUnder(w, h, none, len(strip))
	top := len(d.lines)
	switch l := m.layoutAt(w); {
	case none:
		// No session at all: no columns, the ghost row leads with the cursor.
		for i, g := range m.ghosts() {
			d.addRow(-1, i == 0, w, st, m.ghostRow(g, i == 0, w))
		}
	case len(shown) == 0 && l != Narrow: // the + names wait on the tab row
		d.add(-1, strings.Repeat(" ", w))
		d.add(-1, m.allHidden(w)...)
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
	if len(strip) > 0 {
		d.stripAt, d.strip = len(d.lines), tap
	}
	d.add(-1, strip...)
	return d
}

// boardRows is how many of the board's h rows the columns take: all but the
// doors line above, the ghost row and the recommendations under them, and
// the strip's row. none: no session at all, and no ghost row under.
func (m Model) boardRows(w, h int, none bool) int {
	strip, _, _ := m.strip(w)
	return m.boardRowsUnder(w, h, none, len(strip))
}

// boardRowsUnder is boardRows with the strip's lines worked out.
func (m Model) boardRowsUnder(w, h int, none bool, strip int) int {
	rows := h - strip - len(m.hostLines(w))
	if len(m.boardDoors()) > 0 {
		rows--
	}
	if !none {
		rows--
	}
	if len(m.recs) > 0 {
		rows--
	}
	return max(rows, 0)
}

// boardDrawOf is what a draw of the board works out first: every match in
// its column, narrowed by the filters, the columns shown, and the open
// column with its chosen card.
func (m Model) boardDrawOf() (boardDraw, []boardCol) {
	b := boardDraw{idx: m.boardIndex()}
	b.none = b.empty()
	for c := range b.idx {
		b.idx[c] = slices.DeleteFunc(b.idx[c], func(s sample.Session) bool { return !m.inBoard(s) })
	}
	shown := slices.DeleteFunc(slices.Clone(boardCols[:]), m.colHidden)
	m.boardChoose(&b, shown)
	return b, shown
}

// boardChoose finds the open column and its chosen card: the one the
// person chose, else the first card of the first shown column that has one.
func (m Model) boardChoose(b *boardDraw, shown []boardCol) {
	b.open = m.boardAt
	if len(shown) == 0 {
		return // every column is hidden: no card is chosen
	}
	if !slices.Contains(shown, b.open) {
		b.open = shown[0]
	}
	if len(b.idx[b.open]) == 0 {
		if i := slices.IndexFunc(shown, func(c boardCol) bool { return len(b.idx[c]) > 0 }); i >= 0 {
			b.open = shown[i]
		}
	}
	ss := b.idx[b.open]
	if _, above := m.aboveChosen(); above || len(ss) == 0 {
		return // a line above the columns is chosen in place of a card
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

// boardStateNotice is what the strip's row says of the board's state: the
// service lost, data older than the stale limit, or nothing.
func (m Model) boardStateNotice() string {
	switch age := m.now().Sub(m.snap.At); {
	case !m.have:
	case m.lost:
		return "no service · it redials"
	case age >= staleAfter && age < 2*time.Minute:
		return fmt.Sprintf("data is %d s old · the service is slow", int(age.Seconds()))
	case age >= staleAfter:
		return fmt.Sprintf("data is %d min old · the service is slow", int(age.Minutes()))
	}
	return ""
}

// boardPage is the board on a phone: the page marks, a hairline, and the
// open column.
func (m Model) boardPage(d *boardDrawn, b *boardDraw, shown []boardCol, w, rows int) {
	st := m.st
	type mark struct {
		text string
		ink  lipgloss.Style
		c    boardCol
	}
	var marks []mark
	count := func(c boardCol) string {
		if !m.have {
			return "" // before the first state, never a zero
		}
		return " " + strconv.Itoa(len(b.idx[c]))
	}
	for _, c := range shown {
		if c == b.open {
			marks = append(marks, mark{c.name() + count(c) + " −", st.text.Bold(true), c})
		} else {
			marks = append(marks, mark{c.short() + count(c), st.faint, c})
		}
	}
	for _, c := range boardCols {
		if !slices.Contains(shown, c) {
			marks = append(marks, mark{"+ " + c.short() + count(c), st.faint, c})
		}
	}
	gap, need := "  ", 1 // the marks close up to one cell apart when two do not fit
	for i, mk := range marks {
		need += lipgloss.Width(mk.text) + min(i, 1)*2
	}
	if need > w {
		gap = " "
	}
	line, x := " ", 1
	for i, mk := range marks {
		if i > 0 {
			line, x = line+gap, x+len(gap)
		}
		d.heads = append(d.heads, span{x, min(x+lipgloss.Width(mk.text), w), mk.c.short()})
		line, x = line+mk.ink.Render(mk.text), x+lipgloss.Width(mk.text)
	}
	d.headAt = len(d.lines)
	d.add(-1, fit(line, w), st.hair.Render(strings.Repeat("─", w)))
	if len(shown) == 0 {
		d.add(-1, m.allHidden(w)...)
		return
	}
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
	b.live, d.live = live, live
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
	d.colsTo = first + colH
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
	name := " " + colQuiet.name()
	if m.have {
		name += " · " + strconv.Itoa(len(ss))
	}
	if b.open == colQuiet {
		d.taps[len(d.lines)] = []span{{0, lipgloss.Width(name) + 3, colQuiet.short()}} // with its −
		d.add(-1, m.boardHead(colQuiet, len(ss), w, true))
		m.boardStack(d, b, colQuiet, len(d.lines), 0, w, quietRows, true)
		return
	}
	line := name
	if len(ss) > 0 {
		names := make([]string, len(ss))
		for i, s := range ss {
			names[i] = s.Name
		}
		line += "   " + strings.Join(names, " · ")
	}
	d.taps[len(d.lines)] = []span{{0, lipgloss.Width(name), colQuiet.short()}, {w - 3, w, colQuiet.short()}} // the name and the −
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
		if c == colNeed && m.have && m.filter == "" && m.boardChain == "" && m.boardHost == "" {
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
		if at+len(bl.lines)+reserve > h && (i != sel || i != start) {
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
		oneLine:   m.oneLineIn(c),
		need:      c == colNeed,
		quietRow:  quietRow,
		otherHost: m.hostCount() > 1 && m.boardHost == "" && s.Host != m.opt.Host,
		bare:      b.live > 0,
	}
	if armed, sent := m.peekMarks(s); o.sel {
		o.armed, o.sent = armed, sent
	} else {
		o.sent = sent
	}
	switch {
	case m.landed(paneOf(s), c) && c == colWork:
		o.land = landFire
	case m.landed(paneOf(s), c):
		o.land = landWash
		if m.changes[paneOf(s)].kind == changeStrike {
			o.match = m.stillCell(s) // fire lands only in WORKING
		}
	case c == colQuiet:
		o.match = m.quietCell(s)
	}
	if s.Host != m.opt.Host && m.stale(s.Host) { // a host that is not reached
		o.asOf = m.stateAt(s.Host).Format("15:04")
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

// allHidden is what the body says when every column is hidden: the + names
// are on the tab row, and on a phone in the page marks.
func (m Model) allHidden(w int) []string {
	lines := wrapPlain("Every column is hidden. Tap a + name to show it.", w-3)
	for i, l := range lines {
		lines[i] = fit("  "+m.st.faint.Render(l), w)
	}
	return lines
}

// unreached are the hosts besides this one that are not reached: what
// shows of each is its last state.
func (m Model) unreached() []string {
	var out []string
	for _, h := range m.hosts()[1:] {
		if m.stale(h) {
			out = append(out, h)
		}
	}
	return out
}

// hostLines are the board's top lines, one for each host that is not
// reached.
func (m Model) hostLines(w int) []string {
	var out []string
	for _, h := range m.unreached() {
		out = append(out, m.hostLine(h, w, false)...)
	}
	return out
}

// hostLine says since when host is not reached, and the key that dials it
// again: one line, two on a phone when it does not fit.
func (m Model) hostLine(host string, w int, sel bool) []string {
	st := m.st
	since := m.nameOf(host) + ": not reached since " + m.stateAt(host).Format("15:04")
	again := st.text.Render(m.keys.main.Open.Help().Key) + st.muted.Render(" try again")
	lead := m.chosenMark(sel)
	if line := lead + st.muted.Render(since+" · ") + again; lipgloss.Width(line) <= w {
		return []string{fit(line, w)}
	}
	return []string{fit(lead+st.muted.Render(since), w), fit("   "+again, w)}
}

// chosenMark is the first cell of a line: ▌ in the accent when it is
// chosen.
func (m Model) chosenMark(sel bool) string {
	if sel {
		return m.st.accent.Render("▌")
	}
	return " "
}

// aboveLines are the lines above the columns that ↑ chooses, top to
// bottom: each host that is not reached, then the setup doors ("").
func (m Model) aboveLines() []string {
	lines := m.unreached()
	if len(m.boardDoors()) > 0 {
		lines = append(lines, "")
	}
	return lines
}

// aboveChosen is the line above the columns that is chosen: a host, or ""
// for the setup doors. false: a card is chosen.
func (m Model) aboveChosen() (string, bool) {
	lines := m.aboveLines()
	if m.boardAbove <= 0 || m.boardAbove > len(lines) {
		return "", false
	}
	return lines[len(lines)-m.boardAbove], true
}

// boardHostWord is the header's host filter while the board shows.
func (m Model) boardHostWord() string {
	return cmp.Or(m.boardHost, "all hosts") + " ▾"
}

// boardHostHit is where the header head has the host filter: a tap there
// does what s does.
func (m Model) boardHostHit(head string) span {
	if !m.boardShown() || m.hostCount() == 1 {
		return span{}
	}
	lead, word := " ▰ matchblox · ", m.boardHostWord()
	if !strings.HasPrefix(ansi.Strip(head), lead+word) {
		return span{} // short of room the header drops the host
	}
	x := ansi.StringWidth(lead)
	return span{x, x + ansi.StringWidth(word), m.keys.board.Host.Keys()[0]}
}

// boardCrumb is what the tab row of a screen w wide adds while the board
// shows: the chain the board is narrowed to and the key that shows all,
// then each hidden column with its count (on a phone they are in the page
// marks). A part that does not fit in w-2 leaves whole, with every one
// after it. The spans press esc, or a hidden column's short name.
func (m Model) boardCrumb(w int, b *boardDraw) (string, []span) {
	phone := m.layoutAt(w) == Narrow
	if m.boardChain == "" && (phone || !slices.ContainsFunc(boardCols[:], m.colHidden)) {
		return "", nil
	}
	st := m.st
	type part struct{ text, press, gap string }
	var parts []part
	if m.boardChain != "" {
		n := 0
		for _, ss := range b.idx {
			n += len(ss)
		}
		back := m.keys.main.Back.Help().Key
		parts = append(parts, part{st.text.Render("chain " + m.boardChain + " · " + strconv.Itoa(n)), "", ""},
			part{st.text.Render(back) + st.muted.Render(" all"), back, "   "})
	}
	if !phone {
		gap := "   "
		for _, c := range boardCols {
			if m.colHidden(c) {
				parts = append(parts, part{st.faint.Render("+ " + c.short() + " " + strconv.Itoa(len(b.idx[c]))), c.short(), gap})
				gap = "  "
			}
		}
	}
	line, x := "", 0
	var spans []span
	for i, p := range parts {
		if i == 0 {
			p.gap = ""
		}
		if x+len(p.gap)+lipgloss.Width(p.text) > w-2 {
			break
		}
		x += len(p.gap)
		if p.press != "" {
			spans = append(spans, span{x, x + lipgloss.Width(p.text), p.press})
		}
		line, x = line+p.gap+p.text, x+lipgloss.Width(p.text)
	}
	return line, spans
}

// boardTabRow is the tab row while the board shows: the tabs, and at its
// right the crumb. Short of room the tabs shorten, and then go.
func (m Model) boardTabRow(w int, b *boardDraw) (string, []span) {
	crumb, cs := m.boardCrumb(w, b)
	if crumb == "" {
		return m.tabsLine(w)
	}
	room := w - lipgloss.Width(crumb) - 1
	tabs, spans := m.tabsLine(room)
	if len(spans) == 0 || spans[len(spans)-1].x1 > room-2 {
		tabs, spans, room = " ", nil, 1
	}
	return fit(tabs+crumb, w), append(spans, shift(cs, room)...)
}
