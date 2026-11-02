// Package demo runs the real console and the real service in one process, over
// a copy of a fixture machine, for a person with no host of their own.
package demo

import (
	"archive/tar"
	"bytes"
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
	"github.com/blox-eng/matchblox/internal/stoker"
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
	// Tree is the fixture machine, a folder or a .tar file of one. It is
	// copied, so it may be read-only.
	Tree string
	// In and Out are the terminal's streams. Out must be read or closed for
	// Stop to end: a write to a pipe no one reads blocks for good. A reader of
	// In that cannot be interrupted ends when In does.
	In         io.Reader
	Out        io.Writer
	Cols, Rows int

	console func(*app.Options) // a test's change to the console's options
}

// onlcr does to the output what a terminal driver would, as there is none here:
// the renderer counts on a line feed going back to the first column.
type onlcr struct{ w io.Writer }

func (o onlcr) Write(p []byte) (int, error) {
	if _, err := o.w.Write(bytes.ReplaceAll(p, []byte("\n"), []byte("\r\n"))); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Demo is one run of the console and the service.
type Demo struct {
	prog   *tea.Program
	cancel context.CancelFunc
	done   chan struct{}
	sized  chan struct{} // closed at the program's first size
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

	opts := app.Options{
		Conn: conn, Binary: binary, Demo: Refusal, OutsideTmux: true,
		Pair:     &app.PairHost{}, // the Machine tab offers a device, for the line to answer
		StateDir: stateDir(root), Now: clockFrom(started),
	}
	if o.console != nil {
		o.console(&opts)
	}
	console := app.New(opts)
	d := &Demo{
		script: sc,
		cancel: cancel,
		done:   make(chan struct{}),
		sized:  make(chan struct{}),
	}
	var sizedOnce sync.Once
	sawSize := func(_ tea.Model, msg tea.Msg) tea.Msg {
		if _, ok := msg.(tea.WindowSizeMsg); ok {
			sizedOnce.Do(func() { close(d.sized) })
		}
		return msg
	}
	d.prog = tea.NewProgram(console, tea.WithContext(ctx), tea.WithInput(o.In), tea.WithOutput(onlcr{o.Out}),
		tea.WithWindowSize(o.Cols, o.Rows), tea.WithFPS(15), tea.WithoutSignalHandler(),
		tea.WithEnvironment([]string{"TERM=xterm-256color", "COLORTERM=truecolor"}),
		tea.WithFilter(sawSize))
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

// Resize tells the console the terminal's new size. It waits for the program's
// own first size, which it sends from a goroutine at its start: one that came
// after this one would put back the old size for good.
func (d *Demo) Resize(cols, rows int) {
	select {
	case <-d.sized:
	case <-d.done:
		return
	}
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
	if fi, statErr := os.Stat(tree); statErr == nil && fi.Mode().IsRegular() && strings.HasSuffix(tree, ".tar") {
		err = untar(root, tree)
	} else {
		err = os.CopyFS(root, os.DirFS(tree))
	}
	if err != nil {
		_ = os.RemoveAll(root)
		return "", fmt.Errorf("demo: copying the machine %s to %s: %w", tree, root, err)
	}
	return root, nil
}

// untar unpacks a tar of the machine into root. A symlink may point anywhere,
// as the fixture reads its targets and never follows them, but no entry may
// land outside root, nor be written through a link.
func untar(root, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dst, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer dst.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	tr := tar.NewReader(f)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return endsWithTrailer(f, info.Size())
		}
		if err != nil {
			return fmt.Errorf("reading the tar: %w", err)
		}
		name := strings.TrimSuffix(h.Name, "/")
		if !filepath.IsLocal(name) {
			return fmt.Errorf("the tar entry %q is outside the machine", h.Name)
		}
		if name = filepath.Clean(name); name == "." {
			continue // tar's own "./"
		}
		if err := unpack(dst, tr, h, name, info.Size()); err != nil {
			return fmt.Errorf("the tar entry %q: %w", h.Name, err)
		}
	}
}

// endsWithTrailer tells a whole tar from one cut at an entry's end: only the
// whole one has the zero blocks of the trailer last.
func endsWithTrailer(r io.ReaderAt, size int64) error {
	const trailer = 1024
	if size < trailer {
		return errors.New("the tar is cut short: it has no end")
	}
	last := make([]byte, trailer)
	if _, err := r.ReadAt(last, size-trailer); err != nil {
		return fmt.Errorf("reading the tar's end: %w", err)
	}
	if len(bytes.Trim(last, "\x00")) != 0 {
		return errors.New("the tar is cut short: it has no end")
	}
	return nil
}

// unpack writes one entry; archiveSize is the tar's length, which bounds a
// file's body, so a header that claims more is refused before it writes.
func unpack(dst *os.Root, tr *tar.Reader, h *tar.Header, name string, archiveSize int64) error {
	if dir := filepath.Dir(name); dir != "." {
		if err := dst.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	switch h.Typeflag {
	case tar.TypeDir:
		return dst.MkdirAll(name, 0o755)
	case tar.TypeSymlink:
		return dst.Symlink(h.Linkname, name)
	case tar.TypeReg:
		if h.Size > archiveSize {
			return fmt.Errorf("claims %d bytes in a tar of %d", h.Size, archiveSize)
		}
		f, err := dst.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, os.FileMode(h.Mode).Perm()|0o600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, tr); err != nil {
			_ = f.Close()
			return err
		}
		return f.Close()
	}
	return fmt.Errorf("a tar entry of type %q is not part of a machine", h.Typeflag)
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
	s.Ships = demoShips
	seed := filepath.Join(stateDir(root), "history.csv")
	if err := os.MkdirAll(stateDir(root), 0o755); err != nil {
		return nil, nil, fmt.Errorf("demo: %w", err)
	}
	if err := writeHistory(seed); err != nil {
		return nil, nil, fmt.Errorf("demo: the machine's history: %w", err)
	}
	s.HistorySources = []string{seed}
	if s.Stoker, err = stoker.Open(stateDir(root)); err != nil {
		return nil, nil, fmt.Errorf("demo: the stoker at rest: %w", err)
	}
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
