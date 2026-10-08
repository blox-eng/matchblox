package service

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
)

// doneFor is how long an act that ran answers "already done" to the same
// request: long enough for the next git scan to drop the rec.
var doneFor = 10 * time.Minute

// acts lets one act on each action run at a time and remembers which ran,
// so two consoles that confirm the same step run it once. A second confirm
// waits for the first, then answers "already done".
type acts struct {
	mu       sync.Mutex
	inflight map[string]chan struct{}
	done     map[string]time.Time
}

// begin waits until no act on key runs. It reports false when key ran
// within doneFor; otherwise the caller owns key until end.
func (a *acts) begin(key string) bool {
	for {
		a.mu.Lock()
		if a.inflight == nil {
			a.inflight, a.done = map[string]chan struct{}{}, map[string]time.Time{}
		}
		if at, ok := a.done[key]; ok && time.Since(at) < doneFor {
			a.mu.Unlock()
			return false
		}
		wait, busy := a.inflight[key]
		if !busy {
			a.inflight[key] = make(chan struct{})
			a.mu.Unlock()
			return true
		}
		a.mu.Unlock()
		<-wait
	}
}

func (a *acts) end(key string, ran bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for k, at := range a.done {
		if now.Sub(at) >= doneFor {
			delete(a.done, k)
		}
	}
	if ran {
		a.done[key] = now
	}
	close(a.inflight[key])
	delete(a.inflight, key)
}

func (s *Service) act(ctx context.Context, a proto.Act) proto.Result {
	switch a.RecID {
	case "rescan:git":
		s.rescanGit()
		return proto.Result{}
	case "service:replace":
		s.replace()
		return proto.Result{}
	}
	key := a.RecID + "\x00" + a.Which
	pane, answer := strings.CutPrefix(a.RecID, "answer:")
	if answer {
		// The same text to a new question is a new answer: the key holds
		// the time the session started to wait.
		it, _ := s.queued(pane)
		key += "\x00" + a.Text + "\x00" + it.Since.String()
	}
	if !s.acts.begin(key) {
		return proto.Result{Skipped: []string{"already done"}}
	}
	ran := false
	defer func() { s.acts.end(key, ran) }()
	if answer {
		if err := panes.CheckAnswer(a.Text); err != nil {
			return proto.Result{Err: err.Error()}
		}
		switch it, ok := s.queued(pane); {
		case !ok:
			return proto.Result{Err: "the session in " + pane + " no longer waits"}
		case it.State == queue.StatePermission:
			return proto.Result{Err: "a permission prompt is answered in its pane: Enter goes there"}
		}
		res := s.Actions.Do(ctx, answerAction(pane, a.Text), a.Confirm)
		ran = len(res.Ran) > 0
		return res
	}
	action, ok := s.resolve(a.RecID, a.Which)
	if !ok {
		return proto.Result{Err: "unknown action"}
	}
	if action.Nav {
		return proto.Result{Err: "runs in the console"}
	}
	res := s.Actions.Do(ctx, action, a.Confirm)
	ran = len(res.Ran) > 0
	return res
}

// resolve finds the action in the service's own state: a rec by its id, or
// the x action of a panel row ("orphan:<pid>", "worktree:<path>",
// "session:<pid>" for a stale session).
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
	if v, ok := strings.CutPrefix(id, "session:"); ok {
		// Only an agent idle a week or more, as this service sampled it.
		pid, _ := strconv.Atoi(v)
		for _, x := range s.cur.Sessions {
			if x.PID == pid && pid > 0 && x.Age() == "stale" {
				return advice.Action{Label: "end", Steps: [][]string{{"kill", strconv.Itoa(pid)}}, Destructive: true,
					Guards: []advice.Guard{{PID: pid, StartTicks: x.Start, IdlePane: x.Pane}}}, true
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

// answerAction types one line and Enter. Both steps are guarded: a bare
// Enter at a permission prompt would approve it.
func answerAction(pane, text string) advice.Action {
	g := advice.Guard{AnswerPane: pane}
	return advice.Action{Label: "answer", Steps: panes.Send(pane, text), Destructive: true, Guards: []advice.Guard{g, g}}
}

// queued finds the queue item of a pane. Hold no lock.
func (s *Service) queued(pane string) (queue.Item, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range s.cur.Queue {
		if it.Pane == pane {
			return it, true
		}
	}
	return queue.Item{}, false
}

// answerable is the guard of an answer, checked right before each step.
func (s *Service) answerable(pane string) bool {
	it, ok := s.queued(pane)
	return ok && it.State != queue.StatePermission
}
