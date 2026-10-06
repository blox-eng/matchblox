package app

import (
	"fmt"
	"sort"

	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

// A match for each session (spec 0003 §4.5): lit while the agent works,
// unlit while it waits for the person, burnt at the compact limit. Burnt
// comes first. The state word always stays next to it.
type matchKind int

const (
	matchLit matchKind = iota
	matchUnlit
	matchBurnt
)

var matchGlyph = map[matchKind]string{matchLit: "●", matchUnlit: "●", matchBurnt: "◌"}

func (m Model) matchStyle(k matchKind) lipgloss.Style {
	switch k {
	case matchLit:
		return m.st.flame
	case matchUnlit:
		return m.st.accent
	}
	return m.st.faint
}

func (m Model) burnt(s sample.Session) bool {
	return s.Context == "known" && m.compactAt() > 0 && s.ContextPct >= m.compactAt()
}

func (m Model) matchOf(s sample.Session) matchKind {
	switch {
	case m.burnt(s):
		return matchBurnt
	case s.Busy:
		return matchLit
	}
	return matchUnlit
}

// queueMatch is unlit (the session waits), or burnt when its context is full.
func (m Model) queueMatch(it queue.Item) matchKind {
	for _, s := range m.snap.Sessions {
		if s.Pane == it.Pane && m.burnt(s) {
			return matchBurnt
		}
	}
	return matchUnlit
}

func (m Model) match(k matchKind) string { return m.matchStyle(k).Render(matchGlyph[k]) + " " }

var queueWord = map[string]string{
	queue.StatePermission: "asks",
	queue.StateQuestion:   "waits",
	queue.StateFinished:   "done",
}

func (m Model) queueSelIndex() int {
	for i, it := range m.queue {
		if it.Pane == m.queuePane {
			return i
		}
	}
	return 0
}

func (m Model) selectedQueue() (queue.Item, bool) {
	if len(m.queue) == 0 {
		return queue.Item{}, false
	}
	return m.queue[m.queueSelIndex()], true
}

const colWord = 6

func (m Model) queuePanel(w int) []string {
	st := m.st
	lines := []string{"", st.label.Render(fmt.Sprintf(" %d WAITING FOR YOU", len(m.queue)))}
	if len(m.queue) == 0 {
		return append(lines, st.faint.Render(" nothing waits for you"))
	}
	sel := m.queueSelIndex()
	for i, it := range m.queue {
		word := st.text.Render(pad(queueWord[it.State], colWord))
		if it.State == queue.StatePermission {
			word = st.accent.Render(pad(queueWord[it.State], colWord))
		}
		where := place(it.Target, it.Pane)
		if i == sel {
			where = "▌" + where
		}
		tail := st.muted.Render(it.LastLine)
		if it.Estimated {
			tail = st.faint.Render("estimated")
		}
		line := " " + m.match(m.queueMatch(it)) + word +
			st.muted.Render(pad(sample.Human(m.now().Sub(it.Since)), colIdle)) +
			st.text.Render(pad(it.Name, colName)) + st.muted.Render(pad(where, colPane)) + tail
		if i == sel {
			line = st.selected.Render(fit(line, w))
		}
		lines = append(lines, fit(line, w))
	}
	if m.input != nil {
		lines = append(lines, "", fit(" "+st.label.Render("ANSWER ")+st.muted.Render(m.input.pane+" ")+
			st.text.Render(m.input.text)+st.accent.Render("▏"), w),
			st.faint.Render(" ⏎ review  esc cancel"))
	}
	return lines
}

// paneRow is one tmux pane, with an agent or not.
type paneRow struct {
	id, target, what, path string
}

func (m Model) paneRows() []paneRow {
	var rows []paneRow
	for _, s := range m.snap.Sessions {
		rows = append(rows, paneRow{s.Pane, s.Target, s.Name, s.Cwd})
	}
	for _, p := range m.snap.IdlePanes {
		rows = append(rows, paneRow{p.Pane, p.Target, p.Command, p.Path})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].target < rows[j].target })
	return rows
}

func (m Model) panesPanel(w int) []string {
	st := m.st
	rows := m.paneRows()
	lines := []string{"", st.label.Render(fmt.Sprintf(" %d PANES", len(rows))),
		st.label.Render(fit(" "+pad("TMUX", colPane)+pad("RUNS", colName)+"PATH", w))}
	sel := min(m.paneSel, max(len(rows)-1, 0))
	for i, r := range rows {
		where := place(r.target, r.id)
		if i == sel {
			where = "▌" + where
		}
		line := " " + st.muted.Render(pad(where, colPane)) + st.text.Render(pad(r.what, colName)) + st.muted.Render(tilde(r.path))
		if i == sel {
			line = st.selected.Render(fit(line, w))
		}
		lines = append(lines, fit(line, w))
	}
	return lines
}

func (m Model) selectedPane() (paneRow, bool) {
	rows := m.paneRows()
	if len(rows) == 0 {
		return paneRow{}, false
	}
	return rows[min(m.paneSel, len(rows)-1)], true
}

// answerInput is the one line the person types for `a`.
type answerInput struct {
	pane, text string
}

// answerSteps is what the person confirms; the service builds the same
// steps with panes.Send and runs its own.
func answerSteps(pane, text string) [][]string {
	return [][]string{
		{"tmux", "send-keys", "-t", pane, "-l", "--", text},
		{"tmux", "send-keys", "-t", pane, "Enter"},
	}
}

func (m Model) answerTarget() (string, bool) {
	switch m.tab {
	case tabQueue:
		if it, ok := m.selectedQueue(); ok && it.Pane != "" {
			return it.Pane, true
		}
	case tabSessions:
		if s, ok := m.selected(); ok && s.Pane != "" {
			return s.Pane, true
		}
	}
	return "", false
}

func trimLast(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return string(r[:len(r)-1])
}
