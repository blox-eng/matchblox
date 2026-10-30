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
		return panes.Keys(pane, "-l", "--", lit), ""
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
// saw, and for Enter the time it saw the screen: a key never lands on a new
// agent, and Enter never on a question the person did not see. The checks
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
	if a.Text == "Enter" {
		if s.upScreens.has(pane, a.Seen) {
			return proto.Result{Err: "not sent: that screen was " + x.Name + "'s history, not the live screen — look again"}
		}
		if it, queued := s.queued(pane); queued && it.Since.After(a.Seen) {
			return proto.Result{Err: "not sent: " + x.Name + " asks something new — look again"}
		}
	}
	g := advice.Guard{PID: x.PID, StartTicks: x.Start}
	res := s.Actions.Do(ctx, advice.Action{Label: "keys", Steps: [][]string{step}, Guards: []advice.Guard{g}}, "")
	if res.Err == "" && len(res.Ran) > 0 {
		s.undrive(pane) // the pane's last input is the person's, as with a message
	}
	s.kickScreens(pane)
	return res
}
