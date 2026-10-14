package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// whenAsk is what `matchblox stoker when` sends: a trigger to set, or the
// id of one to drop.
type whenAsk struct {
	stoker.Trigger
	Drop string `json:"drop,omitempty"`
}

// when sets a trigger for the run on, or drops one. The sessions it names
// are read now: the pane of each is what it fires on and acts on.
func (s *Service) when(text string) error {
	var ask whenAsk
	if err := json.Unmarshal([]byte(text), &ask); err != nil {
		return fmt.Errorf("the trigger does not read: %w", err)
	}
	if ask.Drop != "" {
		return s.updateOrder(func(m stoker.Mode) (stoker.Mode, error) {
			i := slices.IndexFunc(m.Triggers, func(t stoker.Trigger) bool { return t.ID == ask.Drop })
			if i < 0 {
				return m, fmt.Errorf("no trigger %s: `matchblox stoker when --list` lists them", ask.Drop)
			}
			m.Triggers = slices.Delete(slices.Clone(m.Triggers), i, i+1)
			return m, nil
		})
	}
	t := ask.Trigger
	t.Text = strings.TrimSpace(t.Text)
	if err := t.Check(); err != nil {
		return err
	}
	switch t.Do {
	case "send":
		if err := panes.CheckAnswer(t.Text); err != nil {
			return err
		}
	case "hold":
		why, err := stoker.Why(t.Text)
		if err != nil {
			return err
		}
		t.Text = why
	}
	if t.Rest != "" {
		x, err := s.sessionNamed(t.Rest)
		if err != nil {
			return err
		}
		t.Rest, t.RestPane = x.Name, x.Pane
	}
	x, err := s.sessionNamed(t.Who)
	if err != nil {
		return err
	}
	t.Who, t.Pane, t.At = x.Name, x.Pane, s.now()
	return s.updateOrder(func(m stoker.Mode) (stoker.Mode, error) {
		if !m.On {
			return m, errors.New("the stoker is off: no run")
		}
		m.NextTrigger++
		t.ID = "t" + strconv.Itoa(m.NextTrigger)
		m.Triggers = append(slices.Clip(m.Triggers), t)
		return m, nil
	})
}

// fire runs the triggers whose event happened, each once, with the hands
// and guards of the stoker's own acts. Each logs a step to the card and is
// told to the stoker at its next wake. While the builder took over, the
// triggers wait. A trigger is taken out of the run's own list under its
// lock before it runs, so only what was taken out fires, and nothing when
// the write fails.
func (s *Service) fire(ctx context.Context, now time.Time) bool {
	if m := s.Stoker.Mode(); !m.On || len(m.Triggers) == 0 {
		return false
	}
	sessions, waiting := s.seen()
	s.mu.Lock()
	rep := s.cur.Ships
	s.mu.Unlock()
	var due []stoker.Trigger
	err := s.updateOrder(func(m stoker.Mode) (stoker.Mode, error) {
		due = nil
		if !m.On || !m.Rests.IsZero() {
			return m, errNoneDue
		}
		var left []stoker.Trigger
		for _, t := range m.Triggers {
			if t.Due(rep, sessions, waiting) {
				due = append(due, t)
			} else {
				left = append(left, t)
			}
		}
		if len(due) == 0 {
			return m, errNoneDue
		}
		m.Triggers = left
		return m, nil
	})
	if err != nil {
		return false
	}
	for _, t := range due {
		res := s.runTrigger(ctx, t)
		e := stoker.Entry{
			ID: "when@" + now.UTC().Format(time.RFC3339Nano) + "/" + t.ID, At: now, Name: t.Who, Pane: t.Pane,
			Rule: stoker.RuleWhen, State: stoker.StepDone, Argv: res.Ran, Text: "when " + t.Event() + " → " + t.Done(),
		}
		if x, ok := s.sessionAt(t.Pane); ok {
			e.Session = x.SessionID
		}
		line := "when " + t.Event() + ": " + t.Done()
		if why := refusal(res); why != "" {
			e.State = stoker.StepFailed
			e.Text = "when " + t.Event() + " → " + t.Action() + " failed: " + why
			line = "when " + t.Event() + ": failed: " + t.Action() + ": " + why
		}
		_ = s.Stoker.Append(e)
		s.wakes.add(line)
	}
	return true
}

// errNoneDue leaves the run's triggers as they are.
var errNoneDue = errors.New("no trigger is due")

// runTrigger runs a trigger's hand as an act from the stoker's own pane:
// the same refusals, counts and marks as if the stoker ran it.
func (s *Service) runTrigger(ctx context.Context, t stoker.Trigger) proto.Result {
	pane, err := s.stokerAgent()
	if err != nil {
		return proto.Result{Err: err.Error()}
	}
	a := proto.Act{RecID: t.Do + ":" + t.Pane, Confirm: "y", From: pane}
	switch t.Do {
	case "send":
		a.RecID, a.Text = "message:"+t.Pane, t.Text
	case "hold":
		a.Text = t.Text
	}
	return s.act(ctx, a)
}

// refusal is why an act did not run, or "".
func refusal(res proto.Result) string {
	switch {
	case res.Err != "":
		return res.Err
	case len(res.Ran) == 0 && len(res.Skipped) > 0:
		return res.Skipped[0]
	case len(res.Ran) == 0:
		return "nothing ran"
	}
	return ""
}
