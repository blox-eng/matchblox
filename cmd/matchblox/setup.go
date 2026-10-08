package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/blox-eng/matchblox/internal/actions"
	"github.com/blox-eng/matchblox/internal/setup"
	"github.com/blox-eng/matchblox/internal/state"
)

// liveSetup is this machine, for the doors.
func liveSetup() *setup.Env {
	home, _ := os.UserHomeDir()
	release, _ := os.ReadFile("/etc/os-release")
	return &setup.Env{
		GOOS: runtime.GOOS, OSRelease: string(release), Home: home,
		Exe:      invokedPath(os.Args[0]),
		StateDir: filepath.Dir(state.Path()),
		Getenv:   os.Getenv, LookPath: exec.LookPath, Source: tmuxSource,
	}
}

// tmuxSource loads a config into the default tmux server, the one the
// service watches.
func tmuxSource(conf string) error {
	c := exec.Command("tmux", "source-file", conf) //nolint:gosec // our own config path
	c.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, "TMUX=") })
	var stderr bytes.Buffer
	c.Stderr = &stderr
	err := c.Run()
	switch msg := stderr.String(); {
	case err == nil:
		return nil
	case errors.Is(err, exec.ErrNotFound), strings.Contains(msg, "no server running"), strings.Contains(msg, "error connecting"):
		return setup.ErrNoServer
	case msg != "":
		return errors.New(strings.TrimSpace(msg))
	}
	return err
}

// firstRun writes the config with this machine's thresholds when there is
// none; a config that cannot be written still leaves the defaults.
func firstRun(path string, cores int, memTotal uint64) {
	if wrote, err := setup.WriteConfig(path, cores, memTotal); err != nil {
		fmt.Fprintln(os.Stderr, "matchblox: write", path+":", err)
	} else if wrote {
		fmt.Fprintln(os.Stderr, "matchblox: wrote", path, "with the goals of this machine")
	}
}

func machineSize() (int, uint64) {
	mem, _ := actions.NewHost().Meminfo()
	return runtime.NumCPU(), mem.Total
}

// setupDoors is `matchblox setup`: every door at once, done ones too. For
// each open door it shows the diff or the command, and opens it on y.
func setupDoors(e setup.Env, in io.Reader, out io.Writer, run func(argv []string) error) error {
	answers := bufio.NewReader(in)
	for _, d := range setup.Doors(e) {
		if d.Done {
			fmt.Fprintf(out, "✓ %s\n", d.Title)
			continue
		}
		fmt.Fprintf(out, "\n%s\n  %s\n", d.Title, d.Why)
		if d.Problem != "" {
			fmt.Fprintf(out, "  %s\n", d.Problem)
			continue
		}
		if d.Path != "" {
			fmt.Fprintf(out, "\n  %s\n", d.Path)
		}
		for _, l := range strings.Split(d.Preview, "\n") {
			fmt.Fprintf(out, "  %s\n", l)
		}
		fmt.Fprint(out, "\ny opens it, Enter skips: ")
		line, err := answers.ReadString('\n')
		if strings.TrimSpace(line) != "y" {
			fmt.Fprintln(out, "skipped")
			if err != nil {
				return nil // no more answers: the rest stay closed
			}
			continue
		}
		if d.Term != nil {
			if err := run(d.Term); err != nil {
				fmt.Fprintf(out, "failed: %v\n", err)
			}
			continue
		}
		backup, err := setup.Open(e, d.ID, d.Sum)
		switch {
		case err != nil:
			fmt.Fprintf(out, "not written: %v\n", err)
		case backup != "":
			fmt.Fprintf(out, "wrote %s; backup %s\n", d.Path, backup)
		default:
			fmt.Fprintf(out, "wrote %s\n", d.Path)
		}
	}
	return nil
}

// runInTerminal gives a door's command this terminal.
func runInTerminal(argv []string) error {
	c := exec.Command(argv[0], argv[1:]...) //nolint:gosec // a door's command, from setup.Doors
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}
