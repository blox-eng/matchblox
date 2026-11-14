//go:build linux || darwin

package guard

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

// tickEvery is how often the filter hears that time passes.
const tickEvery = 25 * time.Millisecond

// killWait is how long an agent has to end after a hangup or a stop
// passed on to it, before the guard takes its terminal away and ends it.
var killWait = 3 * time.Second

// drainWait is how long the guard reads on after the agent ends, for the
// last of its output.
const drainWait = 500 * time.Millisecond

// openTerminal and copyOutput are Run's two steps a test makes fail.
var (
	openTerminal = openPTY
	copyOutput   = func(f *Filter, master io.Reader) { _, _ = io.Copy(f, master) } // ends when the agent's side closes
)

// StoppedError is a guard that broke while the agent ran: the agent ended
// with it, as with a closed terminal.
type StoppedError struct{ Reason string }

func (e *StoppedError) Error() string { return "the redraw guard stopped: " + e.Reason }

// noTerminalError is a guard that got no terminal for the agent.
type noTerminalError struct{ err error }

func (e *noTerminalError) Error() string { return e.err.Error() }

// Main runs argv behind the guard for the session named session, and gives
// the exit status for this process. With no terminal to give the agent, it
// runs argv on the pane itself and says so there.
func Main(argv []string, session string, in, out *os.File) int {
	status, err := Run(argv, in, out)
	var noTerm *noTerminalError
	var stopped *StoppedError
	switch {
	case errors.As(err, &noTerm):
		_, _ = fmt.Fprintf(out, "Cursor runs without the redraw guard: %v\r\n", err)
		cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // the caller's own command, never a shell string
		cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, out
		return exitStatus(cmd.Run(), out)
	case errors.As(err, &stopped):
		if session == "" {
			session = "<session>"
		}
		_, _ = fmt.Fprintf(out, "\r\nmatchblox's redraw guard stopped: %s. Start again: matchblox switch %s --agent cursor-agent\r\n", stopped.Reason, session)
		return 1
	}
	return exitStatus(err, out) + status
}

// exitStatus is the status of a command that ended with err; an err that
// is not its exit is said on out.
func exitStatus(err error, out io.Writer) int {
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit):
		if ws, ok := exit.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return exit.ExitCode()
	}
	_, _ = fmt.Fprintf(out, "matchblox guard: %v\r\n", err)
	return 1
}

// Run runs argv in a terminal of its own behind a Filter. Keys typed at in
// reach it as typed; its output comes to out, unchanged but for a replay;
// its terminal has the size of in, also after a change. It gives argv's
// exit status.
func Run(argv []string, in, out *os.File) (int, error) {
	master, slave, err := openTerminal()
	if err != nil {
		return 0, &noTerminalError{err}
	}
	defer func() { _ = master.Close() }()
	f := NewFilter(out, time.Now)
	f.Arm() // the agent's first screen: a resumed chat is a replay too
	cols := uint16(0)
	resize := func() {
		ws, err := unix.IoctlGetWinsize(int(in.Fd()), unix.TIOCGWINSZ) //nolint:gosec // a file descriptor fits
		if err != nil {
			return
		}
		if cols != 0 && ws.Col != cols {
			f.Resized() // before the agent hears of it
		}
		cols = ws.Col
		_ = unix.IoctlSetWinsize(int(master.Fd()), unix.TIOCSWINSZ, ws) //nolint:gosec // a file descriptor fits
	}
	resize()

	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // the caller's own command, never a shell string
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	err = cmd.Start()
	_ = slave.Close()
	if err != nil {
		return 0, err
	}
	if raw, err := term.MakeRaw(in.Fd()); err == nil {
		defer func() { _ = term.Restore(in.Fd(), raw) }()
	}

	sigs := make(chan os.Signal, 8)
	signal.Notify(sigs, syscall.SIGWINCH, syscall.SIGHUP, syscall.SIGTERM)
	defer signal.Stop(sigs)
	drained, ended := make(chan struct{}), make(chan struct{})
	broke, waited := make(chan string, 1), make(chan error, 1)
	go func() { _, _ = io.Copy(master, in) }()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				broke <- fmt.Sprint(r)
				return
			}
			close(drained)
		}()
		copyOutput(f, master)
	}()
	go func() {
		tick := time.NewTicker(tickEvery)
		defer tick.Stop()
		for {
			select {
			case <-ended:
				return
			case <-tick.C:
				_ = f.Tick()
			case sig := <-sigs:
				if sig == syscall.SIGWINCH {
					resize()
				} else {
					_ = cmd.Process.Signal(sig)
					time.AfterFunc(killWait, func() {
						_ = master.Close() // its terminal goes, as its pane did
						_ = cmd.Process.Kill()
					})
				}
			}
		}
	}()

	go func() { waited <- cmd.Wait() }()
	select {
	case err = <-waited:
	case reason := <-broke:
		close(ended)
		_ = master.Close() // the agent's terminal closes: it ends, as with a closed pane
		_ = cmd.Process.Signal(syscall.SIGHUP)
		kill := time.AfterFunc(drainWait, func() { _ = cmd.Process.Kill() })
		<-waited //nolint:errcheck // how the agent ended does not matter: the guard broke first
		kill.Stop()
		return 0, &StoppedError{reason}
	}
	select {
	case <-drained:
	case <-time.After(drainWait):
	}
	close(ended)
	_ = f.Flush()
	var exit *exec.ExitError
	if err == nil || errors.As(err, &exit) {
		return exitStatus(err, out), nil
	}
	return 0, err
}
