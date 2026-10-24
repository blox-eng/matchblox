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

	"github.com/blox-eng/matchblox/internal/accounts"
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
	fieldAccount // after agent, for an agent with accounts
	fieldSetup   // under account, for a new account
)

var formLabels = [...]string{"where", "branch", "name", "agent", "first", "folders", "account", "setup"}

// newAccountChoice is the last entry of the account field.
const newAccountChoice = "+ new account"

// accountNameWhy is why a new account's name is refused: the rule of
// accounts.CheckName, which needs the host's home and OS for Codex's
// socket length (the service checks that one).
const accountNameWhy = "a–z, 0–9 and -, up to 10"

// setups are the ways a new account starts, in the order ←→ walks them:
// the service's value, the words the form shows, and what it does.
var setups = [...]struct{ id, label, help string }{
	{"like-default", "like default", "shares skills, plugins and settings with default"},
	{"copy", "copy of default", "copies setup files once; any tokens stay on this machine"},
	{"empty", "empty", "nothing shared, the agent's first run"},
}

// checkAccountName is accounts.CheckName's name rule, as the builder types:
// 1 to 10 characters from a-z, 0-9 and -, and not "default".
func checkAccountName(name string) error {
	if name == "default" || name == "" || len(name) > 10 {
		return errors.New(accountNameWhy)
	}
	for _, r := range name {
		if r != '-' && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return errors.New(accountNameWhy)
		}
	}
	return nil
}

// form is the open New session, Rename, Switch agent or scan folders form.
// Rename has the name only; Switch agent the agent and the handoff (in
// first); the scan's folders the roots.
type form struct {
	rename     bool
	swap       bool   // Switch agent
	pick       bool   // the scan's folders, in roots
	roots      string // space separated
	pane, who  string // Rename, Switch agent: the session's pane and its name now
	from       string // Switch agent: the session's agent now (where: its directory)
	at         int    // the field the cursor is on
	where      string
	branch     string
	name       string
	named      bool // the name was typed: it no longer follows where
	agent      string
	first      string
	whereAt    int                        // ←→ on where: the Git path it shows (-1: typed)
	accts      map[string][]string        // the accounts each agent can start on
	skipped    map[string][]accounts.Skip // the folders not listed, per agent
	last       map[string]string          // the account last started, per agent
	account    string                     // "": default; else the account, or the new one's name being typed
	newAccount bool                       // account is a name for a new one
	setup      string                     // a new account's setup: like-default | copy | empty
	act, err   string                     // the act in flight, and why the last one did not go
}

const maxField = 300

// openNew is N: the New session form, where from the selected row (a Git
// repository or worktree, a session's directory), or the first repository.
func (m Model) openNew() (tea.Model, tea.Cmd) {
	switch {
	case m.conn == nil || m.lost:
		m.flash = "not sent, no connection"
		return m, nil
	case len(m.agents) == 0:
		m.flash = "no agent to start here: install claude, codex, opencode or cursor-agent"
		return m, nil
	}
	f := &form{at: fieldWhere, whereAt: -1, agent: m.agents[0], accts: m.accounts, skipped: m.skipped, last: m.lastAccount}
	// This console's last start first, else the service's, after a restart.
	if slices.Contains(m.agents, m.lastAgent) {
		f.agent = m.lastAgent
	} else if slices.Contains(m.agents, m.keptAgent) {
		f.agent = m.keptAgent
	}
	f.account = f.lastAccount()
	switch m.tab {
	case tabGit:
		if wt, ok := m.selectedWorktree(); ok {
			f.where = wt.Path
		}
	case tabSessions:
		if s, ok := m.selectedSession(); ok {
			f.where = s.Cwd
		}
	}
	if paths := m.gitPaths(); f.where == "" && len(paths) > 0 {
		f.where, f.whereAt = paths[0], 0
	}
	if i := slices.Index(m.gitPaths(), f.where); i >= 0 {
		f.whereAt = i
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
	f := &form{swap: true, pane: s.Pane, who: s.Name, from: s.Agent, where: s.Cwd, at: fieldAgent, first: proto.DefaultHandoff, accts: m.accounts, skipped: m.skipped, last: m.lastAccount}
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
	if len(f.accts[f.agent]) > 0 {
		// The account the confirm names: the service refuses the switch when
		// its last account is another by then.
		sw.Account = cmp.Or(f.lastAccount(), "default")
	}
	b, _ := json.Marshal(sw)
	sw.Anyway = true
	again, _ := json.Marshal(sw)
	// The service starts it on its last account: the confirm names it.
	on := ""
	if len(f.accts[f.agent]) > 0 {
		on = " on default,"
		if a := f.lastAccount(); a != "" {
			on = " on " + a + ","
		}
	}
	say := fmt.Sprintf("switch %s: %s → %s in %s, %s,%s then jump there", f.who, f.from, f.agent, f.pane, f.where, on)
	a := &action{label: "switch", destructive: true, rec: "switch:" + f.pane, text: string(b), againText: string(again), say: say, done: "switched " + f.who + " to " + f.agent}
	if i := slices.IndexFunc(m.snap.Sessions, func(s sample.Session) bool { return s.Pane == f.pane }); i >= 0 {
		a.again = m.againFor(m.snap.Sessions[i], "switches")
	}
	return a
}

// gitPaths are ←→ on where: the repositories and worktrees of the Git tab,
// then the ones found on the host, the most recently used first.
func (m Model) gitPaths() []string {
	return append(m.worktreePaths(), m.foundPaths...)
}

// worktreePaths are the Git tab's paths, each once.
func (m Model) worktreePaths() []string {
	var out []string
	for _, r := range m.allWorktreeRows() {
		if !slices.Contains(out, r.wt.Path) {
			out = append(out, r.wt.Path)
		}
	}
	return out
}

// rebuildFoundPaths builds foundPaths when the found checkouts came
// (found) or the Git tab's paths changed: once, not at each draw.
func (m Model) rebuildFoundPaths(found bool) Model {
	wt := m.worktreePaths()
	if !found && slices.Equal(wt, m.foundWt) {
		return m
	}
	m.foundWt, m.foundPaths = wt, nil
	if m.found == nil {
		return m
	}
	on := make(map[string]bool, len(wt))
	for _, p := range wt {
		on[filepath.Clean(p)] = true
	}
	repos := slices.Clone(m.found.Repos)
	slices.SortStableFunc(repos, func(a, b proto.Checkout) int { return b.Active.Compare(a.Active) })
	paths := make([]string, 0, len(repos))
	for _, c := range repos {
		if p := filepath.Clean(c.Path); !on[p] {
			on[p] = true
			paths = append(paths, p)
		}
	}
	m.foundPaths = paths
	return m
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
	out := []int{fieldWhere, fieldBranch, fieldName, fieldAgent}
	if f.hasAccounts() {
		out = append(out, fieldAccount)
		if f.newAccount {
			out = append(out, fieldSetup)
		}
	}
	if !f.newAccount {
		out = append(out, fieldFirst) // a new account signs in first: no message
	}
	return out
}

// hasAccounts tells if the New session form shows the account field: the
// agent keeps accounts, and the service listed them.
func (f *form) hasAccounts() bool {
	if f.rename || f.swap {
		return false
	}
	switch f.agent {
	case "claude", "codex", "cursor-agent", "cursor":
		return len(f.accts[f.agent]) > 0
	}
	return false
}

// accountChoices are what ←→ walks on the account field: the accounts, the
// default first, and the new one.
func (f *form) accountChoices() []string {
	return append(slices.Clone(f.accts[f.agent]), newAccountChoice)
}

// accountShown is the account the field names: default for "".
func (f *form) accountShown() string {
	switch {
	case f.newAccount:
		return newAccountChoice
	case f.account == "":
		return "default"
	}
	return f.account
}

// pickAccount moves the account field by one choice, wrapping.
func (f *form) pickAccount(by int) {
	cs := f.accountChoices()
	i := slices.Index(cs, f.accountShown())
	if i < 0 {
		i = 0
	}
	c := cs[(i+by+len(cs))%len(cs)]
	f.account, f.newAccount = c, false
	switch c {
	case "default":
		f.account = ""
	case newAccountChoice:
		f.account, f.newAccount = "", true
		if f.setup == "" {
			f.setup = setups[0].id
		}
	}
}

// lastAccount is the account the field opens on for the agent: the one last
// started when it is still in the agent's list, else "" for default. It is
// never the new account.
func (f *form) lastAccount() string {
	a := f.last[f.agent]
	if a == "default" || !slices.Contains(f.accts[f.agent], a) {
		return ""
	}
	return a
}

// setAgent picks the agent; another one starts on the account last started
// for it, else on its default account.
func (f *form) setAgent(a string) {
	if a != f.agent {
		f.agent = a
		f.account, f.newAccount, f.setup = f.lastAccount(), false, ""
	}
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
		if f.newAccount && !f.swap {
			return nil
		}
		return &f.first
	case fieldFolders:
		return &f.roots
	case fieldAccount:
		if f.newAccount {
			return &f.account
		}
	}
	return nil
}

// autoName is the name a new session takes when none is typed: the
// worktree's for a branch, else the directory's, as the service makes it.
func (f *form) autoName() string {
	base := filepath.Base(filepath.Clean(f.where))
	if f.where == "" {
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

func (m Model) formKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	f := *m.form
	m.form = &f
	fk := m.keys.form
	switch {
	case bkey.Matches(k, fk.Close):
		m.form = nil // nothing is sent
		return m, nil
	case bkey.Matches(k, fk.Enter):
		return m.formEnter()
	case bkey.Matches(k, fk.Next):
		f.move(1)
		return m, nil
	case bkey.Matches(k, fk.Prev):
		f.move(-1)
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
			f.setAgent(agents[(i+len(agents)-1)%len(agents)])
		case bkey.Matches(k, fk.Right):
			f.setAgent(agents[(i+1)%len(agents)])
		}
		return m, nil
	}
	// On a new account's name, space is not a pick: only the arrows are.
	if f.at == fieldAccount && bkey.Matches(k, fk.Left, fk.Right) && (k.Text == "" || !f.newAccount) {
		by := 1
		if bkey.Matches(k, fk.Left) {
			by = -1
		}
		f.pickAccount(by)
		return m, nil
	}
	if f.at == fieldSetup {
		i := setupAt(f.setup)
		switch {
		case bkey.Matches(k, fk.Left):
			f.setup = setups[(i+len(setups)-1)%len(setups)].id
		case bkey.Matches(k, fk.Right):
			f.setup = setups[(i+1)%len(setups)].id
		}
		return m, nil
	}
	// On where, space is text: only the arrows pick.
	if f.at == fieldWhere && k.Text == "" && bkey.Matches(k, fk.Left, fk.Right) {
		paths := m.gitPaths()
		if len(paths) == 0 {
			return m, nil
		}
		n, right := len(paths), bkey.Matches(k, fk.Right)
		switch {
		case f.whereAt < 0 && right:
			f.whereAt = 0
		case f.whereAt < 0:
			f.whereAt = n - 1
		case right:
			f.whereAt = (f.whereAt + 1) % n
		default:
			f.whereAt = (f.whereAt + n - 1) % n
		}
		f.where = paths[f.whereAt]
		return m, nil
	}
	t := f.text(f.at)
	if t == nil {
		return m, nil
	}
	switch {
	case bkey.Matches(k, fk.Erase):
		*t = trimLast(*t)
	default:
		if k.Text == "" || len([]rune(*t+k.Text)) > maxField {
			return m, nil
		}
		*t += k.Text
	}
	switch f.at {
	case fieldWhere:
		f.whereAt = -1
	case fieldName:
		f.named = f.name != ""
	}
	return m, nil
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
			f.err, f.at = "where does it start? a path, or "+m.keys.form.Left.Help().Key+" for a repository", fieldWhere
			return m, nil
		}
		n := proto.NewSession{Where: strings.TrimSpace(f.where), Branch: strings.TrimSpace(f.branch), Agent: f.agent, First: strings.TrimSpace(f.first)}
		if f.named {
			n.Name = name
		}
		if f.hasAccounts() && f.newAccount {
			if err := checkAccountName(f.account); err != nil {
				f.err, f.at = "name the new account: "+err.Error(), fieldAccount
				return m, nil
			}
			n.Account, n.NewAccount, n.Setup, n.First = f.account, true, f.setup, "" // it signs in in the session first
			sess := name
			if !f.named {
				sess = f.autoName()
			}
			m.form, m.pending = nil, m.startAction(n, sess)
			return m, nil
		}
		if f.hasAccounts() {
			// The account is always named, default too: the service
			// remembers only a start that names its account.
			n.Account = f.accountShown()
		}
		if n.Account != "" && n.Account != "default" {
			m.flash = fmt.Sprintf("start %s in %s on %s", n.Agent, tilde(n.Where), n.Account)
		}
		b, _ := json.Marshal(n)
		cmd = m.send(proto.Act{RecID: "new:session", Confirm: "y", Text: string(b)})
	}
	f.act, f.err = fmt.Sprintf("a%d", m.acts), ""
	return m, cmd
}

// startAction is the plan of a start on a new account: it waits for a typed
// y, the service's confirmed act. The account is made first and signs in in
// the session, sess.
func (m Model) startAction(n proto.NewSession, sess string) *action {
	b, _ := json.Marshal(n)
	say := fmt.Sprintf("new %s account %q, %s: sign in in the session", n.Agent, n.Account, setups[setupAt(n.Setup)].label)
	return &action{label: "start", destructive: true, rec: "new:session", text: string(b), say: say, agent: n.Agent,
		done: "started " + sess + ": sign in in " + sess}
}

// setupAt is the index in setups of the setup id, 0 (like default) if none.
func setupAt(id string) int {
	return max(slices.IndexFunc(setups[:], func(x struct{ id, label, help string }) bool { return x.id == id }), 0)
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
	if m.form == nil || m.form.act != r.ActID {
		return m, false
	}
	f := *m.form
	m.form = &f
	f.act = ""
	// A start with notes on what its account shared or skipped ran all
	// the same.
	notes, ran := proto.StartNotes(r)
	if !f.rename && ran {
		r.Skipped = nil
	}
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
		if len(notes) > 0 {
			m.flash += "; " + strings.Join(notes, "; ")
		}
		m.lastAgent = f.agent
	}
	m.form = nil
	return m, true
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
		if value == "" {
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
	default:
		hint := "a path"
		if len(m.foundPaths) > 0 || len(m.allWorktreeRows()) > 0 {
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
		if f.hasAccounts() {
			m.accountLines(iw, field, add)
		}
		if f.newAccount {
			field(fieldFirst, "", "after sign-in")
		} else {
			field(fieldFirst, f.first, "blank: it waits for you")
		}
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
	box, spans = boxFrame(st.hair, title, lines, m.formFoot(), bw)
	press = append(press, "") // the box's bottom edge: its footer keys are spans
	return box, press, spans, agentsAt, agents
}

// accountLines are the account field, and for a new account its name's
// reason when refused and the setup row with a line of help.
func (m Model) accountLines(iw int, field func(int, string, string), add func(string, ...string)) {
	st, f := m.st, m.form
	room := iw - 9
	lead := func(id int) string {
		label := fmt.Sprintf("%-7s", formLabels[id])
		if f.at == id {
			return st.muted.Render(label) + st.accent.Render("› ")
		}
		return st.faint.Render(label) + "  "
	}
	// choose is ‹ the one chosen › and the others, as many as fit.
	choose := func(chosen string, others []string) string {
		out := st.accent.Render("‹ " + chosen + " ›")
		if rest := strings.Join(others, " · "); rest != "" && lipgloss.Width(out)+2+lipgloss.Width(rest) <= room {
			out += "  " + st.faint.Render(rest)
		}
		return out
	}
	notListed := func() {
		if why := notListedLine(f.skipped[f.agent]); why != "" {
			add("", st.faint.Render(strings.Repeat(" ", 9)+fit(why, room)))
		}
	}
	if f.newAccount {
		field(fieldAccount, f.account, "a name, up to 10")
		notListed()
		if f.account != "" && checkAccountName(f.account) != nil {
			add("", st.faint.Render(strings.Repeat(" ", 9))+st.warn.Render(fit(accountNameWhy, room)))
		}
		labels := make([]string, len(setups))
		for i, x := range setups {
			labels[i] = x.label
		}
		i := setupAt(f.setup)
		add(fmt.Sprintf("field:%d", fieldSetup), lead(fieldSetup)+choose(labels[i], slices.Delete(slices.Clone(labels), i, i+1)))
		for _, l := range wrapPlain(setups[i].label+": "+setups[i].help, room) {
			add(fmt.Sprintf("field:%d", fieldSetup), strings.Repeat(" ", 9)+st.faint.Render(l))
		}
		return
	}
	add(fmt.Sprintf("field:%d", fieldAccount), lead(fieldAccount)+choose(f.accountShown(), f.accountChoices()))
	notListed()
}

// notListedLine says how many folders the account list left out and why: the
// reason when they share it, else the first and "…". "" when none.
func notListedLine(skips []accounts.Skip) string {
	if len(skips) == 0 {
		return ""
	}
	why := skips[0].Why
	for _, s := range skips[1:] {
		if s.Why != why {
			why += " …"
			break
		}
	}
	return fmt.Sprintf("%d not listed: %s", len(skips), why)
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

func (m Model) formFoot() []footPart {
	st := m.st
	fk, sep := m.keys.form, " ─ "
	switch {
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
			f.setAgent(c.press)
			f.at = fieldAgent
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
	if _, err := fmt.Sscanf(sc.peekRows[i], "field:%d", &id); err == nil {
		f.at = id
		m.form = &f
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
