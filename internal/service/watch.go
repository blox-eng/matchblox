package service

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
)

const watchEvery = 150 * time.Millisecond

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
// and once when the pane is gone. A wait that began after the last screen
// sends it again, newer: the console's Enter answers the screen it saw,
// and a still prompt is still the one the builder sees. A watch up the
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
		scr := proto.Screen{Pane: pane, At: s.now()}
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
		if sent && pane == last.Pane && text == last.Text && scr.Gone == last.Gone && scr.Up == last.Up && !s.waitsSince(pane, last.At) {
			continue
		}
		sent, last = true, scr
		if scr.Up > 0 {
			s.upScreens.add(pane, scr.At)
		}
		if cl.c.Send(proto.KindScreen, "", scr) != nil {
			cl.c.Close()
			return
		}
	}
}

// waitsSince tells if the agent in pane began to wait after at.
func (s *Service) waitsSince(pane string, at time.Time) bool {
	it, ok := s.queued(pane)
	return ok && it.Since.After(at)
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

// keptUp is how many screens of the history upScreens keeps for each pane:
// a console's Enter answers the last screen it got.
const keptUp = 32

// upScreens are the times of the screens of the history sent, by pane.
type upScreens struct {
	mu sync.Mutex
	at map[string][]time.Time
}

func (u *upScreens) add(pane string, at time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.at == nil {
		u.at = map[string][]time.Time{}
	}
	kept := append(u.at[pane], at)
	u.at[pane] = kept[max(len(kept)-keptUp, 0):]
}

// has tells if the screen of pane taken at at was one of the history.
func (u *upScreens) has(pane string, at time.Time) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return slices.ContainsFunc(u.at[pane], at.Equal)
}
