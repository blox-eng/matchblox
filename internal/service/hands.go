package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
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

// stokersOwn refuses an act on a stoker's session, and any act of a
// stoker session no unit runs.
const stokersOwn = "no agent acts on the stoker's own session"

// chainOnly refuses a chain stoker's act that is not one of its chain's.
const chainOnly = "a chain stoker acts on the matches of its chain only"

// orderable is what the stoker agent of unit u and the sessions it drives
// may do: nothing while the builder works, nothing to a stoker's own
// session, and no more than OrderSteps messages a run. A nil u is a
// stoker session no unit runs: it may do nothing.
func (s *Service) orderable(u *stokerUnit, a proto.Act) error {
	if u == nil {
		return errors.New(stokersOwn)
	}
	if verb, ok := strings.CutPrefix(a.RecID, "stoker:"); ok {
		// Its plan, its steps and its triggers are the stoker's own, and
		// work while it rests.
		// Every other stoker act is the builder's.
		if verb == "order-chain" {
			if _, stokerPane := s.unitOf(a); !stokerPane {
				return errors.New(sessionGivesNoOrders) // a session a stoker drove
			}
		}
		if u.link.Chain != "" && (verb == "order" || verb == "order-chain") {
			return errors.New("a chain stoker gives no orders")
		}
		switch verb {
		case "plan", "step", "when":
			return nil
		case "order-chain":
			if own, _ := s.unitOf(a); own == u && u == s.hostUnit() {
				return nil
			}
			return errors.New(sessionGivesNoOrders)
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
	if !counts && u.link.Chain != "" {
		// A rec, a ship or a worktree may reach past the chain.
		return errors.New(chainOnly)
	}
	m := u.store.Mode()
	if counts && !m.On {
		return errors.New("the stoker is off: no run")
	}
	if m.On && !m.Rests.IsZero() {
		return errors.New("the stoker rests: a person works")
	}
	if !counts {
		return s.recScope(u, a)
	}
	if pane != "" && pane == a.From {
		return errors.New("the stoker does not act on itself")
	}
	// A session the stoker drives must not type into the stoker's panes:
	// a line there would read as the builder's new order. A pane whose
	// session cannot be told is refused too.
	if _, own := s.unitOf(proto.Act{From: pane}); pane != "" && own {
		return errors.New(stokersOwn)
	}
	if pane != "" {
		if err := s.scopeOf(u, pane); err != nil {
			return err
		}
	}
	if a.RecID == NewSessionID && u.link.Chain != "" {
		if err := s.inChainRepo(u, a.Text); err != nil {
			return err
		}
	}
	n := 0
	for _, e := range u.store.Entries() {
		if e.Ordered() && !e.At.Before(m.Since) {
			n++
		}
	}
	if n >= s.OrderSteps {
		return fmt.Errorf("the run sent its %d messages: max_steps", s.OrderSteps)
	}
	return nil
}

// recordOrder keeps a message the stoker agent of unit u sent in the run,
// for its count and for the orders built on it. A compact records itself.
func (s *Service) recordOrder(u *stokerUnit, a proto.Act, res proto.Result) {
	pane := paneOf(a.RecID)
	if u == nil || pane == "" || strings.HasPrefix(a.RecID, "compact:") || res.Err != "" || len(res.Ran) == 0 {
		return
	}
	now := s.now()
	x, _ := s.sessionAt(pane)
	_ = u.store.Append(stoker.Entry{
		ID: x.SessionID + "@" + now.UTC().Format(time.RFC3339Nano), At: now,
		Session: x.SessionID, Name: x.Name, Pane: pane, Rule: stoker.RuleOrder,
		Argv: res.Ran, Result: stoker.Sent,
	})
	if !strings.HasPrefix(a.RecID, "kill:") { // a pane it killed is gone
		s.drive(u, pane)
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
	if x.SessionID != "" {
		for _, f := range s.family() {
			for _, e := range f.store.Entries() {
				if e.Session == x.SessionID && e.Rule == stoker.RuleCompact && !e.Logged() && now.Sub(e.At) < stoker.PerSession {
					return proto.Result{Err: fmt.Sprintf("%s was compacted %d min ago: one compact in %d minutes",
						x.Name, int(now.Sub(e.At).Minutes()), int(stoker.PerSession.Minutes()))}
				}
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
	// A stoker's compact goes on its own card; the builder's and a driven
	// session's on the host's.
	u, own := s.unitOf(a)
	if !own {
		u = s.hostUnit()
	}
	if u != nil && len(res.Ran) > 0 {
		e := stoker.Entry{
			ID: x.SessionID + "@" + now.UTC().Format(time.RFC3339Nano), At: now,
			Session: x.SessionID, Name: x.Name, Pane: pane, Rule: stoker.RuleCompact, Why: "asked",
			Argv: action.Steps, Before: x.Tokens, Guard: "at rest", Result: stoker.Sent,
		}
		if own {
			e.Why = stoker.WhyOrder
		}
		if len(res.Ran) < len(action.Steps) {
			e.Result = "typed, not sent"
		}
		_ = u.store.Append(e)
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

// recScope refuses a rec or panel act of the host unit u whose action
// reaches a match a chain unit owns: a pane its steps or guards name, or a
// process of that match its guards name.
func (s *Service) recScope(u *stokerUnit, a proto.Act) error {
	action, ok := s.resolve(a.RecID, a.Which)
	if !ok {
		return nil // dispatch finds no action either
	}
	for _, x := range s.reached(action) {
		if o := s.ownerOf(x); o != nil && o != u {
			return fmt.Errorf("%s has the stoker of chain %s: give it an order.", x.Name, s.chainNameOf(o.link.Chain)) //nolint:staticcheck // the line a person reads, as it stands
		}
	}
	return nil
}

// reached are the sessions an action acts on: by the pane of a tmux step's
// -t or of a guard, or by a guard's pid, the agent's own or a shell's.
func (s *Service) reached(action advice.Action) []sample.Session {
	panes := map[string]bool{}
	for _, step := range action.Steps {
		if len(step) == 0 || step[0] != "tmux" {
			continue
		}
		server := ""
		for i := 1; i+1 < len(step); i++ {
			switch step[i] {
			case "-L":
				server = step[i+1]
			case "-t":
				pane := strings.TrimPrefix(step[i+1], "=")
				if server != "" {
					pane += "@" + server
				}
				panes[pane] = true
			}
		}
	}
	pids := map[int]bool{}
	for _, g := range action.Guards {
		for _, p := range []string{g.IdlePane, g.RestPane, g.AnswerPane, g.MessagePane, g.AskPane, g.StokePane} {
			if p != "" {
				panes[p] = true
			}
		}
		if g.PID > 0 {
			pids[g.PID] = true
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []sample.Session
	for _, x := range s.cur.Sessions {
		hit := panes[x.Pane] || pids[x.PID] && x.PID > 0
		for _, sh := range x.Shells {
			hit = hit || pids[sh.PID]
		}
		if hit {
			out = append(out, x)
		}
	}
	return out
}
