package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
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
	"github.com/blox-eng/matchblox/internal/stoker"
	"github.com/blox-eng/matchblox/internal/transport"
)

var update = flag.Bool("update", false, "TestReplay: write www/demo/frames.json, www/demo/replay.svg and the still frame in www/index.html")

const (
	replayCols, replayRows = 100, 30
	demoPID                = 4242
	stepTimeout            = 10 * time.Second
)

// demoBase is the demo machine's clock at the start of the replay.
var demoBase = time.Date(2026, 10, 1, 22, 50, 0, 0, time.UTC)

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
	samples     int // so far: the counters follow them, not the demo clock, which jumps to the night and the morning
	killed      map[int]bool
	calls       int
	busy, total uint64
	coreBusy    []uint64
}

// sampleEvery is how far the demo clock moves for each sample.
const sampleEvery = 2 * time.Second

// now is the sampler's Now. Each sample moves the demo clock by exactly
// sampleEvery, and every counter of the sample follows the samples. A
// clock that ran on its own could give two samples the same time on a
// slow machine (dt 0), and the busy loop would drop under the orphan
// rule's threshold.
func (h *demoHost) now(clk *clock) func() time.Time {
	return func() time.Time {
		clk.Add(sampleEvery)
		h.mu.Lock()
		h.samples++
		h.mu.Unlock()
		return clk.Now()
	}
}

func (h *demoHost) elapsed() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return (time.Duration(h.samples) * sampleEvery).Seconds()
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
	burning = regexp.MustCompile(`flint-auth\s+[✦✧] busy`)
	figure  = regexp.MustCompile(`\d+(\.\d+)?[A-Za-z%]*`)
	sparks  = regexp.MustCompile(`[▁▂▃▄▅▆▇█]+`)
	spaces  = regexp.MustCompile(`\s+`)
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

// step presses its keys, then key, after do; ok says what the screen must
// show before the frame is kept.
type step struct {
	name string
	at   time.Duration
	do   func()
	keys []string
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
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	}
	return tea.KeyPressMsg{Text: k, Code: []rune(k)[0]}
}

func TestReplay(t *testing.T) {
	isolate(t)
	src, err := filepath.Abs("../../testdata/demo")
	if err != nil {
		t.Fatal(err)
	}
	clk := &clock{t: demoBase}
	var host *demoHost
	world := newWorld(t, src, clk, func(pid int) { host.kill(pid) })
	root := world.root
	cfg := config.Default()
	cfg.Interval.Duration = 50 * time.Millisecond
	cfg.Agents = []string{"claude", "codex", "opencode"}
	smp := newSampler(cfg, root)
	// Codex and OpenCode work: their spinners turn on each read of the pane.
	panes, turn := smp.Capture, 0
	smp.Capture = func(ids []string) map[string]string {
		out := panes(ids)
		spin := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
		turn++
		for _, p := range []string{"%6", "%7"} {
			if text, ok := out[p]; ok {
				out[p] = strings.Replace(text, "⠙", string(spin[turn%len(spin)]), 1)
			}
		}
		return out
	}
	host = &demoHost{FS: smp.FS.(procfs.FS)}
	smp.FS, smp.Now = host, host.now(clk)

	s := service.New(cfg, smp, world.Git)
	s.Binary, s.Host, s.Now = version, fixtureHost(root), clk.Now
	s.Capture, s.Activity = smp.Capture, nil                  // the demo's panes, never this machine's tmux
	s.Actions.Check = func(advice.Guard) error { return nil } // the demo's pids are not this machine's
	s.Actions.Run = world.Run
	st, err := stoker.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.Stoker = st
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	// Warm up before the console starts: the replay opens on a machine at work.
	if _, err := waitFor(func() string { return fmt.Sprint(len(s.State().Orphans)) }, func(n string) bool { return n == "1" }, "warm up", stepTimeout); err != nil {
		t.Fatal(err)
	}
	// kindle-web asks for permission, through its hook.
	s.Hook(hooks.Event{Name: "Notification", Kind: hooks.KindPermission, SessionID: "5ea1c0de-0000-4000-8000-000000000004",
		Pane: "%5", Message: "Claude needs your permission to use Bash", At: clk.Now().Add(-90 * time.Second)})
	time.Sleep(500 * time.Millisecond) // ten more samples: full sparklines
	// The console reads its own clock: it stands still for the start screen,
	// then follows the demo clock.
	view := &clock{t: clk.Now()}
	warm := view.Now()

	a, b := net.Pipe()
	go s.Handle(ctx, transport.NewConn(b)) //nolint:errcheck // ends with the test

	var mu sync.Mutex
	var jumps [][]string
	m := app.New(app.Options{
		Conn: transport.NewConn(a), Binary: version, CompactAt: cfg.Sessions.CompactAt, Now: view.Now,
		Run: func(argv []string) error { // a jump: the demo moves no terminal
			mu.Lock()
			jumps = append(jumps, argv)
			mu.Unlock()
			return nil
		},
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
	var names []string // the step of each frame
	keep := func(name string, at time.Duration, view string) {
		t.Helper()
		if len(frames) > 0 && at <= frames[len(frames)-1].At {
			t.Fatalf("step %q at %v does not come after the frame at %v", name, at, frames[len(frames)-1].At)
		}
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
		names = append(names, name)
	}

	// The start screen, frame by frame on the replay clock, once the first
	// state is in (the clock stands still, so the start screen waits). The
	// last frame is the match burning, 1 ms before the console, and it holds.
	if _, err := waitFor(capture, func(s string) bool { return !strings.Contains(s, "waiting for the service") }, "the first state", stepTimeout); err != nil {
		t.Fatal(err)
	}
	for _, ms := range []int{0, 70, 140, 210, 280, 350, 420, 490, 559} {
		view.Set(warm.Add(time.Duration(ms) * time.Millisecond))
		keep("the start screen", time.Duration(ms)*time.Millisecond, capture())
	}

	view.Follow(clk)
	time.Sleep(600 * time.Millisecond) // a dozen samples reach the console: the sparklines fill

	night := time.Date(2026, 10, 1, 23, 10, 0, 0, time.UTC)
	full := time.Date(2026, 10, 1, 23, 40, 0, 0, time.UTC)
	morning := time.Date(2026, 10, 2, 6, 59, 50, 0, time.UTC)
	steps := []step{
		{name: "the queue", at: 1300 * time.Millisecond, ok: has("ws-1", "WAITING FOR YOU", "flint-auth", "waits",
			"kindle-web", "needs your permission", "#212", "ember-billing", "done", "1 match", "2 sparks")},
		{name: "a: the answer field", at: 3800 * time.Millisecond, key: "a", ok: has("flint-auth")},
	}
	at := 3800 * time.Millisecond
	answer := "rotate it, keep the old one valid for a minute"
	for typed := 0; typed < len(answer); {
		next := min(typed+4, len(answer))
		at += 70 * time.Millisecond
		steps = append(steps, step{name: "the answer types itself", at: at, keys: strings.Split(answer[typed:next], ""), ok: has(answer[:next])})
		typed = next
	}
	steps = append(steps, []step{
		{name: "the answer, before it runs", at: 5200 * time.Millisecond, key: "enter", ok: has("RUN tmux send-keys -t %2 -l -- rotate it", "y run")},
		{name: "the answer goes", at: 6400 * time.Millisecond, key: "y", ok: func(s string) bool {
			return !strings.Contains(s, "flint-auth") && strings.Contains(s, "3 WAITING FOR YOU") && strings.Contains(s, "ran: tmux send-keys -t %2")
		}},
		// flint-auth burns: its strike has played, so every run draws the same frame.
		{name: "the sessions", at: 8 * time.Second, key: "2", ok: func(s string) bool {
			return has("hearth-api", "spark-docs", "! compact", "88%", "kindle-web", "ember-bill", "cinder-ci", "wick-mobile", "codex", "opencode")(s) &&
				burning.MatchString(s)
		}},
		{name: "the limits and what to do", at: 11 * time.Second, key: "1", ok: has("Max", "1 match", "2 sparks", "RECOMMENDATIONS",
			"The week runs short: compact spark-docs")},
	}...)
	// seek presses j until the screen shows what ok wants: where the
	// selection starts depends on the steps before.
	seek := func(ok func(string) bool) func() {
		return func() {
			for range 12 {
				if ok(ansi.Strip(capture())) {
					return
				}
				p.Send(keyMsg("j"))
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
	weekRec := has("The week runs short: compact spark-docs", "sparks left", "x send /compact, asks y")
	steps = append(steps, step{name: "the rec selected", at: 12500 * time.Millisecond, do: seek(weekRec), ok: weekRec})
	steps = append(steps, []step{
		{name: "the compact, guarded", at: 14 * time.Second, key: "x", ok: has("tmux send-keys -t %3 /compact Enter", "y run")},
		{name: "the compact ran", at: 15 * time.Second, key: "y", ok: func(s string) bool {
			// Settled: spark-docs compacted, so the week's rec names the next session.
			return strings.Contains(s, "ran: tmux send-keys -t %3 /compact Enter") && !strings.Contains(s, "Compact spark-docs (%3)") &&
				strings.Contains(s, "The week runs short: compact hearth-api")
		}},
		{name: "git", at: 17 * time.Second, key: "5", ok: has("9 WORKTREES", "1 SAFE TO REMOVE", "kindle-cart-v1", "✓ safe to remove")},
		{name: "a worktree marked", at: 18300 * time.Millisecond, key: "X", ok: has("●kindle-cart-v1")},
		{name: "the remove, guarded", at: 19300 * time.Millisecond, key: "x", ok: has("worktree remove /work/kindle-cart-v1", "y run")},
		{name: "removed", at: 20300 * time.Millisecond, key: "y", ok: has("8 WORKTREES", "removed 1")},
	}...)
	steps = append(steps, []step{
		{name: "machine health", at: 22300 * time.Millisecond, key: "3", ok: has("CPU")},
		{name: "the busy loop", at: 23800 * time.Millisecond, key: "4", ok: has("4242", "DETACHED BUSY LOOPS")},
		{name: "a guarded kill", at: 24800 * time.Millisecond, key: "x", ok: has("kill 4242", "y run")},
		// The result as well as the state: a late result would still show
		// in the next step.
		{name: "the loop is gone", at: 26 * time.Second, key: "y", ok: func(s string) bool {
			return !strings.Contains(s, "while :; do go test") && strings.Contains(s, "ran: kill 4242")
		}},
		{name: "back to the queue", at: 27500 * time.Millisecond, key: "1", ok: has("kindle-web")},
		{name: "a night", at: 29 * time.Second, do: func() { clk.Set(night) }, key: "n", ok: has("night → 07:00")},
		{name: "take over?", at: 31 * time.Second, key: "enter", ok: has("The stoker is on. Take over?")},
		{name: "kept on", at: 33 * time.Second, key: "n", ok: func(s string) bool {
			return strings.Contains(s, "night → 07:00") && !strings.Contains(s, "Take over?") && strings.Contains(s, "RUN tmux switch-client -t %5")
		}},
		{name: "back to the night", at: 34200 * time.Millisecond, key: "esc", ok: func(s string) bool {
			return strings.Contains(s, "night → 07:00") && !strings.Contains(s, "RUN tmux switch-client")
		}},
		{name: "the stoker compacts", at: 35500 * time.Millisecond, do: func() {
			clk.Set(full)
			if err := world.Finish("%1", 176000, "Rate limits are in, with tests.\n\nProgress [██████████] rate limits on the public API"); err != nil {
				t.Fatal(err)
			}
		}, ok: has("STOKED", "hearth-api", "176k → 9k", "RESUME", "rate-limit headers")},
		{name: "the morning card", at: 39500 * time.Millisecond, do: func() { clk.Set(morning) },
			ok: has("NIGHT 23:10 → 07:00", "ready 4/5", "1 compacted", "167k tokens given back", "kindle-web       waits for you", "176k → 9k", "RESUME")},
		{name: "folded", at: 44 * time.Second, keys: []string{"k", "k", "k", "k", "k", "k"}, key: "enter", ok: func(s string) bool {
			return !strings.Contains(s, "NIGHT 23:10") && strings.Contains(s, "WAITING FOR YOU")
		}},
	}...)
	var still replay.Frame
	for _, st := range steps {
		if st.do != nil {
			st.do()
		}
		for _, k := range append(st.keys, st.key) {
			if k != "" {
				p.Send(keyMsg(k))
			}
		}
		view, err := waitFor(capture, st.ok, st.name, stepTimeout)
		if err != nil {
			t.Fatal(err)
		}
		keep(st.name, st.at, view)
		if st.name == "the sessions" {
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
	if err := private(frames, thisMachine()); err != nil {
		t.Fatal(err)
	}
	ran := world.Ran()
	want := [][]string{
		{"tmux", "send-keys", "-t", "%2", "-l", "--", answer}, {"tmux", "send-keys", "-t", "%2", "Enter"},
		{"tmux", "send-keys", "-t", "%3", "/compact", "Enter"},
		{"git", "-C", "/work/hearth", "worktree", "remove", "/work/kindle-cart-v1"},
		{"kill", fmt.Sprint(demoPID)},
	}
	if len(ran) != len(want)+2 || !reflect.DeepEqual(ran[:len(want)], want) || ran[len(want)][0] != "tmux" ||
		!strings.HasPrefix(ran[len(want)][6], "/compact ") || !reflect.DeepEqual(ran[len(want)+1], []string{"tmux", "send-keys", "-t", "%1", "Enter"}) {
		t.Fatalf("the service ran %q, want %q and then the stoker's compact of %%1", ran, want)
	}

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
		// skeleton of each frame's screen.
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
		for k := range now {
			if skeleton(was[k]) != skeleton(now[k]) {
				t.Fatalf("www/demo/frames.json is out of date at step %q: run go test ./cmd/matchblox -run TestReplay -update\nwas:\n%s\nnow:\n%s",
					names[k], skeleton(was[k]), skeleton(now[k]))
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
