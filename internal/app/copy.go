package app

import (
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/sample"
)

// The words of a copy (the spec's "Copy").
const (
	tmuxDrops     = "not copied: tmux drops it · tmux set -s set-clipboard on"
	notChecked    = " · not checked here"
	pasteHere     = "Now paste here."
	pasteWrong    = `That is not the code. A hop drops the copy: see docs/console.md, "Copy".`
	pasteChecked  = "Copy reaches this screen."
	copyCodeChars = "abcdefghijklmnopqrstuvwxyz"
)

// point is a cell of the frame: its column and its line.
type point struct{ x, y int }

// farFrom: b is more than one cell from a. Such a move is a drag, and
// ends a long press; a move of one cell is neither.
func farFrom(a, b point) bool { return max(abs(a.x-b.x), abs(a.y-b.y)) > 1 }

// clip puts text on the clipboard. Without a Clip, the program writes OSC 52
// with its output, between frames: it works over SSH and through tmux.
func (m Model) clip(text string) tea.Cmd {
	if m.opt.Clip != nil {
		m.opt.Clip(text)
		return nil
	}
	return tea.SetClipboard(text)
}

// tmuxPassesClipboard reads the set-clipboard of the tmux that run runs.
// Only on passes an application's copy on to the terminal: external, the
// default, and off drop it. known is false outside tmux, or for a value it
// does not know.
func tmuxPassesClipboard(run func(args ...string) (string, error)) (ok, known bool) {
	out, err := run("show", "-sv", "set-clipboard")
	if err != nil {
		return false, false
	}
	switch strings.TrimSpace(out) {
	case "on":
		return true, true
	case "external", "off":
		return false, true
	}
	return false, false
}

// readClip reads again whether the console's own tmux passes a copy on.
func (m *Model) readClip() (ok, known bool) {
	m.clipOK, m.clipKnown = false, false
	if run := m.tmuxRun(); run != nil {
		m.clipOK, m.clipKnown = tmuxPassesClipboard(run)
	}
	return m.clipOK, m.clipKnown
}

// tmuxRun runs a command of the console's own tmux; nil outside tmux.
func (m Model) tmuxRun() func(args ...string) (string, error) {
	switch {
	case m.opt.OutsideTmux || m.opt.Pane == "":
		return nil
	case m.opt.Tmux != nil:
		return m.opt.Tmux
	}
	return tmuxOut
}

// clipMsg is what the console's tmux said of a copy at the start.
type clipMsg struct{ ok, known bool }

// clipRead reads, off the update loop, whether the console's tmux passes a
// copy on, so the board offers its door before the first copy.
func (m Model) clipRead() tea.Cmd {
	run := m.tmuxRun()
	if run == nil {
		return nil
	}
	return func() tea.Msg {
		ok, known := tmuxPassesClipboard(run)
		return clipMsg{ok, known}
	}
}

// copyText puts text on the clipboard and says so, with what it was. When
// the console's tmux drops the write, nothing is written and the notice
// says the fix. Until a paste proves the whole way, it says so too.
func (m *Model) copyText(text, what string) tea.Cmd {
	if ok, known := m.readClip(); known && !ok {
		return m.notice(tmuxDrops)
	}
	said := "copied: " + what
	if !m.copyChecked() {
		said += notChecked
	}
	return tea.Batch(m.clip(text), m.notice(said))
}

// copyDoors are the doors of the copy, on the board only and, as the
// service's doors, from the first state: the check until a paste proves
// this screen, and the fix while the console's tmux drops a copy.
func (m Model) copyDoors() []doors.Door {
	if !m.have {
		return nil
	}
	var ds []doors.Door
	if !m.copyChecked() {
		ds = append(ds, doors.CopyCheckDoor())
	}
	if m.clipKnown && !m.clipOK {
		ds = append(ds, doors.CopyThroughDoor())
	}
	return ds
}

// boardDoors are the doors of the board's doors line: the copy's first.
func (m Model) boardDoors() []doors.Door { return append(m.copyDoors(), m.doors...) }

// doorIndex is the door the doors line names, of ds: the first again once
// the list of doors changes.
func (m Model) doorIndex(ds []doors.Door) int {
	if m.boardDoorsOf != doorIDs(ds) {
		return 0
	}
	return max(min(m.boardDoor, len(ds)-1), 0)
}

// doorIDs names a list of doors.
func doorIDs(ds []doors.Door) string {
	ids := make([]string, len(ds))
	for i, d := range ds {
		ids[i] = d.ID
	}
	return strings.Join(ids, " ")
}

// stepDoor names the door d steps (← -1, → 1) along the doors line.
func (m *Model) stepDoor(d int) {
	ds := m.boardDoors()
	m.boardDoor, m.boardDoorsOf = max(min(m.doorIndex(ds)+d, len(ds)-1), 0), doorIDs(ds)
}

// openBoardDoor is ⏎ or a tap on the doors line: it opens the door the
// line names. The check copies its code, the fix waits for its y, and a
// service's door opens as from its row in Sessions.
func (m Model) openBoardDoor() (tea.Model, tea.Cmd) {
	ds := m.boardDoors()
	if len(ds) == 0 {
		return m, nil
	}
	i := m.doorIndex(ds)
	switch d := ds[i]; d.ID {
	case doors.CopyCheck:
		cmd := m.openCopyCheck()
		return m, cmd
	case doors.CopyThrough:
		m.pending, m.previewTop = m.doorAction(d), 0 // this machine's tmux, whatever the host
		return m, nil
	}
	m.selectRow(i - len(m.copyDoors()))
	return m.key(keyPress(m.keys.main.Open.Keys()[0]))
}

// openCopyCheck copies a new code of six letters; a paste of it checks
// this screen.
func (m *Model) openCopyCheck() tea.Cmd {
	if ok, known := m.readClip(); known && !ok {
		return m.notice(tmuxDrops)
	}
	code := make([]byte, 6)
	for i := range code {
		code[i] = copyCodeChars[rand.N(len(copyCodeChars))] //nolint:gosec // a check of the screen, no secret
	}
	m.copyCode = string(code)
	return tea.Batch(m.clip(m.copyCode), m.notice(pasteHere))
}

// copyPasted is a paste while the check waits: the code checks this
// screen, any other text says that a hop drops the copy.
func (m Model) copyPasted(text string) (Model, tea.Cmd) {
	if strings.TrimSpace(text) != m.copyCode {
		return m, m.notice(pasteWrong)
	}
	m.copyCode = ""
	if m.upright() {
		m.viewPrefs.CopyCheckedPhone = true
	} else {
		m.viewPrefs.CopyCheckedDesk = true
	}
	m.keepViewPrefs()
	return m, m.notice(pasteChecked)
}

// copyRan is the fix's door back from tmux: the console reads again.
func (m Model) copyRan() Model {
	if ok, _ := m.readClip(); ok {
		m.flash = "tmux lets a copy through now"
	} else {
		m.flash = "tmux still drops a copy"
	}
	return m
}

// dragStart keeps a left press as the start of a drag: on the board, on a
// mid screen or a desk. A phone never selects: its menu copies. The frame
// as it is at the press, before any tap acts, is the text a drag copies,
// and the column under the press bounds each row of it.
func (m Model) dragStart(x, y int) Model {
	m.dragFrom, m.dragTo, m.dragLines, m.tapLater = nil, nil, nil, nil // a release that never came leaves no tap
	w := max(m.width, minWidth)
	if m.boardShown() && m.menu == nil && m.layoutAt(w) != Narrow {
		sc := m.drawScreen(w)
		m.dragFrom, m.dragLines, m.dragWithin = &point{x, y}, sc.lines, m.dragColumn(x, y-sc.bodyTop, w)
	}
	return m
}

// dragColumn is the cells of each row that a drag from x on line y of the
// board's body may cover: those of the column or the live view under the
// press, else the whole width.
func (m Model) dragColumn(x, y, w int) [2]int {
	d := m.boardPanel(w, m.bodyRows())
	switch {
	case d.headAt < 0 || y < d.headAt || y >= d.colsTo:
		return [2]int{0, w}
	case d.live > 0 && x >= w-d.live:
		return [2]int{w - d.live, w}
	}
	for _, h := range d.heads {
		if x >= h.x0 && x <= h.x1 { // the gap at a column's right is its own
			return [2]int{h.x0, h.x1}
		}
	}
	return [2]int{0, w}
}

// dragOn is a move with the button down: it starts the drag once it is
// more than a cell from the press, and the drag follows it from then.
func (m Model) dragOn(at point) Model {
	if m.dragFrom == nil || m.menu != nil {
		return m
	}
	if m.dragTo != nil || farFrom(*m.dragFrom, at) {
		m.dragTo = &at
	}
	return m
}

// dragEnd is the release. One within a cell of the press is a tap: the
// act the press left for it runs now. Else a drag copies the text it
// covers.
func (m Model) dragEnd(at point) (Model, tea.Cmd) {
	from, to, lines, later := m.dragFrom, m.dragTo, m.dragLines, m.tapLater
	m.dragFrom, m.dragTo, m.dragLines, m.tapLater = nil, nil, nil, nil
	switch {
	case m.menu != nil || !m.boardShown():
		return m, nil
	case later != nil && !farFrom(*later, at):
		return m.tapOnRelease(*later)
	case from == nil || to == nil || !farFrom(*from, at):
		return m, nil
	}
	m.dragFrom, m.dragTo = from, &at
	text := m.selectedText(drawn{lines: lines})
	m.dragFrom, m.dragTo = nil, nil
	if text == "" {
		return m, nil
	}
	return m, m.copyText(text, characters(utf8.RuneCountInString(text)))
}

// characters is what a drag of n characters copied.
func characters(n int) string {
	if n == 1 {
		return "1 character"
	}
	return strconv.Itoa(n) + " characters"
}

// tapWaits: the press at x, y of the board's body acts on its release,
// so that a drag or a long press may start there: on a mid screen or a
// desk any card and a column's name, so a drag never chooses or presses
// anything; on a phone the card the person chose, which goes in. Only on
// a terminal that reports the release of a left press; on a phone a key
// or an option on a card acts on the press.
func (m Model) tapWaits(x, y int) bool {
	if !m.sawRelease || !m.boardShown() || m.menu != nil {
		return false
	}
	_, s, ok := m.cardAtCell(x, y+m.drawScreen(max(m.width, minWidth)).bodyTop)
	if ok && m.dragFrom != nil {
		return true
	}
	if ok && m.chosenByHand(s) {
		d := m.boardPanel(max(m.width, minWidth), m.bodyRows())
		sp, onSpan := spanAt(d.taps[y], x)
		return !onSpan || sp.press == ""
	}
	if m.dragFrom == nil { // no drag on a phone: a name acts on the press
		return false
	}
	d := m.boardPanel(max(m.width, minWidth), m.bodyRows())
	if _, ok := spanAt(d.heads, x); ok && y == d.headAt {
		return true
	}
	sp, onSpan := spanAt(d.taps[y], x)
	return onSpan && sp.press == colQuiet.short()
}

// chosenByHand: s is the chosen card, and the person chose it, by a key
// or a tap. The board's own first choice is no choice of theirs.
func (m Model) chosenByHand(s sample.Session) bool {
	c, ok := m.boardSelected()
	return ok && paneOf(c) == paneOf(s) && m.boardMine == paneOf(s)
}

// tapOnRelease runs the tap of a press that waited for its release.
func (m Model) tapOnRelease(p point) (Model, tea.Cmd) {
	next, cmd := m.boardTap(p.x, p.y-m.drawScreen(max(m.width, minWidth)).bodyTop)
	if nm, ok := next.(Model); ok {
		return nm, cmd
	}
	return m, cmd
}

// dragSpan is the drag from its first cell to its last, in reading order.
func (m Model) dragSpan() (a, b point, ok bool) {
	if m.dragFrom == nil || m.dragTo == nil {
		return point{}, point{}, false
	}
	a, b = *m.dragFrom, *m.dragTo
	if b.y < a.y || b.y == a.y && b.x < a.x {
		a, b = b, a
	}
	return a, b, true
}

// dragCols are the cells the drag covers on line y of a line w wide, in
// the column within: the first row from the press, the last to the
// pointer, the rows between whole.
func dragCols(a, b point, y, w int, within [2]int) (x0, x1 int) {
	x0, x1 = within[0], min(within[1], w)
	if within[1] == 0 {
		x0, x1 = 0, w
	}
	if y == a.y {
		x0 = max(x0, a.x)
	}
	if y == b.y {
		x1 = min(x1, b.x+1)
	}
	return x0, max(x0, x1)
}

// selectedText is the plain text of the cells of sc the drag covers, row
// by row as a terminal selects in one column: each right-trimmed.
func (m Model) selectedText(sc drawn) string {
	a, b, ok := m.dragSpan()
	if !ok {
		return ""
	}
	var rows []string
	for y := max(a.y, 0); y <= b.y && y < len(sc.lines); y++ {
		plain := ansi.Strip(sc.lines[y])
		x0, x1 := dragCols(a, b, y, ansi.StringWidth(plain), m.dragWithin)
		rows = append(rows, strings.TrimRight(ansi.Cut(plain, x0, x1), " "))
	}
	return strings.TrimRight(strings.Join(rows, "\n"), "\n")
}

// overlayDrag draws, while the button is down, the frame of the press,
// which the drag copies, with the cells it covers on the wash.
func (m Model) overlayDrag(sc *drawn) {
	a, b, ok := m.dragSpan()
	if !ok {
		return
	}
	if len(m.dragLines) == len(sc.lines) {
		sc.lines = slices.Clone(m.dragLines)
	}
	for y := max(a.y, 0); y <= b.y && y < len(sc.lines); y++ {
		l := sc.lines[y]
		x0, x1 := dragCols(a, b, y, lipgloss.Width(l), m.dragWithin)
		if x0 >= x1 {
			continue
		}
		sc.lines[y] = ansi.Truncate(l, x0, "") + m.st.selected.Render(ansi.Strip(ansi.Cut(l, x0, x1))) + ansi.TruncateLeft(l, x1, "")
	}
}
