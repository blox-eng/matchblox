package app

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/limits"
)

// The strip is the board's one line above the key line's hairline: usage
// or the machine, as u or a tap turns it. Its row also carries every
// notice (the spec's "The bottom strip" and "Notices").

// noticeFor is how long a notice shows in the strip's row.
const noticeFor = 2500 * time.Millisecond

// noticeLines is the most lines a notice takes of the board.
const noticeLines = 3

// noticeMsg comes when a notice ends: the frame drops it.
type noticeMsg struct{}

// notice is the one way to show a toast, a refusal or the data's age on
// the board: text takes the strip's row for noticeFor. The header never
// shows it.
func (m *Model) notice(text string) tea.Cmd {
	m.noticeText, m.noticeUntil, m.goArmed, m.noticeNext = text, m.now().Add(noticeFor), paneKey{}, ""
	return tea.Tick(noticeFor, func(time.Time) tea.Msg { return noticeMsg{} })
}

// noticeEnded shows the notice that waits for the one that ends.
func (m *Model) noticeEnded() tea.Cmd {
	if m.noticeNext == "" || m.now().Before(m.noticeUntil) {
		return nil
	}
	return m.notice(m.noticeNext)
}

// flashNotice puts a new flash in the strip's row while the board shows,
// so the board's key line keeps its keys. was: the flash before the
// update. A take-over question keeps its flash on the key line.
func (m *Model) flashNotice(was string) tea.Cmd {
	if !m.boardShown() || m.viewFull() || m.held != nil || m.flash == "" || m.flash == was {
		return nil
	}
	return m.notice(m.flash)
}

// noticeRows are a notice in w columns: one line, more on every shape
// when it does not fit, so its fix is never cut. Only a text longer than
// any of the board's own runs past noticeLines and ends in "…".
func (m Model) noticeRows(text string, w int) []string {
	lines := []string{text}
	if ansi.StringWidth(text) > w-1 {
		lines = upTo(cutNotice(text, w-1), noticeLines, w-1)
	}
	for i, l := range lines {
		lines[i] = fit(" "+m.st.text.Render(l), w)
	}
	return lines
}

// cutNotice breaks text into lines of w cells: at its last " · " where
// both halves fit, else at its spaces.
func cutNotice(text string, w int) []string {
	for i := strings.LastIndex(text, " · "); i >= 0; i = strings.LastIndex(text[:i], " · ") {
		first, rest := text[:i+len(" ·")], text[i+len(" · "):]
		if ansi.StringWidth(first) <= w && ansi.StringWidth(rest) <= w {
			return []string{first, rest}
		}
	}
	return wrapPlain(text, w)
}

// strip is the strip's row in w columns: a notice, else the face it
// shows with the word of the next face at its right. tap covers the row
// and presses u, under the board's state too; a passing notice takes no
// tap. ok is false when the face is off and no notice shows.
func (m Model) strip(w int) (lines []string, tap span, ok bool) {
	turn := span{0, w, m.keys.board.Strip.Help().Key}
	if m.noticeText != "" && m.now().Before(m.noticeUntil) {
		return m.noticeRows(m.noticeText, w), span{}, true
	}
	if m.copyCode != "" {
		return m.noticeRows(m.pasteHere(), w), span{}, true
	}
	if text := m.boardStateNotice(); text != "" {
		return m.noticeRows(text, w), turn, true
	}
	face := m.stripShows()
	if face == "off" {
		return nil, span{}, false
	}
	next := stripNext(face)
	if next == "off" {
		next = "hide"
	}
	word := m.st.faint.Render(next+" ›") + " "
	room := w - lipgloss.Width(word) - 2 // a cell at the left, one before the word
	left := ""
	switch {
	case !m.have: // before the first state, never a zero
	case face == "machine":
		left = m.stripMachine(room)
	default:
		left = m.stripUsage(room)
	}
	return []string{spread(" "+left, word, w)}, turn, true
}

// stripShows is the face the strip shows: a warning's face until the
// owner turns the strip, else the owner's.
func (m Model) stripShows() string {
	if warn := m.stripWarn(); warn != "" && !m.stripTurned {
		return warn
	}
	return m.stripFace()
}

// stripWarn is the face a warning turns the strip to: usage when a plan's
// window is within 10% of its limit, the machine while a detached busy
// loop burns ("": none).
func (m Model) stripWarn() string {
	for _, a := range m.stripAccounts() {
		if a.fullest >= 90 {
			return "usage"
		}
	}
	if len(m.all.orphans) > 0 {
		return "machine"
	}
	return ""
}

// turnStripShown is u on the board: the strip turns from the face it
// shows, and a warning no longer turns it while it lasts.
func (m *Model) turnStripShown() {
	if shown := m.stripShows(); shown != m.stripFace() {
		m.setStripFace(shown)
	}
	m.turnStrip()
	m.stripTurned = m.stripWarn() != ""
}

// stripFilterHost is the host the board's host filter names ("": all).
func (m Model) stripFilterHost() (string, bool) {
	if m.boardHost == "" {
		return "", false
	}
	for _, h := range m.hosts() {
		if m.nameOf(h) == m.boardHost {
			return h, true
		}
	}
	return "", false
}

// stripHosts are the hosts the strip shows: the one in the filter, else
// every host with a state, this machine first.
func (m Model) stripHosts() []string {
	if h, ok := m.stripFilterHost(); ok {
		return []string{h}
	}
	if hs := m.statedHosts(); len(hs) > 0 {
		return hs
	}
	return []string{m.opt.Host}
}

// stripAcct is an account the strip shows: its host, its name on that
// host, and its fullest window.
type stripAcct struct {
	host, name string
	a          limits.Account
	fullest    float64
}

// stripAccounts are the accounts of the strip's hosts, the fullest first.
func (m Model) stripAccounts() []stripAcct {
	now := m.now()
	var out []stripAcct
	for _, h := range m.stripHosts() {
		ls := m.limitsOf(h)
		names, _ := accountNames(ls, false)
		for i, a := range ls {
			sa := stripAcct{host: h, name: names[i], a: a}
			for _, u := range a.Uses(now) {
				sa.fullest = max(sa.fullest, u.Pct)
			}
			out = append(out, sa)
		}
	}
	slices.SortStableFunc(out, func(x, y stripAcct) int { return cmp.Compare(y.fullest, x.fullest) })
	return out
}

// stripJoin is the parts that fit whole in w cells, in order, three cells
// apart; all: every part fits.
func stripJoin(parts []string, w int) (line string, all bool) {
	for _, p := range parts {
		next := p
		if line != "" {
			next = line + "   " + p
		}
		if lipgloss.Width(next) > w {
			return line, false
		}
		line = next
	}
	return line, true
}

// stripShape is how much a part of the strip says: the names, a bar (or
// spark) for every window (or host) or the first only, the resets, the
// memory of a host shown alone.
type stripShape struct {
	names, bars, firstBar, resets, mem bool
}

// The shapes of usage and of the machine, from the most each says to the
// least.
var (
	usageShapes = []stripShape{
		{names: true, bars: true, resets: true},
		{names: true, firstBar: true},
		{firstBar: true},
		{},
	}
	machineShapes = []stripShape{
		{names: true, bars: true, mem: true},
		{names: true, mem: true},
		{mem: true},
		{},
	}
)

// stripFits is the parts that fit whole in w cells, in the first shape
// where the first need parts fit (need 0: every part); else in the last
// shape.
func stripFits(w, need int, shapes []stripShape, parts func(stripShape) []string) string {
	line := ""
	for _, sh := range shapes {
		ps := parts(sh)
		if need > 0 && need < len(ps) {
			if _, all := stripJoin(ps[:need], w); all {
				line, _ = stripJoin(ps, w)
				break
			}
		}
		var all bool
		if line, all = stripJoin(ps, w); all {
			break
		}
	}
	return line
}

// stripUsage is each account's windows on one line of w cells, the
// fullest first; an account of another host says its host. The richest
// shape is the one where every account at 60% or more fits; the
// accounts after them go when they do not fit whole.
func (m Model) stripUsage(w int) string {
	accounts := m.stripAccounts()
	if len(accounts) == 0 {
		return m.st.faint.Render("no session runs under a plan")
	}
	need := 0
	for need < len(accounts) && accounts[need].fullest >= 60 {
		need++
	}
	return stripFits(w, need, usageShapes, func(sh stripShape) []string {
		parts := make([]string, len(accounts))
		for i, a := range accounts {
			parts[i] = m.stripAccount(a, sh)
		}
		return parts
	})
}

// stripAccount is one account in the strip: "✻ Claude Max  5h ▰▰▰▱▱▱▱▱ 38%  wk 41%".
// A window at 60% or more is in warn, at 90% in neg.
func (m Model) stripAccount(sa stripAcct, sh stripShape) string {
	st, now, a := m.st, m.now(), sa.a
	head := m.accountMark(a.Provider) + " "
	if sh.names {
		head += st.text.Render(sa.name) + " "
	}
	if sa.host != m.opt.Host {
		head += st.muted.Render("@"+m.nameOf(sa.host)) + " "
	}
	if sh.names {
		head += " "
	}
	if a.State == limits.Unmeasured {
		return head + st.warn.Render("not measured")
	}
	uses := a.Uses(now)
	if len(uses) == 0 {
		return head + st.faint.Render("no window read")
	}
	sep := "  "
	switch {
	case sh.resets:
		sep = "   "
	case !sh.bars && !sh.firstBar:
		sep = st.faint.Render(" · ")
	}
	cells := make([]string, len(uses))
	for i, u := range uses {
		ink := st.text
		if u.Pct >= 60 {
			ink = m.useStyle(u.Pct)
		}
		cell := st.faint.Render(u.Name + " ")
		if sh.bars || sh.firstBar && i == 0 {
			cell += m.meter(u.Pct, 8, m.useStyle(u.Pct)) + " "
		}
		cell += ink.Render(fmt.Sprintf("%.0f%%", u.Pct))
		if sh.resets && !u.Resets.IsZero() {
			cell += st.faint.Render(" · " + limits.When(u.Resets, now, ""))
		}
		cells[i] = cell
	}
	return head + strings.Join(cells, sep)
}

// stripMachine is the machine on one line of w cells: the host in the
// filter with its memory, or each host's CPU; a detached busy loop leads
// in warn.
func (m Model) stripMachine(w int) string {
	hosts := m.stripHosts()
	var lead []string
	switch n := len(m.all.orphans); {
	case n == 1:
		lead = []string{m.st.warn.Render("a busy loop burns")}
	case n > 1:
		lead = []string{m.st.warn.Render(fmt.Sprintf("%d busy loops burn", n))}
	}
	return stripFits(w, 0, machineShapes, func(sh stripShape) []string {
		parts := slices.Clone(lead)
		for _, h := range hosts {
			parts = append(parts, m.stripHost(h, sh, len(hosts) == 1))
		}
		return parts
	})
}

// stripHost is one host in the strip: "ws-1  cpu ▃▄▅▄▃▅▆▅ 38% · mem 41 of 64 GB".
// Memory shows only for a host alone: with all hosts, each host's CPU.
func (m Model) stripHost(h string, sh stripShape, alone bool) string {
	st := m.st
	line := st.text.Render(m.nameOf(h)) + " "
	if note := m.staleNote(h); note != "" {
		return line + st.faint.Render(note)
	}
	mc := m.machineOf(h)
	if sh.names {
		line += " "
	}
	line += st.label.Render("cpu ")
	if sh.bars {
		line += st.muted.Render(spark(m.viewOf(h).history.cpu, 8, 100)) + " "
	}
	line += st.text.Render(fmt.Sprintf("%.0f%%", mc.CPU))
	if sh.mem && alone {
		used, _ := memUse(mc)
		line += st.faint.Render(" · ") + st.label.Render("mem ") + st.text.Render(fmt.Sprintf("%.0f of %.0f GB", gb(used), gb(mc.MemTotal)))
	}
	return line
}
