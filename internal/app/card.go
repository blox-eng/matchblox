package app

import (
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
)

// The question card sits at the bottom of the chat while its session waits
// on Claude Code's question tool: the question and its options, drawn to
// the chat's width, answered through the pick: act.

// card is the answer being made to the question tool's call: the question
// on screen, the row under the cursor (the typing row after the options;
// -1 none), the options ticked (from 1), the answers to the questions
// before it and the typing row's text.
type card struct {
	askID   string
	q, at   int
	ticks   map[int]bool
	choices []said.Choice
	text    string
}

// cardQuiet is how long a card sends nothing after it shows, or after the
// chat opens on it: a second Enter, a key repeat or a buffered tap meant for
// what was there before must not answer a question nobody has read.
const cardQuiet = 400 * time.Millisecond

// cardSeen is the ask a card showed for, and when its quiet started.
type cardSeen struct {
	askID string
	at    time.Time
}

// cardMaxW is the widest the card grows: a long option reads better
// wrapped in a narrow card than spread over a desk.
const cardMaxW = 62

// cardNow is the chat's session and its card, when the session asks
// through the question tool and the screen shows no permission prompt. A
// new call starts a new card.
func (m Model) cardNow() (sample.Session, card, bool) {
	s, ok := m.selectedSession()
	if !ok || s.Pane == "" || s.Ask == nil || len(s.Ask.Questions) == 0 {
		return sample.Session{}, card{}, false
	}
	if it, waits := m.waitOf(s); waits && it.State == queue.StatePermission {
		return sample.Session{}, card{}, false // the prompt is answered with the agent's own keys
	}
	c := m.askCard
	if c.askID != s.Ask.ID || c.q >= len(s.Ask.Questions) {
		c = card{askID: s.Ask.ID}
	}
	return s, c, true
}

// cardTakes tells if k acts on the card rather than going to the agent.
func (m Model) cardTakes(k tea.KeyPressMsg) bool {
	if _, _, ok := m.cardNow(); !ok {
		return false
	}
	ck := m.keys.card
	text, _ := keyName(k)
	return bkey.Matches(k, ck.Send, ck.Up, ck.Down, ck.Erase) || strings.HasPrefix(text, "lit:")
}

// cardSends tells if k, on the card, answers the question: ⏎, or a digit
// off the typing row.
func (m Model) cardSends(k tea.KeyPressMsg) bool {
	s, c, ok := m.cardNow()
	if !ok {
		return false
	}
	ck := m.keys.card
	return bkey.Matches(k, ck.Send) || c.at != len(s.Ask.Questions[c.q].Options) && bkey.Matches(k, ck.Pick)
}

// noteCard starts the quiet of a card drawn for an ask it was not drawn for
// before.
func (m Model) noteCard() Model {
	if s, _, ok := m.cardNow(); ok && s.Ask.ID != m.cardSeen.askID && m.cardDrawn() {
		m.cardSeen = cardSeen{askID: s.Ask.ID, at: m.now()}
	}
	return m
}

// cardDrawn tells if the chat, and so its card, is on screen: beside the
// list on a desk or an opened phone, or the whole screen once it has the
// keys.
func (m Model) cardDrawn() bool {
	_, right := m.panes(max(m.width, minWidth))
	return m.tab == tabSessions && (m.chatKeys || right > 0)
}

// cardHushed is the card's quiet: what it would send now goes nowhere.
func (m Model) cardHushed(s sample.Session) bool {
	return m.cardSeen.askID == s.Ask.ID && m.now().Before(m.cardSeen.at.Add(cardQuiet))
}

// askedAgain is what the key line says of an answer held back from a
// question the builder may not have read.
func askedAgain(name string) string {
	return "not sent: " + name + " asks something new — look again"
}

// heldCard is a card key held for the take-over answer: it acts on the card
// it was pressed on, and on no newer question. tapped: the key is a tap on
// an option, which picks it from the typing row too.
func (m Model) heldCard(k tea.KeyPressMsg, tapped bool) func(Model) (tea.Model, tea.Cmd) {
	s, _, _ := m.cardNow()
	pane, id, name := s.Pane, s.Ask.ID, s.Name
	return func(m Model) (tea.Model, tea.Cmd) {
		now, c, ok := m.cardNow()
		if !ok || now.Pane != pane || now.Ask.ID != id {
			m.flash = askedAgain(name)
			return m, nil
		}
		if tapped {
			c.at = 0
			m.askCard = c
		}
		return m.cardKey(k)
	}
}

// cardKey is a key the card takes: ↑↓ move, space ticks, a digit picks,
// ⏎ answers and any other text goes on the typing row.
func (m Model) cardKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s, c, _ := m.cardNow()
	q := s.Ask.Questions[c.q]
	n := len(q.Options)
	typing := c.at == n
	ck := m.keys.card
	switch {
	case bkey.Matches(k, ck.Send):
		ch := said.Choice{}
		if q.Multi {
			for i := 1; i <= n; i++ {
				if c.ticks[i] {
					ch.Picks = append(ch.Picks, i)
				}
			}
		}
		switch {
		case typing && strings.TrimSpace(c.text) == "":
			return m, nil
		case typing:
			ch.Text = c.text
		case q.Multi && len(ch.Picks) > 0:
		case c.at < 0:
			m.flash = "pick an option"
			return m, nil
		default:
			ch.Picks = []int{c.at + 1}
		}
		if m.cardHushed(s) {
			m.flash = askedAgain(s.Name)
			return m, nil
		}
		return m.cardChoose(s, c, ch)
	case bkey.Matches(k, ck.Up):
		c.at = max(c.at-1, 0)
	case bkey.Matches(k, ck.Down):
		c.at = min(c.at+1, n)
	case bkey.Matches(k, ck.Erase):
		c.text = trimLast(c.text)
	case !typing && bkey.Matches(k, ck.Pick):
		d := int(k.String()[0] - '0')
		switch {
		case d == n+1:
			c.at = n
		case d > n:
		case q.Multi:
			c.ticks = toggled(c.ticks, d)
		case m.cardHushed(s):
			m.flash = askedAgain(s.Name)
			return m, nil
		default:
			return m.cardChoose(s, c, said.Choice{Picks: []int{d}})
		}
	case !typing && bkey.Matches(k, ck.Tick) && q.Multi && c.at >= 0:
		c.ticks = toggled(c.ticks, c.at+1)
	case k.Text != "" && len([]rune(c.text)) < 500:
		c.at, c.text = n, c.text+k.Text
	}
	m.askCard = c
	return m, nil
}

func toggled(ticks map[int]bool, i int) map[int]bool {
	out := map[int]bool{}
	for k, v := range ticks {
		out[k] = v
	}
	out[i] = !out[i]
	return out
}

// cardChoose answers the question on the card: the next one shows, or the
// call's answers go to the pane.
func (m Model) cardChoose(s sample.Session, c card, ch said.Choice) (tea.Model, tea.Cmd) {
	c.choices = append(slices.Clone(c.choices), ch)
	if c.q+1 < len(s.Ask.Questions) {
		c.q, c.at, c.ticks, c.text = c.q+1, 0, nil, ""
		m.askCard = c
		m.cardSeen.at = m.now() // the next question is new to the builder too
		return m, nil
	}
	b, err := json.Marshal(said.Answer{ID: s.Ask.ID, Choices: c.choices})
	if err != nil {
		m.flash = err.Error()
		return m, nil
	}
	return m.sendCard(s, "pick:"+s.Pane, string(b))
}

// sendCard sends the card's answer. The card starts over with nothing
// picked: a second ⏎ does not answer twice. The toast says "sent to" only
// when the result says every key went; a guard that stopped it says why
// on the key line.
func (m Model) sendCard(s sample.Session, rec, text string) (tea.Model, tea.Cmd) {
	if m.conn == nil || m.lost {
		m.flash = "not sent, no connection: the answer to " + s.Name
		return m, nil
	}
	m.askCard = card{askID: s.Ask.ID, at: -1}
	cmd := m.send(proto.Act{RecID: rec, Which: "secondary", Confirm: "y", Text: text})
	m.cardSent = maps.Clone(m.cardSent)
	if m.cardSent == nil {
		m.cardSent = map[string]string{}
	}
	m.cardSent["a"+strconv.Itoa(m.acts)] = s.Name
	toast := m.say("sending to " + s.Name + "…")
	m = m.pairActed(action{rec: rec})
	return m, tea.Batch(cmd, toast)
}

// cardLines is the card in at most w columns, or nothing when no question
// is open.
func (m Model) cardLines(w int) []string {
	lines, _, _ := m.cardRows(w)
	return lines
}

// cardRows is the card's lines, the key a tap on each line presses (an
// option's digit; "" none; the last line is the footer) and the keys of
// its footer, in spans from the card's left edge.
func (m Model) cardRows(w int) ([]string, []string, []span) {
	s, c, ok := m.cardNow()
	if !ok || w < 8 {
		return nil, nil, nil
	}
	st := m.st
	bw := min(w, cardMaxW)
	iw := bw - 4
	q := s.Ask.Questions[c.q]
	n := len(q.Options)
	title := st.accent.Bold(true).Render(s.Name) + st.muted.Render(" asks")
	if len(s.Ask.Questions) > 1 {
		title += st.faint.Render(" · " + strconv.Itoa(c.q+1) + " of " + strconv.Itoa(len(s.Ask.Questions)))
	}
	var lines, press []string
	for _, l := range wrapPlain(q.Question, iw) {
		lines, press = append(lines, st.text.Render(l)), append(press, "")
	}
	for i, o := range append(slices.Clone(q.Options), said.Option{Label: "Type an answer…"}) {
		on := i == c.at
		mark := "  "
		if on {
			mark = st.accent.Render("› ")
		}
		label := st.muted.Render(o.Label)
		switch {
		case i == n && on:
			label = st.text.Render(ansi.TruncateLeft(c.text, max(lipgloss.Width(c.text)-iw+6, 0), "…")) + st.accent.Render("▏")
		case i == n && c.text != "":
			label = st.muted.Render(c.text)
		case on:
			label = st.text.Render(o.Label)
		}
		box := ""
		if q.Multi && i < n {
			box = st.faint.Render("[ ] ")
			if c.ticks[i+1] {
				box = st.accent.Render("[✔] ")
			}
		}
		line := mark + st.accent.Render(strconv.Itoa(i+1)) + " " + box + label
		if o.Description != "" && o.Description != o.Label {
			line += st.faint.Render("  " + o.Description)
		}
		line = fit(line, iw)
		if on {
			line = st.selected.Render(line)
		}
		lines, press = append(lines, line), append(press, strconv.Itoa(i+1))
	}
	ck, sep := m.keys.card, " ─ "
	send := st.footFor(ck.Send, "", sep)
	if c.q+1 < len(s.Ask.Questions) {
		send = st.footFor(ck.Send, "question "+strconv.Itoa(c.q+2), sep)
	}
	foot := []footPart{send}
	if q.Multi && c.at != n {
		foot = append(foot, st.footFor(ck.Tick, "", sep))
	}
	box, spans := boxFrame(st.hair, title, lines, foot, bw)
	return box, append([]string{""}, append(press, "")...), spans
}

// tapCard is a tap on the card's row i, dx columns from its left edge: on
// an option it picks it, on a key of the footer it presses it. The chat
// takes the keys. At night, a tap that answers asks to take over first.
func (m Model) tapCard(i, dx int, sc drawn) (tea.Model, tea.Cmd) {
	if !m.chatKeys {
		m = m.openChat()
	}
	press := sc.cardRows[i]
	if i == len(sc.cardRows)-1 {
		s, ok := spanAt(sc.cardKeys, dx)
		if !ok {
			return m, nil
		}
		press = s.press
	}
	if press == "" {
		return m, nil
	}
	k, option := keyPress(press), press[0] >= '1' && press[0] <= '9'
	if _, c, ok := m.cardNow(); ok && option {
		c.at = 0 // a tap on an option picks it, also while the typing row has the cursor
		m.askCard = c
	}
	if m.takeOverDue() && m.cardSends(k) {
		m.held = m.heldCard(k, option)
		return m, nil
	}
	return m.cardKey(k)
}
