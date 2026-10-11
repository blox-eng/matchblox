package stoker

import (
	"fmt"
	"slices"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
)

// Seat is a session the score counts.
type Seat struct {
	Session string `json:"session"`
	Name    string `json:"name,omitempty"`
}

// Card is the score of a run that ended: what the stoker watched and did,
// and how many of the sessions at work when it began are ready to go on.
type Card struct {
	Night     bool      `json:"night,omitempty"`
	Since     time.Time `json:"since"`
	Ended     time.Time `json:"ended"`
	Watched   int       `json:"watched"`
	Full      int       `json:"full"`
	Compacted int       `json:"compacted"`
	Waited    int       `json:"waited"`
	Given     int       `json:"given"` // context tokens the compacts gave back
	Ready     int       `json:"ready"`
	Of        int       `json:"of"`
	NotReady  []Miss    `json:"not_ready,omitempty"`
}

// Miss is a session of the start that is not ready, and why.
type Miss struct {
	Name string `json:"name"`
	Why  string `json:"why"`
}

// atWork is a session the score follows: an agent with a session, not cold.
func atWork(s sample.Session) bool {
	return s.SessionID != "" && s.Age() != "cold" && s.Age() != "stale"
}

// Begin seats the sessions at work when the run starts.
func (m Mode) Begin(ss []sample.Session) Mode {
	m.Start, m.Watched = nil, nil
	for _, s := range ss {
		if atWork(s) {
			m.Start = append(m.Start, Seat{s.SessionID, s.Name})
			m.Watched = append(m.Watched, s.SessionID)
		}
	}
	return m
}

// Observe adds what a sample shows to the run; changed tells if the mode
// needs writing. waiting holds the panes the queue lists as waiting.
func (m Mode) Observe(ss []sample.Session, waiting map[string]bool, compactAt float64) (Mode, bool) {
	if !m.On {
		return m, false
	}
	return m.observe(ss, waiting, compactAt)
}

func (m Mode) observe(ss []sample.Session, waiting map[string]bool, compactAt float64) (Mode, bool) {
	changed := false
	add := func(ids []string, id string) []string {
		if slices.Contains(ids, id) {
			return ids
		}
		changed = true
		return append(slices.Clip(ids), id)
	}
	for _, s := range ss {
		if !atWork(s) {
			continue
		}
		m.Watched = add(m.Watched, s.SessionID)
		if s.ContextPct >= compactAt {
			m.Full = add(m.Full, s.SessionID)
		}
		if waits(s, waiting) {
			m.Waited = add(m.Waited, s.SessionID)
		}
	}
	return m, changed
}

func waits(s sample.Session, waiting map[string]bool) bool {
	return waiting[s.Pane] || s.Asks || s.Permits
}

// Score freezes the card of a run that ended, from the sessions as they
// are at its end and the run's steps.
func (m Mode) Score(ss []sample.Session, waiting map[string]bool, compactAt float64, run []Entry) Mode {
	m, _ = m.observe(ss, waiting, compactAt) // the end counts, even between two samples
	c := Card{
		Night: m.Nightly, Since: m.Since, Ended: m.Ended,
		Watched: len(m.Watched), Full: len(m.Full), Waited: len(m.Waited), Of: len(m.Start),
	}
	for _, e := range run {
		if e.Result != Sent || e.Back.IsZero() {
			continue
		}
		c.Compacted++
		if e.After > 0 {
			c.Given += max(e.Before-e.After, 0)
		}
	}
	now := map[string]sample.Session{}
	for _, s := range ss {
		if s.SessionID != "" {
			now[s.SessionID] = s
		}
	}
	for _, seat := range m.Start {
		s, alive := now[seat.Session]
		why := ""
		switch {
		case !alive:
			why = "gone"
		case waits(s, waiting):
			why = "waits for you"
		case s.ContextPct >= compactAt:
			why = fmt.Sprintf("full · %.0f%%", s.ContextPct)
		}
		if why == "" {
			c.Ready++
			continue
		}
		c.NotReady = append(c.NotReady, Miss{seat.Name, why})
	}
	m.Card = &c
	return m
}
