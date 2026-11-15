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

// sideFoot is the panel's last line: a new session, the tips, and the
// setup steps still open.
func (m Model) sideFoot(w int) string {
	st := m.st
	parts := []string{st.accent.Render("+ new")}
	if n := len(m.recs); n > 0 {
		word := " tips"
		if n == 1 {
			word = " tip"
		}
		parts = append(parts, st.muted.Render("✧ "+strconv.Itoa(n)+word))
	}
	if n := len(m.boardDoors()); n > 0 {
		parts = append(parts, st.warn.Render("! "+strconv.Itoa(n)+" setup"))
	}
	return fit(" "+strings.Join(parts, st.faint.Render(" · ")), w)
}

// sidePanel is the list in h lines of w cells: each group under its head,
// a thin line between groups, and the foot on the last line. The list
// scrolls to keep the chosen row in view.
func (m Model) sidePanel(w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	st := m.st
	chosen := m.chosenSide()
	var lines []string
	at := map[int]bool{} // the lines of the chosen row
	add := func(l string, hit bool) {
		if hit {
			at[len(lines)] = true
		}
		lines = append(lines, l)
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
			add(st.hair.Render(strings.Repeat("─", w)), false)
		}
		add(fit(" "+st.label.Render(g.state.head()+" · "+strconv.Itoa(m.sideCount(g, units)))+st.faint.Render("  −"), w), false)
		for _, u := range units {
			lit := slices.ContainsFunc(u.members, func(s sample.Session) bool { return paneOf(s) == chosen })
			if u.chain != (chainKey{}) {
				add(m.chainHead(u, w, lit), false)
			}
			for i, s := range u.members {
				link := sideLink{}
				if u.chain != (chainKey{}) {
					link = sideLink{i: i, n: len(u.members), lit: lit}
				}
				for _, l := range m.sideRow(s, w, paneOf(s) == chosen, link) {
					add(l, paneOf(s) == chosen)
				}
			}
		}
	}
	if len(lines) == 0 {
		lines = m.sideWords(w)
	}
	room := max(h-1, 0)
	start := 0
	for i := range lines {
		if at[i] && i >= room {
			start = max(i-room+1, start)
		}
	}
	lines = lines[min(start, len(lines)):]
	lines = lines[:min(len(lines), room)]
	for len(lines) < room {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return append(lines, m.sideFoot(w))
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
