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

const (
	keysOnlyInDemo       = "not sent: keys go to an agent in its own pane"
	watchOnlyFromConsole = "screens go only to a console"
)

// aConsole tells if the act comes from a person at a console: not from an
// agent's pane, nor the stoker's.
func (s *Service) aConsole(console bool, a proto.Act) bool {
	_, inAgent := s.sessionAt(a.From)
	_, own := s.unitOf(a)
	return console && !inAgent && !s.agentsPane(a) && !own
}

// watched is the pane a console streams, and where in its history: back
// lines up from the live screen, in a pane rows high.
type watched struct {
	pane       string
	back, rows int
}

// scrollStep is the lines one wheel step moves through a pane's history.
const scrollStep = 3

const backToLive = "not sent: back to the live screen — press it again to send"

// scrollKey handles a console's key for the pane it watches while it scrolls
// back: a wheel step moves through the
// history, and any other key only comes back to the live screen, so it never
// answers a prompt the person did not see. handled is false for every other
// key, which goes on to the agent.
func (s *Service) scrollKey(cl *client, clientKind string, a proto.Act, pane string) (res proto.Result, handled bool) {
	w := cl.watch.Load()
	if w == nil || w.pane != pane || !s.aConsole(clientKind == proto.ClientConsole, a) {
		return proto.Result{}, false
	}
	if a.Text != proto.WheelUp && a.Text != proto.WheelDown {
		if w.back == 0 || scrolls(a.Text) {
			return proto.Result{}, false
		}
		s.watchAt(cl, &watched{pane: pane})
		return proto.Result{Err: backToLive}, true
	}
	if s.PaneHistory == nil || s.CaptureBack == nil {
		return proto.Result{}, false
	}
	hist, rows, ok := s.PaneHistory(pane)
	if !ok {
		return proto.Result{Err: "not scrolled: could not read " + pane + "'s history: tmux did not answer"}, true
	}
	back := w.back - scrollStep
	if a.Text == proto.WheelUp {
		back = w.back + scrollStep
	}
	back = min(max(back, 0), hist)
	if back != w.back {
		s.watchAt(cl, &watched{pane: pane, back: back, rows: rows})
	}
	return proto.Result{}, true
}

// watchAt sets what cl streams and wakes its loop.
func (s *Service) watchAt(cl *client, w *watched) {
	cl.watch.Store(w)
	select {
	case cl.screenKick <- struct{}{}:
	default:
	}
}

// watchAct sets the pane cl streams; an empty pane stops the stream.
func (s *Service) watchAct(cl *client, clientKind string, a proto.Act, pane string) proto.Result {
	if !s.aConsole(clientKind == proto.ClientConsole, a) {
		return proto.Result{Err: watchOnlyFromConsole}
	}
	if _, ok := s.sessionAt(pane); pane != "" && !ok {
		return proto.Result{Err: "no agent session in " + pane + " any more"}
	}
	cl.watch.Store(&watched{pane: pane})
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
			switch {
			case w.back > 0:
				text, read = s.CaptureBack(pane, w.back, w.rows)
				scr.Back = w.back
			case s.ScreenOf != nil:
				text, scr.Alt, read = s.ScreenOf(pane)
			default:
				text, read = s.Capture([]string{pane})[pane]
			}
		}
		scr.Text, scr.Gone = text, !live // only a pane with no session in it any more is gone
		if live && !read {
			scr.Err = "could not read " + pane + ": tmux did not answer"
		}
		if sent && pane == last.Pane && text == last.Text && scr.Gone == last.Gone && scr.Err == last.Err && scr.Alt == last.Alt && scr.Back == last.Back && (s.waitShown(pane, last.At) || !s.waitShown(pane, scr.At)) {
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
