package main

import (
	"slices"
	"testing"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/remote"
)

func TestGateRunsOnlyMatchbloxCommands(t *testing.T) {
	tg := remote.Target{Host: "ws-1", Key: "/k"}
	last := func(a []string) string { return a[len(a)-1] }
	for _, c := range []struct {
		orig string
		kind string
		argv []string
	}{
		{remote.ServeCommand, "serve", nil},
		{last(tg.NavArgv([][]string{{"tmux", "switch-client", "-t", "%12"}})), "exec",
			[]string{"tmux", "select-window", "-t", "%12", ";", "select-pane", "-t", "%12", ";", "attach-session", "-t", "%12"}},
		{last(tg.NavArgv([][]string{{"tmux", "new-window", "-c", "/w/a"}})), "exec",
			[]string{"tmux", "new-window", "-c", "/w/a", ";", "attach-session"}},
		{last(tg.TermArgv(doors.GuideArgv(false))), "exec", doors.GuideArgv(false)},
		{last(tg.TermArgv([]string{"sudo", "sh", "-c", "apt-get update && apt-get install -y tmux"})), "exec",
			[]string{"sudo", "sh", "-c", "apt-get update && apt-get install -y tmux"}},
		// Refused: a shell, a shell string, a tmux command that runs one,
		// an empty command (an interactive login), anything quoted otherwise.
		{"", "", nil},
		{"sh", "", nil},
		{"bash -c id", "", nil},
		{"tmux new-window 'rm -rf ~'", "", nil},
		{"tmux attach-session ';' run-shell id", "", nil},
		{"tmux select-window -t %1 ';' new-window -c '/w/#(id)'", "", nil},
		{"matchblox serve --stdio; id", "", nil},
		{`sudo sh -c "apt-get update && apt-get install -y tmux"`, "", nil},
		{"tmux", "", nil},
	} {
		kind, argv, err := gateDecision(c.orig)
		if kind != c.kind || !slices.Equal(argv, c.argv) || (c.kind == "") != (err != nil) {
			t.Errorf("gateDecision(%q) = %q %q %v; want %q %q", c.orig, kind, argv, err, c.kind, c.argv)
		}
	}
}
