package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/remote"
)

// gateMain is the forced command of the matchblox key on a host
// (design/0005-remote-mode.md §3.3). It runs what the console asked for, as
// argv, when it is one of the console's own commands; anything else exits
// 126.
func gateMain() error {
	orig := os.Getenv("SSH_ORIGINAL_COMMAND")
	kind, argv, err := gateDecision(orig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "matchblox gate: refused %q: %v\n", orig, err)
		os.Exit(126)
	}
	if kind == "serve" {
		return run([]string{"serve", "--stdio"})
	}
	bin, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	return execve(bin, argv, gateEnv())
}

// gateEnv is the environment of a gated command: a login's own, without
// what ssh says about the command it was asked for.
func gateEnv() []string {
	return slices.DeleteFunc(os.Environ(), func(kv string) bool {
		return strings.HasPrefix(kv, "SSH_ORIGINAL_COMMAND=") || strings.HasPrefix(kv, "TMUX=")
	})
}

// gateDecision reads the asked command: the service stream ("serve"), a
// chain of tmux moves that ends in an attach, or a door's command ("exec").
func gateDecision(orig string) (kind string, argv []string, err error) {
	if orig == remote.ServeCommand {
		return "serve", nil, nil
	}
	if orig == "" {
		return "", nil, errors.New("no shell for this key")
	}
	argv, err = remote.Split(orig)
	if err != nil {
		return "", nil, err
	}
	if argv[0] == "tmux" {
		if !tmuxChain(argv[1:]) {
			return "", nil, errors.New("only the console's tmux moves")
		}
		return "exec", argv, nil
	}
	if doors.TermAllowed(argv) {
		return "exec", argv, nil
	}
	return "", nil, errors.New("not a command of the console")
}

// tmuxChain is moves separated by ";", each one of the console's tmux moves,
// and a last attach.
func tmuxChain(args []string) bool {
	var steps [][]string
	cur := []string{"tmux"}
	for _, a := range args {
		if a == ";" {
			steps, cur = append(steps, cur), []string{"tmux"}
			continue
		}
		cur = append(cur, a)
	}
	steps = append(steps, cur)
	last := steps[len(steps)-1]
	attach := slices.Equal(last, []string{"tmux", "attach-session"}) || (remote.NavAllowed(last) && last[1] == "attach-session")
	if !attach {
		return false
	}
	for _, s := range steps[:len(steps)-1] {
		if !remote.NavAllowed(s) || s[1] == "attach-session" || s[1] == "switch-client" {
			return false
		}
	}
	return true
}
