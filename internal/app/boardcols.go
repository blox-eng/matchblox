package app

import (
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// asksNow: the match asks a question or a permission. Claude Code's idle
// prompt after a turn asks nothing: the turn ended. An agent that reports
// its own state decides: one that says it works asks nothing, whatever its
// transcript or the queue still hold.
func (m Model) asksNow(s sample.Session) bool {
	if s.ReportsWork() {
		return false
	}
	it, ok := m.waitOf(s)
	return s.ReportsWait() || s.Asks || s.Permits || ok && (it.State == queue.StatePermission || it.State == queue.StateQuestion && !it.Reminder)
}

// goWaitsOn: a stoker step waits for the owner's go on the match's pane.
func (m Model) goWaitsOn(s sample.Session) bool {
	_, ok := m.goSince(s)
	return ok
}

// goSince is when the step that waits for a go on s's pane began, by the
// host's stoker or one of its chain stokers.
func (m Model) goSince(s sample.Session) (time.Time, bool) {
	if s.Pane == "" {
		return time.Time{}, false
	}
	st := m.stateOf(s.Host)
	views := []*proto.StokerView{st.Stoker}
	if s.Host == m.opt.Host {
		views[0] = m.stoker
	}
	for i := range st.Stokers {
		views = append(views, &st.Stokers[i].View)
	}
	for _, v := range views {
		if v == nil {
			continue
		}
		for _, r := range m.onUnit(state.ChainStoker{View: *v}).stokerRows() {
			if r.kind == rowGo && r.state() == "waits" && r.e.Needs != "" && r.e.Pane == s.Pane {
				return r.e.At, true
			}
		}
	}
	return time.Time{}, false
}

// shipOf is the first ship of s, in the order shipsOf gives, of one of
// the kinds.
func (m Model) shipOf(s sample.Session, kinds ...string) (proto.Ship, bool) {
	for _, sh := range m.shipsOf(s) {
		if slices.Contains(kinds, sh.Kind) {
			return sh, true
		}
	}
	return proto.Ship{}, false
}

// needSince is when a match in NEEDS YOU began to wait: the oldest of its
// queue row, its go and the ship that needs the owner. A match that asks
// only in its pane waits since its last activity.
func (m Model) needSince(s sample.Session) time.Time {
	var at []time.Time
	if it, ok := m.waitOf(s); ok {
		at = append(at, it.Since)
	}
	if t, ok := m.goSince(s); ok {
		at = append(at, t)
	}
	if sh, ok := m.shipOf(s, shipWaits, shipStuck); ok {
		at = append(at, sh.Since)
	}
	if len(at) == 0 {
		return m.lastActive(s)
	}
	return slices.MinFunc(at, time.Time.Compare)
}

// lastActive is when the match last did anything: the sample's idle time
// back from now.
func (m Model) lastActive(s sample.Session) time.Time {
	return m.now().Add(-s.Idle)
}

// inBoard: the chain filter, the host filter and the search show s.
func (m Model) inBoard(s sample.Session) bool {
	if r := m.room; r != nil && r.k.isOne() && oneKey(s) != r.k {
		return false // a chain of one's room is its one match
	}
	if m.boardChain != "" && m.chainLabelOf(s) != m.boardChain && !m.inRoomTeam(s) {
		return false
	}
	if !m.inView(s.Host) {
		return false
	}
	if m.filter == "" {
		return true
	}
	word, says := m.cardSays(s)
	text := strings.Join([]string{s.Name, s.Target, s.Pane, s.Tab, s.Cwd, s.Model, m.lastWords(s), m.wordOf(s),
		word, says, m.chainLabelOf(s)}, " ")
	return strings.Contains(strings.ToLower(text), strings.ToLower(m.filter))
}

// inRoomTeam tells if s is a member of the team whose room is open: a team
// with one match left is in no chain the board knows (chainIx drops it),
// and its room still draws that match's card (eng review correction 5).
func (m Model) inRoomTeam(s sample.Session) bool {
	r := m.room
	if r == nil || r.k.isOne() || s.Host != r.k.host || m.isStoker(s) {
		return false
	}
	c, ok := m.teamOf(r.k)
	return ok && slices.ContainsFunc(c.Members, func(mb chains.Member) bool { return mb.Same(memberOf(s)) })
}

// stokerAsksAsCard: s is a chain stoker's agent that asks. It is a need
// as any match that asks (ruling 39), so it is a card; one that does not
// ask is not.
func (m Model) stokerAsksAsCard(s sample.Session) bool {
	_, ok := m.stokerChain(s)
	return ok && m.asksNow(s)
}

// chainLabelOf is the name of the chain s is in, "" for none: a chain
// stoker's agent is in its own chain.
func (m Model) chainLabelOf(s sample.Session) string {
	if k, ok := m.chainKeyOf(s); ok {
		return m.chainLabel(k)
	}
	if k, ok := m.stokerChain(s); ok {
		return m.chainLabel(k)
	}
	return ""
}
