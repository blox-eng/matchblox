package service

import (
	"context"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
)

const watchEvery = 150 * time.Millisecond

// waitSettle is how long after a wait begins a screen counts as showing it.
// The hook that starts the wait fires before the agent draws its menu, so a
// screen taken in between, changed or not, may be the one before the menu.
const waitSettle = 500 * time.Millisecond

const keysOnlyFromConsole = "keys go only from a console"

// aConsole tells if the act comes from a person at a console: not from an
// agent's pane, nor the stoker's.
func (s *Service) aConsole(console bool, a proto.Act) bool {
	_, inAgent := s.sessionAt(a.From)
	return console && !inAgent && !s.agentsPane(a) && !s.fromStoker(a)
}

// watched is the pane a console streams.
type watched struct{ pane string }

// watchAct sets the pane cl streams; an empty pane stops the stream.
func (s *Service) watchAct(cl *client, clientKind string, a proto.Act, pane string) proto.Result {
	if !s.aConsole(clientKind == proto.ClientConsole, a) {
		return proto.Result{Err: keysOnlyFromConsole}
	}
	if _, ok := s.sessionAt(pane); pane != "" && !ok {
		return proto.Result{Err: "no agent session in " + pane + " any more"}
	}
	cl.watch.Store(&watched{pane: pane})
	s.watchSize(cl, pane, a)
	select {
	case cl.screenKick <- struct{}{}:
	default:
	}
	return proto.Result{}
}

// watchLoop sends cl the screen of its watched pane when the text changes,
// once when the pane is gone, and once for each watch act. A wait that the
// last screen does not show sends it again, newer, once the wait has
// settled: the console's Enter answers the screen it saw, and a still prompt
// is still the one the builder sees.
func (s *Service) watchLoop(ctx context.Context, cl *client) {
	tick := time.NewTicker(watchEvery)
	defer tick.Stop()
	var last proto.Screen
	var sent bool
	var lastW *watched
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-cl.screenKick:
		}
		w := cl.watch.Load()
		if w == nil || w.pane == "" {
			sent = false
			continue
		}
		if w != lastW {
			sent, lastW = false, w // each watch act gets a screen back, the same text or not
		}
		pane := w.pane
		// Stamped before the capture: a wait that begins during it is newer.
		scr := proto.Screen{Pane: pane, At: s.now()}
		var text string
		read := false
		_, live := s.sessionAt(pane)
		if live {
			cols, rows, gen := s.sizes.paneSize(pane)
			text, read = s.Capture([]string{pane})[pane]
			if _, _, after := s.sizes.paneSize(pane); after == gen { // not sized while it was read
				scr.Cols, scr.Rows = cols, rows
			}
		}
		scr.Text, scr.Gone = text, !live // only a pane with no session in it any more is gone
		if live && !read {
			scr.Err = "could not read " + pane + ": tmux did not answer"
		}
		if sent && pane == last.Pane && text == last.Text && scr.Gone == last.Gone && scr.Err == last.Err && scr.Cols == last.Cols && scr.Rows == last.Rows && (s.waitShown(pane, last.At) || !s.waitShown(pane, scr.At)) {
			continue
		}
		sent, last = true, scr
		if cl.c.Send(proto.KindScreen, "", scr) != nil {
			cl.c.Close()
			return
		}
	}
}

// waitShown tells if a screen of pane taken at at shows its wait, if any:
// it was taken a settle after the wait began.
func (s *Service) waitShown(pane string, at time.Time) bool {
	it, ok := s.queued(pane)
	return !ok || !it.Since.Add(waitSettle).After(at)
}

// kickScreens wakes the watchLoop of every console that watches pane.
func (s *Service) kickScreens(pane string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for cl := range s.clients {
		if w := cl.watch.Load(); w != nil && w.pane == pane {
			select {
			case cl.screenKick <- struct{}{}:
			default:
			}
		}
	}
}
