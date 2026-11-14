package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"image/color"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/app"
	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/demo"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/procfs"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/replay"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/service"
	"github.com/blox-eng/matchblox/internal/ships"
	"github.com/blox-eng/matchblox/internal/stoker"
	"github.com/blox-eng/matchblox/internal/transport"
)

var update = flag.Bool("update", false, "TestReplay: write www/demo/frames-<size>.json, www/demo/replay-<size>.svg and the open Duo's still frame in www/index.html; TestStokerSVG: write www/demo/stoker.svg and the stoker's still frame")

const stepTimeout = 20 * time.Second

// replaySize is a screen the replay is recorded on. The site's foldable
// plays the phone folded and the duo open (landscape, the Wide layout);
// the README shows the desk and the phone.
type replaySize struct {
	name       string
	cols, rows int
}

var replaySizes = []replaySize{{"desk", 132, 32}, {"duo", 104, 38}, {"phone", 44, 31}}

// demoBase is the demo machine's clock at the start of the replay.
var demoBase = time.Date(2026, 10, 1, 22, 50, 0, 0, time.UTC)

// clock is the replay's clock: the recorder sets it, and both the service
// and the console read it.
type clock struct {
	mu     sync.Mutex
	t      time.Time
	follow *clock        // set: this clock reads that one
	ahead  time.Duration // how far ahead of the one it follows
	// upTo, when set, is the latest the clock may read while it follows.
	upTo func() (time.Time, bool)
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.follow != nil {
		now := c.follow.Now().Add(c.ahead)
		if c.upTo != nil {
			if most, ok := c.upTo(); ok && now.After(most) {
				return most
			}
		}
		return now
	}
	return c.t
}

func (c *clock) Follow(o *clock)       { c.mu.Lock(); c.follow = o; c.mu.Unlock() }
func (c *clock) Set(t time.Time)       { c.mu.Lock(); c.t = t; c.mu.Unlock() }
func (c *clock) Add(d time.Duration)   { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }
func (c *clock) Ahead(d time.Duration) { c.mu.Lock(); c.ahead += d; c.mu.Unlock() }

// heardConn is the console's end of the service, noting the time of the
// last snapshot the console has taken in: when it asks for the next
// message, it is done with the one before.
type heardConn struct {
	transport.Conn
	mu        sync.Mutex
	last, had time.Time
}

func (c *heardConn) Recv() (proto.Envelope, error) {
	c.mu.Lock()
	c.had = c.last
	c.mu.Unlock()
	env, err := c.Conn.Recv()
	if err == nil && env.Kind == proto.KindSnapshot {
		var snap struct {
			At time.Time `json:"at"`
		}
		if json.Unmarshal(env.Body, &snap) == nil {
			c.mu.Lock()
			c.last = snap.At
			c.mu.Unlock()
		}
	}
	return env, err
}

// fresh is the latest a console clock may read so that the console's data
// is never older than within: the demo clock runs 40 times faster than the
// wall's, and a slow (-race) console a second behind its samples would
// show them as stale.
func (c *heardConn) fresh(within time.Duration) func() (time.Time, bool) {
	return func() (time.Time, bool) {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.had.Add(within), !c.had.IsZero()
	}
}

// Hold stops the clock where it reads now; Follow starts it again.
func (c *clock) Hold() {
	now := c.Now()
	c.mu.Lock()
	c.t, c.follow = now, nil
	c.mu.Unlock()
}

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
	case demo.OrphanPID:
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
	figure  = regexp.MustCompile(`\d+(\.\d+)?[A-Za-z%]*`)
	sparks  = regexp.MustCompile(`[▁▂▃▄▅▆▇█]+`)
	spaces  = regexp.MustCompile(`\s+`)
	cutWord = regexp.MustCompile(`\S*…`)
)

// skeleton is a screen without what changes from run to run: figures,
// sparklines, the space around them, and where a line is cut, which moves
// with the width of a figure before it. A change in the console's words,
// tabs or layout still shows.
var spinner = regexp.MustCompile(`[\x{2800}-\x{28FF}]`)

// stillSize is the recording whose still frame stands in www/index.html,
// for no script and reduced motion: the open Duo.
const stillSize = "duo"

var tag = regexp.MustCompile(`<[^>]*>`)

// stillLines is the text of the still frame's HTML, a line per row.
func stillLines(b []byte) []string {
	return strings.Split(strings.Trim(html.UnescapeString(tag.ReplaceAllString(string(b), "")), "\n"), "\n")
}

// frameLines is the text of a frame, a line per row.
func frameLines(f replay.Frame) []string {
	out := make([]string, len(f.Lines))
	for i, l := range f.Lines {
		for _, s := range l {
			out[i] += s.Text
		}
	}
	return out
}

func skeleton(lines []string) string {
	var b strings.Builder
	for i, l := range lines {
		if i == 0 {
			continue // the header's toast says what just happened, at the moment of the frame
		}
		// The busy loop's alert comes and goes with the samples; on a phone
		// it squeezes the tab to its digit.
		l = strings.Replace(strings.Replace(l, "machine !", "machine", 1), " 3 !", " machine", 1)
		l = sparks.ReplaceAllString(figure.ReplaceAllString(cutWord.ReplaceAllString(l, "…"), "0"), "")
		l = strings.ReplaceAll(l, "✧", "✦")  // a burning match flickers between the two
		l = spinner.ReplaceAllString(l, "⠿") // a working session's spinner turns with the clock
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
	// hold stops the console's clock until the step's screen shows: the
	// demo clock runs 40 times faster than the wall's, so a notice of
	// 2.5 s lives some 60 ms, less than one read of a slow (-race) run.
	hold bool
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
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "ctrl+o":
		return tea.KeyPressMsg{Code: rune(k[5]), Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Text: k, Code: []rune(k)[0]}
}

// TestReplay records the console on the demo machine at each of the
// site's sizes: a phone, an iPhone Duo opened horizontally, a desk.
func TestReplay(t *testing.T) {
	for _, z := range replaySizes {
		t.Run(z.name, func(t *testing.T) { replayAt(t, z) })
	}
}

func replayAt(t *testing.T, z replaySize) {
	isolate(t)
	phone := z.cols < 60
	src, err := filepath.Abs("../../testdata/demo")
	if err != nil {
		t.Fatal(err)
	}
	clk := &clock{t: demoBase}
	var host *demoHost
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	world, err := demo.NewWorld(root, clk.Now, func(pid int) { host.kill(pid) })
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Interval.Duration = 50 * time.Millisecond
	cfg.Agents = []string{"claude", "codex", "opencode"}
	smp := newSampler(cfg, root)
	// Codex and OpenCode work: their spinners turn on each read of the pane.
	// The sampler and the guards of the service read panes at once.
	panes, turn := smp.Capture, atomic.Int64{}
	smp.Capture = func(ids []string) map[string]string {
		out := panes(ids)
		spin := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
		n := int(turn.Add(1))
		for _, p := range []string{"%6", "%7"} {
			if text, ok := out[p]; ok {
				out[p] = strings.Replace(text, "⠙", string(spin[n%len(spin)]), 1)
			}
		}
		return out
	}
	host = &demoHost{FS: smp.FS.(procfs.FS)}
	smp.FS, smp.Now = host, host.now(clk)
	world.Uptime = host.Uptime

	s := service.New(cfg, smp, world.Git)
	s.Binary, s.Host, s.Now = version, demo.Host(root), clk.Now
	s.Capture, s.Activity = smp.Capture, nil // the demo's panes, never this machine's tmux
	world.Wire(s)
	st, err := stoker.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.Stoker = st
	// The merge is the one act the demo's world does not take: the replay
	// takes it here, and the ship moves on to merged.
	var merged atomic.Bool
	worldRun := s.Actions.Run
	s.Actions.Run = func(argv []string) error {
		if slices.Equal(argv, demoMerge) {
			merged.Store(true)
			return nil
		}
		return worldRun(argv)
	}
	s.Ships = demoShips(clk, &merged)
	// The ship reads merged at the next read of the pull requests: every
	// 15 s of the wall's time, a long wait for a replay that moves 40 times
	// faster. A read every 300 ms keeps the step to a beat of the replay.
	s.ShipsEvery = 300 * time.Millisecond
	// hearth-api, kindle-web and ember-billing are one chain, hearth.
	s.Chains = &chains.Store{Chains: []chains.Chain{{ID: "c1", Name: "hearth", Repo: "/work/hearth", Members: []chains.Member{
		{SessionID: "5ea1c0de-0000-4000-8000-000000000001"}, {SessionID: "5ea1c0de-0000-4000-8000-000000000004"},
		{SessionID: "5ea1c0de-0000-4000-8000-000000000005"}}}}}
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
	// The replay shows the board to a person new to it: the notices of the
	// keys that changed their job, and the copy check, are for one who used
	// the console before.
	views := t.TempDir()
	if err := os.WriteFile(filepath.Join(views, "view.json"),
		[]byte(`{"told":["chain","copy","density","digit","host","kill","merge","strip"],"copy_checked_phone":true,"copy_checked_desk":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	heard := &heardConn{Conn: transport.NewConn(a)}
	view.upTo = heard.fresh(20 * time.Second) // under staleAfter (30 s)
	m := app.New(app.Options{
		Conn: heard, Binary: version, CompactAt: cfg.Sessions.CompactAt, Now: view.Now, StateDir: views,
		Run: func(argv []string) error { // a jump: the demo moves no terminal
			mu.Lock()
			jumps = append(jumps, argv)
			mu.Unlock()
			return nil
		},
	})
	p := tea.NewProgram(recorder{m}, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(&bytes.Buffer{}),
		tea.WithWindowSize(z.cols, z.rows))
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
		if len(lines) > z.rows {
			t.Fatalf("frame at %v has %d lines, the screen has %d", at, len(lines), z.rows)
		}
		for len(lines) < z.rows {
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

	// chosen is a screen whose chosen card is name's: its line starts with
	// the selection mark, then the match.
	chosen := func(name string) func(string) bool {
		re := regexp.MustCompile(`▌\S+ ` + regexp.QuoteMeta(name))
		return func(s string) bool { return re.MatchString(s) }
	}
	// choose goes to the first column, then moves the selection down it
	// until name's card is chosen.
	choose := func(name string) func() {
		return func() {
			for range 3 {
				p.Send(keyMsg("left"))
			}
			for range 8 {
				if chosen(name)(ansi.Strip(capture())) {
					return
				}
				p.Send(keyMsg("j"))
			}
		}
	}
	// tapOn clicks the first cell of text on the screen, as a thumb or a
	// mouse does.
	tapOn := func(text string) func() {
		return func() {
			for y, l := range strings.Split(ansi.Strip(capture()), "\n") {
				if i := strings.Index(l, text); i >= 0 {
					p.Send(tea.MouseClickMsg{X: ansi.StringWidth(l[:i]), Y: y, Button: tea.MouseLeft})
					return
				}
			}
			t.Fatalf("no %q on the screen to tap", text)
		}
	}
	// settle lets the peek's dead time pass on the demo clock: a digit
	// arms nothing for 400 ms after the peek lands on a card.
	settle := func() { time.Sleep(300 * time.Millisecond) }
	steps := []step{
		// The board: every match in its column, and the accent on one card.
		{name: "the board", at: 1300 * time.Millisecond, ok: has("need you", "NEEDS YOU", "flint-auth")},
		// flint-auth asks: its card's peek shows the question's options.
		{name: "the question", at: 2600 * time.Millisecond, do: choose("flint-auth"), ok: func(s string) bool {
			return chosen("flint-auth")(s) && has("1. ", "2. ")(s)
		}},
		// A digit arms the answer; nothing is sent.
		{name: "an answer armed", at: 4200 * time.Millisecond, do: settle, key: "2", ok: has("❯ 2.", "again sends")},
		// The same digit sends it: the card strikes into WORKING. The frame
		// is the one where it left NEEDS YOU (4 need you before): the ones
		// before it race the samples that carry the answer.
		{name: "sent", at: 5800 * time.Millisecond, key: "2", hold: true, ok: func(s string) bool {
			return has("sent 2 to flint-auth", "● 3 need you")(s) && !chosen("flint-auth")(s)
		}},
		{name: "it works", at: 7400 * time.Millisecond, ok: func(s string) bool {
			return has("✦ flint-auth")(s) && !strings.Contains(s, "sent 2 to flint-auth")
		}},
		// p merges the ship that waits for its merge, after a typed y.
		{name: "a ship waits", at: 9 * time.Second, do: choose("ember-billing"), ok: func(s string) bool {
			return chosen("ember-billing")(s) && has("p merge #4552")(s)
		}},
		{name: "merge?", at: 10500 * time.Millisecond, key: "p", ok: has("pr merge 4552")},
		// The ship reads merged at the next read of the pull requests.
		{name: "merged", at: 12500 * time.Millisecond, key: "y", ok: func(s string) bool {
			return !strings.Contains(s, "pr merge 4552") && merged.Load() && strings.Contains(s, "merged deploying")
		}},
		// A tap on a column's name hides it; a tap on its + name brings it
		// back. A phone shows one column a page: a tap on a page's mark
		// opens it, and a tap on the open page's name hides it.
		{name: "a column hides", at: 14500 * time.Millisecond, do: tapOn("WORKING"), ok: has("+ work")},
		{name: "and comes back", at: 16500 * time.Millisecond, do: tapOn("+ work"), ok: func(s string) bool {
			return !strings.Contains(s, "+ work")
		}},
		// g narrows the board to the chosen card's chain; esc shows all.
		{name: "one chain", at: 18500 * time.Millisecond, do: choose("kindle-web"), key: "g", ok: has("chain hearth", "esc all")},
		{name: "every chain", at: 21 * time.Second, key: "esc", ok: func(s string) bool {
			return !strings.Contains(s, "chain hearth") && has("NEEDS YOU")(s)
		}},
	}
	if phone {
		// The card struck into WORKING, which is another page.
		steps[slices.IndexFunc(steps, func(st step) bool { return st.name == "it works" })].do = tapOn("work")
		at := slices.IndexFunc(steps, func(st step) bool { return st.name == "a column hides" })
		steps = slices.Insert(steps, at, step{name: "the work page", at: 13700 * time.Millisecond, do: tapOn("work"), ok: has("WORKING")})
	}
	var still replay.Frame
	for _, st := range steps {
		if st.do != nil {
			st.do()
		}
		if st.hold {
			view.Hold()
		}
		for _, k := range append(st.keys, st.key) {
			if k != "" {
				p.Send(keyMsg(k))
			}
		}
		// A slow (-race) run can draw a frame before the sample reaches the
		// console: the strip then says the data is old in place of what it
		// shows. The demo clock runs 40 times faster than the wall's, so
		// 750 ms behind is 30 s. A frame is kept only once the data is fresh.
		fresh := func(s string) bool { return st.ok(s) && !strings.Contains(s, "the service is slow") }
		screen, err := waitFor(capture, fresh, st.name, stepTimeout)
		if err != nil {
			t.Fatal(err)
		}
		if st.hold {
			view.Follow(clk)
		}
		keep(st.name, st.at, screen)
		if st.name == "the board" {
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
	mine, err := machineNames()
	if err != nil {
		t.Fatal(err)
	}
	if err := private(frames, mine); err != nil {
		t.Fatal(err)
	}
	// The story goes into no pane: it answers flint-auth's question with its
	// second option, then merges ember-billing's ship.
	mu.Lock()
	if len(jumps) != 0 {
		t.Fatalf("the replay jumped %q, want no jump", jumps)
	}
	mu.Unlock()
	if ran, want := world.Ran(), [][]string{{"tmux", "send-keys", "-t", "%2", "-l", "--", "2"}}; !reflect.DeepEqual(ran, want) {
		t.Fatalf("the service ran %q, want %q", ran, want)
	}
	if !merged.Load() {
		t.Fatalf("the merge of #4552 never ran: %v", demoMerge)
	}

	framesPath, svgPath := "../../www/demo/frames-"+z.name+".json", "../../www/demo/replay-"+z.name+".svg"
	js, err := replay.JSON(z.cols, z.rows, frames)
	if err != nil {
		t.Fatal(err)
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write(js)
	_ = zw.Close()
	if gz.Len() > 150<<10 {
		t.Fatalf("%s is %d KB gzipped, the budget is 150 KB", framesPath, gz.Len()>>10)
	}
	svg := replay.SVG(z.cols, z.rows, frames, map[bool]map[string]color.Color{true: tokens, false: app.Tokens(false)})

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
		old, err := os.ReadFile(framesPath)
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
			t.Fatalf("%s has %d frames, the console now makes %d: run go test ./cmd/matchblox -run TestReplay -update", framesPath, len(was), len(now))
		}
		for k := range now {
			if skeleton(was[k]) != skeleton(now[k]) {
				t.Fatalf("%s is out of date at step %q: run go test ./cmd/matchblox -run TestReplay -update\nwas:\n%s\nnow:\n%s",
					framesPath, names[k], skeleton(was[k]), skeleton(now[k]))
			}
		}
		if z.name == stillSize {
			if was, now := skeleton(stillLines(page[i+len(open):j])), skeleton(frameLines(still)); was != now {
				t.Fatalf("the still frame in %s is not the %s's: run go test ./cmd/matchblox -run TestReplay -update\nwas:\n%s\nnow:\n%s", index, z.name, was, now)
			}
		}
		return
	}
	out := map[string][]byte{framesPath: js}
	if z.name != stillSize { // the README shows the desk and the phone
		out[svgPath] = svg
	}
	if z.name == stillSize { // the page's still frame, for no script and reduced motion
		out[index] = append(append(append([]byte{}, page[:i+len(open)]...), "\n"+replay.Still(still)+"\n"...), page[j:]...)
	}
	for path, b := range out {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSkeletonCatchesAChangedScreen(t *testing.T) {
	a := []string{" ▰ matchblox · ws-1  │  cpu  47% ▅▄▄▄  │  mem 26G/64G ▄▄", " BOARD 7    machine", " web-checkout  idle  4m   88%"}
	b := []string{" ▰ matchblox · ws-1  │  cpu  52% ▄▅  │  mem 27G/64G ▄▄▄▄▄", " BOARD 7    machine", " web-checkout  idle  5m   88%"}
	if skeleton(a) != skeleton(b) {
		t.Fatalf("figures and sparklines change between runs; the skeletons must match:\n%q\n%q", skeleton(a), skeleton(b))
	}
	// A figure one digit wider cuts the line one cell earlier.
	cut := []string{" ╿ done, needs you · 9m · hearth:1.…", " ╿ done, needs you · 10m · hearth:1…"}
	if skeleton(cut[:1]) != skeleton(cut[1:]) {
		t.Fatalf("a cut that moves with a figure must not change the skeleton:\n%q\n%q", skeleton(cut[:1]), skeleton(cut[1:]))
	}
	c := append([]string{}, b...)
	c[1] = " board 7    MACHINE"
	if skeleton(a) == skeleton(c) {
		t.Fatal("a renamed tab must change the skeleton")
	}
}

// demoMerge is the merge of ember-billing's ship, as the service runs it.
var demoMerge = []string{"gh", "pr", "merge", "4552", "--squash", "-R", "hearth/ember-billing"}

// demoShips is the demo machine's pull requests, as GitHub would say them:
// one in CI, one green that waits for your merge until merged says it went,
// one with a conflict, one live.
func demoShips(clk *clock, merged *atomic.Bool) service.ShipsSource {
	return func(context.Context, []sample.Session, *gitscan.Report) ships.Report {
		at := clk.Now()
		ship := func(n int, pane, name, title, kind, note string, stage int, age time.Duration) ships.Ship {
			sh := ships.Ship{Repo: "hearth/" + name, Number: n, Title: title, URL: "https://example.com/hearth/" + name + "/pull/" + fmt.Sprint(n),
				Session: ships.SessionRef{Pane: pane, Name: name}, Evidence: "created", Stage: stage, Kind: kind, Note: note, Since: at.Add(-age), Age: age}
			for i := 0; i <= stage; i++ {
				sh.Stages[i].At = at.Add(-age - time.Duration(stage-i)*25*time.Minute)
			}
			sh.Stages[stage].Note = note
			return sh
		}
		waits := ship(4552, "%8", "ember-billing", "Invoice export as CSV and PDF", ships.WaitsForYou, "green · waits for your merge", ships.StageGreen, 6*time.Minute)
		waits.Merge = demoMerge
		if merged.Load() {
			waits = ship(4552, "%8", "ember-billing", "Invoice export as CSV and PDF", ships.Running, "deploying", ships.StageMerged, time.Minute)
		}
		return ships.Report{At: at, InFlight: 3, Ships: []ships.Ship{
			ship(4551, "%1", "hearth-api", "Rate limits on the public API", ships.Running, "CI · 6 of 9 jobs", ships.StageCI, 4*time.Minute),
			waits,
			ship(4545, "%2", "flint-auth", "Refresh tokens that rotate", ships.Stuck, "conflicts with main", ships.StageCI, 40*time.Minute),
			ship(4540, "%3", "spark-docs", "The 1.0 docs", ships.Done, "live", ships.StageLive, 2*time.Hour),
		}}
	}
}
