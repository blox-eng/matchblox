package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// briefName is the brief file in the state dir. The stoker pane gets one
// line that names it: a brief is longer than a line may be.
const briefName = "stoker-brief.md"

// sealed refuses a new order while a night runs.
const sealed = "the night is sealed: n stops it, then launch a new one"

// orderAsk is what the launch sheet sends with stoker:order: the words,
// and the goes the builder's chips give (merge, release, deploy).
type orderAsk struct {
	Words  string   `json:"words"`
	Night  bool     `json:"night"`
	Grants []string `json:"grants,omitempty"`
}

// orderAct runs the builder's side of an order: give it.
func (s *Service) orderAct(ctx context.Context, a proto.Act) proto.Result {
	if s.Stoker == nil {
		return proto.Result{Err: "this service keeps no stoker"}
	}
	if s.fromStoker(a) {
		return proto.Result{Err: "the stoker does not give itself orders"}
	}
	if err := s.order(ctx, a.Text); err != nil {
		return proto.Result{Err: err.Error()}
	}
	s.refreshStoker()
	return proto.Result{}
}

// order takes the builder's words, and the run starts at once. With no
// stoker agent (fixtures, or its command not on PATH) it turns today's
// stoker on (or on for the night) and keeps the words. With one, it
// writes the brief and points the stoker agent at it. A night that runs
// takes no new order.
func (s *Service) order(ctx context.Context, text string) error {
	var ask orderAsk
	if err := json.Unmarshal([]byte(text), &ask); err != nil {
		return fmt.Errorf("the order does not read: %w", err)
	}
	var grants []string
	for _, g := range ask.Grants {
		if !slices.Contains(stoker.Goes, g) {
			return fmt.Errorf("%q is no go: merge, release, deploy or kill", g)
		}
		if !slices.Contains(grants, g) {
			grants = append(grants, g)
		}
	}
	now := s.now()
	if m := s.Stoker.Mode(); m.On && !m.Nightly && !ask.Night && m.Order != nil && m.Order.State == stoker.OrderRunning {
		return s.dayOrder(ctx, strings.TrimSpace(ask.Words), grants, now)
	}
	o := stoker.Order{Grants: grants, ID: "run-" + now.UTC().Format("20060102-150405.000000000"), Words: strings.TrimSpace(ask.Words), Night: ask.Night, At: now, Went: now, State: stoker.OrderRunning}
	if m := s.Stoker.Mode(); m.On && m.Nightly {
		return errors.New(sealed)
	}
	if s.agentOn() {
		pane, err := s.stokerAgent()
		if err != nil {
			return err
		}
		path := filepath.Join(s.Stoker.Dir(), briefName)
		if err := os.MkdirAll(s.Stoker.Dir(), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(stoker.Brief(o, s.Ways, stoker.Hands, s.board(now))), 0o600); err != nil {
			return err
		}
		if err := s.typeInto(ctx, pane, "Read "+path+" and follow it."); err != nil {
			return err
		}
	}
	sessions, _ := s.seen()
	return s.updateOrder(func(m stoker.Mode) (stoker.Mode, error) {
		if m.On && m.Nightly {
			return m, errors.New(sealed)
		}
		if !m.On {
			// A new run: the one that ended is folded, its card kept in
			// stoker-runs.jsonl, and nothing of it shows with this order.
			m = stoker.Mode{Acked: m.Ack(now).Acked}
		}
		m, err := s.start(m, o.Night, now, sessions)
		// Triggers belong to the order they were set under: a night over
		// the day's run ends the day's.
		m.Order, m.Triggers, m.NextTrigger = &o, nil, 0
		return m, err
	})
}

// board is the machine now, for the brief's snapshot.
func (s *Service) board(now time.Time) stoker.Board {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := stoker.Board{At: now, Sessions: s.cur.Sessions, Ships: s.cur.Ships, Holds: s.cur.Holds, Limits: s.cur.Limits,
		StokerServer: s.StokerServer, Reasons: map[string]string{}}
	for pane, r := range s.cur.Reasons {
		b.Reasons[pane] = r.Word
	}
	return b
}

// orderName is the file a new order by day goes to when it is longer
// than a line may be.
const orderName = "stoker-order.md"

// dayOrder is a new order to the day run that is on: one line to the
// stoker agent, and the run goes on. Its goes become the chips'. The run
// is checked under the mode's lock right before the line is typed, and
// the order lands under the same lock: the stoker never reads a new order
// a run that ended meanwhile did not take.
func (s *Service) dayOrder(ctx context.Context, words string, grants []string, now time.Time) error {
	pane, line := "", ""
	if s.agentOn() {
		var err error
		if pane, err = s.stokerAgent(); err != nil {
			return err
		}
		line = stoker.NewOrder(words)
		if utf8.RuneCountInString(line) > panes.MaxAnswer {
			path := filepath.Join(s.Stoker.Dir(), orderName)
			if err := os.MkdirAll(s.Stoker.Dir(), 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
				return err
			}
			line = "Read " + path + " and follow it."
		}
	}
	return s.updateOrder(func(m stoker.Mode) (stoker.Mode, error) {
		if !m.On || m.Nightly || m.Order == nil || m.Order.State != stoker.OrderRunning {
			return m, errors.New("the run changed meanwhile: look at it again")
		}
		if pane != "" {
			if err := s.typeInto(ctx, pane, line); err != nil {
				return m, err
			}
		}
		o := *m.Order
		o.Earlier = append(slices.Clip(o.Earlier), stoker.Said{At: o.At, Words: o.Words})
		o.Words, o.At, o.Grants = words, now, grants
		o.Unguarded = stoker.Unguarded(o.Plan, grants, func(who string) string {
			x, _ := s.sessionNamed(who)
			return x.Agent
		})
		m.Order = &o
		return m, nil
	})
}

// planAct takes the stoker agent's plan (stoker:plan), one step it logs
// (stoker:step) or a trigger it sets or drops (stoker:when). Only the
// stoker pane sends them.
func (s *Service) planAct(verb string, a proto.Act) proto.Result {
	if s.Stoker == nil {
		return proto.Result{Err: "this service keeps no stoker"}
	}
	var err error
	switch {
	case !s.fromStoker(a) && verb == "plan":
		err = errors.New("only the stoker writes its plan")
	case !s.fromStoker(a) && verb == "when":
		err = errors.New("only the stoker sets its triggers")
	case !s.fromStoker(a):
		err = errors.New("only the stoker logs its steps")
	case verb == "plan":
		err = s.plan(a.Text)
	case verb == "when":
		err = s.when(a.Text)
	default:
		err = s.step(a.Text)
	}
	if err != nil {
		return proto.Result{Err: err.Error()}
	}
	s.refreshStoker()
	return proto.Result{}
}

// plan replaces the plan of the order that runs with the one the agent
// wrote. Nobody approves it; the goes it names are ignored, as the goes
// are the builder's.
func (s *Service) plan(text string) error {
	var p stoker.PlanText
	if err := json.Unmarshal([]byte(text), &p); err != nil {
		return fmt.Errorf("the plan does not read: %w", err)
	}
	o := s.Stoker.Mode().Order
	switch {
	case o == nil:
		return errors.New("no order to plan: launch one first")
	case o.State != stoker.OrderRunning:
		return errors.New("the run ended: a new order starts a new one")
	case len(p.Steps) == 0:
		return errors.New("the plan has no step")
	}
	seen := map[int]bool{}
	for _, st := range p.Steps {
		switch {
		case st.N < 1:
			return errors.New("a step number starts at 1")
		case seen[st.N]:
			return fmt.Errorf("step %d comes twice", st.N)
		case strings.TrimSpace(st.Text) == "":
			return fmt.Errorf("step %d says nothing", st.N)
		case utf8.RuneCountInString(st.Text) > panes.MaxAnswer:
			return fmt.Errorf("step %d is longer than %d characters", st.N, panes.MaxAnswer)
		}
		seen[st.N] = true
		if _, err := s.sessionNamed(st.Session); err != nil {
			return fmt.Errorf("step %d: %w", st.N, err)
		}
	}
	return s.updateOrder(func(m stoker.Mode) (stoker.Mode, error) {
		if m.Order == nil || m.Order.ID != o.ID || m.Order.State != stoker.OrderRunning {
			return m, errors.New("the order changed meanwhile: read the brief again")
		}
		n := *m.Order
		n.Plan = p.Steps
		n.Unguarded = stoker.Unguarded(p.Steps, n.Grants, func(who string) string {
			x, _ := s.sessionNamed(who)
			return x.Agent
		})
		m.Order = &n
		return m, nil
	})
}

// step logs one step of the plan (or of a standing rule, step 0) to the
// run, for the card.
func (s *Service) step(text string) error {
	var in stoker.Entry
	if err := json.Unmarshal([]byte(text), &in); err != nil {
		return fmt.Errorf("the step does not read: %w", err)
	}
	in.Text = strings.TrimSpace(in.Text)
	switch {
	case !slices.Contains(stoker.StepStates, in.State):
		return fmt.Errorf("%q is no state: doing, done, failed or waits", in.State)
	case in.Text == "":
		return errors.New("a step needs its text")
	case utf8.RuneCountInString(in.Text) > panes.MaxAnswer:
		return fmt.Errorf("a step's text is longer than %d characters", panes.MaxAnswer)
	case in.Step < 0:
		return errors.New("a step number starts at 1")
	}
	if in.Needs != "" {
		if err := stoker.CheckGrant(in.Needs); err != nil {
			return err
		}
	}
	now := s.now()
	e := stoker.Entry{
		ID: fmt.Sprintf("step%d@%s", in.Step, now.UTC().Format(time.RFC3339Nano)), At: now,
		Step: in.Step, State: in.State, Needs: in.Needs, Text: in.Text,
	}
	if in.Step == 0 {
		if e.Rule = strings.TrimSpace(in.Rule); e.Rule == "" {
			return errors.New("step 0 is a standing rule: name it with --rule")
		}
		return s.Stoker.Append(e)
	}
	o := s.Stoker.Mode().Order
	i := -1
	if o != nil {
		i = slices.IndexFunc(o.Plan, func(st stoker.PlanStep) bool { return st.N == in.Step })
	}
	if i < 0 {
		return fmt.Errorf("the plan has no step %d", in.Step)
	}
	e.Name = o.Plan[i].Session
	if x, err := s.sessionNamed(o.Plan[i].Session); err == nil {
		e.Session, e.Name, e.Pane = x.SessionID, x.Name, x.Pane
	}
	return s.Stoker.Append(e)
}

// updateOrder writes the mode fn returns; fn's error leaves it as it is.
func (s *Service) updateOrder(fn func(stoker.Mode) (stoker.Mode, error)) error {
	var refused error
	_, err := s.Stoker.Update(func(m stoker.Mode) (stoker.Mode, bool) {
		n, err := fn(m)
		if err != nil {
			refused = err
			return m, false
		}
		return n, true
	})
	return errors.Join(refused, err)
}

// start turns the stoker on, or on until the morning, and seats the
// sessions at work when the run is new.
func (s *Service) start(m stoker.Mode, night bool, now time.Time, sessions []sample.Session) (stoker.Mode, error) {
	was := m.On
	if night {
		var err error
		if m, err = m.Night(now, s.NightEnds); err != nil {
			return m, err
		}
	} else {
		m = m.TurnOn(now)
	}
	if !was {
		m = m.Begin(sessions)
	}
	return m, nil
}

// stokerAgent is the pane of the stoker agent, or why there is none to
// type into yet.
func (s *Service) stokerAgent() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, ok := stokerIn(s.cur.Sessions, s.StokerServer)
	switch {
	case ok && s.startedAt.IsZero():
		return x.Pane, nil
	case ok && ready(x):
		s.startedAt = time.Time{}
		return x.Pane, nil
	case !s.startedAt.IsZero():
		// Never into a half-started agent: its first prompt may not take
		// what is typed.
		return "", errors.New("the stoker is starting: the order goes once it rests")
	}
	return "", errors.New("the stoker is not running: f or n starts it")
}

// stokerIn is the agent session in the stoker's session of its tmux
// server.
func stokerIn(sessions []sample.Session, server string) (sample.Session, bool) {
	for _, x := range sessions {
		if server != "" && x.Server() == server && x.TmuxSession() == sample.StokerSession {
			return x, true
		}
	}
	return sample.Session{}, false
}

// typeInto types one line and Enter into the stoker's pane, with the
// guards of any message.
func (s *Service) typeInto(ctx context.Context, pane, line string) error {
	res := s.message(ctx, pane, proto.Act{Confirm: "y", Text: line})
	switch {
	case res.Err != "":
		return errors.New(res.Err)
	case len(res.Skipped) > 0:
		return errors.New(strings.Join(res.Skipped, "; "))
	}
	return nil
}

// sessionNamed finds the session a plan names, as `matchblox send` does:
// by pane, target or a name only one session has.
func (s *Service) sessionNamed(who string) (sample.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var hit []sample.Session
	for _, x := range s.cur.Sessions {
		if who != "" && (x.Pane == who || x.Target == who) {
			return x, nil
		}
		if who != "" && x.Name == who {
			hit = append(hit, x)
		}
	}
	switch len(hit) {
	case 0:
		return sample.Session{}, fmt.Errorf("no session named %s", who)
	case 1:
		return hit[0], nil
	}
	return sample.Session{}, fmt.Errorf("%s is %d sessions: name the pane", who, len(hit))
}

// waysCache keeps the recipe names of the ways file until it changes.
type waysCache struct {
	mu    sync.Mutex
	mod   time.Time
	size  int64
	names []string
}

// recipes are the `# Recipe:` names of the ways file, read again only
// when the file changes.
func (s *Service) recipes() []string {
	if s.Ways == "" {
		return nil
	}
	c := &s.ways
	c.mu.Lock()
	defer c.mu.Unlock()
	fi, err := os.Stat(s.Ways)
	if err != nil {
		c.mod, c.size, c.names = time.Time{}, 0, nil
		return nil
	}
	if !fi.ModTime().Equal(c.mod) || fi.Size() != c.size {
		b, err := os.ReadFile(s.Ways)
		if err != nil {
			return c.names
		}
		c.mod, c.size, c.names = fi.ModTime(), fi.Size(), stoker.Recipes(b)
	}
	return c.names
}

// stokerView is the stoker as the consoles draw it: the store's view, the
// ways' recipes and whether an agent runs in the stoker's pane.
func (s *Service) stokerView(sessions []sample.Session) *stoker.View {
	v := s.Stoker.View()
	v.Recipes = s.recipes()
	s.agentView(&v, sessions)
	return &v
}
