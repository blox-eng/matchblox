package service

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/proto"
)

// doneFor is how long an act that ran answers "already done" to the same
// request: long enough for the next git scan to drop the rec.
const doneFor = 10 * time.Minute

// acts serialises the acts on each action and remembers which ran, so two
// consoles that confirm the same step run it once.
type acts struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
	done  map[string]time.Time
}

func (a *acts) lock(key string) *sync.Mutex {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.locks == nil {
		a.locks, a.done = map[string]*sync.Mutex{}, map[string]time.Time{}
	}
	l, ok := a.locks[key]
	if !ok {
		l = &sync.Mutex{}
		a.locks[key] = l
	}
	return l
}

func (a *acts) recent(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	at, ok := a.done[key]
	return ok && time.Since(at) < doneFor
}

func (a *acts) markDone(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for k, at := range a.done {
		if now.Sub(at) >= doneFor {
			delete(a.done, k)
			delete(a.locks, k)
		}
	}
	a.done[key] = now
}

func (s *Service) act(ctx context.Context, a proto.Act) proto.Result {
	if a.RecID == "rescan:git" {
		s.rescanGit()
		return proto.Result{}
	}
	key := a.RecID + "\x00" + a.Which
	l := s.acts.lock(key)
	l.Lock()
	defer l.Unlock()
	if s.acts.recent(key) {
		return proto.Result{Skipped: []string{"already done"}}
	}
	action, ok := s.resolve(a.RecID, a.Which)
	if !ok {
		return proto.Result{Err: "unknown action"}
	}
	if action.Nav {
		return proto.Result{Err: "runs in the console"}
	}
	res := s.Actions.Do(ctx, action, a.Confirm)
	if len(res.Ran) > 0 {
		s.acts.markDone(key)
	}
	return res
}

// resolve finds the action in the service's own state: a rec by its id, or
// the x action of a panel row ("orphan:<pid>", "worktree:<path>").
func (s *Service) resolve(id, which string) (advice.Action, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.cur.Recommendations {
		if r.ID != id {
			continue
		}
		a := r.Primary
		if which == "secondary" {
			a = r.Second
		}
		if a == nil || len(a.Steps) == 0 {
			return advice.Action{}, false
		}
		return *a, true
	}
	if which != "secondary" {
		return advice.Action{}, false
	}
	if v, ok := strings.CutPrefix(id, "orphan:"); ok {
		pid, _ := strconv.Atoi(v)
		for _, o := range s.cur.Orphans {
			if o.PID == pid && pid > 0 && len(o.Kill) > 0 {
				return advice.Action{Label: "kill", Steps: [][]string{o.Kill}, Destructive: true,
					Guards: []advice.Guard{{PID: o.PID, StartTicks: o.Start}}}, true
			}
		}
	}
	if path, ok := strings.CutPrefix(id, "worktree:"); ok && s.cur.Git != nil {
		for _, r := range s.cur.Git.Repos {
			for _, wt := range r.Worktrees {
				if wt.Path == path && wt.Safe && len(wt.Remove) > 0 {
					return advice.Action{Label: "remove", Steps: [][]string{wt.Remove}, Destructive: true,
						Guards: []advice.Guard{{Worktree: wt.Path}}}, true
				}
			}
		}
	}
	return advice.Action{}, false
}
