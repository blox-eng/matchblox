package app

import (
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
)

// openDoors are the doors the console shows: done and closed ones fold.
func openDoors(ds []doors.Door) []doors.Door {
	var out []doors.Door
	for _, d := range ds {
		if d.Open() {
			out = append(out, d)
		}
	}
	return out
}

func (m Model) selectedDoor() (doors.Door, bool) {
	if m.tab != tabQueue {
		return doors.Door{}, false
	}
	if i := m.queueIndex(); i < len(m.doors) {
		return m.doors[i], true
	}
	return doors.Door{}, false
}

// doorAction is what Enter does on a door: write its file, or run its
// command in this terminal. Both wait for a typed y.
func (m Model) doorAction(d doors.Door) *action {
	if d.Problem != "" {
		return nil
	}
	if d.Term != nil {
		argv, where := d.Term, "in this terminal"
		if d.ID == doors.Guide && !m.opt.OutsideTmux {
			argv, where = doors.GuideArgv(true), "in a new tmux window"
		}
		return &action{label: d.Title, say: d.Title + ": " + where, steps: [][]string{argv}, destructive: true, term: true, door: d.ID}
	}
	if d.Sum == "" {
		return nil
	}
	return &action{label: d.Title, say: d.Title + ": write " + tilde(d.Path), destructive: true,
		rec: "door:" + d.ID, which: "primary", text: d.Sum, door: d.ID}
}

func closeDoor(d doors.Door) *action {
	return &action{label: "close", say: "close " + d.Title + " (matchblox setup opens it again)", destructive: true,
		rec: "door:" + d.ID, which: "secondary", door: d.ID}
}

// doorRanMsg: a door's command gave the terminal back.
type doorRanMsg struct {
	door, cmd string
	err       error
}

// runDoor gives the terminal to a door's command: the password prompt of
// a package manager, or an agent, belongs to the person.
func (m Model) runDoor(a action) (tea.Model, tea.Cmd) {
	argv := a.steps[0]
	if !doors.TermAllowed(argv) {
		m.flash = "refused: a door runs only the tmux install or the guide"
		return m, nil
	}
	cmd := strings.Join(argv, " ")
	m.flash = "running: " + cmd
	return m, tea.ExecProcess(exec.Command(argv[0], argv[1:]...), func(err error) tea.Msg { //nolint:gosec // argv checked by TermAllowed
		return doorRanMsg{door: a.door, cmd: cmd, err: err}
	})
}

func (m Model) doorRan(msg doorRanMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		m.flash = fmt.Sprintf("failed: %s: %v", msg.cmd, msg.err)
		return m, nil
	case msg.door == doors.Tmux:
		m.flash = "tmux is installed: q, then matchblox, opens the console in tmux"
	case msg.door == doors.Guide:
		m.flash = "the guide runs in its window"
		if m.conn != nil && !m.lost {
			// The guide is opened once; matchblox setup offers it again.
			return m, m.send(proto.Act{RecID: "door:" + doors.Guide, Which: "secondary", Confirm: "y"})
		}
	}
	return m, nil
}

// doorsSection is the doors above the queue: one row each, and under the
// door that waits for its y, the exact diff or command.
func (m Model) doorsSection(b *body, w int) {
	if len(m.doors) == 0 {
		return
	}
	st := m.st
	b.add(-1, st.label.Render(fit(fmt.Sprintf(" SET UP · %d", len(m.doors)), w)))
	sel := m.queueIndex()
	narrow := layout(w) == Narrow
	for i, d := range m.doors {
		mark := "  "
		if i == sel {
			mark = "▌ "
		}
		if narrow {
			// A phone: one line for each door, the why of the selected one,
			// so the queue stays on the first screen.
			b.addRow(i, i == sel, w, st, " "+st.accent.Render(mark)+st.text.Render(d.Title))
			if i != sel {
				continue
			}
			for _, l := range wrap(d.Why, w-2, st.muted.Render) {
				b.addRow(i, i == sel, w, st, "  "+l)
			}
			if d.Problem != "" {
				for _, l := range wrap(d.Problem, w-2, st.warn.Render) {
					b.addRow(i, i == sel, w, st, "  "+l)
				}
			}
			continue
		}
		// The why wraps beside the title: "uses your tokens" must be read
		// before the y.
		text, style := d.Why, st.muted
		if d.Problem != "" {
			text, style = d.Problem, st.warn
		}
		for j, l := range wrap(text, w-colDoor+1, plainText) {
			lead := " " + st.accent.Render(mark) + st.text.Render(pad(d.Title, colDoor-3))
			if j > 0 {
				lead = strings.Repeat(" ", colDoor)
			}
			b.addRow(i, i == sel, w, st, lead+style.Render(strings.TrimPrefix(l, " ")))
		}
	}
	if m.pending != nil && m.pending.door != "" && m.pending.which != "secondary" {
		if d, ok := m.selectedDoor(); ok && d.ID == m.pending.door {
			m.preview(b, d, w)
		}
	}
}

// colDoor is the title column of a door row.
const colDoor = 26

// preview draws a door's diff: new lines in the accent, removed ones in
// neg. What does not fit says where to see it all.
func (m Model) preview(b *body, d doors.Door, w int) {
	st := m.st
	if d.Path != "" {
		b.add(-1, fit(" "+st.muted.Render(tilde(d.Path)), w))
	}
	lines := strings.Split(d.Preview, "\n")
	if d.Term != nil && m.pending != nil {
		lines = []string{shellLine(m.pending.steps[0])} // the exact argv this console runs
	}
	room := max(4, m.height-chrome-len(b.lines)-8)
	cut := 0
	if len(lines) > room {
		lines, cut = lines[:room-1], len(lines)-room+1
	}
	for _, l := range lines {
		style := st.faint
		switch {
		case strings.HasPrefix(l, "+"):
			style = st.accent
		case strings.HasPrefix(l, "-"):
			style = st.neg
		case d.Path == "":
			style = st.text // a command
		}
		b.add(-1, fit(" "+style.Render(tilde(l)), w))
	}
	if cut > 0 {
		b.add(-1, fit(" "+st.faint.Render(fmt.Sprintf("… %d more lines: matchblox setup shows them all", cut)), w))
	}
}

func plainText(s ...string) string { return strings.Join(s, "") }

// shellLine is argv as a shell would read it: a word with a space or a
// quote gets single quotes.
func shellLine(argv []string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = a
		if a == "" || strings.ContainsAny(a, " \t'\"\\$`;&|<>()*?#~") {
			q[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
	}
	return strings.Join(q, " ")
}
