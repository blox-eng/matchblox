package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/sample"
)

// The stoker's panel of the dock (design/0006 §7): the state in a word, and
// for a run that is on or ended its score and its last step; c, or a tap
// on the panel, opens the STOKER tab.

// hasAgent: the service runs the stoker agent on a tmux server of its own.
func (m Model) hasAgent() bool {
	return m.stoker != nil && m.stokerServer != "" && m.stoker.Agent != ""
}

// stokerAttach is the command that attaches to the stoker agent's session
// on its own server.
func (m Model) stokerAttach() string {
	return "tmux -L " + m.stokerServer + " attach -t stoker"
}

// openAgent is ⏎ on the STOKER tab's agent row, or a tap on the header's
// stoker word: inside tmux a popup on the stoker's session, which
// detaching (prefix d) closes, its command shown first as every jump's;
// elsewhere the exact command to run. The builder answers the stoker's
// own prompts there.
func (m Model) openAgent() (tea.Model, tea.Cmd) {
	switch {
	case !m.hasAgent():
		m.flash = "this service starts no stoker agent"
	case !m.stoker.PaneUp && !m.stoker.Starting:
		m.flash = "the stoker is not running: " + m.keys.main.Stoke.Help().Key + " or " + m.keys.main.Night.Help().Key + " starts it"
	case m.opt.Host != "":
		m.flash = "on " + m.opt.Host + ", run: " + m.stokerAttach()
	case m.opt.OutsideTmux:
		m.flash = "outside tmux: run " + m.stokerAttach() + " (prefix d comes back)"
	default:
		m.pending, m.previewTop = &action{label: "open the stoker", nav: true, steps: [][]string{remote.StokerPopup(m.stokerServer)}}, 0
	}
	return m, nil
}

// quitAgent is Q on the STOKER tab: ending the stoker agent and its
// session waits for a typed y.
func (m Model) quitAgent() (tea.Model, tea.Cmd) {
	switch {
	case !m.hasAgent():
		m.flash = "this service starts no stoker agent"
	case !m.stoker.PaneUp && !m.stoker.Starting:
		m.flash = "the stoker is not running"
	default:
		m.pending = &action{label: "quit the stoker agent", destructive: true, rec: "stoker:quit", which: "primary",
			steps: [][]string{{"tmux", "-L", m.stokerServer, "kill-session", "-t", "=stoker"}}}
	}
	return m, nil
}

// isStoker tells a session of the stoker's own tmux server.
func (m Model) isStoker(s sample.Session) bool {
	server := m.stateOf(s.Host).StokerServer
	return server != "" && s.Server() == server
}

// stokerAsks tells if the home host's stoker agent waits for you: a question or
// an approval in its pane.
func (m Model) stokerAsks() bool {
	for _, s := range m.all.sessions {
		if s.Host == m.opt.Host && m.isStoker(s) && (m.asks(s) || s.Asks || s.Permits) {
			return true
		}
	}
	return false
}

// stokerWord is the stoker's state in a word: off, on, night → 07:00,
// rests.
func (m Model) stokerWord() string {
	v := m.stoker
	switch {
	case v == nil || !v.On:
		return "off"
	case !v.Rests.IsZero():
		return "rests"
	case !v.Until.IsZero():
		return "night → " + v.Until.Local().Format("15:04")
	}
	return "on"
}

// stokerCounts are a run's compacts, waits and the context they gave back.
func (m Model) stokerCounts() (compacted, waited, given int) {
	if c := m.card(); c != nil {
		return c.Compacted, c.Waited, c.Given
	}
	for _, e := range m.stoker.Run {
		if e.Result == "sent" {
			compacted++
			if e.After > 0 && e.Before > e.After {
				given += e.Before - e.After
			}
		}
	}
	return compacted, len(m.stoker.Waited), given
}

// stepLine is one step: its time, the session, and before → after or why.
func (m Model) stepLine(e proto.StokerEntry, w int) string {
	st := m.st
	what := st.muted.Render(e.Result)
	switch {
	case e.Result == "sent" && e.After > 0:
		what = st.text.Render(sample.Kilo(e.Before) + " → " + sample.Kilo(e.After))
	case e.Result == "sent":
		what = st.muted.Render(sample.Kilo(e.Before) + " → compacting")
	case e.Why != "":
		what = st.muted.Render(e.Why)
	}
	return fit(" "+st.muted.Render(e.At.Local().Format("15:04")+" ")+m.stepCell(e)+st.text.Render(e.Name)+"  "+what, w)
}

// stokerLines are the stoker's panel in w columns: its fewest lines, or
// (full) with its recent steps.
func (m Model) stokerLines(w int, full bool, toggle string) []string {
	st := m.st
	right := st.stoker.Render(m.stokerWord()) + " "
	if toggle != "" {
		right = st.accent.Render(toggle) + " "
	}
	out := []string{spread(st.label.Render(" STOKER"), right, w)}
	v := m.stoker
	if v == nil || !v.On && len(v.Run) == 0 && m.card() == nil {
		return append(out, fit(" "+st.muted.Render("off · ")+keyWord(m.keys.main.Stoke, st.accent, st.muted, " starts it · ")+
			keyWord(m.keys.main.Night, st.accent, st.muted, " starts a night"), w))
	}
	compacted, waited, given := m.stokerCounts()
	score := fmt.Sprintf("%d compacted · %d waited · %s back", compacted, waited, sample.Kilo(given))
	if c := m.card(); c != nil {
		out = append(out, fit(" "+st.text.Render(fmt.Sprintf("ready %d/%d", c.Ready, c.Of))+st.muted.Render(" · "+score), w))
	} else {
		lead := " " // the state is on the label line, unless a phone's switch has it
		if toggle != "" {
			lead += st.stoker.Render(m.stokerWord()) + st.muted.Render(" · ")
		}
		out = append(out, fit(lead+st.muted.Render(fmt.Sprintf("watching %d · ", len(v.Watched))+score), w))
	}
	switch {
	case len(v.Run) == 0:
		out = append(out, fit(" "+st.faint.Render("no session full yet"), w))
	case full:
		for _, e := range v.Run[max(len(v.Run)-8, 0):] {
			out = append(out, m.stepLine(e, w))
		}
	default:
		out = append(out, m.stepLine(v.Run[len(v.Run)-1], w))
	}
	if v.Unseen {
		out = append(out, fit(" "+keyWord(m.keys.main.Stoker, st.accent, st.muted, " the whole run"), w))
	}
	return out
}

// openStoker is c, or a tap on the stoker's panel: its tab.
func (m Model) openStoker() Model {
	if m.stoker == nil {
		m.flash = "this service keeps no stoker"
		return m
	}
	m.peek = nil
	m.setTab(tabStoker)
	return m
}

// stokerNews is the header's toast for what the stoker did since the last
// state: started, stopped, rests, a step, a run that ended.
func (m Model) stokerNews(next *proto.StokerView) string {
	was := m.stoker
	if next == nil || was == nil {
		return ""
	}
	switch {
	case next.On && !was.On && !next.Until.IsZero():
		return "the stoker runs the night → " + next.Until.Local().Format("15:04")
	case next.On && !was.On:
		return "the stoker is on"
	case !next.On && was.On && next.Card != nil:
		return fmt.Sprintf("the stoker ended · ready %d/%d", next.Card.Ready, next.Card.Of)
	case !next.On && was.On:
		return "the stoker is off"
	case !next.Rests.IsZero() && was.Rests.IsZero():
		return "the stoker rests"
	case len(next.Run) > len(was.Run):
		e := next.Run[len(next.Run)-1]
		if e.Result == "sent" {
			return "the stoker compacted " + e.Name
		}
		return "the stoker: " + e.Name + " " + strings.TrimSpace(e.Result)
	}
	return ""
}
