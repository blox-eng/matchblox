package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/menu"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
)

// badPick is the refusal of a pick that does not read.
const badPick = "not sent: a pick is a number from 1 to 9, its option's text and its screen's signature"

// choose picks option n of the menu the agent in pane draws, for the
// owner at a console: it types the number. Under the pane's lock it reads
// the screen again. The number goes only while option n says exactly the
// text the console showed, the menu and the lines above it (its question,
// the command it asks about) sign as the console's screen did, and the
// same agent runs. Else the owner looks again.
func (s *Service) choose(ctx context.Context, pane string, a proto.Act) proto.Result {
	if !s.aConsole(a.Console, a) {
		return proto.Result{Err: "not sent: only the console picks from a menu"}
	}
	if a.Confirm != "y" {
		return proto.Result{Err: "needs confirm"}
	}
	parts := strings.Split(a.Text, "\x00")
	if len(parts) != 3 {
		return proto.Result{Err: badPick}
	}
	digit, text, sign := parts[0], parts[1], parts[2]
	if n, err := strconv.Atoi(digit); err != nil || len(digit) != 1 || n < 1 || text == "" || sign == "" {
		return proto.Result{Err: badPick}
	}
	n, _ := strconv.Atoi(digit)
	unlock := s.holdPane(pane)
	defer unlock()
	x, ok := s.sessionAt(pane)
	if !ok {
		return proto.Result{Err: "no agent session in " + pane + " any more"}
	}
	if !sameAgent(x, a) {
		return proto.Result{Err: "not sent: " + pane + " runs a different agent now"}
	}
	screen, read := s.Capture([]string{pane})[pane]
	if !read {
		return proto.Result{Err: "not sent: the pane " + pane + " cannot be read"}
	}
	options := menu.Read(screen)
	if options == nil {
		return proto.Result{Err: "not sent: " + x.Name + " shows no menu"}
	}
	if options[n] != text || menu.Sign(screen) != sign {
		return proto.Result{Err: "not sent: " + x.Name + "'s menu changed — look again"}
	}
	g := advice.Guard{PID: x.PID, StartTicks: x.Start}
	res := s.Actions.Do(ctx, advice.Action{Label: "choose", Steps: [][]string{panes.Literal(pane, digit)}, Guards: []advice.Guard{g}}, "")
	if res.Err == "" && len(res.Ran) > 0 {
		s.undrive(pane) // the pane's last input is the owner's, as with a message
	}
	s.kickScreens(pane)
	for _, after := range keyRereads {
		time.AfterFunc(after, func() { s.kickScreens(pane) })
	}
	return res
}
