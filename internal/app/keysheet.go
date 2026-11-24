package app

import (
	"slices"
	"strings"
	"unicode"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The ? sheet: every binding of main, board and side from keys.go, the
// one owner, filtered by what the owner types. ⏎ runs the chosen key as
// a tap on the console; esc clears the filter, then closes the sheet.

// keyRow is one binding on the sheet: its help, and the binding ⏎ runs.
type keyRow struct {
	bind bkey.Binding
}

// keyGroup is one layer's rows under a faint head.
type keyGroup struct {
	name string
	rows []keyRow
}

// sheetGroups is main, board and side from k, each binding with help once
// per group. A binding with no help text is skipped: the sheet's test
// names it.
func sheetGroups(k keymaps) []keyGroup {
	layers := []struct {
		name  string
		binds []bkey.Binding
	}{
		{"main", k.main.bindings()},
		{"board", k.board.bindings()},
		{"side", k.side.bindings()},
	}
	out := make([]keyGroup, 0, len(layers))
	for _, l := range layers {
		g := keyGroup{name: l.name}
		seen := map[string]bool{}
		for _, b := range l.binds {
			if b.Help().Desc == "" {
				continue
			}
			id := b.Help().Key + "\x00" + b.Help().Desc
			if seen[id] {
				continue
			}
			seen[id] = true
			g.rows = append(g.rows, keyRow{bind: b})
		}
		out = append(out, g)
	}
	return out
}

// keyMatch: r's help or one of its keys matches q (empty matches all).
func keyMatch(r keyRow, q string) bool {
	if q == "" {
		return true
	}
	q = strings.ToLower(q)
	return strings.Contains(strings.ToLower(r.bind.Help().Desc), q) ||
		strings.Contains(strings.ToLower(r.bind.Help().Key), q) ||
		slices.ContainsFunc(r.bind.Keys(), func(key string) bool { return key == q })
}

// keyHits is every sheet row that matches q, in group order.
func keyHits(k keymaps, q string) []keyRow {
	var out []keyRow
	for _, g := range sheetGroups(k) {
		for _, r := range g.rows {
			if keyMatch(r, q) {
				out = append(out, r)
			}
		}
	}
	return out
}

// openKeys opens the ? sheet over the agent.
func (m Model) openKeys() Model {
	m.over, m.keyQ, m.keyAt = "keys", "", 0
	m.agent.keys = false
	return m
}

// closeKeys closes the ? sheet and clears its filter.
func (m Model) closeKeys() Model {
	m.over, m.keyQ, m.keyAt = "", "", 0
	return m
}

// keysKey is a key while the ? sheet is open: typing filters, ↑↓ choose,
// ⏎ runs the chosen binding, esc clears then closes.
func (m Model) keysKey(k tea.KeyPressMsg) (Model, tea.Cmd) {
	hits := keyHits(m.keys, m.keyQ)
	switch {
	case bkey.Matches(k, m.keys.side.Leave, m.keys.main.Back):
		if m.keyQ != "" {
			m.keyQ, m.keyAt = "", 0
			return m, nil
		}
		return m.closeKeys(), nil
	case bkey.Matches(k, m.keys.main.Open) && len(hits) > 0:
		at := min(max(m.keyAt, 0), len(hits)-1)
		key := hits[at].bind.Keys()[0]
		m = m.closeKeys()
		next, cmd := m.keyed(keyPress(key))
		return next.(Model), cmd
	case bkey.Matches(k, m.keys.main.Up) && len(hits) > 0:
		m.keyAt = (m.keyAt - 1 + len(hits)) % len(hits)
	case bkey.Matches(k, m.keys.main.Down) && len(hits) > 0:
		m.keyAt = (m.keyAt + 1) % len(hits)
	case bkey.Matches(k, m.keys.search.Erase):
		if r := []rune(m.keyQ); len(r) > 0 {
			m.keyQ, m.keyAt = string(r[:len(r)-1]), 0
		}
	case printsText(k) && !strings.ContainsFunc(k.Text, unicode.IsControl):
		if len([]rune(m.keyQ)) < 40 {
			m.keyQ += k.Text
			m.keyAt = 0
		}
	}
	return m, nil
}

// keysLines is the ? sheet's body: the filter, then each matching group
// and its rows, or the miss words.
func (m Model) keysLines() []string {
	st := m.st
	q := m.keyQ
	lines := []string{st.accent.Render("›") + st.text.Render(" "+q) + st.text.Render("▏"), ""}
	hits := keyHits(m.keys, q)
	at := 0
	if len(hits) > 0 {
		at = min(max(m.keyAt, 0), len(hits)-1)
	}
	n := 0
	for _, g := range sheetGroups(m.keys) {
		var rows []keyRow
		for _, r := range g.rows {
			if keyMatch(r, q) {
				rows = append(rows, r)
			}
		}
		if len(rows) == 0 {
			continue
		}
		lines = append(lines, st.faint.Render(strings.ToUpper(g.name)))
		for _, r := range rows {
			mark, ink := " ", st.muted
			if n == at {
				mark, ink = st.accent.Render("▌"), st.text
			}
			key := r.bind.Help().Key
			pad := max(8-lipgloss.Width(key), 1)
			lines = append(lines, mark+" "+st.text.Render(key)+strings.Repeat(" ", pad)+ink.Render(r.bind.Help().Desc))
			n++
		}
		lines = append(lines, "")
	}
	if len(hits) == 0 {
		esc := m.keys.side.Leave.Help().Key
		lines = append(lines, st.faint.Render(`No key does "`+q+`". `)+st.text.Render(esc)+st.faint.Render(" clears it."))
	}
	return lines
}

// keysFoot is the ? sheet's keys in w columns.
func (m Model) keysFoot(w int) string {
	st, sep := m.st, m.st.faint.Render(" · ")
	mk, sk := m.keys.main, m.keys.side
	parts := []string{
		st.text.Render("type") + st.muted.Render(" to find"),
		st.text.Render(mk.Up.Help().Key) + st.muted.Render(" choose"),
		st.text.Render(mk.Open.Help().Key) + st.muted.Render(" runs it"),
		st.text.Render(sk.Leave.Help().Key) + st.muted.Render(" closes"),
	}
	line := strings.Join(parts, sep)
	if lipgloss.Width(line) <= w {
		return line
	}
	return st.text.Render(sk.Leave.Help().Key) + st.muted.Render(" closes")
}
