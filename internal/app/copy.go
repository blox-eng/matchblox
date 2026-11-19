package app

import (
	"errors"
	"math/rand/v2"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
)

// The words of a copy (the spec's "Copy").
const (
	notChecked    = " · not checked here"
	pasteWrong    = `That is not the code. A hop drops the copy: see docs/console.md, "Copy".`
	pasteChecked  = "Copy reaches this screen."
	copyCodeChars = "abcdefghijklmnopqrstuvwxyz"
)

// point is a cell of the frame: its column and its line.
type point struct{ x, y int }

// farFrom: b is more than one cell from a. Such a move is a drag, and
// ends a long press; a move of one cell is neither.
func farFrom(a, b point) bool { return max(abs(a.x-b.x), abs(a.y-b.y)) > 1 }

// clip puts text on the clipboard of the screen the person uses: by Clip
// where the app gives one (Pocket), by tmux's own load-buffer -w inside
// tmux, so its set-clipboard stays as the person has it, else by OSC 52
// to the terminal.
func (m Model) clip(text string) (tea.Cmd, error) {
	if m.opt.Clip != nil {
		m.opt.Clip(text)
		return nil, nil
	}
	if run := m.tmuxRun(); run != nil {
		client, ok := activeClient(run, m.now(), copyActive, func(string) bool { return true })
		if !ok {
			return nil, errNoScreen
		}
		return nil, loadTo(run, client, text)
	}
	return tea.SetClipboard(text), nil
}

// errNoScreen: no screen of the person's had input lately, so a copy has
// nowhere to go.
var errNoScreen = errors.New("no screen of yours had input in the last 10 s")

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

// copyText puts text on the clipboard and says so, with what it was, or
// why it did not. Until a paste proves the whole way, it says so too.
func (m *Model) copyText(text, what string) tea.Cmd {
	cmd, err := m.clip(text)
	if err != nil {
		return m.notice("not copied: " + err.Error())
	}
	said := "copied: " + what
	if !m.copyChecked() {
		said += notChecked
	}
	return tea.Batch(cmd, m.notice(said))
}

// copyDoors are the doors of the copy, on the board only and, as the
// service's doors, from the first state: the check until a paste proves
// this screen.
func (m Model) copyDoors() []doors.Door {
	if !m.have {
		return nil
	}
	var ds []doors.Door
	if !m.copyChecked() {
		ds = append(ds, doors.CopyCheckDoor())
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
	}
	m.selectRow(i - len(m.copyDoors()))
	return m.key(keyPress(m.keys.main.Open.Keys()[0]))
}

// openCopyCheck copies a new code of six letters; a paste of it checks
// this screen.
func (m *Model) openCopyCheck() tea.Cmd {
	code := make([]byte, 6)
	for i := range code {
		code[i] = copyCodeChars[rand.N(len(copyCodeChars))] //nolint:gosec // a check of the screen, no secret
	}
	cmd, err := m.clip(string(code))
	if err != nil {
		return m.notice("not copied: " + err.Error())
	}
	m.copyCode = string(code)
	return tea.Batch(cmd, m.notice(m.pasteHere()))
}

// pasteHere is what the copy check says while it waits for its paste.
func (m Model) pasteHere() string {
	return "Now paste here · " + m.keys.main.Back.Help().Key + " cancels"
}

// copyCheckOn ends the copy check, unchecked, once the setup steps leave
// the chat's place, or they name another step: the paste it waits for
// answers only that step.
func (m Model) copyCheckOn() Model {
	if m.copyCode == "" {
		return m
	}
	ds := m.boardDoors()
	if !m.sideActive() || m.sidePlace != placeSetup || len(ds) == 0 || ds[m.doorIndex(ds)].ID != doors.CopyCheck {
		m.copyCode = ""
		if m.noticeText == m.pasteHere() {
			m.noticeText = ""
		}
	}
	return m
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

// dragStart keeps a left press as the start of a drag: on the board, on a
// mid screen or a desk. A phone never selects: its menu copies. The frame
// as it is at the press, before any tap acts, is the text a drag copies,
// and the column under the press bounds each row of it.
func (m Model) dragStart(x, y int) Model {
	m.dragFrom, m.dragTo, m.dragLines, m.tapLater = nil, nil, nil, nil // a release that never came leaves no tap
	w := max(m.width, minWidth)
	if m.boardShown() && m.menu == nil && !m.boxZoom && m.layoutAt(w) != Narrow { // behind a box the agent is a picture: it copies nothing
		sc := m.drawScreen(w)
		m.dragFrom, m.dragLines, m.dragWithin = &point{x, y}, sc.lines, m.dragColumn(x, y-sc.bodyTop, w)
	}
	return m
}

// dragDropped ends a drag whose release never came, at a key, a paste or
// a wheel step: the frame of the press shows no more, and the screen is
// live again. A press starts its own drag.
func (m Model) dragDropped(msg tea.Msg) Model {
	if _, press := msg.(tea.MouseClickMsg); !press {
		m.dragFrom, m.dragTo, m.dragLines, m.tapLater = nil, nil, nil, nil
	}
	return m
}

// dragColumn is the cells of each row that a drag from x on line y of the
// board's body may cover: those of the column under the press, else the
// whole width.
func (m Model) dragColumn(x, y, w int) [2]int {
	if !m.sideActive() || y < 0 || y >= m.bodyRows() {
		return [2]int{0, w}
	}
	return m.sideDragCols(x, w)
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

// tapOnRelease runs the tap of a press that waited for its release.
func (m Model) tapOnRelease(p point) (Model, tea.Cmd) {
	next, cmd := m.sideTap(p.x, p.y-m.drawScreen(max(m.width, minWidth)).bodyTop)
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
// by row as a terminal selects in one column: each right-trimmed, and
// without the … that ends a cut line.
func (m Model) selectedText(sc drawn) string {
	a, b, ok := m.dragSpan()
	if !ok {
		return ""
	}
	var rows []string
	for y := max(a.y, 0); y <= b.y && y < len(sc.lines); y++ {
		plain := ansi.Strip(sc.lines[y])
		x0, x1 := dragCols(a, b, y, ansi.StringWidth(plain), m.dragWithin)
		row := strings.TrimRight(ansi.Cut(plain, x0, x1), " ")
		rows = append(rows, strings.TrimSuffix(row, "…")) // the mark of a cut line is no text
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

// doorKey is x on the setup step in the chat's place: a service's door
// closes, behind its y, as from its row in Sessions. false: the door is
// the console's own.
func (m Model) doorKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	ds, own := m.boardDoors(), len(m.copyDoors())
	i := m.doorIndex(ds)
	if i < own {
		return m, nil, false
	}
	m.selectRow(i - own)
	next, cmd := m.key(k)
	return next, cmd, true
}

// A copy reaches the clipboard only from a screen the person just used:
// within copyActive for this host's copy, and within hostCopyActive, with
// that host on the screen, for a peer's (owner, 2026-11-20). A program on
// a host then cannot fill the clipboard while the person looks elsewhere.
const (
	copyActive     = 10 * time.Second
	hostCopyActive = 2 * time.Second
)

// tookCopy puts a copy that a service sent (a tmux copy on that host) on
// the screen the person uses; from names a peer host, "" for this one. A
// console with its own clipboard (Pocket) takes none: the phone copies by
// its own key.
func (m Model) tookCopy(text, from string) (Model, tea.Cmd) {
	if m.opt.Clip != nil {
		return m, nil
	}
	what := characters(utf8.RuneCountInString(text))
	if from != "" {
		what += " from " + from
	}
	run := m.tmuxRun()
	if run == nil {
		if from != "" {
			return m, nil // no screen to tell it shows the host
		}
		cmd := m.notice("copied: " + what)
		return m, tea.Batch(tea.SetClipboard(text), cmd)
	}
	window, shows := copyActive, func(string) bool { return true }
	if from != "" {
		own, _ := run("display", "-p", "-t", m.opt.Pane, "#{session_name}")
		own = strings.TrimSpace(own)
		window, shows = hostCopyActive, func(session string) bool {
			return session == own && m.besideShowsHost(from) || jumpOf(session, from)
		}
	}
	client, ok := activeClient(run, m.now(), window, shows)
	if !ok {
		return m, nil
	}
	if err := loadTo(run, client, text); err != nil {
		return m, m.notice("not copied: " + err.Error())
	}
	said := "copied: " + what
	if !m.copyChecked() {
		said += notChecked
	}
	return m, m.notice(said)
}

// besideShowsHost: the agent pane beside the list shows a session of host,
// alone or in a chain view.
func (m Model) besideShowsHost(host string) bool {
	if m.besideShows.host == host && m.besideShows.pane != "" {
		return true
	}
	if m.chain != nil {
		for _, k := range m.chain.on {
			if k.host == host {
				return true
			}
		}
	}
	return false
}

// jumpOf: session is this machine's session of a jump to host.
func jumpOf(session, host string) bool {
	id, ok := strings.CutPrefix(session, remote.HostSessionName(host, ""))
	return ok && len(id) == 8
}

// activeClient is the person's client that had input last, within window of
// now, on a session shows accepts. A client on a pane's terminal (a nested
// view) is never a person's screen.
func activeClient(run func(args ...string) (string, error), now time.Time, window time.Duration, shows func(session string) bool) (string, bool) {
	clients, err := run("list-clients", "-F", "#{client_name}\t#{client_activity}\t#{client_tty}\t#{client_session}")
	if err != nil {
		return "", false
	}
	ttys, _ := run("list-panes", "-a", "-F", "#{pane_tty}")
	nested := strings.Fields(ttys)
	var best string
	var bestAt int64
	for _, l := range strings.Split(strings.TrimSpace(clients), "\n") {
		f := strings.Split(l, "\t")
		if len(f) != 4 || slices.Contains(nested, f[2]) || !shows(f[3]) {
			continue
		}
		at, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil || now.Sub(time.Unix(at, 0)) > window || at <= bestAt {
			continue
		}
		best, bestAt = f[0], at
	}
	return best, best != ""
}

// loadTo puts text in the console's buffer and on client's clipboard, by
// tmux itself: tmux's set-clipboard does not stop that. The text goes by a
// file of the person's own, as a copy can be longer than an argument.
func loadTo(run func(args ...string) (string, error), client, text string) error {
	f, err := os.CreateTemp("", "matchblox-copy-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(text); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if _, err := run("load-buffer", "-w", "-b", proto.CopyBuffer, "-t", client, f.Name()); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return errors.New(strings.TrimSpace(string(ee.Stderr)))
		}
		return err
	}
	return nil
}
