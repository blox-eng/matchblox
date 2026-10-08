package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image/color"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/app"
	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/procfs"
	"github.com/blox-eng/matchblox/internal/replay"
	"github.com/blox-eng/matchblox/internal/service"
	"github.com/blox-eng/matchblox/internal/transport"
)

var update = flag.Bool("update", false, "TestReplay: write www/demo/frames.json, www/demo/replay.svg and the still frame in www/index.html")

const (
	replayCols, replayRows = 100, 30
	demoPID                = 4242
	stepTimeout            = 10 * time.Second
)

// demoBase is the demo machine's clock at the start of the replay.
var demoBase = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

// clock is the replay's clock: the recorder sets it, and both the service
// and the console read it.
type clock struct {
	mu     sync.Mutex
	t      time.Time
	follow *clock // set: this clock reads that one
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.follow != nil {
		return c.follow.Now()
	}
	return c.t
}

func (c *clock) Follow(o *clock)     { c.mu.Lock(); c.follow = o; c.mu.Unlock() }
func (c *clock) Set(t time.Time)     { c.mu.Lock(); c.t = t; c.mu.Unlock() }
func (c *clock) Add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

// demoHost is testdata/demo with a pulse: CPU counters move, the busy loop
// burns one core, and a killed process goes away. Nothing runs on the real
// machine.
type demoHost struct {
	procfs.FS

	mu          sync.Mutex
	sampled     time.Time // the sampler's clock at the start of this sample
	killed      map[int]bool
	calls       int
	busy, total uint64
	coreBusy    []uint64
}

// sampleEvery is how far the demo clock moves for each sample.
const sampleEvery = 2 * time.Second

// now is the sampler's Now. Each sample moves the demo clock by exactly
// sampleEvery, and every counter of the sample follows that time. A clock
// that ran on its own could give two samples the same time on a slow
// machine (dt 0), and the busy loop would drop under the orphan rule's
// threshold.
func (h *demoHost) now(clk *clock) func() time.Time {
	return func() time.Time {
		clk.Add(sampleEvery)
		t := clk.Now()
		h.mu.Lock()
		h.sampled = t
		h.mu.Unlock()
		return t
	}
}

func (h *demoHost) elapsed() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sampled.Sub(demoBase).Seconds()
}

func (h *demoHost) kill(pid int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.killed == nil {
		h.killed = map[int]bool{}
	}
	h.killed[pid] = true
}

func pulse(pid int, p procfs.Proc, elapsed float64) procfs.Proc {
	switch pid {
	case demoPID:
		p.Ticks += uint64(elapsed * procfs.ClockTicks)
	case 1200: // the busy session
		p.Ticks += uint64(elapsed * procfs.ClockTicks * 0.35)
	}
	return p
}

func (h *demoHost) Procs() (map[int]procfs.Proc, error) {
	ps, err := h.FS.Procs()
	e := h.elapsed()
	h.mu.Lock()
	defer h.mu.Unlock()
	for pid, p := range ps {
		if h.killed[pid] {
			delete(ps, pid)
			continue
		}
		ps[pid] = pulse(pid, p, e)
	}
	return ps, err
}

func (h *demoHost) Proc(pid int) (procfs.Proc, bool) {
	h.mu.Lock()
	gone := h.killed[pid]
	h.mu.Unlock()
	p, ok := h.FS.Proc(pid)
	if gone || !ok {
		return procfs.Proc{}, false
	}
	return pulse(pid, p, h.elapsed()), true
}

func (h *demoHost) UID(int) (int, bool) { return os.Getuid(), true }
func (h *demoHost) Uptime() float64     { return h.FS.Uptime() + h.elapsed() }

var _ procfs.Host = (*demoHost)(nil)

func (h *demoHost) Stat() (procfs.CPUTimes, []procfs.CPUTimes, error) {
	all, cores, err := h.FS.Stat()
	if err != nil {
		return all, cores, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	load := []uint64{38, 44, 41, 52, 47, 43}[h.calls%6]
	h.calls++
	if h.coreBusy == nil {
		h.coreBusy = make([]uint64, len(cores))
	}
	h.total += 100 * uint64(len(cores))
	h.busy += load * uint64(len(cores))
	all.Busy, all.Total = all.Busy+h.busy, all.Total+h.total
	for i := range cores {
		h.coreBusy[i] += min(100, load*coreLoad[i%len(coreLoad)]/8)
		cores[i].Busy += h.coreBusy[i]
		cores[i].Total += h.total / uint64(len(cores))
	}
	return all, cores, nil
}

// coreLoad spreads the load over the cores unevenly, in eighths.
var coreLoad = []uint64{9, 4, 7, 12, 5, 10, 3, 8, 11, 6, 4, 9, 7, 5, 13, 6}

func (h *demoHost) NetBytes() (uint64, uint64, error) {
	rx, tx, err := h.FS.NetBytes()
	e := h.elapsed()
	return rx + uint64(e*1.8e6), tx + uint64(e*0.4e6), err
}

func (h *demoHost) DiskBytes() (uint64, uint64, error) {
	r, w, err := h.FS.DiskBytes()
	e := h.elapsed()
	return r + uint64(e*3e6), w + uint64(e*5e6), err
}

// recorder wraps the console so the test can read its screen.
type recorder struct{ tea.Model }

type captureMsg struct{ reply chan string }

func (r recorder) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if c, ok := msg.(captureMsg); ok {
		c.reply <- r.Model.View().Content
		return r, nil
	}
	m, cmd := r.Model.Update(msg)
	r.Model = m
	return r, cmd
}

// waitFor reads the screen until ok holds, or fails with the step name and
// the last screen.
func waitFor(capture func() string, ok func(string) bool, step string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	last := ""
	for time.Now().Before(deadline) {
		last = capture()
		if ok(ansi.Strip(last)) {
			return last, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return last, fmt.Errorf("step %q: the screen never showed what it waits for; last screen:\n%s", step, ansi.Strip(last))
}

var (
	figure = regexp.MustCompile(`\d+(\.\d+)?[A-Za-z%]*`)
	sparks = regexp.MustCompile(`[▁▂▃▄▅▆▇█]+`)
	spaces = regexp.MustCompile(`\s+`)
)

// skeleton is a screen without what changes from run to run: figures,
// sparklines and the space around them. A change in the console's words,
// tabs or layout still shows.
func skeleton(lines []string) string {
	var b strings.Builder
	for _, l := range lines {
		l = sparks.ReplaceAllString(figure.ReplaceAllString(l, "0"), "")
		l = strings.ReplaceAll(l, "✧", "✦") // a burning match flickers between the two
		b.WriteString(strings.TrimSpace(spaces.ReplaceAllString(l, " ")))
		b.WriteByte('\n')
	}
	return b.String()
}

// screens decodes frames.json into the text of each frame.
func screens(js []byte) ([][]string, error) {
	var doc struct {
		Rows   int `json:"rows"`
		Frames []struct {
			Lines []json.RawMessage `json:"lines"`
		} `json:"frames"`
	}
	if err := json.Unmarshal(js, &doc); err != nil {
		return nil, err
	}
	cur := make([]string, doc.Rows)
	var out [][]string
	for _, f := range doc.Frames {
		for i, raw := range f.Lines {
			if string(raw) == "null" || i >= len(cur) {
				continue
			}
			var spans [][]any
			if err := json.Unmarshal(raw, &spans); err != nil {
				return nil, err
			}
			var b strings.Builder
			for _, sp := range spans {
				if t, ok := sp[0].(string); ok {
					b.WriteString(t)
				}
			}
			cur[i] = b.String()
		}
		out = append(out, append([]string{}, cur...))
	}
	return out, nil
}

func TestReplayWaitsOnScreen(t *testing.T) {
	_, err := waitFor(func() string { return "nothing here" }, func(string) bool { return false }, "the kill", 100*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "the kill") || !strings.Contains(err.Error(), "nothing here") {
		t.Fatalf("err %v, want the step name and the last screen", err)
	}
}

type step struct {
	name string
	at   time.Duration
	key  string
	ok   func(screen string) bool
}

func has(s ...string) func(string) bool {
	return func(screen string) bool {
		for _, x := range s {
			if !strings.Contains(screen, x) {
				return false
			}
		}
		return true
	}
}

func keyMsg(k string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Text: k, Code: []rune(k)[0]}
}

func TestReplay(t *testing.T) {
	isolate(t)
	root, err := filepath.Abs("../../testdata/demo")
	if err != nil {
		t.Fatal(err)
	}
	clk := &clock{t: demoBase}
	cfg := config.Default()
	cfg.Interval.Duration = 50 * time.Millisecond
	cfg.Agents = []string{"claude", "codex", "opencode"}
	smp := newSampler(cfg, root)
	// OpenCode works: its spinner turns on each read of its pane.
	panes, turn := smp.Capture, 0
	smp.Capture = func(ids []string) map[string]string {
		out := panes(ids)
		if text, ok := out["%7"]; ok {
			spin := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
			turn++
			out["%7"] = strings.Replace(text, "⠙", string(spin[turn%len(spin)]), 1)
		}
		return out
	}
	host := &demoHost{FS: smp.FS.(procfs.FS)}
	smp.FS, smp.Now = host, host.now(clk)

	var mu sync.Mutex
	var ran [][]string
	s := service.New(cfg, smp, nil)
	s.Binary, s.Host = version, fixtureHost(root)
	s.Actions.Check = func(advice.Guard) error { return nil } // the demo's pids are not this machine's
	s.Actions.Run = func(argv []string) error {
		mu.Lock()
		ran = append(ran, argv)
		mu.Unlock()
		if len(argv) == 2 && argv[0] == "kill" && argv[1] == fmt.Sprint(demoPID) {
			host.kill(demoPID)
			return nil
		}
		return errors.New("the demo runs only the kill")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	// Warm up before the console starts: the replay opens on a machine at work.
	if _, err := waitFor(func() string { return fmt.Sprint(len(s.State().Orphans)) }, func(n string) bool { return n == "1" }, "warm up", stepTimeout); err != nil {
		t.Fatal(err)
	}
	// billing asks for permission, through its hook.
	s.Hook(hooks.Event{Name: "Notification", Kind: hooks.KindPermission, SessionID: "22222222-bbbb-4000-8000-000000000004",
		Pane: "%5", Message: "Claude needs your permission to use Bash", At: clk.Now().Add(-90 * time.Second)})
	time.Sleep(500 * time.Millisecond) // ten more samples: full sparklines
	// The console reads its own clock: it stands still for the start screen,
	// then follows the demo clock.
	view := &clock{t: clk.Now()}
	warm := view.Now()

	a, b := net.Pipe()
	go s.Handle(ctx, transport.NewConn(b)) //nolint:errcheck // ends with the test

	m := app.New(app.Options{
		Conn: transport.NewConn(a), Binary: version, CompactAt: cfg.Sessions.CompactAt, Now: view.Now,
		Run: func(argv []string) error { return fmt.Errorf("the demo moves no terminal: %v", argv) },
	})
	p := tea.NewProgram(recorder{m}, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(&bytes.Buffer{}),
		tea.WithWindowSize(replayCols, replayRows))
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()
	capture := func() string {
		reply := make(chan string, 1)
		p.Send(captureMsg{reply})
		select {
		case v := <-reply:
			return v
		case <-time.After(stepTimeout):
			t.Fatal("the console does not answer")
			return ""
		}
	}

	tokens := app.Tokens(true)
	var frames []replay.Frame
	keep := func(at time.Duration, view string) {
		t.Helper()
		lines, err := replay.Parse(view, tokens)
		if err != nil {
			t.Fatalf("frame at %v: %v", at, err)
		}
		if len(lines) > replayRows {
			t.Fatalf("frame at %v has %d lines, the screen has %d", at, len(lines), replayRows)
		}
		for len(lines) < replayRows {
			lines = append(lines, nil)
		}
		frames = append(frames, replay.Frame{At: at, Lines: lines})
	}

	// The start screen, frame by frame on the replay clock, once the first
	// state is in (the clock stands still, so the start screen waits). The
	// last frame is the match burning, 1 ms before the console, and it holds.
	if _, err := waitFor(capture, func(s string) bool { return !strings.Contains(s, "waiting for the service") }, "the first state", stepTimeout); err != nil {
		t.Fatal(err)
	}
	for _, ms := range []int{0, 70, 140, 210, 280, 350, 420, 490, 559} {
		view.Set(warm.Add(time.Duration(ms) * time.Millisecond))
		keep(time.Duration(ms)*time.Millisecond, capture())
	}

	view.Follow(clk)
	time.Sleep(600 * time.Millisecond) // a dozen samples reach the console: the sparklines fill

	steps := []step{
		{"the queue", 1300 * time.Millisecond, "", has("ws-1", "WAITING FOR YOU", "billing", "asks", "permission to use Bash", "codex", "Would you like to run", "PROCS !")},
		{"the sessions", 3500 * time.Millisecond, "2", has("api-auth", "web-checkout", "! compact", "codex", "55%", "opencode", "32%")},
		{"machine health", 6 * time.Second, "3", has("CPU")},
		{"what to do, under the queue", 8 * time.Second, "1", has("RECOMMENDATIONS", "Kill detached busy loop 4242")},
		{"the busy loop", 9500 * time.Millisecond, "4", has("4242")},
		{"a guarded action", 10500 * time.Millisecond, "x", has("kill 4242", "y run")},
		// The result as well as the state: a late result would still show in
		// the next step.
		{"the loop is gone", 12 * time.Second, "y", func(s string) bool {
			return !strings.Contains(s, "while :; do go test") && strings.Contains(s, "ran: kill 4242")
		}},
		{"back to the queue", 13500 * time.Millisecond, "1", has("billing")},
	}
	var still replay.Frame
	for _, st := range steps {
		if st.key != "" {
			p.Send(keyMsg(st.key))
		}
		view, err := waitFor(capture, st.ok, st.name, stepTimeout)
		if err != nil {
			t.Fatal(err)
		}
		keep(st.at, view)
		if st.name == "a guarded action" {
			still = frames[len(frames)-1]
		}
	}
	p.Quit()
	<-done

	lit := false
	for _, f := range frames {
		for _, l := range f.Lines {
			for _, sp := range l {
				lit = lit || sp.FG == "flame" || sp.BG == "flame"
				if strings.Contains(sp.Text, "waiting for the service") {
					t.Fatalf("frame at %v shows a slow service: the start screen must be recorded after the first state", f.At)
				}
			}
		}
	}
	if !lit {
		t.Fatal("the replay never shows the struck, burning match")
	}
	// The replay shows the three matches: burning, at rest and spent.
	matches := map[string]bool{}
	for _, f := range frames {
		for _, l := range f.Lines {
			for _, sp := range l {
				switch text := strings.TrimSpace(sp.Text); {
				case (text == "✦" || text == "✧") && strings.HasPrefix(sp.FG, "flame"):
					matches["burning"] = true
				case text == "╿" && strings.HasPrefix(sp.FG, "match-"):
					matches["rest"] = true
				case text == "│" && sp.FG == "faint":
					matches["spent"] = true
				}
			}
		}
	}
	for _, want := range []string{"burning", "rest", "spent"} {
		if !matches[want] {
			t.Fatalf("the replay lacks a %s match; has %v", want, matches)
		}
	}
	if real, _ := os.Hostname(); real != "" && real != "ws-1" {
		for _, f := range frames {
			for _, l := range f.Lines {
				for _, sp := range l {
					if strings.Contains(sp.Text, real) {
						t.Fatalf("a frame shows the real host name %q", real)
					}
				}
			}
		}
	}
	mu.Lock()
	if want := [][]string{{"kill", fmt.Sprint(demoPID)}}; !reflect.DeepEqual(ran, want) {
		t.Fatalf("the service ran %v, want only %v", ran, want)
	}
	mu.Unlock()

	js, err := replay.JSON(replayCols, replayRows, frames)
	if err != nil {
		t.Fatal(err)
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write(js)
	_ = zw.Close()
	if gz.Len() > 150<<10 {
		t.Fatalf("frames.json is %d KB gzipped, the budget is 150 KB", gz.Len()>>10)
	}
	svg := replay.SVG(replayCols, replayRows, frames, map[bool]map[string]color.Color{true: tokens, false: app.Tokens(false)})

	index := "../../www/index.html"
	page, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	const open, closing = "<!-- replay:still -->", "<!-- /replay:still -->"
	i, j := bytes.Index(page, []byte(open)), bytes.Index(page, []byte(closing))
	if i < 0 || j < i {
		t.Fatalf("%s needs %s and %s around the still frame", index, open, closing)
	}
	if !*update {
		// The committed replay must still show this console: compare the
		// skeleton of each step's screen.
		old, err := os.ReadFile("../../www/demo/frames.json")
		if err != nil {
			t.Fatal(err)
		}
		was, err := screens(old)
		if err != nil {
			t.Fatal(err)
		}
		now, err := screens(js)
		if err != nil {
			t.Fatal(err)
		}
		if len(was) != len(now) {
			t.Fatalf("www/demo/frames.json has %d frames, the console now makes %d: run go test ./cmd/matchblox -run TestReplay -update", len(was), len(now))
		}
		for k := len(now) - len(steps); k < len(now); k++ {
			if skeleton(was[k]) != skeleton(now[k]) {
				t.Fatalf("www/demo/frames.json is out of date at step %q: run go test ./cmd/matchblox -run TestReplay -update\nwas:\n%s\nnow:\n%s",
					steps[k-len(now)+len(steps)].name, skeleton(was[k]), skeleton(now[k]))
			}
		}
		return
	}
	newPage := append(append(append([]byte{}, page[:i+len(open)]...), "\n"+replay.Still(still)+"\n"...), page[j:]...)
	for path, b := range map[string][]byte{"../../www/demo/frames.json": js, "../../www/demo/replay.svg": svg, index: newPage} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSkeletonCatchesAChangedScreen(t *testing.T) {
	a := []string{" ▰ matchblox · ws-1  │  cpu  47% ▅▄▄▄  │  mem 26G/64G ▄▄", " 1 SESSIONS   2 MACHINE", " web-checkout  idle  4m   88%"}
	b := []string{" ▰ matchblox · ws-1  │  cpu  52% ▄▅  │  mem 27G/64G ▄▄▄▄▄", " 1 SESSIONS   2 MACHINE", " web-checkout  idle  5m   88%"}
	if skeleton(a) != skeleton(b) {
		t.Fatalf("figures and sparklines change between runs; the skeletons must match:\n%q\n%q", skeleton(a), skeleton(b))
	}
	c := append([]string{}, b...)
	c[1] = " 1 PANES   2 MACHINE"
	if skeleton(a) == skeleton(c) {
		t.Fatal("a renamed tab must change the skeleton")
	}
}
