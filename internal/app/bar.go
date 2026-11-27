package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/sample"
)

// barLine is the desk's one line: the mode word, the hosts, the plan, the
// stoker, and ? keys. A confirm replaces the chips until the list answers.
// A chip tap is Task 6.
func (m Model) barLine(w int) string {
	mode, rest, confirm := strings.Cut(m.barMode, "\t")
	if mode == "" {
		mode = "LIST"
	}
	if confirm {
		line := m.modeWord("CONFIRM") + "  " + m.st.text.Render(rest)
		if w <= 0 {
			return line
		}
		return fit(line, w)
	}
	right := m.barRight(mode)
	usage, stoke := m.usageChip(), m.barStoker()
	word := m.modeWord(mode)
	budget := w
	if w > 0 {
		budget = w - lipgloss.Width(word) - lipgloss.Width(right) - 4
		if usage != "" {
			budget -= lipgloss.Width(usage) + 3
		}
		if stoke != "" {
			budget -= lipgloss.Width(stoke) + 3
		}
	}
	mid := word
	if hosts := m.barHosts(budget); hosts != "" {
		mid += "   " + hosts
	}
	if usage != "" {
		mid += "   " + usage
	}
	if stoke != "" {
		mid += "   " + stoke
	}
	if w <= 0 {
		return mid + "  " + right
	}
	return spread(mid, right, w)
}

// modeWord is the bar's far-left word. LIST is muted. Every place that has
// the keys is accent bold.
func (m Model) modeWord(mode string) string {
	if mode == "LIST" {
		return m.st.muted.Render(mode)
	}
	return m.st.accent.Bold(true).Render(mode)
}

// barRight is ? keys, and ctrl+q list before it while the agent or the
// shell has the keys.
func (m Model) barRight(mode string) string {
	keys := m.st.muted.Render("? keys")
	if mode == "AGENT" || mode == "SHELL" {
		return m.st.muted.Render(m.keys.side.Flip.Help().Key+" list") + "  " + keys
	}
	return keys
}

// barHosts are the host chips, then s▸. Calm hosts fold to "N hosts s▸"
// when the line is short. A host in trouble stays.
func (m Model) barHosts(budget int) string {
	type chip struct {
		text string
		hot  bool
	}
	var chips []chip
	for _, h := range m.hosts() {
		text, hot := m.hostChip(h)
		chips = append(chips, chip{text, hot})
	}
	if len(chips) == 0 {
		return ""
	}
	join := func(cs []chip) string {
		parts := make([]string, len(cs))
		for i, c := range cs {
			parts[i] = c.text
		}
		return strings.Join(parts, "  ")
	}
	tail := m.st.muted.Render("  s▸")
	full := join(chips) + tail
	if budget <= 0 || lipgloss.Width(full) <= budget {
		return full
	}
	var keep []chip
	calm := 0
	for _, c := range chips {
		if c.hot {
			keep = append(keep, c)
		} else {
			calm++
		}
	}
	if calm == 0 {
		return full
	}
	fold := m.st.muted.Render(fmt.Sprintf("%d hosts s▸", calm))
	if len(keep) == 0 {
		return fold
	}
	return join(keep) + "  " + fold
}

// hostChip is one host: cpu and memory, or its trouble. hot: the chip is
// in trouble and must not fold. The trouble colour is on that chip only.
func (m Model) hostChip(host string) (string, bool) {
	st := m.st
	lead := st.text.Render("● " + m.nameOf(host))
	if len(m.orphansOf(host)) > 0 {
		return lead + " " + st.neg.Render("⚠ busy loop"), true
	}
	if m.hostDown(host) {
		since := "now"
		if at := m.stateAt(host); !at.IsZero() {
			if d := m.now().Sub(at); d >= time.Second {
				since = brief(d)
			}
		}
		return lead + " " + st.warn.Render("not reached · since "+since), true
	}
	mc := m.machineOf(host)
	cpu := "cpu unknown"
	if len(mc.Cores) > 0 {
		cpu = fmt.Sprintf("cpu %.0f%%", mc.CPU)
	}
	mem := "mem unknown"
	if _, pct := memUse(mc); mc.MemTotal > 0 {
		mem = fmt.Sprintf("mem %.0f%%", pct)
	}
	return lead + " " + st.muted.Render(cpu+" "+mem), false
}

// orphansOf are host's detached busy loops.
func (m Model) orphansOf(host string) []sample.Orphan {
	if host == m.opt.Host {
		if m.all.orphans != nil {
			return m.all.orphans
		}
		return m.snap.Orphans
	}
	return m.stateOf(host).Orphans
}

// hostDown: the console cannot read host.
func (m Model) hostDown(host string) bool {
	if m.stale(host) {
		return true
	}
	if host == m.opt.Host {
		return m.lost && !m.have
	}
	i := m.peerAt(host)
	if i < 0 {
		return false
	}
	l := m.peers[i]
	return !l.have && (l.lost || l.blocked != nil || l.dialErr != nil)
}

// usageChip is the plan in one chip: 5h and the week, each with ↻ until
// it resets. A window within 10% of its limit takes warn on that chip.
func (m Model) usageChip() string {
	rows, _ := m.usageRows(false)
	for _, r := range rows {
		var bits []string
		for _, u := range r.a.Uses(m.now()) {
			if u.Name != "5h" && u.Name != "wk" {
				continue
			}
			pct := m.st.muted.Render(fmt.Sprintf("%.0f%%", u.Pct))
			if u.Pct >= spentAt {
				pct = m.st.warn.Render(fmt.Sprintf("%.0f%%", u.Pct))
			}
			bit := m.st.muted.Render(u.Name) + " " + pct
			if !u.Resets.IsZero() {
				bit += m.st.muted.Render(" ↻" + sample.Human(u.Resets.Sub(m.now())))
			}
			bits = append(bits, bit)
		}
		if len(bits) == 0 {
			continue
		}
		name := r.name
		if i := strings.LastIndex(name, " "); i >= 0 {
			name = name[i+1:]
		}
		line := m.accountMark(r.a.Provider) + " " + m.st.text.Render(name)
		for i, b := range bits {
			if i == 0 {
				line += " " + b
			} else {
				line += m.st.muted.Render(" · ") + b
			}
		}
		return line
	}
	return ""
}

// barStoker is the stoker's chip: off, ≋ stoker, a night, or rests. ≋ N
// when more than one runs.
func (m Model) barStoker() string {
	n := 0
	kept, night, rests, on := false, false, false, false
	var until time.Time
	for _, h := range m.hosts() {
		n += m.chainsAtWork(h)
		v := m.stokerOf(h)
		if v == nil {
			continue
		}
		kept = true
		if !v.On {
			continue
		}
		switch {
		case !v.Rests.IsZero():
			rests = true
		case !v.Until.IsZero():
			night, until = true, v.Until
		default:
			on = true
			n++
		}
	}
	if !kept && n == 0 {
		return ""
	}
	st := m.st
	if n > 1 {
		return st.stoker.Render("≋ " + strconv.Itoa(n))
	}
	switch {
	case night:
		return st.stoker.Render("☾ night → " + until.Local().Format("15:04"))
	case rests && !on:
		return st.muted.Render("stoker rests")
	case on || n == 1:
		return st.stoker.Render("≋ stoker")
	}
	return st.faint.Render("stoker off")
}

// cogMark: ⚙ shows ! while a detached busy loop burns or a plan is within
// 10% of its limit. The Machine tab's mark, once that tab is gone.
func (m Model) cogMark() bool {
	if len(m.nearlySpent()) > 0 {
		return true
	}
	for _, h := range m.hosts() {
		if len(m.orphansOf(h)) > 0 {
			return true
		}
	}
	return false
}

// confirmRest is the bar's question while the list waits for y or esc.
func (m Model) confirmRest(a action) string {
	yes, esc := m.keys.confirm.Yes.Help().Key, m.keys.main.Back.Help().Key
	if a.label == "kill" {
		if what, _, ok := strings.Cut(a.say, ":"); ok && strings.HasPrefix(what, "kill ") {
			return "kill " + strings.TrimPrefix(what, "kill ") + "?  " + yes + " kill · " + esc + " keep"
		}
		return strings.TrimSpace(a.say) + "  " + yes + " kill · " + esc + " keep"
	}
	what := a.label
	if what == "" {
		what = "run this"
	}
	return what + "?  " + yes + " run · " + esc + " cancel"
}

// deskModeValue is what the list publishes: a confirm while one is open,
// KEYS while the keys sheet is open, else LIST.
func (m Model) deskModeValue() string {
	switch {
	case m.pending != nil:
		return "CONFIRM\t" + m.confirmRest(*m.pending)
	case m.held != nil:
		return "CONFIRM\ttake over?  " + m.keys.takeOver.Rest.Help().Key + " rest · " + m.keys.takeOver.Keep.Help().Key + " keep"
	case m.keySheet != nil:
		return "KEYS"
	}
	return "LIST"
}

// deskModeOwned is a mode the list publishes. AGENT and SHELL are not:
// the pane that has the keys writes those.
func deskModeOwned(mode string) bool {
	return strings.HasPrefix(mode, "CONFIRM") || mode == "KEYS"
}

// syncDeskMode publishes a confirm or KEYS, and LIST again when that ends.
// AGENT and SHELL stay with the writer that moves the keys, until the
// list pane is current again.
func (m *Model) syncDeskMode() tea.Cmd {
	if !m.deskFrame || m.frameKind != "" {
		return nil
	}
	next := m.deskModeValue()
	was := deskModeOwned(m.barMode)
	want := deskModeOwned(next)
	if !was && !want || next == m.barMode {
		return nil
	}
	m.barMode = next
	return m.publishMode(next)
}

// syncListMode publishes LIST, or the open confirm, because the list pane
// is current. It does not wait for the next confirm.
func (m *Model) syncListMode() tea.Cmd {
	if !m.deskFrame || m.frameKind != "" {
		return nil
	}
	next := m.deskModeValue()
	if next == m.barMode {
		return nil
	}
	m.barMode = next
	return m.publishMode(next)
}

// deskPollMsg is the quarter-second look. deskModeMsg is what it read:
// which pane is current, and the mode option as tmux has it. The agent's
// write does not pass through the list's memory.
type deskPollMsg struct{}

type deskModeMsg struct {
	list bool
	mode string
}

// watchDeskMode reads the current pane once. The list pane publishes LIST.
func (m Model) watchDeskMode() tea.Cmd {
	if !m.deskFrame || m.frameKind != "" {
		return nil
	}
	run := m.tmuxRun()
	console := m.opt.Pane
	if run == nil || console == "" {
		return nil
	}
	return func() tea.Msg {
		list, mode := deskPaneLook(run, console)
		return deskModeMsg{list: list, mode: mode}
	}
}

// modePollLoop looks again each quarter second, the bar's own poll.
// The program starts it from Init. A desk that is not open yet just waits.
func (m Model) modePollLoop() tea.Cmd {
	return tea.Tick(time.Second/4, func(time.Time) tea.Msg { return deskPollMsg{} })
}

// deskPaneLook is the current pane against the console, and the mode
// option. Review and shell have no pane id here.
func deskPaneLook(run func(args ...string) (string, error), console string) (bool, string) {
	out, _ := run("show-option", "-wqv", "-t", console, sample.ModeOption)
	return deskListCurrent(run, console), strings.TrimSpace(out)
}

// deskListCurrent: the pane in use is @matchblox_console, and not the
// header or the bar.
func deskListCurrent(run func(args ...string) (string, error), console string) bool {
	cur := inUse(run, console)
	named, err := run("show-option", "-wqv", "-t", console, sample.ConsoleOption)
	list := strings.TrimSpace(named)
	if err != nil || list == "" {
		list = console
	}
	if cur == "" || cur != list {
		return false
	}
	for _, opt := range []string{sample.HeaderOption, sample.BarOption} {
		out, err := run("show-option", "-wqv", "-t", console, opt)
		if err == nil && strings.TrimSpace(out) == cur {
			return false
		}
	}
	return true
}

func (m Model) publishMode(mode string) tea.Cmd {
	run := m.tmuxRun()
	if run == nil {
		return nil
	}
	console := m.opt.Pane
	return func() tea.Msg {
		writeMode(run, console, mode)
		return nil
	}
}
