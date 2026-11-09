package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// The STOKER tab: the builder's words, the plan with each step's outcome,
// the standing rules, today's compacts and, once the run ended, its score;
// then a card for each chain stoker of the family.

const (
	rowPlan = iota
	rowGo
	rowWhen
	rowRule
	rowRun
	rowScore
	rowAgent
	rowChain
)

// stokerRow is a row of the tab: a step of the plan (e is its last logged
// state, nil before it began), a guarded command that waits for the
// builder's go (e is its last state), a standing rule's entry, a step
// the stoker took by itself, or the card of a chain stoker at work. unit
// is the id of the chain stoker whose card the row is on: its own row,
// or a go its run waits for.
type stokerRow struct {
	kind int
	id   string
	plan proto.StokerPlan
	e    *proto.StokerEntry
	unit string
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
	for _, u := range m.stokers {
		if chainAlive(u) {
			rows = append(rows, stokerRow{kind: rowChain, id: "s:" + u.ID, unit: u.ID})
			rows = append(rows, m.unitGoes(u)...)
		}
	}
	return rows
}

// unitGoes are the rows of chain stoker u's run that wait for your go,
// its plan's steps and its goes: y gives each, as on the host's card.
func (m Model) unitGoes(u state.ChainStoker) []stokerRow {
	if !u.View.On {
		return nil
	}
	var out []stokerRow
	for _, r := range m.onUnit(u).stokerRows() {
		if (r.kind == rowGo || r.kind == rowPlan) && r.state() == "waits" && r.e.Needs != "" {
			r.id, r.unit = "s:"+u.ID+"/"+r.id, u.ID
			out = append(out, r)
		}
	}
	return out
}

// chainAlive: the chain stoker's run is on or its agent is alive. An
// ended one, or one that rests with no agent, only shows its card.
func chainAlive(u state.ChainStoker) bool {
	v := u.View
	return v.On || v.PaneUp || v.Starting
}

// chainsAtWork counts host's chain stokers that are alive.
func (m Model) chainsAtWork(host string) int {
	us := m.stokers
	if host != m.opt.Host {
		us = m.stateOf(host).Stokers
	}
	n := 0
	for _, u := range us {
		if chainAlive(u) {
			n++
		}
	}
	return n
}

// chainsWord is the family's count after the stoker's word: " · 2
// chains", " · 1 chain"; "" with none.
func chainsWord(n int) string {
	switch n {
	case 0:
		return ""
	case 1:
		return " · 1 chain"
	}
	return fmt.Sprintf(" · %d chains", n)
}

// selectedUnit is the chain stoker of the selected card.
func (m Model) selectedUnit() (state.ChainStoker, bool) {
	r, ok := m.stokerSelected()
	if !ok || r.kind != rowChain {
		return state.ChainStoker{}, false
	}
	for _, u := range m.stokers {
		if u.ID == r.unit {
			return u, true
		}
	}
	return state.ChainStoker{}, false
}

// onUnit is m with chain stoker u's view in the host's place: the card
// draws its plan with it.
func (m Model) onUnit(u state.ChainStoker) Model {
	m.stoker, m.stokers, m.stokerSel, m.stokerShown = &u.View, nil, -1, ""
	return m
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
	u, chain := m.selectedUnit()
	if r, ok := m.stokerSelected(); ok && r.kind == rowAgent {
		if m.stoker.PaneUp || m.stoker.Starting {
			keys = append(keys, hintFor(mk.Open, "open the stoker"), hintFor(mk.QuitAgent, ""))
		}
	} else if chain {
		if u.View.PaneUp || u.View.Starting {
			keys = append(keys, hintFor(mk.Open, "open the stoker"))
		}
		if chainAlive(u) {
			keys = append(keys, hintFor(mk.QuitAgent, ""))
		}
	} else if ok {
		switch {
		case r.state() == "waits" && r.e.Needs != "":
			keys = append(keys, hintFor(mk.Grant, ""))
		case m.hasSession(r):
			keys = append(keys, hintFor(mk.Into, ""))
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
	text := r.e.ID
	if r.unit != "" {
		text = r.unit + " " + r.e.ID
	}
	return m, m.send(proto.Act{RecID: "stoker:grant", Which: "primary", Confirm: "y", Text: text})
}

// hasSession: the row is a failed step whose session is still there to look at.
func (m Model) hasSession(r stokerRow) bool {
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

// stokerTarget is the pane of a failed step, which a goes into.
func (m Model) stokerTarget() (pane, why string) {
	r, ok := m.stokerSelected()
	switch {
	case !ok:
		return "", ""
	case r.state() != "failed":
		return "", "a goes into the session of a failed step"
	}
	if _, ok := m.sessionIn(m.opt.Host, r.e.Pane); !ok {
		return "", r.e.Name + " is gone: there is no pane to go into"
	}
	return r.e.Pane, ""
}

// stokerInto is a on a failed step: the pane of its session.
func (m Model) stokerInto() (tea.Model, tea.Cmd) {
	pane, why := m.stokerTarget()
	if pane == "" {
		m.flash = why
		return m, nil
	}
	return m.goInto(m.opt.Host, pane)
}

// stokerBody is the tab: one host's stoker, or with two or more hosts a
// section for each host with a stoker, in Hosts order, headed by its name.
// The selection and the tab's keys are the home host's.
func (m Model) stokerBody(w int) body {
	if m.hostCount() == 1 {
		return m.stokerHostBody(w)
	}
	var b body
	for _, h := range m.hosts() {
		if m.stokerOf(h) == nil {
			continue
		}
		on := m.onStoker(h)
		if h != m.opt.Host {
			on.stokerSel, on.stokerShown = -1, ""
		}
		sec := on.stokerHostBody(w)
		if h != m.opt.Host {
			sec.rows = slices.Repeat([]int{-1}, len(sec.rows))
		}
		b.add(-1, "", m.st.label.Render(fit(" "+m.nameOf(h), w)))
		b.lines, b.rows = append(b.lines, sec.lines...), append(b.rows, sec.rows...)
	}
	if len(b.lines) == 0 {
		return m.stokerHostBody(w)
	}
	return b
}

// onStoker is m with host's stoker in the home host's place: the tab draws
// that host's section with it.
func (m Model) onStoker(host string) Model {
	st := m.stateOf(host)
	m.stoker, m.stokerServer, m.stokers = m.stokerOf(host), st.StokerServer, st.Stokers
	return m
}

// stokerHostBody is one host's stoker: its card, then its chain stokers'.
func (m Model) stokerHostBody(w int) body {
	b := m.stokerHostCard(w)
	if m.stoker == nil {
		return b
	}
	rows := m.stokerRows()
	at := min(m.stokerSel, len(rows)-1)
	for _, u := range m.stokers {
		if len(b.lines) > 0 && b.lines[len(b.lines)-1] != "" {
			b.add(-1, "")
		}
		m.chainCard(&b, u, rows, at, w)
	}
	return b
}

// chainCard is the card of chain stoker u: its chain's name and its state,
// then, while it is alive, its agent's row and its order's plan, and the
// goes its run waits for (rows of the tab, at selected); once it ended,
// why and the score of its run.
func (m Model) chainCard(b *body, u state.ChainStoker, rows []stokerRow, at int, w int) {
	i := slices.IndexFunc(rows, func(r stokerRow) bool { return r.kind == rowChain && r.unit == u.ID })
	sel := i >= 0 && i == at
	st, um := m.st, m.onUnit(u)
	um.stokerShown = m.stokerShown // its rows carry the tab's ids, so ⏎ shows a command here too
	b.add(-1, spread(st.label.Render(" CHAIN "+u.Name), st.stoker.Render(um.stokerStatus())+" ", w))
	if i < 0 {
		if why := chainEnd(u.View); why != "" {
			b.add(-1, fit("   "+st.muted.Render(why), w))
		}
		if c := um.card(); c != nil {
			b.add(-1, um.scoreLines(*c, " ", w)...)
		}
		return
	}
	b.addRow(i, sel, w, st, m.agentLine(&u.View, m.unitAsks(u.ID), "not running", sel, w))
	if o := u.View.Order; o != nil {
		if o.Words != "" {
			for _, l := range wrapPlain("“"+o.Words+"”", w-4) {
				b.add(-1, fit("  "+st.muted.Render(l), w))
			}
		}
		var plan []stokerRow
		done := 0
		for _, r := range um.stokerRows() {
			if r.kind == rowPlan {
				plan = append(plan, r)
				if r.state() == "done" {
					done++
				}
			}
		}
		if len(plan) > 0 {
			b.add(-1, st.label.Render(fit(fmt.Sprintf(" PLAN %d of %d", done, len(plan)), w)))
			for _, r := range plan {
				j := slices.IndexFunc(rows, func(q stokerRow) bool { return q.unit == u.ID && q.id == "s:"+u.ID+"/"+r.id })
				if j < 0 {
					b.add(-1, um.stokerRowLines(r, false, w)...)
					continue
				}
				b.addRow(j, j == at, w, st, um.stokerRowLines(rows[j], j == at, w)...)
			}
		}
	}
	first := true
	for j, r := range rows {
		if r.kind != rowGo || r.unit != u.ID {
			continue
		}
		if first {
			b.add(-1, st.label.Render(fit(" YOUR GO", w)))
			first = false
		}
		b.addRow(j, j == at, w, st, um.stokerRowLines(r, j == at, w)...)
	}
}

// chainEnd is the line an ended chain stoker last wrote on its card about
// its chain: its chain ended, or joined another. "": none.
func chainEnd(v proto.StokerView) string {
	for i := len(v.Run) - 1; i >= 0; i-- {
		if t := v.Run[i].Text; t == "its chain ended" || strings.HasPrefix(t, "its chain joined ") {
			return t
		}
	}
	return ""
}

// unitAsks tells if the agent of chain stoker id waits for you.
func (m Model) unitAsks(id string) bool {
	for _, s := range m.all.sessions {
		if s.Host == m.opt.Host && m.isStoker(s) && s.TmuxSession() == unitSession(id) && (m.asks(s) || s.Asks || s.Permits) {
			return true
		}
	}
	return false
}

// unitSession is the tmux session of chain stoker id on the stoker's
// server.
func unitSession(id string) string { return sample.StokerSession + "-" + id }

func (m Model) stokerHostCard(w int) body {
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
		b.addRow(0, at == 0, w, st, m.agentLine(v, m.stokerAsks(), "not running · "+m.keys.main.Stoke.Help().Key+" starts it", at == 0, w))
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
			if r.kind != kind || r.unit != "" { // a chain stoker's row is on its card
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

// agentLine is the row of v's stoker agent: its command and what it does
// now (off when it does not run, asks when it waits for you), and ⏎ that
// opens it.
func (m Model) agentLine(v *proto.StokerView, asks bool, off string, sel bool, w int) string {
	st := m.st
	mark := " "
	if sel {
		mark = st.accent.Render("▌")
	}
	word, style := "works", st.stoker
	switch {
	case !v.PaneUp && v.Starting:
		word, style = "starting", st.muted
	case !v.PaneUp:
		word, style = off, st.faint
	case asks:
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
		if m.hasSession(r) {
			out = append(out, styled{m.keys.main.Into.Help().Key + " goes into its session", st.warn})
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
	for _, ch := range c.Chains { // the night of each chain stoker
		lines = append(lines, fit("   "+st.text.Render(pad("chain "+ch.Name, colName))+st.muted.Render(fmt.Sprintf("ready %d/%d · %s", ch.Card.Ready, ch.Card.Of, cardLines(ch.Card, true)[0])), w))
	}
	for _, x := range c.NotReady {
		lines = append(lines, "   "+st.text.Render(pad(x.Name, colName))+st.warn.Render(missWhy(x, c.Ended)))
	}
	for _, x := range c.Left {
		lines = append(lines, "   "+st.text.Render(pad(x.Name, colName))+st.muted.Render(x.Why))
	}
	return lines
}
