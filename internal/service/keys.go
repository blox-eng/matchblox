package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
)

const (
	litPrefix = "lit:"
	maxLit    = 200
)

// keyNames are the tmux keys a console may press.
var keyNames = []string{"Up", "Down", "Left", "Right", "Enter", "Tab", "BTab", "Escape", "C-c", "BSpace"}

// keyStep is the tmux step for the wire form of one key, or why it is not one.
func keyStep(pane, text string) ([]string, string) {
	if lit, ok := strings.CutPrefix(text, litPrefix); ok {
		if lit == "" || len(lit) > maxLit || strings.ContainsFunc(lit, unicode.IsControl) {
			return nil, "not sent: the text is empty, over 200 bytes or holds a control character"
		}
		return panes.Literal(pane, lit), ""
	}
	if !slices.Contains(keyNames, text) {
		return nil, fmt.Sprintf("not sent: %q is not a key", cut(text, 20))
	}
	return panes.Keys(pane, text), ""
}

// cut keeps the first n runes of text.
func cut(text string, n int) string {
	if r := []rune(text); len(r) > n {
		return string(r[:n])
	}
	return text
}

// keys sends one key to the agent in pane. The console names the agent it
// saw, and the time it saw the screen: a key never lands on a new agent,
// and a key that answers never on a wait the person did not see. The checks
// run under the pane's lock, so nothing changes between them and the key.
func (s *Service) keys(ctx context.Context, pane string, a proto.Act) proto.Result {
	if !s.aConsole(a.Console, a) {
		return proto.Result{Err: keysOnlyFromConsole}
	}
	s.touchStoker(s.now()) // the builder acts: a stoker that rests goes on resting
	unlock := s.holdPane(pane)
	defer unlock()
	x, ok := s.sessionAt(pane)
	if !ok {
		return proto.Result{Err: "no agent session in " + pane + " any more"}
	}
	if a.PID <= 0 || x.PID != a.PID {
		return proto.Result{Err: "not sent: " + pane + " runs a different agent now"}
	}
	step, why := keyStep(pane, a.Text)
	if why != "" {
		return proto.Result{Err: why}
	}
	if why := s.unseen(pane, x.Name, a, x.Permits || x.Asks); why != "" {
		return proto.Result{Err: why}
	}
	g := advice.Guard{PID: x.PID, StartTicks: x.Start}
	res := s.Actions.Do(ctx, advice.Action{Label: "keys", Steps: [][]string{step}, Guards: []advice.Guard{g}}, "")
	if res.Err == "" && len(res.Ran) > 0 {
		s.undrive(pane) // the pane's last input is the person's, as with a message
	}
	s.kickScreens(pane)
	return res
}

// unseen says why key a would answer a wait in pane that the screen the
// console saw did not show, or "". Enter answers any wait. At a menu (a
// permission prompt or a question, by the hooks or on the screen) a digit
// or any text picks, Esc turns it down, and an arrow or Tab moves it.
func (s *Service) unseen(pane, name string, a proto.Act, menu bool) string {
	it, queued := s.queued(pane)
	menu = menu || queued && (it.State == queue.StatePermission || it.State == queue.StateQuestion)
	if a.Text != "Enter" && !(menu && answers(a.Text)) {
		return ""
	}
	if s.upScreens.has(pane, a.Seen) {
		return "not sent: that screen was " + name + "'s history, not the live screen — look again"
	}
	if queued && it.Since.Add(waitSettle).After(a.Seen) {
		return "not sent: " + name + " asks something new — look again"
	}
	return ""
}

// answers tells if a key answers a menu: all but ^C and Backspace.
func answers(text string) bool {
	return text != "C-c" && text != "BSpace"
}
