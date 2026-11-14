package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// agentMarks name the agent that runs in a card's pane. The account rows
// keep providerMarks: their mark names the provider of the plan.
var agentMarks = map[string]string{"claude": "✻", "codex": "❋", "cursor": "⬢", "opencode": "▣"}

func harnessMark(s sample.Session) string {
	if mk, ok := agentMarks[s.Agent]; ok {
		return mk
	}
	return "·"
}

// cardOpts is how a card is drawn. armed and sent are an option's number,
// 0 for none. bare: the chosen card beside the live view, which shows the
// quote and the options. asOf: the time of the last state of a host that
// is not reached, "" for one that is. land: how a card that changed column
// lands. match: the match cell drawn, "" for the match of this moment.
// stacked: the card is in the stack at Medium (boardStacked); ship: it is
// in SHIPPING.
type cardOpts struct {
	sel, lit, oneLine, need, quietRow, otherHost, bare, stacked, ship bool
	armed, sent                                                       int
	asOf, match                                                       string
	land                                                              landing
}

// landing is how a card that changed column lands, for landGlow (design
// decision 7): fire only into WORKING, the wash elsewhere.
type landing int

const (
	landNone landing = iota
	landFire         // the name is flame-core
	landWash         // the card's lines sit on the wash
)

// boardCard is one match drawn in a column (the spec's "The card"):
// every line is w cells, and taps[i] are the spans of line i.
func (m Model) boardCard(s sample.Session, w int, o cardOpts) (lines []string, taps [][]span) {
	add := func(l string, sp []span) {
		lines, taps = append(lines, l), append(taps, sp)
	}
	word, says := m.cardSays(s)
	if o.quietRow {
		add(m.cardHead(s, w, o, word), nil)
		if o.sel {
			for _, l := range m.peekHead(s, w) {
				add(l, nil)
			}
		}
		if last := m.lastWords(s); o.sel && last != "" {
			add(pad("   "+m.st.muted.Render("“"+last+"”"), w), nil)
		}
		return m.washed(lines, w, o), taps
	}
	if o.stacked && !o.need && !o.sel {
		add(m.stackRow(s, w, o, word, says))
		return m.washed(lines, w, o), taps
	}
	whole := o.sel || !o.oneLine
	add(m.cardHead(s, w, o, ""), nil)
	state := "  " + m.wordInk(o).Render(word) + m.st.muted.Render(strings.TrimRight(" "+says, " "))
	switch {
	case whole && o.stacked && o.need: // the track sits at the right of the state line
		add(m.stateTrack(s, state, w, o))
	case whole || o.need:
		add(pad(state, w), nil)
	}
	if whole && !(o.stacked && o.need) {
		add(m.cardTrack(s, w, o))
	}
	if o.sel {
		for _, l := range m.peekHead(s, w) {
			add(l, nil)
		}
	}
	q, answers := m.offersDigits(s)
	if last := m.lastWords(s); o.sel && !o.bare && last != "" {
		for _, l := range upTo(wrapPlain("“"+last+"”", w-3), 3, w-3) {
			add(pad("  "+m.st.muted.Render(l), w), nil)
		}
	}
	if o.sel && answers && o.sent == 0 {
		for i, opt := range q.Options {
			if o.bare && i+1 != o.armed {
				continue // the live view shows the options: a bare card shows only the marked one
			}
			for j, l := range upTo(wrapPlain(opt.Label, w-6), 2, w-6) {
				add(m.optionLine(i+1, j, l, w, o.armed), []span{{0, w, strconv.Itoa(i + 1)}})
			}
		}
	}
	if o.sel || o.need && !o.oneLine && !o.stacked {
		add(m.cardKeys(s, w, o, len(q.Options), answers))
	}
	return m.washed(lines, w, o), taps
}

// peekHead is what the peek adds under the chosen card's own lines, as the
// detail pane did (ruling 35): an account that is not the default with its
// provider's mark, the background work, a chain mate's inbox (ruling 40),
// and the child runs, counted on the head and named under it; the fix for
// tokens with no window known, and each shell's command, under the head
// too.
func (m Model) peekHead(s sample.Session, w int) []string {
	st := m.st
	var parts []string
	if tag := m.accountTag(s); tag != "" {
		parts = append(parts, strings.TrimSpace(providerMarks[s.Provider]+" "+tag))
	}
	if s.Agents > 0 && s.Status != sample.StatusAgents { // the card's word says agents already
		parts = append(parts, plural(s.Agents, "subagent")+" running")
	}
	if len(s.Shells) > 0 {
		shells := plural(len(s.Shells), "shell")
		if old := s.Shells[0].Started; !old.IsZero() {
			shells += " · oldest " + sample.Human(m.now().Sub(old))
		}
		parts = append(parts, shells)
	}
	if v, ok := m.mateOf(s); ok && v.Inbox > 0 {
		inbox := "✉ " + strconv.Itoa(v.Inbox) + " in its inbox"
		if v.Full {
			inbox += ", the oldest dropped"
		}
		parts = append(parts, inbox)
	}
	ref := sample.ProcRef{PID: s.PID, Start: s.Start}
	if n := m.descendants(s.Host, ref); n > 0 {
		parts = append(parts, plural(n, "child run"))
	}
	fix := windowFix(s)
	if len(parts) == 0 && fix == "" {
		return nil
	}
	var out []string
	if len(parts) > 0 {
		out = append(out, fit("  "+st.muted.Render(strings.Join(parts, " · ")), w))
	}
	for _, l := range wrapPlain(fix, max(w-2, 1)) { // it names the fix whole: it wraps
		out = append(out, fit("  "+st.muted.Render(l), w))
	}
	for _, sh := range s.Shells {
		out = append(out, fit("  "+st.faint.Render(strconv.Itoa(sh.PID)+"  "+sh.Command), w))
	}
	for _, l := range m.childRows(s.Host, ref, "  ", 1, w) {
		out = append(out, pad(l, w))
	}
	return out
}

// cardName is the name a card goes by: a chain stoker's agent is its
// chain's stoker (ruling 42), not its tmux session.
func (m Model) cardName(s sample.Session) string {
	if k, ok := m.stokerChain(s); ok {
		return m.roomStokerWord(k)
	}
	return s.Name
}

// mateOf is what s's chain says of s as a mate: its inbox.
func (m Model) mateOf(s sample.Session) (state.MateView, bool) {
	cv := m.chainsOf(s.Host)
	if cv == nil {
		return state.MateView{}, false
	}
	mb := memberOf(s)
	i := slices.IndexFunc(cv.Mates, func(v state.MateView) bool { return v.Member.Same(mb) })
	if i < 0 {
		return state.MateView{}, false
	}
	return cv.Mates[i], true
}

// windowFix is what to do about tokens with no window known, "" when the
// window is known or nothing is spent. For Claude Code the status line
// gives the window, so the fix names setup first.
func windowFix(s sample.Session) string {
	switch {
	case s.Tokens == 0 || s.Window > 0:
		return ""
	case s.Agent == "claude":
		return sample.Kilo(s.Tokens) + " tokens · no window known: run matchblox setup, or add the model to [sessions.windows]"
	}
	return sample.Kilo(s.Tokens) + " tokens · no window known: add the model to [sessions.windows]"
}

// washed puts every cell of a chosen card, or one that lands outside
// WORKING, on the wash, as a selected row.
func (m Model) washed(lines []string, w int, o cardOpts) []string {
	var b body
	b.addRow(0, o.sel || o.land == landWash, w, m.st, lines...)
	return b.lines
}

// wordInk is the state word's ink: the accent on the chosen card, fg bold
// on a card that waits, muted elsewhere.
func (m Model) wordInk(o cardOpts) lipgloss.Style {
	switch {
	case o.sel:
		return m.st.accent
	case o.need:
		return m.st.text.Bold(true)
	}
	return m.st.muted
}

// hostKeep is the fewest cells a card's host word keeps (" @ws-1"), and
// nameKeep the fewest its name is cut to for it.
const hostKeep, nameKeep = 6, 8

// cardHead is line 1: the selection mark, the match, the name and its
// host; at the right the harness mark and the wait, or when a host that is
// not reached sent its last state. A quiet row adds its
// state word after the name.
func (m Model) cardHead(s sample.Session, w int, o cardOpts, word string) string {
	mark, name, match := " ", m.st.text, o.match
	if o.land == landFire {
		name = lipgloss.NewStyle().Foreground(m.marks().Core)
	}
	if o.sel {
		mark, name = m.st.accent.Render("▌"), name.Bold(true)
	}
	if match == "" {
		match = m.matchCell(s)
	}
	wait := padLeft(m.waitCell(s), 3)
	if o.asOf != "" {
		wait = "as of " + o.asOf
	}
	right := m.st.muted.Render(harnessMark(s)) + " " + m.st.faint.Render(wait) + " "
	title, tag := m.cardName(s), ""
	if o.otherHost {
		tag = " @" + m.nameOf(s.Host)
	}
	// A card's name that leaves the host word under four cells is cut
	// before it, so the host word keeps hostKeep cells; where the name
	// would get under nameKeep, and on a quiet row, the host word goes.
	room := w - lipgloss.Width(mark+match+right) - 1
	if cut := room - min(lipgloss.Width(tag), hostKeep); tag != "" && !o.quietRow && room-lipgloss.Width(title) < 4 && cut >= nameKeep {
		title = ansi.Truncate(title, cut, "…")
		tag = ansi.Truncate(tag, room-lipgloss.Width(title), "…")
	}
	left := mark + match + name.Render(title) + m.st.faint.Render(tag)
	// The word lines up after a short name, and is shown whole or not at
	// all: short of room the name is cut, never the word.
	if lw := max(lipgloss.Width(left), 17); word != "" && lw+1+lipgloss.Width(word)+1+lipgloss.Width(right) <= w {
		left = padTo(left, lw) + " " + m.st.faint.Render(word)
	}
	return spread(left, right, w)
}

// cardSays is line 2: the state word and what it says. A match outside
// tmux says so first: it has no pane to go into.
func (m Model) cardSays(s sample.Session) (word, says string) {
	word, says = m.cardSaid(s)
	if s.Pane == "" {
		says = strings.TrimSuffix("outside tmux · "+says, " · ")
	}
	return word, says
}

// cardSaid is cardSays for a match in a pane.
func (m Model) cardSaid(s sample.Session) (word, says string) {
	w, _ := m.rowWord(s)
	word = ansi.Strip(w)
	if h, held := m.holdOf(s); held {
		if h.By == holdByYou && m.asksNow(s) {
			return "⏸ parked · asks", ""
		}
		return word, ""
	}
	switch {
	case s.Status == sample.StatusRetry:
		return word, "retrying · attempt " + strconv.Itoa(s.Retry)
	case s.ReportsWork():
		return word, s.LastLine // the queue's line is of a wait the agent says is over
	case s.ReportsWait() && s.Ask != nil && len(s.Ask.Questions) > 0:
		return word, s.Ask.Questions[0].Question
	case s.ReportsWait() && s.WaitingFor != "":
		return word, s.WaitingFor
	default:
		if q, ok := m.headAsks(s); ok {
			return word, q
		}
	}
	if sh, ok := m.shipOf(s, shipWaits, shipStuck, shipRunning); ok && !s.Works() {
		switch sh.Kind {
		case shipWaits:
			return stageNames[sh.Stage], "#" + strconv.Itoa(sh.Number) + " waits for your merge"
		case shipStuck:
			return "stuck", sh.Note
		}
		return stageNames[sh.Stage], sh.Note
	}
	return word, m.lastWords(s)
}

// cardTrack is line 3: the ship and its track, the count of the other
// ships it drives (shipsMore), the context, and at the right the chain's
// stub. Short of room it drops the stub, then a context under 70%, then
// the count's word and the count, then the track's undone stages from the
// right.
func (m Model) cardTrack(s sample.Session, w int, o cardOpts) (string, []span) {
	left, stub := m.trackParts(s, w, o)
	if stub == "" {
		return fit(left, w), nil
	}
	sw := lipgloss.Width(stub)
	return spread(left, stub+" ", w), []span{{w - 1 - sw, w - 1, hintFor(m.keys.board.Chain, "").press}}
}

// trackParts are cardTrack's parts in w cells: the ship, its track and
// the context after two blanks, and the chain's stub, "" when it does not
// fit.
func (m Model) trackParts(s sample.Session, w int, o cardOpts) (left, stub string) {
	ship, undone := "", 0
	ships := m.shipsOf(s)
	mores := []string{m.shipsMore(ships, false), m.shipsMore(ships, true), ""}
	if len(ships) > 0 {
		sh := ships[0]
		ship = m.st.muted.Render("#"+strconv.Itoa(sh.Number)) + " " + m.trackAt(sh, m.shipSpark(sh))
		undone = len(stageNames) - 1 - sh.Stage
	}
	ctx, keepCtx := "", false
	if s.Context == "known" {
		ctx = m.contextStyle(s).Render(fmt.Sprintf("ctx %.0f%%", s.ContextPct))
		keepCtx = s.ContextPct >= 70 || m.full().Reached(s)
	}
	stub = m.chainStub(s, o)
	more := 0 // the count as it shows: whole, short, gone
	parts := func(cut int, withCtx bool) string {
		var parts []string
		if ship != "" {
			parts = append(parts, ansi.Truncate(ship, lipgloss.Width(ship)-2*cut, "")+mores[more])
		}
		if withCtx && ctx != "" {
			parts = append(parts, ctx)
		}
		return "  " + strings.Join(parts, "  ")
	}
	cut, withCtx, withStub := 0, true, stub != ""
	for {
		l, room := parts(cut, withCtx), w
		if withStub {
			room -= lipgloss.Width(stub) + 2
		}
		if lipgloss.Width(l) <= room {
			break
		}
		switch {
		case withStub:
			withStub = false
		case withCtx && ctx != "" && !keepCtx:
			withCtx = false
		case mores[more] != "":
			more++
		case cut < undone:
			cut++
		default:
			return l, "" // nothing left to drop: fit cuts it
		}
	}
	if !withStub {
		stub = ""
	}
	return parts(cut, withCtx), stub
}

// chainStub is the stub of s's chain, "" for a match on no chain: lit
// while a match of its chain is chosen.
func (m Model) chainStub(s sample.Session, o cardOpts) string {
	k, ok := m.chainKeyOf(s)
	if !ok {
		return ""
	}
	glyph, ink := "─╴", m.st.faint
	if o.lit || o.sel {
		glyph, ink = "━╸", m.st.text
	}
	return ink.Render(glyph + m.chainLabel(k))
}

// stateTrack is a NEEDS YOU card's state line in the stack: the state and
// at its right the track, the context and the chain's stub. The track
// keeps half the line; the state is cut before it.
func (m Model) stateTrack(s sample.Session, state string, w int, o cardOpts) (string, []span) {
	left, stub := m.trackParts(s, w-min(lipgloss.Width(state), w/2)-1, o)
	right := strings.TrimPrefix(left, "  ")
	var taps []span
	if stub != "" {
		if right != "" {
			right += "  "
		}
		right += stub
		sw := lipgloss.Width(stub)
		taps = []span{{w - 1 - sw, w - 1, hintFor(m.keys.board.Chain, "").press}}
	}
	if right == "" {
		return pad(state, w), nil
	}
	return spread(state, right+" ", w), taps
}

// stackRow is a card of WORKING or SHIPPING in the stack, on one row: the
// match, the name, the harness, the state word and what it says; at the
// right the context and the chain's stub in WORKING, the track and the
// wait in SHIPPING.
func (m Model) stackRow(s sample.Session, w int, o cardOpts, word, says string) (string, []span) {
	st := m.st
	name, match := st.text, o.match
	if o.land == landFire {
		name = lipgloss.NewStyle().Foreground(m.marks().Core)
	}
	if match == "" {
		match = m.matchCell(s)
	}
	tag := ""
	if o.otherHost {
		tag = " @" + m.nameOf(s.Host)
	}
	who := match + name.Render(m.cardName(s)) + st.faint.Render(tag) // the names line up, and a long one is whole
	left := " " + who + strings.Repeat(" ", max(16-lipgloss.Width(who), 0)) + " " + st.muted.Render(harnessMark(s)) +
		"  " + m.wordInk(o).Render(word) + st.muted.Render(strings.TrimRight(" "+says, " "))
	var parts []string
	var taps []span
	if o.ship {
		if ships := m.shipsOf(s); len(ships) > 0 {
			sh := ships[0]
			parts = append(parts, st.muted.Render("#"+strconv.Itoa(sh.Number))+" "+m.trackAt(sh, m.shipSpark(sh)))
		}
		wait := padLeft(m.waitCell(s), 3)
		if o.asOf != "" {
			wait = "as of " + o.asOf
		}
		parts = append(parts, st.faint.Render(wait))
	} else {
		if s.Context == "known" {
			parts = append(parts, m.contextStyle(s).Render(fmt.Sprintf("ctx %.0f%%", s.ContextPct)))
		}
		if stub := m.chainStub(s, o); stub != "" {
			parts = append(parts, stub)
			sw := lipgloss.Width(stub)
			taps = []span{{w - 1 - sw, w - 1, hintFor(m.keys.board.Chain, "").press}}
		}
	}
	if len(parts) == 0 {
		return pad(left, w), nil
	}
	return spread(left, strings.Join(parts, "  ")+" ", w), taps
}

// chainLabel is the name a chain goes by: its own, else its first match's.
func (m Model) chainLabel(k chainKey) string {
	if cv := m.chainsOf(k.host); cv != nil {
		for _, c := range cv.Chains {
			if c.ID == k.id && c.Name != "" {
				return c.Name
			}
		}
	}
	return m.chainName(k)
}

// optionLine is line j of option n: "  1. Patch the token check", with
// ❯ in the accent when it is armed.
func (m Model) optionLine(n, j int, text string, w, armed int) string {
	if j > 0 {
		return pad("     "+m.st.text.Render(text), w)
	}
	lead, num, label := "  ", m.st.muted, m.st.text
	if n == armed {
		lead, num, label = m.st.accent.Render("❯")+" ", m.st.accent.Bold(true), m.st.text.Bold(true)
	}
	return pad(lead+num.Render(strconv.Itoa(n)+".")+" "+label.Render(text), w)
}

// cardKeys is line 4: what the match needs, and the key that does it.
func (m Model) cardKeys(s sample.Session, w int, o cardOpts, options int, answers bool) (string, []span) {
	if answers && o.sent > 0 {
		return pad("  "+m.st.muted.Render(fmt.Sprintf("sent %d · waits for %s", o.sent, m.cardName(s))), w), nil
	}
	hs, gap := m.cardHints(s, o, options, answers)
	ink := m.st.muted
	if o.sel {
		ink = m.st.accent
	}
	line, x := "  ", 2
	var spans []span
	for i, h := range hs {
		if i > 0 {
			line, x = line+strings.Repeat(" ", gap), x+gap
		}
		part := ink.Render(h.key) + m.st.muted.Render(" "+h.label)
		if h.press != "" && x < w {
			spans = append(spans, span{x, min(x+lipgloss.Width(part), w), h.press})
		}
		line, x = line+part, x+lipgloss.Width(part)
	}
	return pad(line, w), spans
}

// cardHints are the keys of a card, and the gap between two of them.
func (m Model) cardHints(s sample.Session, o cardOpts, options int, answers bool) ([]hint, int) {
	mk := m.keys.main
	goIn := hintFor(mk.Open, "go in")
	if isShipCard(s) {
		goIn.label = "new session"
	}
	var hs []hint
	gap := 3
	switch {
	case answers && o.sel && o.armed > 0:
		n := strconv.Itoa(o.armed)
		hs, gap = []hint{{n, "again sends ·", n}, hintFor(mk.Back, "clears")}, 1
	case answers && o.sel:
		hs = []hint{{"1-" + strconv.Itoa(options), "mark an answer", ""}, goIn}
	case o.sel && m.goArmedOn(s):
		y := mk.Grant.Keys()[0]
		hs, gap = []hint{{y, "again: " + m.goWhat(s) + " ·", y}, hintFor(mk.Back, "clears")}, 1
	default:
		if sh, ok := m.mergeable(s); ok {
			hs = append(hs, hintFor(m.keys.board.Merge, "merge #"+strconv.Itoa(sh.Number)))
		}
		if m.goWaitsOn(s) {
			hs = append(hs, hintFor(mk.Grant, ""))
		}
		hs = append(hs, goIn)
	}
	return slices.DeleteFunc(hs, func(h hint) bool { return h == (hint{}) }), gap
}

// mergeable is the ship of s that p merges: its leading ship, when it waits
// for the merge and the service gave its merge.
func (m Model) mergeable(s sample.Session) (proto.Ship, bool) {
	if ss := m.shipsOf(s); len(ss) > 0 && ss[0].Kind == shipWaits && len(ss[0].Merge) > 0 {
		return ss[0], true
	}
	return proto.Ship{}, false
}

// oneChoice is the question the digits answer: one question with one
// choice, given with its options.
func (m Model) oneChoice(s sample.Session) (q said.Question, ok bool) {
	if s.Ask == nil || len(s.Ask.Questions) != 1 || !m.asksNow(s) {
		return q, false
	}
	q = s.Ask.Questions[0]
	return q, !q.Multi && len(q.Options) > 0
}

// offersDigits is the question a card offers its digits for: one choice,
// asked on this host. A card on another host takes no digit from the
// peek, so it offers none and shows ⏎ go in.
func (m Model) offersDigits(s sample.Session) (said.Question, bool) {
	q, ok := m.oneChoice(s)
	return q, ok && s.Host == m.opt.Host
}

// upTo keeps the first n lines, the last cut with … when there were more.
func upTo(ls []string, n, w int) []string {
	if len(ls) <= n {
		return ls
	}
	ls = ls[:n]
	ls[n-1] = ansi.Truncate(ls[n-1]+" …", w, "…")
	return ls
}
