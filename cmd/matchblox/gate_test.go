package main

import (
	"testing"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/remote"
)

// The matchblox key starts the service stream and nothing else: jumps and
// door commands use the builder's own login, so a tmux client (which can
// open a shell) or an agent is never one of its commands.
func TestGateRunsOnlyTheServiceStream(t *testing.T) {
	if err := gateDecision(remote.ServeCommand); err != nil {
		t.Fatalf("the stream: %v", err)
	}
	last := func(a []string) string { return a[len(a)-1] }
	for _, orig := range []string{
		"", "sh", "bash -c id", "id",
		"tmux attach-session",
		"tmux select-window -t %12 ';' select-pane -t %12 ';' attach-session -t %12",
		"tmux new-window -c / ';' attach-session",
		last(remote.NavArgv("ws-1", [][]string{{"tmux", "switch-client", "-t", "%12"}})),
		last(remote.TermArgv("ws-1", doors.GuideArgv(false))),
		remote.Quote(doors.GuideArgv(false)),
		"matchblox serve --stdio; id",
		"matchblox serve --stdio ",
		"matchblox serve",
	} {
		if err := gateDecision(orig); err == nil {
			t.Errorf("gateDecision(%q) let it run", orig)
		}
	}
}
