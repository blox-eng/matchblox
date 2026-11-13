package app

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/chains"
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
	if isShipCard(s) {
		return colShip // ruling 31: a ship no session owns ships
	}
	if h, held := m.holdOf(s); held && h.By == holdByYou {
		return colQuiet
	}
	if age := s.Age(); age == "cold" || age == "stale" || s.Stopped() {
		return colQuiet
	}
	if m.asksNow(s) || m.goWaitsOn(s) {
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

// boardIndex is the four columns of every match on every host, each in
// its order, with no filter: one columnOf a match for each draw. The
// stoker's own agent is not a match.
func (m Model) boardIndex() [4][]sample.Session {
	var idx [4][]sample.Session
	if m.frameIdx != nil {
		for c := range idx {
			idx[c] = slices.Clone(m.frameIdx[c]) // its readers cut theirs in place
		}
		return idx
	}
	for _, s := range append(slices.Clone(m.all.sessions), m.shipCards()...) {
		if !m.isStoker(s) || m.stokerAsksAsCard(s) {
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

// boardColumn is the matches of column c that the filters show, in its
// order.
func (m Model) boardColumn(c boardCol) []sample.Session {
	return slices.DeleteFunc(m.boardIndex()[c], func(s sample.Session) bool { return !m.inBoard(s) })
}

// needCount is every match that needs the owner, on every host and in
// every chain, whatever the filters hide: one the owner parked is not
// counted, nor one on a host that is not reached.
func (m Model) needCount() int {
	n := 0
	for _, s := range m.boardIndex()[colNeed] {
		if !m.stale(s.Host) {
			n++
		}
	}
	return n
}

// inBoard: the chain filter, the host filter and the search show s.
func (m Model) inBoard(s sample.Session) bool {
	if r := m.room; r != nil && r.k.isOne() && oneKey(s) != r.k {
		return false // a chain of one's room is its one match
	}
	if m.boardChain != "" && m.chainLabelOf(s) != m.boardChain && !m.inRoomTeam(s) {
		return false
	}
	if m.boardHost != "" && m.nameOf(s.Host) != m.boardHost {
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

// boardChains are the names of the chains with a match that the host
// filter and the search leave on the board, in the order of the hosts and
// of each host's chains: what ] and [ turn.
func (m Model) boardChains() []string {
	all := m
	all.boardChain = ""
	on := map[string]bool{}
	for _, ss := range m.boardIndex() {
		for _, s := range ss {
			if all.inBoard(s) {
				on[m.chainLabelOf(s)] = true
			}
		}
	}
	var out []string
	for _, h := range m.hosts() {
		if cv := m.chainsOf(h); cv != nil {
			for _, c := range cv.Chains {
				if name := m.chainLabel(chainKey{h, c.ID}); on[name] && !slices.Contains(out, name) {
					out = append(out, name)
				}
			}
		}
	}
	return out
}

// shipCards are the ships in flight that no session owns (ruling 31, #80),
// each a card of its own: named by its branch, its pane the ship's mark.
func (m Model) shipCards() []sample.Session {
	var out []sample.Session
	for _, sh := range m.ships() {
		if sh.Kind == shipDone || sh.Session.ID != "" || sh.Session.Pane != "" {
			continue
		}
		out = append(out, sample.Session{Host: sh.Host, Name: m.shipCardName(sh), Pane: shipCardPane(sh), Cwd: sh.Where,
			Status: "idle", Started: sh.Since})
	}
	return out
}

// shipCardPane is the pane of a ship's card: "#4570@hearth/hearth-api". A
// tmux pane's id starts with %, so it is never a pane's.
func shipCardPane(sh proto.Ship) string { return "#" + strconv.Itoa(sh.Number) + "@" + sh.Repo }

// isShipCard: s is the card of a ship no session owns, and has no pane.
func isShipCard(s sample.Session) bool { return strings.HasPrefix(s.Pane, "#") }

// shipCardName is the name of a ship's card: the branch of its checkout,
// else its repository and number.
func (m Model) shipCardName(sh proto.Ship) string {
	if r := m.gitOf(sh.Host); r != nil && sh.Where != "" {
		for _, repo := range r.Repos {
			for _, wt := range repo.Worktrees {
				if wt.Path == sh.Where && wt.Branch != "" {
					return wt.Branch
				}
			}
		}
	}
	return filepath.Base(sh.Repo) + " #" + strconv.Itoa(sh.Number)
}

// shipOfCard is the ship of a ship's card.
func (m Model) shipOfCard(s sample.Session) (proto.Ship, bool) {
	if !isShipCard(s) {
		return proto.Ship{}, false
	}
	i := slices.IndexFunc(m.ships(), func(sh proto.Ship) bool { return sh.Host == s.Host && shipCardPane(sh) == s.Pane })
	if i < 0 {
		return proto.Ship{}, false
	}
	return m.ships()[i], true
}
