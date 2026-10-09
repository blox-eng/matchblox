package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
)

// stokedRow is the first row of the Queue tab while the stoker has steps
// the builder has not seen. It sits with the doors, so it is selected,
// moved over and folded like one.
const stokedRow = "stoked"

func (m Model) stokedShows() bool {
	return m.stoker != nil && m.stoker.Unseen && len(m.stoker.Run) > 0
}

// stokerKey is f (the stoker on or off) and n (a night, or off).
func (m Model) stokerKey(night bool) (tea.Model, tea.Cmd) {
	if m.conn == nil || m.lost {
		m.flash = "not sent, no connection"
		return m, nil
	}
	if m.stoker == nil {
		m.flash = "this service keeps no stoker"
		return m, nil
	}
	verb, say := "on", "the stoker is on: full sessions compact"
	switch {
	case night && m.stoker.On && !m.stoker.Until.IsZero(), !night && m.stoker.On:
		verb, say = "off", "the stoker is off"
	case night:
		verb, say = "night", "night: the stoker runs until morning"
	}
	m.flash = say
	return m, m.send(proto.Act{RecID: "stoker:" + verb, Which: "primary"})
}

func (m Model) ackStoked() (tea.Model, tea.Cmd) {
	if m.conn == nil || m.lost {
		m.flash = "not sent, no connection"
		return m, nil
	}
	m.flash = "folded: the stoker's steps"
	return m, m.send(proto.Act{RecID: "stoker:ack", Which: "primary"})
}

// stokerWord is what the header shows while the stoker runs.
func (m Model) stokerWord() string {
	if m.stoker == nil || !m.stoker.On {
		return ""
	}
	if !m.stoker.Until.IsZero() {
		return "night → " + m.stoker.Until.Local().Format("15:04")
	}
	return "stoker"
}

// stokerShort is the word without the time, for a narrow header.
func (m Model) stokerShort() string {
	if w := m.stokerWord(); strings.HasPrefix(w, "night") {
		return "night"
	}
	return m.stokerWord()
}

func nightDoorAction(d doors.Door) *action {
	return &action{label: d.Title, say: "start the night: the stoker runs until morning", rec: "stoker:night", which: "primary"}
}

// stokedSection is the run the builder has not seen: each step, the
// context before and after a compact, and its RESUME line.
func (m Model) stokedSection(b *body, w int, row int, sel bool) {
	st, v := m.st, m.stoker
	when := v.Since.Local().Format("15:04") + " → "
	switch {
	case v.On && !v.Until.IsZero():
		when += v.Until.Local().Format("15:04")
	case v.On:
		when += "now"
	default:
		when += v.Ended.Local().Format("15:04")
	}
	b.add(-1, st.label.Render(fit(fmt.Sprintf(" STOKED %s · %d", when, len(v.Run)), w)))
	narrow := layout(w) == Narrow
	mark := "  "
	if sel {
		mark = "▌ "
	}
	var lines []string
	for i, e := range v.Run {
		lead := " " + st.accent.Render(mark)
		if i > 0 {
			lead = "   "
		}
		result := st.muted.Render(e.Result)
		if e.Result == "sent" {
			switch {
			case e.After > 0:
				result = st.text.Render(kTok(e.Before) + " → " + kTok(e.After))
			case m.now().Sub(e.At) < proto.StokerReadBack:
				result = st.muted.Render(kTok(e.Before) + " → compacting…")
			default:
				result = st.warn.Render(kTok(e.Before) + " → not read back")
			}
		}
		clock := st.muted.Render(e.At.Local().Format("15:04") + "  ")
		cell := m.stepCell(e)
		if narrow {
			lines = append(lines, lead+clock+cell+st.text.Render(e.Name), "     "+result)
		} else {
			lines = append(lines, lead+clock+cell+st.text.Render(pad(e.Name, colName))+st.muted.Render(pad(e.Pane, colPane))+result)
		}
		if e.Result != "sent" || e.After == 0 {
			continue
		}
		resume := "no RESUME line"
		style := st.warn.Render
		if e.Resume != "" {
			resume, style = "RESUME: "+e.Resume, st.muted.Render
		}
		for _, l := range wrap(resume, w-7, style) {
			lines = append(lines, "      "+strings.TrimPrefix(l, " "))
		}
	}
	b.addRow(row, sel, w, st, lines...)
}

// stepCell is the match of the session a step went to, as it is now; a
// session that is gone shows spent.
func (m Model) stepCell(e proto.StokerEntry) string {
	for _, s := range m.snap.Sessions {
		if s.SessionID == e.Session {
			return m.matchCell(s)
		}
	}
	return m.st.faint.Render("│") + " "
}
