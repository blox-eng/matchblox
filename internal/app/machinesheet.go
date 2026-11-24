package app

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The ⚙ sheet: on the desk, Machine draws as a sheet over the agent and
// keeps its own keys (design review D14; the owner, 2026-11-24). esc
// steps back a level, and ctrl+q leaves every sheet for the list.

// openCog opens Machine on the desk, on the part a tap names: cog:usage,
// cog:health, or cog:loops[:host], whose first loop it chooses when the
// loops are this host's.
func (m Model) openCog(part string) Model {
	m.setTab(tabMachine)
	m.agent.keys = false // the sheet has the keys
	if part == "cog:loops" {
		if i := slices.IndexFunc(m.hosts(), func(h string) bool { return m.inView(h) && len(m.orphansOn(h)) > 0 }); i >= 0 {
			part += ":" + m.hosts()[i]
		}
	}
	if part == "cog:loops:"+m.opt.Host && len(m.snap.Orphans) > 0 {
		m.machinePick(len(machineViews))
	}
	m.cogAt = part
	return m
}

// inSheets: a sheet is open, or a view of Machine is, which ctrl+q leaves.
func (m Model) inSheets() bool {
	_, view := m.openedView()
	return m.deskShape() && (m.over != "" || m.tab == tabMachine || view || m.standIn)
}

// leaveSheets closes every sheet and what one opened, in one press,
// whatever they show, and gives the keys to the list.
func (m Model) leaveSheets() Model {
	if m.standIn {
		m = m.leaveStandIn()
	}
	m.over, m.cogAt, m.pending, m.sheet, m.form, m.menu = "", "", nil, nil, nil, nil
	if m.tab != tabSessions {
		m.setTab(tabSessions)
	}
	m.agent.keys = false
	m.toList()
	return m
}

// cogTitle names the sheet and the hosts in view, then the view open in
// it.
func (m Model) cogTitle() string {
	title := "⚙ machine"
	for _, h := range m.hosts() {
		if m.inView(h) {
			title += " · " + m.nameOf(h)
		}
	}
	if v, ok := m.openedView(); ok {
		title += " › " + v.name
	}
	return title
}

// cogWindow is Machine, or the view open in it, in w columns and h lines,
// with the row each line shows: from the part it opened on, then as the
// chosen row keeps in view.
func (m Model) cogWindow(w, h int) ([]string, []int) {
	b := m.body(w)
	start := windowStart(b, h, m.rowIndex())
	if m.cogAt != "" && m.tab == tabMachine {
		start = m.cogLine(b.lines, w)
	}
	end := min(start+h, len(b.lines))
	return b.lines[start:end], b.rows[start:end]
}

// cogLine is the line of lines where the part the sheet opened on starts:
// the usage follows the views, the machine follows the usage and a blank
// line, and a host's busy loops start at their heading in its section.
func (m Model) cogLine(lines []string, w int) int {
	usage := len(m.viewsBody(w).lines)
	text := func(l string) string { return strings.TrimSpace(ansi.Strip(l)) }
	switch part := m.cogAt; {
	case part == "cog:usage":
		return usage
	case part == "cog:health":
		return usage + len(m.usageLines(w, true)) + 1
	case strings.HasPrefix(part, "cog:loops"):
		from := 0
		if host, ok := strings.CutPrefix(part, "cog:loops:"); ok && m.hostCount() > 1 {
			from = max(slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(text(l), m.nameOf(host)) }), 0)
		}
		if i := slices.IndexFunc(lines[from:], func(l string) bool { return text(l) == "DETACHED BUSY LOOPS" }); i >= 0 {
			return from + i
		}
	}
	return 0
}

// cogFoot is Machine's keys for the chosen row, then the way back to the
// agent, in w columns.
func (m Model) cogFoot(w int) string {
	st, sep := m.st, m.st.faint.Render(" · ")
	back := "closes"
	if name := m.keysGoTo(); name != "" {
		back = "back to " + name
	}
	if _, ok := m.openedView(); ok {
		back = "machine"
	}
	hints := slices.DeleteFunc(append([]hint{hintFor(m.keys.main.Up, "")}, m.view().keys(m)...), func(h hint) bool { return h == (hint{}) })
	esc := st.text.Render(m.keys.main.Back.Help().Key) + st.muted.Render(" "+back)
	for n := len(hints); ; n-- { // the way back stays whole: the last keys go first
		parts := make([]string, 0, n+1)
		for _, h := range hints[:n] {
			parts = append(parts, st.text.Render(h.key)+st.muted.Render(strings.TrimRight(" "+h.label, " ")))
		}
		if line := strings.Join(append(parts, esc), sep); n == 0 || lipgloss.Width(line) <= w {
			return line
		}
	}
}
