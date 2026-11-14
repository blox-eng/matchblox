package app

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// roomLine is one row's line. lit: the row is selected.
func (m Model) roomLine(r roomRow, w int, lit bool) []string {
	st := m.st
	plain := st.text
	if lit {
		plain = st.accent
	}
	switch r.kind {
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
	case roomTalk:
		if m.room != nil && m.room.typing {
			return []string{st.accent.Render("›") + st.text.Render(" "+m.room.draft) + st.accent.Render("▏")}
		}
		if m.room != nil && m.room.draft != "" { // words typed and not sent: ⏎ types on
			r.text = "› " + m.room.draft
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
