package app

import (
	"fmt"
	"regexp"
	"strconv"
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
	"github.com/blox-eng/matchblox/internal/transport"
)

// watchActID starts the id of a watch act; the pane follows, so a refusal says which watch it was for.
const watchActID = "watch:"

// keyActID starts the id of a key the live view or the stand-in sends: its
// answer says nothing unless it is a refusal.
const keyActID = "key:"

// screenMsg is one capture of the watched pane.
type screenMsg proto.Screen

// trailingBlank is the spaces at the end of a line, and the style codes
// among them.
var trailingBlank = regexp.MustCompile(`(?:\x1b\[[0-9;:]*m|[ \t])+$`)

var sgr = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

// controlStrings is an OSC (a hyperlink, a title), or an APC, DCS, PM or SOS
// string, up to its terminator: an agent's screen keeps none of them.
var controlStrings = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[_P^X][^\x1b]*\x1b\\`)

// fitScreen fits a pane's text into w columns and the last h lines: a rule
// is cut, other text wraps between words, and the blank lines at the end go.
// Of its escape sequences only the styles stay: a link or a string control
// of the agent's never reaches the console's terminal.
func fitScreen(text string, w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	text = controlStrings.ReplaceAllString(text, "")
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line = trimBlank(line)
		if isRule(line) {
			out = append(out, ansi.Truncate(line, w, ""))
			continue
		}
		for _, piece := range strings.Split(ansi.Wrap(line, w, ""), "\n") {
			out = append(out, trimBlank(piece))
		}
	}
	for len(out) > 0 && ansi.Strip(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	if len(out) > h {
		out = out[len(out)-h:]
	}
	return out
}

// trimBlank drops the spaces at the end of a line and keeps the style codes
// among them.
func trimBlank(line string) string {
	return trailingBlank.ReplaceAllStringFunc(line, func(tail string) string {
		return strings.Join(sgr.FindAllString(tail, -1), "")
	})
}

// isRule is a line of box-drawing runes and spaces, and not empty.
func isRule(line string) bool {
	plain := ansi.Strip(line)
	if strings.TrimSpace(plain) == "" {
		return false
	}
	for _, r := range plain {
		if r != ' ' && (r < 0x2500 || r > 0x257F) {
			return false
		}
	}
	return true
}

// asDrawn is a screen the agent drew w columns wide, in its last h lines,
// each as it is: cut at w and no more, its trailing spaces and blank rows
// kept. Of its escape sequences only the styles stay, as in fitScreen.
func asDrawn(text string, w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	text = controlStrings.ReplaceAllString(strings.ReplaceAll(text, "\r", ""), "")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	lines = lines[max(len(lines)-h, 0):]
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, "")
	}
	return lines
}

// sizedFor tells if the screen is the agent's live screen drawn for a chat
// w columns wide and h lines tall: the service sized its window to it.
func (m Model) sizedFor(w, h int) bool {
	return m.screen.Cols > 0 && m.screen.Cols == w && m.screen.Rows <= h
}

// sizeDwell is how long the live view's session stays selected before the
// agent's window takes the view's size: a cursor that passes a row resizes
// no agent.
const sizeDwell = time.Second

// sizeDueMsg comes when a selection may have rested sizeDwell.
type sizeDueMsg struct{}

// chatSize is the room the chat gives the agent's screen as the console
// draws it now: its columns, and its rows under the chat's title and strip.
// 0, 0 where no chat is drawn. The service sizes the agent's window to it.
func (m Model) chatSize() (cols, rows int) {
	if m.tab != tabSessions {
		return 0, 0
	}
	w := max(m.width, minWidth)
	_, detailW := m.panes(w)
	place, full := m.chatPlacement(w)
	switch {
	case place == chatFull && m.viewOpen:
		cols, rows = w, full
	case place == chatBeside: // beside the list, as chatPlacement has it
		s, ok := m.chatShown(detailW)
		if !ok {
			return 0, 0
		}
		room := m.bodyRows()
		cols, rows = detailW-1, room-1-len(m.chatStrip(s, detailW, room-1))
	}
	if cols <= 0 || rows <= 0 {
		return 0, 0
	}
	return cols, rows
}

// watchCmd asks the service for the selected session's screen when it is
// not the one already watched, and for none when the selection has no pane
// or is another host's; or the chat's size, when it changed. A screen of another pane never stays.
func (m *Model) watchCmd() tea.Cmd {
	want := ""
	if m.conn != nil && !m.lost && !m.mismatch {
		if s, ok := m.selected(); ok && s.Host == m.opt.Host { // another host's pane id names another pane here
			want = s.Pane
		}
	}
	if m.watchHeld != "" && want == m.watchHeld {
		want = m.watching
	}
	if want != m.watching {
		m.screen = proto.Screen{} // another pane opens blank until its screen comes
		m.watchSince = m.now()
	}
	var size [2]int
	var due tea.Cmd
	if want != "" {
		size[0], size[1] = m.chatSize()
	}
	if wait := m.watchSince.Add(sizeDwell).Sub(m.now()); size != [2]int{} && wait > 0 {
		size = [2]int{} // browsing: no agent redraws for a row the cursor passes
		if !m.sizeTick.Equal(m.watchSince) {
			m.sizeTick = m.watchSince
			due = tea.Tick(wait, func(time.Time) tea.Msg { return sizeDueMsg{} })
		}
	}
	if size == [2]int{} && want == m.watching {
		size = m.watchSize // a chat off screen keeps its size: the agent does not redraw at each tab
	}
	if want == m.watching && size == m.watchSize {
		return due
	}
	m.watching, m.watchSize = want, size
	if m.conn == nil || m.lost || m.mismatch {
		return due // the service forgot the watch with the connection
	}
	c := m.conn
	act := proto.Act{RecID: proto.WatchPrefix + want, Cols: size[0], Rows: size[1]}
	send := func() tea.Msg { // not m.send: the watch takes no number from the acts the builder sees
		if err := c.Send(proto.KindAct, watchActID+want, act); err != nil {
			return lostMsg{err, c}
		}
		return nil
	}
	if due != nil {
		return tea.Batch(send, due)
	}
	return send
}

// chatLines is the watched screen fitted to w by h, or what stands in its
// place: that the session ended, or blank lines until a screen comes.
func (m Model) chatLines(w, h int) []string {
	h = max(h, 0)
	lines := make([]string, h)
	s, ok := m.selected()
	if !ok || s.Pane == "" || s.Host != m.opt.Host {
		return lines
	}
	shown := m.screen.Pane == s.Pane
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
	case m.sizedFor(w, h): // top first, as in the agent's own terminal
		fit = asDrawn(m.screen.Text, w, h)
		fit = append(fit, make([]string, h-len(fit))...)
	default:
		fit = fitScreen(m.screen.Text, w, h)
	}
	return append(lines[:len(lines)-len(fit)], fit...)
}

// chatShown is the session whose chat is the right side of Sessions: the
// selected one, when it has a pane on the home host and the screen has a
// right side. Another host's row is list only: its detail shows there.
func (m Model) chatShown(rightW int) (sample.Session, bool) {
	s, ok := m.selectedSession()
	return s, ok && s.Pane != "" && s.Host == m.opt.Host && m.tab == tabSessions && rightW > 0
}

// chatLeast is the narrowest chat beside the list: an agent's menu reads at
// 44 columns, as on a folded phone.
const chatLeast = 44

// menuLeast is the fewest rows a chat at a menu needs for the whole of what
// the person answers. It is the tallest common Claude Code menu, the question
// tool's multi-select at its four options with descriptions, from its
// question down: the question, a blank, 4 x 2 option lines, the two typing
// lines, a rule, Chat about this, a blank and the key hint. The rule and the
// tabs over the question may scroll off. A permission prompt is shorter.
// The console does not read the menu's height: the agent draws at the chat's
// size, and redraws above this.
const menuLeast = 16

// chatPlace is where Sessions draws the selected session's chat.
type chatPlace int

const (
	chatNone   chatPlace = iota // no room for its menu: the list's opened row shows its question
	chatBeside                  // the right side of Sessions, beside the list
	chatFull                    // the whole screen, once a tap opens it
)

// chatPlacement is the one answer to where the selected session's live
// view is drawn at w columns, and how many rows it has for its title, strip
// and screen: beside the list where Sessions splits, else the whole screen,
// unless it waits at a menu and the screen has under menuLeast rows for it.
// The demo's stand-in is the whole screen at every width.
func (m Model) chatPlacement(w int) (chatPlace, int) {
	if m.tab != tabSessions {
		return chatNone, 0
	}
	if _, right := m.panes(w); right > 0 && !m.standIn {
		return chatBeside, max(m.height-5, 0)
	}
	if s, ok := m.selectedSession(); ok && m.height > 0 && m.height-3 < menuLeast && m.atMenu(s) {
		return chatNone, 0
	}
	return chatFull, max(m.height-3, 0)
}

// oneColumn tells if Sessions is one column at w, with no right side: as
// on a phone, a tap on a row opens its live view over the whole screen.
func (m Model) oneColumn(w int) bool {
	_, right := m.panes(w)
	return m.tab == tabSessions && right == 0
}

// chatRoom tells if a chat right columns wide fits beside the list: it is
// at least chatLeast wide, and the body has room for chatReserve.
func (m Model) chatRoom(right int) bool {
	if right < chatLeast {
		return false
	}
	if m.height == 0 {
		return true // no height yet
	}
	return m.height-5 >= m.chatReserve(right)
}

// chatReserve is the most rows the chat right columns wide takes, of any
// home session the cursor can move to: its title, its strip as drawn (two
// lines on a narrow chat) and menuLeast. Sessions splits by it, so the split
// holds as the cursor moves and as a session comes to a menu.
func (m Model) chatReserve(right int) int {
	most := 1 + menuLeast
	for _, s := range m.snap.Sessions {
		if most == 3+menuLeast { // the strip is two lines at most
			break
		}
		if s.Host == m.opt.Host && s.Pane != "" {
			most = max(most, 1+len(m.chatStrip(s, right, 2))+menuLeast)
		}
	}
	return most
}

// chatTitle names the chat's session and its state.
func (m Model) chatTitle(s sample.Session) string { return s.Name + " · " + m.wordOf(s) }

// chatPane is the live view beside the list, w by h: its title over the
// strip and the screen.
func (m Model) chatPane(s sample.Session, w, h int) []string {
	lines := []string{fit(" "+m.st.label.Render(m.chatTitle(s)), w)}
	strip := m.chatStrip(s, w, h-1)
	lines = append(lines, strip...)
	for _, l := range m.screenLines(w-1, h-1-len(strip)) {
		lines = append(lines, " "+l)
	}
	return lines
}

// chatStrip is what the session's detail said of it, over its chat in w
// columns and at most rows lines: its account, its PR and its background
// work. One line where they fit, else two; none when it has none.
func (m Model) chatStrip(s sample.Session, w, rows int) []string {
	st := m.st
	var facts []string
	if a := accountCell(s); a != "" {
		facts = append(facts, st.muted.Render(a))
	}
	if ss := m.shipsOf(s); len(ss) > 0 {
		facts = append(facts, m.shipCell(ss[0], true)+m.shipsMore(ss, false))
	} else if p := m.prOf(s.Cwd); p != nil {
		pr := st.accent.Render("#" + strconv.Itoa(p.Number))
		if p.Draft {
			pr += " " + st.text.Render("draft")
		}
		facts = append(facts, pr)
	}
	if s.Agents > 0 {
		facts = append(facts, st.text.Render(plural(s.Agents, "subagent")+" running"))
	}
	if len(s.Shells) > 0 {
		shells := plural(len(s.Shells), "shell")
		if old := s.Shells[0].Started; !old.IsZero() {
			shells += " · oldest " + sample.Human(m.now().Sub(old))
		}
		facts = append(facts, st.text.Render(shells))
	}
	if len(facts) == 0 {
		return nil
	}
	sep := st.muted.Render(" · ")
	lines := []string{facts[0]}
	for _, f := range facts[1:] {
		if last := len(lines) - 1; last > 0 || lipgloss.Width(" "+lines[last]+sep+f) <= w {
			lines[last] += sep + f
		} else {
			lines = append(lines, f)
		}
	}
	lines = lines[:min(len(lines), max(rows, 0))]
	for i, l := range lines {
		lines[i] = fit(" "+l, w)
	}
	return lines
}

// screenLines are chatLines, each closed so the pane's styles stop at its
// end.
func (m Model) screenLines(w, h int) []string {
	lines := m.chatLines(w, h)
	for i, l := range lines {
		lines[i] = l + ansi.ResetStyle
	}
	return lines
}

// chatScreen is the live view on an upright phone: the whole screen under a
// title row that goes back to Sessions and says a toast, over the key
// line, with a step that asks to confirm on top. Its title, hairline and
// key line always show: a screen under 4 rows has no room for the agent's
// lines.
func (m Model) chatScreen(w int) drawn {
	st := m.st
	s, _ := m.selectedSession()
	title := " " + st.accent.Render("‹ Sessions") + st.muted.Render(" · ")
	head := m.chatTitle(s)
	if room := w - lipgloss.Width(title); lipgloss.Width(head) > room {
		head = ansi.Truncate(s.Name, max(room, 1), "…") // the state, then the name, give way
	}
	title += st.accent.Render(head)
	if m.toast != "" { // as the header says it, where there is one
		title = spread(title, st.accent.Render(ansi.Truncate(m.toast, max(w/2, 1), "…"))+" ", w)
	}
	title = fit(title, w)
	_, rows := m.chatPlacement(w)
	sc := drawn{lines: []string{title}, bodyTop: 1, listW: w, dockAt: -1, actKeysAt: -1, chatFull: true, chatRows: rows, chatAt: 1, chatW: w}
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
// not import): it leaves the live view, and the stand-in, for Sessions.
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

// chatBackWait is how long after ctrl+a an m leaves the stand-in.
const chatBackWait = time.Second

// chatBacks tells if k is the m of ctrl+a m, whose ctrl+a came at armed.
func (m Model) chatBacks(k tea.KeyPressMsg, armed time.Time) bool {
	return !armed.IsZero() && bkey.Matches(k, m.keys.chat.BackEnd) && m.now().Sub(armed) <= chatBackWait
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
	return m.keys.chat.Back.Help().Key
}

// standInHint is the stand-in's key line while its session waits at a menu:
// it is answered as a real pane is, with the belt.
const standInHint = "type 2 to answer"

// viewFull tells if the live view takes the whole screen: opened on one
// column, with room for its question.
func (m Model) viewFull() bool {
	place, _ := m.chatPlacement(max(m.width, minWidth))
	return m.viewOpen && place == chatFull
}

// openStandIn is the demo's stand-in for the pane of a home session: its
// live view, full-screen on one column, takes the keys and forwards them
// to the demo's world. Outside the demo m is as it was.
func (m Model) openStandIn(pane string) Model {
	s, ok := m.sessionIn(m.opt.Host, pane)
	if m.opt.Demo == "" || !ok {
		return m
	}
	if m.tab != tabSessions {
		m.setTab(tabSessions)
	}
	m.doorPick, m.recPick, m.ghostPick, m.chose, m.selPID, m.selHost = "", "", 0, true, s.PID, s.Host
	m.viewOpen, m.standIn, m.flash = true, true, ""
	return m
}

// leaveStandIn goes back from the stand-in to Sessions, on the session that
// waits next, as a jump back from a real pane does.
func (m Model) leaveStandIn() Model {
	s, _ := m.selectedSession()
	m.viewOpen, m.standIn = false, false
	m.keysOut.queue = nil // typed at a screen that is gone; the key in flight still answers
	return m.nextAfter(s.Pane)
}

// closeView is the way back from the live view: the stand-in's, or the
// read-only view's, which keeps the selection.
func (m Model) closeView() Model {
	if m.standIn {
		return m.leaveStandIn()
	}
	m.viewOpen = false
	return m
}

// keepView closes the live view once Sessions or the selected session's
// pane on this host is gone, or the screen has no room for its question:
// the list's opened row shows the question, and the keys are the console's,
// never a screen's the person cannot see. A new tap or Enter opens it again.
// A session that ended shows the list again, and the key line says why.
func (m Model) keepView() Model {
	if !m.viewOpen {
		return m
	}
	s, ok := m.selectedSession()
	place, _ := m.chatPlacement(max(m.width, minWidth))
	switch {
	case !ok || m.tab != tabSessions || s.Pane == "" || s.Host != m.opt.Host || place == chatNone:
		m.viewOpen, m.standIn = false, false
	case m.screen.Pane == s.Pane && m.screen.Gone:
		m.viewOpen, m.standIn, m.flash = false, false, "That session ended."
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
	if bkey.Matches(k, m.keys.chat.Back) {
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

// liveKeyLine is the key line of the full-screen live view and of the
// stand-in: the way in, or how to answer, and the way back.
func (m Model) liveKeyLine(w int) (string, []span) {
	st := m.st
	s, _ := m.selectedSession()
	type part struct {
		text string
		key  string // the key a tap on it presses; "" none
	}
	in := m.keys.main.Open.Keys()[0]
	lead := part{st.accent.Render(in) + st.muted.Render(" goes into "+s.Name), in}
	back := part{st.accent.Render(m.keys.main.Back.Help().Key) + st.muted.Render(" back"), tapBack}
	switch {
	case m.standIn && m.atMenu(s):
		lead = part{st.text.Render(standInHint), ""}
	case m.standIn:
		lead = part{st.text.Render("keys go to " + s.Name), ""}
	}
	if m.standIn {
		back.text = st.accent.Render(m.backKeys()) + st.muted.Render(" back")
	}
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

// wheelView scrolls the live view back through the service's scroll-back:
// the wheel goes as the keys act, and opens nothing.
func (m Model) wheelView(up bool) (tea.Model, tea.Cmd) {
	s, ok := m.selectedSession()
	if !ok || s.Pane == "" || s.Host != m.opt.Host {
		return m, nil
	}
	text := proto.WheelDown
	if up {
		text = proto.WheelUp
	}
	send := m.sendKey(s, text)
	return m, send
}

// keysOut are the keys the console sends, in order: one goes at a time,
// and the next after the service answers it. Each send runs on its own, so
// two fast keys sent at once could reach the agent swapped (#342).
type keysOut struct {
	conn  transport.Conn // the connection the key in flight went on
	busy  bool
	queue []proto.Act
}

// maxKeysAhead is how many keys wait behind the one in flight.
const maxKeysAhead = 64

// sendKey sends text to the pane of s as the keys act, for the screen of it
// the console shows. Not m.send: a key takes no number from the acts the
// builder sees, and its answer is no toast.
func (m *Model) sendKey(s sample.Session, text string) tea.Cmd {
	if m.conn == nil || m.lost {
		return nil
	}
	act := proto.Act{RecID: proto.KeysPrefix + s.Pane, Text: text, PID: s.PID, Start: s.Start}
	if m.screen.Pane == s.Pane {
		act.Seen = m.screen.At
	}
	if m.keysOut.conn != m.conn { // a new connection: no answer comes for the old one's keys
		m.keysOut = keysOut{conn: m.conn}
	}
	if m.keysOut.busy {
		if len(m.keysOut.queue) >= maxKeysAhead {
			m.flash = "not sent: " + strconv.Itoa(maxKeysAhead) + " keys still wait for " + s.Name
			return nil
		}
		m.keysOut.queue = append(m.keysOut.queue, act)
		return nil
	}
	m.keysOut.busy = true
	return sendKeyAct(m.conn, act)
}

// nextKey sends the key that waits behind the one the service answered.
func (m *Model) nextKey() tea.Cmd {
	if len(m.keysOut.queue) == 0 {
		m.keysOut.busy = false
		return nil
	}
	act := m.keysOut.queue[0]
	m.keysOut.queue = m.keysOut.queue[1:]
	return sendKeyAct(m.keysOut.conn, act)
}

func sendKeyAct(c transport.Conn, act proto.Act) tea.Cmd {
	return func() tea.Msg {
		if err := c.Send(proto.KindAct, keyActID+strings.TrimPrefix(act.RecID, proto.KeysPrefix), act); err != nil {
			return lostMsg{err, c}
		}
		return nil
	}
}
