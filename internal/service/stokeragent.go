package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// The stoker agent runs on a tmux server of matchblox's own
// (StokerServer), in its unit's session (stoker.SessionName): matchblox starts it
// when the builder launches the stoker at a console, keeps it running
// when the stoker turns off, so its context carries to the next order,
// and ends it on stoker quit.

// stokerStartsIn bounds how long the stoker shows as starting: an agent
// that never comes up shows as not running again.
const stokerStartsIn = 2 * time.Minute

// agentOn: there is a stoker agent to start and type into.
func (s *Service) agentOn() bool { return s.StokerServer != "" && s.agentMissing() == "" }

// agentArgv is the stoker agent's command ([stoker] agent), as argv: tmux
// runs it with no shell.
func (s *Service) agentArgv() []string {
	if f := strings.Fields(s.StokerAgent); len(f) > 0 {
		return f
	}
	return []string{"claude"}
}

// agentMissing is why the stoker agent cannot start: its command is not
// on PATH. "": it can.
func (s *Service) agentMissing() string {
	look := s.LookPath
	if look == nil {
		look = exec.LookPath
	}
	cmd := s.agentArgv()[0]
	if _, err := look(cmd); err != nil {
		return fmt.Sprintf("%s is not on PATH: install it, or name the stoker's agent in config.toml, [stoker] agent = \"codex\" (or opencode, cursor-agent)", cmd)
	}
	return ""
}

// startArgv starts unit u's stoker agent on its own server, in the builder's
// home directory. The server reads no tmux.conf, so the builder's plugins
// (tpm, resurrect) never load there, and it ends with its last session, so
// quit leaves no server behind. The agent's environment is the builder's,
// without the variables the service sets for itself: steps run with git
// that never prompts, and the home console marks its own session.
func (s *Service) startArgv(u *stokerUnit) []string {
	home, _ := os.UserHomeDir()
	return slices.Concat(
		[]string{"env", "-u", "GIT_TERMINAL_PROMPT", "-u", "GIT_SSH_COMMAND", "-u", panes.HomeEnv},
		sample.TmuxOn(s.StokerServer), []string{"-f", os.DevNull, "new-session", "-d", "-s", unitSession(u), "-c", home},
		s.agentArgv(),
		[]string{";", "set-option", "-s", "exit-empty", "on"})
}

// stokerSessionArgv is a tmux command on unit u's session (has-session,
// kill-session): "=" names that session exactly.
func (s *Service) stokerSessionArgv(u *stokerUnit, verb string) []string {
	return slices.Concat(sample.TmuxOn(s.StokerServer), []string{verb, "-t", "=" + unitSession(u)})
}

// unitSession is the tmux session of unit u; nil is the host's.
func unitSession(u *stokerUnit) string {
	if u == nil {
		return stoker.SessionName(stoker.HostID)
	}
	return stoker.SessionName(u.id)
}

// startAct starts unit u's stoker agent (stoker:start): f, n or o at a
// console, never the stoker or an agent's pane.
func (s *Service) startAct(ctx context.Context, u *stokerUnit) proto.Result {
	if err := s.startStoker(ctx, u); err != nil {
		return proto.Result{Err: err.Error()}
	}
	s.refreshStoker()
	return proto.Result{}
}

// startStoker starts unit u's stoker agent, or keeps the one that runs: a
// stoker session already on its server is reused.
func (s *Service) startStoker(ctx context.Context, u *stokerUnit) error {
	switch {
	case u == nil:
		return errors.New("this service keeps no stoker")
	case s.StokerServer == "":
		return errors.New("this service starts no stoker agent")
	}
	if why := s.agentMissing(); why != "" {
		return errors.New(why)
	}
	s.mu.Lock()
	_, up := stokerIn(s.cur.Sessions, s.StokerServer, stoker.SessionName(u.id))
	s.mu.Unlock()
	if up {
		return nil
	}
	if s.stokerThere(ctx, u) {
		s.markStarting(u) // there, and the sample has not found its agent yet
		return nil
	}
	res := s.Actions.Do(ctx, advice.Action{Label: "start the stoker", Steps: [][]string{s.startArgv(u)}}, "")
	// Another console's f may have started it a moment earlier: tmux then
	// refuses this new-session, and that agent is the one to reuse.
	if res.Err != "" && !s.stokerThere(ctx, u) {
		return errors.New("the stoker did not start: " + res.Err)
	}
	s.markStarting(u)
	return nil
}

// stokerThere: unit u's session is on its server.
func (s *Service) stokerThere(ctx context.Context, u *stokerUnit) bool {
	res := s.Actions.Do(ctx, advice.Action{Label: "is the stoker there", Steps: [][]string{s.stokerSessionArgv(u, "has-session")}}, "")
	return res.Err == ""
}

func (s *Service) markStarting(u *stokerUnit) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u.startedAt.IsZero() {
		u.startedAt = s.now()
	}
}

// quitAct ends unit u's stoker agent and its session (stoker:quit): the
// builder's, at a console or with matchblox stoker quit in a shell of
// their own. The host's run that is on goes on without its agent. A chain
// unit whose session is gone, and whose run is on, quits too: quitChainAct
// ends that run.
func (s *Service) quitAct(ctx context.Context, u *stokerUnit, a proto.Act) proto.Result {
	if a.Confirm != "y" {
		return proto.Result{Err: "needs confirm"}
	}
	if !a.Console {
		if _, ok := s.sessionAt(a.From); ok {
			return proto.Result{Err: notTheBuilders}
		}
	}
	if s.StokerServer == "" {
		return proto.Result{Err: "this service starts no stoker agent"}
	}
	if !s.stokerThere(ctx, u) {
		if u == nil || u == s.hostUnit() || !u.store.Mode().On {
			return proto.Result{Err: "the stoker is not running"}
		}
		s.refreshStoker()
		return proto.Result{}
	}
	res := s.Actions.Do(ctx, advice.Action{Label: "quit the stoker", Steps: [][]string{s.stokerSessionArgv(u, "kill-session")}, Destructive: true}, "y")
	if res.Err != "" {
		return res
	}
	if u != nil {
		s.mu.Lock()
		u.startedAt, u.brief, u.dropped = time.Time{}, nil, nil
		s.mu.Unlock()
	}
	s.refreshStoker()
	return res
}

// quitChainAct is stoker:quit with Text: it ends the agent of one chain
// stoker and its run, named by its id or by its chain (any member's name,
// a pane or the chain's id): its card is scored, and the cap counts it no
// more. Only the person quits one, at a console or in a shell of their own.
func (s *Service) quitChainAct(ctx context.Context, a proto.Act) proto.Result {
	if why := s.notThePersons(a); why != "" {
		return proto.Result{Err: why}
	}
	if _, stokerPane := s.unitOf(a); stokerPane {
		return proto.Result{Err: "the stoker does not start, stop or rest itself"}
	}
	if !personPlaced(a) {
		return proto.Result{Err: "not quit: matchblox cannot tell who quits this. Quit from a terminal, a tmux pane or the console."}
	}
	u := s.unitByID(a.Text)
	if u == nil {
		if c, _, _, ok := s.chainNamed(a.Text); ok {
			u = s.unitOfChain(c.ID)
		}
	}
	if u == nil || u == s.hostUnit() {
		return proto.Result{Err: fmt.Sprintf("no stoker %s: matchblox status lists the family", a.Text)}
	}
	gone := s.StokerServer != "" && !s.stokerThere(ctx, u)
	res := s.quitAct(ctx, u, a)
	if res.Err != "" {
		return res
	}
	if off := s.stokerAct(u, "off", s.now()); off.Err != "" {
		return off
	}
	res.Note = fmt.Sprintf("ended the stoker of chain %s (%s) and its tmux session", s.chainNameOf(s.chainIDOf(u)), u.id)
	if gone {
		res.Note = fmt.Sprintf("ended the stoker of chain %s (%s): its tmux session is gone", s.chainNameOf(s.chainIDOf(u)), u.id)
	}
	return res
}

// ready: the stoker agent rests and asks nothing (no trust or login
// prompt up): a line typed now is its order.
func ready(x sample.Session) bool { return x.Resting() && !x.Asks && !x.Permits }

// agentView adds the stoker agent to the console's view: its command, why
// it cannot start, and whether unit u's agent is up, starting or ready for
// an order.
func (s *Service) agentView(u *stokerUnit, v *stoker.View, sessions []sample.Session) {
	if s.StokerServer == "" {
		return
	}
	v.Agent, v.Start = strings.Join(s.agentArgv(), " "), strings.Join(s.startArgv(u), " ")
	v.Missing = s.agentMissing()
	x, up := stokerIn(sessions, s.StokerServer, stoker.SessionName(u.id))
	v.PaneUp = up
	v.Ready = up && ready(x)
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case u.startedAt.IsZero():
	case v.Ready, s.now().Sub(u.startedAt) >= stokerStartsIn:
		u.startedAt = time.Time{}
	default:
		v.Starting = true
	}
}
