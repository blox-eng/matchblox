package stoker

import (
	"strings"

	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

// Peek is what the console's peek shows of a session: what it said last
// and what it asks, if anything. `matchblox peek` prints it, and the wakes
// carry its first line.
type Peek struct {
	Name   string `json:"name"`
	Pane   string `json:"pane"`
	Target string `json:"target,omitempty"`
	// Said is this turn's reply, or the line read last from the pane.
	Said string `json:"said"`
	// Asks is the first line of what it asks; "" when it asks nothing.
	Asks      string         `json:"asks,omitempty"`
	Questions []PeekQuestion `json:"questions,omitempty"`
	// Approval: an approval menu waits, answered in the session's pane.
	Approval bool `json:"approval,omitempty"`
}

// PeekQuestion is one question of the question tool, with its options.
type PeekQuestion struct {
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
}

// wakeWidth bounds a line of a wake taken from what a session said.
const wakeWidth = 120

// PeekOf reads a session's peek from the sample and the queue, as the
// console does.
func PeekOf(s sample.Session, items []queue.Item) Peek {
	var it queue.Item
	for _, x := range items {
		if s.Pane != "" && x.Pane == s.Pane || s.Pane == "" && s.SessionID != "" && x.SessionID == s.SessionID {
			it = x
			break
		}
	}
	p := Peek{Name: s.Name, Pane: s.Pane, Target: s.Target, Said: s.Said}
	words := it.LastLine
	if words == "" {
		words = s.LastLine
	}
	p.Approval = s.Ask == nil && (s.Permits || it.State == queue.StatePermission)
	if p.Said == "" && s.Ask == nil || p.Approval {
		p.Said = words
	}
	switch {
	case s.Ask != nil && len(s.Ask.Questions) > 0:
		for _, q := range s.Ask.Questions {
			pq := PeekQuestion{Question: q.Question}
			for _, o := range q.Options {
				label := o.Label
				if o.Description != "" && o.Description != o.Label {
					label += ": " + o.Description
				}
				pq.Options = append(pq.Options, label)
			}
			p.Questions = append(p.Questions, pq)
		}
		p.Asks = firstLine(s.Ask.Questions[0].Question)
	case p.Approval:
		p.Asks = firstLine(words)
		if p.Asks == "" {
			p.Asks = "an approval"
		}
	case s.Asks || it.State == queue.StateQuestion && !it.Reminder:
		p.Asks = lastLine(p.Said)
	}
	return p
}

// firstLine is the first line of text with words, cut to a wake's width.
func firstLine(text string) string {
	for l := range strings.SplitSeq(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return cutLine(l)
		}
	}
	return ""
}

// lastLine is the last line of text with words, cut to a wake's width.
func lastLine(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return cutLine(l)
		}
	}
	return ""
}

func cutLine(l string) string {
	if r := []rune(l); len(r) > wakeWidth {
		return string(r[:wakeWidth-1]) + "…"
	}
	return l
}
