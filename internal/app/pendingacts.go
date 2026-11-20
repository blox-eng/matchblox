package app

import (
	"slices"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
)

// A pending act is a kill, a rename or a park this console sent and the
// samples do not show yet. The row shows its result at once, faint; the
// sample confirms it, or the act's Result or the samples revert it with
// the reason (#211). It is the console's own memory: the service and the
// snapshot know nothing of it, and another console sees the act only when
// its sample shows it.

type actKind int

const (
	actKill actKind = iota + 1
	actRename
	actPark
)

func (k actKind) String() string {
	return [...]string{"", "kill", "rename", "park"}[k]
}

// pendingAct is one act on one session: the session by its pane, pid and
// start, so a reused pid never inherits it.
type pendingAct struct {
	kind  actKind
	key   paneKey
	pid   int
	start uint64
	name  string    // the session's name when the act went
	to    string    // a rename's new name
	park  bool      // a park's wanted hold: true parks, false lets go
	since time.Time // the host's last snapshot when the act went
	seen  int       // the samples since that still show the old state
}

// expect remembers act id on s. An act that did not go (no id) gets none:
// its flash already says "not sent".
func (m *Model) expect(id string, kind actKind, s sample.Session, to string, park bool) {
	if id == "" {
		return
	}
	if m.pendingActs == nil {
		m.pendingActs = map[string]pendingAct{}
	}
	if kind == actKill {
		m.stepOff(paneOf(s))
	}
	m.pendingActs[id] = pendingAct{kind: kind, key: paneOf(s), pid: s.PID, start: s.Start,
		name: s.Name, to: to, park: park, since: m.snapAt(s.Host)}
	if m.side != nil {
		m.side = &sideMemo{} // the list's groups and walk change with it
	}
}

// sessionAt is the listed session of pane on host.
func (m Model) sessionAt(host, pane string) (sample.Session, bool) {
	for _, s := range m.all.sessions {
		if s.Host == host && s.Pane == pane {
			return s, true
		}
	}
	return sample.Session{}, false
}

// snapAt is when host's last snapshot came.
func (m Model) snapAt(host string) time.Time {
	if host != m.opt.Host {
		if i := m.peerAt(host); i >= 0 {
			return m.peers[i].at
		}
		return time.Time{}
	}
	return m.at
}

// pendingOf is the act pending on s: the same pane, pid and start.
func (m Model) pendingOf(s sample.Session) (pendingAct, bool) {
	k := paneOf(s)
	for _, p := range m.pendingActs {
		if p.key == k && p.pid == s.PID && p.start == s.Start {
			return p, true
		}
	}
	return pendingAct{}, false
}

// killing is whether s goes: a kill of it is pending.
func (m Model) killing(s sample.Session) bool {
	p, ok := m.pendingOf(s)
	return ok && p.kind == actKill
}

// stepOff moves the choice off k to the row after it, else the one
// before: a row that goes takes no keys.
func (m *Model) stepOff(k paneKey) {
	if m.chosenSide() != k {
		return
	}
	order := m.sideOrder()
	i := slices.Index(order, k)
	switch {
	case i+1 < len(order):
		m.sideAt = order[i+1]
	case i > 0:
		m.sideAt = order[i-1]
	}
}
