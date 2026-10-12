package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/blox-eng/matchblox/internal/sample"
)

// Layout is the shape of the console for a terminal width (DESIGN.md §6).
type Layout int

const (
	// Narrow is a phone: one column, the selected row opens in place.
	Narrow Layout = iota
	// Medium is a phone opened up: the list and the detail split at the
	// middle.
	Medium
	// Wide is a desk: the list with every column, the detail beside it.
	Wide
)

func layout(width int) Layout {
	switch {
	case width < 60:
		return Narrow
	case width < 100:
		return Medium
	}
	return Wide
}

// body is a panel: its lines, and for each line the row it shows (-1 for
// none), so a tap knows what it hit.
type body struct {
	lines []string
	rows  []int
}

func (b *body) add(row int, lines ...string) {
	for _, l := range lines {
		b.lines = append(b.lines, l)
		b.rows = append(b.rows, row)
	}
}

// addRow adds the lines of one row, fitted to w, on the selected wash when
// it is the selected row.
func (b *body) addRow(row int, selected bool, w int, st styles, lines ...string) {
	for _, l := range lines {
		l = fit(l, w)
		if selected {
			l = st.selected.Render(l)
		}
		b.add(row, l)
	}
}

func plain(lines []string) body {
	var b body
	b.add(-1, lines...)
	return b
}

const (
	colPane = 11 // session:window.pane
	colName = 17
)

// minWidth is the narrowest console drawn; a narrower terminal cuts it.
const minWidth = 30

// progressCell is the bar an agent reported, in n cells and its percent.
func (m Model) progressCell(p *sample.Progress, n int) string {
	if p == nil {
		return ""
	}
	full := min(max((p.Pct*n+50)/100, 0), n)
	return m.st.text.Render(strings.Repeat("▰", full)) + m.st.faint.Render(strings.Repeat("▱", n-full)) +
		" " + m.st.muted.Render(fmt.Sprintf("%d%%", p.Pct))
}

// sessionIn is the agent session of a pane.
func (m Model) sessionIn(pane string) (sample.Session, bool) {
	if pane == "" {
		return sample.Session{}, false // outside tmux: no pane tells sessions apart
	}
	for _, s := range m.all.sessions {
		if s.Pane == pane {
			return s, true
		}
	}
	return sample.Session{}, false
}

// exitedLines are the agents that exited and wait for their parent: one
// faint line, or, after e, each parent with the fix and the pids.
func (m Model) exitedLines(w int) []string {
	ex := m.snap.Exited
	if len(ex) == 0 {
		return nil
	}
	st := m.st
	if !m.showExited {
		line := fmt.Sprintf(" %d exited agents · e shows them", len(ex))
		if len(ex) == 1 {
			line = " 1 exited agent · e shows it"
		}
		return []string{"", st.faint.Render(fit(line, w))}
	}
	lines := []string{"", st.label.Render(fmt.Sprintf(" %d EXITED AGENTS", len(ex))) + st.faint.Render("  · e hides them")}
	type parent struct {
		pid  int
		comm string
	}
	var order []parent
	pids := map[parent][]string{}
	for _, e := range ex {
		p := parent{e.Parent, e.ParentComm}
		if _, ok := pids[p]; !ok {
			order = append(order, p)
		}
		pids[p] = append(pids[p], strconv.Itoa(e.PID))
	}
	for _, p := range order {
		fix := "they go when it waits for them or exits"
		switch {
		case p.comm == "systemd" && p.pid == 1:
			fix = "sudo systemctl daemon-reexec reaps them" // the system manager
		case p.comm == "systemd":
			fix = "systemctl --user daemon-reexec reaps them"
		}
		lines = append(lines,
			fit(" "+st.text.Render(fmt.Sprintf("%s (pid %d) has not reaped them", p.comm, p.pid)), w),
			fit("   "+st.muted.Render(fix), w),
			fit("   "+st.faint.Render("pids "+strings.Join(pids[p], ", ")), w))
	}
	return lines
}

// busyWord is the state of a busy session: a turn runs, or only its
// subagents work and their results start its next turn.
func busyWord(s sample.Session) string {
	if s.Status == sample.StatusAgents {
		return "agents"
	}
	return "busy"
}
