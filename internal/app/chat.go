package app

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/menu"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// chatPanel is the chosen session's chat in h lines of w cells: the head
// line, its facts when it has some, the chain line for a session in a
// chain, the conversation cut from its pane, the question with its options
// when it asks, and the line.
func (m Model) chatPanel(w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	blank := strings.Repeat(" ", w)
	k := m.chosenSide()
	listed := m.sideSessions()
	i := slices.IndexFunc(listed, func(s sample.Session) bool { return paneOf(s) == k })
	var lines []string
	switch {
	case !m.have: // the chat waits for the first state
	case m.filter != "" && !slices.ContainsFunc(listed, m.inBoard):
		lines = append([]string{blank}, m.sideWords(w)...)
	case k.waits():
		if v, c, ok := m.sideWaiting(); ok {
			lines = m.waitChat(v, c, w)
			lines = lines[:min(len(lines), h)]
		}
	case k.pane == "" && i >= 0: // outside tmux, with no process and no session id to know it by
		lines = append(lines, blank)
		for _, l := range wrapPlain(listed[i].Name+" runs outside tmux, and its process is not known: nothing to write to", max(w-2, 1)) {
			lines = append(lines, fit(" "+m.st.muted.Render(l), w))
		}
	case k.pane == "" || i < 0:
		text := "No sessions yet. + new session starts one here (" + m.keys.main.New.Help().Key + "). matchblox setup finds the agents you already run."
		lines = append(lines, blank)
		for _, l := range wrapPlain(text, max(w-2, 1)) {
			lines = append(lines, fit(" "+m.st.muted.Render(l), w))
		}
	default:
		s := listed[i]
		top := []string{m.chatHead(s, w)}
		top = append(top, m.chatFacts(s, w)...)
		if l, ok := m.chatChain(s, w); ok {
			top = append(top, l)
		}
		line := m.chatLine(s, w)
		ask := m.chatAsk(s, w, max(h-len(top)-len(line), 0))
		rows := h - len(top) - len(ask) - len(line)
		lines = append(top, m.chatConversation(s, w, max(rows, 0))...)
		lines = append(lines, ask...)
		lines = append(lines, line...)
		lines = lines[max(len(lines)-h, 0):] // the line stays in view
	}
	for len(lines) < h {
		lines = append(lines, blank)
	}
	return lines
}

// waitChat is the chat's place of v, a member of chain c that waits for
// recover: when its server stopped, what the record keeps, the lines that
// wait for it, and the keys that act on it.
func (m Model) waitChat(v state.MateView, c chainKey, w int) []string {
	st, team, blank := m.st, m.chainLabel(c), strings.Repeat(" ", w)
	at, today := m.stoppedAt(v.Wait.Since)
	if today {
		at = "at " + at
	} else {
		at = "on " + at
	}
	lines := []string{fit(" "+st.text.Bold(true).Render(v.Wait.Name)+"  "+st.wait.Render("waits for recover · in "+team), w), blank}
	say := func(ink lipgloss.Style, text string) {
		for _, l := range wrapPlain(text, max(w-2, 1)) {
			lines = append(lines, fit(" "+ink.Render(l), w))
		}
	}
	say(st.muted, "Its tmux server stopped "+at+".")
	say(st.muted, "Its conversation is saved: "+v.Wait.Agent+" "+v.Wait.Conversation)
	say(st.muted, "It stays in "+team+" until it is back, or until the record forgets it on "+v.Wait.Forget.Local().Format("2 Jan")+".")
	switch {
	case v.Inbox == 1:
		say(st.text, "✉ 1 line waits for it")
	case v.Inbox > 1:
		say(st.text, "✉ "+strconv.Itoa(v.Inbox)+" lines wait for it")
	}
	lines = append(lines, blank)
	say(st.text, m.keys.side.Enter.Help().Key+" reopen it")
	say(st.text, m.keys.board.Menu.Help().Key+" out of "+team)
	return lines
}

// chatHead is the head line: the name, the host when it is not this one,
// the agent's mark, its state and its pull request's badge; at the right
// the context and the keys to expand and for the menu.
func (m Model) chatHead(s sample.Session, w int) string {
	st := m.st
	about := ""
	if s.Host != m.opt.Host {
		about += "@" + m.nameOf(s.Host) + " "
	}
	about += harnessMark(s) + " " + m.wordOf(s)
	if ships := m.shipsOf(s); len(ships) > 0 {
		about += " · #" + strconv.Itoa(ships[0].Number) + " " + stageNames[ships[0].Stage]
	}
	name := m.backMark(w) + " " + st.text.Bold(true).Render(m.cardName(s))
	keys := st.faint.Render(m.keys.main.Jump.Help().Key+" expand · "+m.keys.board.Menu.Help().Key+" menu") + " "
	ctx := ""
	if s.Context == "known" {
		ctx = m.contextStyle(s).Render("ctx "+strconv.Itoa(int(s.ContextPct+0.5))+"%") + "  "
	}
	// Short of room the keys go first (the key line has them), then the
	// context, then the words after the name: the name stays whole.
	left := name + " " + st.muted.Render(about)
	for _, l := range []string{left, name} {
		for _, right := range []string{ctx + keys, ctx, ""} {
			if lipgloss.Width(l)+lipgloss.Width(right)+1 <= w {
				return spread(l, right, w)
			}
		}
	}
	return spread(left, "", w)
}

// chatChain is the line of a session in a chain: the chain, its step, and
// each member with its state in team order, the chosen one lit. false: the
// session is in no chain.
func (m Model) chatChain(s sample.Session, w int) (string, bool) {
	st := m.st
	for _, g := range m.sideGroups() {
		for _, u := range g.units {
			if u.chain == (chainKey{}) || !slices.ContainsFunc(u.members, func(o sample.Session) bool { return paneOf(o) == paneOf(s) }) {
				continue
			}
			n, of := u.step()
			line := " " + st.muted.Render("chain ") + st.text.Bold(true).Render(m.chainLabel(u.chain)) +
				st.muted.Render(" · step "+strconv.Itoa(n)+" of "+strconv.Itoa(of)) + "   "
			for j, o := range u.members {
				if j > 0 {
					line += st.faint.Render(" → ")
				}
				ink := st.muted
				if paneOf(o) == paneOf(s) {
					ink = st.accent
				}
				line += ink.Render(m.cardName(o) + " " + m.chainWord(u, j))
			}
			return fit(line, w), true
		}
	}
	return "", false
}

// chainWord is the state of member j of a chain: an idle member before
// the chain's step is done.
func (m Model) chainWord(u sideUnit, j int) string {
	switch m.sideStateOf(u.members[j]) {
	case sideNeed:
		return "needs you"
	case sideWork:
		return "works"
	case sideHeld:
		return "held"
	}
	if j < u.lead {
		return "done"
	}
	return m.stateWord(u.members[j])
}

// chatConversation is the session's pane cut to its conversation: the
// last rows that fit, under blank rows until a screen comes. Or it says
// why there is none: the host is not reached, or the session ended.
func (m Model) chatConversation(s sample.Session, w, rows int) []string {
	out := make([]string, rows)
	for i := range out {
		out[i] = strings.Repeat(" ", w)
	}
	say := func(text string) []string {
		if rows > 0 {
			out[rows/2] = fit(" "+m.st.muted.Render(text), w)
		}
		return out
	}
	if m.watchConn(s.Host) == nil {
		return say(m.nameOf(s.Host) + " is not reached.")
	}
	if m.watchHost != s.Host || m.watching != s.Pane || m.screen.Pane != s.Pane {
		return out
	}
	if m.screen.Gone {
		return say("That session ended.")
	}
	text := chatCut(s.Agent, m.screen.Text)
	var got []string
	if m.screen.Alt {
		got = cropScreen(text, w-1, rows)
	} else {
		got = fitScreen(text, w-1, rows)
	}
	for i, l := range got {
		out[rows-len(got)+i] = fit(" "+l+ansi.ResetStyle, w)
	}
	return out
}

// chatAsk is the question the digits answer, with its options: the one
// the session asks, or, while the stoker holds it, whether to let it go.
// The agent's own menu in the pane is the question: then only the mark
// shows, in one line. None when the digits answer nothing. In fewer than
// room lines the question keeps its first line, the mark and the options
// that fit, and says how many more it has.
func (m Model) chatAsk(s sample.Session, w, room int) []string {
	q, _, ok := m.digitMenu(s)
	if _, held := m.stokerHolds(s); held && !ok {
		q, ok = said.Question{Question: m.cardName(s) + " is " + m.heldWhy(s), Options: []said.Option{{Label: "Let it go"}}}, true
	}
	if !ok {
		return nil
	}
	armed := m.chatMark(s)
	var asks, options, mark []string
	if !m.paneAsks(s, q) {
		for _, l := range upTo(wrapPlain(q.Question, max(w-2, 1)), 2, max(w-2, 1)) {
			asks = append(asks, fit(" "+m.st.text.Bold(true).Render(l), w))
		}
		for i, o := range q.Options {
			options = append(options, m.optionLine(i+1, 0, ansi.Truncate(o.Label, max(w-6, 1), "…"), w, armed))
		}
	}
	n := strconv.Itoa(armed)
	switch _, sent := m.chatMarks(s); {
	case sent > 0:
		mark = []string{fit(" "+m.st.muted.Render("sent "+strconv.Itoa(sent)+" · waits for "+m.cardName(s)), w)}
	case armed > 0 && armed <= len(q.Options):
		mark = []string{fit(" "+m.st.accent.Render("❯ "+n+". "+q.Options[armed-1].Label)+
			m.st.muted.Render(" · "+n+" again sends · "+m.keys.side.Leave.Help().Key+" clears"), w)}
	}
	if len(asks)+len(options)+len(mark) > room {
		asks = asks[:min(len(asks), 1, max(room-len(mark), 0))] // the mark says what the next digit sends: it stays
		switch fits := room - len(asks) - len(mark); {
		case fits <= 0:
			options = nil
		case fits < len(options):
			keep := fits - 1
			more := fit("  "+m.st.muted.Render("+"+strconv.Itoa(len(options)-keep)+" more · "+m.keys.main.Jump.Help().Key+" shows all"), w)
			options = append(options[:keep], more)
		}
	}
	lines := append(append(asks, options...), mark...)
	return lines[:min(len(lines), room)]
}

// paneAsks: the cut pane of s on screen draws q's options as the agent's
// own menu (menu.Read): option N starts with the label of q's option N.
// A label in the pane's prose is no menu.
func (m Model) paneAsks(s sample.Session, q said.Question) bool {
	opts := m.screenMenu(s)
	if len(q.Options) == 0 || opts == nil {
		return false
	}
	for i, o := range q.Options {
		if !strings.HasPrefix(opts[i+1], o.Label) {
			return false
		}
	}
	return true
}

// screenMenu is the menu at the foot of s's pane, while the chat watches
// s and its screen is here; nil for none. It reads the whole screen, as
// the service does when the pick goes.
func (m Model) screenMenu(s sample.Session) map[int]string {
	if !m.screenOf(s) {
		return nil
	}
	return menu.Read(m.screen.Text)
}

// screenSign is menu.Sign of s's screen: what a pick from it carries.
func (m Model) screenSign(s sample.Session) string {
	if !m.screenOf(s) {
		return ""
	}
	return menu.Sign(m.screen.Text)
}

// screenOf: the chat watches s, and s's screen is here.
func (m Model) screenOf(s sample.Session) bool {
	return m.watchConn(s.Host) != nil && m.watchHost == s.Host && m.watching == s.Pane && m.screen.Pane == s.Pane && !m.screen.Gone
}

// chatLine is the line at the bottom: what the owner types, its end in
// view, and at the right whom it goes to, or what the line says of its
// text. A parked session's line is off. A note too long for the right
// of the line goes on the lines above it, wrapped.
func (m Model) chatLine(s sample.Session, w int) []string {
	st := m.st
	lead := " " + st.accent.Render("›") + " "
	if off := m.lineOff(s); off != "" {
		return []string{fit(lead+st.muted.Render(off), w)}
	}
	send := st.faint.Render(m.keys.side.Enter.Help().Key + " send to " + m.cardName(s))
	right := send
	var above []string
	if n := m.lineNote; n.k == paneOf(s) && n.text != "" {
		right = st.warn.Render(n.text)
	} else if _, waits := m.lineHeld[paneOf(s)]; waits {
		right = st.warn.Render(m.heldNote(s, false))
	} else if m.lines[paneOf(s)] == "" && m.asksNow(s) && !m.chatAnswers(s) {
		right = st.warn.Render(m.inPane())
		if s.UnknownScreen {
			what, fix := m.unknownNote(s)
			right = st.warn.Render(what + " " + fix)
			if lipgloss.Width(lead+right)+2 > w {
				right = send
				for _, l := range wrapKeeping(what, fix, max(w-2, 1)) {
					above = append(above, fit(" "+st.warn.Render(l), w))
				}
			}
		}
	}
	text := lineShown(m.lines[paneOf(s)])
	if room := w - lipgloss.Width(right) - 5; lipgloss.Width(text) > room {
		text = ansi.TruncateLeft(text, lipgloss.Width(text)-max(room-1, 0), "…")
	}
	return append(above, spread(lead+st.text.Render(text), right+" ", w))
}

// unknownNote is what the chat says of a wait whose screen no rule reads:
// the agent and its version, then the fix, which names the key that goes
// into the pane.
func (m Model) unknownNote(s sample.Session) (what, fix string) {
	agent := sample.AgentLabel(s.Agent) + " " + s.Version
	if s.Version == "" {
		agent = sample.AgentLabel(s.Agent) + ", version not read"
	}
	return "unknown screen · " + agent + " — answer it in its pane:", m.keys.main.Jump.Help().Key + " opens it"
}

// wrapKeeping is text and then tail wrapped to w, with tail whole on one
// line.
func wrapKeeping(text, tail string, w int) []string {
	lines := wrapPlain(text, w)
	if last := len(lines) - 1; last >= 0 && lipgloss.Width(lines[last]+" "+tail) <= w {
		lines[last] += " " + tail
		return lines
	}
	return append(lines, tail)
}

// menuOption is an option of an agent's menu, not its prompt: "❯ 1. Yes".
var menuOption = regexp.MustCompile(`^[›❯]\s*\d+\.`)

// footMax is the most lines with words an agent's foot holds under the
// line where its input box starts: the box, its border and the status
// lines.
const footMax = 8

// chatCut is an agent's pane cut to its conversation: its input box and
// the status lines under it go. The cut is made only at the agent's foot:
// the last line that starts the input box, with the frame's own end under
// it and at most footMax lines with words from it down. A past message
// that carries the prompt's mark is no foot, so with no input box on
// screen, and for an agent whose frame the cut does not know, the whole
// pane stays.
//   - Claude Code: a box whose first row is the prompt (╭─ over "│ >"),
//     closed by ╰; or a rule over a "> " prompt line, closed by a rule.
//   - Codex: the composer line, "› " or "▌", that is no option, over the
//     footer that says the context left.
//   - OpenCode: the first of the last run of "┃" lines, over the bottom
//     border (╹▀▀▀).
func chatCut(agent, text string) string {
	lines := strings.Split(onlyColours(text), "\n")
	plain := func(i int) string {
		if i < 0 || i >= len(lines) {
			return ""
		}
		return strings.TrimSpace(ansi.Strip(lines[i]))
	}
	prompt := func(p string) bool {
		return (p == ">" || strings.HasPrefix(p, "> ") || p == "❯" || strings.HasPrefix(p, "❯ ")) && !menuOption.MatchString(p)
	}
	var starts func(i int) bool
	var ends func(p string, i int) bool
	switch agent {
	case "claude":
		starts = func(i int) bool {
			inBox := strings.TrimSpace(strings.TrimPrefix(plain(i+1), "│"))
			return strings.HasPrefix(plain(i), "╭") && prompt(inBox) || isRule(lines[i]) && prompt(plain(i+1))
		}
		ends = func(p string, i int) bool { return strings.HasPrefix(p, "╰") || isRule(lines[i]) }
	case "codex":
		starts = func(i int) bool {
			p := plain(i)
			return (p == "›" || strings.HasPrefix(p, "› ") || strings.HasPrefix(p, "▌")) && !menuOption.MatchString(p)
		}
		ends = func(p string, _ int) bool { return strings.Contains(p, "context left") }
	case "opencode":
		starts = func(i int) bool { return strings.HasPrefix(plain(i), "┃") }
		ends = func(p string, _ int) bool { return strings.HasPrefix(p, "╹") || strings.HasPrefix(p, "▀") }
	default:
		return text
	}
	at := len(lines) - 1
	for at >= 0 && !starts(at) {
		at--
	}
	if agent == "opencode" {
		for at > 0 && starts(at-1) {
			at--
		}
	}
	if at < 0 {
		return text
	}
	words, closed := 0, false
	for i := at; i < len(lines); i++ {
		if p := plain(i); p != "" {
			words++
			closed = closed || i > at && ends(p, i)
		}
	}
	if !closed || words > footMax {
		return text
	}
	return strings.Join(lines[:at], "\n")
}
