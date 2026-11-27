package app

import (
	"strings"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// keySheet is the keys sheet open over the list. Nil while it is closed.
// filter narrows the rows. at is the chosen row among the rows that match.
type keySheet struct {
	filter string
	at     int
}

// keyRun is what enter does with a row of the keys sheet.
type keyRun int

const (
	runList  keyRun = iota // the list handles this key, as typed
	runSide                // the desk handles this key, as typed
	runShell               // the shell act, the same one t runs
	runLater               // the key is named; its act is a later task
)

// keyItem is one key the sheet lists. b is the binding that names the key.
type keyItem struct {
	group, does string
	b           bkey.Binding
	run         keyRun
}

// keyItems are the desk's keys, in the sheet's group order.
// New session is N. n stays night, and it is not in this sheet.
// A row's binding names the key. run says what enter does with it.
func (m Model) keyItems() []keyItem {
	sk, mk, bk, dk := m.keys.side, m.keys.main, m.keys.board, m.keys.desk
	return []keyItem{
		{"anywhere", "flip", sk.Flip, runSide},
		{"anywhere", "go in", mk.Jump, runSide},
		{"anywhere", "new", mk.New, runList},
		{"anywhere", "hosts", bk.Host, runList},
		{"anywhere", "back", mk.Back, runList},

		{"the list", "open", sk.Enter, runList},
		{"the list", "choose", sk.Up, runList},
		{"the list", "prev space", bk.SpacePrev, runList},
		{"the list", "next space", bk.SpaceNext, runList},
		{"the list", "shell", dk.Shell, runShell},
		{"the list", "diff", dk.Diff, runList},
		{"the list", "chain", bk.Chain, runList},
		{"the list", "room", bk.Room, runList},
		{"the list", "park", mk.Park, runList},
		{"the list", "close", mk.Second, runList},
		{"the list", "find", mk.Search, runList},
		{"the list", "menu", bk.Menu, runList},
		{"the list", "copy", bk.Copy, runList},

		{"a review", "comment", bk.Copy, runLater},
		{"a review", "next file", bk.SpaceNext, runLater},
		{"a review", "prev file", bk.SpacePrev, runLater},
		{"a review", "send", sk.Enter, runLater},
		{"a review", "chat", mk.Back, runLater},

		{"the shell", "flip", dk.Shell, runLater},
		{"the shell", "the shell's own", dk.ShellEnd, runLater},

		{"⚙", "choose", sk.Up, runLater},
		{"⚙", "open", sk.Enter, runLater},
		{"⚙", "remove", mk.Second, runLater},
		{"⚙", "mark", mk.Mark, runLater},
		{"⚙", "all safe", mk.MarkSafe, runLater},
		{"⚙", "scan", mk.Rename, runLater},
		{"⚙", "sort", mk.Sort, runLater},
		{"⚙", "quit", mk.QuitAgent, runLater},
		{"⚙", "back", mk.Back, runLater},
	}
}

// keyMatches are the rows the filter keeps. An empty filter keeps every row.
// The filter matches what the key does, and the key itself.
func (m Model) keyMatches() []keyItem {
	if m.keySheet == nil {
		return nil
	}
	q := strings.ToLower(m.keySheet.filter)
	var out []keyItem
	for _, it := range m.keyItems() {
		name := strings.ToLower(it.b.Help().Key)
		if q == "" || strings.Contains(strings.ToLower(it.does), q) || strings.Contains(name, q) {
			out = append(out, it)
		}
	}
	return out
}

// keySheetKey is a key while the sheet is open.
// esc clears the filter, then closes. ctrl+q closes.
// Letters narrow the sheet. ↑↓ choose. enter runs the chosen key.
func (m Model) keySheetKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.keySheet == nil {
		return m, nil
	}
	s := *m.keySheet
	m.keySheet = &s
	switch {
	case bkey.Matches(k, m.keys.side.Flip):
		m.keySheet = nil
		return m, nil
	case bkey.Matches(k, m.keys.main.Back):
		if s.filter != "" {
			s.filter, s.at = "", 0
			return m, nil
		}
		m.keySheet = nil
		return m, nil
	case bkey.Matches(k, m.keys.side.Up):
		s.at = max(s.at-1, 0)
		return m, nil
	case bkey.Matches(k, m.keys.side.Down):
		if n := len(m.keyMatches()); n > 0 {
			s.at = min(s.at+1, n-1)
		}
		return m, nil
	case bkey.Matches(k, m.keys.side.Enter):
		return m.runKeyItem()
	case bkey.Matches(k, m.keys.side.Erase):
		if r := []rune(s.filter); len(r) > 0 {
			s.filter = string(r[:len(r)-1])
			s.at = 0
		}
		return m, nil
	case printsText(k):
		s.filter += k.Text
		s.at = 0
		return m, nil
	}
	return m, nil
}

// runKeyItem runs the chosen row and closes the sheet.
// A row with no match does not run.
func (m Model) runKeyItem() (tea.Model, tea.Cmd) {
	items := m.keyMatches()
	if m.keySheet == nil || len(items) == 0 {
		return m, nil
	}
	at := m.keySheet.at
	if at < 0 || at >= len(items) {
		return m, nil
	}
	it := items[at]
	m.keySheet = nil
	press := ""
	if keys := it.b.Keys(); len(keys) > 0 {
		press = keys[0]
	}
	switch it.run {
	case runShell:
		return m.askShell()
	case runSide:
		next, cmd, _ := m.sideKey(keyPress(press))
		return next, cmd
	case runList:
		sess, ok := m.sideChosen()
		return m.listKey(keyPress(press), sess, ok)
	default:
		return m, nil
	}
}

// keySheetLines are the sheet's rows and the index of the chosen one.
// With no match, the only row is the spec's sentence.
func (m Model) keySheetLines(w int) (lines []string, chosen int) {
	if m.keySheet == nil {
		return nil, 0
	}
	matches := m.keyMatches()
	if m.keySheet.filter != "" && len(matches) == 0 {
		msg := `No key does "` + m.keySheet.filter + `". ` + m.keys.main.Back.Help().Key + ` clears it.`
		return []string{fit(m.st.text.Render(msg), w)}, 0
	}
	at := 0
	if n := len(matches); n > 0 {
		at = min(max(m.keySheet.at, 0), n-1)
	}
	title := "keys"
	if f := m.keySheet.filter; f != "" {
		title = "keys  " + f
	}
	lines = append(lines, m.st.accent.Bold(true).Render(title))
	group := ""
	n := 0
	for _, it := range matches {
		if it.group != group {
			group = it.group
			lines = append(lines, m.st.muted.Render(group))
		}
		if n == at {
			chosen = len(lines)
		}
		lines = append(lines, m.keyRow(it, n == at))
		n++
	}
	return lines, chosen
}

func (m Model) keyRow(it keyItem, on bool) string {
	mark := "  "
	if on {
		mark = "▸ "
	}
	text := mark + it.b.Help().Key + "  " + it.does
	if on {
		return m.st.accent.Render(text)
	}
	return m.st.text.Render(text)
}

// paintKeySheet draws the sheet over the list body. The header stays.
// Rows the sheet does not cover stay, dimmed.
func (m Model) paintKeySheet(sc *drawn, w int) {
	rows, chosen := m.keySheetLines(w)
	top := sc.bodyTop
	if top < 0 || top > len(sc.lines) {
		return
	}
	for i := top; i < len(sc.lines); i++ {
		sc.lines[i] = m.st.faint.Render(ansi.Strip(sc.lines[i]))
	}
	room := len(sc.lines) - top
	for i, line := range windowKeyLines(rows, room, chosen) {
		if top+i >= len(sc.lines) {
			break
		}
		sc.lines[top+i] = fit(line, w)
	}
}

func windowKeyLines(rows []string, room, chosen int) []string {
	if room <= 0 || len(rows) == 0 {
		return nil
	}
	if len(rows) <= room {
		return rows
	}
	start := chosen - room/3
	if start < 0 {
		start = 0
	}
	if start > len(rows)-room {
		start = len(rows) - room
	}
	return rows[start : start+room]
}
