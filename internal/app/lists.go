package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

// lists is what the service sent, before the search and the sort of the
// open tab. The console draws from the copies in the Model.
type lists struct {
	queue    []queue.Item
	recs     []advice.Rec
	sessions []sample.Session
	orphans  []sample.Orphan
}

// sortBy is a column and its direction: rev is the other way from the
// column's own (busy first, longest wait first, A to Z).
type sortBy struct {
	col int
	rev bool
}

const (
	sortState = iota
	sortIdle
	sortContext
	sortCPU
	sortName
	sortPlace
)

var sessCols = []string{"state", "idle", "context", "cpu", "name", "tmux"}

const (
	paneByPlace = iota
	paneByRuns
	paneByPath
)

var paneCols = []string{"tmux", "runs", "path"}

// headerRow marks the column header line of a body: a tap there sorts.
const headerRow = -2

func (s sortBy) arrow() string {
	if s.rev {
		return "▴"
	}
	return "▾"
}

// refresh draws the lists again from what the service sent: the search of
// the open tab, then the sort.
func (m *Model) refresh() {
	q := strings.ToLower(m.filter)
	keep := func(fields ...string) bool {
		if q == "" {
			return true
		}
		return strings.Contains(strings.ToLower(strings.Join(fields, " ")), q)
	}
	m.queue, m.recs, m.snap.Orphans = m.all.queue, m.all.recs, m.all.orphans
	sessions := slices.Clone(m.all.sessions)
	switch m.tab {
	case tabQueue:
		m.queue = slices.DeleteFunc(slices.Clone(m.all.queue), func(it queue.Item) bool {
			return !keep(it.Name, it.Target, it.Pane, it.LastLine, queueWord[it.State])
		})
		m.recs = slices.DeleteFunc(slices.Clone(m.all.recs), func(r advice.Rec) bool { return !keep(r.Title, r.Evidence) })
	case tabSessions:
		sessions = slices.DeleteFunc(sessions, func(s sample.Session) bool {
			return !keep(s.Name, s.Target, s.Pane, s.Tab, s.Cwd, s.Model)
		})
	case tabProcs:
		m.snap.Orphans = slices.DeleteFunc(slices.Clone(m.all.orphans), func(o sample.Orphan) bool {
			return !keep(strconv.Itoa(o.PID), o.Comm, o.Cmdline, o.Cwd, o.Target)
		})
	}
	waits := map[string]bool{}
	for _, it := range m.all.queue {
		waits[it.Pane] = true
	}
	slices.SortStableFunc(sessions, func(a, b sample.Session) int { return m.sessSort.compare(a, b, waits) })
	m.snap.Sessions = sessions
	if m.git != nil {
		for p := range m.picked {
			if !m.safeWorktree(p) {
				delete(m.picked, p)
			}
		}
	}
}

// compare orders two sessions. waits holds the panes in the queue.
func (s sortBy) compare(a, b sample.Session, waits map[string]bool) int {
	if s.rev {
		a, b = b, a
	}
	desc := func(x, y float64) int { return -cmpF(x, y) }
	c := 0
	switch s.col {
	case sortState:
		// Who works, then who waits for the person, then the newest idle:
		// the oldest sink to the bottom. Age outranks waiting: a session
		// that waits since yesterday is cold, not waiting.
		c = rank(a, waits) - rank(b, waits)
		if c == 0 {
			c = cmpF(float64(a.Idle), float64(b.Idle))
		}
	case sortIdle:
		c = desc(float64(a.Idle), float64(b.Idle))
	case sortContext:
		c = desc(a.ContextPct, b.ContextPct)
	case sortCPU:
		c = desc(a.CPU, b.CPU)
	case sortName:
		c = strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	case sortPlace:
		c = strings.Compare(a.Target, b.Target)
	}
	if c == 0 {
		c = strings.Compare(a.Name, b.Name)
	}
	if c == 0 {
		c = a.PID - b.PID
	}
	return c
}

// rank is the group of a session in the default order.
func rank(s sample.Session, waits map[string]bool) int {
	switch age := s.Age(); {
	case age == "busy":
		return 0
	case age == "idle" && waits[s.Pane]:
		return 1
	case age == "idle":
		return 2
	case age == "cold":
		return 3
	}
	return 4
}

func cmpF(x, y float64) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

func (s sortBy) panes(a, b paneRow) int {
	if s.rev {
		a, b = b, a
	}
	c := 0
	switch s.col {
	case paneByRuns:
		c = strings.Compare(a.what, b.what)
	case paneByPath:
		c = strings.Compare(a.path, b.path)
	}
	if c == 0 {
		c = strings.Compare(a.target, b.target)
	}
	return c
}

// sortable is the sort of the open tab, when it has one.
func (m *Model) sortable() (*sortBy, []string) {
	switch m.tab {
	case tabSessions:
		return &m.sessSort, sessCols
	case tabPanes:
		return &m.paneSort, paneCols
	}
	return nil, nil
}

// sortBy sets the column, or turns it around when it is already the one.
func (m *Model) sortOn(col int) {
	s, names := m.sortable()
	if s == nil {
		return
	}
	if s.col == col {
		s.rev = !s.rev
	} else {
		*s = sortBy{col: col}
	}
	m.flash = "sort: " + names[s.col] + " " + s.arrow()
	m.refresh()
}

// sortNote names the sort where no header shows it (a phone).
func (m Model) sortNote(w int, s sortBy, names []string) string {
	if layout(w) != Narrow {
		return ""
	}
	return "  ·  by " + names[s.col] + " " + s.arrow()
}

// sessHeader is the column header of the Sessions tab, with the sort, and
// where each sortable column starts and ends.
func (m Model) sessHeader(tab int) (string, []colSpan) {
	cols := []struct {
		label string
		w     int
		col   int
	}{
		{"TMUX", colPane + tab, sortPlace}, {"NAME", colName, sortName}, {"STATE", colSt, sortState},
		{"IDLE", colIdle, sortIdle}, {"CONTEXT", colBar + 1 + colPct, sortContext}, {"CPU", colCPU, sortCPU},
		{"DO", colDo + 1, -1},
	}
	return header(cols, m.sessSort, " WORKTREE")
}

func (m Model) paneHeader() (string, []colSpan) {
	return header([]struct {
		label string
		w     int
		col   int
	}{{"TMUX", colPane, paneByPlace}, {"RUNS", colName, paneByRuns}, {"PATH", 30, paneByPath}}, m.paneSort, "")
}

type colSpan struct{ x0, x1, col int }

func header(cols []struct {
	label string
	w     int
	col   int
}, s sortBy, tail string) (string, []colSpan) {
	var b strings.Builder
	var spans []colSpan
	x := 1
	b.WriteString(" ")
	for _, c := range cols {
		label := c.label
		if c.col == s.col && c.col >= 0 {
			label += s.arrow()
		}
		b.WriteString(pad(label, c.w))
		if c.col >= 0 {
			spans = append(spans, colSpan{x, x + c.w, c.col})
		}
		x += c.w
	}
	return strings.TrimRight(b.String(), " ") + tail, spans
}

// tapHeader sorts by the column under x.
func (m Model) tapHeader(w, x int) (tea.Model, tea.Cmd) {
	var spans []colSpan
	switch m.tab {
	case tabSessions:
		tab := 0
		if w >= 100 {
			tab = colTab
		}
		_, spans = m.sessHeader(tab)
	case tabPanes:
		_, spans = m.paneHeader()
	}
	for _, s := range spans {
		if x >= s.x0 && x < s.x1 {
			m.sortOn(s.col)
		}
	}
	return m, nil
}

// search edits the search of the open tab. Every printable key is text.
func (m Model) search(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.searching, m.filter = false, ""
	case "enter":
		m.searching = false
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "backspace":
		m.filter = trimLast(m.filter)
	default:
		if k.Text != "" && len([]rune(m.filter)) < 100 {
			m.filter += k.Text
		}
	}
	m.refresh()
	return m, nil
}

// setTab opens a tab; a search belongs to the tab it was typed on.
func (m *Model) setTab(t int) {
	m.tab, m.filter, m.searching, m.armed = t, "", false, ""
	m.refresh()
}

// nothing is the empty state of a list: the search when there is one.
func (m Model) nothing(text string) string {
	if m.filter != "" {
		return " nothing matches " + m.filter + "  esc clears"
	}
	return text
}

// safeWorktree reads every worktree, not the found ones: a search hides
// rows, not marks.
func (m Model) safeWorktree(path string) bool {
	for _, r := range m.allWorktreeRows() {
		if r.wt.Path == path {
			return r.wt.Safe && len(r.wt.Remove) > 0
		}
	}
	return false
}

// mark adds the selected worktree to the ones x removes, or takes it out.
func (m *Model) mark() {
	wt, ok := m.selectedWorktree()
	switch {
	case !ok:
	case m.picked[wt.Path]:
		delete(m.picked, wt.Path)
	case !wt.Safe:
		m.flash = "not safe to remove: " + unsafeWhy(wt)
	default:
		if m.picked == nil {
			m.picked = map[string]bool{}
		}
		m.picked[wt.Path] = true
	}
}

func unsafeWhy(wt proto.Worktree) string {
	switch {
	case wt.Sessions > 0:
		return "an agent works in it"
	case wt.InUse:
		return "a process is in it"
	case wt.Dirty != 0:
		return "it has changes"
	case !wt.Merged:
		return "its branch is not merged"
	}
	return "the main checkout"
}

// markAllSafe marks every safe worktree, or clears the marks when all of
// them are marked already.
func (m *Model) markAllSafe() {
	var safe []string
	for _, r := range m.worktreeRows() {
		if r.wt.Safe && len(r.wt.Remove) > 0 {
			safe = append(safe, r.wt.Path)
		}
	}
	all := len(safe) > 0 && len(m.picked) == len(safe)
	m.picked = map[string]bool{}
	if all {
		return
	}
	for _, p := range safe {
		m.picked[p] = true
	}
	if len(safe) == 0 {
		m.flash = "no worktree is safe to remove"
	}
}

// removeMarked is x with marks: every marked worktree, behind one typed y.
func (m Model) removeMarked() *action {
	var steps [][]string
	var paths []string
	for _, r := range m.allWorktreeRows() {
		if m.picked[r.wt.Path] {
			steps, paths = append(steps, r.wt.Remove), append(paths, r.wt.Path)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	return &action{label: fmt.Sprintf("remove %d", len(paths)), steps: steps, destructive: true, batch: paths}
}

// batch counts the results of the acts of one confirm.
type batch struct {
	ids                      map[string]bool
	removed, skipped, failed int
	why                      string
}

// sendBatch sends one guarded act for each worktree: the service checks
// each one again and removes only what is still safe.
func (m Model) sendBatch(a action) (tea.Model, tea.Cmd) {
	b := &batch{ids: map[string]bool{}}
	var cmds []tea.Cmd
	for _, p := range a.batch {
		cmds = append(cmds, m.send(proto.Act{RecID: "worktree:" + p, Which: "secondary", Confirm: "y"}))
		b.ids["a"+strconv.Itoa(m.acts)] = true
	}
	m.batch, m.picked = b, nil
	m.flash = fmt.Sprintf("removing %d worktrees…", len(a.batch))
	return m, tea.Batch(cmds...)
}

// tally counts one result of the batch; the last one writes the summary.
func (m *Model) tally(r proto.Result) bool {
	if m.batch == nil || !m.batch.ids[r.ActID] {
		return false
	}
	b := m.batch
	delete(b.ids, r.ActID)
	switch {
	case r.Err != "":
		b.failed++
		b.why = r.Err
	case len(r.Skipped) > 0:
		b.skipped++
		b.why = r.Skipped[len(r.Skipped)-1]
	case len(r.Ran) > 0:
		b.removed++
	}
	if len(b.ids) > 0 {
		return true
	}
	text := fmt.Sprintf("removed %d", b.removed)
	if b.skipped > 0 {
		text += fmt.Sprintf(", skipped %d", b.skipped)
	}
	if b.failed > 0 {
		text += fmt.Sprintf(", failed %d", b.failed)
	}
	if b.why != "" {
		text += ": " + b.why
	}
	m.flash, m.batch = text, nil
	return true
}
