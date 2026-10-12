package app

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
)

// The answer peek is a panel over the dimmed screen: what a session said
// last and the answer to it (design/0006 §6). It is the review: what it
// shows goes to the pane named in its title on ⏎, with no second confirm.

// peek is the open answer peek.
type peek struct {
	pane string
	// The question tool's call: the question on screen, the row under the
	// cursor (the typing row after the options; -1 none), the options
	// ticked (from 1), and the answers to the questions before it.
	askID   string
	q, at   int
	ticks   map[int]bool
	choices []said.Choice
	text    string // the typing line
}

// peekView is the session of the open peek and what it asks: the question
// tool's call, or a permission prompt, which is answered in its pane.
type peekView struct {
	s      sample.Session
	ask    *said.Ask
	permit bool
}

// peekNow is the open peek's session, with the peek moved to a new call
// when the session asks another.
func (m Model) peekNow() (peekView, peek, bool) {
	if m.peek == nil {
		return peekView{}, peek{}, false
	}
	s, ok := m.sessionIn(m.peek.pane)
	if !ok {
		return peekView{}, peek{}, false
	}
	p := *m.peek
	v := peekView{s: s, ask: s.Ask}
	if s.Ask == nil {
		it, _ := m.waitOf(s)
		v.permit = it.State == queue.StatePermission
	} else if p.askID != s.Ask.ID {
		// A new call: nothing is picked until the person picks it.
		p = peek{pane: p.pane, askID: s.Ask.ID, at: -1}
	}
	return v, p, true
}

// answerTarget is the pane `a` opens the peek on: a session that waits.
// Why says why there is none.
func (m Model) answerTarget() (pane, why string) {
	s, ok := m.selectedSession()
	if !ok || s.Pane == "" {
		return "", ""
	}
	if _, ok := m.waitOf(s); ok {
		return s.Pane, ""
	}
	return "", "the session in " + s.Pane + " does not wait for you"
}

func (m *Model) openPeek(pane string) {
	m.peek, m.flash = &peek{pane: pane}, ""
	if s, ok := m.sessionIn(pane); ok {
		m.chose, m.selPID = true, s.PID
		if s.Ask != nil {
			m.peek.askID = s.Ask.ID
		}
	}
}

// peekQueue is every other session that waits, in the list's order after
// the one in pane: the ones ⇥ and a send go to.
func (m Model) peekQueue(pane string) []sample.Session {
	ss := m.snap.Sessions
	at := slices.IndexFunc(ss, func(s sample.Session) bool { return s.Pane == pane })
	var out []sample.Session
	for k := 1; k <= len(ss); k++ {
		s := ss[(at+k+len(ss))%len(ss)]
		if s.Pane != "" && s.Pane != pane && m.groupOf(s) == groupWaits {
			out = append(out, s)
		}
	}
	return out
}

// peekQuiet is how long the peek takes no key after it moved to another
// session: a second Enter, a key repeat or a buffered tap meant for the
// question before must not answer one the person has not read.
const peekQuiet = 400 * time.Millisecond

// peekNext moves the peek to the next session that waits, or closes it.
// Nothing is picked on the next one, and keys wait a moment.
func (m Model) peekNext(pane string) Model {
	m.peek = nil
	if next := m.peekQueue(pane); len(next) > 0 {
		m.openPeek(next[0].Pane)
		m.peek.at, m.peekQuiet = -1, time.Now().Add(peekQuiet)
	}
	return m
}

// peekKey is a key while the peek is open. Every printable key is text on
// the typing line, q too.
func (m Model) peekKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	v, p, ok := m.peekNow()
	if !ok {
		m.peek, m.flash = nil, "the session is gone"
		return m, nil
	}
	n := 0
	var q said.Question
	if v.ask != nil {
		q = v.ask.Questions[p.q]
		n = len(q.Options)
	}
	typing := v.ask == nil || p.at == n
	key := k.String()
	if time.Now().Before(m.peekQuiet) && key != "esc" && key != "ctrl+c" {
		return m, nil
	}
	switch {
	case key == "esc":
		m.peek = nil
		return m, nil
	case key == "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case key == "tab":
		return m.peekNext(p.pane), nil
	case v.permit:
		if key == "enter" {
			m.peek = nil
			if a := m.forHost(m.jump(v.s.Pane)); a != nil {
				return m.confirm(*a)
			}
		}
		return m, nil
	case key == "enter" && v.ask == nil:
		if strings.TrimSpace(p.text) == "" {
			return m, nil
		}
		return m.sendPeek(v.s, "answer:"+p.pane, p.text)
	case key == "enter":
		c := said.Choice{}
		if q.Multi {
			for i := 1; i <= n; i++ {
				if p.ticks[i] {
					c.Picks = append(c.Picks, i)
				}
			}
		}
		switch {
		case typing && strings.TrimSpace(p.text) == "":
			return m, nil
		case typing:
			c.Text = p.text
		case q.Multi && len(c.Picks) > 0:
		case p.at < 0:
			m.flash = "pick an option"
			return m, nil
		default:
			c.Picks = []int{p.at + 1}
		}
		return m.peekChoose(v, p, c)
	case key == "up" && v.ask != nil:
		p.at = max(p.at-1, 0)
	case key == "down" && v.ask != nil:
		p.at = min(p.at+1, n)
	case key == "backspace":
		p.text = trimLast(p.text)
	case !typing && len(key) == 1 && key[0] >= '1' && key[0] <= '9':
		d := int(key[0] - '0')
		switch {
		case d == n+1:
			p.at = n
		case d > n:
		case q.Multi:
			p.ticks = toggled(p.ticks, d)
		default:
			return m.peekChoose(v, p, said.Choice{Picks: []int{d}})
		}
	case !typing && (key == "space" || key == " ") && q.Multi && p.at >= 0:
		p.ticks = toggled(p.ticks, p.at+1)
	case k.Text != "" && len([]rune(p.text)) < 500:
		p.at, p.text = n, p.text+k.Text
	}
	m.peek = &p
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

// peekChoose answers the question on screen: the next one shows, or the
// call's answers go to the pane.
func (m Model) peekChoose(v peekView, p peek, c said.Choice) (tea.Model, tea.Cmd) {
	p.choices = append(slices.Clone(p.choices), c)
	if p.q+1 < len(v.ask.Questions) {
		p.q, p.at, p.ticks, p.text = p.q+1, 0, nil, ""
		m.peek = &p
		return m, nil
	}
	b, err := json.Marshal(said.Answer{ID: v.ask.ID, Choices: p.choices})
	if err != nil {
		m.flash = err.Error()
		return m, nil
	}
	return m.sendPeek(v.s, "pick:"+p.pane, string(b))
}

// sendPeek sends the answer the peek shows and moves the peek on. The
// toast says "sent to" only when the result says every key went; a guard
// that stopped it says why on the key line.
func (m Model) sendPeek(s sample.Session, rec, text string) (tea.Model, tea.Cmd) {
	if m.conn == nil || m.lost {
		m.flash = "not sent, no connection: the answer to " + s.Name
		return m, nil
	}
	cmd := m.send(proto.Act{RecID: rec, Which: "secondary", Confirm: "y", Text: text})
	m.peekSent = maps.Clone(m.peekSent)
	if m.peekSent == nil {
		m.peekSent = map[string]string{}
	}
	m.peekSent["a"+strconv.Itoa(m.acts)] = s.Name
	toast := m.say("sending to " + s.Name + "…")
	return m.peekNext(s.Pane), tea.Batch(cmd, toast)
}

// peekBox is the peek's lines in bw columns, and the key each line presses
// on a tap (an option's digit; "" none). The last line is the footer, its
// keys in spans from the box's left edge.
func (m Model) peekBox(bw, maxH int, phone bool) ([]string, []string, []span) {
	st := m.st
	v, p, _ := m.peekNow()
	iw := bw - 4
	title := st.accent.Bold(true).Render(v.s.Name) + st.muted.Render(" said")
	if it, _ := m.waitOf(v.s); v.ask != nil || v.permit || it.State == queue.StateQuestion {
		title = st.accent.Bold(true).Render(v.s.Name) + st.muted.Render(" asks")
	}
	if v.ask != nil && len(v.ask.Questions) > 1 {
		title += st.faint.Render(fmt.Sprintf(" · %d of %d", p.q+1, len(v.ask.Questions)))
	}
	// Said is this turn's: a question tool call with no words before it
	// shows only its question.
	text := v.s.Said
	if text == "" && v.ask == nil || v.permit {
		text = m.lastWords(v.s)
	}
	var rest []string // what follows the message, each with its key
	var keys []string
	add := func(press string, lines ...string) {
		rest = append(rest, lines...)
		for range lines {
			keys = append(keys, press)
		}
	}
	typing := v.ask == nil && !v.permit
	if v.ask != nil {
		q := v.ask.Questions[p.q]
		add("", "")
		for _, l := range wrapPlain(q.Question, iw) {
			add("", st.text.Render(l))
		}
		for i, o := range append(slices.Clone(q.Options), said.Option{Label: "Type an answer…"}) {
			on := i == p.at
			mark := "  "
			if on {
				mark = st.accent.Render("› ")
			}
			label := st.muted.Render(o.Label)
			if on {
				label = st.text.Render(o.Label)
			}
			box := ""
			if q.Multi && i < len(q.Options) {
				box = st.faint.Render("[ ] ")
				if p.ticks[i+1] {
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
			add(strconv.Itoa(i+1), line)
		}
		typing = p.at == len(q.Options)
	}
	if typing {
		add("", "sep", st.accent.Render("› ")+st.text.Render(ansi.TruncateLeft(p.text, max(lipgloss.Width(p.text)-iw+3, 0), "…"))+st.accent.Render("▏"))
	}
	limit := 9
	if phone {
		limit = 8
	}
	limit = max(min(limit, maxH-2-len(rest)), 1)
	var said []string
	for _, l := range strings.Split(text, "\n") {
		said = append(said, wrapPlain(l, iw)...) // a bullet or a table row keeps its own line
	}
	if len(said) > limit {
		// The end is kept: a question closes a reply.
		said = said[len(said)-limit:]
		said[0] = "…" + ansi.TruncateLeft(said[0], max(lipgloss.Width(said[0])-iw+1, 0), "")
	}
	if len(said) == 0 && len(rest) > 0 && rest[0] == "" {
		rest, keys = rest[1:], keys[1:] // no words: the question opens the box
	}
	var foot []hint
	switch {
	case v.permit:
		foot = append(foot, hint{"⏎", "open", "enter"})
	case v.ask != nil && p.q+1 < len(v.ask.Questions):
		foot = append(foot, hint{"⏎", "question " + strconv.Itoa(p.q+2), "enter"})
	default:
		foot = append(foot, hint{"⏎", "send", "enter"})
	}
	if v.ask != nil && v.ask.Questions[p.q].Multi && !typing {
		foot = append(foot, hint{"␣", "tick", "space"})
	}
	if n := len(m.peekQueue(p.pane)); n > 0 {
		foot = append(foot, hint{"⇥", fmt.Sprintf("next (%d)", n), "tab"})
	}
	foot = append(foot, hint{"esc", "", "esc"})

	hair := st.hair
	top := hair.Render("╭─ ") + title + " "
	out := []string{top + hair.Render(strings.Repeat("─", max(bw-lipgloss.Width(top)-1, 0))+"╮")}
	press := []string{""}
	body := func(l, k string) {
		if l == "sep" {
			out = append(out, hair.Render("├"+strings.Repeat("─", bw-2)+"┤"))
		} else {
			out = append(out, hair.Render("│ ")+fit(l, iw)+hair.Render(" │"))
		}
		press = append(press, k)
	}
	for _, l := range said {
		body(st.text.Render(l), "")
	}
	for i, l := range rest {
		body(l, keys[i])
	}
	line, x := hair.Render("╰─ "), 3
	var spans []span
	for i, h := range foot {
		if i > 0 {
			line += hair.Render(" ─ ")
			x += 3
		}
		part := st.accent.Render(h.key)
		if h.label != "" {
			part += st.muted.Render(" " + h.label)
		}
		spans = append(spans, span{x, x + lipgloss.Width(part), h.press})
		line += part
		x += lipgloss.Width(part)
	}
	line += " "
	out = append(out, ansi.Truncate(line, bw-1, "")+hair.Render(strings.Repeat("─", max(bw-1-lipgloss.Width(line), 0))+"╯"))
	return out, append(press, ""), spans
}

// overlayPeek dims the screen and lays the peek over it: on a phone its
// foot on the line above the key line, full width up to 72 columns and
// centred past that (a phone held upright with a small font); wider, a
// centred box of 62 columns at most.
func (m Model) overlayPeek(sc *drawn, w int) {
	phone := m.layoutAt(w) == Narrow
	bw := min(w, 72)
	if !phone {
		bw = min(62, w-8)
	}
	bottom := len(sc.lines) - 2 // the key line's hairline
	box, press, spans := m.peekBox(bw, bottom-1, phone)
	x, y := (w-bw)/2, bottom-len(box)
	if !phone {
		x, y = (w-bw)/2, max((len(sc.lines)-len(box))/2-1, 1)
	}
	y = max(y, 1)
	for i, l := range sc.lines {
		sc.lines[i] = m.st.faint.Render(ansi.Strip(l))
	}
	for j, b := range box {
		r := y + j
		if r >= len(sc.lines) {
			break
		}
		plain := ansi.Strip(sc.lines[r])
		left := ansi.Truncate(plain, x, "")
		left += strings.Repeat(" ", x-lipgloss.Width(left))
		right := ansi.TruncateLeft(plain, x+bw, "")
		sc.lines[r] = m.st.faint.Render(left) + b + m.st.faint.Render(right)
	}
	sc.peekAt, sc.peekX, sc.peekW, sc.peekRows, sc.peekKeys = y, x, bw, press, spans
}

// tapPeek is a tap while the peek is open: on an option it picks it, on
// a key of the footer it presses it; outside the box it closes the peek.
func (m Model) tapPeek(ms tea.Mouse, sc drawn) (tea.Model, tea.Cmd) {
	i, dx := ms.Y-sc.peekAt, ms.X-sc.peekX
	if i < 0 || i >= len(sc.peekRows) || dx < 0 || dx >= sc.peekW {
		m.peek = nil
		return m, nil
	}
	press := sc.peekRows[i]
	if i == len(sc.peekRows)-1 {
		s, ok := spanAt(sc.peekKeys, dx)
		if !ok {
			return m, nil
		}
		press = s.press
	}
	if press == "" {
		return m, nil
	}
	if p := *m.peek; press[0] >= '1' && press[0] <= '9' {
		p.at = 0 // a tap on an option picks it, also while the typing line has the cursor
		m.peek = &p
	}
	return m.peekKey(keyPress(press))
}

// keyPress is the key a tap presses.
func keyPress(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	r := []rune(k)
	return tea.KeyPressMsg{Code: r[0], Text: k}
}
