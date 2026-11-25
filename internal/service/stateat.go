package service

import (
	"time"

	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

// stateMisses is how many samples in a row may miss a session before its
// state is dropped: one sample that misses it does not restart its clock.
const stateMisses = 2

type activity int

const (
	rests activity = iota
	works
	asks
)

type paneState struct {
	class  activity
	since  time.Time
	last   time.Time // the newest sample that showed or missed it
	misses int
}

// queueWaits are the queue's waits that ask, by pane, and by session id
// for an item with no pane: when each began.
type queueWaits struct{ byPane, bySID map[string]time.Time }

func waitsOf(items []queue.Item) queueWaits {
	w := queueWaits{map[string]time.Time{}, map[string]time.Time{}}
	for _, it := range items {
		if it.State != queue.StatePermission && (it.State != queue.StateQuestion || it.Reminder) {
			continue
		}
		if it.Pane != "" {
			w.byPane[it.Pane] = it.Since
		}
		if it.SessionID != "" {
			w.bySID[it.SessionID] = it.Since
		}
	}
	return w
}

// of is the wait x has in the queue, matched as the console's waitOf
// matches: by pane, or by session id for a session with no pane.
func (w queueWaits) of(x sample.Session) (time.Time, bool) {
	if x.Pane != "" {
		at, ok := w.byPane[x.Pane]
		return at, ok
	}
	at, ok := w.bySID[x.SessionID]
	return at, ok && x.SessionID != ""
}

// activityOf mirrors the console's asksNow: a drawn menu asks, a report of
// work asks nothing, and a report, the pane or the hooks' queue can ask.
func activityOf(x sample.Session, w queueWaits) activity {
	_, waits := w.of(x)
	switch {
	case x.Permits || !x.ReportsWork() && (x.ReportsWait() || x.Asks || waits):
		return asks
	case x.Works():
		return works
	}
	return rests
}

// stampStateAt sets each session's StateAt: the time its activity class
// last changed, at now, or when its wait in the queue began if that is
// later (a hook comes between samples). A session first seen began its
// class when its wait began if it asks through the queue, when it went idle
// if it rests, else when it started. Hold s.mu.
func (s *Service) stampStateAt(now time.Time) {
	w := waitsOf(s.cur.Queue)
	next := make(map[string]paneState, len(s.cur.Sessions))
	copied := false
	for i, x := range s.cur.Sessions {
		key := x.Pane
		if key == "" {
			key = "sid:" + x.SessionID
		}
		class := activityOf(x, w)
		st, had := s.states[key]
		at, waits := w.of(x)
		switch {
		case had && st.class == class:
		case had:
			st.class, st.since = class, now
			if waits && class == asks && at.After(now) {
				st.since = at
			}
		case class == asks && waits:
			st = paneState{class: class, since: at}
		case class == rests && x.Idle > 0:
			st = paneState{class: class, since: now.Add(-x.Idle)}
		case !x.Started.IsZero():
			st = paneState{class: class, since: x.Started}
		default:
			st = paneState{class: class, since: now}
		}
		st.last, st.misses = now, 0
		next[key] = st
		if x.StateAt.Equal(st.since) && x.TurnEndedAt.Equal(x.TurnEnded) {
			continue
		}
		if !copied { // cur.Sessions can share its array with the sample
			s.cur.Sessions, copied = append(s.cur.Sessions[:0:0], s.cur.Sessions...), true
		}
		s.cur.Sessions[i].StateAt, s.cur.Sessions[i].TurnEndedAt = st.since, x.TurnEnded
	}
	for key, st := range s.states {
		if _, here := next[key]; here {
			continue
		}
		if now.After(st.last) { // a hook between samples misses nothing
			st.last, st.misses = now, st.misses+1
		}
		if st.misses <= stateMisses {
			next[key] = st
		}
	}
	s.states = next
}
