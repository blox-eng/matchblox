//go:build linux || darwin

package guard

import (
	"errors"
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

// drainWait is how long the guard reads on after the agent ends, for the
// last of its output.
const drainWait = 500 * time.Millisecond

// Run runs argv in a terminal of its own behind a Filter. Keys typed at in
// reach it as typed; its output comes to out, unchanged but for a replay;
// its terminal has the size of in, also after a change. It gives argv's
// exit status.
func Run(argv []string, in, out *os.File) (int, error) {
	master, slave, err := openPTY()
	if err != nil {
		return 0, err
	}
	defer func() { _ = master.Close() }()
	resize := func() {
		if ws, err := unix.IoctlGetWinsize(int(in.Fd()), unix.TIOCGWINSZ); err == nil { //nolint:gosec // a file descriptor fits
			_ = unix.IoctlSetWinsize(int(master.Fd()), unix.TIOCSWINSZ, ws) //nolint:gosec // a file descriptor fits
		}
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
	f := NewFilter(out, time.Now)
	drained, ended := make(chan struct{}), make(chan struct{})
	go func() { _, _ = io.Copy(master, in) }()
	go func() {
		_, _ = io.Copy(f, master) // ends when the agent's side closes
		close(drained)
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
				}
			}
		}
	}()

	err = cmd.Wait()
	select {
	case <-drained:
	case <-time.After(drainWait):
	}
	close(ended)
	_ = f.Flush()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0, nil
	case errors.As(err, &exit):
		if ws, ok := exit.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal()), nil
		}
		return exit.ExitCode(), nil
	}
	return 0, err
}
