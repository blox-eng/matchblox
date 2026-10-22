package main

import (
	"os"
	"os/exec"

	"github.com/blox-eng/matchblox/internal/logos"
)

// consoleGraphics tells the console whether every terminal that shows it
// draws the providers' logos (#183). In tmux it lets images pass through
// the console's own pane only.
func consoleGraphics() func() (bool, string) {
	tmux := func(args ...string) ([]byte, error) { return exec.Command("tmux", args...).Output() }
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		_, _ = tmux("set-option", "-p", "-t", pane, "allow-passthrough", "on") // an older tmux: marks stay
	}
	return func() (bool, string) { return logos.Viewers(os.Getenv, tmux) }
}
