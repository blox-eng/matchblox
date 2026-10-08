package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/hosts"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/setup"
)

// connectEnv is what `matchblox connect` works with.
type connectEnv struct {
	Home, Hosts string
	// Run gives this terminal to argv: the ssh login is the builder's own.
	Run func(argv []string) error
	Out io.Writer
	// Verify reaches the host with the matchblox key, as the console does,
	// and returns the matchblox it runs.
	Verify func(remote.Target) (string, error)
}

// connectHost connects a host (design/0005-remote-mode.md §3): it makes the
// matchblox key once, then, with the builder's own ssh login, installs
// matchblox there when it is missing (always for an update) and lets the
// key start only the gate. It prints each command before it runs it.
func connectHost(e connectEnv, host string, update bool) error {
	if !remote.ValidHost(host) {
		return fmt.Errorf("%q is not a host name", host)
	}
	say := func(argv []string) { _, _ = fmt.Fprintln(e.Out, "$ "+remote.Line(argv)) }
	key := remote.KeyPath(e.Home)
	if _, err := os.Stat(key + ".pub"); err != nil {
		if err := os.MkdirAll(filepath.Dir(key), 0o700); err != nil {
			return err
		}
		argv := []string{"ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "matchblox", "-f", key}
		say(argv)
		if err := e.Run(argv); err != nil {
			return fmt.Errorf("make the matchblox key: %w", err)
		}
	}
	pub, err := os.ReadFile(key + ".pub") //nolint:gosec // our own key
	if err != nil {
		return err
	}
	script, err := remote.Bootstrap(strings.TrimSpace(string(pub)), update)
	if err != nil {
		return fmt.Errorf("%s.pub: %w", key, err)
	}
	argv := []string{"ssh", "-t", "-o", "ForwardAgent=no", "-o", "ClearAllForwardings=yes", "--", host, script}
	say(argv)
	if err := e.Run(argv); err != nil {
		return fmt.Errorf("ssh %s: %w", host, err)
	}
	bin, err := e.Verify(remote.Target{Host: host, Key: key})
	if err != nil {
		return fmt.Errorf("%s does not answer with the matchblox key: %w", host, err)
	}
	if e.Hosts != "" {
		if err := hosts.Add(e.Hosts, host); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(e.Out, "connected %s (matchblox %s). matchblox %s opens it.\n", host, bin, host)
	return err
}

// verifyHost says hello to the host's service with the matchblox key.
func verifyHost(t remote.Target) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := remote.Connect(ctx, t)
	if err != nil {
		return "", err
	}
	defer c.Close()
	if err := c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version, Binary: version}); err != nil {
		return "", err
	}
	for {
		env, err := c.Recv()
		if err != nil {
			return "", err
		}
		if env.Kind == proto.KindHello {
			var h proto.Hello
			if err := json.Unmarshal(env.Body, &h); err != nil {
				return "", err
			}
			if h.Version != proto.Version {
				return "", errors.New("it speaks another wire: matchblox connect " + t.Host + " --update")
			}
			return h.Binary, nil
		}
	}
}

// liveConnect is `matchblox connect` on this machine.
func liveConnect(args []string) error {
	update := false
	var host string
	for _, a := range args {
		switch {
		case a == "--update":
			update = true
		case host == "":
			host = a
		default:
			return errors.New("usage: matchblox connect <host> [--update]")
		}
	}
	if host == "" {
		return errors.New("usage: matchblox connect <host> [--update]")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	err = connectHost(connectEnv{Home: home, Hosts: hosts.Path(), Run: runInTerminal, Out: os.Stdout, Verify: verifyHost}, host, update)
	fi, _ := os.Stdin.Stat()
	return holdFailure(err, fi != nil && fi.Mode()&os.ModeCharDevice != 0, os.Stdin, os.Stderr)
}

// errShown: the error is on the screen already.
var errShown = errors.New("connect failed")

// holdFailure keeps a failed connect on a terminal until Enter: the console
// that ran it takes the screen back as soon as it ends.
func holdFailure(err error, tty bool, in io.Reader, out io.Writer) error {
	if err == nil || !tty {
		return err
	}
	_, _ = fmt.Fprintf(out, "\nmatchblox: %v\npress Enter to go back\n", err)
	_, _ = bufio.NewReader(in).ReadString('\n')
	return errShown
}

// authorize is `matchblox authorize <public key>`, run by connect on the
// host.
func authorize(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: matchblox authorize '<ssh-ed25519 public key>'")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	exe := invokedPath(os.Args[0])
	if exe == "" {
		return errors.New("cannot find this matchblox on the PATH")
	}
	backup, err := setup.Authorize(home, exe, args[0])
	if err != nil {
		return err
	}
	msg := "the matchblox key can start matchblox here, and nothing else"
	if backup != "" {
		msg += "; backup " + backup
	}
	fmt.Println(msg)
	return nil
}
