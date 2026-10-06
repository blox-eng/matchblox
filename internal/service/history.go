package service

import (
	"os/exec"
	"time"

	"github.com/blox-eng/matchblox/internal/history"
	"github.com/blox-eng/matchblox/internal/state"
)

// stateEvery is how often the state file is rewritten: it is for agents
// that ask now and then, so every few seconds is fresh.
const stateEvery = 5 * time.Second

// recorder persists each sample: the state file, one history row every
// history.Every, and the alert hook for alerts that were not firing before.
func (s *Service) recorder() func(state.Doc) {
	logger := &history.Logger{Path: s.HistoryLog}
	firing := map[string]bool{}
	first := true
	var wrote time.Time
	hook := s.AlertHook
	return func(d state.Doc) {
		if s.StatePath != "" && (wrote.IsZero() || d.At.Sub(wrote) >= stateEvery) {
			_ = state.Write(s.StatePath, d)
			wrote = d.At
		}
		if !first && s.HistoryLog != "" {
			_ = logger.Log(history.FromSnapshot(d.Snapshot))
		}
		first = false
		now := map[string]bool{}
		for _, a := range d.Alerts {
			now[a.Key] = true
			if !firing[a.Key] && len(hook) > 0 {
				c := exec.Command(hook[0], append(hook[1:], a.Title, a.Evidence)...) //nolint:gosec // the person's own hook argv
				if c.Start() == nil {
					go func() { _ = c.Wait() }()
				}
			}
		}
		firing = now
	}
}
