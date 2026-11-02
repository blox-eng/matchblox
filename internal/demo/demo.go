// Package demo runs the real console and the real service in one process, over
// a copy of a fixture machine, for a person with no host of their own.
package demo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/app"
	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/procfs"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/service"
	"github.com/blox-eng/matchblox/internal/transport"
)

// Refusal answers what a phone cannot do.
const Refusal = "This is a demo. Install matchblox to do that."

var errRefused = errors.New(Refusal) //nolint:staticcheck // the line a person reads, as it stands

// binary is the version on both sides of the pipe, so the console never asks
// to replace its service.
const binary = "demo"

const copyPrefix = "matchblox-demo-"

// ErrRunning is a Start while a demo runs: there is one console in a process.
var ErrRunning = errors.New("demo: already running")

// Options are the fixture machine and the terminal that shows the console.
type Options struct {
	// Tree is the fixture machine. It is copied, so it may be read-only.
	Tree string
	// In and Out are the terminal's streams. Out must be read or closed for
	// Stop to end: a write to a pipe no one reads blocks for good. A reader of
	// In that cannot be interrupted ends when In does.
	In         io.Reader
	Out        io.Writer
	Cols, Rows int
}

// Demo is one run of the console and the service.
type Demo struct {
	prog   *tea.Program
	cancel context.CancelFunc
	done   chan struct{}
	err    error
	script *script
}

var (
	mu      sync.Mutex
	running *Demo
)

// Start runs the demo until the console quits or Stop is called.
func Start(o Options) (*Demo, error) {
	mu.Lock()
	defer mu.Unlock()
	if running != nil {
		return nil, ErrRunning
	}
	sweepCopies()
	root, err := copyTree(o.Tree)
	if err != nil {
		return nil, err
	}
	if err := nameHost(root); err != nil {
		_ = os.RemoveAll(root)
		return nil, fmt.Errorf("demo: naming the host: %w", err)
	}

	started := time.Now()
	sc, s, err := newService(root, started)
	if err != nil {
		_ = os.RemoveAll(root)
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	a, b := net.Pipe()
	var serving sync.WaitGroup
	serving.Go(func() { s.Run(ctx) })
	serving.Go(func() { _ = s.Handle(ctx, transport.NewConn(b)) }) // ends with the console
	conn := transport.NewConn(a)

	console := app.New(app.Options{
		Conn: conn, Binary: binary, Run: refuse, OutsideTmux: true,
		StateDir: stateDir(root), Now: clockFrom(started),
	})
	d := &Demo{
		script: sc,
		cancel: cancel,
		done:   make(chan struct{}),
		prog: tea.NewProgram(console, tea.WithContext(ctx), tea.WithInput(o.In), tea.WithOutput(o.Out),
			tea.WithWindowSize(o.Cols, o.Rows), tea.WithFPS(15), tea.WithoutSignalHandler(),
			tea.WithEnvironment([]string{"TERM=xterm-256color", "COLORTERM=truecolor"})),
	}
	running = d
	go func() {
		_, err := d.prog.Run()
		stopped := ctx.Err() != nil
		cancel()
		_ = conn.Close()
		serving.Wait()
		sc.stop()
		if stopped && errors.Is(err, tea.ErrProgramKilled) {
			err = nil
		}
		d.err = errors.Join(err, os.RemoveAll(root))
		mu.Lock()
		running = nil
		mu.Unlock()
		close(d.done)
	}()
	return d, nil
}

// Resize tells the console the terminal's new size.
func (d *Demo) Resize(cols, rows int) {
	d.prog.Send(tea.WindowSizeMsg{Width: cols, Height: rows})
}

// Stop ends the console and the service and removes the temp copy. It returns
// when all of that is done.
func (d *Demo) Stop() {
	d.cancel()
	<-d.done
}

// Wait returns when the demo has ended and the temp copy is gone.
func (d *Demo) Wait() error {
	<-d.done
	return d.err
}

// Done reports that the person has finished what the demo asks of them.
func (d *Demo) Done() bool { return d.script.done.Load() }

func refuse([]string) error { return errRefused }

// sweepCopies removes what a killed run left: only the end of a run removes
// its own copy, and a killed process never gets there. One app owns its temp
// folder, and one Start in it runs at a time. A copy that will not go stays in
// the way of nothing, so it does not stop this run.
func sweepCopies() {
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), copyPrefix) {
			_ = os.RemoveAll(filepath.Join(os.TempDir(), e.Name()))
		}
	}
}

func stateDir(root string) string { return filepath.Join(root, "demo-state") }

func copyTree(tree string) (string, error) {
	root, err := os.MkdirTemp("", copyPrefix+"*")
	if err != nil {
		return "", fmt.Errorf("demo: a temp copy of the machine in %s: %w", os.TempDir(), err)
	}
	if err := os.CopyFS(root, os.DirFS(tree)); err != nil {
		_ = os.RemoveAll(root)
		return "", fmt.Errorf("demo: copying the machine %s to %s: %w", tree, root, err)
	}
	return root, nil
}

func newService(root string, started time.Time) (*script, *service.Service, error) {
	cfg := config.Default()
	cfg.Agents = []string{"claude", "codex", "opencode"} // the fixture machine runs all three
	now := clockFrom(started)
	smp := Sampler(cfg, root)
	smp.FS, smp.Now = movingFS{smp.FS.(procfs.FS), started}, now
	w, err := NewWorld(root, now, func(pid int) { _ = os.RemoveAll(filepath.Join(root, "proc", strconv.Itoa(pid))) })
	if err != nil {
		return nil, nil, fmt.Errorf("demo: the world of the machine: %w", err)
	}
	w.Uptime = smp.FS.Uptime
	sc := &script{world: w}
	w.Picked = sc.answered
	s := service.New(cfg, smp, w.Git)
	s.Binary, s.Host, s.Now = binary, Host(root), now
	s.Home, s.StatePath = smp.Home, filepath.Join(stateDir(root), "state.json")
	s.Capture, s.Activity = smp.Capture, nil // the fixture's panes, never a tmux on this machine
	w.Wire(s)
	return sc, s, nil
}

// Sampler reads a fixture tree in place of the live machine.
func Sampler(cfg config.Config, root string) *sample.Sampler {
	return &sample.Sampler{
		Cfg:       cfg,
		Agents:    cfg.Agents,
		Rules:     sample.RulesOf(cfg),
		FS:        procfs.FS{Root: filepath.Join(root, "proc"), PageSize: procfs.FixturePage},
		Sys:       procfs.Sys{Root: filepath.Join(root, "sys")},
		Home:      filepath.Join(root, "home"),
		HomeAs:    "/home/user", // the HOME of every fixture process
		Tmux:      func() ([]byte, error) { return os.ReadFile(filepath.Join(root, "tmux-panes.txt")) },
		Capture:   sample.CaptureDir(filepath.Join(root, "panes")),
		LimitsDir: filepath.Join(root, "state"),
		Disks:     sample.DiskFile(filepath.Join(root, "disks.txt")),
	}
}

// Host is the host name a fixture tree carries, so a demo or a test never
// shows the name of the machine it runs on.
func Host(root string) string {
	b, err := os.ReadFile(filepath.Join(root, "proc", "sys", "kernel", "hostname"))
	if h := strings.TrimSpace(string(b)); err == nil && h != "" {
		return h
	}
	return "fixtures"
}
