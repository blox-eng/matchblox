package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// The STOKER tab: the builder's words, the plan with each step's outcome,
// the standing rules, today's compacts and, once the run ended, its score.

const (
	rowPlan = iota
	rowGo
	rowWhen
	rowRule
	rowRun
	rowScore
	rowAgent
)

// stokerRow is a row of the tab: a step of the plan (e is its last logged
// state, nil before it began), a guarded command that waits for the
// builder's go (e is its last state), a standing rule's entry, or a step
// the stoker took by itself.
type stokerRow struct {
	kind int
	id   string
	plan proto.StokerPlan
	e    *proto.StokerEntry
}

// stokerOrder is the order whose plan the tab shows.
func (m Model) stokerOrder() *proto.StokerOrder {
	if m.stoker == nil {
		return nil
	}
	return m.stoker.Order
}

// stokerRows are the rows in the order they show: the stoker agent first,
// where ⏎ opens it.
func (m Model) stokerRows() []stokerRow {
	v := m.stoker
	if v == nil {
		return nil
	}
	var rows, goes, whens []stokerRow
	order := m.stokerOrder()
	last := map[int]*proto.StokerEntry{}
	goAt := map[string]int{} // a go's row, by its pane and go: its last entry is its state
	for i, e := range v.Run {
		switch {
		case e.Rule == "go":
			k := e.Pane + " " + e.Needs
			if j, ok := goAt[k]; ok {
				goes[j].e = &v.Run[i]
				continue
			}
			goAt[k] = len(goes)
			goes = append(goes, stokerRow{kind: rowGo, id: "g" + e.ID, e: &v.Run[i]})
		case e.Rule == "when":
			whens = append(whens, stokerRow{kind: rowWhen, id: "w" + e.ID, e: &v.Run[i]})
		case e.Logged() && e.Step > 0:
			if order != nil && !e.At.Before(order.Went) {
				last[e.Step] = &v.Run[i]
			}
		case e.Logged():
			rows = append(rows, stokerRow{kind: rowRule, id: "r" + e.ID, e: &v.Run[i]})
		}
	}
	var plan []stokerRow
	if order != nil {
		for _, p := range order.Plan {
			plan = append(plan, stokerRow{kind: rowPlan, id: "p" + strconv.Itoa(p.N), plan: p, e: last[p.N]})
		}
	}
	if !v.On {
		// The run ended before the go: nothing guards the command now,
		// and a shows the session that asked.
		for i, g := range goes {
			if g.state() == "waits" {
				e := *g.e
				e.State, e.Text = "failed", "the run ended before your go: "+e.Want
				goes[i].e = &e
			}
		}
	}
	rows = append(append(append(plan, goes...), whens...), rows...)
	for i, e := range v.Run {
		if !e.Logged() {
			rows = append(rows, stokerRow{kind: rowRun, id: "c" + e.ID, e: &v.Run[i]})
		}
	}
	if m.card() != nil {
		rows = append(rows, stokerRow{kind: rowScore, id: "score"}) // a row, so the selection can scroll to it
	}
	if m.hasAgent() {
		rows = append([]stokerRow{{kind: rowAgent, id: "agent"}}, rows...)
	}
	return rows
}

func (m Model) stokerSelected() (stokerRow, bool) {
	rows := m.stokerRows()
	if len(rows) == 0 {
		return stokerRow{}, false
	}
	return rows[min(m.stokerSel, len(rows)-1)], true
}

// state is the step's state: its last logged one, or "" before it began.
func (r stokerRow) state() string {
	if r.e == nil {
		return ""
	}
	return r.e.State
}

// needs: the row waits for the builder, for a go or after a failure.
func (r stokerRow) needs() bool {
	switch {
	case r.kind != rowPlan && r.kind != rowGo:
		return false
	case r.state() == "failed":
		return true
	}
	return r.state() == "waits" && r.e.Needs != ""
}

// stokerNeeds counts the steps that wait for the builder: for a go, or
// after a failure.
func (m Model) stokerNeeds() int {
	n := 0
	for _, r := range m.stokerRows() {
		if r.needs() {
			n++
		}
	}
	return n
}

func (m Model) stokerKeys() []hint {
	mk := m.keys.main
	var keys []hint
	if r, ok := m.stokerSelected(); ok && r.kind == rowAgent {
		if m.stoker.PaneUp || m.stoker.Starting {
			keys = append(keys, hintFor(mk.Open, "open the stoker"), hintFor(mk.QuitAgent, ""))
		}
	} else if ok {
		switch {
		case r.state() == "waits" && r.e.Needs != "":
			keys = append(keys, hintFor(mk.Grant, ""))
		case m.peekable(r):
			keys = append(keys, hintFor(mk.Peek, ""))
		}
		if r.e != nil {
			keys = append(keys, hintFor(mk.Open, "command"))
		}
	}
	keys = append(keys, tabHint(mk.Tabs, tabSessions, "sessions"), hintFor(mk.Ships, ""))
	if m.stoker != nil && m.stoker.On && !m.stoker.Until.IsZero() {
		return append(keys, hintFor(mk.Night, ""))
	}
	return append(keys, hintFor(mk.Stoke, "stoker"))
}

// showCommand is Enter: the row's exact command, or hide it again.
func (m *Model) showCommand() {
	r, ok := m.stokerSelected()
	switch {
	case !ok:
	case m.stokerShown == r.id:
		m.stokerShown = ""
	default:
		m.stokerShown = r.id
	}
}

// grantStep is y on a step that waits for a go: the go is given, and the
// stoker hears of it at its next wake.
func (m Model) grantStep() (tea.Model, tea.Cmd) {
	r, ok := m.stokerSelected()
	if !ok || r.state() != "waits" || r.e.Needs == "" {
		m.flash = m.keys.main.Grant.Help().Key + " gives the go a step waits for"
		return m, nil
	}
	if m.conn == nil || m.lost {
		m.flash = "not sent, no connection"
		return m, nil
	}
	what := r.e.Needs
	if r.e.Want != "" {
		what = r.e.Want
	}
	m.flash = "go given: " + what
	return m, m.send(proto.Act{RecID: "stoker:grant", Which: "primary", Confirm: "y", Text: r.e.ID})
}

// peekable: the row is a failed step whose session is still there to look at.
func (m Model) peekable(r stokerRow) bool {
	if r.state() != "failed" {
		return false
	}
	_, ok := m.sessionIn(m.opt.Host, r.e.Pane)
	return ok
}

// endedWay is the key that resolves a step whose session ended.
func (m Model) endedWay() string {
	switch {
	case m.stoker.On && !m.stoker.Until.IsZero():
		return " · " + m.keys.main.Night.Help().Key + " stops the night"
	case m.stoker.On:
		return " · " + m.keys.main.Order.Help().Key + " gives a new order"
	}
	return " · " + m.keys.main.Stoke.Help().Key + " starts the stoker with a new order"
}

// firstNeed is the first step that waits for the builder, or 0.
func (m Model) firstNeed() int {
	for i, r := range m.stokerRows() {
		if r.needs() {
			return i
		}
	}
	return 0
}

// stokerPeekTarget is the pane of a failed step: a opens the peek on its
// session.
func (m Model) stokerPeekTarget() (pane, why string) {
	r, ok := m.stokerSelected()
	switch {
	case !ok:
		return "", ""
	case r.state() != "failed":
		return "", "a looks at the session of a failed step"
	}
	if _, ok := m.sessionIn(m.opt.Host, r.e.Pane); !ok {
		return "", r.e.Name + " is gone: there is no pane to look at"
	}
	return r.e.Pane, ""
}

func (m Model) stokerBody(w int) body {
	st, v := m.st, m.stoker
	var b body
	b.add(-1, "")
	order := m.stokerOrder()
	if v == nil {
		b.add(-1, st.faint.Render(" this service keeps no stoker"))
		return b
	}
	rows := m.stokerRows()
	at := min(m.stokerSel, len(rows)-1)
	if len(rows) > 0 && rows[0].kind == rowAgent {
		b.addRow(0, at == 0, w, st, m.agentLine(at == 0, w))
		b.add(-1, "")
	}
	if len(v.Run) == 0 && m.card() == nil && order == nil {
		b.add(-1, m.stokerLines(w, true, "")...)
		return b
	}
	b.add(-1, spread(st.label.Render(" "+m.stokerHeadline()), st.stoker.Render(m.stokerStatus())+" ", w))
	if c := m.card(); c != nil { // the limit stopped the run: the first thing the card tells
		for _, l := range lapseLines(*c) {
			b.add(-1, fit("   "+st.warn.Render(l), w))
		}
	}
	b.add(-1, "")
	if order != nil {
		if order.Words != "" {
			for _, l := range wrapPlain("“"+order.Words+"”", w-4) {
				b.add(-1, fit("  "+st.muted.Render(l), w))
			}
		}
		for i := len(order.Earlier) - 1; i >= 0; i-- { // the orders before it, a line each
			e := order.Earlier[i]
			words := "“" + e.Words + "”"
			if e.Words == "" {
				words = "no words: the standing rules"
			}
			b.add(-1, fit("  "+st.faint.Render("earlier "+e.At.Local().Format("15:04")+" "+words), w))
		}
		b.add(-1, m.goesLines(*order, w)...)
		b.add(-1, "")
	}

	section := func(kind int, head string) {
		first := true
		for i, r := range rows {
			if r.kind != kind {
				continue
			}
			if first {
				if len(b.lines) > 0 && b.lines[len(b.lines)-1] != "" {
					b.add(-1, "")
				}
				if head != "" {
					b.add(-1, st.label.Render(fit(" "+head, w)))
				}
				first = false
			}
			b.addRow(i, i == at, w, st, m.stokerRowLines(r, i == at, w)...)
		}
	}
	if order != nil {
		done, needs := 0, 0
		for _, r := range rows {
			switch {
			case r.kind != rowPlan:
			case r.state() == "done":
				done++
			case r.needs():
				needs++
			}
		}
		head := fmt.Sprintf("PLAN %d of %d", done, len(order.Plan))
		if !v.On && !v.Ended.IsZero() {
			head = fmt.Sprintf("DONE %d of %d", done, len(order.Plan))
			if needs > 0 {
				head += fmt.Sprintf(" · %d WAITS FOR YOU", needs)
			}
		}
		section(rowPlan, head)
	}
	section(rowGo, "YOUR GO")
	if len(v.Triggers) > 0 {
		if len(b.lines) > 0 && b.lines[len(b.lines)-1] != "" {
			b.add(-1, "")
		}
		b.add(-1, st.label.Render(fit(" WAITING ON", w)))
		for _, t := range v.Triggers {
			for i, l := range wrapPlain(t.ID+" when "+t.Event()+" → "+t.Action(), w-5) {
				lead := "    "
				if i == 0 {
					lead = "  " + m.glyph("") + " "
				}
				b.add(-1, fit(lead+st.muted.Render(l), w))
			}
		}
	}
	section(rowWhen, "TRIGGERS")
	section(rowRule, "STANDING RULES")
	section(rowRun, "TODAY")
	section(rowScore, "")
	return b
}

// agentLine is the stoker agent's row: its command and what it does now,
// and ⏎ that opens it.
func (m Model) agentLine(sel bool, w int) string {
	st, v := m.st, m.stoker
	mark := " "
	if sel {
		mark = st.accent.Render("▌")
	}
	word, style := "works", st.stoker
	switch {
	case !v.PaneUp && v.Starting:
		word, style = "starting", st.muted
	case !v.PaneUp:
		word, style = "not running · "+m.keys.main.Stoke.Help().Key+" starts it", st.faint
	case m.stokerAsks():
		word, style = "waits for you", st.accent
	case v.Ready:
		word, style = "rests", st.muted
	}
	left := mark + " " + st.label.Render("STOKER AGENT") + "  " + st.text.Render(v.Agent) + st.faint.Render(" · ") + style.Render(word)
	if !v.PaneUp && !v.Starting {
		return fit(left, w)
	}
	return spread(left, st.faint.Render(plainHints(hintFor(m.keys.main.Open, "open the stoker")))+" ", w)
}

// goesLines say what the run's go chips give, what still asks the
// builder, and the sessions no merge guard watches.
func (m Model) goesLines(o proto.StokerOrder, w int) []string {
	st := m.st
	var out []string
	line := func(glyph string, text string) {
		for i, l := range wrapPlain(text, w-4) {
			lead := "  "
			if i == 0 {
				lead = glyph + " "
			}
			out = append(out, fit("  "+lead+st.muted.Render(l), w))
		}
	}
	if names := grantNames(o.Grants); len(names) > 0 {
		line(st.accent.Render("✓"), joinList(names))
	}
	if asks := asksFor(o.Grants); len(asks) > 0 {
		line(st.warn.Render("!"), joinList(asks)+" still ask you")
	}
	for _, u := range o.Unguarded {
		var goes []string
		for _, g := range u.Goes {
			goes = append(goes, g+"s")
		}
		line(st.warn.Render("!"), "no merge guard watches "+u.Session+": its "+joinList(goes)+" go unchecked")
	}
	return out
}

// stokerHeadline is the run in a line: NIGHT · from → to, or STOKER ·
// since.
func (m Model) stokerHeadline() string {
	v := m.stoker
	since, end := v.Since, v.Ended
	night := v.Nightly || !v.Until.IsZero()
	if c := v.Card; c != nil {
		night = night || c.Night
		if since.IsZero() {
			since = c.Since
		}
		if end.IsZero() {
			end = c.Ended
		}
	}
	clock := func(t time.Time) string { return t.Local().Format("15:04") }
	switch {
	case v.On && !v.Until.IsZero():
		return "NIGHT · " + clock(since) + " → " + clock(v.Until)
	case v.On:
		return "STOKER · since " + clock(since)
	case night:
		return "NIGHT · " + clock(since) + " → " + clock(end)
	}
	return "STOKER · " + clock(since) + " → " + clock(end)
}

// stokerStatus is the right of the headline: sealed at night, on, resting
// or ended.
func (m Model) stokerStatus() string {
	v := m.stoker
	switch {
	case m.resting():
		return "rests"
	case v.On && !v.Until.IsZero():
		return "sealed"
	case v.On:
		return "on"
	case !v.Ended.IsZero():
		return "ended " + v.Ended.Local().Format("15:04")
	}
	return "off"
}

// glyph is a step's state in a character and a style.
func (m Model) glyph(state string) string {
	st := m.st
	switch state {
	case "done":
		return st.accent.Render("✓")
	case "doing":
		return st.warn.Render("◐")
	case "failed":
		return st.neg.Render("✕")
	case "waits":
		return st.warn.Render("⏸")
	}
	return st.faint.Render("○")
}

const stepIndent = 10 // a step's text starts in this column

// stokerRowLines are the lines of one row. sel marks the selected row,
// whose command shows when Enter asked for it.
func (m Model) stokerRowLines(r stokerRow, sel bool, w int) []string {
	st := m.st
	mark := " "
	if sel {
		mark = st.accent.Render("▌")
	}
	var lines []string
	switch r.kind {
	case rowRun:
		lines = m.runLines(*r.e, mark, w)
	case rowScore:
		lines = m.scoreLines(*m.card(), mark, w)
	default:
		who, text, clock := "", r.plan.Text, "     "
		switch r.kind {
		case rowGo:
			text = r.e.Text
			if r.state() == "waits" {
				text = "waits for you: " + text
			}
		case rowWhen:
			text = r.e.Text
		case rowRule:
			who, text = r.e.Name, r.e.Text
			if text == "" {
				text = r.e.Rule
			}
		default:
			who = r.plan.Session
		}
		if r.e != nil {
			clock = r.e.At.Local().Format("15:04")
		}
		head := who + "  " + text
		for i, l := range wrapPlain(head, w-stepIndent-1) {
			lead := strings.Repeat(" ", stepIndent)
			if i == 0 {
				lead = " " + mark + m.glyph(r.state()) + " " + st.muted.Render(clock) + " "
				if rest, ok := strings.CutPrefix(l, who); ok && who != "" {
					l = st.muted.Render(who) + st.text.Render(rest)
				} else {
					l = st.text.Render(l)
				}
			} else {
				l = st.text.Render(l)
			}
			lines = append(lines, lead+l)
		}
		for _, s := range m.outcome(r) {
			for _, l := range wrapPlain(s.text, w-stepIndent-1) {
				lines = append(lines, strings.Repeat(" ", stepIndent)+s.style.Render(l))
			}
		}
	}
	if sel && m.stokerShown == r.id {
		lines = append(lines, m.commandLines(r, w)...)
	}
	return lines
}

type styled struct {
	text  string
	style interface{ Render(...string) string }
}

// outcome is what the step's last entry said, and what a waiting or failed
// step needs.
func (m Model) outcome(r stokerRow) []styled {
	st := m.st
	if r.e == nil {
		return nil
	}
	var out []styled
	if r.e.Text != "" && r.kind == rowPlan {
		out = append(out, styled{r.e.Text, st.muted})
	}
	switch r.e.State {
	case "waits":
		switch {
		case r.kind == rowGo:
			out = append(out, styled{m.keys.main.Grant.Help().Key + " gives the go, once", st.warn})
		case r.e.Needs != "":
			out = append(out, styled{"waits for your go: " + r.e.Needs + " · " + m.keys.main.Grant.Help().Key + " gives it", st.warn})
		}
	case "failed":
		if m.peekable(r) {
			out = append(out, styled{m.keys.main.Peek.Help().Key + " shows its session", st.warn})
		} else {
			out = append(out, styled{"its session ended" + m.endedWay(), st.warn})
		}
	}
	return out
}

// commandLines are the exact commands the row's step ran.
func (m Model) commandLines(r stokerRow, w int) []string {
	st := m.st
	if r.e == nil {
		return []string{strings.Repeat(" ", stepIndent) + st.faint.Render("not started: no command yet")}
	}
	var lines []string
	for _, argv := range r.e.Argv {
		for i, l := range wrapPlain(strings.Join(argv, " "), w-stepIndent-3) {
			lead := "  "
			if i == 0 {
				lead = st.accent.Render("$") + " "
			}
			lines = append(lines, strings.Repeat(" ", stepIndent-1)+" "+lead+st.text.Render(l))
		}
	}
	if len(lines) == 0 {
		return []string{strings.Repeat(" ", stepIndent) + st.faint.Render("no command: the stoker agent logged this step itself")}
	}
	return lines
}

// runLines are a step the stoker took by itself: when, the session, the
// context before and after a compact, why and its RESUME line.
func (m Model) runLines(e proto.StokerEntry, mark string, w int) []string {
	st := m.st
	narrow := w < 60
	result := st.muted.Render(e.Result)
	if e.Result == "sent" {
		switch {
		case e.After > 0:
			result = st.text.Render(sample.Kilo(e.Before) + " → " + sample.Kilo(e.After))
		case !e.Back.IsZero():
			result = st.text.Render(sample.Kilo(e.Before) + " → compacted")
		case e.Before == 0:
			result = st.muted.Render("sent")
		case m.now().Sub(e.At) < proto.StokerReadBack:
			result = st.muted.Render(sample.Kilo(e.Before) + " → compacting…")
		default:
			result = st.warn.Render(sample.Kilo(e.Before) + " → not read back")
		}
	}
	lead := " " + mark + " "
	clock := st.muted.Render(e.At.Local().Format("15:04") + "  ")
	cell := m.stepCell(e)
	var lines []string
	if narrow {
		lines = append(lines, lead+clock+cell+st.text.Render(e.Name), "     "+result)
	} else {
		lines = append(lines, lead+clock+cell+st.text.Render(pad(e.Name, colName))+st.muted.Render(pad(e.Pane, colPane))+result)
	}
	for _, l := range wrap(e.Why, w-7, st.muted.Render) {
		lines = append(lines, "      "+strings.TrimPrefix(l, " "))
	}
	if e.Result != "sent" || e.Back.IsZero() {
		return lines
	}
	resume, style := "no RESUME line", st.warn.Render
	if e.Resume != "" {
		resume, style = "RESUME: "+e.Resume, st.muted.Render
	}
	for _, l := range wrap(resume, w-7, style) {
		lines = append(lines, "      "+strings.TrimPrefix(l, " "))
	}
	return lines
}

// scoreLines are the card of a run that ended: its counts, and the
// sessions that are not ready or were left alone.
func (m Model) scoreLines(c proto.StokerCard, mark string, w int) []string {
	st := m.st
	lines := []string{" " + mark + st.label.Render(fmt.Sprintf("THE RUN · ready %d/%d", c.Ready, c.Of))}
	for _, l := range cardLines(c, w < 60) {
		lines = append(lines, "   "+st.text.Render(l))
	}
	for _, x := range c.NotReady {
		lines = append(lines, "   "+st.text.Render(pad(x.Name, colName))+st.warn.Render(missWhy(x, c.Ended)))
	}
	for _, x := range c.Left {
		lines = append(lines, "   "+st.text.Render(pad(x.Name, colName))+st.muted.Render(x.Why))
	}
	return lines
}
