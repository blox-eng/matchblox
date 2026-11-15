package app

import (
	"slices"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
)

// sideUnit is one entry of the list: a chain with its members in team
// order, or one session in no chain.
type sideUnit struct {
	chain   chainKey // zero for a session in no chain
	members []sample.Session
	lead    int // the place of the most urgent member
}

// sideGroup is the units of one state. count is the sessions of its units
// that are in that state: a chain's members in other states do not count.
type sideGroup struct {
	state sideState
	count int
	units []sideUnit
}

// urgent is the member the unit sits by.
func (u sideUnit) urgent() sample.Session { return u.members[u.lead] }

// step is the place of the most urgent member in team order, and the
// member count.
func (u sideUnit) step() (n, of int) { return u.lead + 1, len(u.members) }

// sideTime is what a group orders its sessions by, as the board did: the
// wait, else the start, and the quiet ones by the last activity.
func (m Model) sideTime(h sideState, s sample.Session) time.Time {
	switch h {
	case sideNeed:
		return m.needSince(s)
	case sideIdle:
		return m.lastActive(s)
	}
	return s.Started
}

// sideBefore orders two sessions of state h: the oldest first, the quiet
// ones newest first, and a session with no time last.
func (m Model) sideBefore(h sideState, a, b sample.Session) int {
	at, bt := m.sideTime(h, a), m.sideTime(h, b)
	switch {
	case at.IsZero() && !bt.IsZero():
		return 1
	case !at.IsZero() && bt.IsZero():
		return -1
	case h == sideIdle:
		return bt.Compare(at)
	}
	return at.Compare(bt)
}

// sideSessions are the sessions the list draws: every match on every host
// the console is linked to, and a row for each ship no session drives.
// The stoker's own agent is one only while it asks.
func (m Model) sideSessions() []sample.Session {
	listed := slices.DeleteFunc(slices.Clone(m.all.sessions), func(s sample.Session) bool {
		return m.isStoker(s) && !m.stokerAsksAsCard(s)
	})
	return append(listed, m.shipCards()...)
}

// sideGroups are the groups that hold a session of the hosts in view, in
// state rank. A chain is one unit, in the group of its most urgent member,
// and counts only the members in that state.
func (m Model) sideGroups() []sideGroup {
	var units []sideUnit
	var states [][]sideState
	done := map[chainKey]bool{}
	listed := slices.DeleteFunc(m.sideSessions(), func(s sample.Session) bool { return !m.inView(s.Host) })
	isListed := map[paneKey]bool{}
	for _, s := range listed {
		isListed[paneOf(s)] = true
	}
	for _, s := range listed {
		k, ok := m.chainKeyOf(s)
		if !ok {
			units = append(units, sideUnit{members: []sample.Session{s}})
			states = append(states, []sideState{m.sideStateOf(s)})
			continue
		}
		if done[k] {
			continue
		}
		done[k] = true
		var ms []sample.Session
		have := map[paneKey]bool{}
		seen, _ := m.members(k)
		for _, o := range seen {
			if isListed[paneOf(o)] {
				ms = append(ms, o)
				have[paneOf(o)] = true
			}
		}
		for _, o := range listed { // a listed match the team does not name still shows
			if ok := have[paneOf(o)]; !ok {
				if ko, in := m.chainKeyOf(o); in && ko == k {
					ms = append(ms, o)
				}
			}
		}
		hs := make([]sideState, len(ms))
		for i, o := range ms {
			hs[i] = m.sideStateOf(o)
		}
		units = append(units, sideUnit{chain: k, members: ms})
		states = append(states, hs)
	}
	var out []sideGroup
	for h := sideNeed; h <= sideIdle; h++ {
		g := sideGroup{state: h}
		for i, u := range units {
			lead, n := -1, 0
			for j, o := range u.members {
				if states[i][j] != h {
					continue
				}
				n++
				if lead < 0 || m.sideBefore(h, o, u.members[lead]) < 0 {
					lead = j
				}
			}
			if lead < 0 || slices.ContainsFunc(states[i], func(o sideState) bool { return o < h }) {
				continue // a more urgent member holds the unit in an earlier group
			}
			u.lead = lead
			g.count += n
			g.units = append(g.units, u)
		}
		if len(g.units) == 0 {
			continue
		}
		slices.SortStableFunc(g.units, func(a, b sideUnit) int { return m.sideBefore(h, a.urgent(), b.urgent()) })
		out = append(out, g)
	}
	return out
}

// sideOrder is the sessions of the list in drawn order: the units of each
// group in order, a chain's members in team order, a folded group's too.
func (m Model) sideOrder() []paneKey { return m.sidePanes(true) }

// sideWalk is the sessions ↑↓ walk: sideOrder without the rows of a folded
// group, which the list does not draw.
func (m Model) sideWalk() []paneKey { return m.sidePanes(false) }

func (m Model) sidePanes(folded bool) []paneKey {
	var out []paneKey
	for _, g := range m.sideGroups() {
		if !folded && m.sideFold[g.state] {
			continue
		}
		for _, u := range m.sideShows(g) {
			for _, s := range u.members {
				out = append(out, paneOf(s))
			}
		}
	}
	return out
}
