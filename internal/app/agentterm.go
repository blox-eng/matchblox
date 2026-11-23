package app

import (
	"cmp"
	"image/color"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"

	"github.com/blox-eng/matchblox/internal/sample"
)

// agentScreen is the terminal the agent's client draws on: it takes the
// client's output and answers its queries itself. vt's emulator is the one.
type agentScreen interface {
	io.ReadWriteCloser
	CellAt(x, y int) uv.Cell
	// cells is a copy of the screen's top left w by h cells.
	cells(w, h int) []uv.Line
	Resize(w, h int)
}

// vtScreen is vt's emulator under a lock. CellAt copies the cell under
// it: vt's own safe emulator hands out a pointer that its next write
// changes. Close ends Read by its pipe: vt's Close sets a flag that Read
// reads without a lock.
type vtScreen struct {
	mu sync.Mutex
	e  *vt.Emulator
}

// newAgentScreen keeps no scrollback: matchblox never reads it, and the
// agent's tmux keeps its own.
func newAgentScreen(w, h int) agentScreen {
	e := vt.NewEmulator(w, h)
	e.SetScrollbackSize(1)
	return &vtScreen{e: e}
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

// startAgentTerm runs a view's argv (clientArgv) in a pty of w by h.
func startAgentTerm(argv []string, w, h int) (*agentTerm, error) {
	run := clientArgv(argv)
	cmd := exec.Command(run[0], run[1:]...) //nolint:gosec // the view's own argv (remote.ViewArgv)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)}) //nolint:gosec // a region's size
	if err != nil {
		return nil, err
	}
	t := &agentTerm{scr: newAgentScreen(w, h), pty: f, cmd: cmd, changed: make(chan struct{}, 1), ended: make(chan struct{})}
	t.wg.Add(2)
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
	go func() { // the screen's answers to the client; a failed write must not stop them, or the screen blocks
		defer t.wg.Done()
		buf := make([]byte, 4<<10)
		for {
			n, err := t.scr.Read(buf)
			if err != nil {
				return
			}
			_, _ = f.Write(buf[:n])
		}
	}()
	return t, nil
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

// close hangs up the client, kills it when it stays a second, and waits
// for its goroutines and process.
func (t *agentTerm) close() {
	hangUp(t.cmd)
	_ = t.pty.Close()
	select {
	case <-t.ended:
	case <-time.After(time.Second): // a client that ignores the hang-up
		killGroup(t.cmd)
		<-t.ended
	}
	_ = t.scr.Close()
	t.wg.Wait()
	_ = t.cmd.Wait()
}

// draw is the screen in w by h cells. A default colour draws as fg and bg,
// nil leaving the terminal's own; every other colour passes as it is.
func (t *agentTerm) draw(fg, bg color.Color, w, h int) []string {
	out := make([]string, h)
	for y, line := range t.scr.cells(w, h) {
		for x, c := range line {
			if !c.IsZero() { // a wide glyph's second cell stays empty
				c.Style.Fg, c.Style.Bg = cmp.Or[color.Color](c.Style.Fg, fg), cmp.Or[color.Color](c.Style.Bg, bg)
			}
			line[x] = c
		}
		out[y] = fit(line.Render(), w)
	}
	return out
}

// agentRegion is the desk's agent region: the screen of the session it
// shows, and whether the agent has the keys. On a switch the keys go to
// the list, and the region says it opens the new agent until its first
// frame, so no key reaches the wrong agent (design review D6).
type agentRegion struct {
	term       *agentTerm
	name, said string // said: why the screen did not start
	keys       bool
	w, h       int
}

// open shows name's screen in place of the one shown, the keys to the list.
func (r *agentRegion) open(name string, argv []string, w, h int) tea.Cmd {
	r.shut()
	r.name, r.keys, r.w, r.h = name, false, w, h
	t, err := startAgentTerm(argv, w, h)
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

// frame marks the screen drawn and waits for its next change; a frame of
// a screen no longer shown is dropped.
func (r *agentRegion) frame(msg agentFrameMsg) tea.Cmd {
	if msg.t != r.term {
		return nil
	}
	r.term.framed = true
	return r.term.next(msg.at)
}

func (r *agentRegion) resize(w, h int) {
	r.w, r.h = w, h
	if r.term != nil {
		_ = pty.Setsize(r.term.pty, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)}) //nolint:gosec // a region's size
		r.term.scr.Resize(w, h)
	}
}

// mode is who has the keys: the agent once its screen shows, else the list.
func (r *agentRegion) mode() string {
	if r.keys && r.term != nil && r.term.framed {
		return "AGENT"
	}
	return "LIST"
}

// lines is the region in w by h cells, in st's theme.
func (r *agentRegion) lines(st styles, w, h int) []string {
	switch {
	case r.term != nil && r.term.framed:
		return r.term.draw(st.theme.Text, st.ground, w, h)
	case r.said != "":
		return regionSays(st.muted, r.said, w, h)
	case r.name != "":
		return regionSays(st.faint, "opening "+r.name+"…", w, h)
	}
	return regionSays(st.faint, "", w, h)
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
