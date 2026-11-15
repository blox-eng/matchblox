package app

import (
	"cmp"
	"strconv"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
)

// sideLink is a row's place in a chain: member i of n, lit while the
// chosen session is in the chain. The zero value is a row in no chain.
type sideLink struct {
	i, n int
	lit  bool
}

// linkOf is the link glyph of member i of n, and the one under it when the
// row has a second line.
func linkOf(i, n int, twoLines bool) (first, rest string) {
	if i == n-1 {
		return "└", " "
	}
	if twoLines {
		return "├", "│"
	}
	return "├", " "
}

// sideRow is a session's row: line 1 holds the selection mark, the link,
// the match, the name, the wait or the badge, and the agent's mark. A
// second line shows what a session that needs you asks, or what the chosen
// session does, or why a held one is held. The row of a host that does not
// answer is its last state, dimmed.
func (m Model) sideRow(s sample.Session, w int, chosen bool, link sideLink) []string {
	h := m.sideStateOf(s)
	word, says := m.cardSays(s)
	switch {
	case h == sideHeld:
		says = m.heldWhy(s)
	case says == "" && h == sideNeed:
		says = cmp.Or(m.askedOf(s), word)
	case says == "":
		says = word
	}
	two := says != "" && (h == sideNeed || chosen)

	mark, name, rest := " ", m.st.text, m.st.muted
	if m.stale(s.Host) { // its host does not answer: the row is its last state, dimmed
		name, rest = m.st.faint, m.st.faint
	}
	if chosen {
		mark, name = m.st.accent.Render("▌"), name.Bold(true)
	} else if h == sideNeed && !m.stale(s.Host) {
		name = name.Bold(true)
	}
	ink := m.st.faint
	if link.lit {
		ink = m.st.accent
	}
	lead, cont := "", ""
	if link.n > 0 {
		first, rest := linkOf(link.i, link.n, two)
		lead, cont = ink.Render(first)+" ", ink.Render(rest)+" "
	}

	right := rest.Render(m.sideRight(s)) + " " + rest.Render(harnessMark(s))
	title := m.sideTitle(s)
	left := mark + lead + m.sideMatch(h) + " "
	room := w - lipgloss.Width(left+right) - 1
	line := spread(left+name.Render(ansi.Truncate(title, max(room, 1), "…")), right, w)
	if !two {
		return []string{line}
	}
	pad := " " + cont + "  "
	text := ansi.Truncate(says, max(w-lipgloss.Width(pad), 0), "…")
	return []string{line, fit(pad+m.st.faint.Render(text), w)}
}

// sideRight is the wait of a session, or its pull request's badge when it
// waits for nothing: the number and the stage word.
func (m Model) sideRight(s sample.Session) string {
	if _, waits := m.waitOf(s); !waits {
		if ships := m.shipsOf(s); len(ships) > 0 {
			return "#" + strconv.Itoa(ships[0].Number) + " " + stageNames[ships[0].Stage]
		}
	}
	return m.waitCell(s)
}

// chainHead is the head row of a chain: "≋ login   step 2 of 3 · 3".
func (m Model) chainHead(u sideUnit, w int, lit bool) string {
	n, of := u.step()
	right := "step " + strconv.Itoa(n) + " of " + strconv.Itoa(of) + " · " + strconv.Itoa(len(u.members))
	name := m.st.text.Bold(true)
	if lit {
		name = m.st.accent.Bold(true)
	}
	left := " " + name.Render(ansi.Truncate("≋ "+m.chainLabel(u.chain), max(w-lipgloss.Width(right)-2, 1), "…"))
	return spread(left, m.st.muted.Render(right)+" ", w)
}

// askedOf is what a session asks, whether or not a hold covers it.
func (m Model) askedOf(s sample.Session) string {
	if s.Ask != nil && len(s.Ask.Questions) > 0 {
		return s.Ask.Questions[0].Question
	}
	if s.WaitingFor != "" {
		return s.WaitingFor
	}
	q, _ := m.headAsks(s)
	return q
}
