package app

import (
	"cmp"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
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
// the match, the name, the badge, the time in state and the agent's mark. A
// second line shows what a session that needs you asks, what a done one said
// last, what the chosen session does, or why a held one is held. The row of a host that does not
// answer is its last state, dimmed.
func (m Model) sideRow(s sample.Session, w int, chosen bool, link sideLink) []string {
	h := m.sideStateOf(s)
	word, says := m.cardSays(s)
	switch {
	case h == sideHeld:
		says = m.heldWhy(s)
	case h == sideDone:
		says = m.lastWords(s)
	case says == "" && h == sideNeed:
		says = cmp.Or(m.askedOf(s), word)
	case says == "":
		says = word
	}
	pending, isPending := m.pendingOf(s)
	switch {
	case !isPending:
	case pending.kind == actKill:
		says = "killing"
	case pending.kind == actPark && pending.park:
		says = "parking"
	case pending.kind == actPark:
		says = "letting go"
	}
	two := says != "" && (h == sideNeed || h == sideDone || chosen || isPending)

	mark, name, rest := " ", m.st.text, m.st.muted
	if m.stale(s.Host) || isPending { // its host does not answer, or an act on it waits for the sample: dimmed
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

	right := rest.Render(m.sideRight(s))
	title := m.sideTitle(s)
	left := mark + lead + m.sideMatch(s, h) + " "
	if m.chainPlace(s) >= 0 {
		left += m.st.accent.Render("▸") + " "
	}
	room := w - lipgloss.Width(left+right) - 1
	line := spread(left+name.Render(ansi.Truncate(title, max(room, 1), "…")), right, w)
	if !two {
		return []string{line}
	}
	pad := " " + cont + "  "
	text := ansi.Truncate(says, max(w-lipgloss.Width(pad), 0), "…")
	return []string{line, fit(pad+m.st.faint.Render(text), w)}
}

// sideRight is a row's right edge: its pull request's badge when it waits
// for nothing, how long it has been in its state (or, while the plan's limit
// stops it, when it can work again), and the agent's mark.
func (m Model) sideRight(s sample.Session) string {
	var at string
	switch {
	case s.Stopped():
		at = m.backAt(s)
	case !s.StateAt.IsZero():
		at = ageOf(m.now().Sub(s.StateAt))
	}
	if _, waits := m.waitOf(s); !waits {
		if ships := m.shipsOf(s); len(ships) > 0 {
			at = strings.TrimSuffix("#"+strconv.Itoa(ships[0].Number)+" "+stageNames[ships[0].Stage]+"  "+at, "  ")
		}
	}
	return strings.TrimPrefix(at+" "+harnessMark(s), " ")
}

// ageOf is a time in state: "now" under a minute, else its first unit.
func ageOf(d time.Duration) string {
	if d < time.Minute {
		return "now"
	}
	return brief(d)
}

// chainHead is the head row of a chain: "≋ login   step 2 of 3 · 3". With
// members that wait for recover it reads "step 2 of 3 · 2 + 1 wait", and
// "3 wait" when all wait.
func (m Model) chainHead(u sideUnit, w int, lit bool) string {
	n, of := u.step()
	count := "step " + strconv.Itoa(n) + " of " + strconv.Itoa(of) + " · " + strconv.Itoa(len(u.members))
	waits := m.st.wait.Render(strconv.Itoa(len(u.waiting)) + " wait")
	right := m.st.muted.Render(count)
	switch {
	case len(u.members) == 0:
		right = waits
	case len(u.waiting) > 0:
		right = m.st.muted.Render(count+" + ") + waits
	}
	name := m.st.text.Bold(true)
	if lit {
		name = m.st.accent.Bold(true)
	}
	left := " " + name.Render(ansi.Truncate("≋ "+m.chainLabel(u.chain), max(w-lipgloss.Width(right)-2, 1), "…"))
	return spread(left, right+" ", w)
}

// waitRow is the row of a chain member that waits for recover: the link,
// ◌ and its name, and "waits for recover", in the wait color. The chosen
// row has a second line: when its server stopped, and the keys. On a desk
// the console draws no chat, so this line carries the facts.
func (m Model) waitRow(v state.MateView, w int, link sideLink, chosen bool) []string {
	ink := m.st.faint
	if link.lit {
		ink = m.st.accent
	}
	mark := " "
	if chosen {
		mark = m.st.accent.Render("▌")
	}
	first, rest := linkOf(link.i, link.n, chosen)
	right := m.st.wait.Render("waits for recover") + " "
	left := mark + ink.Render(first) + " "
	room := w - lipgloss.Width(left+right) - 1
	line := spread(left+m.st.wait.Render(ansi.Truncate("◌ "+v.Wait.Name, max(room, 1), "…")), right, w)
	if !chosen {
		return []string{line}
	}
	at, _ := m.stoppedAt(v.Wait.Since)
	says := "its server stopped " + at + " · " + m.keys.side.Enter.Help().Key + " reopen · " + m.keys.board.Menu.Help().Key + " menu"
	pad := " " + ink.Render(rest) + "  "
	return []string{line, fit(pad+m.st.faint.Render(ansi.Truncate(says, max(w-lipgloss.Width(pad), 0), "…")), w)}
}

// stoppedAt is t in local time: the clock when t is today, else the day
// ("30 Sep"). today tells which.
func (m Model) stoppedAt(t time.Time) (at string, today bool) {
	t = t.Local()
	if t.Format(time.DateOnly) == m.now().Local().Format(time.DateOnly) {
		return t.Format("15:04"), true
	}
	return t.Format("2 Jan"), false
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
