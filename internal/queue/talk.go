package queue

import (
	"slices"
	"time"

	"github.com/blox-eng/matchblox/internal/hooks"
)

// Turn is one prompt ("user") or reply ("assistant") of a Cursor session.
type Turn struct{ Role, Text string }

// TalkLimit is how many bytes of text a session's talk keeps in all.
const TalkLimit = 96_000

// talk is a session's recent turns, in memory only: Cursor keeps no
// transcript a capture can read. It keeps the first prompt, and the newest
// turns after it within TalkLimit.
type talk struct {
	first Turn
	turns []Turn
	size  int
	at    time.Time // the last event that fed it
}

// hear adds the text of a prompt or a reply to its session's talk.
func (q *Queue) hear(ev hooks.Event) {
	role := "user"
	switch {
	case ev.Text == "":
		return
	case ev.Name == hooks.Reply:
		role = "assistant"
	case ev.Name != "UserPromptSubmit":
		return
	}
	t := q.talks[ev.SessionID]
	if t == nil {
		t = &talk{}
		q.talks[ev.SessionID] = t
	}
	turn := Turn{Role: role, Text: ev.Text}
	t.size += len(turn.Text)
	if t.first.Text == "" && role == "user" {
		t.first = turn
	} else {
		t.turns = append(t.turns, turn)
	}
	for t.size > TalkLimit && len(t.turns) > 0 {
		t.size -= len(t.turns[0].Text)
		t.turns = slices.Delete(t.turns, 0, 1)
	}
	if ev.At.After(t.at) {
		t.at = ev.At
	}
}

// Talk is the session's first prompt, then its newest turns, oldest first.
func (q *Queue) Talk(id string) []Turn {
	t := q.talks[id]
	if t == nil {
		return nil
	}
	var turns []Turn
	if t.first.Text != "" {
		turns = append(turns, t.first)
	}
	return append(turns, t.turns...)
}
