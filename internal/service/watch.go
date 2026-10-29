package service

import (
	"context"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
)

const watchEvery = 150 * time.Millisecond

// watchAct sets the pane cl streams; an empty pane stops the stream.
func (s *Service) watchAct(cl *client, clientKind string, a proto.Act, pane string) proto.Result {
	if clientKind != proto.ClientConsole || s.agentsPane(a) {
		return proto.Result{Err: "keys go only from a console"}
	}
	cl.watch.Store(&pane)
	select {
	case cl.screenKick <- struct{}{}:
	default:
	}
	return proto.Result{}
}

// watchLoop sends cl the screen of its watched pane when the text changes,
// and once when the pane is gone.
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
		p := cl.watch.Load()
		if p == nil || *p == "" {
			sent = false
			continue
		}
		pane := *p
		scr := proto.Screen{Pane: pane, At: s.now()}
		text, ok := s.Capture([]string{pane})[pane]
		scr.Text, scr.Gone = text, !ok
		if sent && pane == last.Pane && text == last.Text && scr.Gone == last.Gone {
			continue
		}
		sent, last = true, scr
		if cl.c.Send(proto.KindScreen, "", scr) != nil {
			cl.c.Close()
			return
		}
	}
}

// kickScreens wakes the watchLoop of every console that watches pane.
func (s *Service) kickScreens(pane string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for cl := range s.clients {
		if p := cl.watch.Load(); p != nil && *p == pane {
			select {
			case cl.screenKick <- struct{}{}:
			default:
			}
		}
	}
}
