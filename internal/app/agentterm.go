package app

import (
	"bytes"
	"cmp"
	"encoding/base64"
	"fmt"
	"image/color"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"

	"github.com/blox-eng/matchblox/internal/sample"
)

// agentScreen is the terminal the agent's client draws on: it takes the
// client's output and answers its queries itself, so no sequence of the
// agent's reaches the owner's terminal. vt's emulator is the one.
type agentScreen interface {
	io.ReadWriteCloser
	CellAt(x, y int) uv.Cell
	// cells is a copy of the screen's top left w by h cells.
	cells(w, h int) []uv.Line
	Resize(w, h int)
	// copyAsked is the text the agent last asked to copy (OSC 52) since
	// the last call, which drops it from the screen, and its size in
	// bytes. A request over agentCopyMax keeps no text.
	copyAsked() (text string, size int, ok bool)
	// sendKey, paste and sendMouse type into the client as a terminal
	// does, in the modes the client asked for. They hand the bytes on to
	// Read, which startAgentTerm queues, so a client that reads nothing
	// never holds them up.
	sendKey(k uv.KeyPressEvent)
	paste(text string)
	sendMouse(m uv.MouseEvent)
	// cursor is where the client's cursor is, and whether it shows.
	cursor() (uv.Position, bool)
}

// vtScreen is vt's emulator under a lock. CellAt copies the cell under
// it: vt's own safe emulator hands out a pointer that its next write
// changes. Close ends Read by its pipe: vt's Close sets a flag that Read
// reads without a lock.
type vtScreen struct {
	mu    sync.Mutex
	e     *vt.Emulator
	asked *string // what the agent asked to copy, not yet taken
	size  int     // its size in bytes
}

// agentCopyMax is the most the agent may ask to copy; matchblox keeps no
// larger request.
const agentCopyMax = 64 << 10

// newAgentScreen keeps no scrollback: matchblox never reads it, and the
// agent's tmux keeps its own.
func newAgentScreen(w, h int) agentScreen {
	s := &vtScreen{e: vt.NewEmulator(w, h)}
	s.e.SetScrollbackSize(1)
	s.e.RegisterOscHandler(52, func(data []byte) bool { // under s.mu: only Write parses
		_, b64, ok := bytes.Cut(bytes.TrimPrefix(data, []byte("52;")), []byte(";"))
		if !ok {
			return true
		}
		size := len(b64)*3/4 - bytes.Count(b64[max(len(b64)-2, 0):], []byte("="))
		var text []byte
		var err error
		if size <= agentCopyMax {
			text, err = base64.StdEncoding.DecodeString(string(b64))
		}
		if err == nil {
			asked := string(text)
			s.asked, s.size = &asked, size
		}
		return true
	})
	return s
}

func (s *vtScreen) copyAsked() (string, int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	asked := s.asked
	s.asked = nil
	if asked == nil {
		return "", 0, false
	}
	return *asked, s.size, true
}

// Write parses p a piece at a time, so a draw waits on one piece, never
// on a whole read of a flood: vt's parser carries a sequence across them.
func (s *vtScreen) Write(p []byte) (int, error) {
	const piece = 256
	n := 0
	for n < len(p) {
		s.mu.Lock()
		m, err := s.e.Write(p[n:min(n+piece, len(p))])
		s.mu.Unlock()
		n += m
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func (s *vtScreen) Read(p []byte) (int, error) { return s.e.Read(p) }

func (s *vtScreen) Close() error { return s.e.InputPipe().(io.Closer).Close() }

func (s *vtScreen) Resize(w, h int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.e.Resize(w, h)
}

func (s *vtScreen) sendKey(k uv.KeyPressEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k.Mod &= uv.ModShift | uv.ModAlt | uv.ModCtrl // vt matches a key whole: a lock key's bit would drop it
	k.IsRepeat = false
	if seq, ok := modifiedKey(k); ok {
		s.e.SendText(seq)
		return
	}
	s.e.SendKey(k)
}

func (s *vtScreen) paste(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.e.Paste(text)
}

func (s *vtScreen) sendMouse(m uv.MouseEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.e.SendMouse(m)
}

func (s *vtScreen) cursor() (uv.Position, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.e.CursorPosition(), !s.e.CursorHidden()
}

// xtermKeys are the keys whose modified form is CSI n;m and a final.
var xtermKeys = map[rune]struct {
	n     int
	final byte
}{
	uv.KeyUp: {1, 'A'}, uv.KeyDown: {1, 'B'}, uv.KeyRight: {1, 'C'}, uv.KeyLeft: {1, 'D'},
	uv.KeyHome: {1, 'H'}, uv.KeyEnd: {1, 'F'}, uv.KeyF1: {1, 'P'}, uv.KeyF2: {1, 'Q'}, uv.KeyF3: {1, 'R'}, uv.KeyF4: {1, 'S'},
	uv.KeyInsert: {2, '~'}, uv.KeyDelete: {3, '~'}, uv.KeyPgUp: {5, '~'}, uv.KeyPgDown: {6, '~'},
	uv.KeyF5: {15, '~'}, uv.KeyF6: {17, '~'}, uv.KeyF7: {18, '~'}, uv.KeyF8: {19, '~'},
	uv.KeyF9: {20, '~'}, uv.KeyF10: {21, '~'}, uv.KeyF11: {23, '~'}, uv.KeyF12: {24, '~'},
}

// modifiedKey is k as xterm and the kitty protocol send it, for a modified
// key a plain terminal has no sequence for (shift+enter, ctrl+left), which
// vt would drop. The client's tmux reads both forms.
func modifiedKey(k uv.KeyPressEvent) (string, bool) {
	if plainKey(k, k.Mod&^uv.ModAlt) {
		return "", false
	}
	m := 1 + int(k.Mod) // xterm's and kitty's: shift 1, alt 2, ctrl 4, as uv's bits
	if x, ok := xtermKeys[k.Code]; ok {
		return fmt.Sprintf("\x1b[%d;%d%c", x.n, m, x.final), true
	}
	if k.Code > unicode.MaxRune {
		return "", false
	}
	return fmt.Sprintf("\x1b[%d;%du", k.Code, m), true
}

// plainKey tells if vt sends k, mod being its modifiers but alt, which
// vt sends as an ESC before it.
func plainKey(k uv.KeyPressEvent, mod uv.KeyMod) bool {
	switch mod {
	case 0:
		return true
	case uv.ModCtrl:
		return k.Code == uv.KeySpace || k.Code >= 'a' && k.Code <= 'z' || strings.ContainsRune(`[\]^_`, k.Code)
	case uv.ModShift:
		return k.Text != "" || k.Code == uv.KeyTab
	}
	return false
}

func (s *vtScreen) CellAt(x, y int) uv.Cell {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.e.CellAt(x, y); c != nil {
		return *c
	}
	return uv.EmptyCell
}

// cells takes the lock once: a draw that took it per cell waited on the
// client's every write, 281 ms in a flood.
func (s *vtScreen) cells(w, h int) []uv.Line {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]uv.Line, h)
	for y := range out {
		out[y] = uv.NewLine(w)
		for x := range out[y] {
			if c := s.e.CellAt(x, y); c != nil {
				out[y][x] = *c
			}
		}
	}
	return out
}

// ConsoleFPS is the console's frame rate, bubbletea's highest. A key
// waits for the next frame: 15 frames showed a key 37 ms late on average
// and 60 shows it 11 ms late, for 0.3% more of one core when idle (#285).
// The agent's echo needs 120: at 60 it took 17 ms at the 95th percentile,
// past the 16 ms bar; 120 costs 0.6% more of a core when idle.
const ConsoleFPS = 120

// agentFrame is the agent region's fastest pace (DESIGN.md §5).
const agentFrame = time.Second / 60

// agentFrameMsg says t's screen changed; at is when it was sent.
type agentFrameMsg struct {
	t  *agentTerm
	at time.Time
}

// agentTerm is one agent's screen inside matchblox: the view's nested tmux
// client in a pty, drawing on a screen of matchblox's own.
type agentTerm struct {
	scr     agentScreen
	pty     *os.File
	cmd     *exec.Cmd
	changed chan struct{}
	ended   chan struct{} // the client's output ended
	wg      sync.WaitGroup
	// framed: a frame came. Only the console's loop reads and writes it.
	framed bool
}

// clientArgv is a view's argv for a tmux command line (remote.ViewArgv),
// to run by exec. tmux would hand the pane each lone "\;" as ";"; exec
// hands it nothing, so it is ";" again. The screen draws true colour,
// which terminfo cannot tell the client, so its first tmux is told
// (-T RGB, tmux 3.2), wherever its other flags go.
func clientArgv(argv []string) []string {
	var run []string
	told := false
	for _, a := range argv {
		if a == `\;` {
			a = ";"
		}
		run = append(run, a)
		if a == "tmux" && !told {
			run, told = append(run, "-T", "RGB"), true
		}
	}
	return run
}

// startAgent starts the agent's screen; a test may run none.
var startAgent = startAgentTerm

// startAgentTerm runs a view's argv (clientArgv) in a pty of w by h.
func startAgentTerm(argv []string, w, h int) (*agentTerm, error) {
	run := clientArgv(argv)
	cmd := exec.Command(run[0], run[1:]...) //nolint:gosec // the view's own argv (remote.ViewArgv)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)}) //nolint:gosec // a region's size
	if err != nil {
		return nil, err
	}
	if f, err = pollable(f); err != nil {
		killGroup(cmd)
		_ = cmd.Wait()
		return nil, err
	}
	t := &agentTerm{scr: newAgentScreen(w, h), pty: f, cmd: cmd, changed: make(chan struct{}, 1), ended: make(chan struct{})}
	t.wg.Add(3)
	go func() { // the client's output onto the screen
		defer t.wg.Done()
		defer close(t.ended)
		buf := make([]byte, 32<<10)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				_, _ = t.scr.Write(buf[:n])
				select {
				case t.changed <- struct{}{}:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}()
	in := &inbox{ready: make(chan struct{}, 1)}
	go func() { // the screen's answers and the owner's input, queued: a client that reads nothing never holds up the console
		defer t.wg.Done()
		defer in.close()
		buf := make([]byte, 32<<10)
		for {
			n, err := t.scr.Read(buf)
			if err != nil {
				return
			}
			in.put(buf[:n])
		}
	}()
	go func() { // to the client in order; a failed write drops its bytes, never the rest
		defer t.wg.Done()
		for {
			p, ok := in.take()
			if !ok {
				return
			}
			_, _ = f.Write(p)
		}
	}()
	return t, nil
}

// inbox is the bytes on their way to the client, in order, unbounded.
type inbox struct {
	mu     sync.Mutex
	buf    []byte
	closed bool
	ready  chan struct{} // one wake for take, after a put or the close
}

func (b *inbox) put(p []byte) {
	b.mu.Lock()
	b.buf = append(b.buf, p...)
	b.mu.Unlock()
	b.wake()
}

func (b *inbox) close() {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	b.wake()
}

func (b *inbox) wake() {
	select {
	case b.ready <- struct{}{}:
	default:
	}
}

// take waits for bytes and takes them all; false once closed and empty.
func (b *inbox) take() ([]byte, bool) {
	for {
		b.mu.Lock()
		p, closed := b.buf, b.closed
		b.buf = nil
		b.mu.Unlock()
		if len(p) > 0 {
			return p, true
		}
		if closed {
			return nil, false
		}
		<-b.ready
	}
}

// next waits for the screen to change, then sends a frame no sooner than
// agentFrame after prev. It sends nothing while the agent prints nothing,
// and nil once the client's output ends.
func (t *agentTerm) next(prev time.Time) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-t.changed:
		case <-t.ended:
			select {
			case <-t.changed:
			default:
				return nil
			}
		}
		time.Sleep(agentFrame - time.Since(prev))
		return agentFrameMsg{t: t, at: time.Now()}
	}
}

// close hangs up the client, kills it when it stays a second, and waits a
// second more for its goroutines and process: one stuck past that is left
// behind, its pty closed, so the console never waits on a client.
func (t *agentTerm) close() {
	hangUp(t.cmd)
	select {
	case <-t.ended:
	case <-time.After(time.Second): // a client that ignores the hang-up
		killGroup(t.cmd)
	}
	_ = t.pty.Close() // ends a read, and a write the client never takes
	_ = t.scr.Close()
	done := make(chan struct{})
	go func() {
		t.wg.Wait()
		_ = t.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
	}
}

// draw is the screen in w by h cells, in colours and attributes only. A
// default colour draws as fg and bg, nil leaving the terminal's own; every
// other colour passes as it is. The cells of sel flip their reverse video,
// so a selection shows on any colour (design review D13).
func (t *agentTerm) draw(fg, bg color.Color, w, h int, sel *[2]point) []string {
	if sel != nil {
		whole := t.whole(*sel)
		sel = &whole
	}
	out := make([]string, h)
	for y, line := range t.scr.cells(w, h) {
		x0, x1 := selCols(sel, y, w)
		for x, c := range line {
			if !c.IsZero() { // a wide glyph's second cell stays empty
				c.Style.Fg, c.Style.Bg = cmp.Or[color.Color](c.Style.Fg, fg), cmp.Or[color.Color](c.Style.Bg, bg)
			}
			c.Link = uv.Link{} // an agent's hyperlink (OSC 8) stays inside its screen
			if !c.IsZero() {
				c.Content = cmp.Or(strings.Map(printable, c.Content), " ")
			}
			if x >= x0 && x < x1 {
				c.Style.Attrs ^= uv.AttrReverse
			}
			line[x] = c
		}
		out[y] = fit(line.Render(), w)
	}
	return out
}

// selCols are the cells of line y that sel covers, as a terminal selects:
// the first line from the press, the last to the pointer, those between
// whole.
func selCols(sel *[2]point, y, w int) (x0, x1 int) {
	if sel == nil {
		return 0, 0
	}
	a, b := readingOrder(sel[0], sel[1])
	if y < a.y || y > b.y {
		return 0, 0
	}
	return dragCols(a, b, y, w, [2]int{})
}

// whole is sel in reading order with each end on a whole glyph: a wide
// glyph under either end is selected whole.
func (t *agentTerm) whole(sel [2]point) [2]point {
	a, b := readingOrder(sel[0], sel[1])
	if a.x > 0 && t.scr.CellAt(a.x, a.y).Width == 0 && t.scr.CellAt(a.x-1, a.y).Width > 1 {
		a.x--
	}
	b.x += max(t.scr.CellAt(b.x, b.y).Width-1, 0)
	return [2]point{a, b}
}

// text is the plain text sel covers in a screen w wide, each line
// right-trimmed; a URL the agent drew over several lines is one (#474).
func (t *agentTerm) text(sel [2]point, w int) string {
	sel = t.whole(sel)
	a, b := sel[0], sel[1]
	var rows []string
	for y := a.y; y <= b.y; y++ {
		var row strings.Builder
		x0, x1 := selCols(&sel, y, w)
		for x := x0; x < x1; x++ {
			row.WriteString(t.scr.CellAt(x, y).Content)
		}
		rows = append(rows, strings.TrimRight(row.String(), " "))
	}
	return wholeURL(copyable(strings.TrimRight(strings.Join(rows, "\n"), "\n")))
}

// copyable is text as it may reach the clipboard: printable runes, tabs
// and line breaks; every other control rune is dropped.
func copyable(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' {
			return r
		}
		return printable(r)
	}, text)
}

// printable keeps a glyph's runes that only draw: a control or a
// direction override (U+202E) the agent printed would act on the owner's
// terminal. The joiner stays, so an emoji sequence draws whole.
func printable(r rune) rune {
	if unicode.IsPrint(r) || r == '\u200d' {
		return r
	}
	return -1
}

// agentRegion is the desk's agent region: the screen of the session it
// shows, and whether the agent has the keys. On a switch the keys go to
// the list, and the region says it opens the new agent until its first
// frame, so no key reaches the wrong agent (design review D6).
type agentRegion struct {
	term       *agentTerm
	name, said string    // said: why the screen did not start
	copy       string    // what the agent last asked to copy; c copies it
	press      *point    // a press in the region while its button is down
	sel        *[2]point // the owner's drag over the screen: its two ends
	selText    string    // the text sel covered when it was made
	keys       bool
	shell      bool // the desk shell under the agent has the keys (mode SHELL)
	x, y       int  // its top left on the console's screen
	w, h       int
}

// open shows name's screen in place of the one shown, the keys to the list.
func (r *agentRegion) open(name string, argv []string, w, h int) tea.Cmd {
	r.shut()
	r.name, r.keys, r.shell, r.w, r.h, r.copy, r.press, r.sel = name, false, false, w, h, "", nil, nil
	t, err := startAgent(argv, w, h)
	if err != nil {
		r.said = name + " cannot be shown: " + err.Error() + "."
		return nil
	}
	r.term, r.said = t, ""
	return t.next(time.Time{})
}

// shut ends the screen shown.
func (r *agentRegion) shut() {
	if r.term != nil {
		r.term.close()
		r.term = nil
	}
}

// frame marks the screen drawn, keeps what the agent asked to copy, and
// waits for its next change; a frame of a screen no longer shown is dropped.
// refused is the size of a request over agentCopyMax, which keeps none. A
// frame that changes the selected text (the agent's output scrolled it)
// ends the selection.
func (r *agentRegion) frame(msg agentFrameMsg) (next tea.Cmd, refused int) {
	if msg.t != r.term {
		return nil, 0
	}
	r.term.framed = true
	if text, size, ok := r.term.scr.copyAsked(); ok {
		r.copy = text
		if size > agentCopyMax {
			refused = size
		}
	}
	if r.sel != nil && r.term.text(*r.sel, r.w) != r.selText {
		r.sel, r.press = nil, nil
	}
	return r.term.next(msg.at), refused
}

func (r *agentRegion) resize(w, h int) {
	r.w, r.h, r.sel, r.press = w, h, nil, nil
	if r.term != nil {
		_ = ptySize(r.term.pty, w, h)
		r.term.scr.Resize(w, h)
	}
}

// shown tells if the agent's screen shows: its first frame came.
func (r *agentRegion) shown() bool { return r.term != nil && r.term.framed }

// live: the agent's screen shows, and its view still runs.
func (r *agentRegion) live() bool {
	if !r.shown() {
		return false
	}
	select {
	case <-r.term.ended:
		return false
	default:
		return true
	}
}

// mode is who has the keys: the agent or its shell once its screen shows,
// else the list.
func (r *agentRegion) mode() string {
	if r.keys && r.shown() {
		if r.shell {
			return "SHELL"
		}
		return "AGENT"
	}
	return "LIST"
}

// at is m in the agent's screen, if m is in the region.
func (r *agentRegion) at(m tea.Mouse) (uv.Mouse, bool) {
	m.X, m.Y = m.X-r.x, m.Y-r.y
	return uv.Mouse(m), m.X >= 0 && m.Y >= 0 && m.X < r.w && m.Y < r.h
}

// clamped is m in the agent's screen, held to its edges: a drag that
// leaves the region selects only the region's cells.
func (r *agentRegion) clamped(m tea.Mouse) point {
	at, _ := r.at(m)
	return point{min(max(at.X, 0), r.w-1), min(max(at.Y, 0), r.h-1)}
}

// cursor is the agent's cursor on the console's screen, while the agent
// has the keys and shows it.
func (r *agentRegion) cursor() *tea.Cursor {
	if r.mode() != "AGENT" && r.mode() != "SHELL" {
		return nil
	}
	at, shows := r.term.scr.cursor()
	if !shows {
		return nil
	}
	return tea.NewCursor(r.x+at.X, r.y+at.Y)
}

// lines is the region in w by h cells, in st's theme.
func (r *agentRegion) lines(st styles, w, h int) []string {
	switch {
	case r.term != nil && r.term.framed:
		return r.term.draw(st.theme.Text, st.ground, w, h, r.sel)
	case r.said != "":
		return regionSays(st.muted, r.said, w, h)
	case r.name != "":
		return regionSays(st.faint, "opening "+r.name+"…", w, h)
	}
	return regionSays(st.faint, "", w, h)
}

// agentFrame takes a frame of the agent's screen. The agent's write to the
// clipboard is dropped, so the owner decides: the list foot says once that
// it asked, until the copy key copies it.
func (m *Model) agentFrame(msg agentFrameMsg) tea.Cmd {
	had := m.agent.copy
	next, refused := m.agent.frame(msg)
	if refused > 0 {
		return tea.Batch(next, m.notice("not kept: the agent asked to copy "+sizeWords(refused)+", "+sizeWords(agentCopyMax)+" at most"))
	}
	if had == "" && m.agent.copy != "" {
		return tea.Batch(next, m.notice("the agent asked to copy: "+m.keys.board.Copy.Help().Key+" copies it"))
	}
	return next
}

// sizeWords is n bytes in KB or MB, rounded.
func sizeWords(n int) string {
	if n >= 1<<20 {
		return strconv.Itoa((n+1<<19)>>20) + " MB"
	}
	return strconv.Itoa((n+1<<9)>>10) + " KB"
}

// agentLines is the agent region of the session chosen, or why it is
// empty: no sessions, or its host not reached (design review D6).
func (m Model) agentLines(w, h int) []string {
	k := m.chosenSide()
	listed := m.sideSessions()
	i := slices.IndexFunc(listed, func(s sample.Session) bool { return paneOf(s) == k })
	if k.pane == "" || i < 0 {
		return regionSays(m.st.muted, m.noSessionsWords(), w, h)
	}
	if host := cmp.Or(listed[i].Host, m.opt.Host); host != m.opt.Host && m.watchConn(host) == nil {
		return regionSays(m.st.warn, m.nameOf(host)+" is not reached · it redials", w, h)
	}
	return m.agent.lines(m.st, w, h)
}

// regionSays is text under a blank line, wrapped to w, in h lines.
func regionSays(ink lipgloss.Style, text string, w, h int) []string {
	out := []string{strings.Repeat(" ", w)}
	for _, l := range wrapPlain(text, max(w-2, 1)) {
		out = append(out, fit(" "+ink.Render(l), w))
	}
	for len(out) < h {
		out = append(out, strings.Repeat(" ", w))
	}
	return out[:h]
}
