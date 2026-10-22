package app

import (
	"fmt"
	"strings"
	"time"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// stokedShows: the stoker has steps, or the card of a run, the builder
// has not seen.
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

// stokerKey is f (the stoker off, or the launch sheet) and n (a night's
// launch sheet, or the night off).
func (m Model) stokerKey(night bool) (tea.Model, tea.Cmd) {
	if m.conn == nil || m.lost {
		m.flash = "not sent, no connection"
		return m, nil
	}
	if m.stoker == nil {
		m.flash = "this service keeps no stoker"
		return m, nil
	}
	var verb, say string
	switch {
	case !night && m.resting():
		verb, say = "wake", "the stoker is back on"
	case night && m.stoker.On && !m.stoker.Until.IsZero(), !night && m.stoker.On:
		verb, say = "off", "the stoker is off"
	default:
		return m.openSheet(night, false)
	}
	m.flash = say
	return m, m.send(proto.Act{RecID: "stoker:" + verb, Which: "primary"})
}

// seeStoker marks the stoker's steps seen while the STOKER tab is open: to
// look at them is to see them. At most once in ten seconds, because the
// service's answer comes later.
func (m Model) seeStoker() (Model, tea.Cmd) {
	if m.tab != tabStoker || m.stoker == nil || !m.stoker.Unseen || m.conn == nil || m.lost ||
		m.now().Sub(m.ackSent) < 10*time.Second {
		return m, nil
	}
	m.ackSent = m.now()
	return m, m.send(proto.Act{RecID: "stoker:ack", Which: "primary"})
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

// resting: the builder took over and the stoker waits for them to leave.
func (m Model) resting() bool {
	return m.stoker != nil && m.stoker.On && !m.stoker.Rests.IsZero()
}

// asksTakeOver: k acts on a session while the stoker is in charge, and the
// builder has not been asked in this run. Looking around never asks.
func (m Model) asksTakeOver(k tea.KeyPressMsg) bool {
	if !m.night() || m.stoker.Asked || m.pending != nil {
		return false
	}
	if _, ok := m.selectedDoor(); ok {
		return false
	}
	switch {
	case bkey.Matches(k, m.keys.main.Open):
		return m.primary() != nil
	case bkey.Matches(k, m.keys.main.Second):
		return m.secondary() != nil
	case bkey.Matches(k, m.keys.main.Peek):
		pane, _ := m.peekTarget()
		return pane != ""
	}
	return false
}

// hold is what k does, fixed at the moment it was pressed: the answer
// runs it on the row it was pressed on, wherever the selection is by then.
func (m Model) hold(k tea.KeyPressMsg) func(Model) (tea.Model, tea.Cmd) {
	switch {
	case bkey.Matches(k, m.keys.main.Peek):
		pane, _ := m.peekTarget()
		return func(m Model) (tea.Model, tea.Cmd) {
			m.openPeek(pane)
			return m, nil
		}
	case bkey.Matches(k, m.keys.main.Second):
		a := m.secondary()
		return func(m Model) (tea.Model, tea.Cmd) {
			m.pending = a
			return m, nil
		}
	}
	a := m.primary()
	return func(m Model) (tea.Model, tea.Cmd) {
		m.pending, m.previewTop = a, 0
		return m, nil
	}
}

// answerTakeOver is the builder's answer: y rests the stoker, n keeps it
// on; either way what they pressed runs. Any other key drops the question
// and what was held.
func (m Model) answerTakeOver(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	rest := bkey.Matches(k, m.keys.takeOver.Rest)
	if !rest && !bkey.Matches(k, m.keys.takeOver.Keep) {
		m.held, m.flash = nil, ""
		return m, nil
	}
	if !m.night() { // the run ended or rests since the question: nothing to take over
		held := m.held
		m.held = nil
		return held(m)
	}
	if m.conn == nil || m.lost {
		m.flash = "not sent, no connection · " + k.String() + " tries again"
		return m, nil
	}
	held := m.held
	m.held = nil
	v := *m.stoker
	v.Asked = true
	verb, say := "keep", "the stoker stays on"
	if rest {
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

// missWhy is why a session is not ready, with its reset in the console's
// zone (the service wrote Why in the host's).
func missWhy(x proto.StokerMiss, ended time.Time) string {
	if x.Out.IsZero() {
		return x.Why
	}
	host := "out until " + limits.Until(x.Out, ended)
	return strings.Replace(x.Why, host, "out until "+limits.Until(x.Out.Local(), ended.Local()), 1)
}

// lapseLines say when the plan's limit stopped sessions during the run,
// and when it came back: the first thing a run the limit stopped tells.
func lapseLines(c proto.StokerCard) []string {
	var out []string
	for _, l := range c.Lapses {
		from := l.From.Local()
		out = append(out, "limit out "+limits.Until(from, c.Since.Local())+" → "+limits.Until(l.Until.Local(), from))
	}
	return out
}

// cardLines are the counts of a card: one line, or two on a phone.
func cardLines(c proto.StokerCard, narrow bool) []string {
	full := fmt.Sprintf("%d were full", c.Full)
	if c.Full == 1 {
		full = "1 was full"
	}
	given := ""
	if c.Given > 0 {
		given = " · " + sample.Kilo(c.Given) + " tokens given back"
	}
	if narrow {
		if given != "" {
			given = " · " + sample.Kilo(c.Given) + " given back"
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
