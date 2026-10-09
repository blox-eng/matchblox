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
// off, or folds the steps the builder saw ("ack"). None is destructive:
// the stoker itself never ends, kills or removes anything.
func (s *Service) stokerAct(verb string, now time.Time) proto.Result {
	if s.Stoker == nil {
		return proto.Result{Err: "this service keeps no stoker"}
	}
	m := s.Stoker.Mode()
	switch verb {
	case "on":
		m = m.TurnOn(now)
	case "night":
		var err error
		if m, err = m.Night(now, s.NightEnds); err != nil {
			return proto.Result{Err: err.Error()}
		}
	case "off":
		m = m.Off(now)
	case "ack":
		m = m.Ack(now)
	default:
		return proto.Result{Err: "unknown stoker act"}
	}
	if err := s.Stoker.Set(m); err != nil {
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
	if m, ended := s.Stoker.Mode().Expire(now); ended {
		_ = s.Stoker.Set(m) // a write that fails is tried again at the next sample
		changed = true
	}
	s.mu.Lock()
	sessions := s.cur.Sessions
	waiting := map[string]bool{}
	for _, it := range s.cur.Queue {
		if stoker.Waits(it) {
			waiting[it.Pane] = true
		}
	}
	s.mu.Unlock()

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

	for _, st := range stoker.Plan(s.Stoker.Mode(), sessions, waiting, s.compactAt, s.Stoker.Entries(), now) {
		// The builder confirmed when they turned the stoker on.
		res := s.Actions.Do(ctx, st.Action, "y")
		e := stoker.Entry{
			ID: st.Session + "@" + now.UTC().Format(time.RFC3339Nano), At: now,
			Session: st.Session, Name: st.Name, Pane: st.Pane, Rule: stoker.RuleCompact,
			Argv: st.Action.Steps, Before: st.Tokens, Guard: "idle", Result: stoker.Sent,
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
	if changed {
		s.refreshStoker()
	}
}

// refreshStoker sends the stoker view at once.
func (s *Service) refreshStoker() {
	v := s.Stoker.View()
	s.mu.Lock()
	s.cur.Stoker = &v
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
func (s *Service) stokable(pane string, typed bool) error {
	if !s.idle(pane) {
		return fmt.Errorf("the session in %s is busy", pane)
	}
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
	if !typed && draft != "" {
		return fmt.Errorf("the pane %s has text you typed", pane)
	}
	if typed && !strings.HasPrefix(draft, "/compact") {
		return fmt.Errorf("the pane %s does not show the compact", pane)
	}
	return nil
}
