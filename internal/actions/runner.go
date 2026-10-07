// Package actions runs the steps of an action on the service host. Each
// guard is checked again just before its step: the evidence can be minutes
// old.
package actions

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/proto"
)

// StepTimeout bounds every step; a git fetch over a dead network would
// otherwise hang the action forever.
const StepTimeout = 2 * time.Minute

type Runner struct {
	// Run executes one step. Nil means Exec.
	Run func(argv []string) error
	// Check verifies a pid or worktree guard. Nil checks nothing.
	Check func(advice.Guard) error
	// Idle tells if the agent in a pane is idle now. Nil means never idle.
	Idle func(pane string) bool
	// Answerable tells if the agent in a pane still waits for a typed
	// answer (not at a permission prompt). Nil means never.
	Answerable func(pane string) bool
}

// Do runs each step whose guard still holds and records why it skipped the
// others. A destructive action runs only with confirm "y".
func (r Runner) Do(ctx context.Context, a advice.Action, confirm string) proto.Result {
	if a.Destructive && confirm != "y" {
		return proto.Result{Err: "needs confirm"}
	}
	run := r.Run
	if run == nil {
		run = func(argv []string) error { return Exec(ctx, argv) }
	}
	var res proto.Result
	for i, step := range a.Steps {
		if i < len(a.Guards) {
			if err := r.guard(a.Guards[i]); err != nil {
				res.Skipped = append(res.Skipped, strings.Join(step, " ")+": "+err.Error())
				continue
			}
		}
		if err := run(step); err != nil {
			res.Err = strings.Join(step, " ") + ": " + err.Error()
			return res
		}
		res.Ran = append(res.Ran, step)
	}
	return res
}

func (r Runner) guard(g advice.Guard) error {
	if g.IdlePane != "" && (r.Idle == nil || !r.Idle(g.IdlePane)) {
		return fmt.Errorf("the session in %s is no longer idle", g.IdlePane)
	}
	if g.AnswerPane != "" && (r.Answerable == nil || !r.Answerable(g.AnswerPane)) {
		return fmt.Errorf("the session in %s no longer waits for an answer", g.AnswerPane)
	}
	if r.Check != nil {
		return r.Check(g)
	}
	return nil
}

// Exec runs one step without a terminal: git and ssh fail instead of
// prompting for credentials.
func Exec(ctx context.Context, argv []string) error {
	ctx, cancel := context.WithTimeout(ctx, StepTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // argv built by advice, never a shell string
	c.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	return c.Run()
}
