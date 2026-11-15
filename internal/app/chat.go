package app

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
)

// chatPanel is the chosen session's chat in h lines of w cells: the head
// line, the chain line for a session in a chain, the conversation cut from
// its pane, the question with its options when it asks, and the line.
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
	case k.pane == "" || i < 0:
		text := "No sessions yet. + new session starts one here (" + m.keys.main.New.Help().Key + "). matchblox setup finds the agents you already run."
		lines = append(lines, blank)
		for _, l := range wrapPlain(text, max(w-2, 1)) {
			lines = append(lines, fit(" "+m.st.muted.Render(l), w))
		}
	default:
		s := listed[i]
		top := []string{m.chatHead(s, w)}
		if l, ok := m.chatChain(s, w); ok {
			top = append(top, l)
		}
		ask := m.chatAsk(s, w)
		rows := h - len(top) - len(ask) - 1
		if rows < 0 {
			ask, rows = nil, h-len(top)-1
		}
		lines = append(top, m.chatConversation(s, w, max(rows, 0))...)
		lines = append(lines, ask...)
		lines = append(lines, m.chatLine(s, w))
		lines = lines[max(len(lines)-h, 0):] // the line stays in view
	}
	for len(lines) < h {
		lines = append(lines, blank)
	}
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
	left := " " + st.text.Bold(true).Render(m.cardName(s)) + " " + st.muted.Render(about)
	right := st.faint.Render(m.keys.main.Jump.Help().Key+" expand · "+m.keys.board.Menu.Help().Key+" menu") + " "
	if s.Context == "known" {
		right = m.contextStyle(s).Render("ctx "+strconv.Itoa(int(s.ContextPct+0.5))+"%") + "  " + right
	}
	return spread(left, right, w)
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

// chatAsk is the question the session asks, with its options: none when
// it asks no question with options.
func (m Model) chatAsk(s sample.Session, w int) []string {
	q, ok := m.oneChoice(s)
	if !ok {
		return nil
	}
	var lines []string
	for _, l := range upTo(wrapPlain(q.Question, max(w-2, 1)), 2, max(w-2, 1)) {
		lines = append(lines, fit(" "+m.st.text.Bold(true).Render(l), w))
	}
	for i, o := range q.Options {
		lines = append(lines, m.optionLine(i+1, 0, ansi.Truncate(o.Label, max(w-6, 1), "…"), w, 0))
	}
	return lines
}

// chatLine is the line at the bottom: what the owner types, and at the
// right whom it goes to.
func (m Model) chatLine(s sample.Session, w int) string {
	to := m.st.faint.Render(m.keys.main.Open.Help().Key + " send to " + m.cardName(s))
	return spread(" "+m.st.accent.Render("›")+" ", to+" ", w)
}

// menuOption is an option of an agent's menu, not its prompt: "❯ 1. Yes".
var menuOption = regexp.MustCompile(`^[›❯]\s*\d+\.`)

// chatCut is an agent's pane cut to its conversation: its input box and
// the status lines under it go. An agent whose frame the cut does not know keeps its whole pane.
//   - Claude Code: from the last box whose first row is the prompt
//     (╭─ over "│ >"), or the last rule over a "> " prompt line.
//   - Codex: from the last composer line, "› " or "▌", that is no option.
//   - OpenCode: from the first of the last run of "┃" lines.
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
	at := -1
	for i := len(lines) - 1; i >= 0 && at < 0; i-- {
		p := plain(i)
		switch agent {
		case "claude":
			inBox := strings.TrimSpace(strings.TrimPrefix(plain(i+1), "│"))
			if strings.HasPrefix(p, "╭") && prompt(inBox) || isRule(lines[i]) && prompt(plain(i+1)) {
				at = i
			}
		case "codex":
			if (p == "›" || strings.HasPrefix(p, "› ") || strings.HasPrefix(p, "▌")) && !menuOption.MatchString(p) {
				at = i
			}
		case "opencode":
			if strings.HasPrefix(p, "┃") {
				for at = i; strings.HasPrefix(plain(at-1), "┃"); at-- {
				}
			}
		default:
			return text
		}
	}
	if at < 0 {
		return text
	}
	return strings.Join(lines[:at], "\n")
}
