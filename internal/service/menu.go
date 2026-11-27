package service

import (
	"github.com/blox-eng/matchblox/internal/menu"
	"github.com/blox-eng/matchblox/internal/proto"
)

// readMenu answers a menu: act for the owner at a console: the menu the
// agent in pane draws on its live screen, the option it marks and the
// screen's sign, as choose reads them. It types nothing.
func (s *Service) readMenu(pane string, a proto.Act) proto.Result {
	if !s.aConsole(a.Console, a) {
		return proto.Result{Err: "not read: only the console reads a menu"}
	}
	if _, ok := s.sessionAt(pane); !ok {
		return proto.Result{Err: "no agent session in " + pane + " any more"}
	}
	screen, ok := s.liveScreen(pane)
	if !ok {
		return proto.Result{Err: "not read: the pane " + pane + " cannot be read"}
	}
	options := menu.Read(screen)
	m := &proto.Menu{Chosen: menu.Chosen(screen), Sign: menu.Sign(screen)}
	for n := 1; n <= len(options); n++ {
		m.Options = append(m.Options, proto.Option{N: n, Text: options[n]})
	}
	return proto.Result{Menu: m}
}
