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

// chooseSettle is how long a pick holds the pane after its digit, while
// the agent draws what comes next.
var chooseSettle = time.Second

// liveScreen reads pane's live screen as a console's watch reads it
// (ScreenOf joins the lines tmux wrapped), so a pick signs what the
// console showed.
func (s *Service) liveScreen(pane string) (string, bool) {
	if s.ScreenOf != nil {
		text, _, ok := s.ScreenOf(pane)
		return text, ok
	}
	text, ok := s.Capture([]string{pane})[pane]
	return text, ok
}

// awaitRedraw waits, for up to chooseSettle, until pane no longer shows
// the menu signed sign. The caller holds the pane: a second console's pick
// on the same menu then reads the agent's next screen, never a digit that
// lands on a prompt its owner did not see.
func (s *Service) awaitRedraw(ctx context.Context, pane, sign string) {
	for deadline := time.Now().Add(chooseSettle); time.Now().Before(deadline); {
		if screen, ok := s.liveScreen(pane); !ok || menu.Sign(screen) != sign {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(25 * time.Millisecond):
		}
	}
}

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
	if x.UnknownScreen {
		return proto.Result{Err: unknownScreenRefusal}
	}
	screen, read := s.liveScreen(pane)
	if !read {
		return proto.Result{Err: "not sent: the pane " + pane + " cannot be read"}
	}
	options := menu.Read(screen)
	if options == nil {
		return proto.Result{Err: "not sent: " + x.Name + " shows no menu — answer it in its pane"}
	}
	if options[n] != text || menu.Sign(screen) != sign {
		return proto.Result{Err: "not sent: " + x.Name + "'s menu changed — look again"}
	}
	g := advice.Guard{PID: x.PID, StartTicks: x.Start}
	res := s.Actions.Do(ctx, advice.Action{Label: "choose", Steps: [][]string{panes.Literal(pane, digit)}, Guards: []advice.Guard{g}}, "")
	if res.Err == "" && len(res.Ran) > 0 {
		s.awaitRedraw(ctx, pane, sign) // still under the lock: no other pick reads this menu
	}
	s.kickScreens(pane)
	if res.Err == "" {
		s.rereadAfter(pane)
	}
	return res
}
