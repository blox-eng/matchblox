package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/accounts"
	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/recovery"
	"github.com/blox-eng/matchblox/internal/sample"
)

// RecoverID is the act that previews or reopens the conversations of the
// recovery record; its Text is a proto.Recover.
const RecoverID = "recover"

// recoverable are the agents recover reopens, each by its resume flag.
var recoverable = []string{"claude", "codex", sample.CursorAgent}

// recoverSettle is how long an agent recover started must run before recover
// says it reopened: one that exits at once took its tmux session with it.
const recoverSettle = 1500 * time.Millisecond

// reopenedFor is how long a reopened conversation counts as live before a
// sample shows its agent: a second recover in that time starts nothing.
const reopenedFor = time.Minute

// resumeArgs is what agent takes after its binary to open conversation id
// again, with no prompt: it waits for the builder.
func resumeArgs(agent, id string) []string {
	if agent == "codex" {
		return []string{"resume", id}
	}
	return []string{"--resume", id} // claude, cursor-agent
}

// conversation keys a conversation across the record and the sessions.
func conversation(agent, id string) string { return agent + "\x00" + id }

// recordable are the agents of the builder's server that recover can
// reopen, as recovery entries, and the identity of each server sampled, by
// its socket. An agent with no conversation id yet has nothing to reopen.
func recordable(sessions []sample.Session, ps []sample.Pane) ([]recovery.Entry, map[string]string) {
	alive := map[string]string{}
	byID := map[string]sample.Pane{}
	for _, p := range ps {
		if server, _ := sample.SplitPane(p.ID); server != "" || p.Socket == "" {
			continue
		}
		alive[p.Socket] = recovery.Server(p.Server, p.ServerStart)
		byID[p.ID] = p
	}
	var live []recovery.Entry
	for _, x := range sessions {
		p, ok := byID[x.Pane]
		if !ok || x.SessionID == "" || !slices.Contains(recoverable, x.Agent) {
			continue
		}
		live = append(live, recovery.Entry{
			Agent: x.Agent, ConversationID: x.SessionID, Session: x.TmuxSession(), Pane: x.Pane,
			Cwd: x.Cwd, AccountDir: x.AccountDir, Provider: x.Provider, APIKey: x.Account == "API key",
			Socket: p.Socket, ServerPID: p.Server, ServerStart: p.ServerStart,
		})
	}
	return live, alive
}

// recover answers a recover act: each entry of the record (or of the named
// sessions) and its state. With All or Names it reopens the entries that
// can, each in a tmux session of its own on the builder's server, and says
// reopened only for an agent that still runs after recoverSettle. It runs
// no git command that changes anything, and starts no turn.
func (s *Service) recover(ctx context.Context, a proto.Act) proto.Result {
	if s.Recovery == nil {
		return refuseText("matchblox keeps no recovery record: its state folder did not open (serve.log says why)")
	}
	var r proto.Recover
	if a.Text != "" {
		if err := json.Unmarshal([]byte(a.Text), &r); err != nil {
			return refuseText("recover: " + err.Error())
		}
	}
	reopen := r.All || len(r.Names) > 0
	if _, ok := s.sessionAt(a.From); ok && reopen {
		return proto.Result{Err: notTheBuilders}
	}
	s.recovering.Lock()
	defer s.recovering.Unlock()
	entries := s.Recovery.List()
	var unknown []string
	for _, n := range r.Names {
		if !slices.ContainsFunc(entries, func(e recovery.Entry) bool { return e.Session == n }) {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		return refuseText(fmt.Sprintf("no session %s in the recovery record: matchblox recover lists them", strings.Join(unknown, ", ")))
	}
	live, taken := s.recoverLive()
	var rows []proto.Recovered
	started := map[int]string{} // row → the agent's command, to check it runs
	for _, e := range entries {
		if len(r.Names) > 0 && !slices.Contains(r.Names, e.Session) {
			continue
		}
		row, argv, cmd := s.recoverRow(e, r.Dirs[e.Session], live, taken)
		if argv != nil && reopen {
			if res := s.Actions.Do(ctx, advice.Action{Label: "recover " + e.Session, Steps: [][]string{argv}}, "y"); res.Err != "" {
				row.State, row.Why = proto.RecoverCannot, "tmux did not start it ("+res.Err+"): run matchblox recover "+e.Session+" again"
			} else {
				taken[row.As] = true
				live[conversation(e.Agent, e.ConversationID)] = true
				started[len(rows)] = cmd
			}
		}
		rows = append(rows, row)
	}
	if len(started) > 0 {
		s.settled(ctx, rows, started)
	}
	return proto.Result{Recover: rows}
}

// recoverLive are the conversations that run now (or that recover reopened
// within reopenedFor), and the tmux session names taken on the builder's
// server.
func (s *Service) recoverLive() (live, taken map[string]bool) {
	live, taken = map[string]bool{}, map[string]bool{}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range slices.Concat(s.cur.Sessions, s.cur.Children) {
		if x.SessionID != "" {
			live[conversation(x.Agent, x.SessionID)] = true
		}
	}
	for c, at := range s.reopened {
		if live[c] || s.now().Sub(at) >= reopenedFor {
			delete(s.reopened, c) // a sample shows it, or it is old
			continue
		}
		live[c] = true
	}
	for _, p := range s.cur.Panes {
		if server, _ := sample.SplitPane(p.ID); server == "" {
			name, _, _ := strings.Cut(p.Target, ":")
			taken[name] = true
		}
	}
	return live, taken
}

// settled waits recoverSettle, then says reopened for each started row
// whose tmux session still runs, and cannot for one whose agent exited.
func (s *Service) settled(ctx context.Context, rows []proto.Recovered, started map[int]string) {
	wait := s.settle
	if wait == 0 {
		wait = recoverSettle
	}
	select {
	case <-ctx.Done():
	case <-time.After(wait):
	}
	s.mu.Lock()
	if s.reopened == nil {
		s.reopened = map[string]time.Time{}
	}
	s.mu.Unlock()
	for i, cmd := range started {
		row := &rows[i]
		if _, err := s.tmuxOut(slices.Concat(sample.TmuxOn(""), []string{"has-session", "-t", "=" + row.As})); err != nil {
			row.State, row.Why = proto.RecoverCannot, fmt.Sprintf("%s exited at once: run cd %s && %s to see why", row.Agent, row.Dir, cmd)
			continue
		}
		row.State = proto.RecoverReopened
		s.mu.Lock()
		s.reopened[conversation(row.Agent, row.Conversation)] = s.now()
		s.mu.Unlock()
	}
}

// recoverRow is the state of entry e, the argv that reopens it when it can
// (nil when it cannot), and the agent's own command in it. moved is the
// folder the builder gave for it with --dir.
func (s *Service) recoverRow(e recovery.Entry, moved string, live, taken map[string]bool) (proto.Recovered, []string, string) {
	row := proto.Recovered{Name: e.Session, Agent: e.Agent, Conversation: e.ConversationID, Dir: e.Cwd}
	cannot := func(format string, args ...any) (proto.Recovered, []string, string) {
		row.State, row.Why = proto.RecoverCannot, fmt.Sprintf(format, args...)
		return row, nil, ""
	}
	if live[conversation(e.Agent, e.ConversationID)] {
		row.State = proto.RecoverLive
		return row, nil, ""
	}
	if moved != "" {
		if !filepath.IsAbs(moved) {
			return cannot("--dir %s is not a full path: give the folder from /", moved)
		}
		row.Dir = moved
	}
	resume := strings.Join(slices.Concat([]string{e.Agent}, resumeArgs(e.Agent, e.ConversationID)), " ")
	switch {
	case e.Provider == "bedrock" || e.Provider == "vertex":
		return cannot("it ran on %s through your shell's environment, which matchblox does not keep: run cd %s && %s in that shell", e.Provider, row.Dir, resume)
	case e.APIKey:
		return cannot("it ran on an API key, which matchblox does not keep: run cd %s && %s where that key is set", row.Dir, resume)
	}
	bin := s.agentBin(e.Agent)
	if bin == "" {
		return cannot("%s is not installed here: install it, then run matchblox recover again", e.Agent)
	}
	if st, err := os.Stat(row.Dir); err != nil || !st.IsDir() {
		return cannot("the folder %s is gone: run matchblox recover %s --dir <its new place>", row.Dir, e.Session)
	}
	if e.Repo != "" {
		c := s.Recovery.CheckoutOf(row.Dir)
		switch {
		case c.Root == "":
			return cannot("%s is no git checkout now: run matchblox recover %s --dir <the checkout of %s>", row.Dir, e.Session, orNone(e.Branch))
		case e.Common != "" && c.Common != e.Common:
			return cannot("%s is another repository than %s: run matchblox recover %s --dir <a checkout of %s>", row.Dir, e.Repo, e.Session, e.Repo)
		case c.Branch != e.Branch:
			return cannot("%s is on branch %s now, not %s: check out %s there, then run matchblox recover again", row.Dir, orNone(c.Branch), orNone(e.Branch), orNone(e.Branch))
		}
	}
	config := e.AccountDir
	if config == "" {
		config = accounts.DefaultDir(s.home(), e.Agent)
	}
	path, checked := recovery.History(e.Agent, config, e.ConversationID)
	switch {
	case !checked:
		row.Why = "history not checked: matchblox does not read Cursor's history"
	case path == "":
		return cannot("no saved history of conversation %s in %s: run matchblox new --where %s for a new session", e.ConversationID, config, row.Dir)
	}
	if pid := recovery.OpenIn(e.Agent, config, e.ConversationID); pid > 0 {
		return cannot("conversation %s is open in another process (pid %d): quit it there, then run matchblox recover again", e.ConversationID, pid)
	}
	row.State, row.As = proto.RecoverReady, freeName(e.Session, taken)
	argv := slices.Concat(
		[]string{"env", "-u", "GIT_TERMINAL_PROMPT", "-u", "GIT_SSH_COMMAND", "-u", panes.HomeEnv},
		sample.TmuxOn(""), []string{"new-session", "-d", "-s", row.As, "-n", row.As, "-c", row.Dir})
	agent := []string{bin}
	if e.AccountDir != "" {
		argv = append(argv, accounts.EnvArgv(e.Agent, e.AccountDir, false)...)
		if strings.HasPrefix(e.AccountDir, accounts.Root(s.home())+string(filepath.Separator)) {
			agent = append(agent, accounts.Args(e.Agent)...) // a hand-made folder may sign in by the keyring
		}
	}
	agent = append(agent, resumeArgs(e.Agent, e.ConversationID)...)
	return row, append(argv, agent...), strings.Join(agent, " ")
}

// freeName is name, or name-2, name-3, … when a tmux session has it.
func freeName(name string, taken map[string]bool) string {
	free := name
	for i := 2; taken[free]; i++ {
		free = fmt.Sprintf("%s-%d", name, i)
	}
	return free
}

func orNone(branch string) string {
	if branch == "" {
		return "(no branch)"
	}
	return branch
}
