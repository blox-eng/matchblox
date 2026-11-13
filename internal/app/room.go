package app

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

// room is a chain's room on screen: the chain, the cursor's row, the
// session each client flipped from (by client tty: desk and phone share one
// console), and whether the plan is unfolded.
type room struct {
	k        chainKey
	at       int
	from     map[string]string
	unfolded bool
	typing   bool
	draft    string
	sent     string // the words of the order in flight, back in the line if it is refused
	act      string // the order act in flight; "" when none
	queued   bool   // a chain of one's words wait for the host's stoker it started
	starts   bool   // the act in flight starts the chain's stoker (stoker:new)
	woke     bool   // the stoker:new went through: the card says waking until it runs
	failed   string // why the chain's stoker did not start; "" when it did or was not asked
}

type roomKind int

// The room's own rows: the stoker's card, its steps and the fold, and
// the talk line. The board draws its matches, ships and git (Task 12 step
// 6d); roomNone is no row, as the cursor on the board.
const (
	roomNone roomKind = iota
	roomCard
	roomStep
	roomMore
	roomTalk
)

// roomRow is one row of a room: what it is, the id it acts on (a step's
// number), and its words.
type roomRow struct {
	kind roomKind
	id   string
	text string
	tone string // "warn" or "neg" for a state that needs the owner; "" plain
}

// oneKey is the room of a session in no chain: its chain of one, which the
// host's stoker drives.
func oneKey(s sample.Session) chainKey { return chainKey{s.Host, "=" + s.Pane} }

// isOne tells if k is a chain of one.
func (k chainKey) isOne() bool { return strings.HasPrefix(k.id, "=") }

// teamOf is chain k as its host sent it.
func (m Model) teamOf(k chainKey) (chains.Chain, bool) {
	cv := m.chainsOf(k.host)
	if cv == nil || k.isOne() {
		return chains.Chain{}, false
	}
	i := slices.IndexFunc(cv.Chains, func(c chains.Chain) bool { return c.ID == k.id })
	if i < 0 {
		return chains.Chain{}, false
	}
	return cv.Chains[i], true
}

// roomName is what the room calls its chain.
func (m Model) roomName(k chainKey) string {
	if c, ok := m.teamOf(k); ok && c.Name != "" {
		return c.Name
	}
	return k.id
}

// roomKeys are the rooms there are: every team of every host, and the team
// a chain stoker keeps though no match of it is left (eng review
// correction 5: chainIx drops a team with one present match).
func (m Model) roomKeys() []chainKey {
	hosts := []string{m.opt.Host}
	for _, p := range m.peers {
		hosts = append(hosts, p.host)
	}
	var out []chainKey
	for _, h := range hosts {
		if cv := m.chainsOf(h); cv != nil {
			for _, c := range cv.Chains {
				out = append(out, chainKey{h, c.ID})
			}
		}
		for _, u := range m.stateOf(h).Stokers {
			if k := (chainKey{h, u.Chain}); !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
	}
	return out
}

// members are the sessions of room k the sampler sees, in the team's order,
// and the members it cannot see.
func (m Model) members(k chainKey) ([]sample.Session, []chains.Member) {
	if k.isOne() {
		for _, s := range m.all.sessions {
			if oneKey(s) == k {
				return []sample.Session{s}, nil
			}
		}
		return nil, nil
	}
	c, ok := m.teamOf(k)
	if !ok {
		return nil, nil
	}
	var seen []sample.Session
	var unseen []chains.Member
	for _, mb := range c.Members {
		i := slices.IndexFunc(m.all.sessions, func(s sample.Session) bool {
			return s.Host == k.host && !m.isStoker(s) && mb.Same(memberOf(s))
		})
		if i < 0 {
			unseen = append(unseen, mb)
			continue
		}
		seen = append(seen, m.all.sessions[i])
	}
	return seen, unseen
}

// chainNeed is what room k waits on the person for: an ask of one of its
// matches (the pane), or a go its stoker waits for.
type chainNeed struct {
	pane string
	g    goWait
	isGo bool
	at   int64
}

// needOf is the oldest need of room k: an ask of one of its matches, or a
// step of its stoker that waits for a go.
func (m Model) needOf(k chainKey) (chainNeed, bool) {
	var needs []chainNeed
	seen, _ := m.members(k)
	for _, s := range seen {
		if it, ok := m.waitOf(s); ok && (it.State == queue.StatePermission || it.State == queue.StateQuestion) {
			needs = append(needs, chainNeed{pane: it.Pane, at: it.Since.UnixNano()})
		}
	}
	for _, g := range m.goWaits(k) {
		needs = append(needs, chainNeed{pane: g.pane, g: g, isGo: true, at: g.at})
	}
	if len(needs) == 0 {
		return chainNeed{}, false
	}
	slices.SortStableFunc(needs, func(a, b chainNeed) int { return cmp.Compare(a.at, b.at) })
	return needs[0], true
}

// roomEmpty is the id of the card's line of a chain with no match: ⏎ on it
// starts a session.
const roomEmpty = "empty"

// roomRows are room k's own rows, top down: the stoker's card and its
// steps, a line when the chain has no match, then the talk line. The board
// draws the need (NEEDS YOU), the matches (the cards), their ships and git
// (Task 12 step 6d).
func (m Model) roomRows(k chainKey) []roomRow {
	rows := m.cardRows(k)
	if seen, unseen := m.members(k); !k.isOne() && len(seen) == 0 && len(unseen) == 0 {
		rows = append(rows, roomRow{kind: roomCard, id: roomEmpty, text: "the chain has no matches · + start one here"})
	}
	return append(rows, roomRow{kind: roomTalk, text: m.talkText(k)})
}

// cardRows are the stoker's card: its order and plan, with the active
// step and one on each side, then the rest folded.
func (m Model) cardRows(k chainKey) []roomRow {
	var v proto.StokerView
	var title string
	switch u, ok := m.unitOf(k); {
	case k.isOne():
		title = "≋ the stoker · every session in no chain"
		if m.stoker != nil {
			v = *m.stoker
		}
	case ok:
		v = u.View
		if o := v.Order; o != nil && o.Words != "" {
			title = "≋ \"" + o.Words + "\""
		}
	}
	var rows []roomRow
	if title != "" {
		rows = append(rows, roomRow{kind: roomCard, text: title})
	}
	rows = append(rows, m.cardState(k, v)...)
	done, all := v.PlanProgress()
	if all == 0 {
		return rows
	}
	plan := v.Order.Plan
	lo, hi := 0, len(plan)
	if m.room == nil || m.room.k != k || !m.room.unfolded {
		lo, hi = max(0, done-1), min(len(plan), done+2)
	}
	for i := lo; i < hi; i++ {
		mark := " "
		switch {
		case i < done:
			mark = "✓"
		case i == done:
			mark = "▸"
		}
		row := roomRow{kind: roomStep, id: strconv.Itoa(plan[i].N), text: mark + " " + plan[i].Text}
		if _, waits := m.goOf(k, row); waits {
			row.text += " · waits for your go"
		}
		rows = append(rows, row)
	}
	if hidden := len(plan) - (hi - lo); hidden > 0 {
		rows = append(rows, roomRow{kind: roomMore, text: "↓ " + strconv.Itoa(hidden) + " more"})
	}
	return rows
}

// quietFor is how long a stoker that runs an order may say nothing before
// the card calls it quiet.
const quietFor = 60 * time.Second

// cardState is the card's state line, when it has one: the stoker did not
// start, it wakes, it asks, or it is quiet.
func (m Model) cardState(k chainKey, v proto.StokerView) []roomRow {
	if r := m.room; r != nil && r.k == k {
		u, ok := m.unitOf(k)
		switch {
		case r.failed != "":
			return []roomRow{{kind: roomCard, text: "the stoker did not start: " + r.failed, tone: "neg"}}
		case r.woke && (!ok || !chainAlive(u)):
			return []roomRow{{kind: roomCard, text: "≋ waking…"}}
		}
	}
	if u, ok := m.unitOf(k); ok {
		if s, ok := m.agentOf(k.host, u.ID); ok {
			if q, asks := m.headAsks(s); asks {
				return []roomRow{{kind: roomCard, tone: "warn", text: "≋ " + m.roomStokerWord(k) + " asks: " + q}}
			}
		}
	}
	o := v.Order
	if o == nil || o.State != "running" || v.Ready || !v.PaneUp {
		return nil
	}
	last := o.At
	for _, at := range append([]time.Time{o.Went}, runTimes(v.Run)...) {
		if at.After(last) {
			last = at
		}
	}
	if last.IsZero() || m.now().Sub(last) < quietFor {
		return nil
	}
	return []roomRow{{kind: roomCard, id: "quiet", tone: "warn",
		text: "≋ " + m.roomStokerWord(k) + " is quiet since " + last.Local().Format("15:04") + " · " + m.keys.room.Open.Help().Key + " asks it"}}
}

func runTimes(run []proto.StokerEntry) []time.Time {
	out := make([]time.Time, len(run))
	for i, e := range run {
		out[i] = e.At
	}
	return out
}

// talkText is the talk line: whom it talks to, or that the chain's stoker
// waits for its first order.
func (m Model) talkText(k chainKey) string {
	if k.isOne() {
		return "› tell the stoker…"
	}
	name := m.roomName(k)
	if u, ok := m.unitOf(k); ok && chainAlive(u) {
		return "› tell " + name + "'s stoker…"
	}
	return "› what should " + name + "'s stoker do?"
}

// roomKeyOf is the room of session s: its chain's (its first team's), the
// chain of a head row's stoker, else its chain of one.
func (m Model) roomKeyOf(s sample.Session) chainKey {
	if u, head := m.headRow(s); head {
		return chainKey{s.Host, u.Chain}
	}
	if teams := m.teamsOf(s); len(teams) > 0 {
		return chainKey{s.Host, teams[0].ID}
	}
	return oneKey(s)
}

// openRoom opens room k on the board: the board narrows to the chain, and
// the cursor lands on the need (the card that asks, or the step that
// waits for a go), else on the talk line.
func (m Model) openRoom(k chainKey) Model {
	m.room = &room{k: k, from: map[string]string{}}
	m.tab, m.boardAbove, m.boardBelow = tabSessions, 0, 0
	m.boardChain, m.peekAt = "", m.now()
	if !k.isOne() {
		m.boardChain = m.chainLabel(k)
	}
	m.room.at = m.roomLanding(k)
	if d := m.drafts[k]; d != "" { // words typed before the room closed: back on the talk line
		m.room.draft, m.room.at = d, len(m.roomRows(k))-1
	}
	return m
}

// roomLanding is the row the cursor lands on in room k: the step of the
// need's go; else -1, the board, with the card the need is on chosen; with
// no need, the talk line.
func (m *Model) roomLanding(k chainKey) int {
	rows := m.roomRows(k)
	talk := len(rows) - 1
	need, ok := m.needOf(k)
	if !ok {
		return talk
	}
	pane := need.pane
	if g := need.g; need.isGo {
		if g.step > 0 {
			step := strconv.Itoa(g.step)
			if i := slices.IndexFunc(rows, func(r roomRow) bool { return r.kind == roomStep && r.id == step }); i >= 0 {
				return i
			}
			r := *m.room
			r.unfolded = true // the step is folded away: the card opens to it
			m.room = &r
			rows = m.roomRows(k)
			if i := slices.IndexFunc(rows, func(r roomRow) bool { return r.kind == roomStep && r.id == step }); i >= 0 {
				return i
			}
			return len(rows) - 1
		}
		pane = g.pane
	}
	seen, _ := m.members(k)
	if i := slices.IndexFunc(seen, func(s sample.Session) bool { return s.Pane == pane }); i >= 0 {
		m.boardChooseCard(m.columnOf(seen[i]), seen[i])
		return -1
	}
	return talk
}

// keepRoomRow keeps the room's cursor on the row it was on, by its kind and
// id, when rows come or go under it. A tap that moved it moved it.
func (m Model) keepRoomRow(was *room, anchor roomRow) Model {
	if was == nil || m.room == nil || m.room.k != was.k || m.room.at != was.at || anchor.kind == roomNone {
		return m
	}
	rows := m.roomRows(m.room.k)
	i := slices.IndexFunc(rows, func(r roomRow) bool { return r.kind == anchor.kind && r.id == anchor.id })
	if i < 0 || i == m.room.at {
		return m
	}
	r := *m.room
	r.at = i
	m.room = &r
	return m
}
