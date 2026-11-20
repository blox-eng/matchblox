package app

import (
	"maps"
	"slices"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
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
	at    time.Time // the time of its host's last sample read against it
	seen  int       // the samples of its host since, that still show the old state
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
		name: s.Name, to: to, park: park, at: m.sampleAt(s.Host)}
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

// sampleAt is the time of host's last sample.
func (m Model) sampleAt(host string) time.Time {
	if host == m.opt.Host {
		return m.snap.At
	}
	if i := m.peerAt(host); i >= 0 {
		return m.peers[i].st.At
	}
	return time.Time{}
}

// settleAfter is how many samples of its host an act waits for before it
// counts as not taken: the agent can take a sample or two to exit after
// its pane goes (the owner, 2026-10-24).
const settleAfter = 3

// settle reads a new sample of host against its pending acts: each one
// the sample shows is done, and one it still does not show after
// settleAfter samples comes back with why.
func (m *Model) settle(host string) {
	changed := false
	for id, p := range m.pendingActs {
		if p.key.host != host {
			continue
		}
		s, ok := m.sessionAt(p.key.host, p.key.pane)
		same := ok && s.PID == p.pid && s.Start == p.start
		var done bool
		switch p.kind {
		case actKill:
			done = !same
		case actRename:
			done = !same || s.Name == p.to
		case actPark:
			_, held := m.holdOf(s)
			done = !same || held == p.park
		}
		if !done {
			if at := m.sampleAt(host); at.After(p.at) { // a sample sent again is not a new one
				p.at = at
				p.seen++
			}
			m.pendingActs[id] = p
			if p.seen < settleAfter {
				continue
			}
			m.sayError(p.notTaken())
		}
		delete(m.pendingActs, id)
		changed = true
	}
	if changed && m.side != nil {
		m.side = &sideMemo{}
	}
}

// notTaken says that p did not take, and the key that tries again.
func (p pendingAct) notTaken() string {
	switch p.kind {
	case actKill:
		return p.name + " is still running after the kill. Press x to kill it again."
	case actRename:
		return p.name + ": the rename did not take. Press r to rename it again."
	}
	return p.name + ": the park did not take. Press z to try again."
}

// settleResult reads the service's answer to a pending act. A refusal
// ends it and says why (true: the answer is said); a skip ends it, and
// the usual words say why; a run leaves it to the samples. A rename's
// form says its own refusal.
func (m *Model) settleResult(r proto.Result) bool {
	p, ok := m.pendingActs[r.ActID]
	if !ok || r.Err == "" && len(r.Skipped) == 0 {
		return false
	}
	delete(m.pendingActs, r.ActID)
	if m.side != nil {
		m.side = &sideMemo{}
	}
	if r.Err == "" || p.kind == actRename {
		return false
	}
	m.sayError(p.kind.String() + " failed: " + r.Err)
	return true
}

// actsLost ends the pending acts of host when its connection drops: no
// sample can confirm them now, and the list must not wait on them. It
// says so, or "" when none was pending.
func (m Model) actsLost(host string) (Model, string) {
	n := len(m.pendingActs)
	maps.DeleteFunc(m.pendingActs, func(_ string, p pendingAct) bool { return p.key.host == host })
	if len(m.pendingActs) == n {
		return m, ""
	}
	if m.side != nil {
		m.side = &sideMemo{}
	}
	return m, "not confirmed: the connection to " + m.nameOf(host) + " dropped. Look at the list when it is back."
}
