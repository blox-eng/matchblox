package app

import (
	"cmp"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
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
	accts      map[string][]string        // the accounts each agent can start on
	skipped    map[string][]accounts.Skip // the folders not listed, per agent
	last       map[string]string          // the account last started, per agent
	account    string                     // "": default; else the account, or the new one's name being typed
	newAccount bool                       // account is a name for a new one
	setup      string                     // a new account's setup: like-default | copy | empty
	act, err   string                     // the act in flight, and why the last one did not go
	// where's list (#174): its rows for the filter, the selected one and
	// the first shown; picked: where holds a pick or a ←→ step, not a
	// filter, and query is what was typed before it; extra: the folder N
	// opened on, when it is not a known repo. rowsFor is the filter the
	// rows are for: the scroll stays while it does.
	rows     []whereRow
	rowsFor  string
	sel, top int
	listOpen bool
	picked   bool
	query    string
	extra    string
	// A git URL in where (#176): plan is the service's plan for planFor,
	// planErr why there is none, planAct the clone:plan in flight.
	// cloning is the target of the clone this form started, startAct its
	// clone:start while it is on its way; startWhenDone: the final ⏎ came
	// while it ran, and the session starts as startAs when it is done.
	plan          *proto.ClonePlan
	planFor       string
	planErr       string
	planAct       string
	cloning       string
	startAct      string
	startWhenDone bool
	startAs       proto.NewSession
	pickOnPlan    bool // ⏎ or tab came before the plan: done when it lands
	// cloneID is the id the service gave the clone: only its records
	// count, never an earlier try's in the same folder. seen: a record of
	// it was listed.
	cloneID int64
	seen    bool
	// token is the form's own for its clone:start: after a lost answer,
	// only a clone listed with it is the form's, never another console's
	// start into the same folder.
	token string
	// cloned is the folder this form's clone made: a session there says
	// Cloned, and Cursor asks before it trusts it.
	cloned string
}

// cloneStop is a stop whose answer has not come: act is its clone:stop,
// or the clone:start it waits for (id 0 until that start answers), whose
// token is token.
type cloneStop struct {
	act, target, token string
	id                 int64
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
	f := &form{at: fieldWhere, agent: m.agents[0], accts: m.accounts, skipped: m.skipped, last: m.lastAccount}
	// This console's last start first, else the service's, after a restart.
	if slices.Contains(m.agents, m.lastAgent) {
		f.agent = m.lastAgent
	} else if slices.Contains(m.agents, m.keptAgent) {
		f.agent = m.keptAgent
	}
	f.account = f.lastAccount()
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
	if i := slices.IndexFunc(m.snap.Sessions, func(s sample.Session) bool { return s.Pane == f.pane }); i >= 0 {
		sw.ExpectedPID, sw.ExpectedStart = m.snap.Sessions[i].PID, m.snap.Sessions[i].Start
	}
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
	say += "; add match awareness"
	if f.agent == "codex" {
		say += "; trust this workspace and enable workspace-write network access"
	}
	if sw.Handoff == "" {
		say += "; no task or handover"
	} else {
		say += "; capture handover before replacement"
	}
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

// moveTo is move, and back on where the list opens (onWhere).
func (m *Model) moveTo(f *form, by int) tea.Cmd {
	f.move(by)
	return m.onWhere(f)
}

// onWhere is the cursor come back to where, by key or tap: the list
// opens, on a git URL whose clone failed or stopped with a fresh plan
// asked.
func (m *Model) onWhere(f *form) tea.Cmd {
	if f.at != fieldWhere || f.cloning != "" { // no list while it clones
		return nil
	}
	m.openWhere(f)
	return m.planWhere(f)
}

func (m Model) formKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	f := *m.form
	m.form = &f
	fk := m.keys.form
	if f.whereList() {
		f.err = ""
		if r := m.whereSelectedIn(&f); r.kind == rowClone && bkey.Matches(k, fk.Enter, fk.Next) {
			if r.repo.path == "" { // the plan is on its way, or asked for now
				f.pickOnPlan = true
				return m, m.planWhere(&f)
			}
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
		return m, m.moveTo(&f, 1)
	case bkey.Matches(k, fk.Prev, fk.Up):
		return m, m.moveTo(&f, -1)
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
	return m, tea.Batch(stop, m.planSoon(&f))
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
		f.listOpen, f.pickOnPlan = true, false
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
		if n.NewAccount {
			if err := checkAccountName(f.account); err != nil {
				f.err, f.at = "name the new account: "+err.Error(), fieldAccount
				return m, nil
			}
		}
		if f.cloning != "" { // it starts when the clone is done
			f.startWhenDone, f.startAs, f.err = true, n, ""
			return m, nil
		}
		if _, ok := cloneOf(f.where); ok && !f.picked {
			// A URL that is not cloned: back to its rows, where ⏎ clones.
			m.openWhere(&f)
			return m, m.planWhere(&f)
		}
		cmd := m.startNew(&f, n)
		return m, cmd
	}
	f.act, f.err = fmt.Sprintf("a%d", m.acts), ""
	return m, cmd
}

// newSession is the New session form's new:session, in where.
func (f *form) newSession(where string) proto.NewSession {
	n := proto.NewSession{Where: where, Branch: strings.TrimSpace(f.branch), Agent: f.agent, First: strings.TrimSpace(f.first)}
	n.Cloned = f.cloned != "" && filepath.Clean(where) == filepath.Clean(f.cloned)
	if f.named {
		n.Name = strings.TrimSpace(f.name)
	}
	switch {
	case f.hasAccounts() && f.newAccount:
		n.Account, n.NewAccount, n.Setup, n.First = f.account, true, f.setup, "" // it signs in in the session first
	case f.hasAccounts():
		// The account is always named, default too: the service
		// remembers only a start that names its account.
		n.Account = f.accountShown()
	}
	return n
}

// startNew starts n, f's new session; one on a new account waits for its
// typed y first.
func (m *Model) startNew(f *form, n proto.NewSession) tea.Cmd {
	if n.NewAccount {
		sess := strings.TrimSpace(f.name)
		if !f.named {
			sess = f.autoName()
		}
		m.form, m.pending = nil, m.startAction(n, sess)
		return nil
	}
	if n.Account != "" && n.Account != "default" {
		m.flash = fmt.Sprintf("start %s in %s on %s", n.Agent, tilde(n.Where), n.Account)
	}
	return m.sendNew(f, n)
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

// planTickMsg is a typed git URL that has stood still for planWait: the
// plan is asked for text if it is still the text in where.
type planTickMsg struct{ text string }

// planWait is how long typed text stands still before its plan is asked for:
// a URL typed by hand is not planned for each key.
const planWait = 250 * time.Millisecond

// planSoon is planWhere for typed text: it waits until the text has stood
// still for planWait. A paste and ⏎ plan at once.
func (m *Model) planSoon(f *form) tea.Cmd {
	q := strings.TrimSpace(f.filter())
	if _, ok := cloneOf(q); f.at != fieldWhere || !ok || q == f.planFor {
		return nil
	}
	return tea.Tick(planWait, func(time.Time) tea.Msg { return planTickMsg{q} })
}

// planTick plans the text of a tick that is still the text in where.
func (m Model) planTick(msg planTickMsg) (tea.Model, tea.Cmd) {
	if m.form == nil || strings.TrimSpace(m.form.filter()) != msg.text {
		return m, nil
	}
	f := *m.form
	m.form = &f
	return m, m.planWhere(&f)
}

// startClone is ⏎ or tab on a clone row: the clone starts in the service,
// where keeps the URL and the cursor goes to branch. Nothing while the
// plan is on its way.
func (m *Model) startClone(f *form, r whereRow) tea.Cmd {
	if r.kind != rowClone || r.repo.path == "" || f.plan == nil {
		return nil
	}
	token := startToken()
	b, _ := json.Marshal(proto.CloneStart{URL: f.plan.URL, Target: r.repo.path, Token: token})
	cmd := m.send(proto.Act{RecID: "clone:start", Text: string(b)})
	f.startAct, f.cloning, f.startWhenDone, f.err = fmt.Sprintf("a%d", m.acts), r.repo.path, false, ""
	f.token = token
	f.cloneID, f.seen = 0, false
	f.listOpen, f.at = false, fieldBranch
	return cmd
}

// startToken is a fresh token for a clone:start.
func startToken() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b) // never fails
	return fmt.Sprintf("%x", b)
}

// stopClone stops the clone f started: the form forgets it and the
// service deletes its partial folder. The next ⏎ asks for a fresh plan.
func (m *Model) stopClone(f *form) tea.Cmd {
	target := f.cloning
	if target == "" {
		return nil
	}
	if f.startAct != "" { // a stop now could come before the start: it stops when the start lands
		m.stopAfter = append(slices.Clone(m.stopAfter), cloneStop{act: f.startAct, target: target, token: f.token})
		f.forgetClone("")
		return nil
	}
	id := f.cloneID
	f.forgetClone("")
	return m.sendStop(target, id)
}

// sendStop sends clone:stop for the clone id into target, kept until
// its answer comes: a stop the connection lost goes again. Its answer
// says nothing, the builder has moved on, but for a clone that was done
// by then: where it is.
func (m *Model) sendStop(target string, id int64) tea.Cmd {
	b, _ := json.Marshal(proto.CloneStop{Target: target, ID: id})
	cmd := m.send(proto.Act{RecID: "clone:stop", Text: string(b)})
	m.stopQuiet = append(slices.Clone(m.stopQuiet), cloneStop{act: fmt.Sprintf("a%d", m.acts), target: target, id: id})
	return cmd
}

// stopResult takes the answer to a start that esc came before, or to a
// stop: they live on past the form, which may have closed.
func (m Model) stopResult(r proto.Result) (Model, tea.Cmd, bool) {
	id := r.ActID
	if i := slices.IndexFunc(m.stopQuiet, func(s cloneStop) bool { return s.act == id }); i >= 0 {
		m.stopQuiet = slices.Delete(slices.Clone(m.stopQuiet), i, i+1)
		if r.Note != "" { // the clone was done: its folder is there
			m.flash = tilde(r.Note)
		}
		return m, nil, true
	}
	i := slices.IndexFunc(m.stopAfter, func(s cloneStop) bool { return s.act == id })
	if i < 0 {
		return m, nil, false
	}
	target := m.stopAfter[i].target
	m.stopAfter = slices.Delete(slices.Clone(m.stopAfter), i, i+1)
	if r.Err != "" { // it did not start: nothing to stop
		return m, nil, true
	}
	return m, m.sendStop(target, r.CloneID), true
}

// cloneAgain is the first snapshot after a lost connection, for the
// clone acts whose answers went with it: a plan is asked again; a start
// is the clone the snapshot lists there, else it never started; a clone
// never listed and gone now is not waited for; a stop with no answer goes
// again; and the stops that waited for a start go to the clone listed.
// A clone listed is a start's only when it carries that start's token:
// another console may have started one into the same folder.
func (m *Model) cloneAgain() tea.Cmd {
	var cmds []tea.Cmd
	stops := m.stopQuiet
	for _, s := range m.stopAfter {
		if c, ok := m.cloneFor(s.target); ok && c.Err == "" && ownStart(c, s.token) {
			stops = append(slices.Clone(stops), cloneStop{target: s.target, id: c.ID})
		}
	}
	m.stopAfter, m.stopQuiet = nil, nil
	for _, s := range stops {
		if s.id == 0 { // a bare stop would stop whatever runs there
			continue
		}
		cmds = append(cmds, m.sendStop(s.target, s.id))
	}
	if m.form == nil {
		return tea.Batch(cmds...)
	}
	f := *m.form
	m.form = &f
	if f.planAct != "" {
		f.planAct, f.planFor, f.pickOnPlan = "", "", false
		cmds = append(cmds, m.planWhere(&f))
	}
	c, ok := m.cloneFor(f.cloning)
	switch {
	case f.cloning == "":
	case f.startAct != "" && ok && c.Err == "" && ownStart(c, f.token): // a failed one may be the last try's
		f.startAct, f.cloneID = "", c.ID
	case f.startAct != "":
		f.forgetClone("the clone's start got no answer: " + m.keys.form.Enter.Help().Key + " tries again")
	case !f.seen && (!ok || c.ID != f.cloneID):
		f.forgetClone("the clone of " + f.cloneRepo() + " stopped before it was done")
	}
	return tea.Batch(cmds...)
}

// ownStart tells if c is the clone of the start whose token is token.
func ownStart(c proto.Clone, token string) bool {
	return token != "" && c.Token == token
}

// forgetClone drops the form's clone and its plan, err the form's error:
// a clone that failed or stopped is planned afresh.
func (f *form) forgetClone(err string) {
	if err != "" {
		f.err = err
	}
	f.cloning, f.startWhenDone, f.plan, f.planFor = "", false, nil, ""
	f.startAct, f.cloneID, f.seen, f.token = "", 0, false, ""
}

// formClone is the service's clone of the form, while it runs; false
// while its start is on its way.
func (m Model) formClone(f *form) (proto.Clone, bool) {
	if f.startAct != "" {
		return proto.Clone{}, false
	}
	c, ok := m.cloneFor(f.cloning)
	return c, ok && c.ID == f.cloneID && c.Err == "" && !c.Done
}

// cloneRepo is the o/r the form clones.
func (f *form) cloneRepo() string {
	if f.plan != nil {
		return f.plan.Repo
	}
	r, _ := cloneOf(f.where)
	return r.Slug()
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
	if f.startAct != "" {
		return nil // no id yet
	}
	// Only a record with the clone's id counts: snapshots are not ordered
	// against the start's answer, and one built before the service took
	// the start may list an earlier try in this folder, or none.
	c, ok := m.cloneFor(f.cloning)
	switch ok = ok && c.ID == f.cloneID; {
	case !ok && !f.seen:
		return nil
	case !ok: // the service restarted, or the console was away past the minute
		f.forgetClone("the clone of " + f.cloneRepo() + " stopped before it was done")
		return nil
	case c.Err != "":
		f.forgetClone(c.Err)
		return nil
	case !c.Done:
		f.seen = true
		return nil
	}
	f.where, f.picked, f.query, f.planFor, f.cloning = f.cloning, true, "", "", ""
	f.cloned = f.where
	if !f.startWhenDone {
		return nil
	}
	f.startWhenDone = false
	n := f.startAs
	n.Where, n.Cloned = f.where, true
	return m.startNew(&f, n)
}

// cloneText is where's line while the clone runs, in w columns: how far
// it is, and that the session starts when it is done.
func (m Model) cloneText(f *form, w int) string {
	repo := f.cloneRepo()
	pct := ""
	if c, ok := m.formClone(f); ok {
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
	if strings.TrimSpace(f.first) == "" {
		return m, nil
	}
	cmd := m.send(proto.Act{RecID: "switch-preview:" + f.pane})
	m.pending.previewAct = fmt.Sprintf("a%d", m.acts)
	return m, cmd
}

// formResult takes the result of the form's act: it closes when the act
// went through, and a refusal stays in it.
func (m Model) formResult(r proto.Result) (Model, tea.Cmd, bool) {
	if m, cmd, ok := m.stopResult(r); ok {
		return m, cmd, true
	}
	if m.form != nil && r.ActID != "" && (r.ActID == m.form.planAct || r.ActID == m.form.startAct) {
		m, cmd := m.cloneResult(r)
		return m, cmd, true
	}
	if m.form == nil || m.form.act != r.ActID {
		return m, nil, false
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
		return m, nil, true
	case f.pick: // as the box's yes: the reply says how it went
		m = m.closeScan()
		roots, _ := splitFolders(f.roots)
		m.flash = m.scanFlash(r, showFolders(roots))
	case len(r.Skipped) > 0 && f.rename && strings.HasSuffix(r.Skipped[0], "renames when it rests"):
		m.flash = r.Skipped[0]
	case len(r.Skipped) > 0:
		f.err = strings.Join(r.Skipped, "; ")
		return m, nil, true
	case f.rename:
		m.flash = "renamed " + f.who + " to " + strings.TrimSpace(f.name)
	default:
		name := strings.TrimSpace(f.name)
		if !f.named {
			name = f.autoName()
		}
		m.flash = "started " + name + ": it shows in Sessions in a few seconds"
		if r.Note != "" { // what its where came to
			m.flash = tilde(r.Note) + " · " + m.flash
		}
		if len(notes) > 0 {
			m.flash += "; " + strings.Join(notes, "; ")
		}
		m.lastAgent = f.agent
	}
	m.form = nil
	return m, nil, true
}

// cloneResult takes the result of a clone act of the form: the plan
// fills where's rows, and a ⏎ that came before it goes now; a clone that
// does not start or stop says why.
func (m Model) cloneResult(r proto.Result) (Model, tea.Cmd) {
	f := *m.form
	m.form = &f
	switch r.ActID {
	case f.startAct:
		f.startAct = ""
		if r.Err != "" {
			f.forgetClone(r.Err)
		} else {
			f.cloneID = r.CloneID
			// A snapshot may have said it is done before this answer: read it now.
			return m, m.cloneNews()
		}
		return m, nil
	}
	f.planAct, f.plan, f.planErr = "", r.Clone, r.Err
	if f.plan == nil && f.planErr == "" {
		f.planErr = "no plan for this git URL"
	}
	pick := f.pickOnPlan
	f.pickOnPlan = false
	if !f.whereList() {
		return m, nil
	}
	was := m.whereSelectedIn(&f)
	m.refreshWhere(&f, was.path())
	if was.kind == rowClone { // the clone row was selected: the plan's first row is
		if i := slices.IndexFunc(f.rows, func(r whereRow) bool { return r.kind == rowHave || r.kind == rowClone }); i >= 0 {
			f.sel = i
		}
	}
	if !pick || f.planErr != "" {
		return m, nil
	}
	if sel := m.whereSelectedIn(&f); sel.kind == rowClone {
		return m, m.startClone(&f, sel)
	}
	f.pickWhere()
	return m, nil
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
		if f.hasAccounts() {
			m.accountLines(iw, field, add)
		}
		if f.newAccount {
			field(fieldFirst, "", "after sign-in")
		} else {
			field(fieldFirst, f.first, "blank: it waits for you")
		}
	}
	if !f.rename && !f.pick {
		if f.agent == "codex" {
			add("", st.muted.Render(fit("Codex: trust this workspace; workspace-write network on", iw)))
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
	box, spans = boxFrame(st.hair, title, lines, m.formFoot(bw), bw)
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

// notListedLine names the folders the account list left out, and why:
// "not listed: A, b_ (why)" when they share the reason, else each with its
// own; three named, then how many more. A skip of the agent's accounts
// folder itself (a child of the accounts root) says it was not read. "" when
// none.
func notListedLine(skips []accounts.Skip) string {
	if len(skips) == 0 {
		return ""
	}
	if s := skips[0]; filepath.Base(filepath.Dir(s.Path)) == "accounts" {
		return "accounts not read: " + s.Why
	}
	shared := !slices.ContainsFunc(skips, func(s accounts.Skip) bool { return s.Why != skips[0].Why })
	var names []string
	for i, s := range skips {
		if i == 3 {
			names = append(names, fmt.Sprintf("and %d more", len(skips)-3))
			break
		}
		n := filepath.Base(s.Path)
		if !shared {
			n += " (" + s.Why + ")"
		}
		names = append(names, n)
	}
	line := "not listed: " + strings.Join(names, ", ")
	if shared {
		line += " (" + skips[0].Why + ")"
	}
	return line
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
	if _, err := fmt.Sscanf(sc.peekRows[i], "row:%d", &id); err == nil && f.whereList() && id < len(f.rows) && f.rows[id].kind != rowLabel {
		m.form = &f
		if f.sel = id; f.rows[id].kind == rowClone {
			f.pickOnPlan = f.rows[id].repo.path == ""
			return m, m.startClone(&f, f.rows[id])
		}
		if !f.pickWhere() {
			f.top = scrollTop(f.top, f.sel, m.listHeight(), len(f.rows))
		}
		return m, nil
	}
	if _, err := fmt.Sscanf(sc.peekRows[i], "field:%d", &id); err == nil {
		m.form = &f
		if f.at = id; !f.listOpen {
			cmd := m.onWhere(&f)
			return m, cmd
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
