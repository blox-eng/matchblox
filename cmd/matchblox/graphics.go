package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/logos"
)

// graphicsTimeout bounds each tmux call the console makes to learn who
// shows it: a wedged tmux costs the logos, not a pile of processes.
const graphicsTimeout = time.Second

// consoleGraphics tells the console whether every terminal that shows it
// draws the providers' logos (#183), and gives back the pane's own
// allow-passthrough when the console ends.
func consoleGraphics() (ask func() (bool, string), restore func()) {
	return consoleGraphicsWith(os.Getenv, func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), graphicsTimeout)
		defer cancel()
		return exec.CommandContext(ctx, "tmux", args...).Output()
	})
}

// consoleGraphicsWith lets images pass through the console's own pane,
// while it is hidden too ("all"), so logos sent then reach the terminal;
// an older tmux takes "on". A tmux that takes neither passes no image:
// the marks stay. restore gives the pane back its own setting, unset or
// as it was, since passthrough left open lets anything later printed in
// that pane write to the terminal. env is os.Getenv; tmux runs tmux.
func consoleGraphicsWith(env func(string) string, tmux func(args ...string) ([]byte, error)) (ask func() (bool, string), restore func()) {
	ask = func() (bool, string) { return logos.Viewers(env, tmux) }
	pane := env("TMUX_PANE")
	if env("TMUX") == "" || pane == "" {
		return ask, func() {}
	}
	out, _ := tmux("show-options", "-p", "-v", "-t", pane, "allow-passthrough")
	before := strings.TrimSpace(string(out))
	passes := false
	for _, v := range []string{"all", "on"} {
		if _, err := tmux("set-option", "-p", "-t", pane, "allow-passthrough", v); err == nil {
			passes = true
			break
		}
	}
	if !passes {
		return func() (bool, string) { return false, "" }, func() {}
	}
	return ask, func() {
		if before == "" {
			_, _ = tmux("set-option", "-p", "-u", "-t", pane, "allow-passthrough")
			return
		}
		_, _ = tmux("set-option", "-p", "-t", pane, "allow-passthrough", before)
	}
}
