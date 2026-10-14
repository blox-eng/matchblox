package main

import (
	"slices"
	"testing"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/remote"
)

// The matchblox key starts the service stream or the console and nothing else: jumps and
// door commands use the builder's own login, so a tmux client (which can
// open a shell) or an agent is never one of its commands.
func TestGateRunsOnlyItsCommands(t *testing.T) {
	for orig, want := range map[string][]string{
		remote.ServeCommand:   {"serve", "--stdio"},
		remote.ConsoleCommand: {},
	} {
		got, err := gateDecision(orig)
		if err != nil || !slices.Equal(got, want) || got == nil {
			t.Fatalf("gateDecision(%q) = %q, %v; want %q", orig, got, err, want)
		}
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
		"matchblox console; id",
		"matchblox console ",
		"matchblox  console",
		"matchblox gate pair 0123456789ab",
	} {
		if argv, err := gateDecision(orig); err == nil || argv != nil {
			t.Errorf("gateDecision(%q) let it run", orig)
		}
	}
}
