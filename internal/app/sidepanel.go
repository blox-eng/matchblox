package app

import (
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
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
		return !slices.ContainsFunc(u.members, m.inBoard)
	})
}

// The foot's parts, as a tap on each names it.
const (
	footNew   = "foot:new"
	footTips  = "foot:tips"
	footSetup = "foot:setup"
)

// sideFoot is the panel's last line: a new session, the tips, and the
// setup steps still open. The spans are where a tap opens each part.
func (m Model) sideFoot(w int) (string, []span) {
	st := m.st
	type part struct{ text, press string }
	parts := []part{{st.accent.Render("+ new"), footNew}}
	if n := len(m.recs); n > 0 {
		word := " tips"
		if n == 1 {
			word = " tip"
		}
		parts = append(parts, part{st.muted.Render("✧ " + strconv.Itoa(n) + word), footTips})
	}
	if n := len(m.boardDoors()); n > 0 {
		parts = append(parts, part{st.warn.Render("! " + strconv.Itoa(n) + " setup"), footSetup})
	}
	line, x := " ", 1
	var spans []span
	for i, p := range parts {
		if i > 0 {
			line += st.faint.Render(" · ")
			x += 3
		}
		pw := ansi.StringWidth(p.text)
		if x+pw <= w {
			spans = append(spans, span{x, x + pw, p.press})
		}
		line, x = line+p.text, x+pw
	}
	return fit(line, w), spans
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
	for _, g := range groups {
		units := m.sideShows(g)
		if len(units) == 0 {
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
			lit := slices.ContainsFunc(u.members, func(s sample.Session) bool { return paneOf(s) == chosen })
			if u.chain != (chainKey{}) {
				add(m.chainHead(u, w, lit), sideHit{}, false)
			}
			for i, s := range u.members {
				link := sideLink{}
				if u.chain != (chainKey{}) {
					link = sideLink{i: i, n: len(u.members), lit: lit}
				}
				for _, l := range m.sideRow(s, w, paneOf(s) == chosen, link) {
					add(l, sideHit{k: paneOf(s), state: g.state}, paneOf(s) == chosen)
				}
			}
		}
	}
	if len(lines) == 0 {
		for _, l := range m.sideWords(w) {
			add(l, sideHit{}, false)
		}
	}
	room := max(h-1, 0)
	start := 0
	for i := range lines {
		if at[i] && i >= room {
			start = max(i-room+1, start)
		}
	}
	start = min(start, len(lines))
	lines, hits = lines[start:], hits[start:]
	lines, hits = lines[:min(len(lines), room)], hits[:min(len(hits), room)]
	for len(lines) < room {
		lines, hits = append(lines, strings.Repeat(" ", w)), append(hits, sideHit{})
	}
	foot, spans := m.sideFoot(w)
	return append(lines, foot), append(hits, sideHit{}), spans
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
