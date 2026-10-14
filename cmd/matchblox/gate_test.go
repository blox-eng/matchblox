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
	for _, c := range []struct {
		mode, orig string
		want       []string
	}{
		{"", remote.ServeCommand, []string{"serve", "--stdio"}},
		{"phone", remote.ServeCommand, []string{"serve", "--stdio"}},
		{"phone", remote.ConsoleCommand, []string{}},
	} {
		got, err := gateDecision(c.mode, c.orig)
		if err != nil || !slices.Equal(got, c.want) || got == nil {
			t.Fatalf("gateDecision(%q, %q) = %q, %v; want %q", c.mode, c.orig, got, err, c.want)
		}
	}
	// A laptop key never gets the console, and an unknown mode gets nothing.
	if argv, err := gateDecision("", remote.ConsoleCommand); err == nil || argv != nil {
		t.Errorf("a laptop key opened the console: %q", argv)
	}
	for _, orig := range []string{remote.ServeCommand, remote.ConsoleCommand, ""} {
		if argv, err := gateDecision("pair", orig); err == nil || argv != nil {
			t.Errorf("unknown mode ran %q", orig)
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
		for _, mode := range []string{"", "phone"} {
			if argv, err := gateDecision(mode, orig); err == nil || argv != nil {
				t.Errorf("gateDecision(%q, %q) let it run", mode, orig)
			}
		}
	}
}

func TestGateRefusalNamesTheFixOfItsMode(t *testing.T) {
	for _, c := range []struct{ mode, orig, want string }{
		{"", "", "this key starts only the matchblox stream; use your own ssh login for a shell"},
		{"", "sh", "this key starts only the matchblox stream; use your own ssh login for a shell"},
		{"phone", "", "this key starts only the matchblox stream and console; use your own ssh login for a shell"},
		{"phone", "sh", "this key starts only the matchblox stream and console; use your own ssh login for a shell"},
		{"", remote.ConsoleCommand, "this key starts only the matchblox stream; run matchblox pair to add a phone"},
	} {
		if _, err := gateDecision(c.mode, c.orig); err == nil || err.Error() != c.want {
			t.Errorf("gateDecision(%q, %q) error = %v, want %q", c.mode, c.orig, err, c.want)
		}
	}
}
