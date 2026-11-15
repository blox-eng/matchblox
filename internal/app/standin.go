package app

import (
	"fmt"
	"strings"
	"time"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/logos"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/sample"
)

// The demo's stand-in for a pane: the watched screen takes the whole
// screen and forwards the keys to the demo's world.

// viewLines is the watched screen fitted to w by h, or what stands in its
// place: that the session ended, or blank lines until a screen comes.
func (m Model) viewLines(w, h int) []string {
	h = max(h, 0)
	lines := make([]string, h)
	s, ok := m.selected()
	if !ok || s.Pane == "" || isShipCard(s) || s.Host != m.opt.Host {
		return lines
	}
	shown := m.watchHost == s.Host && m.screen.Pane == s.Pane
	if shown && m.screen.Gone {
		const text = "That session ended."
		if h > 0 {
			lines[h/2] = strings.Repeat(" ", max((w-ansi.StringWidth(text))/2, 0)) + text
		}
		return lines
	}
	var fit []string
	switch {
	case !shown:
	case m.screen.Alt:
		fit = cropScreen(m.screen.Text, w, h)
	default:
		fit = fitScreen(m.screen.Text, w, h)
	}
	return append(lines[:len(lines)-len(fit)], fit...)
}

// menuLeast is the fewest rows a chat at a menu needs for the whole of what
// the person answers. It is the tallest common Claude Code menu, the question
// tool's multi-select at its four options with descriptions, from its
// question down: the question, a blank, 4 x 2 option lines, the two typing
// lines, a rule, Chat about this, a blank and the key hint. The rule and the
// tabs over the question may scroll off. A permission prompt is shorter.
// The console does not read the menu's height: the agent draws at the chat's
// size, and redraws above this.
const menuLeast = 16

// viewPlace is where the selected session's stand-in is drawn.
type viewPlace int

const (
	placeNone viewPlace = iota // no stand-in, or no room for its menu
	placeFull                  // the whole screen: the demo's stand-in for a pane
)

// viewPlacement is the one answer to where the selected session's live
// view is drawn, and how many rows it has for its screen: the demo's
// stand-in is the whole screen at every width, unless it waits at a menu
// and the screen has under menuLeast rows for it.
func (m Model) viewPlacement() (viewPlace, int) {
	if m.tab != tabSessions || !m.standIn {
		return placeNone, 0
	}
	if s, ok := m.selectedSession(); ok && m.height > 0 && m.height-3 < menuLeast && m.atMenu(s) {
		return placeNone, 0
	}
	return placeFull, max(m.height-3, 0)
}

// viewTitle names the chat's session and its state.
func (m Model) viewTitle(s sample.Session) string { return m.titleOf(s) + " · " + m.wordOf(s) }

// screenLines are viewLines, each closed so the pane's styles stop at its
// end.
func (m Model) screenLines(w, h int) []string {
	lines := m.viewLines(w, h)
	for i, l := range lines {
		lines[i] = l + ansi.ResetStyle
	}
	return lines
}

// viewScreen is the stand-in: the whole screen under a title row that goes
// back to Sessions and says a toast, over the key line, with a step that
// asks to confirm on top. Its title, hairline and key line always show: a
// screen under 4 rows has no room for the agent's lines.
func (m Model) viewScreen(w int) drawn {
	st := m.st
	s, _ := m.selectedSession()
	title := " " + st.accent.Render("‹ sessions") + st.muted.Render(" · ")
	head := m.viewTitle(s)
	if room := w - lipgloss.Width(title); lipgloss.Width(head) > room {
		head = ansi.Truncate(s.Name, max(room, 1), "…") // the state, then the name, give way
	}
	title += st.accent.Render(head)
	if m.toast != "" { // as the header says it, where there is one
		title = spread(title, st.accent.Render(ansi.Truncate(m.toast, max(w/2, 1), "…"))+" ", w)
	}
	title = fit(title, w)
	_, rows := m.viewPlacement()
	sc := drawn{lines: []string{title}, bodyTop: 1, placeFull: true, viewRows: rows, viewAt: 1, viewW: w}
	sc.lines = append(sc.lines, m.screenLines(w, rows)...)
	hair, keys, spans := m.keyRows(w)
	sc.keysAt, sc.keys = len(sc.lines)+1, spans
	sc.lines = append(sc.lines, hair, keys)
	if m.consenting() {
		m.overlayConsent(&sc, w)
	}
	return sc
}

// homeKey is the app's home gesture (panes.HomeKey, which the console may
// not import): it leaves the stand-in for the board.
const homeKey = "\x1b_matchblox\x1b\\"

// menuWait tells if a wait is answered on a menu: a digit or Enter there
// picks.
func menuWait(it queue.Item) bool {
	return it.State == queue.StatePermission || it.State == queue.StateQuestion
}

// atMenu tells if s waits on an agent's menu: a permission prompt or the
// question tool, by the hooks, the transcript or the screen.
func (m Model) atMenu(s sample.Session) bool {
	if it, waits := m.waitOf(s); waits && menuWait(it) {
		return true
	}
	return s.Permits || s.Ask != nil
}

// viewBackWait is how long after ctrl+a an m leaves the stand-in.
const viewBackWait = time.Second

// viewBacks tells if k is the m of ctrl+a m, whose ctrl+a came at armed.
func (m Model) viewBacks(k tea.KeyPressMsg, armed time.Time) bool {
	return !armed.IsZero() && bkey.Matches(k, m.keys.view.BackEnd) && m.now().Sub(armed) <= viewBackWait
}

// minSignalTmux is the oldest tmux that passes the agent signal through to
// the terminal (allow-passthrough).
const minSignalTmux = "3.3"

// sayAgent writes an agent signal to the terminal, through tmux when the
// console runs in it; nil for a tmux too old to pass it.
func sayAgent(say string, o Options) tea.Cmd {
	switch {
	case o.OutsideTmux:
		return tea.Raw(say)
	case remote.TmuxTooOld(o.LocalTmux, minSignalTmux):
		return nil
	}
	return tea.Raw(logos.Passthrough(say))
}

// tapBack is what a tap on the way back presses: no key, as the stand-in
// would send it to the agent.
const tapBack = "back"

// backKeys is the way back as the key line names it: this machine's tmux
// prefix, read once at the start, then m. Without tmux, or a prefix it can
// name, it is the stand-in's own ctrl+a m. A console tmux does not know by name
// has only the tap for sure, as its prefix may be ctrl+a.
func (m Model) backKeys() string {
	if m.opt.NoPrefixM {
		return "tap"
	}
	if m.opt.Prefix != nil {
		if p, ok := prefixName(m.opt.Prefix()); ok {
			return p + " m"
		}
	}
	return m.keys.view.Back.Help().Key
}

// standInHint is the stand-in's key line while its session waits at a menu:
// it is answered as a real pane is, with the belt.
const standInHint = "type 2 to answer"

// viewFull tells if the stand-in takes the whole screen: opened on one
// column, with room for its question.
func (m Model) viewFull() bool {
	place, _ := m.viewPlacement()
	return m.standIn && place == placeFull
}

// openStandIn is the demo's stand-in for the pane of a home session: its
// screen, full-screen, takes the keys and forwards them to the demo's
// world. Outside the demo m is as it was.
func (m Model) openStandIn(pane string) Model {
	s, ok := m.sessionIn(m.opt.Host, pane)
	if m.opt.Demo == "" || !ok {
		return m
	}
	if m.tab != tabSessions {
		m.setTab(tabSessions)
	}
	m.doorPick, m.recPick, m.blockedPick, m.chose, m.selPID, m.selHost = "", "", 0, true, s.PID, s.Host
	m.standIn, m.flash = true, ""
	return m
}

// leaveStandIn goes back from the stand-in to the list, on the session
// that waits next, as a jump back from a real pane does. Where one pane
// shows, the list comes, not the chat the stand-in opened from.
func (m Model) leaveStandIn() Model {
	s, _ := m.selectedSession()
	m.standIn = false
	m.keysOut.queue = nil // typed at a screen that is gone; the key in flight still answers
	if m.onePane(max(m.width, minWidth)) {
		m.toList()
	}
	return m.nextAfter(s.Pane)
}

// keepView closes the stand-in once the board or the chosen session's pane
// on this host is gone, or the screen has no room for its question: the
// card shows the question then, and the keys are the console's, never a
// screen's the person cannot see. A new tap or Enter opens it again. A
// session that ended shows the board again, and the key line says why.
func (m Model) keepView() Model {
	if !m.standIn {
		return m
	}
	s, ok := m.selectedSession()
	place, _ := m.viewPlacement()
	switch {
	case !ok || m.tab != tabSessions || s.Pane == "" || s.Host != m.opt.Host || place == placeNone:
		m.standIn = false
	case m.screen.Pane == s.Pane && m.screen.Gone:
		m.standIn, m.flash = false, "That session ended."
	}
	if !m.standIn {
		m.keysOut.queue = nil
	}
	return m
}

// standInKey is k in the demo's stand-in: ctrl+a arms the way back, and
// every other key goes to the agent's pane in the demo's world. Outside the
// demo it sends nothing.
func (m Model) standInKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.opt.Demo == "" {
		return m, nil
	}
	m.flash = ""
	if bkey.Matches(k, m.keys.view.Back) {
		m.backArmed = m.now()
		return m, nil
	}
	text, ok := keyName(k)
	s, on := m.selectedSession()
	if !ok || !on || s.Pane == "" || s.Host != m.opt.Host {
		return m, nil
	}
	send := m.sendKey(s, text)
	return m, send
}

// liveKeyLine is the key line of the stand-in: the way in, or how to
// answer, and the way back.
func (m Model) liveKeyLine(w int) (string, []span) {
	st := m.st
	s, _ := m.selectedSession()
	type part struct {
		text string
		key  string // the key a tap on it presses; "" none
	}
	lead := part{st.text.Render("keys go to " + s.Name), ""}
	if m.atMenu(s) {
		lead = part{st.text.Render(standInHint), ""}
	}
	back := part{st.accent.Render(m.backKeys()) + st.muted.Render(" back"), tapBack}
	if n := m.screen.Back; n > 0 {
		lead.text = st.warn.Render(fmt.Sprintf("↑ %d lines back", n)) + st.muted.Render(" · ") + lead.text
	}
	sep := st.muted.Render(" · ")
	parts := []part{lead, back}
	if 1+lipgloss.Width(lead.text)+lipgloss.Width(sep)+lipgloss.Width(back.text) > w {
		parts = parts[:1]
	}
	line, x := " ", 1
	var spans []span
	for i, p := range parts {
		if i > 0 {
			line += sep
			x += lipgloss.Width(sep)
		}
		if p.key != "" {
			spans = append(spans, span{x, x + lipgloss.Width(p.text), p.key})
		}
		line += p.text
		x += lipgloss.Width(p.text)
	}
	return fit(line, w), spans
}

// wheelView scrolls the stand-in back through the service's scroll-back:
// the wheel goes as the keys act, and opens nothing.
func (m Model) wheelView(up bool) (tea.Model, tea.Cmd) {
	s, ok := m.selectedSession()
	if !ok || s.Pane == "" || isShipCard(s) || s.Host != m.opt.Host {
		return m, nil
	}
	text := proto.WheelDown
	if up {
		text = proto.WheelUp
	}
	send := m.sendKey(s, text)
	return m, send
}
