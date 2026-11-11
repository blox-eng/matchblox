package app

import (
	"slices"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// The board puts each match in one of four columns, left to right: the
// leftmost whose rule holds (the spec's Columns table).
type boardCol int

const (
	colNeed boardCol = iota
	colWork
	colShip
	colQuiet
)

var boardCols = [...]boardCol{colNeed, colWork, colShip, colQuiet}

func (c boardCol) name() string {
	return [...]string{"NEEDS YOU", "WORKING", "SHIPPING", "QUIET"}[c]
}

func (c boardCol) short() string {
	return [...]string{"need", "work", "ship", "quiet"}[c]
}

// columnOf is the column of a match. A park by the owner is QUIET whatever
// else is true; a question, a permission or a go waits even while the
// match works; a busy match is on its stuck ship.
func (m Model) columnOf(s sample.Session) boardCol {
	if h, held := m.holdOf(s); held && h.By == holdByYou {
		return colQuiet
	}
	if age := s.Age(); age == "cold" || age == "stale" || s.Stopped() {
		return colQuiet
	}
	if m.asksNow(s) || m.goWaits(s) {
		return colNeed
	}
	if s.Works() || len(s.Shells) > 0 {
		return colWork
	}
	if _, waits := m.waitOf(s); waits {
		return colNeed // its turn ended, and it waits for input
	}
	if _, ok := m.shipOf(s, shipWaits, shipStuck); ok {
		return colNeed
	}
	if _, ok := m.shipOf(s, shipRunning); ok {
		return colShip
	}
	return colQuiet
}

// asksNow: the match asks a question or a permission. Claude Code's idle
// prompt after a turn asks nothing: the turn ended.
func (m Model) asksNow(s sample.Session) bool {
	it, ok := m.waitOf(s)
	return s.Asks || s.Permits || ok && (it.State == queue.StatePermission || it.State == queue.StateQuestion && !it.Reminder)
}

// goWaits: a stoker step waits for the owner's go on the match's pane.
func (m Model) goWaits(s sample.Session) bool {
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

// boardIndex is the four columns of every match on every host, each in
// its order, with no filter: one columnOf a match for each draw. The
// stoker's own agent is not a match.
func (m Model) boardIndex() [4][]sample.Session {
	var idx [4][]sample.Session
	for _, s := range m.all.sessions {
		if !m.isStoker(s) {
			c := m.columnOf(s)
			idx[c] = append(idx[c], s)
		}
	}
	type keyed struct {
		s  sample.Session
		at time.Time
	}
	for _, c := range boardCols {
		ks := make([]keyed, len(idx[c]))
		for i, s := range idx[c] {
			ks[i] = keyed{s, m.boardTime(c, s)}
		}
		slices.SortStableFunc(ks, func(a, b keyed) int {
			switch {
			case a.at.IsZero() && !b.at.IsZero():
				return 1 // no time: last
			case !a.at.IsZero() && b.at.IsZero():
				return -1
			case c == colQuiet:
				return b.at.Compare(a.at) // the newest first
			}
			return a.at.Compare(b.at)
		})
		for i, k := range ks {
			idx[c][i] = k.s
		}
		idx[c] = m.chainsTogether(idx[c])
	}
	return idx
}

// boardTime is what a column orders its matches by: the wait, when the
// session started, the ship's stage, or the last activity.
func (m Model) boardTime(c boardCol, s sample.Session) time.Time {
	switch c {
	case colNeed:
		return m.needSince(s)
	case colShip:
		sh, _ := m.shipOf(s, shipRunning)
		return sh.Since
	case colQuiet:
		return m.lastActive(s)
	}
	return s.Started
}

// lastActive is when the match last did anything: the sample's idle time
// back from now.
func (m Model) lastActive(s sample.Session) time.Time {
	return m.now().Add(-s.Idle)
}

// chainsTogether moves each chain's later members up to its first one,
// each in the order it had.
func (m Model) chainsTogether(ss []sample.Session) []sample.Session {
	out := make([]sample.Session, 0, len(ss))
	done := map[chainKey]bool{}
	for _, s := range ss {
		k, ok := m.chainKeyOf(s)
		if !ok {
			out = append(out, s)
			continue
		}
		if done[k] {
			continue
		}
		done[k] = true
		for _, o := range ss {
			if ko, ok := m.chainKeyOf(o); ok && ko == k {
				out = append(out, o)
			}
		}
	}
	return out
}

// boardColumn is the matches of column c, in its order.
func (m Model) boardColumn(c boardCol) []sample.Session {
	return m.boardIndex()[c]
}

// needCount is every match that needs the owner, on every host: one the
// owner parked is not counted.
func (m Model) needCount() int {
	return len(m.boardIndex()[colNeed])
}
