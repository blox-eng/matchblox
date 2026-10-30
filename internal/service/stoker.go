package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// stokerAct turns the stoker on ("on"), on until the morning ("night"),
// off, or folds the steps the builder saw ("ack"). While it runs the
// builder takes over ("rest"), keeps it on ("keep"), turns it back on
// ("wake"), and each key keeps the rest ("touch"). None is destructive:
// the stoker itself never ends, kills or removes anything.
func (s *Service) stokerAct(verb string, now time.Time) proto.Result {
	if s.Stoker == nil {
		return proto.Result{Err: "this service keeps no stoker"}
	}
	sessions, waiting := s.seen()
	var refused string
	_, err := s.Stoker.Update(func(m stoker.Mode) (stoker.Mode, bool) {
		was := m.On
		switch verb {
		case "on":
			m = m.TurnOn(now)
		case "night":
			var err error
			if m, err = m.Night(now, s.NightEnds); err != nil {
				refused = err.Error()
				return m, false
			}
		case "off":
			if !was {
				return m, false
			}
			m = m.Off(now).Score(sessions, waiting, s.full, s.Stoker.View().Run)
		case "ack":
			m = m.Ack(now)
		case "rest", "keep", "wake", "touch":
			if !m.On {
				refused = "the stoker is off"
				return m, false
			}
			switch verb {
			case "rest":
				m = m.Rest(now)
			case "keep":
				m.Asked = true
			case "wake":
				m = m.Wake()
			case "touch":
				m = m.Touch(now)
			}
		default:
			refused = "unknown stoker act"
			return m, false
		}
		if m.On && !was {
			m = m.Begin(sessions)
		}
		return m, true
	})
	if refused != "" {
		return proto.Result{Err: refused}
	}
	if err != nil {
		return proto.Result{Err: err.Error()}
	}
	s.refreshStoker()
	return proto.Result{}
}

// stoke runs after each sample: it ends a night at its morning, reads back
// what the compacts it sent left, and sends the steps the plan finds. Each
// step goes through the runner, so its idle guard is checked right before
// the send.
func (s *Service) stoke(ctx context.Context, now time.Time) {
	if s.Stoker == nil {
		return
	}
	changed := false
	sessions, waiting := s.seen()
	s.mu.Lock()
	alone := s.alone
	s.mu.Unlock()
	var typed time.Time // the builder's last key in tmux, read only during a rest
	if !s.Stoker.Mode().Rests.IsZero() && s.Activity != nil {
		typed = s.Activity()
	}
	// One update for all three, so an act in between is never undone (a
	// write that fails is tried again at the next sample).
	_, _ = s.Stoker.Update(func(m stoker.Mode) (stoker.Mode, bool) {
		m, seen := m.Observe(sessions, waiting, s.full, now)
		m = m.Touch(typed)
		m, woke := m.Awake(now, alone)
		m, ended := m.Expire(now)
		if ended {
			m = m.Score(sessions, waiting, s.full, s.Stoker.View().Run)
		}
		changed = seen || woke || ended
		return m, changed
	})

	for _, e := range s.Stoker.Pending(now) {
		path := ""
		for _, ss := range sessions {
			if ss.SessionID == e.Session {
				path = ss.Transcript
			}
		}
		if c, ok := sample.Compacted(path, e.At); ok && path != "" {
			_ = s.Stoker.Append(stoker.Entry{ID: e.ID, Before: c.Before, After: c.After, Resume: c.Resume, Back: now})
			changed = true
		}
	}

	for _, st := range stoker.Plan(s.Stoker.Mode(), sessions, waiting, s.full, s.Stoker.Entries(), now, s.heldSessions()) {
		if m := s.Stoker.Mode(); !m.On || !m.Rests.IsZero() {
			break // the builder took over or turned it off since the plan
		}
		// The builder confirmed when they turned the stoker on.
		release := s.holdPane(st.Pane)
		res := s.Actions.Do(ctx, st.Action, "y")
		release()
		e := stoker.Entry{
			ID: st.Session + "@" + now.UTC().Format(time.RFC3339Nano), At: now,
			Session: st.Session, Name: st.Name, Pane: st.Pane, Rule: stoker.RuleCompact, Why: st.Why,
			Argv: st.Action.Steps, Before: st.Tokens, Guard: "at rest", Result: stoker.Sent,
		}
		switch {
		case res.Err != "":
			e.Result = "error: " + res.Err
		case len(res.Ran) == 0:
			e.Guard = why(st, res.Skipped)
			e.Result = stoker.Skipped + ": " + e.Guard
		case len(res.Ran) < len(st.Action.Steps):
			// The text is in the field, without its Enter: the builder sees it.
			e.Guard = why(st, res.Skipped)
			e.Result = "typed, not sent: " + e.Guard
		}
		_ = s.Stoker.Append(e)
		changed = true
	}
	if s.fire(ctx, now) {
		changed = true
	}
	if changed {
		s.refreshStoker()
	} else {
		s.syncDriven() // a lost driven.json comes back
	}
	s.wake(ctx, now)
}

// touchStoker keeps a rest while the builder acts from a console.
func (s *Service) touchStoker(now time.Time) {
	if s.Stoker == nil {
		return
	}
	_, _ = s.Stoker.Update(func(m stoker.Mode) (stoker.Mode, bool) {
		return m.Touch(now), !m.Rests.IsZero()
	})
}

// seen are the sessions of the last sample and the panes the queue lists
// as waiting for the builder.
func (s *Service) seen() ([]sample.Session, map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	waiting := map[string]bool{}
	for _, it := range s.cur.Queue {
		if stoker.Waits(it) {
			waiting[it.Pane] = true
		}
	}
	return s.cur.Sessions, waiting
}

// refreshStoker sends the stoker view at once.
func (s *Service) refreshStoker() {
	sessions, _ := s.seen()
	v := s.stokerView(sessions)
	s.syncDriven()
	s.mu.Lock()
	s.cur.Stoker = v
	s.mu.Unlock()
	s.broadcast()
}

// why is the reason the runner gave for the first step it skipped, without
// the step's own argv in front of it.
func why(st stoker.Step, skipped []string) string {
	if len(skipped) == 0 {
		return ""
	}
	for _, step := range st.Action.Steps {
		if r, ok := strings.CutPrefix(skipped[0], strings.Join(step, " ")+": "); ok {
			return r
		}
	}
	return skipped[0]
}

// stokable is the guard of each stoker step, read right before it: the
// session is idle, nothing in the queue waits there (a hook can arrive at
// any time), and the pane, captured now, shows no menu, no question and
// no draft of the builder's. Before the Enter the draft must be the
// step's own text.
func (s *Service) stokable(pane, typed string) error {
	if it, ok := s.queued(pane); ok && stoker.Waits(it) {
		return fmt.Errorf("the session in %s waits for you", pane)
	}
	screen, ok := s.Capture([]string{pane})[pane]
	if !ok {
		return fmt.Errorf("the pane %s cannot be read", pane)
	}
	switch _, kind := said.Screen(screen); kind {
	case said.Permits:
		return fmt.Errorf("the pane %s shows a permission prompt", pane)
	case said.Asks:
		return fmt.Errorf("the pane %s asks a question", pane)
	}
	draft := said.Draft(screen)
	if typed == "" && draft != "" {
		return fmt.Errorf("the pane %s has text you typed", pane)
	}
	if typed != "" && !strings.HasPrefix(draft, typed) {
		return fmt.Errorf("the pane %s does not show %s", pane, typed)
	}
	return nil
}
