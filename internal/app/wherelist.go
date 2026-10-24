package app

import (
	"cmp"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/gitremote"
)

// The where list of N (#174): while where has focus, the builder's repos
// fill the form box, the ones in use first, each with its worktrees under
// it. Typing filters them; ⏎ or tab picks one.

// The kinds of a where row.
const (
	rowLabel = iota // SUGGESTED, ALL · N · by name
	rowRepo
	rowTree  // a worktree, under its repo
	rowPath  // a folder N opened on that is not a known repo
	rowNone  // nothing matches what is typed
	rowClone // a git URL to clone (#176)
	rowHave  // the URL's repo is here already (#176)
)

// whereRepo is a repo the builder has, with its worktrees.
type whereRepo struct {
	path, name, branch string
	main               string // a worktree: its repo's path
	pr                 int
	at                 time.Time // the newest of used and the checkout's own activity
	used               time.Time // the last session matchblox started there
	live               int       // the sessions that work there, its worktrees not counted
	trees              []whereRepo
}

// whereRow is one line of the list.
type whereRow struct {
	kind int
	// repo: a clone row's path is its target, "" while the plan is on its
	// way; a have row's the clone that is there.
	repo whereRepo
	tree string // "", "├─ ", "╰─ "
	text string // a label, a path, a message
	hits []int  // the matched runes of the name (a worktree: of its branch)
}

// path is where a pick of the row starts the session: "" for none.
func (r whereRow) path() string {
	switch r.kind {
	case rowRepo, rowTree, rowHave:
		return r.repo.path
	case rowPath:
		return r.text
	}
	return ""
}

// picks tells if the row is a place to start.
func (r whereRow) picks() bool { return r.path() != "" }

// suggested is how many repos SUGGESTED holds.
const suggested = 5

// whereRepos are the known repos, by name, each path once: the Git tab's,
// the ones found on the host and the ones matchblox started sessions in.
// A worktree goes under its main checkout.
func (m Model) whereRepos() []whereRepo {
	type entry struct {
		r    whereRepo
		main string // a worktree: its main checkout
	}
	known := map[string]*entry{}
	var order []string
	get := func(p string) *entry {
		p = filepath.Clean(p)
		e, ok := known[p]
		if !ok {
			e = &entry{r: whereRepo{path: p, name: filepath.Base(p)}}
			known[p] = e
			order = append(order, p)
		}
		return e
	}
	// within is the known path p is in, nearest first; nil none.
	within := func(p string) *entry {
		for q := filepath.Clean(p); ; q = filepath.Dir(q) {
			if e, ok := known[q]; ok {
				return e
			}
			if q == filepath.Dir(q) {
				return nil
			}
		}
	}
	if m.git != nil {
		for _, r := range m.git.Repos {
			main := get(r.Path).r.path
			for _, wt := range r.Worktrees {
				e := get(wt.Path)
				if e.r.path != main && e.main == "" {
					e.main = main
				}
				if e.r.branch == "" {
					e.r.branch = wt.Branch
				}
				if wt.PR != nil {
					e.r.pr = wt.PR.Number
				}
			}
		}
	}
	if m.found != nil {
		for _, c := range m.found.Repos {
			e := get(c.Path)
			if c.Main != "" && filepath.Clean(c.Main) != e.r.path && e.main == "" {
				e.main = get(c.Main).r.path
			}
			if e.r.branch == "" {
				e.r.branch = c.Branch
			}
			if c.Active.After(e.r.at) {
				e.r.at = c.Active
			}
		}
	}
	for _, u := range m.used.Repos {
		e := within(u.Path)
		if e == nil {
			e = get(u.Path)
		}
		if u.At.After(e.r.used) {
			e.r.used = u.At
		}
		if u.At.After(e.r.at) {
			e.r.at = u.At
		}
	}
	for _, s := range m.snap.Sessions {
		if s.Cwd == "" {
			continue
		}
		if e := within(s.Cwd); e != nil {
			e.r.live++
		}
	}
	trees := map[string][]whereRepo{}
	var repos []whereRepo
	for _, p := range order {
		e := known[p]
		if e.main != "" && known[e.main].main == "" {
			e.r.main = e.main
			trees[e.main] = append(trees[e.main], e.r)
		}
	}
	for _, p := range order {
		e := known[p]
		if e.main != "" && known[e.main].main == "" {
			continue
		}
		e.r.trees = trees[p]
		slices.SortFunc(e.r.trees, func(a, b whereRepo) int {
			return cmp.Or(b.at.Compare(a.at), cmp.Compare(a.branch, b.branch), cmp.Compare(a.path, b.path))
		})
		repos = append(repos, e.r)
	}
	slices.SortFunc(repos, byName)
	return repos
}

func byName(a, b whereRepo) int {
	return cmp.Or(cmp.Compare(a.name, b.name), cmp.Compare(a.path, b.path))
}

// rank is a repo with its worktrees: sessions there, the last session
// matchblox started there, the last activity.
func (r whereRepo) rank() (live int, used, at time.Time) {
	live, used, at = r.live, r.used, r.at
	for _, t := range r.trees {
		live += t.live
		if t.used.After(used) {
			used = t.used
		}
		if t.at.After(at) {
			at = t.at
		}
	}
	return live, used, at
}

// bySuggested is SUGGESTED's order: a repo with a live session, then the
// last session matchblox started, then the repo's own activity, then the
// name.
func bySuggested(a, b whereRepo) int {
	la, ua, aa := a.rank()
	lb, ub, ab := b.rank()
	return cmp.Or(cmp.Compare(min(lb, 1), min(la, 1)), ub.Compare(ua), ab.Compare(aa), byName(a, b))
}

// whereRows are the list's rows for query: with none, SUGGESTED and then
// ALL by name; with one, the repos that match, the best first.
func (m Model) whereRows(query string) []whereRow {
	extra := ""
	if m.form != nil {
		extra = m.form.extra
	}
	if q := strings.TrimSpace(query); q != "" {
		if r, ok := cloneOf(q); ok && m.form != nil {
			rows := m.form.cloneRows(r, q)
			if shorthand(q) { // a bare o/r filters too: the repos that match first
				if match := m.matchRows(q); match[0].kind != rowNone {
					rows = append(match, rows...)
				}
			}
			return rows
		}
		return m.matchRows(q)
	}
	repos := m.wrepos
	if len(repos) == 0 && extra == "" {
		return nil
	}
	rows := []whereRow{{kind: rowLabel, text: "SUGGESTED"}}
	if extra != "" {
		rows = append(rows, whereRow{kind: rowPath, text: extra})
	}
	top := slices.Clone(repos)
	slices.SortStableFunc(top, bySuggested)
	top = top[:min(suggested, len(top))]
	for _, r := range top {
		rows = append(rows, repoRows(r, r.trees, nil, nil)...)
	}
	if rest := len(repos) - len(top); rest > 0 {
		rows = append(rows, whereRow{kind: rowLabel, text: fmt.Sprintf("ALL · %d · by name", rest)})
		for _, r := range repos {
			if !slices.ContainsFunc(top, func(t whereRepo) bool { return t.path == r.path }) {
				rows = append(rows, repoRows(r, r.trees, nil, nil)...)
			}
		}
	}
	return rows
}

// cloneOf is the remote of a git URL typed in where; false for a path
// (from the root, the home or here), which is never a clone.
func cloneOf(q string) (gitremote.Remote, bool) {
	q = strings.TrimSpace(q)
	if q == "" || pathLike(q) {
		return gitremote.Remote{}, false
	}
	return gitremote.Parse(q, "https")
}

// shorthand tells if the git URL q is the bare o/r, which may also be
// typed to filter: no scheme, no gh:, no user@.
func shorthand(q string) bool { return !strings.ContainsAny(q, ":@") }

// cloneRows are the rows of the git URL q: one clone row at once, then
// the service's plan for it: the clones there already, and where a clone
// goes.
func (f *form) cloneRows(r gitremote.Remote, q string) []whereRow {
	p := f.plan
	switch {
	case f.planFor != q || p == nil && f.planErr == "":
		return []whereRow{{kind: rowClone, text: "clone " + r.Slug() + "…"}}
	case p == nil:
		return []whereRow{{kind: rowNone, text: f.planErr}}
	}
	var rows []whereRow
	for _, h := range p.Have {
		rows = append(rows, whereRow{kind: rowHave, repo: whereRepo{path: h}, text: "start in " + tilde(h) + " (already a clone of " + p.Repo + ")"})
	}
	target, text := p.Target, "clone "+p.Repo+" into "+tilde(p.Target)
	if p.Fresh != "" {
		target, text = p.Fresh, "clone anew into "+tilde(p.Fresh)
	}
	if p.Taken != "" {
		text += " (" + tilde(filepath.Join(filepath.Dir(p.Target), path.Base(p.Repo))) + " is " + p.Taken + ")"
	}
	return append(rows, whereRow{kind: rowClone, repo: whereRepo{path: target}, text: text})
}

// repoRows are a repo's row and its worktrees' under it.
func repoRows(r whereRepo, trees []whereRepo, hits []int, treeHits [][]int) []whereRow {
	rows := []whereRow{{kind: rowRepo, repo: r, hits: hits}}
	for i, t := range trees {
		glyph := "├─ "
		if i == len(trees)-1 {
			glyph = "╰─ "
		}
		var h []int
		if treeHits != nil {
			h = treeHits[i]
		}
		rows = append(rows, whereRow{kind: rowTree, repo: t, tree: glyph, hits: h})
	}
	return rows
}

// matchRows are the repos that match q, the best match first; recency
// orders the ones within a tenth of the best score.
func (m Model) matchRows(q string) []whereRow {
	type match struct {
		r        whereRepo
		score    int
		hits     []int
		trees    []whereRepo
		treeHits [][]int
		at       time.Time
	}
	var ms []match
	best := 0
	for _, r := range m.wrepos {
		mt := match{r: r, score: -1}
		mt.score, mt.hits = placeScore(q, r.name, r.path)
		for _, t := range r.trees {
			if s, h := placeScore(q, t.branch, t.path); s >= 0 {
				mt.trees, mt.treeHits = append(mt.trees, t), append(mt.treeHits, h)
				mt.score = max(mt.score, s)
			}
		}
		if mt.score < 0 {
			continue
		}
		_, _, mt.at = r.rank()
		best = max(best, mt.score)
		ms = append(ms, mt)
	}
	if len(ms) == 0 {
		return []whereRow{{kind: rowNone, text: `no repo matches "` + q + `" · ` + m.keys.form.Enter.Help().Key + " uses it as a path"}}
	}
	near := func(s int) bool { return s*10 >= best*9 }
	slices.SortStableFunc(ms, func(a, b match) int {
		na, nb := near(a.score), near(b.score)
		switch {
		case na && nb:
			return cmp.Or(b.at.Compare(a.at), cmp.Compare(b.score, a.score), byName(a.r, b.r))
		case na != nb:
			if na {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(b.score, a.score), b.at.Compare(a.at), byName(a.r, b.r))
	})
	var rows []whereRow
	for _, mt := range ms {
		rows = append(rows, repoRows(mt.r, mt.trees, mt.hits, mt.treeHits)...)
	}
	// A typed path no row is: the path itself first, so ⏎ takes it.
	if pathLike(q) && !slices.ContainsFunc(rows, func(r whereRow) bool { return r.picks() && samePath(r.path(), q) }) {
		rows = append([]whereRow{{kind: rowPath, text: q}}, rows...)
	}
	return rows
}

// placeScore is how well q matches a place: on its name (a worktree: its
// branch), with the name's matched runes, else on its path at half the
// score; a q typed as a path matches the start of the path. -1: no match.
func placeScore(q, name, path string) (int, []int) {
	if pathLike(q) {
		// A typed path means that path: the places in it or under it.
		if inPath(path, q) || inPath(tilde(path), q) {
			return fuzzyRune * len([]rune(q)), nil
		}
		return -1, nil
	}
	s, hits := fuzzy(q, name)
	if s >= 0 {
		return s, hits
	}
	p, _ := fuzzy(q, path)
	if t, _ := fuzzy(q, tilde(path)); t > p {
		p = t
	}
	if p < 0 {
		return -1, nil
	}
	return p / 2, nil
}

// pathLike tells if q is typed as a path: from the root, the home or
// here.
func pathLike(q string) bool {
	return strings.HasPrefix(q, "/") || strings.HasPrefix(q, "~") || strings.HasPrefix(q, ".")
}

// inPath tells if path is the folder q or one under it: /src/api is not
// in /src/api-gateway.
func inPath(path, q string) bool {
	if !strings.HasSuffix(q, "/") {
		if strings.EqualFold(path, q) {
			return true
		}
		q += "/"
	}
	return len(path) >= len(q) && strings.EqualFold(path[:len(q)], q)
}

// samePath tells if the place at path is the folder q names.
func samePath(path, q string) bool {
	q = filepath.Clean(q)
	return strings.EqualFold(path, q) || strings.EqualFold(tilde(path), q)
}

// The weights of fuzzy: each matched rune, and the bonus of one at the
// start of a word or right after the one before.
const (
	fuzzyRune  = 32
	fuzzyStart = 3
	fuzzyRun   = 5
)

// fuzzy scores query as a subsequence of s, ignoring case: each rune
// counts, more at a word start (after / - _ . or a space, or at the start)
// and in a run. score < 0: no match. hits are the matched runes of s.
func fuzzy(query, s string) (score int, hits []int) {
	q := []rune(strings.ToLower(query))
	r := []rune(strings.ToLower(s))
	if len(q) == 0 {
		return 0, nil
	}
	if !subsequence(q, r) {
		return -1, nil
	}
	start := func(j int) bool { return j == 0 || strings.ContainsRune("/-_. ", r[j-1]) }
	const no = -1 << 30
	// at[i][j]: the best score with q[i] on r[j]; from[i][j] the j of q[i-1].
	at := make([][]int, len(q))
	from := make([][]int, len(q))
	for i := range q {
		at[i], from[i] = make([]int, len(r)), make([]int, len(r))
		bestPrev, bestJ := no, -1
		for j := range r {
			if i > 0 && j > 0 && at[i-1][j-1] > bestPrev {
				bestPrev, bestJ = at[i-1][j-1], j-1
			}
			at[i][j] = no
			if r[j] != q[i] {
				continue
			}
			gain := fuzzyRune
			if start(j) {
				gain += fuzzyStart
			}
			switch {
			case i == 0:
				at[i][j], from[i][j] = gain, -1
			case bestPrev > no:
				at[i][j], from[i][j] = bestPrev+gain, bestJ
				if j > 0 && at[i-1][j-1] > no && at[i-1][j-1]+fuzzyRun >= bestPrev {
					at[i][j], from[i][j] = at[i-1][j-1]+gain+fuzzyRun, j-1
				}
			}
		}
	}
	last := len(q) - 1
	end := -1
	for j := range r {
		if at[last][j] > no && (end < 0 || at[last][j] > at[last][end]) {
			end = j
		}
	}
	if end < 0 {
		return -1, nil
	}
	hits = make([]int, len(q))
	for i, j := last, end; i >= 0; i-- {
		hits[i] = j
		j = from[i][j]
	}
	return at[last][end], hits
}

// subsequence tells if q is in r in order: the cheap test before the
// scoring.
func subsequence(q, r []rune) bool {
	i := 0
	for _, c := range r {
		if i < len(q) && c == q[i] {
			i++
		}
	}
	return i == len(q)
}

// whereList tells if the form shows the where list.
func (f *form) whereList() bool { return f.at == fieldWhere && f.listOpen }

// filter is what the list is filtered by: a pick is not a filter.
func (f *form) filter() string {
	if f.picked {
		return f.query
	}
	return f.where
}

// whereSelected is the selected row of the list; the zero row for none.
func (m Model) whereSelected() whereRow {
	if m.form == nil {
		return whereRow{}
	}
	return m.whereSelectedIn(m.form)
}

func (m Model) whereSelectedIn(f *form) whereRow {
	if f.sel < 0 || f.sel >= len(f.rows) {
		return whereRow{}
	}
	return f.rows[f.sel]
}

// refreshWhere rebuilds f's rows and selects the row of path, else the
// first row.
func (m Model) refreshWhere(f *form, path string) {
	m.form = f // whereRows reads its extra
	f.rows = m.whereRows(f.filter())
	f.sel, f.top = firstRow(f.rows), 0
	if path == "" {
		return
	}
	path = filepath.Clean(path)
	if i := slices.IndexFunc(f.rows, func(r whereRow) bool { return r.picks() && filepath.Clean(r.path()) == path }); i >= 0 {
		f.sel = i
	}
}

// firstRow is the first row that is not a label; 0 for none.
func firstRow(rows []whereRow) int {
	if i := slices.IndexFunc(rows, func(r whereRow) bool { return r.kind != rowLabel }); i >= 0 {
		return i
	}
	return 0
}

// openWhere puts the cursor on where with the list open, on the row of
// the path there.
func (m Model) openWhere(f *form) {
	f.at, f.listOpen = fieldWhere, true
	if !f.picked && f.where != "" {
		m.refreshWhere(f, "")
		return
	}
	m.refreshWhere(f, f.where)
}

// stepSel moves the selection by one row, over the labels; repo only
// stops on a repo, wrapping.
func (f *form) stepSel(by int, repo bool) bool {
	n := len(f.rows)
	for i, k := f.sel+by, 0; k < n; i, k = i+by, k+1 {
		if repo {
			i = (i + n) % n
		}
		if i < 0 || i >= n {
			return false
		}
		r := f.rows[i]
		if r.kind != rowLabel && (!repo || r.kind == rowRepo || r.kind == rowPath) {
			f.sel = i
			return true
		}
	}
	return false
}

// pickWhere takes the selected row into where: false when there is
// nothing to take.
func (f *form) pickWhere() bool {
	r := f.rows
	switch {
	case f.sel < len(r) && r[f.sel].picks():
		f.where, f.picked, f.query = r[f.sel].path(), true, ""
	case strings.TrimSpace(f.where) == "":
		return false
	}
	f.listOpen, f.at = false, fieldBranch
	return true
}

// typeWhere is text typed or pasted on where: it drops a pick and filters.
func (m Model) typeWhere(f *form, text string) {
	f.pickOnPlan = false
	if f.picked {
		f.where, f.picked, f.query = "", false, ""
	}
	f.where = cutField(f.where + text)
	f.listOpen = true
	m.refreshWhere(f, "")
}

// cutField is s cut at maxField runes.
func cutField(s string) string {
	if r := []rune(s); len(r) > maxField {
		return string(r[:maxField])
	}
	return s
}

// whereKey is a key on where with the list open; false: not the list's.
func (m Model) whereKey(f *form, k tea.KeyPressMsg) bool {
	fk := m.keys.form
	switch {
	case bkey.Matches(k, fk.Close):
		f.listOpen = false
	case bkey.Matches(k, fk.Next):
		if !f.pickWhere() {
			f.listOpen = false
			f.move(1)
		}
	case bkey.Matches(k, fk.Prev): // no field before where
	case bkey.Matches(k, fk.Down):
		f.stepSel(1, false)
	case bkey.Matches(k, fk.Up):
		f.stepSel(-1, false)
	default:
		return false
	}
	return true
}

// stepRepo is ← → on where: the previous or next repo of the list, its
// path in where; the list stays filtered by what was typed.
func (m Model) stepRepo(f *form, right bool) {
	if !f.listOpen {
		m.openWhere(f)
	}
	by := -1
	if right {
		by = 1
	}
	if !f.stepSel(by, true) {
		return
	}
	if !f.picked {
		f.query, f.picked = f.where, true
	}
	f.where = f.rows[f.sel].path()
}

// listHeight is how many rows the list shows.
func (m Model) listHeight() int { return max(min(12, m.height-10), 3) }

// scrollTop is the first row shown for the selection sel in h rows from
// top: sel shows, and the label over it when it is the first.
func scrollTop(top, sel, h, n int) int {
	switch {
	case sel < top:
		top = sel
	case sel >= top+h:
		top = sel - h + 1
	}
	if top > 0 && top == sel && h > 1 {
		top--
	}
	return max(min(top, n-h), 0)
}

// whereLines are the list's lines in iw columns, and the key a tap on each
// presses.
func (m Model) whereLines(iw int) (lines, press []string) {
	st, f := m.st, m.form
	h := m.listHeight()
	add := func(p, l string) { lines, press = append(lines, l), append(press, p) }
	switch {
	case len(f.rows) == 0:
		add("", st.muted.Render(fit("no repos known yet · type a path, or paste a git URL", iw)))
		if m.found == nil || m.found.Consent == "" {
			add("", st.faint.Render(fit("matchblox scan finds your repos", iw)))
		}
	default:
		top := scrollTop(f.top, f.sel, h, len(f.rows))
		shown := f.rows[top:min(top+h, len(f.rows))]
		tr := m.whereTracks(shown, iw)
		for i, r := range shown {
			add(fmt.Sprintf("row:%d", top+i), m.whereLine(r, top+i == f.sel, tr, iw))
		}
	}
	for len(lines) < h {
		add("", "")
	}
	count := ""
	if strings.TrimSpace(f.filter()) != "" && (len(f.rows) == 0 || !slices.Contains([]int{rowNone, rowClone, rowHave}, f.rows[0].kind)) {
		n := 0
		for _, r := range f.rows {
			if r.kind == rowRepo {
				n++
			}
		}
		count = st.faint.Render(fmt.Sprintf("%d of %d", n, len(m.wrepos)))
	}
	add("", count)
	return lines, press
}

// whereCols are the widths of a row's cells: the name (a worktree: its tree
// and branch), the short path, the branch (a worktree: its pull request),
// the age and the sessions there. 0: not shown.
type whereCols struct{ name, path, branch, age, live int }

// whereCells are a row's cells as plain text.
func (m Model) whereCells(r whereRow) (name, path, branch, age, live string) {
	switch r.kind {
	case rowRepo, rowTree:
	case rowPath:
		return filepath.Base(r.text), tilde(filepath.Dir(r.text)), "", "", m.liveText(m.liveAt(r.text))
	default:
		return r.text, "", "", "", ""
	}
	p := r.repo
	name, path, branch = p.name, tilde(filepath.Dir(p.path)), p.branch
	if branch == "" {
		branch = "detached"
	}
	if r.kind == rowTree {
		name, path, branch = " "+r.tree+cmp.Or(p.branch, "detached"), treeFolder(p), ""
		if p.pr > 0 {
			branch = fmt.Sprintf("#%d", p.pr)
		}
	}
	if !p.at.IsZero() {
		age = whereAge(m.now().Sub(p.at))
	}
	n := p.live
	if r.kind == rowRepo {
		n, _, _ = p.rank() // a repo counts its worktrees' sessions too
	}
	return name, path, branch, age, m.liveText(n)
}

// treeFolder is a worktree's folder, short: from the parent of its repo's
// folder when it is in it.
func treeFolder(t whereRepo) string {
	if rel, err := filepath.Rel(filepath.Dir(t.main), t.path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return tilde(t.path)
}

func (m Model) liveText(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%d live", n)
}

// liveAt counts the sessions in p or under it.
func (m Model) liveAt(p string) int {
	n := 0
	p = filepath.Clean(p)
	for _, s := range m.snap.Sessions {
		if c := filepath.Clean(s.Cwd); s.Cwd != "" && (c == p || strings.HasPrefix(c, p+string(filepath.Separator))) {
			n++
		}
	}
	return n
}

// whereAge is how long ago, as the Sessions tab says it: now under a
// minute.
func whereAge(d time.Duration) string {
	if d < time.Minute {
		return "now"
	}
	return brief(d)
}

// whereTracks fit the cells of rows in iw columns: the path gives way
// first, then the branch, the sessions and the age; the name stays whole
// while it can.
func (m Model) whereTracks(rows []whereRow, iw int) whereCols {
	var t whereCols
	for _, r := range rows {
		if r.kind == rowLabel || r.kind == rowNone {
			continue
		}
		name, path, branch, age, live := m.whereCells(r)
		t.name = max(t.name, lipgloss.Width(name))
		t.path = max(t.path, lipgloss.Width(path))
		t.branch = max(t.branch, min(lipgloss.Width(branch), 16))
		t.age = max(t.age, lipgloss.Width(age))
		t.live = max(t.live, lipgloss.Width(live))
	}
	room := iw - 1 // the selection mark
	used := func() int {
		n := t.name
		for _, w := range []int{t.path, t.branch, t.age, t.live} {
			if w > 0 {
				n += 2 + w
			}
		}
		return n
	}
	if over := used() - room; over > 0 {
		t.path = max(t.path-over, 0)
		if t.path < 6 {
			t.path = 0
		}
	}
	for _, w := range []*int{&t.branch, &t.live, &t.age} {
		if used() > room {
			*w = 0
		}
	}
	t.name = min(t.name, room)
	return t
}

// whereLine is one row of the list on iw columns.
func (m Model) whereLine(r whereRow, selected bool, t whereCols, iw int) string {
	st := m.st
	switch r.kind {
	case rowLabel:
		return st.label.Render(fit(r.text, iw))
	case rowNone:
		return st.muted.Render(fit(r.text, iw))
	}
	name, path, branch, age, live := m.whereCells(r)
	mark := " "
	if selected {
		mark = st.accent.Render("▌")
	}
	cell := func(s string, w int, style lipgloss.Style, cutStart bool) string {
		if w == 0 {
			return ""
		}
		if cutStart && lipgloss.Width(s) > w {
			rs := []rune(s)
			s = "…" + string(rs[len(rs)-(w-1):])
		}
		return "  " + style.Render(fit(s, w))
	}
	line := mark + m.whereName(r, name, t.name) +
		cell(path, t.path, st.muted, true) + cell(branch, t.branch, st.muted, false) +
		cell(age, t.age, st.muted, false) + cell(live, t.live, st.faint, false)
	line = fit(line, iw)
	if selected {
		line = paint(st.selected.Render(line), st.theme.AccentWash, iw)
	}
	return line
}

// whereName is a row's name cell in w columns: the tree in faint, the
// matched runes bold.
func (m Model) whereName(r whereRow, name string, w int) string {
	st := m.st
	glyph := ""
	if r.kind == rowTree {
		glyph = " " + r.tree
		name = strings.TrimPrefix(name, glyph)
	}
	rs := []rune(ansiFit(name, w-lipgloss.Width(glyph)))
	var b strings.Builder
	b.WriteString(st.faint.Render(glyph))
	bold := st.text.Bold(true)
	for i, c := range rs {
		if slices.Contains(r.hits, i) {
			b.WriteString(bold.Render(string(c)))
		} else {
			b.WriteString(st.text.Render(string(c)))
		}
	}
	return fit(b.String(), w)
}

// ansiFit is s cut to w columns with …, unpadded.
func ansiFit(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	return strings.TrimRightFunc(fit(s, w), unicode.IsSpace)
}

// thenLine is the fields after where, folded to one line.
func (m Model) thenLine(iw int) string {
	f := m.form
	line := "then  " + strings.Join([]string{formLabels[fieldBranch], formLabels[fieldName], formLabels[fieldAgent] + " " + f.agent, formLabels[fieldFirst]}, " · ")
	if lipgloss.Width(line) > iw {
		line = "then  " + strings.Join([]string{formLabels[fieldBranch], formLabels[fieldName], formLabels[fieldAgent], formLabels[fieldFirst]}, " · ")
	}
	return m.st.faint.Render(fit(line, iw))
}

// listFoot is the footer with the list open, cut to bw columns: the
// arrows go first.
func (m Model) listFoot(bw int) []footPart {
	st, fk, sep := m.st, m.keys.form, " · "
	verb := "pick"
	if m.whereSelected().kind == rowClone {
		verb = "clone"
	}
	enter, move, repo := st.footFor(fk.Enter, verb, sep), st.footFor(fk.Down, "move", sep), st.footFor(fk.Left, "repo", sep)
	next, keep := st.footFor(fk.Next, "", sep), st.footFor(fk.Close, "keep the path", sep)
	full := []footPart{enter, move, repo, next, keep}
	if m.form.cloneList() && !slices.ContainsFunc(m.form.rows, func(r whereRow) bool { return r.kind == rowRepo }) { // no repo to step to
		full = []footPart{enter, move, next, keep}
	}
	for _, parts := range [][]footPart{full, {enter, move, next, keep}, {enter, move, keep}, {enter, keep}} {
		w := -lipgloss.Width(sep)
		for _, p := range parts {
			w += lipgloss.Width(p.text) + lipgloss.Width(sep)
		}
		if w <= bw-5 {
			return parts
		}
	}
	return []footPart{enter, keep}
}
