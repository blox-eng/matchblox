package app

import "github.com/blox-eng/matchblox/internal/sample"

// sideState is the state of a session in the list. The order is the rank:
// the list draws its groups in it.
type sideState int

const (
	sideNeed sideState = iota
	sideDone           // a turn ended that the owner has not opened
	sideWork
	sideIdle
	sideHeld // last: the builder put it away (#211)
)

func (h sideState) head() string {
	return [...]string{"NEEDS YOU", "DONE", "WORKING", "IDLE", "HELD"}[h]
}

// sideStateOf is the state of a session: the first rule that holds. A park
// by the owner holds whatever else is true. A question, a permission or a
// go needs the owner even while the session works or the stoker holds it.
// A session that waits on a ship runs on the agent's own state. A rest
// whose turn ended after the owner last opened it is done.
func (m Model) sideStateOf(s sample.Session) sideState {
	h, held := m.holdOf(s)
	if p, ok := m.pendingOf(s); ok && p.kind == actPark { // a park sent shows at once
		if p.park {
			return sideHeld
		}
		held = false
	}
	if held && h.By == holdByYou {
		return sideHeld
	}
	done := s.TurnEndedAt.After(s.SeenAt) || m.doneKept != (paneKey{}) && paneOf(s) == m.doneKept && m.doneKept == m.sideAt
	if age := s.Age(); age == "cold" || age == "stale" || s.Stopped() {
		if done { // only an open takes a turn out of DONE
			return sideDone
		}
		return sideIdle
	}
	if m.asksNow(s) || m.goWaitsOn(s) {
		return sideNeed
	}
	if held {
		return sideHeld
	}
	if s.Works() || len(s.Shells) > 0 {
		return sideWork
	}
	if _, waits := m.waitOf(s); waits {
		return sideNeed
	}
	if _, ok := m.shipOf(s, shipWaits, shipStuck); ok {
		return sideNeed
	}
	if done {
		return sideDone
	}
	return sideIdle
}

// heldWhy is the words of a hold: who holds the session, and why.
func (m Model) heldWhy(s sample.Session) string {
	h, ok := m.holdOf(s)
	switch {
	case !ok:
		return ""
	case h.By == holdByYou:
		return "parked by you · z lets it go"
	case h.Why != "":
		return "held by the stoker · " + h.Why
	}
	return "held by the stoker"
}

// needCount is every session that needs the owner, on every host in view
// and in every chain, whatever the filters hide: one the owner parked is
// not counted, nor one on a host that is not reached, nor one on a hidden
// host that s does not show.
func (m Model) needCount() int {
	n := 0
	for _, s := range m.sideSessions() {
		if m.sideStateOf(s) == sideNeed && !m.stale(s.Host) && (!m.hostHidden(s.Host) || m.inView(s.Host)) {
			n++
		}
	}
	return n
}
