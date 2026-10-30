package app

import (
	"cmp"
	"fmt"
	"maps"
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
	"github.com/blox-eng/matchblox/internal/proto"
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

// fitScreen fits a pane's text into w columns and the last h lines: a rule
// is cut, other text wraps between words, and the blank lines at the end go.
func fitScreen(text string, w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
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

// watchCmd asks the service for the selected session's screen when it is
// not the one already watched, and for none when the selection has no pane;
// and for the lines up the chat shows when they changed. A screen of
// another pane never stays.
func (m *Model) watchCmd() tea.Cmd {
	want := ""
	if m.conn != nil && !m.lost && !m.mismatch {
		if s, ok := m.selected(); ok {
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
	if want == m.watching && m.up == m.watchUp {
		return nil
	}
	m.watching, m.watchUp = want, m.up
	if m.conn == nil || m.lost || m.mismatch {
		return nil // the service forgot the watch with the connection
	}
	c, up := m.conn, proto.UpPrefix+strconv.Itoa(m.up)
	return func() tea.Msg { // not m.send: the watch takes no number from the acts the builder sees
		if err := c.Send(proto.KindAct, watchActID+want, proto.Act{RecID: proto.WatchPrefix + want, Text: up}); err != nil {
			return lostMsg{err, c}
		}
		return nil
	}
}

// chatLines is the watched screen fitted to w by h, with the question card
// at its foot, or what stands in its place: that the session ended, or
// blank lines until a screen comes.
func (m Model) chatLines(w, h int) []string {
	h = max(h, 0)
	lines := make([]string, h)
	s, ok := m.selected()
	if !ok || s.Pane == "" {
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
		if len(card) > 0 && m.screen.Up == 0 {
			text = agentMenuCut(text) // the card is the one place to answer
		}
		fit = fitScreen(text, w, h-len(card))
	}
	fit = append(fit, card...)
	return append(lines[:len(lines)-len(fit)], fit...)
}

// chatShown is the session whose chat is the right side of Sessions: the
// selected one, when it has a pane and the screen has a right side.
func (m Model) chatShown(rightW int) (sample.Session, bool) {
	s, ok := m.selectedSession()
	return s, ok && s.Pane != "" && m.tab == tabSessions && rightW > 0
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

// scrollChat turns the wheel over the chat: up shows earlier lines of the
// agent's pane, down goes back toward live. The chat takes the keys: it
// goes back to live when it loses them.
func (m Model) scrollChat(up bool) Model {
	if !m.chatKeys {
		if m = m.openChat(); !m.chatKeys {
			return m
		}
	}
	if up {
		m.up = min(m.up+chatStep, proto.MaxUp)
	} else {
		from := m.up
		if m.screen.Up > 0 {
			from = min(from, m.screen.Up) // the history stopped short
		}
		m.up = max(from-chatStep, 0)
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
	sc := drawn{lines: []string{title}, bodyTop: 1, listW: w, dockAt: -1, actKeysAt: -1, chatFull: true}
	sc.lines = append(sc.lines, m.screenLines(w, max(m.height-3, 0))...)
	m.markCard(&sc, len(sc.lines), 0, w, max(m.height-3, 0))
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
// pane waits as act ("compact:", "clear:").
type chatKey struct {
	pane string
	pid  int
	text string
	seen time.Time
	pick string
	name string
	act  string
}

// maxLit is the most text one "lit:" key carries; the service takes no more.
const maxLit = 200

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
	}
	return s.Pane, ""
}

// chatOn is Sessions with the session in pane selected and its chat
// holding the keys. The selection stays on the session while the chat has
// its keys.
func (m Model) chatOn(pane string) Model {
	s, ok := m.sessionIn(pane)
	if !ok {
		return m
	}
	cur, _ := m.selectedSession()
	shown := cur.Pane == pane && m.cardDrawn()
	if m.tab != tabSessions {
		m.setTab(tabSessions)
	}
	m.doorPick, m.recPick, m.ghostPick, m.chose, m.selPID = "", "", 0, true, s.PID
	m.chatKeys, m.flash = true, ""
	if !shown || s.Ask != nil && m.cardSeen.askID != s.Ask.ID { // the chat, or its card, was not on screen before
		m.cardSeen = cardSeen{pane: pane, at: m.now()}
		if s.Ask != nil {
			m.cardSeen.askID = s.Ask.ID
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
	s, ok := m.sessionIn(k.pane)
	if _, waits := m.waitOf(s); !ok || !waits {
		return false
	}
	typed, isText := strings.CutPrefix(k.text, "lit:")
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

// ctrlCMark names an agent the chat sent ^C to, and when.
type ctrlCMark struct {
	pane string
	pid  int
	at   time.Time
}

func (c ctrlCMark) is(k chatKey) bool { return c.pane != "" && c.pane == k.pane && c.pid == k.pid }

// chatKey is k while the chat has the keys: ctrl+] gives them back, ctrl+o
// goes into the pane, the question card takes its own keys, and every
// other key goes to the agent.
func (m Model) chatKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.flash = ""
	switch {
	case bkey.Matches(k, m.keys.chat.Back):
		m.chatKeys = false
		return m, nil
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
	if !ok || s.Pane == "" {
		return "", nil
	}
	var seen time.Time
	if m.screen.Pane == s.Pane && m.screen.Up == 0 { // no Enter answers the history
		seen = m.screen.At
	}
	for _, t := range texts {
		keys = append(keys, chatKey{pane: s.Pane, pid: s.PID, text: t, seen: seen})
	}
	return s.Name, keys
}

// heldKeys is the take-over answer's replay of keys pressed in the chat:
// they go to the agent they were pressed for, wherever the selection is.
func (m Model) heldKeys(name string, keys []chatKey) func(Model) (tea.Model, tea.Cmd) {
	return func(m Model) (tea.Model, tea.Cmd) { return m.toAgent(name, keys...) }
}

// chatPaste is a paste in the chat: text to the agent. A newline goes as
// Enter; other control characters drop.
func (m Model) chatPaste(text string) (tea.Model, tea.Cmd) {
	m.flash = ""
	name, keys := m.pressed(pasteTexts(text)...)
	if len(keys) == 0 {
		return m, nil
	}
	if m.night() && !m.stoker.Asked {
		m.held = m.heldKeys(name, keys)
		return m, nil
	}
	return m.toAgent(name, keys...)
}

// commandPrefix starts a command key the phone sends: its id follows, then
// the APC's end (keycmd.Sequence).
const commandPrefix = "\x1b_matchblox;cmd="

// agentNames are the agents of the command table, as a key line names them.
var agentNames = map[string]string{"claude": "Claude Code", "codex": "Codex", "opencode": "OpenCode", "cursor": "Cursor"}

// commandAgent is the command table's id of the agent a session runs.
func commandAgent(agent string) string {
	if agent == sample.CursorAgent {
		return "cursor"
	}
	return agent
}

// commandKey runs the phone's command key apc in the chat, behind the
// chat's guards, as a key pressed there. Compact and clear run matchblox's
// own acts on the pane; any other command types its text and Enter. A
// command the chat's agent does not have sends nothing; an id the table
// does not know does nothing.
func (m Model) commandKey(apc string) (tea.Model, tea.Cmd) {
	id, ok := strings.CutPrefix(apc, commandPrefix)
	id, end := strings.CutSuffix(id, "\x1b\\")
	c, known := keycmd.ByID(id)
	if !ok || !end || !known {
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
	m.flash = ""
	s, _ := m.selectedSession()
	if have := commandAgent(s.Agent); have != c.Agent {
		runs := agentNames[have]
		if runs == "" {
			runs = cmp.Or(s.Agent, "no agent matchblox knows")
		}
		m.flash = fmt.Sprintf("/%s is %s's: %s runs %s", c.Label, agentNames[c.Agent], s.Name, runs)
		return m, touch
	}
	var name string
	var keys []chatKey
	switch c.Run {
	case "compact", "clear":
		name, keys = m.pressed("")
		for i := range keys {
			keys[i].act = c.Run + ":"
		}
	default:
		name, keys = m.pressed("lit:"+c.Text, enterName)
	}
	if m.night() && !m.stoker.Asked {
		m.held = m.heldKeys(name, keys)
		return m, touch
	}
	next, cmd := m.toAgent(name, keys...)
	return next, tea.Batch(touch, cmd)
}

// pasteTexts are the keys of a paste: its text in "lit:" chunks of at most
// maxLit bytes, with Enter for each newline: \n, \r or \r\n, as terminals
// paste a newline as \r.
func pasteTexts(text string) []string {
	var out []string
	var chunk strings.Builder
	flush := func() {
		if chunk.Len() > 0 {
			out = append(out, "lit:"+chunk.String())
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
			if chunk.Len()+utf8.RuneLen(r) > maxLit {
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
		if m.ctrlCHeld.is(k) {
			m.ctrlCHeld = ctrlCMark{}
		} else if last := m.lastCtrlC(k); last != "" {
			m.ctrlCHeld = ctrlCMark{pane: k.pane, pid: k.pid, at: m.now()}
			stop := m.keys.chat.Stop.Help().Key
			m.flash = fmt.Sprintf("%s held: %s, %s again stops the agent", stop, last, stop)
			return m, nil
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
	if since := m.now().Sub(m.ctrlC.at); m.ctrlC.is(k) && since < time.Second {
		return fmt.Sprintf("one went %.1f s ago", since.Seconds())
	}
	return ""
}

// agentIn tells if k's agent is still in its pane, in all the service
// sent: a search or the stoker's own row does not hide it.
func (m Model) agentIn(k chatKey) bool {
	return slices.ContainsFunc(m.all.sessions, func(s sample.Session) bool { return s.Pane == k.pane && s.PID == k.pid })
}

// queueKey puts k behind the keys that wait. Text typed after text for the
// same agent joins it, up to maxLit.
func (m *Model) queueKey(k chatKey) {
	q := slices.Clone(m.keyQ)
	if n := len(q); n > 0 {
		last := &q[n-1]
		typed, isText := strings.CutPrefix(k.text, "lit:")
		if isText && strings.HasPrefix(last.text, "lit:") && last.pane == k.pane && last.pid == k.pid && len(last.text)+len(typed) <= len("lit:")+maxLit {
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
	act := proto.Act{RecID: proto.KeysPrefix + k.pane, Text: k.text, PID: k.pid, Seen: k.seen}
	switch {
	case k.pick != "":
		act = proto.Act{RecID: "pick:" + k.pane, Which: "secondary", Confirm: "y", Text: k.pick}
	case k.act != "":
		act = proto.Act{RecID: k.act + k.pane, Confirm: "y"}
	}
	cmd := m.send(act)
	m.keyInFlight = "a" + strconv.Itoa(m.acts)
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
// goes, or a refusal shows and the keys behind it drop, as they were
// pressed for a screen that is no longer there.
func (m Model) keyAnswered(r proto.Result) (tea.Model, tea.Cmd) {
	m.keyInFlight = ""
	if name, ok := m.cardSent[r.ActID]; ok {
		return m.cardAnswered(name, r)
	}
	if r.Err != "" {
		m.keyQ, m.flash = nil, r.Err
		return m, m.recv()
	}
	cmd := m.sendKey()
	return m, tea.Batch(m.recv(), cmd)
}

// keepChat gives the keys back to Sessions when the chat no longer shows
// the session it had (a chat opened just now on another session keeps
// them), and drops the keys of an agent that left.
func (m Model) keepChat(was Model) Model {
	if m.chatKeys {
		s, ok := m.selectedSession()
		before, _ := was.selectedSession()
		switch {
		case ok && s.Pane != "" && s.Pane == before.Pane && m.screen.Pane == s.Pane && m.screen.Gone:
			m.chatKeys, m.flash = false, "That session ended." // a phone shows the list again: the key line says why
		case !ok || s.Pane == "" || was.chatKeys && s.Pane != before.Pane:
			m.chatKeys = false
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

// chatKeyLine is the key line while the chat has the keys: whose agent
// they go to, and the way back and the way into tmux; or, up the history,
// that a key goes back to live.
func (m Model) chatKeyLine(w int) (string, []span) {
	st := m.st
	s, _ := m.selectedSession()
	if m.notLive() { // ctrl+] and ctrl+o too only go back to live
		return fit(" "+st.text.Render("a key goes back to live"), w), nil
	}
	lead := "keys go to " + s.Name + " · "
	line, x := " "+st.text.Render(lead), 1+lipgloss.Width(lead)
	var spans []span
	for i, b := range []bkey.Binding{m.keys.chat.Back, m.keys.main.Jump} {
		if i > 0 {
			line += st.muted.Render(" · ")
			x += lipgloss.Width(" · ")
		}
		part := st.accent.Render(b.Help().Key) + st.muted.Render(" "+b.Help().Desc)
		spans = append(spans, span{x, x + lipgloss.Width(part), b.Keys()[0]})
		line += part
		x += lipgloss.Width(part)
	}
	return fit(line, w), spans
}
