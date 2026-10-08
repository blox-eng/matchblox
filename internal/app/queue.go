package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

var queueWord = map[string]string{
	queue.StatePermission: "asks",
	queue.StateQuestion:   "waits",
	queue.StateFinished:   "done",
}

// queueWordOf is the word of a queue row: a turn that ended under 100%
// progress, with no question, stopped before its task was done: paused.
func (m Model) queueWordOf(it queue.Item) string {
	if it.State == queue.StateFinished {
		if s, ok := m.sessionIn(it.Pane); ok && s.Progress != nil && s.Progress.Pct < 100 {
			return "paused"
		}
	}
	return queueWord[it.State]
}

// stateWord is a session's state in Sessions: busy; for a session that
// waits for the person, the word of its queue row; else its age.
func (m Model) stateWord(s sample.Session) string {
	if age := s.Age(); age != "idle" {
		return age
	}
	for _, it := range m.queue {
		if it.Pane == s.Pane {
			return m.queueWordOf(it)
		}
	}
	return "idle"
}

func (m Model) queueSelIndex() int {
	for i, it := range m.queue {
		if it.Pane == m.queuePane {
			return i
		}
	}
	return 0
}

// queueIndex is the selected row of the Queue tab: the open doors, the
// queue items, then the recommendations under them. A door that folds
// passes the selection to the next door. Nothing picked: the first agent
// that waits, else the first door.
func (m Model) queueIndex() int {
	nd := len(m.doors)
	for i, d := range m.doors {
		if d.ID == m.doorPick {
			return i
		}
	}
	if nd > 0 && m.doorPick != "" {
		return min(m.doorAt, nd-1)
	}
	if nd > 0 && m.queuePane == "" && m.recPick == "" && len(m.queue) == 0 {
		return 0
	}
	if m.recPick != "" || len(m.queue) == 0 {
		for i, r := range m.recs {
			if r.ID == m.recPick || m.recPick == "" {
				return nd + len(m.queue) + i
			}
		}
	}
	return nd + m.queueSelIndex()
}

func (m Model) selectedQueue() (queue.Item, bool) {
	if i := m.queueIndex() - len(m.doors); i >= 0 && i < len(m.queue) {
		return m.queue[i], true
	}
	return queue.Item{}, false
}

const colWord = 7

func (m Model) queuePanel(w int) body {
	st := m.st
	var b body
	m.doorsSection(&b, w)
	b.add(-1, "", st.label.Render(fmt.Sprintf(" %d WAITING FOR YOU", len(m.queue))))
	if len(m.queue) == 0 {
		b.add(-1, st.faint.Render(m.nothing(" nothing waits for you")))
	}
	nd := len(m.doors)
	sel := m.queueIndex() - nd
	narrow := layout(w) == Narrow
	for i, it := range m.queue {
		word := st.text.Render(pad(m.queueWordOf(it), colWord))
		if it.State == queue.StatePermission {
			word = st.accent.Render(pad(queueWord[it.State], colWord))
		}
		where := place(it.Target, it.Pane)
		if i == sel {
			where = "▌" + where
		}
		tail := st.muted.Render(it.LastLine)
		switch {
		case it.FromPane:
			tail = st.faint.Render("from the pane: ") + tail // first: a long line is cut at the end
		case it.Estimated:
			tail = st.faint.Render("estimated")
		}
		if s, ok := m.sessionIn(it.Pane); ok {
			if pr := m.prOf(s.Cwd); pr != nil {
				tail = st.accent.Render("#"+strconv.Itoa(pr.Number)) + "  " + tail
			}
		}
		if s, ok := m.sessionIn(it.Pane); ok && s.Progress != nil {
			tail = m.progressCell(s.Progress, 5) + "  " + tail
		}
		idle := st.muted.Render(pad(sample.Human(m.now().Sub(it.Since)), colIdle))
		lines := []string{" " + m.queueCell(it) + word + idle + st.text.Render(pad(it.Name, colName)) + st.muted.Render(pad(where, colPane)) + tail}
		if narrow {
			// A phone: who waits on the first line; where, and what it
			// said last, on the second.
			lines = []string{" " + m.queueCell(it) + word + idle + st.text.Render(it.Name),
				"   " + st.muted.Render(where) + "  " + tail}
		}
		b.addRow(nd+i, i == sel, w, st, lines...)
	}
	m.recsSection(&b, w)
	return b
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
	q := strings.ToLower(m.filter)
	if m.tab == tabPanes && q != "" {
		rows = slices.DeleteFunc(rows, func(r paneRow) bool {
			return !strings.Contains(strings.ToLower(r.target+" "+r.id+" "+r.what+" "+r.path), q)
		})
	}
	slices.SortStableFunc(rows, m.paneSort.panes)
	return rows
}

func (m Model) panesPanel(w int) body {
	st := m.st
	rows := m.paneRows()
	var b body
	b.add(-1, "", st.label.Render(fmt.Sprintf(" %d PANES", len(rows))+m.sortNote(w, m.paneSort, paneCols)))
	narrow := layout(w) == Narrow
	if !narrow {
		head, _ := m.paneHeader()
		b.add(headerRow, st.label.Render(fit(head, w)))
	}
	if len(rows) == 0 {
		b.add(-1, st.faint.Render(m.nothing(" no tmux panes")))
	}
	sel := min(m.paneSel, max(len(rows)-1, 0))
	for i, r := range rows {
		where := place(r.target, r.id)
		if i == sel {
			where = "▌" + where
		}
		lines := []string{" " + st.muted.Render(pad(where, colPane)) + st.text.Render(pad(r.what, colName)) + st.muted.Render(tilde(r.path))}
		if narrow {
			lines = []string{" " + st.muted.Render(pad(where, colPane)) + st.text.Render(r.what), "   " + st.muted.Render(tilde(r.path))}
		}
		b.addRow(i, i == sel, w, st, lines...)
	}
	return b
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

// answerTarget is the pane `a` types into: a session in the queue that
// waits for an answer. At a permission prompt the Enter of an answer would
// approve it, so the person goes to the pane instead (why says so).
func (m Model) answerTarget() (pane, why string) {
	switch m.tab {
	case tabQueue:
		if it, ok := m.selectedQueue(); ok {
			pane = it.Pane
		}
	case tabSessions:
		if s, ok := m.selected(); ok {
			pane = s.Pane
		}
	}
	if pane == "" {
		return "", ""
	}
	for _, it := range m.queue {
		if it.Pane != pane {
			continue
		}
		if it.State == queue.StatePermission {
			return "", "a permission prompt is answered in its pane: Enter goes there"
		}
		return pane, ""
	}
	return "", "the session in " + pane + " does not wait for an answer"
}

func trimLast(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return string(r[:len(r)-1])
}
