package app

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

// Sessions is one list in three groups, by what each session needs from
// the person (design/0006 §5.1). The doors, the stoker's card and the
// recommendations sit above them.
const (
	groupWaits = iota
	groupWorking
	groupQuiet
)

var groupNames = [...]string{"WAITS FOR YOU", "WORKING", "QUIET"}

// waitOf is the queue row of a session that waits: by its pane, or by its
// id outside tmux, where no pane tells sessions apart.
func (m Model) waitOf(s sample.Session) (queue.Item, bool) {
	for _, it := range m.queue {
		if s.Pane != "" && it.Pane == s.Pane || s.Pane == "" && s.SessionID != "" && it.SessionID == s.SessionID {
			return it, true
		}
	}
	return queue.Item{}, false
}

// groupOf is the group of a session. One that waits since yesterday is
// cold, not waiting; one the plan's limit stops can do nothing now.
func (m Model) groupOf(s sample.Session) int {
	switch _, waits := m.waitOf(s); {
	case s.Busy:
		return groupWorking
	case s.Age() != "idle" || s.Stopped():
		return groupQuiet
	case waits:
		return groupWaits
	case len(s.Shells) > 0:
		return groupWorking
	}
	return groupQuiet
}

// grouped: the default sort is on, so the list shows its groups. Any
// other sort is one flat list.
func (m Model) grouped() bool { return m.sessSort == sortBy{} }

var testRunner = regexp.MustCompile(`\b(go test|pytest|jest|vitest|playwright|mocha|rspec|cargo test|(npm|pnpm|yarn|bun)( run)? (test|e2e)|make (test|e2e|check))\b`)

// wordOf is the state of a session in a row: what it does, the word of
// its queue row, or its age.
func (m Model) wordOf(s sample.Session) string {
	switch {
	case s.Busy:
		return busyWord(s)
	case m.groupOf(s) == groupWorking:
		for _, sh := range s.Shells {
			if testRunner.MatchString(sh.Command) {
				return "tests"
			}
		}
		return "shell"
	}
	return m.stateWord(s)
}

// waitCell is how long a session waits, or sat idle, or when the plan's
// limit lets it work again; a session at work has none.
func (m Model) waitCell(s sample.Session) string {
	if it, ok := m.waitOf(s); ok && m.groupOf(s) == groupWaits {
		return brief(m.now().Sub(it.Since))
	}
	if m.groupOf(s) == groupWorking {
		return ""
	}
	if s.Stopped() {
		return m.idleCell(s)
	}
	return brief(s.Idle)
}

// brief is the first unit of a duration: 4m, 2h, 20d.
func brief(d time.Duration) string {
	h := sample.Human(d)
	for i, r := range h {
		if r < '0' || r > '9' {
			return h[:i+1]
		}
	}
	return h
}

// firstWaiting is the first session of the list that waits, or -1.
func (m Model) firstWaiting() int {
	for i, s := range m.snap.Sessions {
		if m.groupOf(s) == groupWaits {
			return i
		}
	}
	return -1
}

// sessIndex is the selected row of Sessions: the doors, the
// recommendations, then the sessions. A door that folds passes the
// selection to the next door. Nothing picked: the stoker's card, else the
// first session that waits, else the first door.
func (m Model) sessIndex() int {
	nd, nr := len(m.doors), len(m.recs)
	if nd > 0 && m.doors[0].ID == stokedRow && m.doorPick == "" && m.recPick == "" && !m.chose {
		return 0 // the night's steps come first in the morning
	}
	for i, d := range m.doors {
		if d.ID == m.doorPick {
			return i
		}
	}
	if nd > 0 && m.doorPick != "" {
		return min(m.doorAt, nd-1)
	}
	for i, r := range m.recs {
		if r.ID == m.recPick && m.recPick != "" {
			return nd + i
		}
	}
	if nd > 0 && !m.chose && m.firstWaiting() < 0 {
		return 0
	}
	if len(m.snap.Sessions) == 0 {
		if nr > 0 {
			return nd
		}
		return 0
	}
	return nd + nr + m.selIndex()
}

// selectedSession is the session under the selection in Sessions; a door
// or a recommendation has none.
func (m Model) selectedSession() (sample.Session, bool) {
	if m.tab != tabSessions || m.sessIndex() < len(m.doors)+len(m.recs) {
		return sample.Session{}, false
	}
	return m.selected()
}

func (m Model) selectedDoor() (doors.Door, bool) {
	if m.tab != tabSessions {
		return doors.Door{}, false
	}
	if i := m.sessIndex(); i < len(m.doors) {
		return m.doors[i], true
	}
	return doors.Door{}, false
}

// selectedRec is the recommendation selected above the sessions.
func (m Model) selectedRec() (advice.Rec, bool) {
	if m.tab != tabSessions {
		return advice.Rec{}, false
	}
	i := m.sessIndex() - len(m.doors)
	if i < 0 || i >= len(m.recs) {
		return advice.Rec{}, false
	}
	return m.recs[i], true
}

// sessionKeys are the keys of the selected row of Sessions.
func (m Model) sessionKeys() []hint {
	if d, ok := m.selectedDoor(); ok {
		if d.ID == stokedRow {
			return []hint{{"⏎", "fold", "enter"}, {"f", "stoker", "f"}}
		}
		return []hint{{"⏎", "open", "enter"}, {"x", "close", "x"}}
	}
	if _, ok := m.selectedRec(); ok {
		return []hint{{"⏎", "do", "enter"}, {"x", "other", "x"}, {"f", "stoker", "f"}}
	}
	keys := []hint{{"⏎", "open", "enter"}}
	if s, ok := m.selectedSession(); ok && s.Age() == "stale" {
		keys = append(keys, hint{"x", "end", "x"})
	}
	return append(keys, hint{"a", "answer", "a"}, hint{"p", "ships", "p"}, hint{"f", "stoker", "f"})
}

func (m Model) sessionsPanel(w int) body {
	var b body
	m.doorsSection(&b, w)
	m.recsSection(&b, w)
	m.sessionRows(&b, w)
	st := m.st
	if n := len(m.snap.IdlePanes); n > 0 {
		var ids []string
		for _, p := range m.snap.IdlePanes {
			ids = append(ids, place(p.Target, p.Pane)+" "+p.Command)
		}
		b.add(-1, "", st.faint.Render(fit(fmt.Sprintf(" %d without an agent: %s", n, strings.Join(ids, ", ")), w)))
	}
	b.add(-1, m.exitedLines(w)...)
	return b
}

// sessionRows are the sessions under a faint label for each group, or
// under the sort when another one is on.
func (m Model) sessionRows(b *body, w int) {
	st := m.st
	ss := m.snap.Sessions
	if len(b.lines) > 0 {
		b.add(-1, "")
	}
	if len(ss) == 0 {
		b.add(-1, st.faint.Render(m.nothing(" no agent sessions found in tmux")))
		return
	}
	base := len(m.doors) + len(m.recs)
	sel := m.sessIndex() - base
	var count [len(groupNames)]int
	for _, s := range ss {
		count[m.groupOf(s)]++
	}
	if !m.grouped() {
		b.add(-1, st.label.Render(fit(fmt.Sprintf(" %d SESSIONS · by %s %s", len(ss), sessCols[m.sessSort.col], m.sessSort.arrow()), w)))
	}
	group := -1
	for i, s := range ss {
		if g := m.groupOf(s); m.grouped() && g != group {
			group = g
			b.add(-1, st.label.Render(fit(fmt.Sprintf(" %s · %d", groupNames[g], count[g]), w)))
		}
		b.addRow(base+i, i == sel, w, st, m.sessionRow(s, i == sel, w)...)
	}
}

// sessionRow is one line for a session. The desk adds the plan, the pull
// request and the place; on a phone the selected one opens in place.
func (m Model) sessionRow(s sample.Session, sel bool, w int) []string {
	st := m.st
	mark := " "
	if sel {
		mark = st.accent.Render("▌")
	}
	word := st.muted.Render(m.wordOf(s))
	if m.groupOf(s) == groupWaits {
		word = st.accent.Render(m.wordOf(s))
	}
	wait := st.faint.Render(m.waitCell(s))
	ctx := m.contextCell(s)
	if w >= 70 {
		name := st.text.Render(pad(s.Name, 25))
		plan := pad("", 13)
		switch {
		case s.Do == "compact":
			plan = st.neg.Render(pad("! compact", 13))
		case s.Do == "clear":
			plan = st.warn.Render(pad("▲ clear", 13))
		case s.Progress != nil:
			plan = padTo(m.progressCell(s.Progress, 5), 13)
		}
		pr := pad("", 7)
		if p := m.prOf(s.Cwd); p != nil {
			pr = st.accent.Render(pad("#"+strconv.Itoa(p.Number), 7))
		}
		line := mark + m.matchCell(s) + name + padTo(word, 8) + padLeft(wait, 4) + padLeft(ctx, 6) + "  " + plan + pr +
			st.muted.Render(place(s.Target, s.Pane))
		return []string{fit(line, w)}
	}
	wordW := 8
	if layout(m.width) == Narrow {
		wordW = 7
	}
	line := spread(mark+m.matchCell(s)+st.text.Render(s.Name), padTo(word, wordW)+padLeft(wait, 3)+padLeft(ctx, 6)+" ", w)
	if !sel || layout(m.width) != Narrow {
		return []string{line}
	}
	return append([]string{line}, m.opened(s, w)...)
}

// opened is what a selected row on a phone grows by: what the agent said
// last, its pull request, then its plan and its keys.
func (m Model) opened(s sample.Session, w int) []string {
	st := m.st
	var out []string
	if said := m.lastWords(s); said != "" {
		lines := wrapPlain("“"+said+"”", w-4)
		if len(lines) > 2 {
			lines = []string{lines[0], ansi.Truncate(lines[1], w-5, "") + "…"}
		}
		for _, l := range lines {
			out = append(out, "   "+st.muted.Render(l))
		}
	}
	if p := m.prOf(s.Cwd); p != nil {
		title := p.Title
		if p.Draft {
			title = "draft " + title
		}
		out = append(out, "   "+st.accent.Render("#"+strconv.Itoa(p.Number))+" "+st.muted.Render(title))
	}
	left := "  "
	if p := s.Progress; p != nil {
		full := min(max((p.Pct*5+50)/100, 0), 5)
		left += " " + st.text.Render(strings.Repeat("▰", full)) + st.faint.Render(strings.Repeat("▱", 5-full)) +
			st.muted.Render(fmt.Sprintf(" plan %d%%", p.Pct))
	}
	switch s.Do {
	case "compact":
		left += "  " + st.neg.Render("! compact")
	case "clear":
		left += "  " + st.warn.Render("▲ clear")
	}
	keys := st.accent.Render("⏎") + st.muted.Render(" open") + " "
	if pane, _ := m.answerTarget(); pane != "" {
		keys = st.accent.Render("a") + st.muted.Render(" answer  ") + keys
	}
	return append(out, spread(left, keys, w))
}

// lastWords is what the agent said last: its queue row's message while it
// waits, else the last line read from its reply or its pane.
func (m Model) lastWords(s sample.Session) string {
	if it, ok := m.waitOf(s); ok && it.LastLine != "" {
		return it.LastLine
	}
	return s.LastLine
}

// contextCell is a session's context use: warn from 70%, neg past the
// line; fresh before the first turn, nothing when it is not measured.
func (m Model) contextCell(s sample.Session) string {
	switch s.Context {
	case "known":
	case "fresh":
		return m.st.faint.Render("fresh")
	default:
		return ""
	}
	style := m.st.muted
	switch {
	case m.full().Reached(s):
		style = m.st.neg
	case s.ContextPct >= 70:
		style = m.st.warn
	}
	return style.Render(fmt.Sprintf("%.0f%%", s.ContextPct))
}

// longWord is a session's state in words, for the detail.
func (m Model) longWord(s sample.Session) string {
	switch w := m.wordOf(s); w {
	case "asks":
		return "asks you"
	case "waits":
		return "waits for you"
	case "done", "paused":
		return w + ", waits for you"
	case "busy":
		return "working"
	case "agents":
		return "working through subagents"
	case "tests":
		return "running tests"
	case "shell":
		return "running a shell"
	case "out":
		return "out until " + m.idleCell(s)
	case "stale":
		return "stale, x ends it"
	default:
		return w
	}
}

// detailPane is the right pane of Sessions on an opened phone and a desk:
// the selected session, recommendation or door.
func (m Model) detailPane(w int) []string {
	st := m.st
	if d, ok := m.selectedDoor(); ok {
		if d.ID == stokedRow {
			return []string{"", " " + st.accent.Render("The stoker's steps"), "", " " + st.accent.Render("⏎") + st.muted.Render(" folds them")}
		}
		out := []string{"", fit(" "+st.accent.Render(d.Title), w), ""}
		text, style := d.Why, st.muted
		if d.Problem != "" {
			text, style = d.Problem, st.warn
		}
		for _, l := range wrapPlain(text, w-2) {
			out = append(out, " "+style.Render(l))
		}
		return append(out, "", " "+st.accent.Render("⏎")+st.muted.Render(" open   ")+st.accent.Render("x")+st.muted.Render(" close"))
	}
	if r, ok := m.selectedRec(); ok {
		out := []string{""}
		for _, l := range wrapPlain(tilde(r.Title), w-2) {
			out = append(out, " "+st.accent.Render(l))
		}
		out = append(out, "")
		for _, l := range wrapPlain(tilde(r.Evidence), w-2) {
			out = append(out, " "+st.muted.Render(l))
		}
		return append(append(out, ""), m.recActions(r, w)...)
	}
	s, ok := m.selectedSession()
	if !ok {
		return []string{"", st.faint.Render(m.nothing(" no agent sessions found in tmux"))}
	}
	return m.sessionDetail(s, w)
}

func (m Model) sessionDetail(s sample.Session, w int) []string {
	st := m.st
	out := []string{"", fit(" "+st.accent.Bold(true).Render(s.Name), w)}
	where := []string{m.longWord(s)}
	if c := m.waitCell(s); c != "" && m.wordOf(s) != "out" {
		where = append(where, c)
	}
	where = append(where, place(s.Target, s.Pane), tilde(s.Cwd))
	out = append(out, fit(" "+m.matchCell(s)+st.muted.Render(strings.Join(where, " · ")), w))
	if said := m.lastWords(s); said != "" {
		agent := s.Agent
		if agent == "" {
			agent = "it"
		}
		out = append(out, "", " "+st.label.Render(strings.ToUpper(agent)+" SAID"))
		lines := wrapPlain(said, w-3)
		if len(lines) > 6 {
			lines = append(lines[:5], ansi.Truncate(lines[5], w-4, "")+"…")
		}
		for _, l := range lines {
			out = append(out, " "+st.text.Render(l))
		}
	}
	out = append(out, "")
	field := func(name, value string) {
		out = append(out, fit(" "+st.label.Render(pad(name, 9))+value, w))
	}
	if s.Context == "known" {
		field("context", m.meter(s.ContextPct, 10, m.contextStyle(s))+" "+m.contextCell(s))
	} else {
		field("context", st.faint.Render(contextWord(s.Context)))
	}
	if p := s.Progress; p != nil {
		field("plan", m.progressCell(p, 10)+"  "+st.text.Render(p.Step))
	}
	if p := m.prOf(s.Cwd); p != nil {
		title := p.Title
		if p.Draft {
			title = "draft " + title
		}
		field("ship", st.accent.Render("#"+strconv.Itoa(p.Number))+" "+st.text.Render(title))
		if p.URL != "" {
			out = append(out, fit(" "+pad("", 9)+st.faint.Render(p.URL), w))
		}
	}
	if s.Agents > 0 {
		field("agents", st.text.Render(plural(s.Agents, "subagent")+" running"))
	}
	if len(s.Shells) > 0 {
		shells := plural(len(s.Shells), "shell")
		if old := s.Shells[0].Started; !old.IsZero() {
			shells += " · oldest " + sample.Human(m.now().Sub(old))
		}
		field("shells", st.text.Render(shells))
		for _, sh := range s.Shells {
			out = append(out, fit("   "+st.muted.Render(strconv.Itoa(sh.PID)+"  "+sh.Command), w))
		}
	}
	switch {
	case s.Do != "" && s.Why != "":
		field("why", st.text.Render(s.Do+": "+s.Why))
	case s.Why != "":
		field("why", st.text.Render(s.Why))
	}
	facts := m.facts(s)
	for _, l := range wrapPlain(strings.Join(facts, " · "), w-2) {
		out = append(out, " "+st.faint.Render(l))
	}
	if s.FromPane {
		for _, l := range wrapPlain("no hooks for "+s.Name+": busy and idle are read from the pane", w-2) {
			out = append(out, " "+st.faint.Render(l))
		}
	}
	keys := " "
	if pane, _ := m.answerTarget(); pane != "" {
		keys += st.accent.Render("a") + st.muted.Render(" answer   ")
	}
	if s.Pane != "" {
		keys += st.accent.Render("⏎") + st.muted.Render(" open it")
	}
	if s.Age() == "stale" {
		keys += "   " + st.accent.Render("x") + st.muted.Render(" end it")
	}
	return append(out, "", fit(keys, w))
}

// facts are the figures of a session the rows have no room for.
func (m Model) facts(s sample.Session) []string {
	var facts []string
	if s.Model != "" {
		facts = append(facts, s.Model)
	}
	switch {
	case s.Tokens > 0 && s.Window > 0:
		facts = append(facts, fmt.Sprintf("%s / %s tokens", sample.Kilo(s.Tokens), sample.Kilo(s.Window)), "+"+sample.Kilo(s.Burn30m)+" in 30 min")
	case s.Tokens > 0:
		facts = append(facts, sample.Kilo(s.Tokens)+" tokens · no window known: add the model to [sessions.windows]")
	}
	if a := accountCell(s); a != "" {
		facts = append(facts, a)
	}
	facts = append(facts, fmt.Sprintf("pid %d", s.PID), plural(s.Procs, "process"))
	if !s.Started.IsZero() {
		facts = append(facts, "started "+sample.Human(m.now().Sub(s.Started))+" ago")
	}
	return facts
}

func (m Model) contextStyle(s sample.Session) lipgloss.Style {
	switch {
	case m.full().Reached(s):
		return m.st.neg
	case s.ContextPct >= 70:
		return m.st.warn
	}
	return m.st.muted
}

// meter is a bar of n cells for a percent.
func (m Model) meter(pct float64, n int, style lipgloss.Style) string {
	full := min(max(int(pct/100*float64(n)+0.5), 0), n)
	return style.Render(strings.Repeat("▰", full)) + m.st.faint.Render(strings.Repeat("▱", n-full))
}

// wrapPlain breaks text into lines of at most w cells, a word too long
// for a line cut into pieces.
func wrapPlain(s string, w int) []string {
	w = max(w, 4)
	var out []string
	line := ""
	for _, word := range strings.Fields(s) {
		for lipgloss.Width(word) > w {
			if line != "" {
				out, line = append(out, line), ""
			}
			head := ansi.Truncate(word, w, "")
			out, word = append(out, head), word[len(head):]
		}
		switch {
		case line == "":
			line = word
		case lipgloss.Width(line)+1+lipgloss.Width(word) <= w:
			line += " " + word
		default:
			out, line = append(out, line), word
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// spread puts left and right on one line of w cells; left is cut first.
func spread(left, right string, w int) string {
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if lw+rw+1 > w {
		return fit(ansi.Truncate(left, max(w-rw-1, 0), "…")+" "+right, w)
	}
	return left + strings.Repeat(" ", w-lw-rw) + right
}

// padTo pads styled text to w cells, cutting it when it is wider.
func padTo(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(w-lipgloss.Width(s), 0))
}

// padLeft right-aligns styled text in w cells.
func padLeft(s string, w int) string {
	return strings.Repeat(" ", max(w-lipgloss.Width(s), 0)) + s
}
