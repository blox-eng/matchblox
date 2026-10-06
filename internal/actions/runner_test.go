package actions

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/advice"
)

func TestDestructiveNeedsConfirm(t *testing.T) {
	r := Runner{Run: func([]string) error { t.Fatal("ran without confirm"); return nil }}
	res := r.Do(context.Background(), advice.Action{Steps: [][]string{{"kill", "1"}}, Destructive: true}, "")
	if res.Err != "needs confirm" {
		t.Fatalf("err %q", res.Err)
	}
}

func TestFailedGuardSkipsStep(t *testing.T) {
	var ran []string
	r := Runner{
		Run: func(argv []string) error { ran = append(ran, strings.Join(argv, " ")); return nil },
		Check: func(g advice.Guard) error {
			if g.Worktree == "/w/a" {
				return errors.New("/w/a has changes")
			}
			return nil
		},
		Idle: func(pane string) bool { return pane == "%1" },
	}
	a := advice.Action{
		Steps:       [][]string{{"rm", "a"}, {"rm", "b"}, {"send", "1"}, {"send", "2"}},
		Guards:      []advice.Guard{{Worktree: "/w/a"}, {Worktree: "/w/b"}, {IdlePane: "%1"}, {IdlePane: "%2"}},
		Destructive: true,
	}
	res := r.Do(context.Background(), a, "y")
	if got := strings.Join(ran, ","); got != "rm b,send 1" {
		t.Fatalf("ran %q", got)
	}
	if len(res.Ran) != 2 || len(res.Skipped) != 2 || res.Err != "" {
		t.Fatalf("result %+v", res)
	}
	if !strings.Contains(res.Skipped[0], "/w/a has changes") || !strings.Contains(res.Skipped[1], "%2 is no longer idle") {
		t.Fatalf("skipped %q", res.Skipped)
	}
}

func TestFailedStepStops(t *testing.T) {
	n := 0
	r := Runner{Run: func([]string) error { n++; return errors.New("exit 1") }}
	res := r.Do(context.Background(), advice.Action{Steps: [][]string{{"a"}, {"b"}}}, "")
	if n != 1 || !strings.Contains(res.Err, "a: exit 1") {
		t.Fatalf("n=%d err=%q", n, res.Err)
	}
}

// A step must never stop to ask for credentials.
func TestExecNeverPrompts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	if err := Exec(context.Background(), []string{"sh", "-c", `test "$GIT_TERMINAL_PROMPT" = 0 && test -n "$GIT_SSH_COMMAND"`}); err != nil {
		t.Fatalf("step ran with prompts allowed: %v", err)
	}
}
