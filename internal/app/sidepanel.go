package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// chosenSide is the session the list chooses: sideAt, or the first one
// drawn while sideAt names none.
func (m Model) chosenSide() paneKey {
	order := m.sideOrder()
	if slices.Contains(order, m.sideAt) {
		return m.sideAt
	}
	if len(order) > 0 {
		return order[0]
	}
	return paneKey{}
}

// sideShows is the units of g the search, the chain of g and the host of
// s keep: a chain stays whole while one of its members is kept.
func (m Model) sideShows(g sideGroup) []sideUnit {
	return slices.DeleteFunc(slices.Clone(g.units), func(u sideUnit) bool {
		if len(u.members) == 0 {
			return !m.waitShows(u)
		}
		return !slices.ContainsFunc(u.members, m.inBoard)
	})
}

// waitShows tells if the list shows unit u, whose members all wait: no
// search and no other chain narrows the list.
func (m Model) waitShows(u sideUnit) bool {
	r := m.room
	return m.filter == "" && (r == nil || !r.k.isOne()) &&
		(m.boardChain == "" || m.chainLabel(u.chain) == m.boardChain || (r != nil && r.k == u.chain))
}

// The foot's parts, as a tap on each names it.
const (
	footNew    = "foot:new"
	footTips   = "foot:tips"
	footSetup  = "foot:setup"
	footExited = "foot:exited"
	footHost   = "foot:host:" // then the host's routing key
)

// listFootPart is a part of the list's foot: its words, and what it opens.
type listFootPart struct{ text, press string }

// footParts are the parts the foot shows: a new session, the tips, the
// setup steps still open, and the exited agents while they are hidden.
func (m Model) footParts() []listFootPart {
	parts := []listFootPart{{"+ new", footNew}}
	if n := len(m.recs); n > 0 {
		word := " tips"
		if n == 1 {
			word = " tip"
		}
		parts = append(parts, listFootPart{"✧ " + strconv.Itoa(n) + word, footTips})
	}
	if n := len(m.boardDoors()); n > 0 {
		parts = append(parts, listFootPart{"! " + strconv.Itoa(n) + " setup", footSetup})
	}
	for _, h := range m.unreached() {
		word := " not reached"
		if _, ok := m.blockedOf(h); ok {
			word = " blocked"
		}
		parts = append(parts, listFootPart{"! " + m.nameOf(h) + word, footHost + h})
	}
	if n := len(m.snap.Exited); n > 0 && !m.showExited {
		parts = append(parts, listFootPart{strconv.Itoa(n) + " exited", footExited})
	}
	return parts
}

// sideFoot is the panel's last line, the foot's parts: the spans are where
// a tap opens each. The part the arrows chose is marked as a chosen row.
func (m Model) sideFoot(w int) (string, []span) {
	st := m.st
	ink := map[string]lipgloss.Style{footNew: st.accent, footTips: st.muted, footSetup: st.warn, footExited: st.faint}
	line, x := " ", 1
	if m.footAt > 0 {
		line = st.accent.Render("▌")
	}
	var spans []span
	for i, p := range m.footParts() {
		if i > 0 {
			line += st.faint.Render(" · ")
			x += 3
		}
		ink, ok := ink[p.press]
		if !ok { // a host's trouble
			ink = st.warn
		}
		text := ink.Render(p.text)
		if m.footAt == i+1 {
			text = st.accent.Bold(true).Render(p.text)
		}
		pw := ansi.StringWidth(p.text)
		if x+pw <= w {
			spans = append(spans, span{x, x + pw, p.press})
		}
		line, x = line+text, x+pw
	}
	return fit(line, w), spans
}

// exitedRows are the agents that exited and wait for their parent, dim at
// the end of IDLE while e shows them: each parent with its fix and pids.
func (m Model) exitedRows(w int) []string {
	type parent struct {
		pid  int
		comm string
	}
	var order []parent
	pids := map[parent][]string{}
	for _, e := range m.snap.Exited {
		p := parent{e.Parent, e.ParentComm}
		if _, ok := pids[p]; !ok {
			order = append(order, p)
		}
		pids[p] = append(pids[p], strconv.Itoa(e.PID))
	}
	st := m.st
	var out []string
	for _, p := range order {
		fix := "they go when it waits for them or exits"
		switch {
		case p.comm == "systemd" && p.pid == 1:
			fix = "sudo systemctl daemon-reexec reaps them" // the system manager
		case p.comm == "systemd":
			fix = "systemctl --user daemon-reexec reaps them"
		}
		out = append(out, fit(" "+st.faint.Render(fmt.Sprintf("╵ %d exited · %s pid %d", len(pids[p]), p.comm, p.pid)), w))
		for _, l := range wrapPlain(fix, w-3) {
			out = append(out, fit("   "+st.muted.Render(l), w))
		}
		out = append(out, fit("   "+st.faint.Render("pids "+strings.Join(pids[p], ", ")), w))
	}
	return out
}

// sideHit is what one line of the list holds: a session's row, a group's
// head, or nothing (the zero value).
type sideHit struct {
	k     paneKey
	head  bool
	state sideState
}

// sidePanel is the list in h lines of w cells: each group under its head,
// a thin line between groups, and the foot on the last line. The list
// scrolls to keep the chosen row in view.
func (m Model) sidePanel(w, h int) []string {
	lines, _, _ := m.sideList(w, h)
	return lines
}

// sideList is sidePanel, with what each line holds and where the foot's
// parts are. A folded group keeps its head and its count, with a +, and
// draws no row.
func (m Model) sideList(w, h int) ([]string, []sideHit, []span) {
	if w <= 0 || h <= 0 {
		return nil, nil, nil
	}
	st := m.st
	chosen := m.chosenSide()
	if m.footAt > 0 {
		chosen = paneKey{} // the foot's part is the chosen line
	}
	exited := m.showExited && len(m.snap.Exited) > 0
	var lines []string
	var hits []sideHit
	at := map[int]bool{} // the lines of the chosen row
	add := func(l string, hit sideHit, lit bool) {
		if lit {
			at[len(lines)] = true
		}
		lines, hits = append(lines, l), append(hits, hit)
	}
	groups := m.sideGroups()
	if !m.have {
		groups = nil
	}
	if exited && !slices.ContainsFunc(groups, func(g sideGroup) bool { return g.state == sideIdle }) {
		groups = append(groups, sideGroup{state: sideIdle}) // its head holds the exited agents
	}
	for _, g := range groups {
		units := m.sideShows(g)
		if len(units) == 0 && (!exited || g.state != sideIdle) {
			continue
		}
		if len(lines) > 0 {
			add(st.hair.Render(strings.Repeat("─", w)), sideHit{}, false)
		}
		fold := "  −"
		if m.sideFold[g.state] {
			fold = "  +"
		}
		add(fit(" "+st.label.Render(g.state.head()+" · "+strconv.Itoa(m.sideCount(g, units)))+st.faint.Render(fold), w), sideHit{head: true, state: g.state}, false)
		if m.sideFold[g.state] {
			continue
		}
		for _, u := range units {
			lit := slices.ContainsFunc(u.members, func(s sample.Session) bool { return paneOf(s) == chosen }) ||
				slices.ContainsFunc(u.waiting, func(v state.MateView) bool { return waitKey(u.chain, v) == chosen })
			if u.chain != (chainKey{}) {
				add(m.chainHead(u, w, lit), sideHit{}, false)
			}
			n := len(u.members) + len(u.waiting)
			for i, s := range u.members {
				link := sideLink{}
				if u.chain != (chainKey{}) {
					link = sideLink{i: i, n: n, lit: lit}
				}
				for _, l := range m.sideRow(s, w, paneOf(s) == chosen, link) {
					add(l, sideHit{k: paneOf(s), state: g.state}, paneOf(s) == chosen)
				}
			}
			for j, v := range u.waiting {
				k := waitKey(u.chain, v)
				for _, l := range m.waitRow(v, w, sideLink{i: len(u.members) + j, n: n, lit: lit}, k == chosen) {
					add(l, sideHit{k: k, state: g.state}, k == chosen)
				}
			}
		}
		if exited && g.state == sideIdle {
			for _, l := range m.exitedRows(w) {
				add(l, sideHit{}, false)
			}
		}
	}
	if len(lines) == 0 {
		for _, l := range m.sideWords(w) {
			add(l, sideHit{}, false)
		}
	}
	room := max(h-1, 0)
	startFor := func(r int) int {
		start := 0
		for i := range lines {
			if at[i] && i >= r {
				start = max(i-r+1, start)
			}
		}
		return min(start, len(lines))
	}
	// A cut list says what it hides: a line above, a line below, each
	// taken from the rows it shows.
	up, down, r, start := false, false, room, startFor(room)
	for range 3 {
		r = max(room-btoi(up)-btoi(down), 1)
		start = startFor(r)
		up, down = start > 0, start+r < len(lines)
	}
	if room < 3 {
		up, down, r, start = false, false, room, startFor(room)
	}
	above, below := hits[:start], hits[min(start+r, len(hits)):]
	lines, hits = lines[start:], hits[start:]
	lines, hits = lines[:min(len(lines), r)], hits[:min(len(hits), r)]
	if up {
		lines, hits = append([]string{m.cutMark("↑", above, w)}, lines...), append([]sideHit{{}}, hits...)
	}
	if down {
		lines, hits = append(lines, m.cutMark("↓", below, w)), append(hits, sideHit{})
	}
	for len(lines) < room {
		lines, hits = append(lines, strings.Repeat(" ", w)), append(hits, sideHit{})
	}
	foot, spans := m.sideFoot(w)
	return append(lines, foot), append(hits, sideHit{}), spans
}

// cutMark is the line of a cut list over or under the rows it shows: how
// many sessions it hides there, and how many of those need you.
func (m Model) cutMark(arrow string, hidden []sideHit, w int) string {
	seen := map[paneKey]bool{}
	need := 0
	for _, h := range hidden {
		if h.k == (paneKey{}) || seen[h.k] {
			continue
		}
		seen[h.k] = true
		if h.state == sideNeed {
			need++
		}
	}
	text := arrow + " " + strconv.Itoa(len(seen)) + " more"
	switch {
	case need == 1:
		text += " · 1 needs you"
	case need > 1:
		text += " · " + strconv.Itoa(need) + " need you"
	}
	ink := m.st.faint
	if need > 0 {
		ink = m.st.warn
	}
	return fit(" "+ink.Render(text), w)
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// sideWords is what the list says in place of rows: before the first
// state, and for a search that matches nothing.
func (m Model) sideWords(w int) []string {
	st := m.st
	switch {
	case !m.have:
		return []string{fit(" "+st.muted.Render("reading your matches…"), w)}
	case m.filter != "":
		text := `Nothing matches "` + m.filter + `". ` + m.keys.main.Back.Help().Key + " clears the search."
		return []string{fit(" "+st.muted.Render(ansi.Truncate(text, max(w-1, 1), "…")), w)}
	}
	return nil
}

// sideCount is the sessions of units in the state of g: all of g.count
// while no search narrows the list.
func (m Model) sideCount(g sideGroup, units []sideUnit) int {
	if m.filter == "" {
		return g.count
	}
	n := 0
	for _, u := range units {
		for _, s := range u.members {
			if m.inBoard(s) && m.sideStateOf(s) == g.state {
				n++
			}
		}
	}
	return n
}

// chosenMark is the first cell of a line: ▌ in the accent when it is
// chosen.
func (m Model) chosenMark(sel bool) string {
	if sel {
		return m.st.accent.Render("▌")
	}
	return " "
}
