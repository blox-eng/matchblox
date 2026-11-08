package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// paneActs are the acts that name a pane after the colon.
var paneActs = []string{"message:", proto.PickPrefix, "answer:", "compact:", "hold:", "release:", "park:", "kill:", "rename:", "switch:", proto.KeysPrefix}

// typedPane is the pane a message, an answer or a pick types into.
func typedPane(recID string) string {
	for _, p := range []string{"message:", "answer:", proto.PickPrefix} {
		if pane, ok := strings.CutPrefix(recID, p); ok {
			return pane
		}
	}
	return ""
}

func paneOf(recID string) string {
	for _, p := range paneActs {
		if pane, ok := strings.CutPrefix(recID, p); ok {
			return pane
		}
	}
	return ""
}

// fromStoker tells if the act was typed in a pane of the tmux server
// matchblox keeps the stoker agent on. The pane's server decides, not the
// agent list: a pane of it the last sample missed is still the stoker's,
// and the builder's pane of the same id never is.
func (s *Service) fromStoker(a proto.Act) bool {
	if s.StokerServer == "" || a.From == "" {
		return false
	}
	server, _ := sample.SplitPane(a.From)
	return server == s.StokerServer
}

// orderable is what the stoker agent and the sessions it drives may do:
// nothing while the builder works, nothing to the stoker's own session,
// and no more than OrderSteps messages a run.
func (s *Service) orderable(a proto.Act) error {
	if verb, ok := strings.CutPrefix(a.RecID, "stoker:"); ok {
		// Its plan, its steps and its triggers are the stoker's own, and
		// work while it rests.
		// Every other stoker act is the builder's.
		switch verb {
		case "plan", "step", "when":
			return nil
		case "grant":
			return errors.New("the stoker does not give itself the go")
		case "order":
			return errors.New("the stoker does not give itself orders")
		}
		return errors.New("the stoker does not start, stop or rest itself")
	}
	if strings.HasPrefix(a.RecID, "park:") {
		return errors.New("the stoker does not park: z is yours")
	}
	if a.RecID == RecoverID {
		return errors.New("the stoker does not reopen sessions: matchblox recover is yours")
	}
	if cloneAct(a.RecID) || (a.RecID == NewSessionID && s.newClones(a.Text)) {
		return errors.New(noClone)
	}
	pane := paneOf(a.RecID)
	counts := pane != "" || a.RecID == NewSessionID // what makes a step of the run
	if counts && (s.Stoker == nil || !s.Stoker.Mode().On) {
		return errors.New("the stoker is off: no run")
	}
	if s.Stoker == nil {
		return nil
	}
	m := s.Stoker.Mode()
	if m.On && !m.Rests.IsZero() {
		return errors.New("the stoker rests: a person works")
	}
	if !counts {
		return nil
	}
	if pane != "" && pane == a.From {
		return errors.New("the stoker does not act on itself")
	}
	// A session the stoker drives must not type into the stoker's panes:
	// a line there would read as the builder's new order. A pane whose
	// session cannot be told is refused too.
	if pane != "" && s.fromStoker(proto.Act{From: pane}) {
		return errors.New("no agent acts on the stoker's own session")
	}
	n := 0
	for _, e := range s.Stoker.Entries() {
		if e.Ordered() && !e.At.Before(m.Since) {
			n++
		}
	}
	if n >= s.OrderSteps {
		return fmt.Errorf("the run sent its %d messages: max_steps", s.OrderSteps)
	}
	return nil
}

// recordOrder keeps a message the stoker agent sent in the run, for its
// count and for the orders built on it. A compact records itself.
func (s *Service) recordOrder(a proto.Act, res proto.Result) {
	pane := paneOf(a.RecID)
	if s.Stoker == nil || pane == "" || strings.HasPrefix(a.RecID, "compact:") || res.Err != "" || len(res.Ran) == 0 {
		return
	}
	now := s.now()
	x, _ := s.sessionAt(pane)
	_ = s.Stoker.Append(stoker.Entry{
		ID: x.SessionID + "@" + now.UTC().Format(time.RFC3339Nano), At: now,
		Session: x.SessionID, Name: x.Name, Pane: pane, Rule: stoker.RuleOrder,
		Argv: res.Ran, Result: stoker.Sent,
	})
	if !strings.HasPrefix(a.RecID, "kill:") { // a pane it killed is gone
		s.drive(pane)
	}
	s.refreshStoker()
}

// compact types /compact into a session under the guards of the stoker's
// own compacts: Claude Code, at rest, not waiting for you or held, not
// compacted in the last 30 minutes, and the pane read again right before
// each key. A refusal says which. A console names the agent it showed
// (PID and Start) and the screen it saw; the CLI may name none.
func (s *Service) compact(ctx context.Context, pane string, a proto.Act) proto.Result {
	if x, ok := s.sessionAt(pane); ok && x.Agent != "claude" {
		return proto.Result{Err: x.Name + " is not a Claude Code session: /compact is typed only there"}
	}
	x, err := s.restsFor(pane, "compact", a)
	if err != nil {
		return proto.Result{Err: err.Error()}
	}
	now := s.now()
	if s.Stoker != nil && x.SessionID != "" {
		for _, e := range s.Stoker.Entries() {
			if e.Session == x.SessionID && e.Rule == stoker.RuleCompact && !e.Logged() && now.Sub(e.At) < stoker.PerSession {
				return proto.Result{Err: fmt.Sprintf("%s was compacted %d min ago: one compact in %d minutes",
					x.Name, int(now.Sub(e.At).Minutes()), int(stoker.PerSession.Minutes()))}
			}
		}
	}
	action := stoker.CompactAction(pane, x.PID, x.Start)
	release := s.holdPane(pane)
	res := s.Actions.Do(ctx, action, a.Confirm)
	if len(res.Ran) > 0 {
		s.latchRest(pane)
	}
	release()
	if s.Stoker != nil && len(res.Ran) > 0 {
		e := stoker.Entry{
			ID: x.SessionID + "@" + now.UTC().Format(time.RFC3339Nano), At: now,
			Session: x.SessionID, Name: x.Name, Pane: pane, Rule: stoker.RuleCompact, Why: "asked",
			Argv: action.Steps, Before: x.Tokens, Guard: "at rest", Result: stoker.Sent,
		}
		if s.fromStoker(a) {
			e.Why = stoker.WhyOrder
		}
		if len(res.Ran) < len(action.Steps) {
			e.Result = "typed, not sent"
		}
		_ = s.Stoker.Append(e)
		s.refreshStoker()
	}
	return res
}

// restsFor is the session in pane when /<cmd>, sent by act a, may go to
// it: the agent a names (any agent for the CLI's pid 0; a console's by pid
// and start), one the builder may message, not waiting for them, and at
// rest. A console's command ends in an Enter, so it needs the live screen
// seen, as an Enter does (unseen).
func (s *Service) restsFor(pane, cmd string, a proto.Act) (sample.Session, error) {
	x, ok := s.sessionAt(pane)
	if !ok {
		return x, errors.New("no agent session in " + pane + " any more")
	}
	if a.Console && !sameAgent(x, a) {
		return x, errors.New("another agent runs in " + pane + " now")
	}
	if err := s.messageable(pane, cmp.Or(a.PID, x.PID)); err != nil {
		return x, err
	}
	if it, ok := s.queued(pane); ok && stoker.Waits(it) {
		return x, errors.New(x.Name + " waits for you")
	}
	if !s.resting(pane) {
		return x, fmt.Errorf("%s is not at rest: a %s waits for the end of its turn", x.Name, cmd)
	}
	if a.Console {
		if why := s.unseen(pane, x.Name, proto.Act{Text: "Enter", Seen: a.Seen}, x.Permits || x.Asks); why != "" {
			return x, errors.New(strings.TrimPrefix(why, "not sent: "))
		}
	}
	return x, nil
}

// readsPrompt are the agents whose prompt said.Draft reads: a draft there
// stops a command, and the Enter waits for the command on the prompt.
var readsPrompt = map[string]bool{"claude": true, "codex": true}
