package app

import (
	"fmt"
	"slices"
	"strings"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
)

// cogSheet is Machine open over the list. Nil while it is closed.
// part is the section a chip asked for: "" is the top, as `,` opens it.
type cogSheet struct {
	part string
}

// cogSheetKey is a key while Machine is open over the list.
// esc and ctrl+q close it. The list's row stays where it was.
func (m Model) cogSheetKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.cogSheet == nil {
		return m, nil
	}
	if bkey.Matches(k, m.keys.side.Flip) || bkey.Matches(k, m.keys.main.Back) {
		m.cogSheet = nil
	}
	return m, nil
}

// paintCogSheet draws today's Machine over the list body. The header stays.
// Rows the sheet does not cover stay, dimmed.
func (m Model) paintCogSheet(sc *drawn, w int) {
	if m.cogSheet == nil {
		return
	}
	lines, at := m.cogLines(w)
	top := sc.bodyTop
	if top < 0 || top > len(sc.lines) {
		return
	}
	for i := top; i < len(sc.lines); i++ {
		sc.lines[i] = m.st.faint.Render(ansi.Strip(sc.lines[i]))
	}
	room := len(sc.lines) - top
	for i, line := range cogWindow(lines, room, at) {
		if top+i >= len(sc.lines) {
			break
		}
		sc.lines[top+i] = fit(line, w)
	}
}

// cogLines are today's Machine, from dock.go, and the line a chip asked
// to put at the top. A peer's busy loop is drawn with the same procs
// Machine uses for the home host: the Machine panel names that host and
// does not repeat its loops.
func (m Model) cogLines(w int) (lines []string, at int) {
	b := m.machinePanel(w)
	lines = append([]string(nil), b.lines...)
	if m.cogSheet == nil {
		return lines, 0
	}
	part := m.cogSheet.part
	switch {
	case strings.HasPrefix(part, "loop:"):
		at = m.cogShowLoop(&lines, w, strings.TrimPrefix(part, "loop:"), b.heads)
	case part == "usage":
		at = cogFind(lines, "USAGE")
	case part == "stoker":
		at = cogFind(lines, "stoker")
	case strings.HasPrefix(part, "host:"):
		if i, ok := b.heads[strings.TrimPrefix(part, "host:")]; ok {
			at = i
		}
	}
	return lines, at
}

// cogShowLoop scrolls to host's detached busy loop. When Machine's panel
// does not list it, the loop's lines go under that host's head.
func (m Model) cogShowLoop(lines *[]string, w int, host string, heads map[string]int) int {
	os := m.orphansOf(host)
	if len(os) == 0 {
		if i, ok := heads[host]; ok {
			return i
		}
		return 0
	}
	needle := fmt.Sprintf("%d %s", os[0].PID, os[0].Comm)
	if i := cogIndex(*lines, needle); i >= 0 {
		return cogRegionAt(*lines, i)
	}
	extra := m.cogLoopLines(host, w)
	if len(extra) == 0 {
		if i, ok := heads[host]; ok {
			return i
		}
		return 0
	}
	at := len(*lines)
	if i, ok := heads[host]; ok {
		at = i + 1
	}
	*lines = slices.Insert(*lines, at, extra...)
	if i := cogIndex((*lines)[at:], needle); i >= 0 {
		return cogRegionAt(*lines, at+i)
	}
	return at
}

// cogLoopLines are host's detached busy loops, drawn as Machine draws them,
// and nothing past that region.
func (m Model) cogLoopLines(host string, w int) []string {
	p := m.viewOf(host).procs(w)
	var out []string
	for _, l := range p.lines {
		if strings.Contains(strings.ToUpper(ansi.Strip(l)), "TOP PROCESSES") {
			break
		}
		out = append(out, l)
	}
	return out
}

func cogFind(lines []string, sub string) int {
	if i := cogIndex(lines, sub); i >= 0 {
		return i
	}
	return 0
}

func cogIndex(lines []string, sub string) int {
	for i, l := range lines {
		if strings.Contains(ansi.Strip(l), sub) {
			return i
		}
	}
	return -1
}

// cogRegionAt is the detached-busy-loops head above the orphan line i.
func cogRegionAt(lines []string, i int) int {
	for j := i; j >= 0; j-- {
		if strings.Contains(strings.ToUpper(ansi.Strip(lines[j])), "DETACHED BUSY LOOPS") {
			return j
		}
	}
	return i
}

func cogWindow(lines []string, room, at int) []string {
	if room <= 0 || len(lines) == 0 {
		return nil
	}
	if len(lines) <= room {
		return lines
	}
	if at < 0 {
		at = 0
	}
	if at > len(lines)-room {
		at = len(lines) - room
	}
	return lines[at : at+room]
}

func writeCog(run func(args ...string) (string, error), console, act string) {
	if act == "" {
		_, _ = run("set-option", "-wu", "-t", console, sample.CogOption)
		return
	}
	_, _ = run("set-option", "-w", "-t", console, sample.CogOption, act)
}

func (m Model) readCog() (string, bool) {
	run := m.tmuxRun()
	if run == nil {
		return "", false
	}
	out, err := run("show-option", "-wqv", "-t", m.opt.Pane, sample.CogOption)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(out), true
}

func (m Model) writeCogAct(act string) tea.Cmd {
	run := m.tmuxRun()
	if run == nil || act == "" {
		return nil
	}
	console := m.opt.Pane
	return func() tea.Msg {
		writeCog(run, console, act)
		return nil
	}
}

func (m Model) clearCogAct() tea.Cmd {
	run := m.tmuxRun()
	if run == nil {
		return nil
	}
	console := m.opt.Pane
	return func() tea.Msg {
		writeCog(run, console, "")
		return nil
	}
}

// applyCogOption opens Machine from a header or bar tap, then clears the
// one-shot act. A keys sheet that is already open stays the only overlay.
func (m *Model) applyCogOption() tea.Cmd {
	if !m.deskFrame || m.frameKind != "" {
		return nil
	}
	got, ok := m.readCog()
	if !ok || got == "" {
		return nil
	}
	return m.takeCog(got)
}

// takeCog opens Machine on act. "open" is the top, the same place `,` uses.
// The act is cleared either way, so a second poll does not open it again.
func (m *Model) takeCog(act string) tea.Cmd {
	clear := m.clearCogAct()
	if act == "" || m.keySheet != nil {
		return clear
	}
	if act == "open" {
		act = ""
	}
	m.cogSheet = &cogSheet{part: act}
	return tea.Batch(m.syncDeskMode(), clear)
}
