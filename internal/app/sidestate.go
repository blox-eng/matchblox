package app

import (
	"cmp"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// sideState is the state of a session in the list. The order is the rank:
// the list draws its groups in it.
type sideState int

const (
	sideNeed sideState = iota
	sideDone
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
// A session that waits on a ship runs on the agent's own state.
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
	if s.SigningIn {
		return sideIdle
	}
	if _, ok := m.refreshOf(s.Host, s.Pane); ok {
		return sideIdle
	}
	if m.finished(s) {
		return sideDone
	}
	return sideIdle
}

// finished tells if the session's last turn ended since the owner gave it
// the keys. It needs a proof: a turn end this console saw, or a KeysAt that
// the session's last activity is after. A session first seen idle, with no
// KeysAt, has no proof and is IDLE: the service restarts on each update and
// forgets KeysAt, and DONE must not fill with every idle agent.
func (m Model) finished(s sample.Session) bool {
	at := m.keysAtOf(s)
	_, saw := m.turnEnded[m.paneKeyOf(s)]
	if !saw && at.IsZero() {
		return false
	}
	return m.lastActive(s).After(at)
}

func (m Model) paneKeyOf(s sample.Session) paneKey { return m.paneKeyOn(m.opt.Host, s) }

// paneKeyOn is the key of s, on host when the sample names none.
func (m Model) paneKeyOn(host string, s sample.Session) paneKey {
	k := paneOf(s)
	k.host = cmp.Or(s.Host, host)
	return k
}

// keysAtOf is when the owner last gave s the keys: the later of the
// service's KeysAt (by session id) and this console's own give (by pane,
// for a session with no id yet).
func (m Model) keysAtOf(s sample.Session) time.Time {
	var at time.Time
	if s.SessionID != "" {
		at = m.keysAt[s.SessionID]
	}
	if g := m.gave[m.paneKeyOf(s)]; g.After(at) {
		at = g
	}
	return at
}

// noteGave records that this console gave s the keys.
func (m *Model) noteGave(s sample.Session) {
	if m.gave == nil {
		m.gave = map[paneKey]time.Time{}
	}
	m.gave[m.paneKeyOf(s)] = m.now()
}

// noteTurnEnds records each session of host that went from work to rest
// between prev and next. The first sample of a host (had false) holds none:
// a session first seen idle did not end a turn that this console saw. A
// session that works again loses its mark; a session that is gone loses it.
func (m *Model) noteTurnEnds(host string, prev []sample.Session, had bool, next []sample.Session) {
	if m.turnEnded == nil {
		m.turnEnded = map[paneKey]time.Time{}
	}
	was := map[string]bool{}
	for _, s := range prev {
		was[paneOf(s).pane] = s.Works()
	}
	here := map[paneKey]bool{}
	for _, s := range next {
		at := m.paneKeyOn(host, s)
		here[at] = true
		switch {
		case s.Works():
			delete(m.turnEnded, at)
		case had && was[at.pane]:
			m.turnEnded[at] = m.now()
		}
	}
	for at := range m.turnEnded {
		if at.host == host && !here[at] {
			delete(m.turnEnded, at)
		}
	}
}

// takeKeysAt replaces this host's KeysAt from st. It copies the snapshot's
// ids and drops this host's session ids that the snapshot does not name.
// Other hosts' ids stay: a home sample must not wipe a peer's times.
func (m *Model) takeKeysAt(prev []sample.Session, st proto.State) {
	if m.keysAt == nil {
		m.keysAt = map[string]time.Time{}
	}
	ours := map[string]struct{}{}
	for _, s := range prev {
		if s.SessionID != "" {
			ours[s.SessionID] = struct{}{}
		}
	}
	for _, s := range st.Sessions {
		if s.SessionID != "" {
			ours[s.SessionID] = struct{}{}
		}
	}
	for id := range ours {
		if _, ok := st.KeysAt[id]; !ok {
			delete(m.keysAt, id)
		}
	}
	for id, at := range st.KeysAt {
		if _, ok := ours[id]; ok {
			m.keysAt[id] = at
		}
	}
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

// doneCount is sessions in the DONE group: a finished member that still
// sits under a NEEDS YOU chain is not counted.
func (m Model) doneCount() int {
	for _, g := range m.sideGroups() {
		if g.state == sideDone {
			return g.count
		}
	}
	return 0
}
