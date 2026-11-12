package app

import (
	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/queue"
)

// feelOf is the haptic a new queue calls for on the phone: needs when a
// match starts to ask, else done when one finishes, else none. One feel a
// sample, however many matches changed; busy and idle churn feels nothing.
func feelOf(was, now []queue.Item) string {
	had := map[string]string{}
	for _, it := range was {
		had[it.Pane] = it.State
	}
	feel := ""
	for _, it := range now {
		if it.Pane == "" || had[it.Pane] == it.State {
			continue
		}
		switch it.State {
		case queue.StatePermission, queue.StateQuestion:
			return keycmd.FeelNeeds
		case queue.StateFinished:
			feel = keycmd.FeelDone
		}
	}
	return feel
}
