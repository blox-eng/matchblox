package service

import (
	"context"
	"fmt"
	"slices"
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
// ("wake"), and each key keeps the rest ("touch"), on unit u. None is
// destructive: the stoker itself never ends, kills or removes anything.
// The host's night is the family's: it puts each chain unit whose run is
// on into the night, and its end ends theirs.
func (s *Service) stokerAct(u *stokerUnit, verb string, now time.Time) proto.Result {
	if u == nil {
		return proto.Result{Err: "this service keeps no stoker"}
	}
	var chains []stoker.ChainCard
	if m := u.store.Mode(); verb == "off" && s.chainIDOf(u) == "" && m.On && m.Nightly {
		s.endChainNights(now)
		chains = s.nightCards(m.Since)
	}
	sessions, waiting := s.seen()
	var refused string
	_, err := u.store.Update(func(m stoker.Mode) (stoker.Mode, bool) {
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
			m = m.Off(now).Score(sessions, waiting, s.full, u.store.View().Run)
			m.Card.Chains = chains
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
	if verb == "night" && s.chainIDOf(u) == "" {
		s.nightFamily(now)
	}
	s.refreshStoker()
	return proto.Result{}
}

// chainUnits are the chain units of the family, by id.
func (s *Service) chainUnits() []*stokerUnit {
	s.fam.mu.Lock()
	defer s.fam.mu.Unlock()
	return slices.Clone(s.fam.chains)
}

// nightFamily puts each chain unit whose run is on into the night the host
// stoker runs: it is sealed, and ends at night_ends.
func (s *Service) nightFamily(now time.Time) {
	for _, u := range s.chainUnits() {
		_, _ = u.store.Update(func(m stoker.Mode) (stoker.Mode, bool) {
			if !m.On || m.Nightly {
				return m, false
			}
			n, err := m.Night(now, s.NightEnds)
			return n, err == nil
		})
	}
}

// endChainNights ends the run of each chain unit that is on at at, as the
// host's night ends, and scores its card. It tells if one ended.
func (s *Service) endChainNights(at time.Time) bool {
	sessions, waiting := s.seen()
	ended := false
	for _, u := range s.chainUnits() {
		mine := s.sessionsOf(u, sessions)
		_, _ = u.store.Update(func(m stoker.Mode) (stoker.Mode, bool) {
			if !m.On {
				return m, false
			}
			ended = true
			return m.Off(at).Score(mine, waiting, s.full, u.store.View().Run), true
		})
	}
	return ended
}

// nightCards are the cards of the chain units whose night ended in the
// host's run that began at since, for its morning card.
func (s *Service) nightCards(since time.Time) []stoker.ChainCard {
	var out []stoker.ChainCard
	for _, u := range s.chainUnits() {
		if c := u.store.Mode().Card; c != nil && c.Night && !c.Ended.Before(since) {
			out = append(out, stoker.ChainCard{Name: s.unitName(u), Stoker: u.id, Card: *c})
		}
	}
	return out
}

// sessionsOf are the sessions whose match unit u owns: a chain unit's
// chain members while its run is on, and every other one the host's.
func (s *Service) sessionsOf(u *stokerUnit, sessions []sample.Session) []sample.Session {
	var out []sample.Session
	for _, x := range sessions {
		if s.ownerOf(x) == u {
			out = append(out, x)
		}
	}
	return out
}

// stoke runs after each sample, for each unit of the family: it ends a
// night at its morning, reads back what the compacts it sent left, and
// sends the steps the plan finds on the matches the unit owns. Each step
// goes through the runner, so its idle guard is checked right before the
// send. A night ends for the whole family in one pass: the chain units
// first, so the host's morning card lists their cards.
func (s *Service) stoke(ctx context.Context, now time.Time) {
	host := s.hostUnit()
	if host == nil {
		return
	}
	sessions, waiting := s.seen()
	changed := false
	var chains []stoker.ChainCard
	if m := host.store.Mode(); m.On && m.Nightly && !m.Until.IsZero() && !now.Before(m.Until) {
		changed = s.endChainNights(m.Until)
		chains = s.nightCards(m.Since)
	}
	fam := s.family()
	for _, u := range fam {
		if s.stokeUnit(ctx, u, now, sessions, waiting, chains) {
			changed = true
		}
	}
	if changed {
		s.refreshStoker()
	} else {
		s.syncDriven() // a lost driven.json comes back
	}
	for _, u := range fam {
		s.wake(ctx, u, now)
	}
}

// stokeUnit is stoke for unit u; chains go on the host's morning card. It
// tells if u's card changed.
func (s *Service) stokeUnit(ctx context.Context, u *stokerUnit, now time.Time, sessions []sample.Session, waiting map[string]bool, chains []stoker.ChainCard) bool {
	changed := false
	mine := s.sessionsOf(u, sessions)
	seen := sessions // the host reads the whole machine, as it always has
	if s.chainIDOf(u) != "" {
		seen = mine
	}
	s.mu.Lock()
	alone := s.alone
	s.mu.Unlock()
	var typed time.Time // the builder's last key in tmux, read only during a rest
	if !u.store.Mode().Rests.IsZero() && s.Activity != nil {
		typed = s.Activity()
	}
	// One update for all three, so an act in between is never undone (a
	// write that fails is tried again at the next sample).
	_, _ = u.store.Update(func(m stoker.Mode) (stoker.Mode, bool) {
		m, saw := m.Observe(seen, waiting, s.full, now)
		m = m.Touch(typed)
		m, woke := m.Awake(now, alone)
		m, ended := m.Expire(now)
		if ended {
			m = m.Score(seen, waiting, s.full, u.store.View().Run)
			if s.chainIDOf(u) == "" && m.Card.Night {
				m.Card.Chains = chains
			}
		}
		changed = saw || woke || ended
		return m, changed
	})

	for _, e := range u.store.Pending(now) {
		path := ""
		for _, ss := range sessions {
			if ss.SessionID == e.Session {
				path = ss.Transcript
			}
		}
		if c, ok := sample.Compacted(path, e.At); ok && path != "" {
			_ = u.store.Append(stoker.Entry{ID: e.ID, Before: c.Before, After: c.After, Resume: c.Resume, Back: now})
			changed = true
		}
	}

	for _, st := range stoker.Plan(u.store.Mode(), mine, waiting, s.full, u.store.Entries(), now, s.heldSessions()) {
		if m := u.store.Mode(); !m.On || !m.Rests.IsZero() {
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
		_ = u.store.Append(e)
		changed = true
	}
	if s.fire(ctx, u, now) {
		changed = true
	}
	return changed
}

// touchStoker keeps each unit's rest while the builder acts from a
// console.
func (s *Service) touchStoker(now time.Time) {
	for _, u := range s.family() {
		_, _ = u.store.Update(func(m stoker.Mode) (stoker.Mode, bool) {
			return m.Touch(now), !m.Rests.IsZero()
		})
	}
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
	for pane := range s.refreshes {
		waiting[pane] = true
	}
	return s.cur.Sessions, waiting
}

// refreshStoker sends the host stoker's view at once.
func (s *Service) refreshStoker() {
	sessions, _ := s.seen()
	var v *stoker.View
	if u := s.hostUnit(); u != nil {
		v = s.stokerView(u, sessions)
	}
	stokers := s.chainStokers(sessions)
	s.syncDriven()
	s.mu.Lock()
	s.cur.Stoker, s.cur.Stokers = v, stokers
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
	return s.stokableFor(pane, typed, nil)
}

// stokableFor is stokable, except that own, a refresh that types its own
// request, passes the pane's switching mark.
func (s *Service) stokableFor(pane, typed string, own *refreshRun) error {
	if err := s.switchingErr(pane, own); err != nil {
		return err
	}
	if it, ok := s.queued(pane); ok && stoker.Waits(it) {
		return fmt.Errorf("the session in %s needs you", pane)
	}
	screen, ok := s.Capture([]string{pane})[pane]
	if !ok {
		return fmt.Errorf("the pane %s cannot be read", pane)
	}
	return stokableScreen(pane, screen, typed)
}

// stokableScreen is stokable on a screen already read.
func stokableScreen(pane, screen, typed string) error {
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
