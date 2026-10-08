package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/blox-eng/matchblox/internal/remote"
)

// gateMain is the forced command of the matchblox key on a host
// (design/0005-remote-mode.md §3.3). The key starts the service stream and
// nothing else; anything else exits 126.
func gateMain() error {
	orig := os.Getenv("SSH_ORIGINAL_COMMAND")
	if err := gateDecision(orig); err != nil {
		fmt.Fprintf(os.Stderr, "matchblox gate: refused %q: %v\n", orig, err)
		os.Exit(126)
	}
	// ssh's own PATH (a non-login shell, or sshd's default on macOS) may
	// not have tmux: the service it starts lists the panes with it.
	_ = os.Setenv("PATH", os.ExpandEnv(remote.HostPath)+string(os.PathListSeparator)+os.Getenv("PATH"))
	_ = os.Unsetenv("SSH_ORIGINAL_COMMAND")
	return run([]string{"serve", "--stdio"})
}

// gateDecision lets the stream through: exactly the console's command.
func gateDecision(orig string) error {
	if orig == remote.ServeCommand {
		return nil
	}
	if orig == "" {
		return errors.New("no shell for this key")
	}
	return errors.New("this key starts only the matchblox stream")
}
