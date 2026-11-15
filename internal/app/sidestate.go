package app

import "github.com/blox-eng/matchblox/internal/sample"

// sideState is the state of a session in the list. The order is the rank:
// the list draws its groups in it.
type sideState int

const (
	sideNeed sideState = iota
	sideWork
	sideHeld
	sideIdle
)

func (h sideState) head() string {
	return [...]string{"NEEDS YOU", "WORKING", "HELD", "IDLE"}[h]
}

// sideStateOf is the state of a session: the first rule that holds. A park
// by the owner holds whatever else is true. A question, a permission or a
// go needs the owner even while the session works or the stoker holds it.
// A session that waits on a ship runs on the agent's own state.
func (m Model) sideStateOf(s sample.Session) sideState {
	h, held := m.holdOf(s)
	if held && h.By == holdByYou {
		return sideHeld
	}
	if age := s.Age(); age == "cold" || age == "stale" || s.Stopped() {
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
