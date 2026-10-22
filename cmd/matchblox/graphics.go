package main

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/blox-eng/matchblox/internal/logos"
)

// graphicsTimeout bounds each tmux call the console makes to learn who
// shows it: a wedged tmux costs the logos, not a pile of processes.
const graphicsTimeout = time.Second

// consoleGraphics tells the console whether every terminal that shows it
// draws the providers' logos (#183). In tmux it lets images pass through
// the console's own pane only, while the pane is hidden too ("all"), so
// logos sent then reach the terminal; an older tmux takes "on".
func consoleGraphics() func() (bool, string) {
	tmux := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), graphicsTimeout)
		defer cancel()
		return exec.CommandContext(ctx, "tmux", args...).Output()
	}
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		if _, err := tmux("set-option", "-p", "-t", pane, "allow-passthrough", "all"); err != nil {
			_, _ = tmux("set-option", "-p", "-t", pane, "allow-passthrough", "on") // older still: marks stay
		}
	}
	return func() (bool, string) { return logos.Viewers(os.Getenv, tmux) }
}
