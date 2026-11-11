package app

import (
	"slices"
	"strconv"
	"strings"
	"time"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
)

// longPressFor is how long a press is held before it opens the menu.
const longPressFor = 480 * time.Millisecond

// menuItem is a line of the card's menu: its words, the key that does it
// with the menu open ("" none), and what it does.
type menuItem struct {
	label, key string
	run        func(m *Model) tea.Cmd
}

// boardMenu is the menu open on the card of pane: its lines, the chosen
// line, and the cell it opened at.
type boardMenu struct {
	pane     paneKey
	items    []menuItem
	at, x, y int
}

// longPress is a left press on a card that the terminal has not released
// yet: the menu opens when its tick comes with the same id.
type longPress struct {
	id, x, y int
	pane     paneKey
	on       bool
}

type longPressMsg struct{ id int }

// menuFor are the lines of s's menu (the spec's "The context menu"), each
// only when it applies. None of them destroys anything.
func (m Model) menuFor(s sample.Session) []menuItem {
	bk, mk := m.keys.board, m.keys.main
	items := []menuItem{{"copy last message", bk.Copy.Help().Key, func(m *Model) tea.Cmd { return m.boardCopy(s, true) }}}
	if q := questionText(s); q != "" && m.asksNow(s) {
		items = append(items, menuItem{"copy the question", "", func(m *Model) tea.Cmd {
			return m.copyText(q, "the question of "+s.Name)
		}})
	}
	if link, n, ok := m.shipLink(s); ok {
		number := "#" + strconv.Itoa(n)
		items = append(items, menuItem{"copy the link of " + number, "", func(m *Model) tea.Cmd {
			return m.copyText(link, "the link of "+number)
		}})
	}
	items = append(items, menuItem{"copy its name", "", func(m *Model) tea.Cmd { return m.copyText(s.Name, s.Name) }})
	if chain := m.chainLabelOf(s); chain != "" {
		items = append(items, menuItem{"show only the chain " + chain, bk.Chain.Help().Key, func(m *Model) tea.Cmd {
			return m.boardChainOf(s, true)
		}})
	}
	items = append(items, menuItem{"go in", mk.Into.Help().Key, func(m *Model) tea.Cmd {
		next, cmd := m.boardInto(s)
		if nm, ok := next.(Model); ok {
			*m = nm
		}
		return cmd
	}})
	c := m.columnOf(s)
	return append(items, menuItem{"hide " + strings.ToLower(c.name()), "", func(m *Model) tea.Cmd {
		m.flipHidden(c)
		return nil
	}})
}

// questionText is what s asks: each question, then its options, one a line.
func questionText(s sample.Session) string {
	if s.Ask == nil {
		return ""
	}
	var lines []string
	for _, q := range s.Ask.Questions {
		lines = append(lines, q.Question)
		for i, o := range q.Options {
			lines = append(lines, strconv.Itoa(i+1)+". "+o.Label)
		}
	}
	return strings.Join(lines, "\n")
}

// shipLink is the link of the pull request s drives, its leading ship, and
// its number; false when s drives none with a link.
func (m Model) shipLink(s sample.Session) (string, int, bool) {
	if ss := m.shipsOf(s); len(ss) > 0 && ss[0].URL != "" {
		return ss[0].URL, ss[0].Number, true
	}
	return "", 0, false
}

// openMenu opens s's menu at the cell x, y. It opens only on the board.
func (m Model) openMenu(s sample.Session, x, y int) Model {
	if m.boardShown() {
		m.menu, m.longPress.on = &boardMenu{pane: paneOf(s), items: m.menuFor(s), x: x, y: y}, false
		m.dragFrom, m.dragTo, m.tapLater = nil, nil, nil // a long press starts no drag, and is no tap
	}
	return m
}

// openMenuOnChosen is m: the menu of the chosen card, at its first line.
func (m Model) openMenuOnChosen(s sample.Session) Model {
	w := max(m.width, minWidth)
	d := m.boardPanel(w, m.bodyRows())
	x, y := 0, len(d.lines)
	for spot, k := range d.cards {
		if k != paneOf(s) || spot.line >= y {
			continue
		}
		x, y = 0, spot.line
		if i := slices.IndexFunc(d.heads, func(h span) bool { return h.press == spot.col.short() }); i >= 0 && m.layoutAt(w) != Narrow {
			x = d.heads[i].x0
		}
	}
	return m.openMenu(s, x+2, m.drawScreen(w).bodyTop+y+1)
}

// menuKey is k while the menu is open: it takes every key. ↑ ↓ move, ⏎
// or a line's own key does the line, esc closes.
func (m Model) menuKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	mk := m.keys.main
	mn := *m.menu
	switch {
	case bkey.Matches(k, mk.Up):
		mn.at = max(mn.at-1, 0)
	case bkey.Matches(k, mk.Down):
		mn.at = min(mn.at+1, len(mn.items)-1)
	case bkey.Matches(k, mk.Back):
		m.menu = nil
		return m, nil
	case bkey.Matches(k, mk.Open):
		return m.menuDo(mn.at)
	default:
		if i := slices.IndexFunc(mn.items, func(it menuItem) bool { return it.key != "" && it.key == k.String() }); i >= 0 {
			return m.menuDo(i)
		}
	}
	m.menu = &mn
	return m, nil
}

// menuDo closes the menu and does its line i.
func (m Model) menuDo(i int) (tea.Model, tea.Cmd) {
	it := m.menu.items[i]
	m.menu = nil
	cmd := it.run(&m)
	return m, cmd
}

// menuBox is the menu's box and the cell of its top left: at the pointer on
// a mid screen and a desk, clamped to the screen above the key line; on a
// phone the full width, rising from the key line.
func (m Model) menuBox(w, h int) (lines []string, x, y int) {
	st, mn := m.st, m.menu
	name := ""
	if s, ok := m.menuOn(); ok {
		name = s.Name
	}
	labelW, keyW := 0, 0
	for _, it := range mn.items {
		labelW, keyW = max(labelW, lipgloss.Width(it.label)), max(keyW, lipgloss.Width(it.key))
	}
	foot := []footPart{st.footKey(m.keys.main.Open, "do", "", st.text), st.footKey(m.keys.main.Back, "close", " ─ ", st.text)}
	bw := min(max(2+labelW+3+keyW+4, lipgloss.Width(name)+6, 22), w)
	phone := m.layoutAt(w) == Narrow
	if phone {
		bw = w
	}
	rows := make([]string, len(mn.items))
	for i, it := range mn.items {
		label := st.text
		if i == mn.at {
			label = st.text.Bold(true)
		}
		rows[i] = spread(m.chosenMark(i == mn.at)+" "+label.Render(it.label), st.muted.Render(it.key), bw-4)
	}
	lines, _ = boxFrame(st.hair, st.text.Bold(true).Render(name), rows, foot, bw)
	bottom := h - 2 // the key line's hairline
	x, y = min(max(mn.x, 0), w-bw), min(max(mn.y, 1), bottom-len(lines))
	if phone {
		x, y = 0, bottom-len(lines)
	}
	return lines, x, max(y, 1)
}

// menuOn is the match the menu is open on; false with no menu, off the
// board, or when the match is gone.
func (m Model) menuOn() (sample.Session, bool) {
	if m.menu == nil || !m.boardShown() {
		return sample.Session{}, false
	}
	i := slices.IndexFunc(m.snap.Sessions, func(s sample.Session) bool { return paneOf(s) == m.menu.pane })
	if i < 0 {
		return sample.Session{}, false
	}
	return m.snap.Sessions[i], true
}

// overlayMenu lays the menu's box over the board, the board left as it is
// around it.
func (m Model) overlayMenu(sc *drawn, w int) {
	box, x, y := m.menuBox(w, len(sc.lines))
	for j, b := range box {
		r := y + j
		if r >= len(sc.lines) {
			break
		}
		l := sc.lines[r]
		left := ansi.Truncate(l, x, "")
		left += strings.Repeat(" ", max(x-lipgloss.Width(left), 0))
		sc.lines[r] = left + b + ansi.TruncateLeft(l, x+lipgloss.Width(b), "")
	}
}

// menuTap is a tap while the menu is open: on a line it does the line,
// elsewhere on the box nothing, and outside the box it closes the menu.
func (m Model) menuTap(ms tea.Mouse) (tea.Model, tea.Cmd) {
	w := max(m.width, minWidth)
	box, x, y := m.menuBox(w, len(m.drawScreen(w).lines))
	if ms.X < x || ms.X >= x+lipgloss.Width(box[0]) || ms.Y < y || ms.Y >= y+len(box) {
		m.menu = nil
		return m, nil
	}
	if i := ms.Y - y - 1; ms.Button == tea.MouseLeft && i >= 0 && i < len(m.menu.items) {
		return m.menuDo(i)
	}
	return m, nil
}

// boardRightClick is a right-click on the board: on a card it chooses the
// card and opens its menu at the pointer.
func (m Model) boardRightClick(ms tea.Mouse) (tea.Model, tea.Cmd) {
	if !m.peekReach() || m.splashing() {
		return m, nil
	}
	c, s, ok := m.cardAtCell(ms.X, ms.Y)
	if !ok {
		return m, nil
	}
	m.boardChooseCard(c, s)
	m.boardMine = paneOf(s)
	return m.openMenu(s, ms.X, ms.Y), nil
}

// boardCardOf is the card of pane k on the board, and its column.
func (m Model) boardCardOf(k paneKey) (boardCol, sample.Session, bool) {
	b, _ := m.boardDrawOf()
	for _, c := range boardCols {
		if i := slices.IndexFunc(b.idx[c], func(s sample.Session) bool { return paneOf(s) == k }); i >= 0 {
			return c, b.idx[c][i], true
		}
	}
	return 0, sample.Session{}, false
}

// cardAtCell is the card at the frame's cell x, y, and its column.
func (m Model) cardAtCell(x, y int) (boardCol, sample.Session, bool) {
	w := max(m.width, minWidth)
	sc := m.drawScreen(w)
	if y < sc.bodyTop || y >= sc.keysAt-1 {
		return 0, sample.Session{}, false
	}
	b, _ := m.boardDrawOf()
	return m.cardAt(m.boardPanel(w, m.bodyRows()), b, x, y-sc.bodyTop)
}

// holdPress arms the long press of a left press at ms on a card, after the
// tap has acted on it: only on a terminal that reports releases, and only
// while the board takes keys.
func (m Model) holdPress(next tea.Model, cmd tea.Cmd, ms tea.Mouse) (tea.Model, tea.Cmd) {
	nm, ok := next.(Model)
	if !ok || !nm.sawRelease || !nm.peekReach() {
		return next, cmd
	}
	_, s, on := m.cardAtCell(ms.X, ms.Y)
	if !on {
		return next, cmd
	}
	id := nm.longPress.id + 1
	nm.longPress = longPress{id: id, x: ms.X, y: ms.Y, pane: paneOf(s), on: true}
	tick := tea.Tick(longPressFor, func(time.Time) tea.Msg { return longPressMsg{id} })
	return nm, tea.Batch(cmd, tick)
}

// pointer is a release, a drag and the long press's tick. A release says
// the terminal reports them, ends the press and copies a drag; a move of
// more than one cell is a drag, and no long press.
func (m Model) pointer(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseReleaseMsg:
		ms := msg.Mouse()
		// Only a left release: a phone's long press may send a right
		// press and release while its taps send none.
		m.sawRelease, m.longPress.on = m.sawRelease || ms.Button == tea.MouseLeft, false
		return m.dragEnd(point{ms.X, ms.Y})
	case tea.MouseMotionMsg:
		ms := msg.Mouse()
		if farFrom(point{m.longPress.x, m.longPress.y}, point{ms.X, ms.Y}) {
			m.longPress.on = false
		}
		return m.dragOn(point{ms.X, ms.Y}), nil
	case longPressMsg:
		c, s, ok := m.boardCardOf(m.longPress.pane)
		if !m.longPress.on || msg.id != m.longPress.id || !ok || !m.peekReach() {
			return m, nil
		}
		m.boardChooseCard(c, s) // a press that waits for its release has not chosen it yet
		m.boardMine = paneOf(s)
		return m.openMenu(s, m.longPress.x, m.longPress.y), nil
	}
	return m, nil
}

func abs(n int) int { return max(n, -n) }
