package app

import (
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/logos"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/sample"
)

// watchActID starts the id of a watch act; the pane follows, so a refusal says which watch it was for.
const watchActID = "watch:"

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

// agentMenuCut is a screen without the agent's own question: from the rule
// over the question tool's header (its ☐, ☒ or ✔ tabs) down. Claude Code
// draws more rules inside its menu, so the last rule is not the start. A
// screen with no such header stays whole.
func agentMenuCut(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if isRule(lines[i]) && questionHeader(lines[i+1:]) {
			return strings.Join(lines[:i], "\n")
		}
	}
	return text
}

// questionHeader is the first line that is not blank, when it holds a tab
// of the question tool.
func questionHeader(lines []string) bool {
	for _, l := range lines {
		plain := strings.TrimSpace(ansi.Strip(l))
		if plain == "" {
			continue
		}
		return strings.ContainsAny(plain, "☐☒✔")
	}
	return false
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
	return m.screen.Cols > 0 && m.screen.Cols == w && m.screen.Rows <= h && m.screen.Up == 0
}

// sizeDwell is how long the chat's session stays selected before the
// agent's window takes the chat's size, unless the chat takes the keys:
// a cursor that passes a row resizes no agent.
const sizeDwell = time.Second

// sizeDueMsg comes when a selection may have rested sizeDwell.
type sizeDueMsg struct{}

// chatSize is the room the chat gives the agent's screen as the console
// draws it now: its columns, and its rows under the chat's title and strip
// and over its card. 0, 0 where no chat is drawn. The service sizes the
// agent's window to it.
func (m Model) chatSize() (cols, rows int) {
	if m.tab != tabSessions {
		return 0, 0
	}
	w := max(m.width, minWidth)
	_, detailW := m.panes(w)
	switch {
	case detailW > 0: // beside the list, as chatPlacement has it
		s, ok := m.chatShown(detailW)
		if !ok {
			return 0, 0
		}
		room := m.bodyRows()
		cols, rows = detailW-1, room-1-len(m.chatStrip(s, detailW, room-1))
	case m.chatKeys:
		place, full := m.chatPlacement(w)
		if place != chatFull {
			return 0, 0
		}
		cols, rows = w, full
	}
	rows -= len(m.cardLines(cols))
	if cols <= 0 || rows <= 0 {
		return 0, 0
	}
	return cols, rows
}

// watchCmd asks the service for the selected session's screen when it is
// not the one already watched, and for none when the selection has no pane
// or is another host's;
// and for the lines up the chat shows, or the chat's size, when they
// changed. A screen of another pane never stays.
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
	switch {
	case want != m.watching:
		m.up, m.screen, m.liveWait = 0, proto.Screen{}, false // another pane opens live
	case m.up == 0 && m.watchUp > 0:
		m.screen, m.liveWait = proto.Screen{}, true // the history goes; blank until the live screen comes
	}
	if want != m.watching {
		m.watchSince = m.now()
	}
	var size [2]int
	var due tea.Cmd
	if want != "" {
		size[0], size[1] = m.chatSize()
	}
	if wait := m.watchSince.Add(sizeDwell).Sub(m.now()); size != [2]int{} && !m.chatKeys && wait > 0 {
		size = [2]int{} // browsing: no agent redraws for a row the cursor passes
		if !m.sizeTick.Equal(m.watchSince) {
			m.sizeTick = m.watchSince
			due = tea.Tick(wait, func(time.Time) tea.Msg { return sizeDueMsg{} })
		}
	}
	if size == [2]int{} && want == m.watching {
		size = m.watchSize // a chat off screen keeps its size: the agent does not redraw at each tab
	}
	if want == m.watching && m.up == m.watchUp && size == m.watchSize {
		return due
	}
	m.watching, m.watchUp, m.watchSize = want, m.up, size
	if m.conn == nil || m.lost || m.mismatch {
		return due // the service forgot the watch with the connection
	}
	c, up := m.conn, proto.UpPrefix+strconv.Itoa(m.up)
	act := proto.Act{RecID: proto.WatchPrefix + want, Text: up, Cols: size[0], Rows: size[1]}
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

// chatLines is the watched screen fitted to w by h, with the question card
// at its foot, or what stands in its place: that the session ended, or
// blank lines until a screen comes.
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
	card := m.cardLines(w)
	card = card[max(len(card)-h, 0):]
	var fit []string
	if shown {
		text := m.screen.Text
		if len(card) > 0 {
			text = agentMenuCut(text) // the card is the one place to answer, live or up the history
		}
		switch room := h - len(card); {
		case !m.sizedFor(w, room):
			fit = fitScreen(text, w, room)
		case len(card) > 0: // what is left over the agent's menu sits on the card
			fit = asDrawn(text, w, room)
			for len(fit) > 0 && strings.TrimSpace(ansi.Strip(fit[len(fit)-1])) == "" {
				fit = fit[:len(fit)-1]
			}
		default: // top first, as in the agent's own terminal
			fit = asDrawn(text, w, room)
			fit = append(fit, make([]string, room-len(fit))...)
		}
	}
	fit = append(fit, card...)
	return append(lines[:len(lines)-len(fit)], fit...)
}

// chatShown is the session whose chat is the right side of Sessions: the
// selected one, when it has a pane on the home host and the screen has a
// right side. Another host's row is list only: its detail shows there.
func (m Model) chatShown(rightW int) (sample.Session, bool) {
	s, ok := m.selectedSession()
	return s, ok && s.Pane != "" && s.Host == m.opt.Host && m.tab == tabSessions && rightW > 0
}

// chatLeast is the narrowest chat beside the list: the question card reads
// at 44 columns, as on a folded phone.
const chatLeast = 44

// cardLeast is the fewest lines a question card takes: its frame, a
// question of one line, two options and the typing row. Sessions keeps
// that room for the chat of a session that asks nothing yet, so the split
// does not come and go with the selection.
const cardLeast = 6

// chatPlace is where Sessions draws the selected session's chat.
type chatPlace int

const (
	chatNone   chatPlace = iota // no room for its question: the key line says so
	chatBeside                  // the right side of Sessions, beside the list
	chatFull                    // the whole screen, while it has the keys
)

// chatPlacement is the one answer to where the selected session's chat is
// drawn at w columns, and how many rows it has for its title, strip and
// screen: beside the list where Sessions splits, else the whole screen
// where that shows its whole wait, else nowhere.
func (m Model) chatPlacement(w int) (chatPlace, int) {
	if m.tab != tabSessions {
		return chatNone, 0
	}
	if _, right := m.panes(w); right > 0 {
		return chatBeside, max(m.height-5, 0)
	}
	if m.height == 0 || m.askRows(w) <= m.height-3 {
		return chatFull, max(m.height-3, 0)
	}
	return chatNone, 0
}

// oneColumn tells if Sessions is one column at w, with no right side: as
// on a phone, its selected row opens in place and its chat takes the
// screen.
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
// lines on a narrow chat) and its whole wait (its card, or the selected
// one's permission prompt; cardLeast at the least). Sessions splits by it,
// so the split holds as the cursor moves.
func (m Model) chatReserve(right int) int {
	sel, _ := m.selectedSession()
	most := 1 + cardLeast
	for _, s := range m.snap.Sessions {
		if s.Host != m.opt.Host || s.Pane == "" {
			continue
		}
		wait := m.cardHeight(s, right-1)
		if s.Host == sel.Host && s.PID == sel.PID {
			wait = max(wait, m.askRows(right-1))
		}
		if need := max(wait, cardLeast); 3+need > most { // the strip is two lines at most: draw it only where it can matter
			most = max(most, 1+len(m.chatStrip(s, right, 2))+need)
		}
	}
	return most
}

// cardHeight is how many lines s's question card takes w columns wide, at
// its tallest question; 0 when s asks nothing on a card.
func (m Model) cardHeight(s sample.Session, w int) int {
	if s.Ask == nil || w < 8 {
		return 0
	}
	if it, waits := m.waitOf(s); waits && it.State == queue.StatePermission {
		return 0
	}
	iw := min(w, cardMaxW) - 4
	n := 0
	for _, q := range s.Ask.Questions {
		n = max(n, 2+len(wrapPlain(q.Question, iw))+len(q.Options)+1)
	}
	return n
}

// askRows is how many lines w columns wide the chat must show whole for
// its session's wait: the question card, or the permission prompt's box
// from its top down, which holds what the person approves. A prompt whose
// screen has not come counts as cardLeast.
func (m Model) askRows(w int) int {
	if n := len(m.cardLines(w)); n > 0 {
		return n
	}
	s, ok := m.selectedSession()
	if !ok {
		return 0
	}
	if it, waits := m.waitOf(s); !s.Permits && (!waits || it.State != queue.StatePermission) {
		return 0
	}
	if m.screen.Pane != s.Pane || m.screen.Gone {
		return cardLeast
	}
	return len(fitScreen(promptBox(m.screen.Text), w, math.MaxInt))
}

// permitLine is a line of an agent's approval menu: a numbered Yes, or
// OpenCode's words.
var permitLine = regexp.MustCompile(`(?i)^[❯›>]?\s*\d\.\s+yes\b|^permission required\b|^allow once\b`)

// promptBox is the screen from the top of its permission prompt down: the
// rule or box top over the last line of the menu, else the run of lines
// with words that holds it.
func promptBox(text string) string {
	lines := strings.Split(text, "\n")
	at := -1
	for i := len(lines) - 1; i >= 0 && at < 0; i-- {
		if permitLine.MatchString(strings.TrimSpace(strings.Trim(ansi.Strip(lines[i]), " │┃"))) {
			at = i
		}
	}
	if at < 0 {
		return text
	}
	top := -1
	for i := at; i >= 0 && top < 0; i-- {
		plain := strings.TrimSpace(ansi.Strip(lines[i]))
		if plain != "" && strings.Trim(plain, "─━ ") == "" || strings.HasPrefix(plain, "╭") || strings.HasPrefix(plain, "┌") {
			top = i
		}
	}
	if top < 0 { // no rule: the run of lines it ends
		top = at
		for top > 0 && strings.TrimSpace(ansi.Strip(lines[top-1])) != "" {
			top--
		}
	}
	return strings.Join(lines[top:], "\n")
}

// chatSeen tells if the selected session's chat, with the keys, shows its
// whole question: beside the list, or full-screen when it fits.
func (m Model) chatSeen() bool {
	place, _ := m.chatPlacement(max(m.width, minWidth))
	return place != chatNone
}

// tooShort is what the key line says when the screen has no room for the
// chat's question.
func (m Model) tooShort(s sample.Session) string {
	return "too short to show " + s.Name + "'s question · " + m.keys.main.Jump.Help().Key + " goes into its pane"
}

// chatTitle names the chat's session and its state.
func (m Model) chatTitle(s sample.Session) string { return s.Name + " · " + m.wordOf(s) }

// upNote is what the chat's title adds while it shows the history: how far
// up.
func (m Model) upNote() string {
	if n := m.upShown(); n > 0 {
		return " · ↑ " + plural(n, "line") + " up"
	}
	return ""
}

// chatStep is how many lines one turn of the wheel scrolls the chat.
const chatStep = 3

// upShown is how many lines up the screen the chat shows is: as far as
// the history went.
func (m Model) upShown() int {
	if m.up == 0 {
		return 0
	}
	return m.screen.Up
}

// notLive tells if the chat does not show the agent's live screen: it is
// up the history, or waits for the live screen after it. A key then goes
// nowhere.
func (m Model) notLive() bool { return m.up > 0 || m.screen.Up > 0 || m.liveWait }

// scrollChat moves the chat by lines: up shows earlier lines of the
// agent's pane, down goes back toward live. The chat takes the keys: it
// goes back to live when it loses them.
func (m Model) scrollChat(up bool, lines int) Model {
	if !m.chatKeys {
		if m = m.openChat(); !m.chatKeys {
			return m
		}
	}
	if up {
		m.up = min(m.up+lines, proto.MaxUp)
	} else {
		from := m.up
		if m.screen.Up > 0 {
			from = min(from, m.screen.Up) // the history stopped short
		}
		m.up = max(from-lines, 0)
	}
	return m
}

// backToLive is the chat on the agent's live screen again, after a key
// that went nowhere.
func (m Model) backToLive() Model {
	m.up, m.flash = 0, "back to live — the agent did not get that key"
	return m
}

// chatPane is the chat beside the list, w by h: its title, in the accent
// while the chat has the keys and shows the live screen, over the strip
// and the screen.
func (m Model) chatPane(s sample.Session, w, h int) []string {
	ink := m.st.label
	switch {
	case m.notLive():
		ink = m.st.muted
	case m.chatKeys:
		ink = m.st.accent
	}
	lines := []string{fit(" "+ink.Render(m.chatTitle(s)+m.upNote()), w)}
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

// chatScreen is the chat on an upright phone: the whole screen under a
// title row that goes back to Sessions and says a toast, over the key
// line, with a step that asks to confirm on top. Its title, hairline and
// key line always show: a screen under 4 rows has no room for the agent's
// lines.
func (m Model) chatScreen(w int) drawn {
	st := m.st
	s, _ := m.selectedSession()
	ink := st.accent
	if m.notLive() {
		ink = st.muted
	}
	title := " " + st.accent.Render("‹ Sessions") + st.muted.Render(" · ")
	head, note := m.chatTitle(s), m.upNote() // the state, then the name, give way to the note
	if room := w - lipgloss.Width(title) - lipgloss.Width(note); lipgloss.Width(head) > room {
		head = ansi.Truncate(s.Name, max(room, 1), "…")
	}
	title += ink.Render(head + note)
	if m.toast != "" { // as the header says it, where there is one
		title = spread(title, st.accent.Render(ansi.Truncate(m.toast, max(w/2, 1), "…"))+" ", w)
	}
	title = fit(title, w)
	_, rows := m.chatPlacement(w)
	sc := drawn{lines: []string{title}, bodyTop: 1, listW: w, dockAt: -1, actKeysAt: -1, chatFull: true, chatRows: rows}
	sc.lines = append(sc.lines, m.screenLines(w, rows)...)
	m.markCard(&sc, len(sc.lines), 0, w, rows)
	keys, spans := m.keyLine(w)
	sc.keysAt, sc.keys = len(sc.lines)+1, spans
	sc.lines = append(sc.lines, st.hair.Render(strings.Repeat("─", w)), keys)
	if m.consenting() {
		m.overlayConsent(&sc, w)
	}
	return sc
}

// markCard finds the question card at the foot of a chat w columns wide
// and h lines tall, whose last line is above line bottom and whose first
// column is x.
func (m Model) markCard(sc *drawn, bottom, x, w, h int) {
	if s, ok := m.selected(); ok && m.screen.Pane == s.Pane && m.screen.Gone {
		return // the chat says the session ended, and shows no card
	}
	_, press, spans := m.cardRows(w)
	press = press[max(len(press)-h, 0):]
	sc.cardAt, sc.cardX, sc.cardW, sc.cardRows, sc.cardKeys = bottom-len(press), x, min(w, cardMaxW), press, spans
}

// homeKey is the app's home gesture (panes.HomeKey, which the console may
// not import): it gives the keys back to Sessions.
const homeKey = "\x1b_matchblox\x1b\\"

// chatKey is one key the chat sends to an agent: the pane and the agent it
// was pressed for, and when the screen it answers was taken. A card's
// answer waits among the keys as pick, the said.Answer for the session
// named name. A command key that runs one of matchblox's own acts on the
// pane waits as act ("compact:", or "cmd:" with the command's id
// as text).
type chatKey struct {
	pane  string
	pid   int
	start uint64
	text  string
	seen  time.Time
	pick  string
	name  string
	act   string
}

// openChat gives the keys to the chat of the selected session, or of the
// session of a failed step, or says why there is none.
func (m Model) openChat() Model {
	pane, why := m.chatTarget()
	if pane == "" {
		m.flash = why
		return m
	}
	return m.chatOn(pane)
}

// chatTarget is the pane whose chat a opens: the selected session's, when
// it has a pane to type into, or a failed step's. Why says why there is
// none.
func (m Model) chatTarget() (pane, why string) {
	if m.tab == tabStoker {
		return m.stokerChatTarget()
	}
	s, ok := m.selectedSession()
	switch {
	case !ok:
		return "", ""
	case s.Pane == "":
		return "", s.Name + " runs outside tmux: there is no pane to type into"
	case s.Host != m.opt.Host:
		return "", s.Name + " runs on " + m.nameOf(s.Host) + ": " + m.keys.main.Open.Help().Key + " goes into its pane"
	}
	return s.Pane, ""
}

// chatOn is Sessions with the session in pane selected and its chat
// holding the keys. The selection stays on the session while the chat has
// its keys.
func (m Model) chatOn(pane string) Model {
	s, ok := m.sessionIn(m.opt.Host, pane)
	if !ok {
		return m
	}
	cur, _ := m.selectedSession()
	shown := cur.Pane == pane && m.cardDrawn()
	if m.tab != tabSessions {
		m.setTab(tabSessions)
	}
	m.doorPick, m.recPick, m.ghostPick, m.chose, m.selPID, m.selHost = "", "", 0, true, s.PID, s.Host
	if !m.chatSeen() {
		m.chatKeys, m.flash = false, m.tooShort(s)
		return m
	}
	m.chatKeys, m.flash = true, ""
	if !shown || s.Ask != nil && m.cardSeen.askID != s.Ask.ID { // the chat, or its card, was not on screen before
		m.cardSeen = cardSeen{pane: pane, at: m.now()}
		if s.Ask != nil {
			m.cardSeen.askID = s.Ask.ID
		}
		if it, waits := m.waitOf(s); waits {
			m.cardSeen.since = it.Since
		}
	}
	return m
}

// chatHushed tells if k answers the wait of a chat that opened a moment
// ago: an Enter or a digit, which a prompt takes as its answer.
func (m Model) chatHushed(k chatKey) bool {
	if m.cardSeen.pane != k.pane || !m.now().Before(m.cardSeen.at.Add(cardQuiet)) {
		return false
	}
	s, ok := m.sessionIn(m.opt.Host, k.pane)
	if _, waits := m.waitOf(s); !ok || !waits {
		return false
	}
	typed, isText := strings.CutPrefix(k.text, proto.LitPrefix)
	return k.text == enterName || isText && typed != "" && typed[0] >= '0' && typed[0] <= '9'
}

// intoPane is ctrl+o: the selected session's pane, behind a confirm, as
// Enter on a session was before the chat.
func (m Model) intoPane() Model {
	if s, ok := m.selectedSession(); ok && s.Pane != "" {
		m.pending, m.previewTop = m.primary(), 0
	}
	return m
}

// ctrlCWindow is how close two ^C come to stop Claude Code; a held ^C
// lapses after it.
const ctrlCWindow = time.Second

// ctrlCLapsed is the end of the window of the ^C held at at.
type ctrlCLapsed struct{ at time.Time }

// lapseCtrlC drops a held ^C whose window ended, and its hint.
func (m Model) lapseCtrlC(msg ctrlCLapsed) Model {
	if m.ctrlCHeld.pane != "" && m.ctrlCHeld.at.Equal(msg.at) {
		m.ctrlCHeld = ctrlCMark{}
		if strings.HasSuffix(m.flash, " again stops the agent") {
			m.flash = ""
		}
	}
	return m
}

// ctrlCMark names an agent the chat sent ^C to, and when.
type ctrlCMark struct {
	pane string
	pid  int
	at   time.Time
}

func (c ctrlCMark) is(k chatKey) bool { return c.pane != "" && c.pane == k.pane && c.pid == k.pid }

// chatBackWait is how long after ctrl+a an m gives the keys back.
const chatBackWait = time.Second

// chatBacks tells if k is the m of ctrl+a m, whose ctrl+a came at armed.
func (m Model) chatBacks(k tea.KeyPressMsg, armed time.Time) bool {
	return !armed.IsZero() && bkey.Matches(k, m.keys.chat.BackEnd) && m.now().Sub(armed) <= chatBackWait
}

// chatKey is k while the chat has the keys: ctrl+o goes into the pane, the
// question card takes its own keys, and every other key goes to the agent.
// ctrl+a goes too, at once, and arms the way back (Update takes the m). With
// the stoker in charge it stays here: the way back asks no question.
func (m Model) chatKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.flash, m.chatTyped = "", m.now()
	if bkey.Matches(k, m.keys.chat.Back) {
		m.backArmed = m.now()
		if m.takeOverOn(m.opt.Host) {
			return m, nil
		}
	}
	switch {
	case bkey.Matches(k, m.keys.main.Jump):
		return m.intoPane(), nil
	case m.cardTakes(k):
		return m.cardKey(k)
	}
	text, ok := keyName(k)
	if !ok {
		return m, nil
	}
	name, keys := m.pressed(text)
	return m.toAgent(name, keys...)
}

// pressed is what goes to the chat's agent for the texts pressed now: its
// pane and agent, and the screen they answer, fixed at this moment. Name
// is the session's, for a line that says it ended.
func (m Model) pressed(texts ...string) (name string, keys []chatKey) {
	s, ok := m.selectedSession()
	if !ok || s.Pane == "" || s.Host != m.opt.Host {
		return "", nil
	}
	var seen time.Time
	if m.screen.Pane == s.Pane && m.screen.Up == 0 { // no Enter answers the history
		seen = m.screen.At
	}
	for _, t := range texts {
		keys = append(keys, chatKey{pane: s.Pane, pid: s.PID, start: s.Start, text: t, seen: seen})
	}
	return s.Name, keys
}

// heldKeys is the take-over answer's replay of keys pressed in the chat:
// they go to the agent they were pressed for, wherever the selection is.
func (m Model) heldKeys(name string, keys []chatKey) func(Model) (tea.Model, tea.Cmd) {
	return func(m Model) (tea.Model, tea.Cmd) { return m.toAgent(name, keys...) }
}

// chatPaste is a paste in the chat: text to the agent, or onto the card's
// typing row while the card is up. A newline goes as Enter; other control
// characters drop.
func (m Model) chatPaste(text string) (tea.Model, tea.Cmd) {
	m.flash = ""
	m.chatTyped = m.now()
	if s, c, ok := m.cardNow(); ok {
		return m.cardPaste(s, c, text), nil
	}
	name, keys := m.pressed(pasteTexts(text)...)
	if len(keys) == 0 {
		return m, nil
	}
	if m.takeOverOn(m.opt.Host) {
		m.held, m.heldOn = m.heldKeys(name, keys), m.opt.Host
		return m, nil
	}
	return m.toAgent(name, keys...)
}

// commandPrefix starts a command key the phone sends: its id follows, then
// the APC's end (keycmd.Sequence).
const commandPrefix = "\x1b_matchblox;cmd="

// maxCommandID is the longest command id the key line repeats.
const maxCommandID = 32

// minSignalTmux is the oldest tmux that passes the agent signal through to
// the terminal (allow-passthrough).
const minSignalTmux = "3.3"

// tellAgent writes to the terminal, when it changes, which agent the chat
// is on, for the phone's Commands: the command table's id, OtherAgent for
// an agent with no commands, or none for a closed chat. A tmux too old to
// pass it gets none, and the key line says so once.
func (m *Model) tellAgent() tea.Cmd {
	say := m.agentSaid
	switch {
	case m.chatKeys:
		s, _ := m.selectedSession()
		id := keycmd.AgentOf(s.Agent)
		if len(keycmd.ForAgent(id)) == 0 {
			id = keycmd.OtherAgent
		}
		say = keycmd.AgentSignal(id)
	case say != "":
		say = keycmd.AgentSignal("")
	}
	if say == m.agentSaid {
		return nil
	}
	m.agentSaid = say
	cmd := sayAgent(say, m.opt)
	if cmd == nil && m.chatKeys && !m.tmuxSaid {
		m.tmuxSaid, m.flash = true, "COMMANDS · needs tmux "+minSignalTmux
	}
	return cmd
}

// noAgent is the empty agent signal: no chat runs the phone's Commands.
func noAgent(o Options) tea.Cmd { return sayAgent(keycmd.AgentSignal(""), o) }

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

// agentGone is the empty agent signal of a console that goes while the app
// shows an agent's Commands: no chat runs them now.
func (m *Model) agentGone() tea.Cmd {
	if m.agentSaid == "" || m.agentSaid == keycmd.AgentSignal("") {
		return nil
	}
	m.chatKeys = false
	return m.tellAgent()
}

// commandKey runs the phone's command key apc in the chat, behind the
// chat's guards, as a key pressed there. Compact runs matchblox's own
// act on the pane; any other command runs the command act, which
// types its text and Enter at rest, with no question and no draft. Each
// names the agent the chat showed. A command the chat's agent does not
// have sends nothing; an id the table does not know says the host is older.
// A layer on top takes no command: the key line shows that layer.
func (m Model) commandKey(apc string) (tea.Model, tea.Cmd) {
	id, ok := strings.CutPrefix(apc, commandPrefix)
	id, end := strings.CutSuffix(id, "\x1b\\")
	c, known := keycmd.ByID(id)
	if !ok || !end || len(id) > maxCommandID || strings.ContainsFunc(id, unicode.IsControl) {
		return m, nil
	}
	if !known {
		m.flash = "/" + id + " is new to this console: update matchblox on the host"
		return m, nil
	}
	if !m.chatKeys {
		m.flash = "commands run from the chat"
		return m, nil
	}
	if m.focus(tea.KeyPressMsg{}) != layerMain {
		return m, nil // as a paste: a layer on top takes no command
	}
	m, touch := m.touchRest()
	if m.notLive() {
		return m.backToLive(), touch
	}
	m.flash, m.chatTyped = "", m.now()
	s, _ := m.selectedSession()
	if keycmd.AgentOf(s.Agent) != c.Agent {
		m.flash = keycmd.Refusal(c, s.Name, s.Agent)
		return m, touch
	}
	var act, text string
	switch c.Run {
	case "compact":
		act = "compact:"
	case "type":
		act, text = proto.CommandPrefix, c.ID
	default:
		m.flash = "/" + c.Label + " runs no way this console knows"
		return m, touch
	}
	name, keys := m.pressed(text)
	for i := range keys {
		keys[i].act = act
	}
	if m.takeOverOn(m.opt.Host) {
		m.held, m.heldOn = m.heldKeys(name, keys), m.opt.Host
		return m, touch
	}
	next, cmd := m.toAgent(name, keys...)
	return next, tea.Batch(touch, cmd)
}

// pasteTexts are the keys of a paste: its text in proto.LitPrefix chunks
// of at most proto.MaxLit bytes, with Enter for each newline: \n, \r or \r\n, as terminals
// paste a newline as \r.
func pasteTexts(text string) []string {
	var out []string
	var chunk strings.Builder
	flush := func() {
		if chunk.Len() > 0 {
			out = append(out, proto.LitPrefix+chunk.String())
			chunk.Reset()
		}
	}
	var prev rune
	for _, r := range text {
		switch {
		case r == '\n' && prev == '\r': // \r\n is one newline
		case r == '\n' || r == '\r':
			flush()
			out = append(out, enterName)
		case unicode.IsControl(r):
		default:
			if chunk.Len()+utf8.RuneLen(r) > proto.MaxLit {
				flush()
			}
			chunk.WriteRune(r)
		}
		prev = r
	}
	flush()
	return out
}

// toAgent queues keys for the agent they were pressed for, unless there is
// no connection or that agent left. A ^C within a second of the last one
// sent to the same agent waits for a third: two ^C stop Claude Code.
func (m Model) toAgent(name string, keys ...chatKey) (tea.Model, tea.Cmd) {
	if len(keys) == 0 {
		return m, nil
	}
	if m.conn == nil || m.lost {
		m.flash = "not sent, no connection"
		return m, nil
	}
	if !m.agentIn(keys[0]) {
		m.flash = "not sent: " + name + " ended"
		return m, nil
	}
	if slices.ContainsFunc(keys, m.chatHushed) {
		m.flash = askedAgain(name)
		return m, nil
	}
	if k := keys[0]; k.text == ctrlCName {
		if m.ctrlCHeld.is(k) && m.now().Before(m.ctrlCHeld.at.Add(ctrlCWindow)) {
			m.ctrlCHeld = ctrlCMark{}
		} else if last := m.lastCtrlC(k); last != "" {
			at := m.now()
			m.ctrlCHeld = ctrlCMark{pane: k.pane, pid: k.pid, at: at}
			stop := m.keys.chat.Stop.Help().Key
			m.flash = fmt.Sprintf("%s held: %s, %s again stops the agent", stop, last, stop)
			return m, tea.Tick(ctrlCWindow, func(time.Time) tea.Msg { return ctrlCLapsed{at} })
		}
	}
	for _, k := range keys {
		m.queueKey(k)
	}
	cmd := m.sendKey()
	return m, cmd
}

// lastCtrlC says the ^C to k's agent that a new one comes too close to:
// one that waits in the queue, or one sent less than a second ago. Empty
// when there is none.
func (m Model) lastCtrlC(k chatKey) string {
	if slices.ContainsFunc(m.keyQ, func(q chatKey) bool { return q.text == k.text && q.pane == k.pane && q.pid == k.pid }) {
		return "one waits to go"
	}
	if since := m.now().Sub(m.ctrlC.at); m.ctrlC.is(k) && since < ctrlCWindow {
		return fmt.Sprintf("one went %.1f s ago", since.Seconds())
	}
	return ""
}

// agentIn tells if k's agent is still in its pane, in all the service
// sent: a search or the stoker's own row does not hide it.
func (m Model) agentIn(k chatKey) bool {
	return slices.ContainsFunc(m.all.sessions, func(s sample.Session) bool {
		return s.Host == m.opt.Host && s.Pane == k.pane && s.PID == k.pid && s.Start == k.start
	})
}

// queueKey puts k behind the keys that wait. Text typed after text for the
// same agent joins it, up to proto.MaxLit.
func (m *Model) queueKey(k chatKey) {
	q := slices.Clone(m.keyQ)
	if n := len(q); n > 0 {
		last := &q[n-1]
		typed, isText := strings.CutPrefix(k.text, proto.LitPrefix)
		if isText && strings.HasPrefix(last.text, proto.LitPrefix) && last.pane == k.pane && last.pid == k.pid && len(last.text)+len(typed) <= len(proto.LitPrefix)+proto.MaxLit {
			last.text += typed
			m.keyQ = q
			return
		}
	}
	m.keyQ = append(q, k)
}

// sendKey sends the first key that waits when none is in flight: one at a
// time, so they reach the agent in the order they were pressed. A card's
// answer goes as its pick: act.
func (m *Model) sendKey() tea.Cmd {
	if m.keyInFlight != "" || len(m.keyQ) == 0 || m.conn == nil || m.lost {
		return nil
	}
	k := m.keyQ[0]
	m.keyQ = m.keyQ[1:]
	act := proto.Act{RecID: proto.KeysPrefix + k.pane, Text: k.text, PID: k.pid, Start: k.start, Seen: k.seen}
	switch {
	case k.pick != "":
		act = proto.Act{RecID: proto.PickPrefix + k.pane, Which: "secondary", Confirm: "y", Text: k.pick}
	case k.act != "":
		act = proto.Act{RecID: k.act + k.pane, Confirm: "y", PID: k.pid, Start: k.start, Seen: k.seen, Text: k.text}
	}
	cmd := m.send(act)
	m.keyInFlight, m.keyFlying = "a"+strconv.Itoa(m.acts), k
	if k.pick != "" {
		m.cardSent = maps.Clone(m.cardSent)
		if m.cardSent == nil {
			m.cardSent = map[string]string{}
		}
		m.cardSent[m.keyInFlight] = k.name
	}
	if k.text == ctrlCName {
		m.ctrlC = ctrlCMark{pane: k.pane, pid: k.pid, at: m.now()}
	}
	return cmd
}

// keyAnswered is the service's answer to the key in flight: the next key
// goes, or a refusal shows and the keys behind it for its agent drop, as
// they were pressed for a screen that is no longer there. A card's answer
// waiting among them still goes: it names its question itself.
func (m Model) keyAnswered(r proto.Result) (tea.Model, tea.Cmd) {
	m.keyInFlight = ""
	if name, ok := m.cardSent[r.ActID]; ok {
		return m.cardAnswered(name, r)
	}
	if r.Err != "" {
		f := m.keyFlying
		m.keyQ = slices.DeleteFunc(slices.Clone(m.keyQ), func(k chatKey) bool { return k.pick == "" && k.pane == f.pane && k.pid == f.pid })
		m.flash = r.Err
	}
	cmd := m.sendKey()
	return m, tea.Batch(m.recv(), cmd)
}

// chatShutHold is how long after the chat closes by itself the keys still
// typed for it go nowhere, rather than to Sessions' own keys.
const chatShutHold = time.Second

// chatTypingHold is how long after the person's last key in the chat a
// focus from the island or another client does not move it.
const chatTypingHold = 2 * time.Second

// chatJustShut tells if the chat closed by itself a moment ago.
func (m Model) chatJustShut() bool {
	return !m.chatKeys && m.now().Before(m.chatShut.Add(chatShutHold))
}

// keepChat gives the keys back to Sessions when the chat no longer shows
// the session it had (a chat opened just now on another session keeps
// them), and drops the keys of an agent that left. A chat it closes holds
// the keys typed after it for chatShutHold.
func (m Model) keepChat(was Model) Model {
	if m.chatKeys {
		s, ok := m.selectedSession()
		before, _ := was.selectedSession()
		switch {
		case ok && s.Pane != "" && s.Pane == before.Pane && m.screen.Pane == s.Pane && m.screen.Gone:
			m.chatKeys, m.flash = false, "That session ended." // a phone shows the list again: the key line says why
		case !ok || s.Pane == "" || s.Host != m.opt.Host || was.chatKeys && (s.Pane != before.Pane || s.Host != before.Host):
			m.chatKeys = false
		case !m.chatSeen():
			m.chatKeys, m.flash = false, m.tooShort(s) // the keys never go to a screen the person cannot see
		}
		if !m.chatKeys {
			m.chatShut = m.now()
		}
	}
	if !m.chatKeys {
		m.up = 0
	}
	if m.conn == nil || m.lost {
		m.keyQ, m.keyInFlight = nil, ""
		return m
	}
	var gone []chatKey
	q := slices.DeleteFunc(slices.Clone(m.keyQ), func(k chatKey) bool {
		if m.agentIn(k) {
			return false
		}
		gone = append(gone, k)
		return true
	})
	if len(gone) == 0 {
		return m
	}
	name := gone[0].pane
	if i := slices.IndexFunc(was.all.sessions, func(s sample.Session) bool { return s.PID == gone[0].pid }); i >= 0 {
		name = was.all.sessions[i].Name
	}
	m.keyQ, m.flash = q, "not sent: "+name+" ended"
	return m
}

// tapBack is what a tap on the way back presses: no key, as its ctrl+a would
// go to the agent.
const tapBack = "back"

// backKeys is the way back as the key line names it: this machine's tmux
// prefix, read once at the start, then m. Without tmux, or a prefix it can
// name, it is the chat's own ctrl+a m. A console tmux does not know by name
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

// chatKeyLine is the key line while the chat has the keys: whose agent
// they go to, the way back and the way into tmux; or, up the history,
// that a key goes back to live. A hint that does not fit goes whole, by
// worth: the agent's name (the title names it), then the way into tmux;
// the way back stays.
func (m Model) chatKeyLine(w int) (string, []span) {
	st := m.st
	s, _ := m.selectedSession()
	if m.notLive() { // ctrl+o too only goes back to live; the way back leaves
		line := " " + st.text.Render("a key goes back to live")
		if wide := line + st.muted.Render(" · ") + st.accent.Render(m.backKeys()) + st.muted.Render(" "+m.keys.chat.Back.Help().Desc); lipgloss.Width(wide) <= w {
			line = wide
		}
		return fit(line, w), nil
	}
	type part struct {
		text string
		key  string // the key a tap on it presses; "" none
	}
	hint := func(key, desc, press string) part {
		return part{st.accent.Render(key) + st.muted.Render(" "+desc), press}
	}
	name := part{st.text.Render("keys go to " + s.Name), ""}
	jump := m.keys.main.Jump
	back, into := hint(m.backKeys(), m.keys.chat.Back.Help().Desc, tapBack), hint(jump.Help().Key, jump.Help().Desc, jump.Keys()[0])
	sep := st.muted.Render(" · ")
	width := func(ps []part) int {
		n := 1
		for i, p := range ps {
			if i > 0 {
				n += lipgloss.Width(sep)
			}
			n += lipgloss.Width(p.text)
		}
		return n
	}
	parts := []part{name, back, into}
	for _, try := range [][]part{{name, back, into}, {back, into}, {back}} {
		if parts = try; width(try) <= w {
			break
		}
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
