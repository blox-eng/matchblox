package service

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// chatSizes are the sizes the consoles' live views give the agents'
// screens, and the windows sized to them. A window of one pane takes the
// size of the view that sent the pane keys last (the demo's stand-in),
// else of the one that watched it last; with no view on it, it is put back.
type chatSizes struct {
	mu     sync.Mutex
	seq    uint64
	wants  map[string]map[*client]sizeWant // by pane
	held   map[string]heldWindow           // by pane
	closed bool                            // the service stops: nothing is sized again
	// gen is odd while a window is sized: a screen captured then does not
	// say a size.
	gen atomic.Uint64
}

// sizeWant is one chat's size for a pane, and when it watched the pane
// and took its keys (0: never), in the order of seq.
type sizeWant struct {
	cols, rows     int
	watched, keyed uint64
}

// heldWindow is a window sized to a chat: the size asked, the pane's size
// after, and the mark that puts it back.
type heldWindow struct {
	window, mark string
	cols, rows   int
	paneW, paneH int
}

// watch notes that cl watches pane ("": none) with a chat of cols by rows
// (0: not drawn). It returns the panes whose window may change.
func (z *chatSizes) watch(cl *client, pane string, cols, rows int) []string {
	z.mu.Lock()
	defer z.mu.Unlock()
	touched := z.dropLocked(cl, pane)
	if pane == "" {
		return touched
	}
	if z.wants == nil {
		z.wants = map[string]map[*client]sizeWant{}
	}
	ws := z.wants[pane]
	if ws == nil {
		ws = map[*client]sizeWant{}
		z.wants[pane] = ws
	}
	w, had := ws[cl]
	if !had { // a new watch; a new size of the same chat is not one
		z.seq++
		w.watched = z.seq
	}
	w.cols, w.rows = max(cols, 0), max(rows, 0)
	ws[cl] = w
	return append(touched, pane)
}

// keyed notes that cl sent a key to pane; it tells if cl's chat watches
// it.
func (z *chatSizes) keyed(cl *client, pane string) bool {
	z.mu.Lock()
	defer z.mu.Unlock()
	w, ok := z.wants[pane][cl]
	if ok {
		z.seq++
		w.keyed = z.seq
		z.wants[pane][cl] = w
	}
	return ok
}

// drop forgets every pane cl watches, and returns them.
func (z *chatSizes) drop(cl *client) []string {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.dropLocked(cl, "")
}

// dropLocked forgets the panes cl watches but keep, and returns them.
func (z *chatSizes) dropLocked(cl *client, keep string) []string {
	var out []string
	for p, ws := range z.wants {
		if _, ok := ws[cl]; ok && p != keep {
			delete(ws, cl)
			if len(ws) == 0 {
				delete(z.wants, p)
			}
			out = append(out, p)
		}
	}
	return out
}

// winner is the size pane's window takes: the chat's with a size that took
// the keys last, else watched last. False: none, or the service stops.
func (z *chatSizes) winner(pane string) (sizeWant, bool) {
	z.mu.Lock()
	defer z.mu.Unlock()
	var best sizeWant
	found := false
	for _, w := range z.wants[pane] {
		if z.closed || w.cols <= 0 || w.rows <= 0 {
			continue
		}
		if !found || w.keyed > best.keyed || w.keyed == best.keyed && w.watched > best.watched {
			best, found = w, true
		}
	}
	return best, found
}

func (z *chatSizes) heldOf(pane string) (heldWindow, bool) {
	z.mu.Lock()
	defer z.mu.Unlock()
	h, ok := z.held[pane]
	return h, ok
}

func (z *chatSizes) hold(pane string, h heldWindow) {
	z.mu.Lock()
	defer z.mu.Unlock()
	if z.held == nil {
		z.held = map[string]heldWindow{}
	}
	z.held[pane] = h
}

func (z *chatSizes) forget(pane string) {
	z.mu.Lock()
	defer z.mu.Unlock()
	delete(z.held, pane)
}

// paneSize is the size of pane where a chat sized its window, and the
// generation it was read at: a screen captured between two reads of the
// same even generation is of that size.
func (z *chatSizes) paneSize(pane string) (cols, rows int, gen uint64) {
	gen = z.gen.Load()
	if h, ok := z.heldOf(pane); ok && gen%2 == 0 {
		return h.paneW, h.paneH, gen
	}
	return 0, 0, gen
}

// watchSize notes the size of cl's watch of pane, and sizes the windows
// it changes. The tmux steps run apart: the act's answer does not wait on
// another act's hold of a pane.
func (s *Service) watchSize(cl *client, pane string, a proto.Act) {
	for _, p := range s.sizes.watch(cl, pane, a.Cols, a.Rows) {
		go s.resize(p)
	}
}

// keyedSize notes a key cl sent: the chat that takes the keys sizes the
// window.
func (s *Service) keyedSize(cl *client, a proto.Act) {
	pane, ok := strings.CutPrefix(a.RecID, proto.KeysPrefix)
	if ok && s.sizes.keyed(cl, pane) {
		go s.resize(pane)
	}
}

// unwatchSizes forgets cl's chats, and puts back or resizes their windows.
func (s *Service) unwatchSizes(cl *client) {
	for _, p := range s.sizes.drop(cl) {
		s.resize(p)
	}
}

// resize gives the window of pane the size of its winning chat, or puts
// it back when no chat watches it. Only a window of one pane is sized:
// another pane of the window would reflow. It runs under the pane's hold,
// so two sizes of one pane do not cross.
func (s *Service) resize(pane string) {
	defer s.holdPane(pane)()
	z := &s.sizes
	want, ok := z.winner(pane)
	held, wasHeld := z.heldOf(pane)
	if !ok {
		if wasHeld {
			s.putBack(pane, held)
		}
		return
	}
	cols := min(max(want.cols, panes.MinCols), panes.MaxCols)
	rows := min(max(want.rows, panes.MinRows), panes.MaxRows)
	if wasHeld && held.cols == cols && held.rows == rows {
		return
	}
	out, err := s.tmuxOut(panes.WindowOf(pane))
	if err != nil {
		return
	}
	f := strings.Split(strings.TrimRight(out, "\n"), "\t")
	if len(f) < 4 || f[1] != "1" {
		if wasHeld { // split since it was sized
			s.putBack(pane, held)
		}
		return
	}
	window, size, mark := f[0], f[2], f[3]
	if wasHeld && held.window != window { // the pane moved to another window
		s.putBack(pane, held)
	}
	server, _ := sample.SplitPane(pane)
	newMark := ""
	if mark == "" {
		prev, err := s.tmuxOut(panes.WindowSizeOf(server, window))
		if err != nil {
			return
		}
		mark = panes.MarkOf(prev, size)
		newMark = mark
	}
	z.gen.Add(1)
	defer z.gen.Add(1)
	out, err = s.tmuxOut(panes.SizeWindow(pane, window, newMark, cols, rows))
	if err != nil { // tmux may have taken part of it: put back what it did
		s.putBack(pane, heldWindow{window: window, mark: mark})
		return
	}
	h := heldWindow{window: window, mark: mark, cols: cols, rows: rows}
	if pw, ph, ok := strings.Cut(strings.TrimSpace(out), " "); ok {
		h.paneW, _ = strconv.Atoi(pw)
		h.paneH, _ = strconv.Atoi(ph)
	}
	z.hold(pane, h)
	s.kickScreens(pane)
}

// putBack restores the window a chat sized for pane.
func (s *Service) putBack(pane string, h heldWindow) {
	server, _ := sample.SplitPane(pane)
	s.sizes.gen.Add(1)
	_, _ = s.tmuxOut(panes.RestoreWindow(server, h.window, h.mark)) // a window gone has nothing to put back
	s.sizes.forget(pane)
	s.sizes.gen.Add(1)
	s.kickScreens(pane)
}

// restoreSized puts back each window a service before this one left
// sized, on the builder's tmux server and on matchblox's own: one that
// ended without its stop.
func (s *Service) restoreSized() {
	servers := []string{""}
	if s.StokerServer != "" {
		servers = append(servers, s.StokerServer)
	}
	for _, server := range servers {
		out, err := s.tmuxOut(panes.SizedWindows(server))
		if err != nil {
			continue // no server: nothing sized
		}
		for _, line := range strings.Split(out, "\n") {
			if window, mark, ok := strings.Cut(line, "\t"); ok && mark != "" {
				_, _ = s.tmuxOut(panes.RestoreWindow(server, window, mark))
			}
		}
	}
}

// unsizeAll puts back every window the chats sized, and sizes none again:
// the service stops.
func (s *Service) unsizeAll() {
	z := &s.sizes
	z.mu.Lock()
	z.closed = true
	var sized []string // the windows held, and those a size is on its way to
	for p := range z.held {
		sized = append(sized, p)
	}
	for p := range z.wants {
		if _, ok := z.held[p]; !ok {
			sized = append(sized, p)
		}
	}
	z.mu.Unlock()
	for _, p := range sized {
		s.resize(p)
	}
}
