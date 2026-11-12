package app

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/accounts"
	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
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
	groupHeld
)

var groupNames = [...]string{"WAITS FOR YOU", "WORKING", "QUIET", "HELD"}

// holdByYou is the hold of the builder; the stoker's are the others.
const holdByYou = "you"

// waitOf is the queue row of a session that waits: by its pane, or by its
// id outside tmux, where no pane tells sessions apart.
func (m Model) waitOf(s sample.Session) (queue.Item, bool) {
	for _, it := range m.queue {
		if it.Host != s.Host {
			continue
		}
		if s.Pane != "" && it.Pane == s.Pane || s.Pane == "" && s.SessionID != "" && it.SessionID == s.SessionID {
			return it, true
		}
	}
	return queue.Item{}, false
}

// groupOf is the group of a session. One that waits since yesterday is
// cold, not waiting; one the plan's limit stops can do nothing now.
func (m Model) groupOf(s sample.Session) int {
	if _, held := m.holdOf(s); held {
		return groupHeld
	}
	if _, head := m.headRow(s); head && (s.Asks || s.Permits) { // a chain stoker's agent at a menu
		return groupWaits
	}
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

// holdOf is the hold on a session: by its id, or by its pane outside
// tmux's reach of ids.
func (m Model) holdOf(s sample.Session) (proto.Hold, bool) {
	for _, h := range m.stateOf(s.Host).Holds {
		if s.SessionID != "" && h.SessionID == s.SessionID || s.SessionID == "" && s.Pane != "" && h.Pane == s.Pane {
			return h, true
		}
	}
	return proto.Hold{}, false
}

// groupLabel is the group a session is listed under: its need.
func (m Model) groupLabel(s sample.Session) string { return groupNames[m.needGroup(s)] }

// repoOf is the repository a session works in: the checkout its directory
// is in, else the directory's own name.
func (m Model) repoOf(s sample.Session) string {
	if repo, _, ok := proto.RepoOf(m.git, s.Cwd); ok {
		return filepath.Base(repo)
	}
	if s.Cwd == "" {
		return "no directory"
	}
	return filepath.Base(s.Cwd)
}

// rowWord is the state cell of a session row. merged: the cell is a hold
// or a reason, long enough to take the wait column's cells too.
func (m Model) rowWord(s sample.Session) (word string, merged bool) {
	st := m.st
	if m.hostCount() > 1 && m.stale(s.Host) {
		return st.faint.Render("stale"), false
	}
	if h, ok := m.holdOf(s); ok {
		if h.By == holdByYou {
			return st.muted.Render("⏸ parked by you"), true
		}
		why := "held by stoker"
		if h.Why != "" {
			why += " · " + h.Why
		}
		return st.muted.Render("⏸ " + why), true
	}
	if _, ok := m.refreshOf(s.Host, s.Pane); ok {
		return st.accent.Render("writes its record"), true
	}
	plain := m.wordOf(s)
	asks := plain == "asks" || plain == "waits"
	if s.SigningIn && !asks {
		// The new account's sign-in is not done: in the pane, and by hand
		// when matchblox gave up driving it. A note whose pane was not
		// found names the tmux session. An ask or a wait shows instead,
		// so it is never hidden.
		tmuxName, _, _ := strings.Cut(s.Target, ":")
		signIns := m.stateOf(s.Host).SignIns
		if signIns[s.Pane] == "by hand" || tmuxName != "" && signIns[tmuxName] == "by hand" {
			return st.accent.Render("sign in by hand"), true
		}
		return st.muted.Render("signing in"), true
	}
	if r, ok := m.stateOf(s.Host).Reasons[s.Pane]; ok && r.Word != "" && s.Pane != "" && !asks {
		return m.tone(r.Tone).Render(r.Word), true
	}
	if m.groupOf(s) == groupWaits {
		return st.accent.Render(plain), false
	}
	return st.muted.Render(plain), false
}

// tone is the style a reason is drawn in; ok has no colour of its own.
func (m Model) tone(t string) lipgloss.Style {
	switch t {
	case "warn":
		return m.st.warn
	case "neg":
		return m.st.neg
	case "faint":
		return m.st.faint
	}
	return m.st.accent
}

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
// limit lets it work again; a session at work has none. A stale host of
// two or more: how long since it last spoke.
func (m Model) waitCell(s sample.Session) string {
	if m.hostCount() > 1 && m.stale(s.Host) {
		return brief(m.now().Sub(m.stateAt(s.Host)))
	}
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

// sessIndex is the selected row of Sessions: the doors, the sessions,
// the ghost rows, then the recommendations. A door that folds passes the
// selection to the next door. Nothing picked: the first session that
// waits, else the first door; with no session, the first ghost row.
func (m Model) sessIndex() int {
	nd, ns, ng, nr := len(m.doors), len(m.snap.Sessions), len(m.ghosts())+len(m.blockedHosts()), m.recRows()
	for i, d := range m.doors {
		if d.ID == m.doorPick {
			return i
		}
	}
	if nd > 0 && m.doorPick != "" {
		return min(m.doorAt, nd-1)
	}
	for i, r := range m.recs[:nr] {
		if r.ID == m.recPick && m.recPick != "" {
			return nd + ns + ng + i
		}
	}
	if m.ghostPick > 0 {
		return nd + ns + min(m.ghostPick, ng) - 1
	}
	if ns == 0 {
		return nd
	}
	if nd > 0 && !m.chose && m.firstWaiting() < 0 {
		return 0
	}
	return nd + m.selIndex()
}

// selectedSession is the session under the selection in Sessions; a door
// or a recommendation has none.
func (m Model) selectedSession() (sample.Session, bool) {
	if i := m.sessIndex(); m.tab != tabSessions || i < len(m.doors) || i >= len(m.doors)+len(m.snap.Sessions) {
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

// selectedRec is the recommendation selected under the sessions.
func (m Model) selectedRec() (advice.Rec, bool) {
	if m.tab != tabSessions {
		return advice.Rec{}, false
	}
	i := m.sessIndex() - len(m.doors) - len(m.snap.Sessions) - len(m.ghosts()) - len(m.blockedHosts())
	if i < 0 || i >= m.recRows() {
		return advice.Rec{}, false
	}
	return m.recs[i], true
}

// maxTreeDepth is how many levels of children are drawn under a session.
const maxTreeDepth = 4

// childrenOf are the children started by one process of host, in
// snapshot order.
func (m Model) childrenOf(host string, parent sample.ProcRef) []sample.Session {
	var out []sample.Session
	for _, c := range m.snap.Children {
		if c.Host == host && c.Parent != nil && *c.Parent == parent {
			out = append(out, c)
		}
	}
	return out
}

func (m Model) descendants(host string, parent sample.ProcRef) int {
	n := 0
	for _, c := range m.childrenOf(host, parent) {
		n += 1 + m.descendants(host, sample.ProcRef{PID: c.PID, Start: c.Start})
	}
	return n
}

// childRows are the lines under one session or child, depth first: a
// branch for each child and a fold for what lies below the last level.
// indent carries a bar down past each ancestor that has siblings below.
func (m Model) childRows(host string, parent sample.ProcRef, indent string, depth, w int) []string {
	st := m.st
	kids := m.childrenOf(host, parent)
	if len(kids) == 0 {
		return nil
	}
	if depth > maxTreeDepth {
		return []string{fit(st.faint.Render(indent+"└─ … "+strconv.Itoa(m.descendants(host, parent))+" more"), w)}
	}
	var out []string
	for i, c := range kids {
		branch, below := "├─ ", "│ "
		if i == len(kids)-1 {
			branch, below = "└─ ", "  "
		}
		line := st.faint.Render(indent+branch) + st.text.Render(c.Name)
		if c.Status != "" {
			line += st.faint.Render(" " + c.Status)
		}
		out = append(out, fit(line, w))
		out = append(out, m.childRows(host, sample.ProcRef{PID: c.PID, Start: c.Start}, indent+below, depth+1, w)...)
	}
	return out
}

// accountTag is the account a session runs on, for a row: empty on the
// default sign-in.
func (m Model) accountTag(s sample.Session) string {
	if s.AccountDir == "" {
		return ""
	}
	return accounts.Name(s.AccountDir)
}

func shift(spans []span, dx int) []span {
	out := make([]span, len(spans))
	for i, s := range spans {
		out[i] = span{s.x0 + dx, s.x1 + dx, s.press}
	}
	return out
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
// line; fresh before the first turn, a dash when it is not measured.
func (m Model) contextCell(s sample.Session) string {
	switch s.Context {
	case "known":
	case "fresh":
		return m.st.faint.Render("fresh")
	case "unmeasured":
		return m.st.faint.Render("—")
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

// facts are the figures of a session the rows have no room for.
func (m Model) facts(s sample.Session) []string {
	var facts []string
	if s.Model != "" {
		facts = append(facts, s.Model)
	}
	switch {
	case s.Tokens > 0 && s.Window > 0:
		facts = append(facts, fmt.Sprintf("%s / %s tokens", sample.Kilo(s.Tokens), sample.Kilo(s.Window)), "+"+sample.Kilo(s.Burn30m)+" in 30 min")
	case s.Tokens > 0 && s.Agent == "claude":
		facts = append(facts, sample.Kilo(s.Tokens)+" tokens · no window known: run matchblox setup, or add the model to [sessions.windows]")
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
