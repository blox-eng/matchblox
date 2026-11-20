package app

import (
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The console says things in one place (#450): a stack of toasts in the
// body's top-right corner. It never takes the key line, and it moves to
// the body's foot when the top holds the chosen row. An event goes after
// noticeLife; an error stays twice as long.

// toast is one thing said, and when it goes.
type toast struct {
	text  string
	err   bool
	until time.Time
}

// sayMax is the most toasts the stack holds; an older one gives way.
const sayMax = 3

// sayEndsMsg comes when a toast's life ends: the stack drops the old ones.
type sayEndsMsg struct{}

// sayEnds is the command that says a toast ended. It runs on the
// console's clock, as noticeEnds does.
var sayEnds = func(d time.Duration) tea.Cmd { return tea.Tick(d, func(time.Time) tea.Msg { return sayEndsMsg{} }) }

// say shows what happened as a toast. It is also the last thing said:
// m.flash, which a key clears.
func (m *Model) say(text string) tea.Cmd {
	m.flash = text
	return m.stack(text, false)
}

// sayError shows a failure, with its fix, as a toast that stays longer.
func (m *Model) sayError(text string) tea.Cmd {
	m.flash = text
	return m.stack(text, true)
}

// stack puts text on top of the stack, once.
func (m *Model) stack(text string, err bool) tea.Cmd {
	if text == "" {
		return nil
	}
	life := m.noticeLife()
	if err {
		life *= 2
	}
	kept := slices.DeleteFunc(m.sayingNow(), func(t toast) bool { return t.text == text })
	m.saying = append([]toast{{text: text, err: err, until: m.now().Add(life)}}, kept...)
	if len(m.saying) > sayMax {
		m.saying = m.saying[:sayMax]
	}
	return sayEnds(life)
}

// unsay drops the toast that says text: what it said no longer holds.
func (m *Model) unsay(text string) {
	m.saying = slices.DeleteFunc(m.saying, func(t toast) bool { return t.text == text })
	if m.flash == text {
		m.flash = ""
	}
}

// sayingNow are the toasts whose life has not ended.
func (m Model) sayingNow() []toast {
	return slices.DeleteFunc(slices.Clone(m.saying), func(t toast) bool { return !m.now().Before(t.until) })
}

// sayUnder says a standing error as an error toast under the ones shown
// now: it is not the last thing said, and the message's own words stay
// on top.
func (m *Model) sayUnder(text string) {
	if text == "" || m.sayingIt(text) {
		return
	}
	now := m.sayingNow()
	m.saying = append(now, toast{text: text, err: true, until: m.now().Add(2 * m.noticeLife())})
	if len(m.saying) > sayMax { // the standing error gives way: its ⚠ mark still counts it
		m.saying = m.saying[:sayMax]
	}
}

// sayEnded drops the toasts whose life ended. The last thing said goes
// with its toast: it never comes back on another line.
func (m *Model) sayEnded() {
	now := m.sayingNow()
	for _, t := range m.saying {
		if !slices.Contains(now, t) && m.flash == t.text {
			m.flash = ""
		}
	}
	m.saying = now
}

// askingAgain: m.flash is a question that waits for a key, the tap again, not
// a thing said. It keeps the key line. The take-over question has its own.
func (m Model) askingAgain() bool {
	return m.armed != "" && m.flash != "" && !m.sayingIt(m.flash)
}

// sayingIt: a toast says text now, so no other place says it again.
func (m Model) sayingIt(text string) bool {
	return text != "" && slices.ContainsFunc(m.sayingNow(), func(t toast) bool { return t.text == text })
}

// sayWidth is the stack's width in w cells: half a wide screen, all of a
// narrow one.
func sayWidth(w int) int {
	if w < 60 {
		return w
	}
	return w / 2
}

// sayRows are the stack's lines, right-aligned in w cells.
func (m Model) sayRows(w int) []string {
	tw := sayWidth(w)
	var out []string
	for _, t := range m.sayingNow() {
		mark, ink := "  ", m.st.accent
		if t.err {
			mark, ink = "⚠ ", m.st.warn
		}
		lines := []string{t.text}
		if ansi.StringWidth(t.text) > tw-4 { // only a text too long for the stack is cut
			lines = upTo(cutNotice(t.text, tw-4), noticeLines, tw-4)
		}
		for i, l := range lines {
			if i > 0 {
				mark = "  "
			}
			out = append(out, ink.Render(" "+mark+l+" "))
		}
	}
	return out
}

// overlaySaying draws the stack over the body: at its top-right, or at its
// foot when the top holds the chosen row. It never covers box, the lines of
// a box the person answers: where neither place is clear, the stack waits.
func (m Model) overlaySaying(sc *drawn, w int, box [2]int) {
	rows := m.sayRows(w)
	room := len(sc.lines) - sc.bodyTop // the body: down to the key line's hair
	if sc.keysAt > 0 {
		room = sc.keysAt - 1 - sc.bodyTop
	}
	if len(rows) == 0 || room <= 0 {
		return
	}
	rows = rows[:min(len(rows), room)]
	tw := 0
	for _, r := range rows {
		tw = max(tw, ansi.StringWidth(r))
	}
	tw = min(tw, w)
	// The last body line holds the list's foot or the chat's line: the
	// stack sits over it only when the body has no other room.
	foot := max(sc.bodyTop+room-1-len(rows), sc.bodyTop)
	at := []int{sc.bodyTop, foot}
	if w < 60 { // a phone: the top holds the pane's title and its way back
		at = []int{foot, sc.bodyTop}
	}
	c := m.chosenLine(sc, w, w-tw)
	onChosen := func(y int) bool { return c >= 0 && sc.bodyTop+c < y+len(rows)+1 && sc.bodyTop+c >= y-1 } // the row's two lines
	onBox := func(y int) bool { return y < box[1] && box[0] < y+len(rows) }
	top := -1
	for _, clear := range []func(int) bool{func(y int) bool { return !onChosen(y) && !onBox(y) }, func(y int) bool { return !onBox(y) }} {
		if i := slices.IndexFunc(at, clear); i >= 0 {
			top = at[i]
			break
		}
	}
	if top < 0 { // a box takes the body: the stack shows when it closes
		return
	}
	sc.sayY0, sc.sayY1, sc.sayX0 = top, top+len(rows), w-tw
	for i, r := range rows {
		y := top + i
		if y >= len(sc.lines) {
			break
		}
		left := fit(ansi.Truncate(sc.lines[y], w-tw, ""), w-tw)
		sc.lines[y] = left + r + strings.Repeat(" ", max(tw-ansi.StringWidth(r), 0))
	}
}

// chosenLine is the body line of the chosen row, when the stack from
// column x0 would cover it; -1 when it would not.
func (m Model) chosenLine(sc *drawn, w, x0 int) int {
	if m.sideActive() {
		box := m.sideBoxAt(w)
		if box.listW == 0 || x0 >= box.listW {
			return -1
		}
		_, hits, _ := m.sideList(box.listW, len(sc.rows))
		k := m.chosenSide()
		for i, h := range hits {
			if h.k == k && !h.head {
				return i
			}
		}
		return -1
	}
	return slices.Index(sc.rows, m.rowIndex())
}
