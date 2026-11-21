package service

import (
	"time"

	"github.com/blox-eng/matchblox/internal/queue"
)

// unreadGrace is how long a permission wait goes with no menu read before
// its screen is unknown: Claude Code fires the hook before it draws.
const unreadGrace = 3 * time.Second

// unknownScreenRefusal refuses an answer typed into a screen no rule reads.
const unknownScreenRefusal = "matchblox cannot read this screen: answer it in its pane"

// menuUnreadRefusal refuses a digit within the grace time, while no rule
// reads the menu yet: the agent may still be drawing it.
const menuUnreadRefusal = "not sent: matchblox has not read this menu yet: answer it in its pane"

type unreadWait struct {
	since   time.Time // the wait's start
	samples int       // the samples taken since then that read no menu
	last    time.Time // the newest of them
	unknown bool
}

// markUnread marks the sessions whose hook waits for a permission while no
// rule reads a menu on their screen: MenuUnread at once, UnknownScreen
// after the grace time and two samples. now is the sample's time: a hook
// with no new sample counts no sample, and a sample from before the wait
// read the menu before it, not this one. Hold s.mu.
func (s *Service) markUnread(now time.Time) {
	waits := map[string]queue.Item{}
	for _, it := range s.cur.Queue {
		if it.State == queue.StatePermission && s.queue.Waits(it.SessionID) {
			waits[it.Pane] = it
		}
	}
	next := map[string]unreadWait{}
	copied := false
	for i, sess := range s.cur.Sessions {
		it, ok := waits[sess.Pane]
		unread, unknown := false, false
		if ok && sess.Pane != "" && (!sess.Permits || !now.After(it.Since)) && sess.Ask == nil {
			unread = true
			w, had := s.unread[sess.Pane]
			if !had {
				w.since = it.Since
			}
			if now.After(w.last) && !now.Before(w.since) {
				w.samples, w.last = w.samples+1, now
			}
			w.unknown = w.unknown || now.Sub(w.since) >= unreadGrace && w.samples >= 2
			next[sess.Pane], unknown = w, w.unknown
		}
		if unread == sess.MenuUnread && unknown == sess.UnknownScreen {
			continue
		}
		if !copied { // cur.Sessions can share its array with the sample
			s.cur.Sessions, copied = append(s.cur.Sessions[:0:0], s.cur.Sessions...), true
		}
		s.cur.Sessions[i].MenuUnread, s.cur.Sessions[i].UnknownScreen = unread, unknown
	}
	s.unread = next
}

// hookedPermission: pane's hook waits for a permission, the wait
// markUnread watches.
func (s *Service) hookedPermission(pane string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range s.cur.Queue {
		if it.Pane == pane && it.State == queue.StatePermission && s.queue.Waits(it.SessionID) {
			return true
		}
	}
	return false
}
