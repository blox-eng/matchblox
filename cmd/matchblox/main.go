// Command matchblox is a terminal console for a machine that runs many AI
// coding agents: sessions by tmux pane, machine load, git, and what to do.
//
//	matchblox                 open the console (starts the service if needed)
//	matchblox serve           run the service; --stdio speaks on stdin/stdout
//	matchblox status          print what the service knows as JSON (for agents)
//	matchblox status --text   the same, as a short summary
//	matchblox version         print the version
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"slices"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/actions"
	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/app"
	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/history"
	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/procfs"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/service"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/transport"
)

var version = "dev"

func main() {
	// MATCHBLOX_CPUPROFILE=file profiles this process, to keep its own cost
	// honest.
	if path := os.Getenv("MATCHBLOX_CPUPROFILE"); path != "" {
		if f, err := os.Create(path); err == nil { //nolint:gosec // the person names their own profile file
			_ = pprof.StartCPUProfile(f)
			defer pprof.StopCPUProfile()
		}
	}
	if err := run(os.Args[1:]); err != nil {
		pprof.StopCPUProfile()
		fmt.Fprintln(os.Stderr, "matchblox:", err)
		holdOnError(err, os.Getenv, os.Stdin, os.Stderr)
		os.Exit(1)
	}
}

// holdOnError keeps an error on the screen in the home session: the console
// is its only command, so the pane would close before the person reads it.
func holdOnError(err error, env func(string) string, in io.Reader, out io.Writer) {
	if err == nil || env(panes.HomeEnv) != "1" {
		return
	}
	_, _ = fmt.Fprintln(out, "press Enter to close")
	_, _ = bufio.NewReader(in).ReadString('\n')
}

// configArg is the --config the person gave, passed on to a service the
// console starts.
var configArg string

func run(args []string) error {
	if len(args) > 0 && args[0] == "hook" {
		return hook(args[1:], os.Stdin, os.Stdout, transport.SocketPath(), spoolPath(), progressPrompt(args[1:]))
	}
	cmd := "console"
	if len(args) > 0 {
		switch args[0] {
		case "serve", "status", "version":
			cmd, args = args[0], args[1:]
		}
	}
	fl := flag.NewFlagSet("matchblox", flag.ContinueOnError)
	cfgPath := fl.String("config", config.Path(), "machine goals (TOML); a missing file means defaults")
	interval := fl.Duration("interval", 0, "sampling interval (default from config, else 2s)")
	root := fl.String("fixtures", "", "read proc/, sys/, home/ and tmux-panes.txt from this directory instead of the live machine")
	text := fl.Bool("text", false, "status: print a summary instead of JSON")
	stdio := fl.Bool("stdio", false, "serve: speak on stdin and stdout, joined to the local service")
	noMotion := fl.Bool("no-motion", os.Getenv("NO_MOTION") == "1", "every motion is an instant change (also NO_MOTION=1)")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if cmd == "version" {
		fmt.Println(version)
		return nil
	}
	fl.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			configArg = f.Value.String()
		}
	})
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return fmt.Errorf("%s: %w", *cfgPath, err)
	}
	if *interval > 0 {
		cfg.Interval.Duration = *interval
	}

	if cmd == "console" && *root == "" {
		// A phone that connects lands in the console's own tmux session, and
		// every pane has a way back to it. Fixtures (demos, the replay) run
		// in place. Before the nice below: a tmux server started here must
		// not run every agent at nice 19.
		if err := goHome(); err != nil {
			return err
		}
	}

	// matchblox must not add load: lowest CPU priority for us and every
	// command we start.
	lowestPriority()

	path := transport.SocketPath()
	switch cmd {
	case "serve":
		if *stdio {
			return serveStdio(path, os.Stdin, os.Stdout)
		}
		return serve(path, newService(cfg, *root))
	case "status":
		return status(os.Stdout, path, cfg, *root, *text)
	}
	return console(path, cfg, *root, *noMotion)
}

// goHome replaces this process with `tmux new-session -A -s matchblox
// matchblox …` outside tmux. It returns only when the console runs in place.
func goHome() error {
	tmux, err := exec.LookPath("tmux")
	self := invokedPath(os.Args[0])
	if self == "" {
		return nil
	}
	argv := panes.HomeArgv(os.Getenv("TMUX") != "", err == nil, append([]string{self}, os.Args[1:]...))
	if argv == nil {
		return nil
	}
	return execve(tmux, argv, os.Environ())
}

// hookTimeout bounds the whole delivery, so `matchblox hook` stops in
// less than 50 ms with the service up, down or stuck.
const hookTimeout = 30 * time.Millisecond

// hook delivers one agent hook event to the service, or to the spool when
// no service answers. It returns nil on every path: a hook that fails must
// not disturb the agent.
func hook(args []string, stdin io.Reader, out io.Writer, sock, spool string, progress bool) error {
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	ev, err := hooks.Parse(stdin, name, os.Getenv)
	if err != nil {
		return nil
	}
	if transport.Notify(sock, hookTimeout, proto.KindHook, ev) != nil {
		_ = hooks.Append(spool, ev)
	}
	if progress && ev.Name == "SessionStart" && ev.Pane != "" { // a session in tmux: what the console watches
		// Claude Code adds what a SessionStart hook prints to the session's
		// context; the console reads the bar back from the transcript.
		_, _ = fmt.Fprintln(out, hooks.ProgressPrompt)
	}
	return nil
}

// progressPrompt reads the config only for SessionStart, the one event that
// prints: the other hooks stay as fast as they were.
func progressPrompt(args []string) bool {
	if len(args) == 0 || args[0] != "SessionStart" {
		return false
	}
	cfg, err := config.Load(config.Path())
	return err == nil && cfg.Sessions.ProgressPrompt
}

func spoolPath() string { return filepath.Join(filepath.Dir(state.Path()), "spool.jsonl") }

func console(path string, cfg config.Config, root string, noMotion bool) error {
	opt := app.Options{Binary: version, CompactAt: cfg.Sessions.CompactAt, NoMotion: noMotion,
		OutsideTmux: os.Getenv("TMUX") == ""}
	if root != "" {
		// Fixtures: a service in this process, for demos and tests.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		s := newService(cfg, root)
		a, b := net.Pipe()
		go s.Run(ctx)
		go s.Handle(ctx, transport.NewConn(b)) //nolint:errcheck // ends with the console
		opt.Conn = transport.NewConn(a)
	} else {
		c, err := ensureService(path)
		if err != nil {
			return err
		}
		opt.Conn = c
		opt.Redial = func() (transport.Conn, error) { return ensureService(path) }
		opt.Local = true
		opt.Owner = func() int { return transport.Owner(path) }
	}
	defer opt.Conn.Close()
	// The screen changes on a state or a key press, so 15 frames a second
	// is still instant to the eye and wakes the process 4x less than 60.
	_, err := tea.NewProgram(app.New(opt), tea.WithFPS(15)).Run()
	return err
}

// newService builds the service for this machine, or for a fixture tree
// (no git, nothing written).
func newService(cfg config.Config, root string) *service.Service {
	var git service.GitSource
	if root == "" {
		git = gitSource(cfg)
	}
	s := service.New(cfg, newSampler(cfg, root), git)
	s.Binary = version
	if root != "" {
		s.Host = fixtureHost(root)
		// The fixture's panes and pids exist on this machine too.
		s.Actions.Run = func([]string) error { return errors.New("fixtures: nothing runs on this machine") }
		return s
	}
	s.Actions.Run = func(argv []string) error { return actions.Exec(context.Background(), argv) }
	s.StatePath = state.Path()
	s.Spool = spoolPath()
	s.AlertHook = cfg.Hooks.Alert
	s.HistorySources = []string{cfg.History.Source}
	switch cfg.History.Log {
	case "":
		s.HistoryLog = history.DefaultLog()
	case "-":
	default:
		s.HistoryLog = cfg.History.Log
	}
	s.Exe = invokedPath(os.Args[0])
	return s
}

// fixtureHost is the host name a fixture tree carries, so a demo or a test
// never shows the name of the machine it runs on.
func fixtureHost(root string) string {
	b, err := os.ReadFile(filepath.Join(root, "proc", "sys", "kernel", "hostname"))
	if h := strings.TrimSpace(string(b)); err == nil && h != "" {
		return h
	}
	return "fixtures"
}

// invokedPath is the path matchblox was started by, links kept: Homebrew
// and nix upgrade by repointing a link, and the resolved file never changes.
func invokedPath(arg0 string) string {
	p := arg0
	if !strings.ContainsRune(arg0, filepath.Separator) {
		found, err := exec.LookPath(arg0)
		if err != nil {
			return ""
		}
		p = found
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return ""
	}
	return abs
}

// serve runs the service on the socket until a signal, or until the binary
// on disk is replaced (the next console starts the new one).
func serve(path string, s *service.Service) error {
	l, err := transport.Listen(path)
	if errors.Is(err, transport.ErrInUse) {
		fmt.Fprintf(os.Stderr, "matchblox: the service already runs on %s (pid %d)\n", path, transport.Owner(path))
		return nil
	}
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = s.Serve(ctx, l)
	if errors.Is(err, service.ErrReplaced) || errors.Is(err, service.ErrSocketGone) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// serveStdio joins stdin and stdout to the local service, starting it if
// needed. Remote mode runs it over SSH.
func serveStdio(path string, in io.Reader, out io.Writer) error {
	c, err := ensureService(path)
	if err != nil {
		return err
	}
	transport.Pipe(transport.Stdio(in, out), c)
	return nil
}

// ensureService connects to the local service, or starts it detached and
// connects within 2 s.
func ensureService(path string) (transport.Conn, error) {
	if c, err := transport.Dial(path, 200*time.Millisecond); err == nil {
		return c, nil
	}
	if err := spawnService(); err != nil {
		return nil, fmt.Errorf("start the service: %w", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := transport.Dial(path, 200*time.Millisecond); err == nil {
			return c, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, fmt.Errorf("the service did not start on %s; run `matchblox serve` to see why", path)
}

// spawnService starts `matchblox serve` in its own session, so it keeps
// running when the console and its terminal close.
var spawnService = func() error {
	c, closeLog, err := serviceCmd()
	if err != nil {
		return err
	}
	defer closeLog()
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}

// serviceCmd is the detached `matchblox serve`. It runs from / so that it
// never holds a worktree open, and writes to serve.log next to the state
// file.
func serviceCmd() (*exec.Cmd, func(), error) {
	// Start it by the path we were started by, so that it watches the link
	// an upgrade repoints.
	exe := invokedPath(os.Args[0])
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return nil, nil, err
		}
	}
	args := []string{"serve"}
	if configArg != "" {
		args = append(args, "--config", configArg)
	}
	c := exec.Command(exe, args...) //nolint:gosec // our own binary
	c.Dir = "/"
	// The service outlives this console: it watches the default tmux server,
	// not the one this console happens to run in.
	c.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool {
		return strings.HasPrefix(kv, "TMUX=") || strings.HasPrefix(kv, "TMUX_PANE=")
	})
	closeLog := func() {}
	logPath := filepath.Join(filepath.Dir(state.Path()), "serve.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err == nil {
		if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil { //nolint:gosec // our own state dir
			c.Stdout, c.Stderr = f, f
			// The child holds its own descriptor; a failed close of ours
			// loses nothing it writes, but say so.
			closeLog = func() {
				if err := f.Close(); err != nil {
					fmt.Fprintln(os.Stderr, "matchblox: serve.log:", err)
				}
			}
		}
	}
	detach(c)
	return c, closeLog, nil
}

// staleAfter is the age at which status treats the state file as left by
// a service that no longer runs.
const staleAfter = 15 * time.Second

func gitSource(cfg config.Config) service.GitSource {
	scanner := &gitscan.Scanner{Git: gitscan.Git, Merged: gitscan.GHMerged, RecheckIdle: 2 * time.Hour}
	return func(sessionCwds []string) gitscan.Report {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		in := gitscan.Input{
			SessionCwds: sessionCwds,
			ProcessCwds: actions.ProcessCwds(),
			Extra:       cfg.Git.Repos,
			Main:        cfg.Git.Main,
			Remote:      cfg.Git.Remote,
		}
		return scanner.Scan(ctx, in)
	}
}
func newSampler(cfg config.Config, root string) *sample.Sampler {
	smp := &sample.Sampler{
		Cfg:    cfg,
		Agents: cfg.Agents,
		Rules: sample.Rules{
			CompactAt:   cfg.Sessions.CompactAt,
			ClearIdle:   cfg.Sessions.ClearIdle.Duration,
			ClearMinPct: cfg.Sessions.ClearMinPct,
			ClearStale:  cfg.Sessions.ClearStale.Duration,
			Windows:     cfg.Sessions.Windows,
		},
	}
	if root != "" {
		smp.FS = procfs.FS{Root: filepath.Join(root, "proc")}
		smp.Sys = procfs.Sys{Root: filepath.Join(root, "sys")}
		smp.Home = filepath.Join(root, "home")
		smp.Tmux = func() ([]byte, error) { return os.ReadFile(filepath.Join(root, "tmux-panes.txt")) }
		return smp
	}
	smp.FS, smp.Sys = actions.NewHost(), procfs.Sys{Root: "/sys"}
	smp.Home, _ = os.UserHomeDir()
	smp.GPU, smp.Docker = sample.NvidiaSMI, sample.DockerPS
	smp.LatencyTarget = cfg.LatencyTarget
	smp.ProcEvery = 5 * cfg.Interval.Duration
	return smp
}

// status prints the service's state. Without a service it uses a fresh
// state file, and else samples on its own: two samples a second apart so
// rates are real, and one git scan.
func status(w io.Writer, path string, cfg config.Config, root string, text bool) error {
	doc, ok := state.Doc{}, false
	maxAge := max(staleAfter, 3*cfg.Interval.Duration)
	if root == "" {
		doc, ok = fromService(path, maxAge)
	}
	if !ok && root == "" {
		if d, err := state.Read(state.Path()); err == nil && time.Since(d.At) <= maxAge {
			doc, ok = d, true
		}
	}
	if !ok {
		smp := newSampler(cfg, root)
		smp.ProcEvery = 0
		smp.Sample()
		time.Sleep(time.Second)
		doc = state.Doc{Snapshot: smp.Sample()}
		if root == "" {
			var cwds []string
			for _, s := range doc.Sessions {
				cwds = append(cwds, s.Cwd)
			}
			r := gitSource(cfg)(cwds)
			doc.Git = &r
		}
		doc.Recommendations = advice.Build(doc.Snapshot, doc.Git)
	}
	if text {
		_, err := fmt.Fprint(w, app.Summary(doc))
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// fromService reads the first snapshot from a running service. A snapshot
// older than maxAge means its sample loop is stuck: not current.
func fromService(path string, maxAge time.Duration) (state.Doc, bool) {
	c, err := transport.Dial(path, 200*time.Millisecond)
	if err != nil {
		return state.Doc{}, false
	}
	defer c.Close()
	timer := time.AfterFunc(3*time.Second, func() { c.Close() })
	defer timer.Stop()
	if c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version, Binary: version}) != nil {
		return state.Doc{}, false
	}
	for {
		env, err := c.Recv()
		if err != nil {
			return state.Doc{}, false
		}
		switch env.Kind {
		case proto.KindHello:
			var h proto.Hello
			if json.Unmarshal(env.Body, &h) != nil || h.Version != proto.Version {
				return state.Doc{}, false
			}
		case proto.KindSnapshot:
			var st proto.State
			if json.Unmarshal(env.Body, &st) != nil || time.Since(st.At) > maxAge {
				return state.Doc{}, false
			}
			return st.Doc, true
		}
	}
}
