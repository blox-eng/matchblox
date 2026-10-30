package app

import (
	"fmt"
	"slices"
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
	if it.State == queue.StateFinished && it.Pane != "" {
		s, ok := m.sessionIn(it.Host, it.Pane)
		switch {
		case ok && s.Stopped():
			return "out" // nothing to answer before the plan's limit resets
		case ok && s.Progress != nil && s.Progress.Pct < 100:
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
	if s.Stopped() {
		return "out"
	}
	if it, ok := m.waitOf(s); ok && s.Ask != nil {
		return "asks" // the question tool's menu: its hook calls it a permission
	} else if ok {
		return m.queueWordOf(it)
	}
	return "idle"
}

// paneRow is one tmux pane of a host, with an agent or not.
type paneRow struct {
	host, id, target, what, path string
}

func (m Model) paneRows() []paneRow {
	var rows []paneRow
	for _, s := range m.snap.Sessions {
		rows = append(rows, paneRow{s.Host, s.Pane, s.Target, s.Name, s.Cwd})
	}
	for _, p := range m.snap.IdlePanes {
		rows = append(rows, paneRow{m.opt.Host, p.Pane, p.Target, p.Command, p.Path})
	}
	for _, l := range m.peers {
		if l.have {
			for _, p := range l.st.IdlePanes {
				rows = append(rows, paneRow{l.host, p.Pane, p.Target, p.Command, p.Path})
			}
		}
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
	narrow := m.layoutAt(w) == Narrow
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
		path := st.muted.Render(tilde(r.path))
		if m.hostCount() > 1 {
			path = st.text.Render(m.nameOf(r.host)) + st.faint.Render(" · ") + path
		}
		lines := []string{" " + st.muted.Render(pad(where, colPane)) + st.text.Render(pad(r.what, colName)) + path}
		if narrow {
			lines = []string{" " + st.muted.Render(pad(where, colPane)) + st.text.Render(r.what), "   " + path}
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

func trimLast(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return string(r[:len(r)-1])
}
