package app

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

// A session's life from the console (#74): N opens New session, r opens
// Rename, A opens Switch agent, x kills (the secondary action of a
// session's row).

// killAnyway is the Text of a kill confirmed a second time (the service's
// KillAnyway).
const killAnyway = "anyway"

// The fields of the New session form, in the order Tab walks them.
const (
	fieldWhere = iota
	fieldBranch
	fieldName
	fieldAgent
	fieldFirst
	fieldFolders
)

var formLabels = [...]string{"where", "branch", "name", "agent", "first", "folders"}

// form is the open New session, Rename, Switch agent or scan folders form.
// Rename has the name only; Switch agent the agent and the handoff (in
// first); the scan's folders the roots.
type form struct {
	rename    bool
	swap      bool   // Switch agent
	pick      bool   // the scan's folders, in roots
	roots     string // space separated
	pane, who string // Rename, Switch agent: the session's pane and its name now
	from      string // Switch agent: the session's agent now (where: its directory)
	at        int    // the field the cursor is on
	where     string
	branch    string
	name      string
	named     bool // the name was typed: it no longer follows where
	agent     string
	first     string
	act, err  string // the act in flight, and why the last one did not go
	// where's list (#174): its rows for the filter, the selected one and
	// the first shown; picked: where holds a pick or a ←→ step, not a
	// filter, and query is what was typed before it; extra: the folder N
	// opened on, when it is not a known repo.
	rows     []whereRow
	sel, top int
	listOpen bool
	picked   bool
	query    string
	extra    string
	// A git URL in where (#176): plan is the service's plan for planFor,
	// planErr why there is none, planAct the clone:plan in flight.
	// cloning is the target of the clone this form started, cloneAct its
	// clone:start or clone:stop in flight; startWhenDone: the final ⏎ came
	// while it ran, and the session starts as startAs when it is done.
	plan          *proto.ClonePlan
	planFor       string
	planErr       string
	planAct       string
	cloning       string
	cloneAct      string
	startWhenDone bool
	startAs       proto.NewSession
}

const maxField = 300

// openNew is N: the New session form, where's list open on the selected
// row's folder (a Git worktree, a session's directory), or the first repo.
func (m Model) openNew() (tea.Model, tea.Cmd) {
	switch {
	case m.conn == nil || m.lost:
		m.flash = "not sent, no connection"
		return m, nil
	case len(m.agents) == 0:
		m.flash = "no agent to start here: install claude, codex, opencode or cursor-agent"
		return m, nil
	}
	f := &form{at: fieldWhere, agent: m.agents[0]}
	if slices.Contains(m.agents, m.lastAgent) {
		f.agent = m.lastAgent
	}
	at := ""
	switch m.tab {
	case tabGit:
		if wt, ok := m.selectedWorktree(); ok {
			at = wt.Path
		}
	case tabSessions:
		if s, ok := m.selectedSession(); ok {
			at = s.Cwd
		}
	}
	f.listOpen = true
	m.refreshWhere(f, at)
	if at != "" && filepath.Clean(m.whereSelectedIn(f).path()) != filepath.Clean(at) {
		f.extra = filepath.Clean(at) // not a known repo: the first row
		m.refreshWhere(f, at)
	}
	m.form, m.pending, m.peek = f, nil, nil
	return m, nil
}

// openRename is r on a session: Rename, its name to edit.
func (m Model) openRename() (tea.Model, tea.Cmd) {
	s, ok := m.selectedSession()
	switch {
	case !ok:
		return m, nil
	case s.Pane == "":
		m.flash = s.Name + " is outside tmux: nothing to rename"
		return m, nil
	case m.conn == nil || m.lost:
		m.flash = "not sent, no connection"
		return m, nil
	}
	name := s.Name
	if proto.CheckName(name) != nil {
		name = proto.NameOf(name)
	}
	m.form, m.pending, m.peek = &form{rename: true, pane: s.Pane, who: s.Name, name: name, named: true, at: fieldName}, nil, nil
	return m, nil
}

// openSwitch is A on a session: Switch agent, the first other agent
// picked and the default handoff to edit or clear.
func (m Model) openSwitch() (tea.Model, tea.Cmd) {
	s, ok := m.selectedSession()
	switch {
	case !ok:
		return m, nil
	case s.Pane == "":
		m.flash = s.Name + " is outside tmux: nothing to switch"
		return m, nil
	case m.conn == nil || m.lost:
		m.flash = "not sent, no connection"
		return m, nil
	}
	f := &form{swap: true, pane: s.Pane, who: s.Name, from: s.Agent, where: s.Cwd, at: fieldAgent, first: proto.DefaultHandoff}
	for _, a := range m.agents {
		if a != s.Agent {
			f.agent = a
			break
		}
	}
	if f.agent == "" {
		m.flash = "no other agent to switch to here"
		return m, nil
	}
	m.form, m.pending, m.peek = f, nil, nil
	return m, nil
}

// switchAction is the plan of a Switch agent form: it waits for a typed y,
// and a second one for a session that works or waits.
func (m Model) switchAction(f form) *action {
	sw := proto.SwitchAgent{Agent: f.agent, Handoff: strings.TrimSpace(f.first)}
	b, _ := json.Marshal(sw)
	sw.Anyway = true
	again, _ := json.Marshal(sw)
	say := fmt.Sprintf("switch %s: %s → %s in %s, %s, then jump there", f.who, f.from, f.agent, f.pane, f.where)
	a := &action{label: "switch", destructive: true, rec: "switch:" + f.pane, text: string(b), againText: string(again), say: say, done: "switched " + f.who + " to " + f.agent}
	if i := slices.IndexFunc(m.snap.Sessions, func(s sample.Session) bool { return s.Pane == f.pane }); i >= 0 {
		a.again = m.againFor(m.snap.Sessions[i], "switches")
	}
	return a
}

// fields are the form's fields, in Tab's order.
func (f *form) fields() []int {
	switch {
	case f.pick:
		return []int{fieldFolders}
	case f.rename:
		return []int{fieldName}
	case f.swap:
		return []int{fieldAgent, fieldFirst}
	}
	return []int{fieldWhere, fieldBranch, fieldName, fieldAgent, fieldFirst}
}

func (f *form) text(field int) *string {
	switch field {
	case fieldWhere:
		return &f.where
	case fieldBranch:
		return &f.branch
	case fieldName:
		return &f.name
	case fieldFirst:
		return &f.first
	case fieldFolders:
		return &f.roots
	}
	return nil
}

// autoName is the name a new session takes when none is typed: the
// worktree's for a branch, else the directory's, as the service makes it.
func (f *form) autoName() string {
	where := f.where
	if f.cloning != "" { // the session starts in the clone
		where = f.cloning
	}
	base := filepath.Base(filepath.Clean(where))
	if where == "" {
		return ""
	}
	if f.branch != "" {
		return proto.NameOf(base + "-" + proto.NameOf(f.branch))
	}
	return proto.NameOf(base)
}

func (f *form) move(by int) {
	fs := f.fields()
	i := slices.Index(fs, f.at)
	f.at = fs[(i+by+len(fs))%len(fs)]
}

// moveTo is move, and back on where the list opens.
func (m Model) moveTo(f *form, by int) {
	f.move(by)
	if f.at == fieldWhere && f.cloning == "" { // no list while it clones
		m.openWhere(f)
	}
}

func (m Model) formKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	f := *m.form
	m.form = &f
	fk := m.keys.form
	if f.whereList() {
		f.err = ""
		if r := m.whereSelectedIn(&f); r.kind == rowClone && bkey.Matches(k, fk.Enter, fk.Next) {
			return m, m.startClone(&f, r)
		}
		if bkey.Matches(k, fk.Enter) && f.pickWhere() {
			return m, nil
		}
		if m.whereKey(&f, k) {
			f.top = scrollTop(f.top, f.sel, m.listHeight(), len(f.rows))
			return m, nil
		}
	}
	switch {
	case bkey.Matches(k, fk.Close):
		if f.cloning != "" { // the first esc stops the clone; the form stays
			return m, m.stopClone(&f)
		}
		m.form = nil // nothing is sent
		return m, nil
	case bkey.Matches(k, fk.Enter):
		return m.formEnter()
	case bkey.Matches(k, fk.Next, fk.Down):
		m.moveTo(&f, 1)
		return m, nil
	case bkey.Matches(k, fk.Prev, fk.Up):
		m.moveTo(&f, -1)
		return m, nil
	}
	f.err = ""
	if f.at == fieldAgent {
		agents := m.formAgents()
		if len(agents) == 0 {
			return m, nil
		}
		i := slices.Index(agents, f.agent)
		switch {
		case bkey.Matches(k, fk.Left):
			f.agent = agents[(i+len(agents)-1)%len(agents)]
		case bkey.Matches(k, fk.Right):
			f.agent = agents[(i+1)%len(agents)]
		}
		return m, nil
	}
	// Editing where stops the clone of the URL there.
	var stop tea.Cmd
	if f.at == fieldWhere && f.cloning != "" && (k.Text != "" || bkey.Matches(k, fk.Erase, fk.Left, fk.Right)) {
		stop = m.stopClone(&f)
	}
	// On where, space is text: only the arrows pick.
	if f.at == fieldWhere && k.Text == "" && bkey.Matches(k, fk.Left, fk.Right) {
		m.stepRepo(&f, bkey.Matches(k, fk.Right))
		f.top = scrollTop(f.top, f.sel, m.listHeight(), len(f.rows))
		return m, stop
	}
	switch {
	case bkey.Matches(k, fk.Erase):
		m.eraseField(&f)
	case k.Text != "" && len([]rune(m.fieldText(&f)+k.Text)) <= maxField:
		m.typeField(&f, k.Text)
	}
	return m, tea.Batch(stop, m.planWhere(&f))
}

// fieldText is the text of the field the cursor is on.
func (m Model) fieldText(f *form) string {
	if t := f.text(f.at); t != nil {
		return *t
	}
	return ""
}

// typeField is text typed or pasted on the field the cursor is on, cut at
// maxField.
func (m Model) typeField(f *form, text string) {
	t := f.text(f.at)
	switch {
	case t == nil:
	case f.at == fieldWhere:
		m.typeWhere(f, text)
	default:
		*t = cutField(*t + text)
		f.named = f.named || f.at == fieldName
	}
}

// eraseField is ⌫: on a pick in where, back to what was typed before it.
func (m Model) eraseField(f *form) {
	t := f.text(f.at)
	switch {
	case t == nil:
		return
	case f.at == fieldWhere && f.picked:
		f.where, f.picked, f.query = f.query, false, ""
	default:
		*t = trimLast(*t)
	}
	switch f.at {
	case fieldWhere:
		f.listOpen = true
		m.refreshWhere(f, "")
	case fieldName:
		f.named = f.name != ""
	}
}

// formPaste is a paste on the open form: its text goes in the field the
// cursor is on as typed, line breaks dropped and the ends trimmed.
func (m Model) formPaste(text string) (tea.Model, tea.Cmd) {
	f := *m.form
	m.form = &f
	text = strings.TrimSpace(strings.NewReplacer("\r\n", "", "\n", "", "\r", "").Replace(text))
	if text == "" || f.text(f.at) == nil {
		return m, nil
	}
	f.err = ""
	var stop tea.Cmd
	if f.at == fieldWhere {
		stop = m.stopClone(&f)
	}
	m.typeField(&f, text)
	return m, tea.Batch(stop, m.planWhere(&f))
}

// formAgents are the agents the form picks from: Switch agent leaves out
// the one the session runs.
func (m Model) formAgents() []string {
	if !m.form.swap {
		return m.agents
	}
	var out []string
	for _, a := range m.agents {
		if a != m.form.from {
			out = append(out, a)
		}
	}
	return out
}

// formEnter sends the form: a refusal stays in it, where it can be fixed.
func (m Model) formEnter() (tea.Model, tea.Cmd) {
	f := *m.form
	m.form = &f
	if m.conn == nil || m.lost {
		f.err = "not sent, no connection"
		return m, nil
	}
	if f.swap {
		return m.swapEnter(f)
	}
	if f.pick {
		roots, err := splitFolders(f.roots)
		if err != nil {
			f.err = err.Error()
			return m, nil
		}
		if len(roots) == 0 {
			f.err = "which folders? ~ is your home"
			return m, nil
		}
		cmd := m.send(proto.Act{RecID: "scan:yes", Confirm: "y", Text: strings.Join(roots, "\n")})
		f.act, f.err = fmt.Sprintf("a%d", m.acts), ""
		return m, cmd
	}
	name := strings.TrimSpace(f.name)
	if f.rename || f.named {
		if err := proto.CheckName(name); err != nil {
			f.err, f.at = err.Error(), fieldName
			return m, nil
		}
	}
	var cmd tea.Cmd
	if f.rename {
		cmd = m.send(proto.Act{RecID: "rename:" + f.pane, Confirm: "y", Text: name})
		m = m.pairActed(action{rec: "rename:" + f.pane})
	} else {
		if strings.TrimSpace(f.where) == "" {
			f.err = "where does it start? type a path"
			if len(m.wrepos) > 0 {
				f.err = "where does it start? a path, or " + m.keys.form.Left.Help().Key + " for a repository"
			}
			m.openWhere(&f)
			return m, nil
		}
		n := f.newSession(strings.TrimSpace(f.where))
		if f.cloning != "" { // it starts when the clone is done
			f.startWhenDone, f.startAs, f.err = true, n, ""
			return m, nil
		}
		if _, ok := cloneOf(f.where); ok && !f.picked {
			// A URL that is not cloned: back to its rows, where ⏎ clones.
			m.openWhere(&f)
			return m, m.planWhere(&f)
		}
		return m, m.sendNew(&f, n)
	}
	f.act, f.err = fmt.Sprintf("a%d", m.acts), ""
	return m, cmd
}

// newSession is the New session form's new:session, in where.
func (f *form) newSession(where string) proto.NewSession {
	n := proto.NewSession{Where: where, Branch: strings.TrimSpace(f.branch), Agent: f.agent, First: strings.TrimSpace(f.first)}
	if f.named {
		n.Name = strings.TrimSpace(f.name)
	}
	return n
}

// sendNew sends n as f's act.
func (m *Model) sendNew(f *form, n proto.NewSession) tea.Cmd {
	b, _ := json.Marshal(n)
	cmd := m.send(proto.Act{RecID: "new:session", Confirm: "y", Text: string(b)})
	f.act, f.err = fmt.Sprintf("a%d", m.acts), ""
	return cmd
}

// The clone in the form (#176): a git URL in where asks the service for a
// plan; ⏎ on its clone row starts the clone, and the builder fills the
// rest while it runs.

// cloneList tells if where's list holds the rows of a git URL.
func (f *form) cloneList() bool {
	_, ok := cloneOf(f.filter())
	return ok
}

// planWhere asks the service where a clone of the git URL in where goes,
// once for each text.
func (m *Model) planWhere(f *form) tea.Cmd {
	q := strings.TrimSpace(f.filter())
	if _, ok := cloneOf(q); f.at != fieldWhere || !ok || q == f.planFor {
		return nil
	}
	f.plan, f.planErr, f.planFor = nil, "", q
	cmd := m.send(proto.Act{RecID: "clone:plan", Text: q})
	f.planAct = fmt.Sprintf("a%d", m.acts)
	return cmd
}

// startClone is ⏎ or tab on a clone row: the clone starts in the service,
// where keeps the URL and the cursor goes to branch. Nothing while the
// plan is on its way.
func (m *Model) startClone(f *form, r whereRow) tea.Cmd {
	if r.kind != rowClone || r.repo.path == "" || f.plan == nil {
		return nil
	}
	b, _ := json.Marshal(proto.CloneStart{URL: f.plan.URL, Target: r.repo.path})
	cmd := m.send(proto.Act{RecID: "clone:start", Text: string(b)})
	f.cloneAct, f.cloning, f.startWhenDone, f.err = fmt.Sprintf("a%d", m.acts), r.repo.path, false, ""
	f.listOpen, f.at = false, fieldBranch
	return cmd
}

// stopClone stops the clone f started, while it runs: the form forgets it
// and the service deletes its partial folder.
func (m *Model) stopClone(f *form) tea.Cmd {
	target := f.cloning
	if target == "" {
		return nil
	}
	f.cloning, f.startWhenDone = "", false
	if c, ok := m.cloneFor(target); ok && (c.Done || c.Err != "") {
		return nil
	}
	cmd := m.send(proto.Act{RecID: "clone:stop", Text: target})
	f.cloneAct = fmt.Sprintf("a%d", m.acts)
	return cmd
}

// cloneFor is the service's clone into target.
func (m Model) cloneFor(target string) (proto.Clone, bool) {
	target = filepath.Clean(target)
	i := slices.IndexFunc(m.clones, func(c proto.Clone) bool { return filepath.Clean(c.Target) == target })
	if i < 0 {
		return proto.Clone{}, false
	}
	return m.clones[i], true
}

// cloneNews is what a snapshot says of the form's clone: a failure is the
// form's error; done, its folder is where, and a session that waited for
// it starts there.
func (m *Model) cloneNews() tea.Cmd {
	f := *m.form
	m.form = &f
	c, ok := m.cloneFor(f.cloning)
	switch {
	case !ok || !c.Done && c.Err == "":
		return nil
	case c.Err != "":
		f.err, f.cloning, f.startWhenDone = c.Err, "", false
		return nil
	}
	f.where, f.picked, f.query, f.planFor, f.cloning = f.cloning, true, "", "", ""
	if !f.startWhenDone {
		return nil
	}
	f.startWhenDone = false
	n := f.startAs
	n.Where = f.where
	return m.sendNew(&f, n)
}

// cloneText is where's line while the clone runs, in w columns: how far
// it is, and that the session starts when it is done.
func (m Model) cloneText(f *form, w int) string {
	repo := ""
	if f.plan != nil {
		repo = f.plan.Repo
	}
	pct := ""
	if c, ok := m.cloneFor(f.cloning); ok {
		repo = cmp.Or(c.Repo, repo)
		if c.Pct >= 0 {
			pct = fmt.Sprintf(" %d%%", c.Pct)
		}
	}
	line := "cloning " + repo + "…" + pct
	if f.startWhenDone {
		const after = "starts when the clone is done"
		for _, l := range []string{line + " · " + after, strings.TrimSpace(pct) + " · " + after, after} {
			if lipgloss.Width(l) <= w && !strings.HasPrefix(l, " ") {
				return l
			}
		}
		line = after
	}
	return fit(line, w)
}

// swapEnter turns the Switch agent form into its plan, checked as the
// service checks it.
func (m Model) swapEnter(f form) (tea.Model, tea.Cmd) {
	if f.agent == "" {
		m.form.err, m.form.at = "which agent? "+m.keys.form.Left.Help().Key+" picks one", fieldAgent
		return m, nil
	}
	if handoff := strings.TrimSpace(f.first); handoff != "" {
		if err := proto.CheckFirst(handoff, "the handoff"); err != nil {
			m.form.err, m.form.at = err.Error(), fieldFirst
			return m, nil
		}
	}
	m.form, m.pending = nil, m.switchAction(f)
	return m, nil
}

// formResult takes the result of the form's act: it closes when the act
// went through, and a refusal stays in it.
func (m Model) formResult(r proto.Result) (Model, bool) {
	if m.form != nil && r.ActID != "" && (r.ActID == m.form.planAct || r.ActID == m.form.cloneAct) {
		return m.cloneResult(r), true
	}
	if m.form == nil || m.form.act != r.ActID {
		return m, false
	}
	f := *m.form
	m.form = &f
	f.act = ""
	switch {
	case r.Err != "":
		f.err = r.Err // the folders typed stay, to fix
		return m, true
	case f.pick: // as the box's yes: the reply says how it went
		m = m.closeScan()
		roots, _ := splitFolders(f.roots)
		m.flash = m.scanFlash(r, showFolders(roots))
	case len(r.Skipped) > 0 && f.rename && strings.HasSuffix(r.Skipped[0], "renames when it rests"):
		m.flash = r.Skipped[0]
	case len(r.Skipped) > 0:
		f.err = strings.Join(r.Skipped, "; ")
		return m, true
	case f.rename:
		m.flash = "renamed " + f.who + " to " + strings.TrimSpace(f.name)
	default:
		name := strings.TrimSpace(f.name)
		if !f.named {
			name = f.autoName()
		}
		m.flash = "started " + name + ": it shows in Sessions in a few seconds"
		m.lastAgent = f.agent
	}
	m.form = nil
	return m, true
}

// cloneResult takes the result of a clone act of the form: the plan
// fills where's rows; a clone that does not start or stop says why.
func (m Model) cloneResult(r proto.Result) Model {
	f := *m.form
	m.form = &f
	if r.ActID == f.planAct {
		f.planAct, f.plan, f.planErr = "", r.Clone, r.Err
		if f.plan == nil && f.planErr == "" {
			f.planErr = "no plan for this git URL"
		}
		if f.whereList() {
			m.refreshWhere(&f, "")
		}
		return m
	}
	f.cloneAct = ""
	if r.Err != "" {
		f.err, f.cloning, f.startWhenDone = r.Err, "", false
	}
	return m
}

// formBox is the form's lines in bw columns, the key a tap on each line
// presses, the footer's key spans, and the agent chips' line with the
// agent a tap on each chip picks.
func (m Model) formBox(bw int) (box, press []string, spans []span, agentsAt int, agents []span) {
	st, f := m.st, m.form
	iw := bw - 4
	var lines []string
	press = []string{""} // the box's top edge
	add := func(p string, l ...string) {
		for _, x := range l {
			lines, press = append(lines, x), append(press, p)
		}
	}
	field := func(id int, value, placeholder string) {
		label := fmt.Sprintf("%-7s", formLabels[id])
		if f.swap && id == fieldFirst {
			label = "handoff"
		}
		lead := st.faint.Render(label) + "  "
		if f.at == id {
			lead = st.muted.Render(label) + st.accent.Render("› ")
		}
		room := iw - 9
		shown := tilde(value)
		if w := lipgloss.Width(shown); w > room-1 {
			shown = "…" + string([]rune(shown)[len([]rune(shown))-(room-2):])
		}
		body := st.text.Render(shown)
		switch {
		case id == fieldWhere && f.cloning != "":
			body = st.muted.Render(m.cloneText(f, room-1))
		case value == "":
			body = st.faint.Render(fit(placeholder, room-1))
		}
		if f.at == id {
			body += st.accent.Render("▏")
		}
		add(fmt.Sprintf("field:%d", id), lead+body)
	}
	title := st.accent.Bold(true).Render("New session")
	switch {
	case f.swap:
		title = st.accent.Bold(true).Render("Switch agent of " + f.who)
		agentsAt = len(press)
		line, chips := m.agentChips(iw)
		add(fmt.Sprintf("field:%d", fieldAgent), line)
		agents = chips
		field(fieldFirst, f.first, "blank: it waits for you")
	case f.pick:
		title = st.accent.Bold(true).Render("Find repositories in these folders")
		field(fieldFolders, f.roots, "~ is your home")
		add("", "", st.faint.Render(fit(`a space between folders; "~/My Work" keeps its space`, iw)))
	case f.rename:
		title = st.accent.Bold(true).Render("Rename " + f.who)
		field(fieldName, f.name, "")
		add("", "", st.faint.Render(fit("tmux and the agent take this name", iw)))
	case f.whereList():
		field(fieldWhere, f.where, "a repo, a path, or a git URL")
		add("", "")
		rows, keys := m.whereLines(iw)
		for i, l := range rows {
			add(keys[i], l)
		}
		add("", "", m.thenLine(iw))
	default:
		hint := "a path"
		if len(m.wrepos) > 0 {
			hint = "a path, or " + m.keys.form.Left.Help().Key + " for a repository"
		}
		field(fieldWhere, f.where, hint)
		field(fieldBranch, f.branch, "blank: no new worktree")
		name := f.name
		if !f.named {
			name = ""
		}
		field(fieldName, name, f.autoName())
		agentsAt = len(press)
		line, chips := m.agentChips(iw)
		add(fmt.Sprintf("field:%d", fieldAgent), line)
		agents = chips
		field(fieldFirst, f.first, "blank: it waits for you")
	}
	if f.act != "" {
		add("", "", st.muted.Render("starting…"))
	}
	if f.err != "" {
		add("", "")
		for _, l := range wrapPlain(f.err, iw-2) {
			add("", st.warn.Render("! ")+st.text.Render(l))
		}
	}
	box, spans = boxFrame(st.hair, title, lines, m.formFoot(bw), bw)
	press = append(press, "") // the box's bottom edge: its footer keys are spans
	return box, press, spans, agentsAt, agents
}

// agentChips is the agent chips, the one chosen filled, with the span of
// each a tap picks.
func (m Model) agentChips(iw int) (string, []span) {
	st, f := m.st, m.form
	label := fmt.Sprintf("%-7s", formLabels[fieldAgent])
	line := st.faint.Render(label) + "  "
	if f.at == fieldAgent {
		line = st.muted.Render(label) + st.accent.Render("› ")
	}
	x := 9
	var spans []span
	for i, a := range m.formAgents() {
		if i > 0 {
			line += "  "
			x += 2
		}
		chip, style := "○ "+a, st.muted
		if a == f.agent {
			chip, style = "● "+a, st.accent
		}
		if x+lipgloss.Width(chip) > iw {
			break
		}
		line += style.Render(chip)
		spans = append(spans, span{x + 2, x + 2 + lipgloss.Width(chip), a}) // + the box's "│ "
		x += lipgloss.Width(chip)
	}
	return line, spans
}

func (m Model) formFoot(bw int) []footPart {
	st := m.st
	fk, sep := m.keys.form, " ─ "
	switch {
	case m.form.whereList():
		return m.listFoot(bw)
	case m.form.pick:
		return []footPart{st.footFor(fk.Enter, "find", sep), st.footFor(fk.Close, "", sep)}
	case m.form.rename:
		return []footPart{st.footFor(fk.Enter, "rename", sep), st.footFor(fk.Close, "", sep)}
	case m.form.swap:
		return []footPart{st.footFor(fk.Enter, "plan", sep), st.footFor(fk.Next, "", sep), st.footFor(fk.Close, "", sep)}
	}
	return []footPart{st.footFor(fk.Enter, "", sep), st.footFor(fk.Next, "", sep), st.footFor(fk.Close, "", sep)}
}

func (m Model) overlayForm(sc *drawn, w int) {
	phone := m.layoutAt(w) == Narrow || m.upright()
	bw := min(w, 72)
	if !phone {
		bw = min(62, w-8)
	}
	box, press, spans, agentsAt, agents := m.formBox(bw)
	m.overlayBox(sc, w, bw, phone, box, press, spans)
	sc.formAgentsAt, sc.formAgents = sc.peekAt+agentsAt, agents
}

// tapForm: a tap on a field puts the cursor there, one on an agent chip
// picks it, one on a key of the footer presses it.
func (m Model) tapForm(ms tea.Mouse, sc drawn) (tea.Model, tea.Cmd) {
	i, dx := ms.Y-sc.peekAt, ms.X-sc.peekX
	if i < 0 || i >= len(sc.peekRows) || dx < 0 || dx >= sc.peekW {
		return m, nil
	}
	f := *m.form
	if ms.Y == sc.formAgentsAt {
		if c, ok := spanAt(sc.formAgents, dx); ok {
			f.agent, f.at = c.press, fieldAgent
			m.form = &f
			return m, nil
		}
	}
	if i == len(sc.peekRows)-1 {
		if s, ok := spanAt(sc.peekKeys, dx); ok {
			return m.formKey(keyPress(s.press))
		}
		return m, nil
	}
	var id int
	if _, err := fmt.Sscanf(sc.peekRows[i], "row:%d", &id); err == nil && f.whereList() && id < len(f.rows) && f.rows[id].kind != rowLabel {
		m.form = &f
		if f.sel = id; f.rows[id].kind == rowClone {
			return m, m.startClone(&f, f.rows[id])
		}
		if !f.pickWhere() {
			f.top = scrollTop(f.top, f.sel, m.listHeight(), len(f.rows))
		}
		return m, nil
	}
	if _, err := fmt.Sscanf(sc.peekRows[i], "field:%d", &id); err == nil {
		m.form = &f
		if f.at = id; id == fieldWhere && !f.listOpen && f.cloning == "" {
			m.openWhere(&f)
		}
	}
	return m, nil
}

// killAction is x on a session: kill-pane, then a TERM for an agent that
// outlived it. One that works or waits asks for a second y.
func (m Model) killAction(s sample.Session) *action {
	if s.Pane == "" {
		return nil
	}
	if sv, _ := sample.SplitPane(s.Pane); sv != "" && sv == m.stokerServer {
		return nil // the stoker's own session: Q on its tab quits it
	}
	steps := proto.KillSteps(s.Pane, s.PID)
	say := "kill " + s.Name + ": " + shellLine(steps[0])
	if len(steps) > 1 {
		say += ", then " + shellLine(steps[1])
	}
	return &action{label: "kill", steps: steps, destructive: true, rec: "kill:" + s.Pane, which: "secondary", say: say, again: m.againFor(s, "kills")}
}

// againFor is what the second y's confirm says when s works or waits for
// you, verb what y again does; "" when it rests.
func (m Model) againFor(s sample.Session, verb string) string {
	switch {
	case s.Ask != nil || s.Permits || m.waitsFor(s.Pane):
		return s.Name + " waits for you: " + m.keys.main.Grant.Help().Key + " again " + verb + " it all the same"
	case s.Works():
		return s.Name + " works: " + m.keys.main.Grant.Help().Key + " again " + verb + " it mid-turn"
	}
	return ""
}

// waitsFor tells if the queue says the session in pane waits for you.
func (m Model) waitsFor(pane string) bool {
	for _, it := range m.queue {
		if it.Pane == pane && (it.State == queue.StatePermission || it.State == queue.StateQuestion && !it.Reminder) {
			return true
		}
	}
	return false
}

// splitFolders is the folders field: a space between folders, and a
// folder in double quotes keeps its spaces.
func splitFolders(s string) ([]string, error) {
	var out []string
	var word strings.Builder
	quoted := false
	end := func() {
		if word.Len() > 0 {
			out = append(out, word.String())
		}
		word.Reset()
	}
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
		case !quoted && unicode.IsSpace(r):
			end()
		default:
			word.WriteRune(r)
		}
	}
	if quoted {
		return nil, errors.New("a quote is not closed")
	}
	end()
	return out, nil
}

// showFolders is folders as the field takes them, ~ the home.
func showFolders(folders []string) string {
	shown := make([]string, len(folders))
	for i, f := range folders {
		if shown[i] = tilde(f); strings.ContainsFunc(f, unicode.IsSpace) {
			shown[i] = `"` + shown[i] + `"`
		}
	}
	return strings.Join(shown, " ")
}
