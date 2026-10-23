package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
)

// signInWait and signInEvery are how long the driver waits for a new
// account's screens, and how often it looks at the pane.
const (
	signInWait  = 20 * time.Second
	signInEvery = 500 * time.Millisecond
)

// SignInByHand is the note a pane gets when matchblox gave up driving its
// sign-in: the builder finishes it.
const SignInByHand = "by hand"

// signInScreens are the first screens of each agent on a new account and
// the keys that move toward its sign-in link, in the order the agent shows
// them. Probed on Claude Code 2.1.308, codex-cli 0.163.0 and Cursor
// 2026.10.19. final marks the screen after which the link shows.
var signInScreens = map[string][]struct {
	match string
	keys  []string
	final bool
}{
	"claude": {
		{"Choose the text style", []string{"Enter"}, false},
		{"Select login method", []string{"Enter"}, true},
	},
	"codex":            {{"Sign in with ChatGPT", []string{"Down", "Enter"}, true}},
	sample.CursorAgent: {{"Press any key to log in", []string{"Enter"}, true}},
}

// signInStep is the screen of agent that screen shows: its index in the
// agent's table, or -1. The screen is cleaned of its styles first, and the
// later screen wins: one still showing an earlier prompt above a later one
// is at the later one.
func signInStep(agent, screen string) int {
	screen = said.Clean(screen)
	steps := signInScreens[agent]
	for i := len(steps) - 1; i >= 0; i-- {
		if strings.Contains(screen, steps[i].match) {
			return i
		}
	}
	return -1
}

// signInKeys are the keys that answer the sign-in screen the agent shows;
// ok is false for any other screen. Only the keys leave this function,
// never a line of the screen.
func signInKeys(agent, screen string) (keys []string, ok bool) {
	i := signInStep(agent, screen)
	if i < 0 {
		return nil, false
	}
	return signInScreens[agent][i].keys, true
}

// driveSignIn waits for the first screens of the agent in pane, a new
// account's, and sends the keys that reach its sign-in link, each screen's
// once. It never reads the link or the code: the builder finishes the
// sign-in. When the screens do not come in time, or ctx ends first, it
// stops; after the time it notes the pane for the builder. It never kills
// the pane.
func (s *Service) driveSignIn(ctx context.Context, pane, agent, name string) {
	wait, every := s.signInWait, s.signInEvery
	if wait <= 0 {
		wait = signInWait
	}
	if every <= 0 {
		every = signInEvery
	}
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	tick := time.NewTicker(every)
	defer tick.Stop()
	sent := map[int]bool{}
	for {
		if screen, ok := s.Capture([]string{pane})[pane]; ok {
			if i := signInStep(agent, screen); i >= 0 && !sent[i] {
				sent[i] = true
				if !s.sendSignIn(ctx, pane, signInScreens[agent][i].keys) {
					s.signIns.set(pane, SignInByHand)
					s.noteSignIn()
					return
				}
				if signInScreens[agent][i].final {
					return
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			s.signIns.set(pane, SignInByHand)
			s.noteSignIn()
			return
		case <-tick.C:
		}
	}
}

// sendSignIn presses ks in pane, one send-keys each, and tells if they all
// went.
func (s *Service) sendSignIn(ctx context.Context, pane string, ks []string) bool {
	act := advice.Action{Label: "sign in"}
	for i, k := range ks {
		if i > 0 {
			act.Steps = append(act.Steps, panes.Pause)
		}
		act.Steps = append(act.Steps, panes.Keys(pane, k))
	}
	release := s.holdPane(pane)
	defer release()
	res := s.Actions.Do(ctx, act, "")
	return res.Err == "" && len(res.Skipped) == 0 && len(res.Ran) == len(act.Steps)
}

// noteSignIn puts the notes in the state the consoles get now.
func (s *Service) noteSignIn() {
	s.mu.Lock()
	s.cur.SignIns = s.signIns.all()
	s.mu.Unlock()
	s.broadcast()
}

// signInState keeps the notes on the sign-ins, by pane, or by session name
// when the pane was not found.
type signInState struct {
	mu   sync.Mutex
	note map[string]string
}

func (r *signInState) set(pane, note string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.note == nil {
		r.note = map[string]string{}
	}
	r.note[pane] = note
}

// all are the notes. Nil: none.
func (r *signInState) all() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.copyLocked()
}

// keep drops the notes of panes and sessions that are gone and returns the
// rest.
func (r *signInState) keep(here map[string]bool) map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for pane := range r.note {
		if !here[pane] {
			delete(r.note, pane)
		}
	}
	return r.copyLocked()
}

func (r *signInState) copyLocked() map[string]string {
	if len(r.note) == 0 {
		return nil
	}
	out := make(map[string]string, len(r.note))
	for k, v := range r.note {
		out[k] = v
	}
	return out
}
