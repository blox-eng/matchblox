// Package queue is the list of sessions that wait for the person, oldest
// first. Hook events are the truth; a session that never sent one is
// guessed from what the sampler reads, and marked as estimated.
package queue

import (
	"path/filepath"
	"sort"
	"time"

	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/sample"
)

const (
	StatePermission = "permission" // asks for permission
	StateQuestion   = "question"   // waits for input
	StateFinished   = "finished"   // finished its turn
)

type Item struct {
	SessionID string    `json:"session_id"`
	Pane      string    `json:"pane"`
	Target    string    `json:"target"`
	Name      string    `json:"name"`
	State     string    `json:"state"`
	Since     time.Time `json:"since"`
	LastLine  string    `json:"last_line,omitempty"`
	Estimated bool      `json:"estimated,omitempty"`
}

// Queue is not safe for concurrent use; the service holds its lock.
type Queue struct {
	hooked  map[string]hooks.Event // session id -> last event, for each session that sent one
	waiting map[string]Item        // session id -> item, from hooks
	items   []Item
}

func New() *Queue {
	return &Queue{hooked: map[string]hooks.Event{}, waiting: map[string]Item{}}
}

// Apply takes one hook event. Call Merge after it to update Items.
func (q *Queue) Apply(ev hooks.Event) {
	if ev.SessionID == "" {
		return
	}
	if ev.Pane == "" {
		ev.Pane = q.hooked[ev.SessionID].Pane
	}
	q.hooked[ev.SessionID] = ev
	it, was := q.waiting[ev.SessionID]
	set := func(state, line string) {
		if !was {
			it = Item{SessionID: ev.SessionID, Since: ev.At}
		}
		it.State, it.Pane = state, ev.Pane
		if line != "" {
			it.LastLine = line
		}
		if it.Name == "" && ev.Cwd != "" {
			it.Name = filepath.Base(ev.Cwd)
		}
		q.waiting[ev.SessionID] = it
	}
	switch ev.Name {
	case "Notification":
		switch ev.Kind {
		case hooks.KindPermission:
			set(StatePermission, ev.Message)
		case hooks.KindIdlePrompt:
			line := ""
			if !was || it.LastLine == "" {
				line = ev.Message
			}
			set(StateQuestion, line)
		}
	case "Stop":
		set(StateFinished, ev.Message)
	case "UserPromptSubmit", "SessionStart", "SessionEnd":
		// An ended session stays hooked until the sampler stops seeing it,
		// so it is never guessed back into the queue.
		delete(q.waiting, ev.SessionID)
	}
}

// Merge joins the queue with the sessions the sampler sees: it fills the
// pane and name, drops sessions that are gone, and guesses the sessions
// that never sent a hook event.
func (q *Queue) Merge(sessions []sample.Session, now time.Time) {
	byPane := map[string]string{} // pane -> hooked session id
	for id, ev := range q.hooked {
		if ev.Pane != "" {
			byPane[ev.Pane] = id
		}
	}
	seen := map[string]bool{}
	var items []Item
	for _, s := range sessions {
		id := s.SessionID
		if _, ok := q.hooked[id]; !ok || id == "" {
			id = byPane[s.Pane]
		}
		if id != "" {
			seen[id] = true
			if it, ok := q.waiting[id]; ok {
				it.Pane, it.Target, it.Name = s.Pane, s.Target, s.Name
				q.waiting[id] = it
				items = append(items, it)
			}
			continue
		}
		if s.Status == "idle" && !s.Busy {
			items = append(items, Item{
				SessionID: s.SessionID, Pane: s.Pane, Target: s.Target, Name: s.Name,
				State: StateFinished, Since: now.Add(-s.Idle), Estimated: true,
			})
		}
	}
	for id := range q.hooked {
		if !seen[id] {
			delete(q.hooked, id)
			delete(q.waiting, id)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].Since.Equal(items[j].Since) {
			return items[i].Since.Before(items[j].Since)
		}
		return items[i].Name < items[j].Name
	})
	q.items = items
}

func (q *Queue) Items() []Item { return append([]Item(nil), q.items...) }
