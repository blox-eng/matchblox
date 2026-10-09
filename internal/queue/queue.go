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
	// FromPane: an agent without hooks, read from its pane. It waits when
	// it is idle and its last lines ask; that can be later than a hook.
	FromPane bool `json:"from_pane,omitempty"`
	// Reminder: the question is Claude Code's idle_prompt after a turn that
	// asked nothing. It waits for the person, but nothing asks.
	Reminder bool `json:"reminder,omitempty"`
}

// UnseenGrace is how long a hooked session the sampler does not see yet
// keeps its events: the sampler can lag behind a hook.
const UnseenGrace = 30 * time.Second

// Queue is not safe for concurrent use; the service holds its lock.
type Queue struct {
	hooked  map[string]hooks.Event // session id -> last event, for each session that sent one
	waiting map[string]Item        // session id -> item, from hooks
	seen    map[string]time.Time   // session id -> last time a hook or the sampler showed it
	items   []Item
}

func New() *Queue {
	return &Queue{hooked: map[string]hooks.Event{}, waiting: map[string]Item{}, seen: map[string]time.Time{}}
}

// Apply takes one hook event. Call Merge after it to update Items.
func (q *Queue) Apply(ev hooks.Event) {
	if ev.SessionID == "" {
		return
	}
	prev, known := q.hooked[ev.SessionID]
	if known && ev.At.Before(prev.At) {
		return // a spooled event older than what the queue knows
	}
	if ev.Pane == "" {
		ev.Pane = prev.Pane
	}
	q.hooked[ev.SessionID] = ev
	if ev.At.After(q.seen[ev.SessionID]) {
		q.seen[ev.SessionID] = ev.At
	}
	it, was := q.waiting[ev.SessionID]
	set := func(state, line string) {
		if !was {
			it = Item{SessionID: ev.SessionID, Since: ev.At}
		}
		it.State, it.Pane, it.Reminder = state, ev.Pane, false
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
			asked := was && (it.State == StatePermission || it.State == StateQuestion && !it.Reminder)
			set(StateQuestion, line)
			it.Reminder = !asked
			q.waiting[ev.SessionID] = it
		}
	case "Stop":
		if ev.Asks {
			set(StateQuestion, ev.Message)
		} else {
			set(StateFinished, ev.Message)
		}
	case "PostToolUse":
		// A tool ran: the person allowed it in the pane.
		if was && it.State == StatePermission {
			delete(q.waiting, ev.SessionID)
		}
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
			q.seen[id] = now
			if it, ok := q.waiting[id]; ok {
				it.Pane, it.Target, it.Name = s.Pane, s.Target, s.Name
				q.waiting[id] = it
				items = append(items, it)
			}
			continue
		}
		if s.Status == "idle" && !s.Busy {
			state := StateFinished
			switch {
			case s.Permits:
				state = StatePermission // answered in its pane: a typed line would approve it
			case s.Asks:
				state = StateQuestion
			}
			items = append(items, Item{
				SessionID: s.SessionID, Pane: s.Pane, Target: s.Target, Name: s.Name,
				State: state, Since: now.Add(-s.Idle), LastLine: s.LastLine, Estimated: true, FromPane: s.FromPane,
			})
		}
	}
	for id := range q.hooked {
		if !seen[id] && now.Sub(q.seen[id]) > UnseenGrace {
			delete(q.hooked, id)
			delete(q.waiting, id)
			delete(q.seen, id)
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
