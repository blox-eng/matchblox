package app

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/accounts"
	"github.com/blox-eng/matchblox/internal/limits"
)

// The dock is usage, the machine and the stoker, between the body and the
// key line, when the body leaves room for it and one blank row
// (design/0006 §7). It is never scrolled: when it does not fit it folds
// away, and the Machine tab has it all. Rows the body leaves free beyond
// its minimum, it grows into: usage its forecasts and resets, the machine
// its top processes, the stoker its recent steps.

// The panels of the dock, in the order u cycles them on a phone.
const (
	panelUsage = iota
	panelMachine
	panelStoker
	panels
)

var panelWords = [panels]string{"usage ›", "machine ›", "stoker ›"}

// stokerClaims: a run is in charge, or has steps the builder has not
// seen; the stoker's panel takes the dock until the builder switches.
func (m Model) stokerClaims() bool {
	return m.stoker != nil && (m.stoker.On || m.stokedShows())
}

// dockShows is the panel a phone's dock shows.
func (m Model) dockShows() int {
	if m.stokerClaims() && !m.dockSwitched {
		return panelStoker
	}
	return m.dockPanel
}

// cycleDock is u: the next panel of a phone's dock.
func (m *Model) cycleDock() {
	m.dockPanel = (m.dockShows() + 1) % panels
	m.dockSwitched = m.stokerClaims()
}

// docked is the dock drawn: its lines, whether every panel shows all it
// has, and the columns of the stoker's panel (a tap there opens its card;
// x1 0: no stoker panel).
type docked struct {
	lines      []string
	full       bool
	stx0, stx1 int
}

// panel is one panel of the dock in w columns: its fewest lines, and all
// it has to say. A cut keeps the fewest lines and never cuts usage.
func (m Model) panel(p, w int, toggle string) (least, most []string) {
	switch p {
	case panelUsage:
		return m.usageLines(w, false, toggle), m.usageLines(w, true, toggle)
	case panelMachine:
		least = m.machineLines(w, false, toggle)
		return least, append(slices.Clone(least), m.topLines(w)...)
	}
	return m.stokerLines(w, false, toggle), m.stokerLines(w, true, toggle)
}

// dock is the dock in w columns and at most rows lines (rows < 0: its
// fewest). A phone shows one panel, the one u picks; an opened phone, an
// upright drawer and a desk short of room two, usage and the machine
// (the stoker in the machine's place while it claims the dock); a wide
// desk all three.
func (m Model) dock(w, rows int) docked {
	type col struct {
		p, w   int
		toggle string
	}
	var cols []col
	switch {
	case m.phoneDock(w):
		p := m.dockShows()
		cols = []col{{p, w, panelWords[(p+1)%panels]}}
	case w >= 152 && m.layoutAt(w) == Wide:
		third := (w - 2) / 3
		cols = []col{{panelUsage, third, ""}, {panelMachine, third, ""}, {panelStoker, w - 2 - 2*third, ""}}
	default:
		split := splitAt(w, m.layoutAt(w))
		right := panelMachine
		if m.stokerClaims() {
			right = panelStoker
		}
		cols = []col{{panelUsage, split, ""}, {right, w - split - 1, ""}}
	}
	d := docked{full: true}
	var parts [][]string
	x := 0
	for i, c := range cols {
		least, most := m.panel(c.p, c.w, c.toggle)
		lines := least
		switch {
		case rows < 0:
			d.full = d.full && len(most) <= len(least)
		case len(most) <= rows:
			lines = most
		case c.p != panelUsage:
			lines = most[:max(rows, len(least))]
			d.full = false
		default:
			d.full = false
		}
		if c.p == panelStoker {
			d.stx0, d.stx1 = x, x+c.w
		}
		x += c.w
		if i < len(cols)-1 {
			x++
		}
		parts = append(parts, lines)
	}
	n := 0
	for _, p := range parts {
		n = max(n, len(p))
	}
	for i := range n {
		line := ""
		for j, c := range cols {
			cell := ""
			if i < len(parts[j]) {
				cell = parts[j][i]
			}
			if j > 0 {
				line += m.st.hair.Render("│")
			}
			line += fit(cell, c.w)
		}
		d.lines = append(d.lines, line)
	}
	return d
}

// listAbove is the part of the list that fits in h lines of w columns.
// When rows are cut below, the window takes one line less and the line
// under it says how many: the hint never covers the selection.
func (m Model) listAbove(b body, w, h int) body {
	sel := m.rowIndex()
	if h <= 1 || len(b.lines) <= h {
		vis := windowed(b, h, sel)
		return body{lines: slices.Clone(vis.lines), rows: slices.Clone(vis.rows)}
	}
	start := windowStart(b, h-1, sel)
	below := map[int]bool{}
	for _, r := range b.rows[start+h-1:] {
		if r >= 0 {
			below[r] = true
		}
	}
	if len(below) == 0 { // the window is at the list's end: nothing to say
		vis := windowed(b, h, sel)
		return body{lines: slices.Clone(vis.lines), rows: slices.Clone(vis.rows)}
	}
	col := body{lines: slices.Clone(b.lines[start : start+h-1]), rows: slices.Clone(b.rows[start : start+h-1])}
	col.add(-1, fit(m.st.faint.Render(fmt.Sprintf(" ↓ %d more", len(below))), w))
	return col
}

// stokerPanelRow marks the stoker's panel in a desk's left column: a tap
// there opens its tab.
const stokerPanelRow = -4

// minListRows are the fewest rows of the list a desk's left column keeps
// over its panels, with the hint under them.
const minListRows = 3

// leftColumn is Sessions on a desk, h lines of w columns: the list, and
// under it usage, the machine and the stoker, each below a hairline. The
// stoker is there while it claims the dock, in the machine's place when
// the rows are short, or when the list leaves it room. The panels take
// their fewest lines and grow into the rows the list leaves free. When
// the list does not fit beside them, the list gives way: the panels stay
// while it keeps minListRows rows and the hint, then the machine folds,
// then all; the Machine tab has it all. A list cut short says how many
// rows are below it. All: the panels show all they have.
func (m Model) leftColumn(b body, w, h int) (col body, all bool) {
	least, most := map[int][]string{}, map[int][]string{}
	for p := range panels {
		least[p], most[p] = m.panel(p, w, "")
	}
	height := func(ps []int) int {
		n := 0
		for _, p := range ps {
			n += 1 + len(least[p])
		}
		return n
	}
	tall := len(b.lines)
	fits := func(ps []int) bool {
		n := height(ps)
		return tall+n <= h || h-n >= minListRows+1
	}
	every := []int{panelUsage, panelMachine, panelStoker}
	var tries [][]int
	switch {
	case m.stokerClaims():
		tries = [][]int{every, {panelUsage, panelStoker}}
	case tall+height(every) <= h:
		tries = [][]int{every}
	default:
		tries = [][]int{{panelUsage, panelMachine}, {panelUsage}}
	}
	var set []int
	for _, ps := range tries {
		if fits(ps) {
			set = ps
			break
		}
	}
	if set == nil {
		return m.listAbove(b, w, h), false
	}
	spare := max(h-len(b.lines)-height(set), 0)
	all = len(set) == len(every)
	var stats body
	for _, p := range set {
		lines, grow := least[p], len(most[p])-len(least[p])
		switch {
		case grow <= spare:
			lines, spare = most[p], spare-grow
		case p != panelUsage: // usage shows whole or at its fewest, never cut
			lines, spare = most[p][:len(least[p])+spare], 0
			all = false
		default:
			all = false
		}
		row := -1
		if p == panelStoker {
			row = stokerPanelRow
		}
		stats.add(row, m.st.hair.Render(strings.Repeat("─", w)))
		stats.add(row, lines...)
	}
	col = m.listAbove(b, w, h-len(stats.lines))
	for len(col.lines) < h-len(stats.lines) {
		col.add(-1, "")
	}
	col.lines, col.rows = append(col.lines, stats.lines...), append(col.rows, stats.rows...)
	return col, all
}

// dockToggle is where the phone dock's switch sits on its first line.
func (m Model) dockToggle(w int) (x0, x1 int) {
	word := panelWords[(m.dockShows()+1)%panels]
	return w - lipgloss.Width(word) - 1, w - 1
}

// usageLines are the limits of each account a session runs under, as
// /usage says them: each window's name, a bar (warn from 60%, neg from
// 90%), the percent and its reset; the forecast and an old reading on the
// account's last line. Short of room (a phone's dock) an account takes
// two lines, and one of another provider than Anthropic one.
func (m Model) usageLines(w int, full bool, toggle string) []string {
	st := m.st
	right := ""
	if toggle != "" {
		right = st.accent.Render(toggle) + " "
	}
	out := []string{spread(st.label.Render(" USAGE"), right, w)}
	if len(m.snap.Limits) == 0 {
		return append(out, fit(" "+st.faint.Render("no session runs under a plan"), w))
	}
	now := m.now()
	tight := !full && w < 50
	names, nameW := m.accountNames(tight)
	for i, a := range m.snap.Limits {
		head := " " + m.accountMark(a.Provider) + " " + st.text.Render(pad(names[i], nameW+1)) + " "
		indent := strings.Repeat(" ", lipgloss.Width(head))
		if a.State == limits.Unmeasured {
			out = append(out, fit(head+st.warn.Render("not measured · "+a.Fix), w))
			continue
		}
		uses := a.Uses(now)
		if len(uses) == 0 {
			out = append(out, fit(head+st.faint.Render("no window read"), w))
			continue
		}
		if tight && a.Provider != "anthropic" && len(uses) > 1 {
			line := head + st.faint.Render(uses[0].Name+" ") + m.meter(uses[0].Pct, 3, m.useStyle(uses[0].Pct)) +
				st.text.Render(fmt.Sprintf(" %.0f%%", uses[0].Pct))
			for _, u := range uses[1:] {
				line += st.faint.Render(" · "+u.Name+" ") + st.text.Render(fmt.Sprintf("%.0f%%", u.Pct))
			}
			out = append(out, fit(line, w))
			continue
		}
		if tight {
			uses = uses[:min(2, len(uses))]
		}
		// The meter gives way to the resets on a narrow dock, as small as the
		// folded line's.
		resets := make([]string, len(uses))
		meterW := 8
		for j, u := range uses {
			if !u.Resets.IsZero() {
				resets[j] = " · " + limits.When(u.Resets, now, "")
			}
			meterW = min(meterW, w-lipgloss.Width(head)-10-lipgloss.Width(resets[j]))
		}
		meterW = max(meterW, 3)
		var lines []string
		for j, u := range uses {
			lead := head
			if j > 0 {
				lead = indent
			}
			line := lead + st.faint.Render(pad(u.Name, 5)) + m.meter(u.Pct, meterW, m.useStyle(u.Pct)) + st.text.Render(pad(fmt.Sprintf(" %.0f%%", u.Pct), 5))
			if resets[j] != "" {
				line += st.faint.Render(resets[j])
			}
			lines = append(lines, line)
		}
		for _, p := range m.outlook(a) {
			last := lines[len(lines)-1]
			style := st.muted
			if p.Tone == limits.Warn {
				style = st.warn
			}
			switch more := last + st.faint.Render(" · ") + style.Render(p.Text); {
			case lipgloss.Width(more) <= w:
				lines[len(lines)-1] = more
			case !tight:
				lines = append(lines, indent+style.Render(p.Text))
			case p.Tone == limits.Warn:
				// A phone keeps the account on its lines: a warning takes the
				// place of the reset.
				cut, _, _ := strings.Cut(last, st.faint.Render(" · "))
				lines[len(lines)-1] = cut + st.faint.Render(" · ") + style.Render(p.Text)
			}
		}
		for _, l := range lines {
			out = append(out, fit(l, w))
		}
	}
	return out
}

// outlook is what an account's block ends with: the forecast of its week,
// and the time of a reading over an hour old.
func (m Model) outlook(a limits.Account) []limits.Part {
	var out []limits.Part
	for _, p := range limits.Parts(a, m.now()) {
		if p.Tone != "" {
			out = append(out, p)
		}
	}
	return out
}

func (m Model) useStyle(pct float64) lipgloss.Style {
	switch {
	case pct >= 90:
		return m.st.neg
	case pct >= 60:
		return m.st.warn
	}
	return m.st.muted
}

// providerMarks stand for the logos a terminal cannot draw: one cell each,
// none an emoji.
var providerMarks = map[string]string{"anthropic": "✻", "openai": "❋", "opencode": "▣", "cursor": "⬢"}

// providerNames name the provider of each account: the plans of two
// providers are often the same word ("Team"). OpenCode's accounts are
// under the provider its model runs on; another provider goes by its id.
var providerNames = map[string]string{"anthropic": "Claude", "openai": "OpenAI", "opencode": "OpenCode", "cursor": "Cursor", "google": "Google"}

// accountNames are the short names of the accounts, after their provider:
// one account of a provider by its plan, two by the name of their emails.
// A phone (tight) leaves out the plan.
func (m Model) accountNames(tight bool) ([]string, int) {
	per := map[string]int{}
	for _, a := range m.snap.Limits {
		if a.Dir == "" {
			per[a.Provider]++ // an account folder is named by its folder, not by its email
		}
	}
	names := make([]string, len(m.snap.Limits))
	nameW := 0
	for i, a := range m.snap.Limits {
		p := providerNames[a.Provider]
		if p == "" {
			p = a.Provider
		}
		email, plan, _ := strings.Cut(a.Account, " · ")
		name := plan
		switch {
		case a.Dir != "":
			name = accounts.Name(a.Dir)
		case per[a.Provider] > 1 || plan == "":
			name, _, _ = strings.Cut(email, "@")
		case tight:
			name = ""
		}
		names[i] = strings.TrimSpace(p + " " + name)
		nameW = max(nameW, lipgloss.Width(names[i]))
	}
	return names, nameW
}

// machineLines are the machine in a few lines: cpu with its last minute
// and the load, memory, disk, and the agents that run.
func (m Model) machineLines(w int, full bool, toggle string) []string {
	st, mc := m.st, m.snap.Machine
	host := m.opt.Host
	if host == "" {
		host = m.host.Host
	}
	about := ""
	if host != "" {
		about = host + " · "
	}
	about += plural(len(mc.Cores), "core")
	right := st.faint.Render(about + " ")
	if toggle != "" {
		right = st.accent.Render(toggle) + " "
	}
	sp := max(min(w-30, 24), 8)
	if full {
		sp = 24
	}
	used := mc.MemTotal - mc.MemAvail
	memPct := 0.0
	if mc.MemTotal > 0 {
		memPct = float64(used) / float64(mc.MemTotal) * 100
	}
	out := []string{
		spread(st.label.Render(" MACHINE"), right, w),
		" " + st.label.Render("cpu  ") + st.accent.Render(spark(m.history.cpu, sp, 100)) +
			st.text.Render(pad(fmt.Sprintf(" %.0f%%", mc.CPU), 6)) + st.label.Render("load ") + st.text.Render(fmt.Sprintf("%.1f", mc.Load[0])),
		" " + st.label.Render("mem  ") + m.meter(memPct, 8, st.muted) + st.text.Render(fmt.Sprintf(" %.0f of %.0f GB", gb(used), gb(mc.MemTotal))),
		" " + st.label.Render("disk ") + m.diskCell(),
		" " + st.label.Render("runs ") + st.text.Render(m.runs()),
	}
	for i := range out {
		out[i] = fit(out[i], w)
	}
	return out
}

func gb(b uint64) float64 { return float64(b) / (1 << 30) }

// diskCell is the use of the home's file system, "▰▰▰▰▰▱▱▱ 71% of 1 TB",
// and of / when it is another; the disk's traffic when none was read.
func (m Model) diskCell() string {
	st, mc := m.st, m.snap.Machine
	if len(mc.Disks) == 0 {
		return st.text.Render("r " + size(mc.DiskRead) + "/s · w " + size(mc.DiskWrite) + "/s")
	}
	d := mc.Disks[0]
	cell := m.meter(d.Pct(), 8, m.useStyle(d.Pct())) + st.text.Render(fmt.Sprintf(" %.0f%% of %s", d.Pct(), bytesWord(d.Total)))
	for _, o := range mc.Disks[1:] {
		cell += st.faint.Render(" · "+o.Path+" ") + m.useStyle(o.Pct()).Render(fmt.Sprintf("%.0f%%", o.Pct()))
	}
	return cell
}

// bytesWord is a size as a disk is named: 931 GB, 2 TB.
func bytesWord(b uint64) string {
	if tb := float64(b) / (1 << 40); tb >= 1 {
		return strings.TrimSuffix(fmt.Sprintf("%.1f", tb), ".0") + " TB"
	}
	return fmt.Sprintf("%.0f GB", gb(b))
}

// runs counts the agents by their command: "13 claude · 1 codex".
func (m Model) runs() string {
	n := map[string]int{}
	for _, s := range m.all.sessions {
		agent := s.Agent
		if agent == "" {
			agent = "agent"
		}
		n[agent]++
	}
	if len(n) == 0 {
		return "no agents"
	}
	var names []string
	for a := range n {
		names = append(names, a)
	}
	slices.SortFunc(names, func(a, b string) int {
		if n[a] != n[b] {
			return n[b] - n[a]
		}
		return strings.Compare(a, b)
	})
	parts := make([]string, len(names))
	for i, a := range names {
		parts[i] = fmt.Sprintf("%d %s", n[a], a)
	}
	return strings.Join(parts, " · ")
}

// machinePanel is the Machine tab: usage first, then the machine, then
// the detached busy loops and the top processes.
func (m Model) machinePanel(w int) body {
	var b body
	b.add(-1, m.usageLines(w, true, "")...)
	b.add(-1, "")
	b.add(-1, m.machineLines(w, true, "")...)
	b.add(-1, m.machine(w)...)
	p := m.procs(w)
	b.lines, b.rows = append(b.lines, p.lines...), append(b.rows, p.rows...)
	return b
}
