package app

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/doors"
)

// The words of a copy (the spec's "Copy").
const (
	tmuxDrops     = "not copied: tmux drops it · tmux set -s set-clipboard on"
	notChecked    = " · not checked here"
	pasteHere     = "Now paste here."
	pasteWrong    = `That is not what the console copied. A hop drops it: see docs/console.md, "Copy".`
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

// tmuxPassesClipboard reads the set-clipboard of the tmux that run runs:
// on and external pass a copy on to the terminal, off drops it. known is
// false outside tmux, or for a value it does not know.
func tmuxPassesClipboard(run func(args ...string) (string, error)) (ok, known bool) {
	out, err := run("show", "-sv", "set-clipboard")
	if err != nil {
		return false, false
	}
	switch strings.TrimSpace(out) {
	case "on", "external":
		return true, true
	case "off":
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

// openCopyDoor is ⏎ on the doors line when a door of the copy leads: the
// check copies its code, the fix waits for its y. false: another door leads.
func (m Model) openCopyDoor() (Model, tea.Cmd, bool) {
	ds := m.copyDoors()
	if len(ds) == 0 {
		return m, nil, false
	}
	switch d := ds[0]; d.ID {
	case doors.CopyCheck:
		cmd := m.openCopyCheck()
		return m, cmd, true
	default:
		m.pending, m.previewTop = m.doorAction(d), 0 // this machine's tmux, whatever the host
		return m, nil, true
	}
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
// mid screen or a desk. A phone never selects: its menu copies.
func (m Model) dragStart(x, y int) Model {
	m.dragFrom, m.dragTo = nil, nil
	if m.boardShown() && m.menu == nil && m.layoutAt(max(m.width, minWidth)) != Narrow {
		m.dragFrom = &point{x, y}
	}
	return m
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

// dragEnd is the release: a drag copies the text it covers. A press with
// no drag was a tap, and it acted.
func (m Model) dragEnd(at point) (Model, tea.Cmd) {
	dragged := m.dragTo != nil && m.menu == nil && m.boardShown()
	if !dragged {
		m.dragFrom, m.dragTo = nil, nil
		return m, nil
	}
	m.dragTo = &at
	text := m.selectedText(m.drawScreen(max(m.width, minWidth)))
	m.dragFrom, m.dragTo = nil, nil
	if text == "" {
		return m, nil
	}
	return m, m.copyText(text, strconv.Itoa(utf8.RuneCountInString(text))+" characters")
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

// dragCols are the columns the drag covers on line y of a line w wide.
func dragCols(a, b point, y, w int) (x0, x1 int) {
	x0, x1 = 0, w
	if y == a.y {
		x0 = a.x
	}
	if y == b.y {
		x1 = b.x + 1
	}
	return x0, min(x1, w)
}

// selectedText is the plain text of the cells of sc the drag covers, row
// by row as a terminal selects: the first row from the press, the last to
// the pointer, each right-trimmed.
func (m Model) selectedText(sc drawn) string {
	a, b, ok := m.dragSpan()
	if !ok {
		return ""
	}
	var rows []string
	for y := max(a.y, 0); y <= b.y && y < len(sc.lines); y++ {
		plain := ansi.Strip(sc.lines[y])
		x0, x1 := dragCols(a, b, y, ansi.StringWidth(plain))
		rows = append(rows, strings.TrimRight(ansi.Cut(plain, x0, x1), " "))
	}
	return strings.TrimRight(strings.Join(rows, "\n"), "\n")
}

// overlayDrag draws the cells the drag covers in the accent while the
// button is down.
func (m Model) overlayDrag(sc *drawn) {
	a, b, ok := m.dragSpan()
	if !ok {
		return
	}
	for y := max(a.y, 0); y <= b.y && y < len(sc.lines); y++ {
		l := sc.lines[y]
		x0, x1 := dragCols(a, b, y, lipgloss.Width(l))
		if x0 >= x1 {
			continue
		}
		sc.lines[y] = ansi.Truncate(l, x0, "") + m.st.accent.Render(ansi.Strip(ansi.Cut(l, x0, x1))) + ansi.TruncateLeft(l, x1, "")
	}
}
