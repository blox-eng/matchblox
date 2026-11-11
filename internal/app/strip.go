package app

import (
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

// noticeMsg comes when a notice ends: the frame drops it.
type noticeMsg struct{}

// notice is the one way to show a toast, a refusal or the data's age on
// the board: text takes the strip's row for noticeFor. The header never
// shows it.
func (m *Model) notice(text string) tea.Cmd {
	m.noticeText, m.noticeUntil, m.goArmed = text, m.now().Add(noticeFor), paneKey{}
	return tea.Tick(noticeFor, func(time.Time) tea.Msg { return noticeMsg{} })
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

// stripNotice is what the strip's row says in place of the strip: a live
// notice, then the board's state ("": nothing).
func (m Model) stripNotice() string {
	if m.noticeText != "" && m.now().Before(m.noticeUntil) {
		return m.noticeText
	}
	return m.boardStateNotice()
}

// noticeRows are a notice in w columns: one line, two on Narrow when it
// does not fit, so its fix is never cut.
func (m Model) noticeRows(text string, w int) []string {
	lines := []string{text}
	if m.layoutAt(w) == Narrow && ansi.StringWidth(text) > w-1 {
		lines = upTo(cutNotice(text, w-1), 2, w-1)
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
// shows with the word of the next face at its right. tap covers the face
// and presses u. ok is false when the face is off and no notice shows.
func (m Model) strip(w int) (lines []string, tap span, ok bool) {
	if text := m.stripNotice(); text != "" {
		return m.noticeRows(text, w), span{}, true
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
	return []string{spread(" "+left, word, w)}, span{0, w, m.keys.board.Strip.Help().Key}, true
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
	now := m.now()
	for _, h := range m.stripUsageHosts() {
		for _, a := range m.limitsOf(h) {
			for _, u := range a.Uses(now) {
				if u.Pct >= 90 {
					return "usage"
				}
			}
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

// stripUsageHosts are the hosts whose plans the strip shows: the one in
// the filter, else this machine.
func (m Model) stripUsageHosts() []string {
	if h, ok := m.stripFilterHost(); ok {
		return []string{h}
	}
	return []string{m.opt.Host}
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
// memory of every host or the first only.
type stripShape struct {
	names, bars, firstBar, resets, mem, firstMem bool
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
		{names: true, firstBar: true, firstMem: true},
		{firstMem: true},
		{},
	}
)

// stripFits is the parts in the first shape where every part fits w
// cells; else the last shape's parts that fit whole.
func stripFits(w int, shapes []stripShape, parts func(stripShape) []string) string {
	line := ""
	for _, sh := range shapes {
		var all bool
		if line, all = stripJoin(parts(sh), w); all {
			break
		}
	}
	return line
}

// stripUsage is each account's windows on one line of w cells.
func (m Model) stripUsage(w int) string {
	var accounts []limits.Account
	for _, h := range m.stripUsageHosts() {
		accounts = append(accounts, m.limitsOf(h)...)
	}
	if len(accounts) == 0 {
		return m.st.faint.Render("no session runs under a plan")
	}
	names, _ := accountNames(accounts, false)
	return stripFits(w, usageShapes, func(sh stripShape) []string {
		parts := make([]string, len(accounts))
		for i, a := range accounts {
			parts[i] = m.stripAccount(a, names[i], sh)
		}
		return parts
	})
}

// stripAccount is one account in the strip: "✻ Claude Max  5h ▰▰▰▱▱▱▱▱ 38%  wk 41%".
// A window at 60% or more is in warn, at 90% in neg.
func (m Model) stripAccount(a limits.Account, name string, sh stripShape) string {
	st, now := m.st, m.now()
	head := m.accountMark(a.Provider) + " "
	if sh.names {
		head += st.text.Render(name) + "  "
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
// filter, or each host with a state; a detached busy loop leads in warn.
func (m Model) stripMachine(w int) string {
	hosts := m.statedHosts()
	if h, ok := m.stripFilterHost(); ok {
		hosts = []string{h}
	}
	if len(hosts) == 0 {
		hosts = []string{m.opt.Host}
	}
	var lead []string
	switch n := len(m.all.orphans); {
	case n == 1:
		lead = []string{m.st.warn.Render("a busy loop burns")}
	case n > 1:
		lead = []string{m.st.warn.Render(fmt.Sprintf("%d busy loops burn", n))}
	}
	return stripFits(w, machineShapes, func(sh stripShape) []string {
		parts := slices.Clone(lead)
		for i, h := range hosts {
			parts = append(parts, m.stripHost(h, sh, i == 0))
		}
		return parts
	})
}

// stripHost is one host in the strip: "ws-1  cpu ▃▄▅▄▃▅▆▅ 38% · mem 41 of 64 GB".
func (m Model) stripHost(h string, sh stripShape, first bool) string {
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
	if sh.bars || sh.firstBar && first {
		line += st.muted.Render(spark(m.viewOf(h).history.cpu, 8, 100)) + " "
	}
	line += st.text.Render(fmt.Sprintf("%.0f%%", mc.CPU))
	if sh.mem || sh.firstMem && first {
		used, _ := memUse(mc)
		line += st.faint.Render(" · ") + st.label.Render("mem ") + st.text.Render(fmt.Sprintf("%.0f of %.0f GB", gb(used), gb(mc.MemTotal)))
	}
	return line
}
