package app

import (
	"fmt"
	"maps"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
)

// openDoors are the doors the console shows: done and closed ones fold. A
// console of another host shows no hosts door: hosts are picked on the
// machine the builder connects from.
func openDoors(ds []doors.Door, remote bool) []doors.Door {
	var out []doors.Door
	for _, d := range ds {
		if d.Open() && !(remote && d.ID == doors.Hosts) {
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
	if d, ok := m.previewDoor(); ok {
		m.preview(b, d, w)
	}
}

// previewDoor is the door whose diff waits for a y.
func (m Model) previewDoor() (doors.Door, bool) {
	if m.pending == nil || m.pending.door == "" || m.pending.which == "secondary" {
		return doors.Door{}, false
	}
	if d, ok := m.selectedDoor(); ok && d.ID == m.pending.door {
		return d, true
	}
	return doors.Door{}, false
}

func (m Model) previewLines(d doors.Door) []string {
	if d.ID == doors.Hosts {
		return m.hostLines(d)
	}
	if d.Term != nil && m.pending != nil {
		return []string{shellLine(m.pending.steps[0])} // the exact argv this console runs
	}
	return strings.Split(d.Preview, "\n")
}

// previewRoom is how many lines of the diff fit under the doors.
func (m Model) previewRoom() int {
	var rows body
	mm := m
	mm.pending = nil
	mm.doorsSection(&rows, mm.width)
	return max(3, m.height-chrome-len(rows.lines)-3)
}

// previewSeen: the last line of the diff was on the screen. A door opens
// only on what the person saw, so y waits until then.
func (m Model) previewSeen() bool {
	d, ok := m.previewDoor()
	return !ok || m.previewTop+m.previewRoom() >= len(m.previewLines(d))
}

// scrollPreview moves a long diff with ↑↓; other keys go on to the confirm.
func (m Model) scrollPreview(k string) (Model, bool) {
	d, ok := m.previewDoor()
	if !ok {
		return m, false
	}
	last := max(0, len(m.previewLines(d))-m.previewRoom())
	if d.ID == doors.Hosts {
		return m.pickHost(d, k, last)
	}
	switch k {
	case "down", "j":
		m.previewTop = min(m.previewTop+1, last)
	case "up", "k":
		m.previewTop = max(m.previewTop-1, 0)
	case "y":
		return m, !m.previewSeen() // y waits for the end of the diff
	default:
		return m, false
	}
	return m, true
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
	lines := m.previewLines(d)
	top := min(m.previewTop, max(0, len(lines)-1))
	end := min(top+m.previewRoom(), len(lines))
	cut := len(lines) - end
	lines = lines[top:end]
	for i, l := range lines {
		lead := " "
		if d.ID == doors.Hosts && top+i == hostsHead+m.hostAt {
			lead = st.accent.Render("▌")
		}
		style := st.faint
		switch {
		case strings.HasPrefix(l, "+"):
			style = st.accent
		case strings.HasPrefix(l, "-"):
			style = st.neg
		case d.Path == "":
			style = st.text // a command
		}
		b.add(-1, fit(lead+style.Render(tilde(l)), w))
	}
	if cut > 0 {
		b.add(-1, fit(" "+st.faint.Render(fmt.Sprintf("… %d more lines: ↓ the rest of the diff, then y", cut)), w))
	}
}

func plainText(s ...string) string { return strings.Join(s, "") }

// shellLine is argv as the person would type it.
func shellLine(argv []string) string { return remote.Line(argv) }

// hostsHead is how many lines of the hosts block come before the first host.
const hostsHead = 3

// hostLines is the block the hosts door appends, with every host of
// ~/.ssh/config in it: a picked one is a new line ("+"), the others are
// not written.
func (m Model) hostLines(d doors.Door) []string {
	block := strings.Split(strings.TrimSuffix(doors.HostsBlock(d.Choices), "\n"), "\n")
	out := make([]string, 0, len(block))
	for i, l := range block {
		h := i - hostsHead
		if h >= 0 && h < len(d.Choices) && !m.hostPicks[d.Choices[h]] {
			out = append(out, " "+l)
			continue
		}
		out = append(out, "+"+l)
	}
	return out
}

// pickHost moves the cursor of the hosts door and picks with space. The
// list scrolls with the cursor; y waits for its last line, as for a diff.
func (m Model) pickHost(d doors.Door, k string, last int) (Model, bool) {
	switch k {
	case "down", "j":
		m.hostAt = min(m.hostAt+1, len(d.Choices)-1)
	case "up", "k":
		m.hostAt = max(m.hostAt-1, 0)
	case "space", " ":
		h := d.Choices[m.hostAt]
		picks := maps.Clone(m.hostPicks)
		if picks == nil {
			picks = map[string]bool{}
		}
		picks[h] = !picks[h]
		m.hostPicks = picks
		return m, true
	case "y":
		return m, !m.previewSeen()
	default:
		return m, false
	}
	// Keep the cursor on the screen; past the last host, show the end.
	line, room := hostsHead+m.hostAt, m.previewRoom()
	switch {
	case m.hostAt == len(d.Choices)-1:
		m.previewTop = last
	case line < m.previewTop:
		m.previewTop = line
	case line >= m.previewTop+room:
		m.previewTop = line - room + 1
	}
	return m, true
}

// pickedHosts are the picked hosts of the selected hosts door, in its order.
func (m Model) pickedHosts() []string {
	var out []string
	if d, ok := m.selectedDoor(); ok && d.ID == doors.Hosts {
		for _, h := range d.Choices {
			if m.hostPicks[h] {
				out = append(out, h)
			}
		}
	}
	return out
}
