package app

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/said"
)

// submitRow is what a tap on a multi-select's own Submit or Next row presses:
// that row has no digit.
const submitRow = "submit"

// tickedRow is an option with a tick box: a digit there only toggles it.
var tickedRow = regexp.MustCompile(`^[❯›>\s]*\d+\.\s+\[`)

// tapLapse is the longest a tapped option stays washed when no new screen
// comes.
const tapLapse = time.Second

// tappedOption is the option of pane a tap pressed, washed until the next
// screen.
type tappedOption struct {
	pane  string
	press string
	at    time.Time
}

// tapLapsed ends the wash of the tap made at at.
type tapLapsed struct{ at time.Time }

// menuPlain is a row of the agent's screen in plain text, out of its box.
func menuPlain(line string) string { return strings.Trim(ansi.Strip(line), " │┃") }

// cursorOn tells if a plain row holds a menu's cursor mark.
func cursorOn(plain string) bool { return strings.IndexAny(plain, "❯›>") == 0 }

// menuTargets is what a tap on each row of the screen presses, "" none: the
// menu at the bottom of the screen and nothing above it. It is the last run
// of options numbered down from 1 with no blank row among them, holding the
// one cursor mark a menu has: a list the agent merely prints has no mark and
// is no target. A rule may sit inside the run (Claude Code draws one over
// Chat about this). Under an option, its description and wrapped lines are
// its target too.
func menuTargets(lines []string) []string {
	none := make([]string, len(lines))
	top, bottom, want := -1, -1, 0
	for i := len(lines) - 1; i >= 0 && top < 0; i-- {
		plain := menuPlain(lines[i])
		n, ok := said.OptionNumber(plain)
		switch {
		case !ok && plain == "" && want > 0:
			return none
		case !ok:
		case n < 1 || n > 9 || want > 0 && n != want:
			return none
		default:
			if want == 0 {
				bottom = i
			}
			if want = n - 1; n == 1 {
				top = i
			}
		}
	}
	if top < 0 {
		return none
	}
	press := make([]string, len(lines))
	cur, marks, ticked := "", 0, false
	for i := top; i < len(lines); i++ {
		plain := menuPlain(lines[i])
		if plain == "" || said.MenuHint(plain) {
			break
		}
		if isRule(plain) {
			if i > bottom {
				break
			}
			cur = ""
			continue
		}
		if cursorOn(plain) {
			marks++
		}
		name := strings.TrimLeft(plain, "❯›> ")
		if n, ok := said.OptionNumber(plain); ok {
			cur, ticked = strconv.Itoa(n), ticked || tickedRow.MatchString(plain)
			press[i] = cur
		} else if ticked && (name == "Submit" || name == "Next") {
			press[i], cur = submitRow, ""
		} else {
			press[i] = cur
		}
	}
	if marks != 1 {
		return none
	}
	return press
}

// menuPage names the page of the menu in lines, "" with no menu: the line over
// its first option, and that option. A tick, the cursor or typed text leave it
// as it is; the agent's next question does not.
func menuPage(lines []string) string {
	press := menuTargets(lines)
	top := slices.IndexFunc(press, func(p string) bool { return p != "" })
	if top < 0 {
		return ""
	}
	first := said.OptionLabel(menuPlain(lines[top]))
	for i := top - 1; i >= 0; i-- {
		if plain := menuPlain(lines[i]); plain != "" {
			return plain + "\n" + first
		}
	}
	return first
}

// typingDigit is the digit of the row that takes typed text when the cursor is
// on it, else "": the last option above the menu's inner rule or its Submit
// row. A digit pressed there lands in its field. A menu with neither, as a
// permission prompt, has no such row.
func typingDigit(lines, press []string) string {
	last, cursor, gap := "", "", false
	for i, p := range press {
		switch {
		case p == "":
			gap = last != ""
		case p == submitRow || gap:
			if cursor == last {
				return last
			}
			return ""
		default:
			last = p
			if cursorOn(menuPlain(lines[i])) {
				cursor = p
			}
		}
	}
	return ""
}

// submitTexts are the keys of a tap on a multi-select's Submit row: the
// cursor goes to it from where the screen shows the cursor, one key to a row
// as panes.Pick counts them, then Enter.
func submitTexts(lines, press []string) []string {
	var rows []string
	cursor := 0
	for i, p := range press {
		if p == "" {
			continue
		}
		if !slices.Contains(rows, p) {
			rows = append(rows, p)
		}
		if cursorOn(menuPlain(lines[i])) {
			cursor = len(rows) - 1
		}
	}
	at := slices.Index(rows, submitRow)
	var out []string
	for ; cursor < at; cursor++ {
		out = append(out, keyNames["down"])
	}
	for ; cursor > at; cursor-- {
		out = append(out, keyNames["up"])
	}
	return append(out, enterName)
}

// optionAt is what a tap at ms presses on the agent's menu, and the texts it
// sends: only at a menu wait, on the live screen, across the chat's width.
func (m Model) optionAt(ms tea.Mouse, sc drawn) (press string, texts []string) {
	i := ms.Y - sc.chatAt
	if sc.chatW == 0 || i < 0 || i >= sc.chatRows || ms.X < sc.chatX || m.focus(tea.KeyPressMsg{}) != layerMain {
		return "", nil
	}
	s, ok := m.selectedSession()
	if !ok || s.Host != m.opt.Host || m.screen.Pane != s.Pane || m.screen.Gone || !m.atMenu(s) {
		return "", nil
	}
	lines := m.chatLines(sc.chatW, sc.chatRows)
	targets := menuTargets(lines)
	switch press = targets[i]; press {
	case "":
		return "", nil
	case submitRow:
		return press, submitTexts(lines, targets)
	}
	digit := proto.LitPrefix + press
	switch typingDigit(lines, targets) {
	case "":
		return press, []string{digit}
	case press:
		return "", nil
	}
	return press, []string{keyNames["up"], digit}
}

// tapOption presses a menu's option through the chat's own keys, so every
// guard holds and a refusal says why, as for a key. At night it asks to take
// over first.
func (m Model) tapOption(press string, texts []string) (tea.Model, tea.Cmd) {
	m.flash = ""
	name, keys := m.pressed(texts...)
	if m.takeOverOn(m.opt.Host) {
		m.held, m.heldOn = m.heldKeys(name, keys), m.opt.Host
		return m, nil
	}
	next, cmd := m.toAgent(name, keys...)
	nm := next.(Model)
	if nm.flash != "" {
		return nm, cmd
	}
	at := nm.now()
	nm.tapped = tappedOption{pane: keys[0].pane, press: press, at: at}
	return nm, tea.Batch(cmd, tea.Tick(tapLapse, func(time.Time) tea.Msg { return tapLapsed{at} }))
}

// washTapped draws the tapped option's rows in the selected wash, like a
// button under a finger. Nothing else of the agent's menu changes.
func (m Model) washTapped(lines []string, w int) {
	if s, ok := m.selected(); m.tapped.press == "" || !ok || s.Pane != m.tapped.pane {
		return
	}
	for i, p := range menuTargets(lines) {
		if p == m.tapped.press {
			lines[i] = m.st.selected.Render(fit(ansi.Strip(lines[i]), w))
		}
	}
}
