package app

import (
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
)

// roomTitle takes the tab row's place in a room: the way back and the
// chain's name, cut to w.
func (m Model) roomTitle(w int) string {
	if m.room == nil {
		return ""
	}
	line := m.st.muted.Render(" ‹ ≋ ") + m.st.text.Render(m.roomName(m.room.k))
	return ansi.Truncate(line, w, "…")
}

// roomView is the room's rows as lines w wide, at most h of them. The
// accent marks the need line; with no need, the selected row (rule 4).
func (m Model) roomView(w, h int) []string {
	if m.room == nil {
		return nil
	}
	k := m.room.k
	rows := m.roomRows(k)
	_, more, needs := m.needOf(k)
	var lines []string
	for i, r := range rows {
		sel := i == m.room.at
		mark := " "
		if sel {
			if needs {
				mark = m.st.text.Render("▌")
			} else {
				mark = m.st.accent.Render("▌")
			}
		}
		for _, l := range m.roomLine(r, w-1, sel && !needs, more) {
			lines = append(lines, ansi.Truncate(mark+l, w, "…"))
			mark = " "
		}
	}
	if len(lines) > h {
		lines = lines[:max(h, 0)]
	}
	return lines
}

// roomLine is one row's line, or two for a need that wraps. lit: the row
// is selected and no need holds the accent.
func (m Model) roomLine(r roomRow, w int, lit bool, more int) []string {
	st := m.st
	plain := st.text
	if lit {
		plain = st.accent
	}
	switch r.kind {
	case roomNeed:
		text := r.text
		if more > 0 {
			text += "  +" + strconv.Itoa(more)
		}
		wrapped := strings.Split(ansi.Wordwrap("╿ "+text, max(w, 1), " "), "\n")
		if len(wrapped) > 2 {
			wrapped = wrapped[:2]
			wrapped[1] = ansi.Truncate(wrapped[1]+" …", w, "…")
		}
		for i := range wrapped {
			if i > 0 {
				wrapped[i] = "  " + wrapped[i]
			}
			wrapped[i] = st.accent.Render(wrapped[i])
		}
		return wrapped
	case roomCard:
		switch r.tone {
		case "neg":
			return []string{st.neg.Render(r.text)}
		case "warn":
			return []string{st.warn.Render(r.text)}
		}
		if rest, ok := strings.CutPrefix(r.text, "≋ "); ok && m.inCharge() {
			// A chain stoker in charge paints only its own glyph in the
			// night's ink; the console wears the night for the host's run.
			return []string{lipgloss.NewStyle().Foreground(nightInk).Render("≋") + st.muted.Render(" "+rest)}
		}
		return []string{st.muted.Render(r.text)}
	case roomStep:
		mark, text, _ := strings.Cut(r.text, " ")
		switch mark {
		case "✓":
			return []string{"  " + st.muted.Render(r.text)}
		case "▸":
			return []string{"  " + plain.Render(mark+" "+text)}
		}
		return []string{"  " + st.muted.Render(r.text)}
	case roomMore:
		return []string{"  " + st.faint.Render(r.text)}
	case roomMatch:
		if r.faint {
			return []string{"  " + st.faint.Render(r.text)}
		}
		cell := "  "
		if s, ok := m.roomSession(r.id); ok {
			cell = m.matchCell(s)
		}
		return []string{cell + plain.Render(r.text)}
	case roomShips, roomGit:
		label, text, _ := strings.Cut(r.text, "  ")
		return []string{st.faint.Render(padTo(label, 6)) + plain.Render(text)}
	case roomTalk:
		if m.room != nil && m.room.typing {
			return []string{st.accent.Render("›") + st.text.Render(" "+m.room.draft) + st.accent.Render("▏")}
		}
		if lit {
			return []string{st.accent.Render(r.text)}
		}
		return []string{st.muted.Render(r.text)}
	}
	return []string{r.text}
}

// inCharge tells if the room's chain stoker runs its order now.
func (m Model) inCharge() bool {
	u, ok := m.unitOf(m.room.k)
	return ok && !m.room.k.isOne() && chainAlive(u) && u.View.On && u.View.Rests.IsZero()
}

// roomSession is the session of a match row of the room on screen.
func (m Model) roomSession(pane string) (sample.Session, bool) {
	if m.room == nil {
		return sample.Session{}, false
	}
	seen, _ := m.members(m.room.k)
	i := slices.IndexFunc(seen, func(s sample.Session) bool { return s.Pane == pane })
	if i < 0 {
		return sample.Session{}, false
	}
	return seen[i], true
}

// roomScreen draws the room in the body: on a desk the left 40%, with the
// selected match's live view on the right; on a phone the whole width.
func (m Model) roomScreen(w int, sc drawn) drawn {
	sc.lines[1] = m.roomTitle(w)
	rows := m.bodyRows()
	leftW, rightW := w, 0
	if _, right := m.panes(w); right > 0 {
		leftW = max(w*2/5, viewLeast)
		rightW = w - leftW - 1
	}
	left := m.roomView(leftW, rows)
	var right []string
	if rightW > 0 {
		if s, ok := m.selected(); ok && m.roomAtMatch(s.Pane) {
			right = m.viewLines(rightW, rows)
		}
	}
	for i := range rows {
		line := ""
		if i < len(left) {
			line = left[i]
		}
		if rightW > 0 {
			r := ""
			if i < len(right) {
				r = right[i]
			}
			line = fit(line, leftW) + m.st.hair.Render("│") + fit(r, rightW)
		}
		sc.lines, sc.rows = append(sc.lines, line), append(sc.rows, -1)
	}
	sc.listW = leftW
	hair, keys, keySpans := m.keyRows(w)
	sc.keysAt, sc.keys = len(sc.lines)+1, keySpans
	sc.lines = append(sc.lines, hair, keys)
	if m.consenting() {
		m.overlayConsent(&sc, w)
	}
	return sc
}

// roomAtMatch tells if the room's cursor is on the match in pane.
func (m Model) roomAtMatch(pane string) bool {
	rows := m.roomRows(m.room.k)
	return m.room.at < len(rows) && rows[m.room.at].kind == roomMatch && rows[m.room.at].id == pane
}
