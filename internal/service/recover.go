package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

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

// resumeArgs is what agent takes after its binary to open conversation id
// again, with no prompt: it waits for the builder.
func resumeArgs(agent, id string) []string {
	if agent == "codex" {
		return []string{"resume", id}
	}
	return []string{"--resume", id} // claude, cursor-agent
}

// recordable are the agents of the builder's server that recover can
// reopen, as recovery entries, and the pid of each server sampled, by its
// socket. An agent with no conversation id yet has nothing to reopen.
func recordable(sessions []sample.Session, ps []sample.Pane) ([]recovery.Entry, map[string]string) {
	alive := map[string]string{}
	byID := map[string]sample.Pane{}
	for _, p := range ps {
		if server, _ := sample.SplitPane(p.ID); server != "" || p.Socket == "" {
			continue
		}
		alive[p.Socket] = p.Server
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
			Cwd: x.Cwd, AccountDir: x.AccountDir, Socket: p.Socket, ServerPID: p.Server,
		})
	}
	return live, alive
}

// recover answers a recover act: each entry of the record and its state.
// With All or Names it reopens the entries that can, each in a tmux
// session of its own on the builder's server. It runs no git command that
// changes anything, and starts no turn.
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
	live, taken := map[string]bool{}, map[string]bool{}
	s.mu.Lock()
	for _, x := range slices.Concat(s.cur.Sessions, s.cur.Children) {
		if x.SessionID != "" {
			live[x.Agent+"\x00"+x.SessionID] = true
		}
	}
	for _, p := range s.cur.Panes {
		if server, _ := sample.SplitPane(p.ID); server == "" {
			name, _, _ := strings.Cut(p.Target, ":")
			taken[name] = true
		}
	}
	s.mu.Unlock()
	var rows []proto.Recovered
	for _, e := range entries {
		row, argv := s.recoverRow(e, r.Dirs[e.Session], live, taken)
		if argv != nil && (r.All || slices.Contains(r.Names, e.Session)) {
			res := s.Actions.Do(ctx, advice.Action{Label: "recover " + e.Session, Steps: [][]string{argv}}, "y")
			if res.Err != "" {
				row.State, row.Why = proto.RecoverCannot, "tmux did not start it ("+res.Err+"): run matchblox recover "+e.Session+" again"
			} else {
				row.State = proto.RecoverReopened
				taken[row.As] = true
				live[e.Agent+"\x00"+e.ConversationID] = true
			}
		}
		rows = append(rows, row)
	}
	return proto.Result{Recover: rows}
}

// recoverRow is the state of entry e, and the argv that reopens it when it
// can (nil when it cannot). moved is the folder the builder gave for it
// with --dir.
func (s *Service) recoverRow(e recovery.Entry, moved string, live, taken map[string]bool) (proto.Recovered, []string) {
	row := proto.Recovered{Name: e.Session, Agent: e.Agent, Conversation: e.ConversationID, Dir: e.Cwd}
	cannot := func(format string, args ...any) (proto.Recovered, []string) {
		row.State, row.Why = proto.RecoverCannot, fmt.Sprintf(format, args...)
		return row, nil
	}
	if live[e.Agent+"\x00"+e.ConversationID] {
		row.State = proto.RecoverLive
		return row, nil
	}
	if moved != "" {
		row.Dir = moved
	}
	bin := s.agentBin(e.Agent)
	if bin == "" {
		return cannot("%s is not installed here: install it, then run matchblox recover again", e.Agent)
	}
	if st, err := os.Stat(row.Dir); err != nil || !st.IsDir() {
		return cannot("the folder %s is gone: run matchblox recover %s --dir <its new place>", row.Dir, e.Session)
	}
	if e.Repo != "" {
		root, branch := s.Recovery.CheckoutOf(row.Dir)
		switch {
		case root == "":
			return cannot("%s is no git checkout now: run matchblox recover %s --dir <the checkout of %s>", row.Dir, e.Session, orNone(e.Branch))
		case branch != e.Branch:
			return cannot("%s is on branch %s now, not %s: check out %s there, then run matchblox recover again", row.Dir, orNone(branch), orNone(e.Branch), orNone(e.Branch))
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
	if e.AccountDir != "" {
		argv = slices.Concat(argv, accounts.EnvArgv(e.Agent, e.AccountDir, false), []string{bin})
		if strings.HasPrefix(e.AccountDir, accounts.Root(s.home())+string(filepath.Separator)) {
			argv = append(argv, accounts.Args(e.Agent)...) // a hand-made folder may sign in by the keyring
		}
	} else {
		argv = append(argv, bin)
	}
	return row, append(argv, resumeArgs(e.Agent, e.ConversationID)...)
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
