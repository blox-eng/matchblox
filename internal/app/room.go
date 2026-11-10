package app

import (
	"cmp"
	"fmt"
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
}

type roomKind int

const (
	roomNeed roomKind = iota
	roomCard
	roomStep
	roomMore
	roomMatch
	roomShips
	roomGit
	roomTalk
)

// roomRow is one row of a room: what it is, the id it acts on (a step's
// number, a match's pane), its words, and faint for a mate the sampler
// cannot see.
type roomRow struct {
	kind  roomKind
	id    string
	text  string
	faint bool
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

// needOf is the oldest need of room k: an ask of one of its matches, or a
// step of its stoker that waits for a go; and how many more wait.
func (m Model) needOf(k chainKey) (roomRow, int, bool) {
	type need struct {
		row roomRow
		at  int64
	}
	var needs []need
	seen, _ := m.members(k)
	for _, s := range seen {
		if it, ok := m.waitOf(s); ok && (it.State == queue.StatePermission || it.State == queue.StateQuestion) {
			text := it.Name + " asks"
			if it.LastLine != "" {
				text += ": " + it.LastLine
			}
			needs = append(needs, need{roomRow{kind: roomNeed, id: it.Pane, text: text}, it.Since.UnixNano()})
		}
	}
	for _, g := range m.goWaits(k) {
		needs = append(needs, need{roomRow{kind: roomNeed, id: g.id, text: "▸ " + g.text + " waits for your go · y"}, g.at})
	}
	if len(needs) == 0 {
		return roomRow{}, 0, false
	}
	slices.SortStableFunc(needs, func(a, b need) int { return cmp.Compare(a.at, b.at) })
	return needs[0].row, len(needs) - 1, true
}

// roomRows are room k's rows, top down: the need, the stoker's card and its
// steps, the matches, ships, git and the talk line.
func (m Model) roomRows(k chainKey) []roomRow {
	var rows []roomRow
	if need, _, ok := m.needOf(k); ok {
		rows = append(rows, need)
	}
	rows = append(rows, m.cardRows(k)...)
	seen, unseen := m.members(k)
	rows = append(rows, m.roomMatchRows(k, seen, unseen)...)
	rows = append(rows, m.shipsRow(seen), m.gitRow(k.host, seen), roomRow{kind: roomTalk, text: m.talkText(k)})
	return rows
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
		rows = append(rows, roomRow{kind: roomStep, id: strconv.Itoa(plan[i].N), text: mark + " " + plan[i].Text})
	}
	if hidden := len(plan) - (hi - lo); hidden > 0 {
		rows = append(rows, roomRow{kind: roomMore, text: "↓ " + strconv.Itoa(hidden) + " more"})
	}
	return rows
}

// roomMatchRows are one row a match: its agent's mark, its name, its repo when
// the team spans two, its state, its inbox and its other teams. A mate the
// sampler cannot see stays, faint.
func (m Model) roomMatchRows(k chainKey, seen []sample.Session, unseen []chains.Member) []roomRow {
	repos := map[string]bool{}
	for _, s := range seen {
		repos[m.repoOf(s)] = true
	}
	var mates []state.MateView
	if cv := m.chainsOf(k.host); cv != nil {
		mates = cv.Mates
	}
	mateOf := func(mb chains.Member) state.MateView {
		if i := slices.IndexFunc(mates, func(v state.MateView) bool { return v.Member.Same(mb) }); i >= 0 {
			return mates[i]
		}
		return state.MateView{}
	}
	var rows []roomRow
	for _, s := range seen {
		text := s.Name
		if mark := providerMarks[s.Provider]; mark != "" {
			text = mark + " " + text
		}
		if repo := m.repoOf(s); len(repos) > 1 && repo != s.Name {
			text += "  " + repo
		}
		text += "  " + m.stateWord(s)
		if n := mateOf(memberOf(s)).Inbox; n > 0 {
			text += "  ✉ " + strconv.Itoa(n)
		}
		var also []string
		for _, c := range m.teamsOf(s) {
			if c.ID != k.id {
				also = append(also, c.Name)
			}
		}
		if len(also) > 0 {
			text += " · also in " + strings.Join(also, ", ")
		}
		rows = append(rows, roomRow{kind: roomMatch, id: s.Pane, text: text})
	}
	for _, mb := range unseen {
		text := mb.Pane + "  not seen"
		if at := mateOf(mb).Unseen; !at.IsZero() {
			text += " · " + shortAgo(m.now().Sub(at))
		}
		rows = append(rows, roomRow{kind: roomMatch, id: mb.Pane, text: text, faint: true})
	}
	return rows
}

// shortAgo is a wait in whole minutes, hours or days: 4m, 2h, 3d.
func shortAgo(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", max(0, int(d/time.Minute)))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
}

// shipsRow is the chain's pull requests on one line.
func (m Model) shipsRow(seen []sample.Session) roomRow {
	var parts []string
	got := map[string]bool{}
	for _, s := range seen {
		for _, sh := range m.shipsOf(s) {
			key := sh.Repo + "#" + strconv.Itoa(sh.Number)
			if got[key] {
				continue
			}
			got[key] = true
			word := stageNames[min(sh.Stage, stageLive)]
			switch sh.Kind {
			case shipStuck:
				word = "stuck"
			case shipWaits:
				word = "waits"
			}
			parts = append(parts, "#"+strconv.Itoa(sh.Number)+" "+word)
		}
	}
	if len(parts) == 0 {
		return roomRow{kind: roomShips, text: "SHIPS  no ships yet"}
	}
	return roomRow{kind: roomShips, text: "SHIPS  " + strings.Join(parts, " · ")}
}

// gitRow counts the worktrees of the repos the chain's matches work in,
// with the ones no session works in (eng review correction 4).
func (m Model) gitRow(host string, seen []sample.Session) roomRow {
	var r *proto.GitReport
	for _, g := range m.gitReports() {
		if g.host == host {
			r = g.r
		}
	}
	var n, merged, safe int
	if r != nil {
		for _, repo := range r.Repos {
			if !slices.ContainsFunc(seen, func(s sample.Session) bool { return inRepo(repo.Worktrees, s.Cwd) }) {
				continue
			}
			for _, wt := range repo.Worktrees {
				n++
				if wt.Merged {
					merged++
				}
				if wt.Safe {
					safe++
				}
			}
		}
	}
	if n == 0 {
		return roomRow{kind: roomGit, text: "GIT  no worktrees"}
	}
	parts := []string{plural(n, "worktree")}
	if merged > 0 {
		parts = append(parts, strconv.Itoa(merged)+" merged")
	}
	if safe > 0 {
		parts = append(parts, strconv.Itoa(safe)+" safe to remove")
	}
	return roomRow{kind: roomGit, text: "GIT  " + strings.Join(parts, " · ")}
}

// inRepo tells if dir is one of the worktrees, or inside one.
func inRepo(wts []proto.Worktree, dir string) bool {
	return dir != "" && slices.ContainsFunc(wts, func(wt proto.Worktree) bool {
		return dir == wt.Path || strings.HasPrefix(dir, wt.Path+"/")
	})
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

// openRoom opens room k with the cursor on its need, else on its talk line.
func (m Model) openRoom(k chainKey) Model {
	rows := m.roomRows(k)
	at := len(rows) - 1
	if i := slices.IndexFunc(rows, func(r roomRow) bool { return r.kind == roomNeed }); i >= 0 {
		at = i
	}
	m.room = &room{k: k, at: at, from: map[string]string{}}
	return m
}
