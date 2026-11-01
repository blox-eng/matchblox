package service

import (
	"context"
	"strconv"
	"strings"
	"sync"
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

// watched is the pane a console streams, and how many lines up from its
// bottom: 0 is the live screen.
type watched struct {
	pane string
	up   int
}

// upOf is the offset of a watch act's text: "" or "up:<n>".
func upOf(text string) (int, bool) {
	if text == "" {
		return 0, true
	}
	n, err := strconv.Atoi(strings.TrimPrefix(text, proto.UpPrefix))
	return n, err == nil && strings.HasPrefix(text, proto.UpPrefix) && n >= 0 && n <= proto.MaxUp
}

// watchAct sets the pane cl streams; an empty pane stops the stream.
func (s *Service) watchAct(cl *client, clientKind string, a proto.Act, pane string) proto.Result {
	if !s.aConsole(clientKind == proto.ClientConsole, a) {
		return proto.Result{Err: keysOnlyFromConsole}
	}
	up, ok := upOf(a.Text)
	if !ok {
		return proto.Result{Err: "not watched: " + strconv.Quote(cut(a.Text, 20)) + " is not up:0 to up:" + strconv.Itoa(proto.MaxUp)}
	}
	if _, ok := s.sessionAt(pane); pane != "" && !ok {
		return proto.Result{Err: "no agent session in " + pane + " any more"}
	}
	cl.watch.Store(&watched{pane: pane, up: up})
	select {
	case cl.screenKick <- struct{}{}:
	default:
	}
	return proto.Result{}
}

// watchLoop sends cl the screen of its watched pane when the text changes,
// and once when the pane is gone. A wait that the last screen does not
// show sends it again, newer, once the wait has settled: the console's
// Enter answers the screen it saw, and a still prompt is still the one the
// builder sees. A watch up the
// history sends those lines, and no Enter answers them.
func (s *Service) watchLoop(ctx context.Context, cl *client) {
	tick := time.NewTicker(watchEvery)
	defer tick.Stop()
	var last proto.Screen
	var sent bool
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
		pane := w.pane
		// Stamped before the capture: a wait that begins during it is newer.
		scr := proto.Screen{Pane: pane, At: s.upScreens.stamp(pane, s.now, w.up > 0)}
		var text string
		_, live := s.sessionAt(pane)
		switch {
		case live && w.up > 0:
			text, scr.Up, live = s.CaptureUp(pane, w.up)
		case live:
			text, live = s.Capture([]string{pane})[pane]
		}
		scr.Text, scr.Gone = text, !live
		if scr.Gone {
			scr.Up = 0
		}
		if sent && pane == last.Pane && text == last.Text && scr.Gone == last.Gone && scr.Up == last.Up && (s.waitShown(pane, last.At) || !s.waitShown(pane, scr.At)) {
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

// upScreens are, for each pane, the latest run of screens of the history:
// those taken after the last live screen before them. It is one span of
// times, so it never grows; an older run is forgotten, and the queued
// question's Since stays the backstop for an Enter seen on one.
type upScreens struct {
	mu sync.Mutex
	at map[string]upSpan
}

// upSpan: the screens taken after from, up to to, were of the history;
// live is the last live screen taken.
type upSpan struct{ from, to, live time.Time }

// stamp is the time of a screen of pane taken now, of the history when up.
// It is taken and noted under one lock, and the stamps of a pane rise, so a
// live screen of one console is never inside another console's run of the
// history.
func (u *upScreens) stamp(pane string, now func() time.Time, up bool) time.Time {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.at == nil {
		u.at = map[string]upSpan{}
	}
	sp := u.at[pane]
	at := now()
	if last := maxTime(sp.to, sp.live); !at.After(last) {
		at = last.Add(time.Nanosecond)
	}
	switch {
	case !up:
		sp.live = at
	case sp.to.IsZero() || sp.live.After(sp.to): // the first of a new run
		sp.from, sp.to = sp.live, at
	default:
		sp.to = at
	}
	u.at[pane] = sp
	return at
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// has tells if the screen of pane taken at at was one of the latest run of
// the history.
func (u *upScreens) has(pane string, at time.Time) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	sp, ok := u.at[pane]
	return ok && !sp.to.IsZero() && at.After(sp.from) && !at.After(sp.to)
}
