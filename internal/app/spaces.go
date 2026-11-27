package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
)

const (
	spaceAll  = "all"
	spaceMore = "space:more"
	spacePref = "space:"
)

// spaceMenu is the overflow list of spaces that did not fit, open over the
// list. The header stays one row.
type spaceMenu struct {
	names []string
	at    int
}

func (m Model) spaceRepos() []string {
	seen := map[string]bool{}
	var names []string
	for _, s := range m.sideSessions() {
		if !m.inView(s.Host) {
			continue
		}
		repo := m.repoOf(s)
		if seen[repo] {
			continue
		}
		seen[repo] = true
		names = append(names, repo)
	}
	slices.Sort(names)
	return names
}

func (m Model) spaceStops() []string {
	names := m.spaceRepos()
	if len(names) <= 1 {
		return nil
	}
	return append([]string{""}, names...)
}

func (m Model) spaceNeed(repo string) int {
	n := 0
	for _, s := range m.sideSessions() {
		if m.repoOf(s) == repo && m.inView(s.Host) && m.sideStateOf(s) == sideNeed && !m.stale(s.Host) {
			n++
		}
	}
	return n
}

func (m Model) inSpace(s sample.Session) bool {
	return m.space == "" || m.repoOf(s) == m.space
}

func spaceLabel(name string, need int) string {
	if name == "" {
		return spaceAll
	}
	if need == 0 {
		return name
	}
	return name + " ●" + strconv.Itoa(need)
}

func (m Model) spaceBudget(w int) int {
	right := ansi.StringWidth(fmt.Sprintf("● %d need you · ✓ %d done  ⚙", m.needCount(), m.doneCount()))
	return max(w-ansi.StringWidth(" ▰ matchblox  ")-right-2, 8)
}

func (m Model) fitSpaces(w int) (text string, hits []span, hidden []string) {
	names := m.spaceRepos()
	if len(names) <= 1 {
		return "", nil, nil
	}
	labels := []string{spaceLabel("", 0)}
	press := []string{spacePref}
	for _, name := range names {
		labels = append(labels, spaceLabel(name, m.spaceNeed(name)))
		press = append(press, spacePref+name)
	}
	join := func(n int) string { return strings.Join(labels[:n], " · ") }
	hitsOf := func(n int) []span {
		var out []span
		x := 0
		for i := 0; i < n; i++ {
			if i > 0 {
				x += ansi.StringWidth(" · ")
			}
			out = append(out, span{x, x + ansi.StringWidth(labels[i]), press[i]})
			x += ansi.StringWidth(labels[i])
		}
		return out
	}
	if ansi.StringWidth(join(len(labels))) <= w {
		return join(len(labels)), hitsOf(len(labels)), nil
	}
	for keep := len(labels) - 1; keep >= 1; keep-- {
		n := len(labels) - keep
		more := "+" + strconv.Itoa(n) + " ▾"
		text = join(keep) + " · " + more
		if ansi.StringWidth(text) > w {
			continue
		}
		hits = hitsOf(keep)
		x := ansi.StringWidth(join(keep) + " · ")
		hits = append(hits, span{x, x + ansi.StringWidth(more), spaceMore})
		return text, hits, names[keep-1:]
	}
	more := "+" + strconv.Itoa(len(names)) + " ▾"
	return more, []span{{0, ansi.StringWidth(more), spaceMore}}, names
}

func (m Model) spaceSpans(head string, w int) []span {
	text, hits, _ := m.fitSpaces(m.spaceBudget(w))
	if text == "" {
		return nil
	}
	plain := ansi.Strip(head)
	at := strings.Index(plain, ansi.Strip(text))
	if at < 0 {
		return nil
	}
	x := ansi.StringWidth(plain[:at])
	for i := range hits {
		hits[i].x0 += x
		hits[i].x1 += x
	}
	return hits
}

func writeSpace(run func(args ...string) (string, error), console, space string) {
	if space == "" {
		_, _ = run("set-option", "-wu", "-t", console, sample.SpaceOption)
		return
	}
	_, _ = run("set-option", "-w", "-t", console, sample.SpaceOption, space)
}

func (m *Model) applySpaceOption() {
	if !m.deskFrame || m.frameKind != "" {
		return
	}
	got, ok := m.readSpace()
	if !ok {
		return
	}
	switch {
	case got == spaceMore:
		if m.spaceMenu == nil {
			m.openSpaceMenu()
		}
		if run := m.tmuxRun(); run != nil {
			writeSpace(run, m.opt.Pane, m.space)
		}
	case got != m.space:
		m.space, m.spaceMenu, m.peekAt = got, nil, m.now()
	}
}

func (m Model) writeSpace() tea.Cmd {
	run := m.tmuxRun()
	if run == nil {
		return nil
	}
	console, space := m.opt.Pane, m.space
	return func() tea.Msg {
		writeSpace(run, console, space)
		return nil
	}
}

func (m *Model) setSpace(name string) tea.Cmd {
	if name == spaceAll {
		name = ""
	}
	m.space, m.spaceMenu, m.peekAt = name, nil, m.now()
	return m.writeSpace()
}

func (m *Model) turnSpace(d int) tea.Cmd {
	stops := m.spaceStops()
	if len(stops) == 0 {
		return nil
	}
	i := slices.Index(stops, m.space)
	if i < 0 {
		i = 0
	}
	return m.setSpace(stops[(i+d+len(stops))%len(stops)])
}

func (m *Model) openSpaceMenu() {
	if m.spaceMenu != nil {
		return
	}
	_, _, hidden := m.fitSpaces(m.spaceBudget(max(m.width, minWidth)))
	if len(hidden) == 0 {
		return
	}
	m.spaceMenu = &spaceMenu{names: hidden}
}

func (m Model) spaceEmpty() string {
	text := "Nothing in " + m.space
	if m.boardHost != "" {
		text += " on " + m.boardHost
	}
	return text + ". " + m.keys.main.Back.Help().Key + " shows all."
}

func (m Model) spaceMenuKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	mk := m.keys.main
	sm := *m.spaceMenu
	switch {
	case bkey.Matches(k, mk.Up):
		sm.at = max(sm.at-1, 0)
	case bkey.Matches(k, mk.Down):
		sm.at = min(sm.at+1, len(sm.names)-1)
	case bkey.Matches(k, mk.Back):
		m.spaceMenu = nil
		return m, nil
	case bkey.Matches(k, mk.Open):
		return m, m.setSpace(sm.names[sm.at])
	default:
		return m, nil
	}
	m.spaceMenu = &sm
	return m, nil
}

func (m Model) overlaySpaceMenu(sc *drawn, w int) {
	st, sm := m.st, m.spaceMenu
	labelW := 0
	for _, name := range sm.names {
		labelW = max(labelW, lipgloss.Width(name))
	}
	bw := min(max(labelW+6, 16), w)
	rows := make([]string, len(sm.names))
	for i, name := range sm.names {
		label := st.text
		if i == sm.at {
			label = st.text.Bold(true)
		}
		rows[i] = spread(m.chosenMark(i == sm.at)+" "+label.Render(name), "", bw-4)
	}
	box, _ := boxFrame(st.hair, "spaces", rows, nil, bw)
	x, y := min(2, w-bw), max(sc.bodyTop, 3)
	for j, b := range box {
		r := y + j
		if r >= len(sc.lines) {
			break
		}
		l := sc.lines[r]
		left := ansi.Truncate(l, x, "")
		left += strings.Repeat(" ", max(x-lipgloss.Width(left), 0))
		sc.lines[r] = left + b + ansi.TruncateLeft(l, x+lipgloss.Width(b), "")
	}
	sc.boxAt, sc.boxX, sc.boxW, sc.boxRows = y, x, bw, box
}

func (m Model) spaceMenuTap(ms tea.Mouse) (tea.Model, tea.Cmd) {
	w := max(m.width, minWidth)
	sc := m.draw(w)
	if ms.X < sc.boxX || ms.X >= sc.boxX+sc.boxW || ms.Y < sc.boxAt || ms.Y >= sc.boxAt+len(sc.boxRows) {
		m.spaceMenu = nil
		return m, nil
	}
	i := ms.Y - sc.boxAt - 1
	if ms.Button == tea.MouseLeft && i >= 0 && i < len(m.spaceMenu.names) {
		return m, m.setSpace(m.spaceMenu.names[i])
	}
	return m, nil
}

func (m Model) pickSpace(press string) (tea.Model, tea.Cmd) {
	switch {
	case press == spaceMore:
		m.openSpaceMenu()
		return m, nil
	case strings.HasPrefix(press, spacePref):
		return m, m.setSpace(strings.TrimPrefix(press, spacePref))
	}
	return m, nil
}

// headerBar is the one-row desk header: spaces, wait count, done count, ⚙.
func headerBar(w int, chips string, need, done int, mark bool) string {
	left := " ▰ matchblox"
	if chips != "" {
		left += "  " + chips
	}
	cog := "⚙"
	if mark { // a busy loop, or a plan within 10% of its limit
		cog = "⚙ !"
	}
	right := fmt.Sprintf("● %d need you · ✓ %d done  %s", need, done, cog)
	if w <= 0 {
		return left + "  " + right
	}
	return spread(left, right, w)
}
