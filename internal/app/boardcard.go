package app

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
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

// cardName is the name a card goes by: a chain stoker's agent is its
// chain's stoker (ruling 42), not its tmux session.
func (m Model) cardName(s sample.Session) string {
	if k, ok := m.stokerChain(s); ok {
		return m.roomStokerWord(k)
	}
	return s.Name
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

// digitMenu is the menu the digits answer on s, as a question for the
// chat to draw. The question tool's one question with one choice comes
// from its call, and a pick: act answers it: texts is nil. Any other menu
// is the one the agent draws on the chat's screen while s needs you, with
// its options' whole texts by number, and a choose: act answers it. None
// for a menu of checkboxes: a digit only ticks a box there, and Submit
// has no number. None on a screen the service cannot read: the console's
// reader may still find numbers there, but no rule says what they answer.
func (m Model) digitMenu(s sample.Session) (q said.Question, texts map[int]string, ok bool) {
	if q, one := m.oneChoice(s); one {
		return q, nil, true
	}
	texts = m.screenMenu(s)
	if texts == nil || !m.asksNow(s) || s.UnknownScreen {
		return q, nil, false
	}
	for n := 1; n <= len(texts); n++ {
		if checkbox.MatchString(texts[n]) {
			return said.Question{}, nil, false
		}
		q.Options = append(q.Options, said.Option{Label: texts[n]})
	}
	return q, texts, true
}

// checkbox starts an option of a menu that picks many.
var checkbox = regexp.MustCompile(`^\[[ xX✓✔]\]`)

// upTo keeps the first n lines, the last cut with … when there were more.
func upTo(ls []string, n, w int) []string {
	if len(ls) <= n {
		return ls
	}
	ls = ls[:n]
	ls[n-1] = ansi.Truncate(ls[n-1]+" …", w, "…")
	return ls
}
