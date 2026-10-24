package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/blox-eng/matchblox/internal/accounts"
	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/holds"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// A session's life from matchblox (#74): new starts an agent in a tmux
// session of its own, rename names its tmux session, its window and the
// agent alike, and kill ends the agent and closes its pane. Its worktree
// and its transcript stay.

// NewSessionID is the act that starts a session; its Text is a
// proto.NewSession.
const NewSessionID = "new:session"

// KillAnyway is the Text of a kill the builder confirmed a second time:
// the session works or waits for them.
const KillAnyway = "anyway"

// Startable are the agents matchblox starts, each with its way to take a
// name and a first message.
var Startable = []string{"claude", "codex", "opencode", sample.CursorAgent}

// cursorBins are the commands Cursor's CLI is installed as, the first one
// found the one a new session runs; `agent` only when it is Cursor's.
var cursorBins = []string{sample.CursorAgent, "agent"}

// agentBin is the command that starts agent here ("" when none is
// installed), agent itself but for Cursor's CLI.
func agentBin(agent string, look func(string) (string, error)) string {
	bins := []string{agent}
	if agent == sample.CursorAgent {
		bins = cursorBins
	}
	for _, b := range bins {
		path, err := look(b)
		if err != nil {
			continue
		}
		if b == "agent" && !cursorsAgent(path) {
			continue
		}
		return b
	}
	return ""
}

// cursorsAgent: the `agent` at path is Cursor's, a link into its installed
// versions. Other tools are called agent too.
func cursorsAgent(path string) bool {
	real, err := filepath.EvalSymlinks(path)
	return err == nil && strings.Contains(filepath.ToSlash(real), "/cursor-agent/versions/")
}

// launchArgv is the agent's command, run as bin: Claude Code takes the
// session's name (--name, which its /rename changes later) and the first
// message as its prompt; Codex takes the prompt; Cursor takes it after
// --trust; OpenCode takes it with --prompt. extra goes right after bin.
func launchArgv(bin, agent, name, first string, extra []string) []string {
	argv := append([]string{bin}, extra...)
	switch agent {
	case "claude":
		argv = append(argv, "--name", name)
	case "opencode":
		if first != "" {
			argv = append(argv, "--prompt")
		}
	case sample.CursorAgent:
		// The builder chose the directory: Cursor does not ask again
		// whether to trust it.
		argv = append(argv, "--trust")
	}
	if first != "" {
		argv = append(argv, first)
	}
	return argv
}

func (s *Service) tmuxOut(argv []string) (string, error) {
	if s.TmuxOut != nil {
		return s.TmuxOut(argv)
	}
	out, err := exec.Command(argv[0], argv[1:]...).Output() //nolint:gosec // argv built here, never a shell string
	return string(out), err
}

func (s *Service) isDir(path string) bool {
	if s.IsDir != nil {
		return s.IsDir(path)
	}
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// tmuxSessions are the names of the tmux sessions on server, and how many
// windows each holds, as the last sample listed every pane.
func (s *Service) tmuxSessions(server string) map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	windows := map[string]map[string]bool{}
	for _, p := range s.cur.Panes {
		if sv, _ := sample.SplitPane(p.ID); sv != server {
			continue
		}
		sess, rest, _ := strings.Cut(p.Target, ":")
		win, _, _ := strings.Cut(rest, ".")
		if windows[sess] == nil {
			windows[sess] = map[string]bool{}
		}
		windows[sess][win] = true
	}
	out := make(map[string]int, len(windows))
	for name, w := range windows {
		out[name] = len(w)
	}
	return out
}

func taken(name string) error {
	return fmt.Errorf("a tmux session is named %s already: choose another name", name)
}

// waitsOn tells if the session waits for the builder: a question or a
// permission prompt, by its hooks or its pane.
func (s *Service) waitsOn(pane string, x sample.Session) bool {
	if x.Ask != nil || x.Permits {
		return true
	}
	it, ok := s.queued(pane)
	return ok && stoker.Waits(it)
}

// kill ends the agent in pane and closes the pane: kill-pane hangs it up,
// and a TERM follows for an agent that outlived its pane. Both steps are
// guarded by the agent's pid and start time, so a pid reused since is
// never signalled. The builder confirms a second time (KillAnyway) for a
// session that works or waits. The stoker kills only a session at rest
// that you did not park, and only with the order's kills go or your y.
func (s *Service) kill(ctx context.Context, pane string, a proto.Act) proto.Result {
	if a.Confirm != "y" {
		return proto.Result{Err: "needs confirm"}
	}
	if s.fromStoker(proto.Act{From: pane}) {
		return proto.Result{Err: "the stoker is not killed here: Q on the STOKER tab quits it"}
	}
	x, ok := s.sessionAt(pane)
	if !ok {
		return proto.Result{Err: "no agent session in " + pane + " any more"}
	}
	word := ""
	switch {
	case s.waitsOn(pane, x):
		word = "waits for you"
	case x.Works():
		word = "works"
	}
	if s.agentsPane(a) {
		if word != "" {
			return proto.Result{Err: x.Name + " " + word + ": the stoker kills only a session at rest"}
		}
		if h, held := s.heldIn(pane, x); held && h.By == holds.ByYou {
			return proto.Result{Err: "parked by you: the stoker leaves it alone"}
		}
		if !s.killGo(a.From, pane) {
			s.askKill(a.From, pane, x)
			return proto.Result{Err: "the kill waits for the builder's go: it is on the card, y gives it"}
		}
	} else if word != "" && a.Text != KillAnyway {
		return proto.Result{Err: x.Name + " " + word + ": y again kills it mid-turn"}
	}
	release := s.holdPane(pane)
	res := s.Actions.Do(ctx, KillAction(pane, x), "y")
	release()
	if len(res.Ran) > 0 {
		res.Skipped = nil // the agent went with its pane: no TERM was needed
		s.renames.drop(pane)
	}
	return res
}

// KillAction is what kill runs: both steps guarded by the agent's pid
// and start time.
func KillAction(pane string, x sample.Session) advice.Action {
	steps := proto.KillSteps(pane, x.PID)
	g := advice.Guard{PID: x.PID, StartTicks: x.Start}
	guards := make([]advice.Guard, len(steps))
	for i := range guards {
		guards[i] = g
	}
	return advice.Action{Label: "kill", Steps: steps, Guards: guards, Destructive: true, Each: true}
}

// killGo tells if the stoker's kill of pane may go: the order gave kills,
// or the builder's y gave the go for this one, which it uses up.
func (s *Service) killGo(from, pane string) bool {
	ok := false
	_ = s.updateDriven(func(d *stoker.Driven, grants []string) bool {
		if slices.Contains(grants, "kill") {
			ok = true
			return false
		}
		need := "kill:" + pane
		i := slices.Index(d.Steps[from], need)
		if i < 0 {
			return false
		}
		d.Steps[from], ok = slices.Delete(d.Steps[from], i, i+1), true
		return true
	})
	return ok
}

// askKill puts the stoker's kill on the card as a step that waits for the
// builder's y, once while it waits.
func (s *Service) askKill(from, pane string, x sample.Session) {
	if s.Stoker == nil {
		return
	}
	who := "stoker"
	if !s.fromStoker(proto.Act{From: from}) {
		if y, ok := s.sessionAt(from); ok {
			who = y.Name
		}
	}
	now := s.now()
	need := "kill:" + pane
	want := "matchblox kill " + x.Name
	e := stoker.Entry{
		ID: "go@" + now.UTC().Format(time.RFC3339Nano) + "/" + need, At: now, Session: x.SessionID, Name: who, Pane: from,
		Rule: stoker.RuleGo, State: stoker.StepWaits, Needs: need, Want: want, Argv: [][]string{{want}},
		Text: who + " wants to kill " + x.Name,
	}
	if stoker.LastGo(s.Stoker.View().Run, e).State == stoker.StepWaits {
		return
	}
	if s.Stoker.Append(e) == nil {
		s.refreshStoker()
	}
}

// switchAgent starts another agent in the session's place, its name, cwd
// and pane: one respawn-pane -k ends the agent there and starts the new
// one in the same pane, guarded by the old agent's pid and start time. It
// never closes the pane, and the new agent is named as the old one was
// (seatWindow, and Claude's --name), on the account it was last started on
// (switchAccount), its env on the command; the switch does not change that
// choice. A hold on the seat waits for the new agent's
// session id and moves to it (Rebind); a guard that stops the switch, or
// a respawn that fails, puts it back as it was. The builder takes the seat
// back from the stoker, which drives it no more, and the old agent's
// queue state goes with it. Like kill, a session that works or waits
// takes a second confirm (Anyway).
func (s *Service) switchAgent(ctx context.Context, pane string, a proto.Act) proto.Result {
	if a.Confirm != "y" {
		return refuseText("needs confirm")
	}
	var n proto.SwitchAgent
	if err := json.Unmarshal([]byte(a.Text), &n); err != nil {
		return refuseText("the switch does not read: " + err.Error())
	}
	if n.Agent == "cursor" {
		n.Agent = sample.CursorAgent
	}
	x, ok := s.sessionAt(pane)
	switch {
	case ok && x.Agent == n.Agent:
		return refuseText(x.Name + " runs " + n.Agent + " already")
	case !slices.Contains(Startable, n.Agent):
		return refuseText(n.Agent + " is not an agent matchblox starts: claude, codex, opencode or cursor-agent")
	case s.agentBin(n.Agent) == "":
		return refuseText(n.Agent + " is not installed here")
	case !ok:
		return refuseText("no agent session in " + pane + " any more")
	case s.fromStoker(proto.Act{From: pane}):
		return refuseText("the stoker's session keeps its agent")
	case s.agentsPane(a):
		return refuseText("the stoker does not switch agents: the builder does")
	}
	if n.Handoff != "" {
		if err := proto.CheckFirst(n.Handoff, "the handoff"); err != nil {
			return refuse(err)
		}
	}
	if !s.isDir(x.Cwd) {
		return refuseText("no directory " + x.Cwd)
	}
	word := ""
	switch {
	case s.waitsOn(pane, x):
		word = "waits for you"
	case x.Works():
		word = "works"
	}
	if word != "" && !n.Anyway {
		return refuseText(x.Name + " " + word + ": y again switches it mid-turn")
	}
	// The account is held before the hold moves: a switch it refuses
	// leaves the seat as it was.
	acct := s.switchAccount(n.Agent)
	if n.Account != "" {
		if now := acct.label(); now != n.Account {
			return refuseText(fmt.Sprintf("the last %s account is now %s: switch again", n.Agent, now))
		}
	}
	argv := launchArgv(s.agentBin(n.Agent), n.Agent, x.Name, n.Handoff, accountArgs(n.Agent, acct))
	var notes []string
	if acct != nil {
		argv = slices.Concat(accounts.EnvArgv(n.Agent, acct.folder, false), argv)
		unlock, err := s.holdAccount(n.Agent, acct)
		if err != nil {
			return refuseUnlessRead(err)
		}
		defer unlock()
		if acct.again {
			// It switches on the account as it is when that fails.
			rep, err := accounts.Prepare(s.home(), n.Agent, acct.folder, acct.mode)
			notes = prepareNotes(acct, rep, err)
		}
	}
	h, held := s.heldIn(pane, x)
	if held {
		moving := h
		moving.Rebind = true
		if err := s.Holds.Set(moving); err != nil {
			return proto.Result{Err: err.Error()}
		}
	}
	release := s.holdPane(pane)
	act := SwitchAction(pane, x, s.seatWindow(pane, x), argv)
	s.mu.Lock()
	old := s.queue.HookedIn(pane)
	s.mu.Unlock()
	res := s.Actions.Do(ctx, act, "y")
	release()
	if res.Err != "" || len(res.Ran) < len(act.Steps) {
		switch {
		case res.Err == "":
			// A guard stopped a step, maybe after the rename ran: the old
			// agent still runs, and the switch did not happen.
			res.Err = strings.Join(res.Skipped, "; ")
			res.Skipped = nil
			res.Refused = len(res.Ran) == 0
		case len(res.Ran) == len(act.Steps)-1:
			res.Err += ": the pane may have no agent now"
		}
		if held {
			if err := s.Holds.Set(h); err != nil {
				res.Err = strings.TrimPrefix(res.Err+"; "+err.Error(), "; ")
			}
		}
		return res
	}
	res.Skipped = append(res.Skipped, notes...)
	s.undrive(pane)
	s.renames.drop(pane)
	s.mu.Lock()
	s.queue.Forget(old, s.now())
	s.cur.Queue = s.queue.Items()
	s.mu.Unlock()
	select {
	case s.sampleKick <- struct{}{}:
	default: // a sample is asked for already
	}
	return res
}

// switchAccount is the account a switch starts agent on: the one it was last
// started on, when that is an account folder that is there or a hand-made
// folder a live session runs on. Anything else, a Cursor account off macOS
// included, is nil: the default. It never makes an account.
func (s *Service) switchAccount(agent string) *onAccount {
	label := s.lastAccounts.of(lastAccountsPath(s.StatePath), agent)
	if label == "" || label == "default" || !slices.Contains(accountAgents, agent) {
		return nil
	}
	acct, err := s.account(proto.NewSession{Agent: agent, Account: label})
	if err != nil || acct == nil || acct.fresh {
		return nil
	}
	return acct
}

// seatWindow is the name the seat's window takes before a switch, so the
// new agent is named as the old one was: "" when tmux names the new agent
// so already. A window tmux names after its command gives way to the tmux
// session's name, and a window someone named keeps its name, as does one
// the seat shares with another pane.
func (s *Service) seatWindow(pane string, x sample.Session) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.cur.Panes, func(p sample.Pane) bool { return p.ID == pane })
	if i < 0 {
		return ""
	}
	p := s.cur.Panes[i]
	session, _, _ := strings.Cut(p.Target, ":")
	if x.Name == "" || x.Name == session || sample.WindowNamed(p, x.Agent) {
		return ""
	}
	server, _ := sample.SplitPane(pane)
	window := windowOf(p.Target)
	for _, o := range s.cur.Panes {
		if o.ID == pane {
			continue
		}
		if srv, _ := sample.SplitPane(o.ID); srv == server && windowOf(o.Target) == window {
			return ""
		}
	}
	return x.Name
}

// windowOf is the session:window of a pane's session:window.pane target.
func windowOf(target string) string {
	if i := strings.LastIndex(target, "."); i >= 0 {
		return target[:i]
	}
	return target
}

// SwitchAction is what switchAgent runs: one respawn-pane, after naming
// the window when window is set, each guarded by the old agent's pid and
// start time.
func SwitchAction(pane string, x sample.Session, window string, argv []string) advice.Action {
	server, bare := sample.SplitPane(pane)
	tmux := sample.TmuxOn(server)
	guard := advice.Guard{PID: x.PID, StartTicks: x.Start}
	a := advice.Action{Label: "switch", Destructive: true}
	if window != "" {
		a.Steps = append(a.Steps, slices.Concat(tmux, []string{"rename-window", "-t", bare, window}))
		a.Guards = append(a.Guards, guard)
	}
	a.Steps = append(a.Steps, slices.Concat(tmux,
		[]string{"respawn-pane", "-k", "-t", bare, "-c", x.Cwd, "env", "-u", "GIT_TERMINAL_PROMPT", "-u", "GIT_SSH_COMMAND", "-u", panes.HomeEnv},
		argv))
	a.Guards = append(a.Guards, guard)
	return a
}

// renameState keeps the renames that wait for their session's rest.
type renameState struct {
	mu   sync.Mutex
	wait map[string]pendingRename
}

type pendingRename struct {
	name string
	pid  int
}

func (r *renameState) set(pane string, p pendingRename) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.wait == nil {
		r.wait = map[string]pendingRename{}
	}
	r.wait[pane] = p
}

func (r *renameState) drop(pane string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.wait, pane)
}

func (r *renameState) all() map[string]pendingRename {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]pendingRename, len(r.wait))
	for k, v := range r.wait {
		out[k] = v
	}
	return out
}

// names are the names that wait, by pane. Nil: none.
func (r *renameState) names() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.wait) == 0 {
		return nil
	}
	out := make(map[string]string, len(r.wait))
	for pane, p := range r.wait {
		out[pane] = p.name
	}
	return out
}

// agentBin is the command that starts agent on this machine.
func (s *Service) agentBin(agent string) string {
	look := s.LookPath
	if look == nil {
		look = exec.LookPath
	}
	return agentBin(agent, look)
}

// startable are the agents of Startable installed here.
func (s *Service) startable() []string {
	look := s.LookPath
	if look == nil {
		look = exec.LookPath
	}
	var out []string
	for _, a := range Startable {
		if agentBin(a, look) != "" {
			out = append(out, a)
		}
	}
	return out
}

// rename names the session in pane: its window, its tmux session when the
// agent is that session's one window, and a Claude Code agent with
// /rename, so all of them read the same. Claude Code takes /rename only
// at rest, so a busy one is renamed, both sides at once, when it rests.
func (s *Service) rename(ctx context.Context, pane string, a proto.Act) proto.Result {
	name := strings.TrimSpace(a.Text)
	if err := proto.CheckName(name); err != nil {
		return proto.Result{Err: err.Error()}
	}
	if s.fromStoker(proto.Act{From: pane}) {
		return proto.Result{Err: "the stoker's session keeps its name"}
	}
	x, ok := s.sessionAt(pane)
	if !ok {
		return proto.Result{Err: "no agent session in " + pane + " any more"}
	}
	server, _ := sample.SplitPane(pane)
	if _, in := s.tmuxSessions(server)[name]; in && name != x.TmuxSession() {
		return proto.Result{Err: taken(name).Error()}
	}
	if x.Agent == "claude" && (!s.resting(pane) || s.waitsOn(pane, x)) {
		s.renames.set(pane, pendingRename{name: name, pid: x.PID})
		s.mu.Lock()
		s.cur.Renames = s.renames.names()
		s.mu.Unlock()
		s.broadcast()
		return proto.Result{Skipped: []string{x.Name + " renames when it rests"}}
	}
	s.renames.drop(pane)
	return s.doRename(ctx, pane, x, name)
}

func (s *Service) doRename(ctx context.Context, pane string, x sample.Session, name string) proto.Result {
	server, _ := sample.SplitPane(pane)
	one := s.tmuxSessions(server)[x.TmuxSession()] == 1
	release := s.holdPane(pane)
	defer release()
	return s.Actions.Do(ctx, RenameAction(pane, x, name, one), "y")
}

// RenameAction is what rename runs: the agent's /rename first, guarded
// as a message at rest, so a guard that stops it leaves tmux as it was.
func RenameAction(pane string, x sample.Session, name string, one bool) advice.Action {
	a := advice.Action{Label: "rename", Destructive: true, Steps: panes.RenameSteps(pane, x.Agent, name, one)}
	if x.Agent == "claude" {
		g := advice.Guard{MessagePane: pane, MessagePID: x.PID, RestPane: pane}
		a.Guards = []advice.Guard{g, g}
	}
	return a
}

// renameAtRest runs the renames whose session rests now. One whose agent
// is gone or replaced is dropped.
func (s *Service) renameAtRest(ctx context.Context) {
	for pane, p := range s.renames.all() {
		x, ok := s.sessionAt(pane)
		switch {
		case !ok || x.PID != p.pid:
			s.renames.drop(pane)
		case s.resting(pane) && !s.waitsOn(pane, x):
			s.renames.drop(pane)
			s.doRename(ctx, pane, x, p.name)
		}
	}
}

// newSession starts an agent in a tmux session of its own on the
// builder's server, never the stoker's: the session, its window and the
// agent share one name. With a branch it first adds a worktree for it
// next to the repository, branched from origin's default branch (or HEAD)
// and tracking nothing, so a bare push never goes to main. The stoker
// starts sessions only in a run, each a message toward max_steps, and the
// session it starts is one it drives from its first second.
func (s *Service) newSession(ctx context.Context, a proto.Act) proto.Result {
	var n proto.NewSession
	if err := json.Unmarshal([]byte(a.Text), &n); err != nil {
		return refuse(errors.New("the new session does not read: " + err.Error()))
	}
	if n.Agent == "cursor" {
		n.Agent = sample.CursorAgent
	}
	if s.agentsPane(a) && (n.NewAccount || n.Account != "" && n.Account != "default") {
		return refuse(errors.New("the stoker starts on default only"))
	}
	steps, name, dir, acct, err := s.newSteps(ctx, n)
	if err != nil {
		return refuseUnlessRead(err)
	}
	act := advice.Action{Label: "new session"}
	var launch []string
	first := n.First
	if acct != nil {
		launch = accounts.EnvArgv(n.Agent, acct.folder, acct.fresh)
		if acct.fresh {
			// The builder signs in in the session: no first message.
			first = ""
			act.Label = newAccountLabel(n.Agent, acct.name, string(acct.mode))
			act.Destructive = true
		}
	}
	launch = slices.Concat(launch, launchArgv(s.agentBin(n.Agent), n.Agent, name, first, accountArgs(n.Agent, acct)))
	steps = append(steps, slices.Concat(
		[]string{"env", "-u", "GIT_TERMINAL_PROMPT", "-u", "GIT_SSH_COMMAND", "-u", panes.HomeEnv},
		sample.TmuxOn(""), []string{"new-session", "-d", "-s", name, "-n", name, "-c", dir},
		launch))
	act.Steps = steps
	// made: this call makes the folder, and takes it away when the start
	// fails, so a retry finds no account.
	made := false
	if acct != nil {
		unlock, err := s.holdAccount(n.Agent, acct)
		if err != nil {
			return refuseUnlessRead(err)
		}
		defer unlock()
	}
	// Only a confirmed act makes a folder: a dry plan leaves the disk as it
	// was. A like-default account gets what the default gained since; when
	// that fails, it starts as it is. What a copy left shared or out, and
	// whether it ended, goes in the result as notes.
	var notes []string
	switch {
	case acct != nil && acct.fresh && a.Confirm == "y":
		_, err := os.Lstat(acct.folder)
		made = errors.Is(err, fs.ErrNotExist)
		rep, err := accounts.Prepare(s.home(), n.Agent, acct.folder, acct.mode)
		if err != nil {
			s.unmake(made, acct.folder)
			return proto.Result{Err: err.Error()}
		}
		notes = prepareNotes(acct, rep, nil)
	case acct != nil && acct.again:
		rep, err := accounts.Prepare(s.home(), n.Agent, acct.folder, acct.mode)
		notes = prepareNotes(acct, rep, err)
	}
	res := s.Actions.Do(ctx, act, a.Confirm)
	if res.Err != "" || len(res.Ran) < len(steps) {
		s.unmake(made, acct.folderOrEmpty())
		return res
	}
	res.Skipped = append(res.Skipped, notes...)
	var pane string
	if s.agentsPane(a) || (acct != nil && acct.fresh) {
		out, _ := s.tmuxOut(slices.Concat(sample.TmuxOn(""), []string{"display-message", "-p", "-t", "=" + name + ":", "#{pane_id}"}))
		pane = strings.TrimSpace(out)
	}
	if s.agentsPane(a) {
		s.recordNew(pane, name, res.Ran)
	} else {
		s.rememberStart(n, acct) // the stoker's starts are not the builder's choice
	}
	switch {
	case acct == nil || !acct.fresh:
	case pane != "":
		go s.driveSignIn(ctx, pane, n.Agent, name)
	default:
		// No pane to drive: the builder signs in by hand, told by the
		// session's name.
		s.signIns.set(name, SignInByHand)
		s.noteSignIn()
	}
	select {
	case s.sampleKick <- struct{}{}:
	default: // a sample is asked for already
	}
	return res
}

// newSteps checks a new session and builds the worktree's step, if any.
// It returns the session's name and the directory it starts in.
func (s *Service) newSteps(ctx context.Context, n proto.NewSession) (steps [][]string, name, dir string, acct *onAccount, err error) {
	look := s.LookPath
	if look == nil {
		look = exec.LookPath
	}
	switch {
	case !slices.Contains(Startable, n.Agent):
		return nil, "", "", nil, fmt.Errorf("%s is not an agent matchblox starts: claude, codex, opencode or cursor-agent", n.Agent)
	}
	if acct, err = s.account(n); err != nil {
		return nil, "", "", nil, err
	}
	if rest, ok := strings.CutPrefix(n.Where, "~/"); ok || n.Where == "~" {
		n.Where = filepath.Join(s.home(), rest)
	}
	if !filepath.IsAbs(n.Where) {
		return nil, "", "", nil, errors.New("a new session starts in a full path")
	}
	if agentBin(n.Agent, look) == "" {
		return nil, "", "", nil, fmt.Errorf("%s is not installed here", n.Agent)
	}
	if n.First != "" {
		if err := proto.CheckFirst(n.First, "the first message"); errors.Is(err, proto.ErrDash) {
			return nil, "", "", nil, errors.New("a first message does not start with -")
		} else if err != nil {
			return nil, "", "", nil, err
		}
	}
	dir = filepath.Clean(n.Where)
	if !s.isDir(dir) {
		return nil, "", "", nil, fmt.Errorf("no directory %s", dir)
	}
	if n.Branch != "" {
		step, wt, err := worktreeStep(ctx, dir, n.Branch)
		if err != nil {
			return nil, "", "", nil, err
		}
		steps, dir = append(steps, step), wt
	}
	name = n.Name
	if name == "" {
		name = proto.NameOf(filepath.Base(dir))
	}
	if err := proto.CheckName(name); err != nil {
		return nil, "", "", nil, err
	}
	if _, in := s.tmuxSessions("")[name]; in {
		return nil, "", "", nil, taken(name)
	}
	return steps, name, dir, acct, nil
}

// onAccount is the account a new session starts on: its folder, whether it
// is made now, and how it is set up. again: an existing like-default
// account, or a copy that did not end, that is prepared again before it
// starts.
type onAccount struct {
	name, folder string
	fresh, again bool
	hand         bool // a hand-made folder a live session runs on
	mode         accounts.Mode
}

// label is the account's name: "default" for nil.
func (a *onAccount) label() string {
	if a == nil {
		return "default"
	}
	return a.name
}

// errBusy, errGone and errThere are holdAccount's refusals.
var (
	errBusy  = errors.New("being set up")
	errGone  = errors.New("gone")
	errThere = errors.New("there already")
)

// holdAccount holds acct's folder until the returned func is called. It
// never waits: a folder another start holds is refused at once. Under the
// hold it checks the account again, as the check before may be old: a new
// account another start made since is there already, and an account that
// was there and is gone since (a new account whose start failed took it
// away) is refused. A hand-made folder is checked by its live session
// only, as account does.
func (s *Service) holdAccount(agent string, acct *onAccount) (func(), error) {
	m, _ := s.accountLocks.LoadOrStore(acct.folder, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	if !mu.TryLock() {
		return nil, fmt.Errorf("the %s account %s is %w: start it when that ends", agent, acct.name, errBusy)
	}
	if acct.hand {
		return mu.Unlock, nil // checked by its live session, not on disk
	}
	have, _, err := accounts.List(s.home(), agent)
	switch there := slices.Contains(have, acct.name); {
	case err != nil:
	case acct.fresh && there:
		err = alreadyThere(agent, acct.name)
	case !acct.fresh && !there:
		err = fmt.Errorf("the %s account %s is %w", agent, acct.name, errGone)
	}
	if err != nil {
		mu.Unlock()
		return nil, err
	}
	return mu.Unlock, nil
}

// refuse is the result of a start or a switch the service refused:
// nothing ran.
func refuse(err error) proto.Result {
	return proto.Result{Err: err.Error(), Refused: true}
}

func refuseText(why string) proto.Result { return refuse(errors.New(why)) }

// refuseUnlessRead refuses for err, unless err is a file that did not read
// (the accounts folder): that is a failure, not a refusal.
func refuseUnlessRead(err error) proto.Result {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return proto.Result{Err: err.Error()}
	}
	return refuse(err)
}

// leaves are the errors err joins, each on its own; err itself when it
// joins none.
func leaves(err error) []string {
	j, ok := err.(interface{ Unwrap() []error }) //nolint:errorlint // a join's own errors, not a wrapped one
	if !ok {
		return []string{strings.ReplaceAll(err.Error(), "\n", "; ")}
	}
	var out []string
	for _, e := range j.Unwrap() {
		if e != nil {
			out = append(out, leaves(e)...)
		}
	}
	return out
}

// prepareNotes are the notes of a start on acct whose Prepare gave rep and
// err: each link a copy left on the default, each entry it left out, five
// of each at most, what failed, and whether the copy has not ended.
func prepareNotes(acct *onAccount, rep accounts.Report, err error) []string {
	notes := proto.Notes(proto.StillShared, rep.Shared)
	skipped := make([]string, 0, len(rep.Skipped))
	for _, k := range rep.Skipped {
		skipped = append(skipped, k.Path+": "+k.Why)
	}
	notes = append(notes, proto.Notes(proto.NotCopied, skipped)...)
	if err != nil {
		prefix := proto.NotLinked
		if acct.mode == accounts.Copy {
			prefix = proto.NotCopied
		}
		notes = append(notes, proto.Notes(prefix, leaves(err))...)
	}
	if acct.mode == accounts.Copy && accounts.Incomplete(acct.folder) {
		notes = append(notes, proto.CopyGoesOn)
	}
	return notes
}

// alreadyThere refuses a new account whose folder is there.
func alreadyThere(agent, name string) error {
	return fmt.Errorf("the %s account %s is %w: start on it", agent, name, errThere)
}

func (a *onAccount) folderOrEmpty() string {
	if a == nil {
		return ""
	}
	return a.folder
}

// unmake takes away the folder of a new account that did not start, when
// this call made it.
func (s *Service) unmake(made bool, folder string) {
	if made && folder != "" {
		_ = os.RemoveAll(folder) // links go, never what they point to
	}
}

// accountArgs is what the agent takes after its binary on acct. A
// hand-made folder takes none: it may keep its sign-in in the keyring.
func accountArgs(agent string, acct *onAccount) []string {
	if acct == nil || acct.hand {
		return nil
	}
	return accounts.Args(agent)
}

// newAccountLabel is what the console's confirm shows for a new account.
func newAccountLabel(agent, name, setup string) string {
	return fmt.Sprintf("new %s account %q, %s: sign in in the session", agent, name, setup)
}

// accountAgents are the agents that have accounts.
var accountAgents = []string{"claude", "codex", sample.CursorAgent}

// knownAccounts lists, per agent with accounts, what a new session can pick:
// default, the folders under the root, then the hand-made folders live
// sessions run on, deduplicated. It also returns, per agent, the folders it
// left out and why; an agent with none has no entry. The answer is kept
// while the accounts folders' mtimes and the live sessions' folders are
// as they were, so a sample reads no folder.
func (s *Service) knownAccounts(sessions []sample.Session) (map[string][]string, map[string][]accounts.Skip) {
	home := s.home()
	root := accounts.Root(home)
	sig, fresh := s.accountsSig(home, sessions)
	s.knownMu.Lock()
	defer s.knownMu.Unlock()
	if s.known.sig == sig && s.known.out != nil && !fresh {
		return s.known.out, s.known.skipped
	}
	out := make(map[string][]string, len(accountAgents))
	var skipped map[string][]accounts.Skip
	for _, agent := range accountAgents {
		if agent == sample.CursorAgent && s.goos() != "darwin" {
			continue // Cursor accounts run on macOS only: no field
		}
		list := []string{"default"}
		have, skips, _ := accounts.ListWith(s.readDirOrOS(), home, agent) // unreadable: default only, and named
		list = append(list, have...)
		if len(skips) > 0 {
			if skipped == nil {
				skipped = map[string][]accounts.Skip{}
			}
			skipped[agent] = skips
		}
		for _, x := range sessions {
			if x.Agent != agent || x.AccountDir == "" || accounts.Under(root, x.AccountDir) {
				continue
			}
			if l := s.folderLabel(x.AccountDir); !slices.Contains(list, l) {
				list = append(list, l)
			}
		}
		out[agent] = list
	}
	s.known.sig, s.known.out, s.known.skipped = sig, out, skipped
	return out, skipped
}

// knownCache is the last answer of knownAccounts and what it was made from.
type knownCache struct {
	sig     string
	out     map[string][]string
	skipped map[string][]accounts.Skip
}

// recent is how long a folder's mtime may be too new to trust: a change
// within the clock's tick of a read leaves the same mtime.
const recent = 2 * time.Second

// accountsSig is what knownAccounts depends on without reading a folder:
// the mtime of each agent's accounts root, and the folders live sessions
// run on. fresh is true when a root changed within recent, so the answer
// must not be kept.
func (s *Service) accountsSig(home string, sessions []sample.Session) (sig string, fresh bool) {
	var b strings.Builder
	b.WriteString(home + "\x00" + s.goos())
	for _, agent := range accountAgents {
		b.WriteString("\x00" + agent + "=")
		fi, err := os.Stat(filepath.Join(accounts.Root(home), agent))
		if err != nil {
			b.WriteString(err.Error())
			continue
		}
		fmt.Fprint(&b, fi.ModTime().UnixNano())
		fresh = fresh || time.Since(fi.ModTime()) < recent
	}
	var dirs []string
	for _, x := range sessions {
		if x.AccountDir != "" {
			dirs = append(dirs, x.Agent+"\x00"+x.AccountDir)
		}
	}
	slices.Sort(dirs)
	b.WriteString("\x00" + strings.Join(slices.Compact(dirs), "\x01"))
	return b.String(), fresh
}

// folderLabel is a hand-made folder as the console lists and sends it: with
// "~/" when under home, else its path.
func (s *Service) folderLabel(dir string) string {
	if rest, ok := strings.CutPrefix(dir, s.home()+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return dir
}

// liveFolder is the folder a hand-made account label, or its full path,
// names, when a live session of agent in the last snapshot runs on it.
func (s *Service) liveFolder(agent, label string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.sampled {
		if x.Agent != agent || x.AccountDir == "" {
			continue
		}
		// The console sends the label; a shell sends the full path.
		if s.folderLabel(x.AccountDir) == label || filepath.IsAbs(label) && accounts.SameDir(label, x.AccountDir) {
			return x.AccountDir, true
		}
	}
	return "", false
}

func (s *Service) goos() string {
	if s.GOOS != "" {
		return s.GOOS
	}
	return runtime.GOOS
}

func (s *Service) home() string {
	if s.Home != "" {
		return s.Home
	}
	h, _ := os.UserHomeDir()
	return h
}

// account checks the account a new session asks for: nil is the builder's
// default sign-in. An account that is not there needs NewAccount, and a
// new one must not be there yet. Nothing is written.
func (s *Service) account(n proto.NewSession) (*onAccount, error) {
	if n.Account == "" || (n.Account == "default" && !n.NewAccount) {
		return nil, nil
	}
	if n.Agent == "opencode" {
		return nil, errors.New("opencode has no accounts")
	}
	goos := s.goos()
	if n.Agent == sample.CursorAgent && goos != "darwin" {
		// On Linux the account's HOME links ~/.config, where the default
		// keeps its sign-in.
		return nil, errors.New("Cursor accounts run on macOS only for now") //nolint:staticcheck // Cursor is a name
	}
	if strings.HasPrefix(n.Account, "~/") || strings.HasPrefix(n.Account, "/") {
		// A hand-made folder is never made here: only one a live session runs on.
		folder, ok := s.liveFolder(n.Agent, n.Account)
		if n.NewAccount || !ok {
			return nil, fmt.Errorf("no %s account %s", n.Agent, n.Account)
		}
		return &onAccount{name: n.Account, folder: folder, hand: true}, nil
	}
	home := s.home()
	if err := accounts.CheckName(n.Agent, n.Account, home, goos); err != nil {
		return nil, err
	}
	acct := &onAccount{name: n.Account, folder: accounts.Folder(home, n.Agent, n.Account), fresh: n.NewAccount}
	have, _, err := accounts.List(home, n.Agent)
	if err != nil {
		return nil, err
	}
	there := slices.Contains(have, n.Account)
	switch {
	case !n.NewAccount && !there:
		return nil, fmt.Errorf("no %s account %s: start it as a new account", n.Agent, n.Account)
	case n.NewAccount && there:
		return nil, alreadyThere(n.Agent, n.Account)
	case !n.NewAccount:
		switch setup := accounts.SetupOf(acct.folder); {
		case setup == accounts.LikeDefault:
			acct.again, acct.mode = true, accounts.LikeDefault
		case setup == accounts.Copy && accounts.Incomplete(acct.folder):
			// A copy that did not end goes on where it stopped.
			acct.again, acct.mode = true, accounts.Copy
		}
		return acct, nil
	}
	switch mode := accounts.Mode(n.Setup); mode {
	case "":
		acct.mode = accounts.LikeDefault
	case accounts.LikeDefault, accounts.Copy, accounts.Empty:
		acct.mode = mode
	default:
		return nil, fmt.Errorf("the setup %q is none of like-default, copy or empty", n.Setup)
	}
	return acct, nil
}

// worktreeStep adds a worktree for branch beside the repository dir is
// in: <repo>-<branch>, from origin's default branch, or HEAD with no
// origin.
func worktreeStep(ctx context.Context, dir, branch string) ([]string, string, error) {
	git := func(args ...string) (string, error) {
		out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output() //nolint:gosec // argv, never a shell string
		return strings.TrimSpace(string(out)), err
	}
	root, err := git("rev-parse", "--show-toplevel")
	if err != nil || root == "" {
		return nil, "", fmt.Errorf("a new worktree needs a git repository: %s is none", dir)
	}
	if _, err := git("check-ref-format", "--branch", branch); err != nil {
		return nil, "", fmt.Errorf("%s is no branch name", branch)
	}
	if _, err := git("rev-parse", "--verify", "-q", "refs/heads/"+branch); err == nil {
		return nil, "", fmt.Errorf("the branch %s is there already: start the session in its worktree", branch)
	}
	wt := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-"+proto.NameOf(branch))
	if _, err := os.Stat(wt); err == nil {
		return nil, "", fmt.Errorf("%s is there already", wt)
	}
	base := "HEAD"
	if _, err := git("rev-parse", "--verify", "-q", "refs/remotes/origin/HEAD"); err == nil {
		base = "origin/HEAD"
	}
	return []string{"git", "-C", root, "worktree", "add", "--no-track", "-b", branch, wt, base}, wt, nil
}

// recordNew keeps a session the stoker started in the run: it counts as a
// message, and the stoker drives its pane, so the guard watches it.
func (s *Service) recordNew(pane, name string, ran [][]string) {
	if s.Stoker == nil || pane == "" {
		return
	}
	now := s.now()
	_ = s.Stoker.Append(stoker.Entry{
		ID: name + "@" + now.UTC().Format(time.RFC3339Nano), At: now,
		Name: name, Pane: pane, Rule: stoker.RuleOrder, Argv: ran, Result: stoker.Sent,
	})
	s.drive(pane)
	s.refreshStoker()
}

func (s *Service) readDirOrOS() func(string) ([]os.DirEntry, error) {
	if s.readDir != nil {
		return s.readDir
	}
	return os.ReadDir
}
