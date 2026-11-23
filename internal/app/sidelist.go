package app

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// sideUnit is one entry of the list: a chain with its members in team
// order, or one session in no chain. waiting are the chain's members that
// wait for recover, in team order. ahead[i] is the count of them before
// member i in team order (nil when none waits).
type sideUnit struct {
	chain   chainKey // zero for a session in no chain
	members []sample.Session
	lead    int // the place of the most urgent member; -1 when all wait
	waiting []state.MateView
	ahead   []int
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
// member count with the waiting members.
func (u sideUnit) step() (n, of int) {
	n = u.lead + 1
	if u.ahead != nil && u.lead >= 0 {
		n += u.ahead[u.lead]
	}
	return n, len(u.members) + len(u.waiting)
}

// waitsOf are the members of unseen that wait for recover, as the doc of
// chain k's host tells them.
func (m Model) waitsOf(k chainKey, unseen []chains.Member) []state.MateView {
	cv := m.chainsOf(k.host)
	if cv == nil {
		return nil
	}
	var out []state.MateView
	for _, mb := range unseen {
		if i := slices.IndexFunc(cv.Mates, func(v state.MateView) bool { return v.Wait != nil && v.Member.Same(mb) }); i >= 0 {
			out = append(out, cv.Mates[i])
		}
	}
	return out
}

// aheadOf is, for each member of ms, the count of waiting members before it
// in chain k's team order. A match the team does not name comes after all.
func (m Model) aheadOf(k chainKey, ms []sample.Session, waiting []state.MateView) []int {
	if len(waiting) == 0 {
		return nil
	}
	ahead := make([]int, len(ms))
	for j := range ahead {
		ahead[j] = len(waiting)
	}
	c, _ := m.teamOf(k)
	n := 0
	for _, mb := range c.Members {
		if slices.ContainsFunc(waiting, func(v state.MateView) bool { return v.Member.Same(mb) }) {
			n++
		} else if j := slices.IndexFunc(ms, func(s sample.Session) bool { return mb.Same(memberOf(s)) }); j >= 0 {
			ahead[j] = n
		}
	}
	return ahead
}

// sideTime is what a group orders its sessions by, as the board did: the
// wait, the turn's end, else the start, and the quiet ones by the last
// activity.
func (m Model) sideTime(h sideState, s sample.Session) time.Time {
	switch h {
	case sideNeed:
		return m.needSince(s)
	case sideDone:
		return s.TurnEndedAt
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
// the console is linked to. The stoker's own agent is one only while it
// asks.
func (m Model) sideSessions() []sample.Session {
	return slices.DeleteFunc(slices.Clone(m.all.sessions), func(s sample.Session) bool {
		return m.isStoker(s) && !m.stokerAsksAsCard(s)
	})
}

// sideGroups are the groups that hold a session of the hosts in view, in
// state rank. A chain is one unit, in the group of its most urgent member,
// and counts only the members in that state.
func (m Model) sideGroups() []sideGroup {
	if m.side != nil && m.side.built {
		return slices.Clip(m.side.groups)
	}
	groups := m.buildSideGroups()
	if m.side != nil {
		m.side.groups, m.side.built = groups, true
		m.side.builds++
	}
	return slices.Clip(groups)
}

// sideMemo keeps one build of the groups, and the order they give, for a
// step that changes nothing they are built from: a frame, the steps after
// a message, a move of the choice. builds counts its builds, for the tests.
type sideMemo struct {
	built           bool
	builds          int
	groups          []sideGroup
	order, walk     []paneKey
	ordered, walked bool
}

func (m Model) buildSideGroups() []sideGroup {
	var units []sideUnit
	var states [][]sideState
	done := map[chainKey]bool{}
	// A member that waits in two teams is one row, in the first: a second
	// row with the same key would stop ↓, as for a session in two chains.
	placed := map[paneKey]bool{}
	once := func(k chainKey, ws []state.MateView) []state.MateView {
		return slices.DeleteFunc(ws, func(v state.MateView) bool {
			key := waitKey(k, v)
			had := placed[key]
			placed[key] = true
			return had
		})
	}
	listed := slices.DeleteFunc(m.sideSessions(), func(s sample.Session) bool { return !m.inView(s.Host) })
	if m.space != "" { // a chain with a member in the space shows whole: the owner's ruling, don't break the chains
		open := map[chainKey]bool{}
		for _, s := range listed {
			if k, ok := m.chainKeyOf(s); ok && m.inSpace(s) {
				open[k] = true
			}
		}
		listed = slices.DeleteFunc(listed, func(s sample.Session) bool {
			k, ok := m.chainKeyOf(s)
			return !m.inSpace(s) && !(ok && open[k])
		})
	}
	isListed := map[paneKey]bool{}
	inChain := map[chainKey][]sample.Session{} // the listed matches of each chain, in list order
	for _, s := range listed {
		isListed[paneOf(s)] = true
		if k, ok := m.chainKeyOf(s); ok {
			inChain[k] = append(inChain[k], s)
		}
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
		seen, unseen := m.members(k)
		for _, o := range seen {
			// A session in two chains is one row, in the chain it counts
			// for: a second row would stop ↑↓, which walk by session.
			if isListed[paneOf(o)] && !m.inOtherChain(o, k) {
				ms = append(ms, o)
				have[paneOf(o)] = true
			}
		}
		for _, o := range inChain[k] { // a listed match the team does not name still shows
			if !have[paneOf(o)] {
				ms = append(ms, o)
			}
		}
		hs := make([]sideState, len(ms))
		for i, o := range ms {
			hs[i] = m.sideStateOf(o)
		}
		ws := once(k, m.waitsOf(k, unseen))
		units = append(units, sideUnit{chain: k, members: ms, waiting: ws, ahead: m.aheadOf(k, ms, ws)})
		states = append(states, hs)
	}
	for _, host := range m.hosts() { // a chain with no listed match still shows the members that wait
		cv := m.chainsOf(host)
		if cv == nil || !m.inView(host) || m.space != "" { // a member that waits works in no repo the list knows
			continue
		}
		for _, c := range cv.Chains {
			k := chainKey{host, c.ID}
			if done[k] {
				continue
			}
			_, unseen := m.members(k)
			if ws := once(k, m.waitsOf(k, unseen)); len(ws) > 0 {
				units = append(units, sideUnit{chain: k, lead: -1, waiting: ws})
				states = append(states, nil)
			}
		}
	}
	var out []sideGroup
	for h := sideNeed; h <= sideHeld; h++ {
		g := sideGroup{state: h}
		for i, u := range units {
			if len(u.members) == 0 { // all its members wait: the team sits in IDLE
				if h == sideIdle {
					g.count += len(u.waiting)
					g.units = append(g.units, u)
				}
				continue
			}
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
		slices.SortStableFunc(g.units, func(a, b sideUnit) int {
			if a.lead < 0 || b.lead < 0 { // a team whose members all wait goes last
				return cmp.Compare(b.lead, a.lead)
			}
			return m.sideBefore(h, a.urgent(), b.urgent())
		})
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
	if m.side != nil {
		got, done := &m.side.walk, &m.side.walked
		if folded {
			got, done = &m.side.order, &m.side.ordered
		}
		if !*done {
			*got, *done = m.buildSidePanes(folded), true
		}
		return slices.Clip(*got)
	}
	return m.buildSidePanes(folded)
}

func (m Model) buildSidePanes(folded bool) []paneKey {
	var out []paneKey
	for _, g := range m.sideGroups() {
		if !folded && m.sideFold[g.state] {
			continue
		}
		for _, u := range m.sideShows(g) {
			for _, s := range u.members {
				if !m.killing(s) {
					out = append(out, paneOf(s))
				}
			}
			for _, v := range u.waiting {
				out = append(out, waitKey(u.chain, v))
			}
		}
	}
	return out
}

// waitKey is the list's key of v, a member of chain k that waits for
// recover. Its pane starts with "wait:", which no pane key has: no live
// row and no act takes it for a session.
func waitKey(k chainKey, v state.MateView) paneKey {
	return paneKey{k.host, "wait:" + v.Member.SessionID}
}

// waits tells if k is the key of a member that waits for recover.
func (k paneKey) waits() bool { return strings.HasPrefix(k.pane, "wait:") }

// sideWaiting is the member that waits for recover whose row the list
// chooses, and its chain. false: the list chooses no such row.
func (m Model) sideWaiting() (state.MateView, chainKey, bool) {
	k := m.chosenSide()
	if !k.waits() {
		return state.MateView{}, chainKey{}, false
	}
	for _, g := range m.sideGroups() {
		for _, u := range g.units {
			for _, v := range u.waiting {
				if waitKey(u.chain, v) == k {
					return v, u.chain, true
				}
			}
		}
	}
	return state.MateView{}, chainKey{}, false
}

// inOtherChain is whether o counts for a chain other than k.
func (m Model) inOtherChain(o sample.Session, k chainKey) bool {
	own, ok := m.chainKeyOf(o)
	return ok && own != k
}
