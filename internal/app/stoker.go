package app

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// stokerOf is host's stoker view; nil when its service keeps none. Each
// host keeps its own stoker (R14).
func (m Model) stokerOf(host string) *proto.StokerView {
	if host == m.opt.Host {
		return m.stoker
	}
	return m.stateOf(host).Stoker
}

// setStoker keeps v as host's stoker view until its next state.
func (m *Model) setStoker(host string, v *proto.StokerView) {
	if host == m.opt.Host {
		m.stoker = v
		return
	}
	if i := m.peerAt(host); i >= 0 {
		m.peers = slices.Clone(m.peers)
		m.peers[i].st.Stoker = v
	}
}

// rowHost is the host of the selected session, else the home host: the
// host a stoker key acts on.
func (m Model) rowHost() string {
	if s, ok := m.selectedSession(); ok {
		return s.Host
	}
	return m.opt.Host
}

// unsent is why an act to host cannot go now; "" when it can.
func (m Model) unsent(host string) string {
	if why := m.refusal(host); why != "" {
		return why
	}
	if host == m.opt.Host && (m.conn == nil || m.lost) {
		return "not sent, no connection"
	}
	return ""
}

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
// launch sheet, or the night off), on the selected row's host.
func (m Model) stokerKey(night bool) (tea.Model, tea.Cmd) {
	host := m.rowHost()
	if why := m.unsent(host); why != "" {
		m.flash = why
		return m, nil
	}
	v := m.stokerOf(host)
	if v == nil {
		m.flash = "this service keeps no stoker"
		return m, nil
	}
	var verb, say string
	switch {
	case !night && m.restingOn(host):
		verb, say = "wake", "the stoker is back on"
	case night && v.On && !v.Until.IsZero(), !night && v.On:
		verb, say = "off", "the stoker is off"
	default:
		return m.openSheet(host, night, false)
	}
	m.flash = say
	return m, m.sendTo(host, proto.Act{RecID: "stoker:" + verb, Which: "primary"})
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

// stokersBadge is the stoker's word of two or more hosts: how many of
// them it runs on, `stoker 2/3`; off when it runs on none.
func (m Model) stokersBadge(short bool) string {
	st, kept, runs := m.st, false, 0
	for _, h := range m.hosts() {
		if v := m.stokerOf(h); v != nil {
			kept = true
			if v.On {
				runs++
			}
		}
	}
	switch {
	case !kept:
		return ""
	case m.stokerAsks() && short:
		return st.accent.Render("stoker waits")
	case m.stokerAsks():
		return st.accent.Render("stoker waits for you")
	case runs == 0:
		return st.faint.Render("stoker off")
	}
	return st.stoker.Render(fmt.Sprintf("stoker %d/%d", runs, m.hostCount()))
}

// resting: the builder took over and the home host's stoker waits for
// them to leave.
func (m Model) resting() bool { return m.restingOn(m.opt.Host) }

// restingOn: the builder took over and host's stoker waits for them to
// leave.
func (m Model) restingOn(host string) bool {
	v := m.stokerOf(host)
	return v != nil && v.On && !v.Rests.IsZero()
}

// nightOn: host's stoker is in charge.
func (m Model) nightOn(host string) bool {
	v := m.stokerOf(host)
	return v != nil && v.On && v.Rests.IsZero()
}

// takeOverOn: host's stoker is in charge and the builder has not been
// asked in this run, so a key that acts on a session of host will ask.
func (m Model) takeOverOn(host string) bool { return m.nightOn(host) && !m.stokerOf(host).Asked }

// takeOverDue: some host's take-over question is still to come.
func (m Model) takeOverDue() bool { return slices.ContainsFunc(m.hosts(), m.takeOverOn) }

// asksTakeOver: k acts on a session of a host whose stoker is in charge,
// and the builder has not been asked in this run. Looking around never
// asks.
func (m Model) asksTakeOver(k tea.KeyPressMsg) bool {
	if m.pending != nil {
		return false
	}
	if _, ok := m.selectedDoor(); ok {
		return false
	}
	on := func(a *action) bool { return a != nil && m.takeOverOn(m.actHost(a)) }
	switch {
	case bkey.Matches(k, m.keys.main.Open):
		return on(m.primary())
	case bkey.Matches(k, m.keys.main.Second):
		return on(m.secondary())
	case bkey.Matches(k, m.keys.main.Peek):
		at, _ := m.peekTarget()
		return at.pane != "" && m.takeOverOn(cmp.Or(at.host, m.opt.Host))
	}
	return false
}

// hold is what k does, fixed at the moment it was pressed, and the host it
// acts on: the answer runs it on the row it was pressed on, wherever the
// selection is by then.
func (m Model) hold(k tea.KeyPressMsg) (func(Model) (tea.Model, tea.Cmd), string) {
	switch {
	case bkey.Matches(k, m.keys.main.Peek):
		at, _ := m.peekTarget()
		return func(m Model) (tea.Model, tea.Cmd) {
			m.openPeek(at)
			return m, nil
		}, cmp.Or(at.host, m.opt.Host)
	case bkey.Matches(k, m.keys.main.Second):
		a := m.secondary()
		return func(m Model) (tea.Model, tea.Cmd) {
			m.pending = a
			return m, nil
		}, m.actHost(a)
	}
	a := m.primary()
	return func(m Model) (tea.Model, tea.Cmd) {
		m.pending, m.previewTop = a, 0
		return m, nil
	}, m.actHost(a)
}

// actHost is the host a acts on; the home host for none.
func (m Model) actHost(a *action) string {
	if a == nil {
		return m.opt.Host
	}
	return m.hostOf(*a)
}

// answerTakeOver is the builder's answer about the stoker of the host the
// held key acts on: y rests it, n keeps it on; either way what they
// pressed runs. Any other key drops the question and what was held.
func (m Model) answerTakeOver(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	rest := bkey.Matches(k, m.keys.takeOver.Rest)
	if !rest && !bkey.Matches(k, m.keys.takeOver.Keep) {
		m.held, m.flash = nil, ""
		return m, nil
	}
	host := cmp.Or(m.heldOn, m.opt.Host)
	if !m.nightOn(host) { // the run ended or rests since the question: nothing to take over
		held := m.held
		m.held = nil
		return held(m)
	}
	if why := m.unsent(host); why != "" {
		m.flash = why + " · " + k.String() + " tries again"
		return m, nil
	}
	held := m.held
	m.held = nil
	v := *m.stokerOf(host)
	v.Asked = true
	verb, say := "keep", "the stoker stays on"
	if rest {
		verb, say = "rest", "stoker rests · "+m.keys.main.Stoke.Help().Key+" turns it back on"
		v.Rests, v.Touched = m.now(), m.now()
		m.touchedAt(host)
	}
	m.setStoker(host, &v)
	send := m.sendTo(host, proto.Act{RecID: "stoker:" + verb, Which: "primary"})
	next, run := held(m)
	if nm, ok := next.(Model); ok && nm.flash == "" {
		nm.flash = say
		next = nm
	}
	return next, tea.Batch(send, run)
}

// touchRest tells the service of the host the selected row acts on, at
// most once a minute, that the builder is still at the console, so its
// stoker goes on resting.
func (m Model) touchRest() (Model, tea.Cmd) {
	if !slices.ContainsFunc(m.hosts(), m.restingOn) {
		return m, nil // the act's host is looked up only when a stoker rests
	}
	host := m.actHost(m.primary())
	if !m.restingOn(host) || m.unsent(host) != "" || m.now().Sub(m.touched[host]) < time.Minute {
		return m, nil
	}
	m.touchedAt(host)
	return m, m.sendTo(host, proto.Act{RecID: "stoker:touch", Which: "primary"})
}

// touchedAt keeps now as the last key that kept host's rest.
func (m *Model) touchedAt(host string) {
	touched := maps.Clone(m.touched)
	if touched == nil {
		touched = map[string]time.Time{}
	}
	touched[host] = m.now()
	m.touched = touched
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
