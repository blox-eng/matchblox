// Package queue is the list of sessions that wait for the person, oldest
// first. Hook events are the truth; a session that never sent one is
// guessed from what the sampler reads, and marked as estimated.
package queue

import (
	"path/filepath"
	"sort"
	"time"

	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/said"
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

// AskLife is how long a question tool's call from a hook is kept with no
// hook that ends it: a session that died mid-menu sends none.
const AskLife = 24 * time.Hour

// hookAsk is the question tool's call a PreToolUse hook announced.
type hookAsk struct {
	ask said.Ask
	at  time.Time
}

// Queue is not safe for concurrent use; the service holds its lock.
type Queue struct {
	hooked  map[string]hooks.Event // session id -> last event, for each session that sent one
	waiting map[string]Item        // session id -> item, from hooks
	seen    map[string]time.Time   // session id -> last time a hook or the sampler showed it
	asks    map[string]hookAsk     // session id -> the question tool's call its menu waits on
	items   []Item
}

func New() *Queue {
	return &Queue{hooked: map[string]hooks.Event{}, waiting: map[string]Item{}, seen: map[string]time.Time{}, asks: map[string]hookAsk{}}
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
	case "PreToolUse":
		if ev.Ask != nil {
			q.asks[ev.SessionID] = hookAsk{ask: *ev.Ask, at: ev.At}
		}
	case "PostToolUse":
		if a, ok := q.asks[ev.SessionID]; ok && a.ask.ID == ev.ToolUseID {
			delete(q.asks, ev.SessionID)
		}
	case "UserPromptSubmit", "Stop", "SessionStart", "SessionEnd":
		delete(q.asks, ev.SessionID) // the menu was answered or left
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

// Pane is the pane of a hooked session, "" when none is known.
func (q *Queue) Pane(id string) string {
	return q.hooked[id].Pane
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
			if a, ok := q.asks[id]; ok && s.TurnEnded.After(a.at) {
				// A turn ends only once its menu is gone: answered, or left
				// with Esc, which sends no hook.
				delete(q.asks, id)
			}
			if it, ok := q.waiting[id]; ok {
				it.Pane, it.Target, it.Name = s.Pane, s.Target, s.Name
				q.waiting[id] = it
				if s.Busy && (it.State == StateFinished || it.Reminder) {
					// A turn that ended asking nothing does not wait while the
					// session works: its subagents, or the turn their results
					// start with no prompt. It is back once the work ends.
					continue
				}
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
			delete(q.asks, id)
		}
	}
	for id, a := range q.asks {
		if now.Sub(a.at) > AskLife {
			delete(q.asks, id)
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

// WithAsks is sessions with the question tool's call each one waits on.
// The transcript's call stays when the hooks know none or the same one. A
// hook's call fills a session whose transcript shows none, as in auto mode,
// where Claude Code writes the call only once it is answered; when the two
// differ the hook's is the newer, sent before the menu draws. sessions is
// not changed: a changed list is a copy.
func (q *Queue) WithAsks(sessions []sample.Session) []sample.Session {
	if len(q.asks) == 0 {
		return sessions
	}
	byPane := map[string]string{} // pane -> session id with a call
	for id := range q.asks {
		if p := q.hooked[id].Pane; p != "" {
			byPane[p] = id
		}
	}
	out, copied := sessions, false
	for i, s := range sessions {
		id := s.SessionID
		if _, ok := q.hooked[id]; !ok || id == "" {
			id = byPane[s.Pane]
		}
		a, ok := q.asks[id]
		if !ok || s.Ask != nil && s.Ask.ID == a.ask.ID {
			continue
		}
		if !copied {
			out, copied = append([]sample.Session(nil), sessions...), true
		}
		ask := a.ask
		out[i].Ask = &ask
	}
	return out
}
