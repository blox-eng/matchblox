package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/blox-eng/matchblox/internal/remote"
)

// gateMain is the forced command of the matchblox key on a host
// (design/0005-remote-mode.md §3.3). The key starts the service stream or
// the console and nothing else; anything else exits 126.
func gateMain(args []string) error {
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "matchblox gate: refused: this key takes no arguments\n")
		os.Exit(126)
	}
	orig := os.Getenv("SSH_ORIGINAL_COMMAND")
	argv, err := gateDecision(orig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "matchblox gate: refused %q: %v\n", orig, err)
		os.Exit(126)
	}
	// ssh's own PATH (a non-login shell, or sshd's default on macOS) may
	// not have tmux: the service it starts lists the panes with it.
	_ = os.Setenv("PATH", os.ExpandEnv(remote.HostPath)+string(os.PathListSeparator)+os.Getenv("PATH"))
	_ = os.Unsetenv("SSH_ORIGINAL_COMMAND")
	// The console re-runs itself in the home session with these arguments:
	// without "gate" it is the builder's own `matchblox`.
	os.Args = os.Args[:1]
	return run(argv)
}

// gateDecision maps exactly two requested commands to what runs: the
// service stream and the console. A tmux client is never one of them: it can
// open a shell, and jumps and door commands use the builder's own login. The
// console command attaches the shared home session, where goHome already
// decides tmux or in place.
func gateDecision(orig string) (argv []string, err error) {
	switch orig {
	case remote.ServeCommand:
		return []string{"serve", "--stdio"}, nil
	case remote.ConsoleCommand:
		return []string{}, nil
	case "":
		return nil, errors.New("no shell for this key")
	}
	return nil, errors.New("this key starts only the matchblox stream and console")
}
