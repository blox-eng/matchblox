package app

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
)

// stokedRow is the first row of the Queue tab while the stoker has steps
// the builder has not seen. It sits with the doors, so it is selected,
// moved over and folded like one.
const stokedRow = "stoked"

func (m Model) stokedShows() bool {
	return m.stoker != nil && m.stoker.Unseen && (len(m.stoker.Run) > 0 || m.card() != nil)
}

// card is the score of the run that ended, while no run is on.
func (m Model) card() *proto.StokerCard {
	if m.stoker == nil || m.stoker.On {
		return nil
	}
	return m.stoker.Card
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
	case !night && m.resting():
		verb, say = "wake", "the stoker is back on"
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
	return stokerWordOf(m.stoker.Mode)
}

func stokerWordOf(md proto.StokerMode) string {
	if !md.Rests.IsZero() {
		return "stoker rests"
	}
	if !md.Until.IsZero() {
		return "night → " + md.Until.Local().Format("15:04")
	}
	return "stoker"
}

// stokerShort is the word without the time, for a narrow header.
func (m Model) stokerShort() string {
	switch w := m.stokerWord(); {
	case strings.HasPrefix(w, "night"):
		return "night"
	case w == "stoker rests":
		return "rests"
	}
	return m.stokerWord()
}

// resting: the builder took over and the stoker waits for them to leave.
func (m Model) resting() bool {
	return m.stoker != nil && m.stoker.On && !m.stoker.Rests.IsZero()
}

// asksTakeOver: k acts on a session while the stoker is in charge, and the
// builder has not been asked in this run. Looking around never asks.
func (m Model) asksTakeOver(k string) bool {
	if !m.night() || m.stoker.Asked || m.pending != nil {
		return false
	}
	if _, ok := m.selectedDoor(); ok {
		return false
	}
	switch k {
	case "enter":
		return m.primary() != nil
	case "x":
		return m.secondary() != nil
	case "a":
		pane, _ := m.answerTarget()
		return pane != ""
	}
	return false
}

// answerTakeOver is the builder's answer: y rests the stoker, n keeps it
// on; either way what they pressed runs. Any other key drops the question
// and what was held.
func (m Model) answerTakeOver(k string) (tea.Model, tea.Cmd) {
	if k != "y" && k != "n" {
		m.held, m.flash = nil, ""
		return m, nil
	}
	if m.conn == nil || m.lost {
		m.flash = "not sent, no connection · " + k + " tries again"
		return m, nil
	}
	held := m.held
	m.held = nil
	v := *m.stoker
	v.Asked = true
	verb, say := "keep", "the stoker stays on"
	if k == "y" {
		verb, say = "rest", "stoker rests · f turns it back on"
		v.Rests, v.Touched = m.now(), m.now()
		m.touched = m.now()
	}
	m.stoker = &v
	send := m.send(proto.Act{RecID: "stoker:" + verb, Which: "primary"})
	next, run := held(m)
	if nm, ok := next.(Model); ok && nm.flash == "" {
		nm.flash = say
		next = nm
	}
	return next, tea.Batch(send, run)
}

// touchRest tells the service, at most once a minute, that the builder is
// still at the console, so the stoker goes on resting.
func (m Model) touchRest() (Model, tea.Cmd) {
	if !m.resting() || m.conn == nil || m.lost || m.now().Sub(m.touched) < time.Minute {
		return m, nil
	}
	m.touched = m.now()
	return m, m.send(proto.Act{RecID: "stoker:touch", Which: "primary"})
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
	narrow := layout(w) == Narrow
	mark := "  "
	if sel {
		mark = "▌ "
	}
	var lines []string
	head := fmt.Sprintf(" STOKED %s · %d", when, len(v.Run))
	if c := m.card(); c != nil {
		head = fmt.Sprintf(" STOKED %s → %s · ready %d/%d", c.Since.Local().Format("15:04"), c.Ended.Local().Format("15:04"), c.Ready, c.Of)
		if c.Night {
			head = " NIGHT" + strings.TrimPrefix(head, " STOKED")
		}
		for i, l := range cardLines(*c, narrow) {
			lead := "   "
			if i == 0 {
				lead = " " + st.accent.Render(mark)
			}
			lines = append(lines, lead+st.text.Render(l))
		}
		for _, x := range c.NotReady {
			lines = append(lines, "   "+st.text.Render(pad(x.Name, colName))+st.warn.Render(x.Why))
		}
	}
	b.add(-1, st.label.Render(fit(head, w)))
	for i, e := range v.Run {
		lead := " " + st.accent.Render(mark)
		if i > 0 || len(lines) > 0 {
			lead = "   "
		}
		result := st.muted.Render(e.Result)
		if e.Result == "sent" {
			switch {
			case e.After > 0:
				result = st.text.Render(kTok(e.Before) + " → " + kTok(e.After))
			case !e.Back.IsZero():
				result = st.text.Render(kTok(e.Before) + " → compacted")
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
		if e.Result != "sent" || e.Back.IsZero() {
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

// cardLines are the counts of a card: one line, or two on a phone.
func cardLines(c proto.StokerCard, narrow bool) []string {
	full := fmt.Sprintf("%d were full", c.Full)
	if c.Full == 1 {
		full = "1 was full"
	}
	given := ""
	if c.Given > 0 {
		given = " · " + kTok(c.Given) + " tokens given back"
	}
	if narrow {
		if given != "" {
			given = " · " + kTok(c.Given) + " given back"
		}
		return []string{
			fmt.Sprintf("%d watched · %d full · %d compacted", c.Watched, c.Full, c.Compacted),
			fmt.Sprintf("%d waited", c.Waited) + given,
		}
	}
	return []string{fmt.Sprintf("%d watched · %s · %d compacted · %d waited", c.Watched, full, c.Compacted, c.Waited) + given}
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
