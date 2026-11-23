//go:build linux

package app

import (
	"errors"
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"

	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

// agentDesk is a test tmux server with a session per agent, each running
// its own shell command at the region's size.
type agentDesk struct {
	t          *testing.T
	sock, path string
}

func newAgentDesk(t *testing.T, w, h int, agents map[string]string) agentDesk {
	t.Helper()
	d := agentDesk{t: t, sock: tmuxtest.Server(t)}
	d.tmux("-f", "/dev/null", "new-session", "-d", "-s", "console", "-x", "120", "-y", "30", "sleep", "600")
	for name, script := range agents {
		d.tmux("new-session", "-d", "-s", name, "-x", fmt.Sprint(w), "-y", fmt.Sprint(h), "/bin/sh", "-c", script)
	}
	d.path = d.tmux("display", "-p", "#{socket_path}")
	return d
}

func (d agentDesk) tmux(args ...string) string {
	d.t.Helper()
	cmd := exec.Command("tmux", append([]string{"-L", d.sock}, args...)...) //nolint:gosec // a test's own server
	cmd.Env = noTmuxEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		d.t.Fatalf("tmux %q: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (d agentDesk) pane(name string) string {
	return d.tmux("display", "-p", "-t", "="+name+":", "#{pane_id}")
}

// view is the argv the desk runs for name's pane: the view's nested client.
func (d agentDesk) view(name, pane string) []string {
	j := remote.Jump{Pane: pane, Prefix: "C-b", ID: remote.NewJumpID(), Outer: true}
	if !remote.ViewAllowed(j, name, d.path) {
		d.t.Fatalf("a view of %s is not allowed", name)
	}
	return remote.ViewArgv(j, name, d.path)
}

// regionRun feeds a region its frames as the console's loop would, and
// counts them.
type regionRun struct {
	t      *testing.T
	r      *agentRegion
	msgs   chan tea.Msg
	frames int
}

func runRegion(t *testing.T) *regionRun {
	x := &regionRun{t: t, r: &agentRegion{}, msgs: make(chan tea.Msg, 64)}
	t.Cleanup(x.r.shut)
	return x
}

func (x *regionRun) pump(cmd tea.Cmd) {
	if cmd != nil {
		go func() { x.msgs <- cmd() }()
	}
}

func (x *regionRun) open(name string, argv []string, w, h int) {
	x.pump(x.r.open(name, argv, w, h))
}

// until handles frames until ok holds, or fails after 10 s.
func (x *regionRun) until(what string, ok func() bool) {
	x.t.Helper()
	for end := time.Now().Add(10 * time.Second); !ok(); {
		if time.Now().After(end) {
			x.t.Fatalf("no %s; the region shows:\n%s", what, strings.Join(plainLines(x.r.lines(newStyles(true), x.r.w, x.r.h)), "\n"))
		}
		x.step(20 * time.Millisecond)
	}
}

// step handles the frames that come within d.
func (x *regionRun) step(d time.Duration) {
	timeout := time.After(d)
	for {
		select {
		case msg := <-x.msgs:
			if f, ok := msg.(agentFrameMsg); ok {
				x.frames++
				x.pump(x.r.frame(f))
			}
		case <-timeout:
			return
		}
	}
}

// typed types text into the agent as the screen's keys do, through its
// queue, if the agent has the keys.
func (x *regionRun) typed(text string) bool {
	if x.r.mode() != "AGENT" {
		return false
	}
	x.r.term.scr.(*vtScreen).e.SendText(text)
	return true
}

func (x *regionRun) shows(text string) func() bool {
	return func() bool {
		return strings.Contains(strings.Join(plainLines(x.r.lines(newStyles(true), x.r.w, x.r.h)), "\n"), text)
	}
}

// claudeScreen is Claude Code's own output in short: a box, a diff with
// true colour on padded lines, a menu, wide characters, the attributes,
// and text placed by moving the cursor.
const claudeScreen = "\x1b[38;2;215;119;87m╭───────────────────────────╮\x1b[0m\r\n" +
	"\x1b[38;2;215;119;87m│\x1b[0m \x1b[1m✻ Welcome to Claude Code!\x1b[0m \x1b[38;2;215;119;87m│\x1b[0m\r\n" +
	"\x1b[38;2;215;119;87m╰───────────────────────────╯\x1b[0m\r\n" +
	"\x1b[38;2;153;153;153m  12\x1b[0m \x1b[48;2;34;92;43m\x1b[38;2;230;255;230m+ added: 日本語のテスト         \x1b[0m\r\n" +
	"\x1b[38;2;153;153;153m  13\x1b[0m \x1b[48;2;122;41;54m- removed: 한국어            \x1b[0m\r\n" +
	" Do you want to make this edit to agentterm.go?\r\n" +
	" \x1b[38;2;177;185;249m❯ 1. Yes\x1b[0m\r\n" +
	"   2. Yes, and don't ask again \x1b[2mshift+tab\x1b[0m\r\n" +
	"   3. No, and tell Claude what to do differently \x1b[1m(esc)\x1b[0m\r\n" +
	"\x1b[3mitalic\x1b[0m \x1b[4munder\x1b[0m \x1b[7mreverse\x1b[0m \x1b[9mstruck\x1b[0m \x1b[38;5;208mindexed\x1b[0m \x1b[31mred\x1b[0m\r\n" +
	"\x1b[20;10Hmoved here\x1b[15;60H\x1b[38;2;80;200;120mand there\x1b[0m\x1b[25;3H"

// screenCells is what a screen of w by h shows after text: each cell's glyph,
// width and style, a default colour as "default".
func screenCells(text string, w, h int) [][]string {
	e := vt.NewEmulator(w, h)
	_, _ = e.WriteString(text)
	col := func(c color.Color) string {
		if c == nil {
			return "default"
		}
		r, g, b, _ := c.RGBA()
		return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
	}
	out := make([][]string, h)
	for y := range h {
		for x := range w {
			c := e.CellAt(x, y)
			out[y] = append(out[y], fmt.Sprintf("%q/%d fg=%s bg=%s attrs=%d ul=%d", c.Content, c.Width, col(c.Style.Fg), col(c.Style.Bg), c.Style.Attrs, c.Style.Underline))
		}
	}
	return out
}

func TestTheAgentScreenMatchesItsPane(t *testing.T) {
	const w, h = 100, 30
	fixture := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(fixture, []byte(claudeScreen), 0o600); err != nil {
		t.Fatal(err)
	}
	d := newAgentDesk(t, w, h, map[string]string{"flint-auth": "cat " + fixture + "; exec sleep 600"})
	pane := d.pane("flint-auth")
	x := runRegion(t)
	x.open("flint-auth", d.view("flint-auth", pane), w, h)
	x.until("fixture", x.shows("and there"))
	x.step(300 * time.Millisecond) // the client's last bytes

	// capture-pane's lines end in "\n"; a screen moves down a line with "\r\n".
	captured := strings.ReplaceAll(strings.TrimSuffix(d.tmux("capture-pane", "-e", "-p", "-t", pane), "\n"), "\n", "\r\n")
	want := screenCells(captured, w, h)
	got := screenCells(strings.Join(x.r.term.draw(nil, nil, w, h), "\r\n"), w, h)
	diffs := 0
	for y := range h {
		for c := range w {
			if got[y][c] != want[y][c] && diffs < 10 {
				diffs++
				t.Errorf("row %d col %d: the region has %s, the pane %s", y, c, got[y][c], want[y][c])
			}
		}
	}
	for _, sure := range []string{`"日"/2 fg=#E6FFE6 bg=#225C2B`, `"m"/1 fg=default bg=default attrs=0`, `"a"/1 fg=#50C878`} {
		if !slices.ContainsFunc(want, func(row []string) bool {
			return slices.ContainsFunc(row, func(c string) bool { return strings.HasPrefix(c, sure) })
		}) {
			t.Errorf("the pane has no cell %s: the fixture did not reach it", sure)
		}
	}
}

func TestASwitchNeverShowsTheOldAgent(t *testing.T) {
	const w, h = 80, 20
	d := newAgentDesk(t, w, h, map[string]string{
		"flint-auth": "printf 'FLINT SCREEN\\n'; exec cat",
		"ux-pass":    "printf 'ux-pass ready\\n'; exec cat",
	})
	flint, ux := d.pane("flint-auth"), d.pane("ux-pass")
	x := runRegion(t)
	x.open("flint-auth", d.view("flint-auth", flint), w, h)
	x.until("flint-auth's screen", x.shows("FLINT SCREEN"))
	x.r.keys = true
	if !x.typed("before\r") {
		t.Fatal("with the agent's keys a key does not reach flint-auth")
	}
	x.until("the key in flint-auth", func() bool { return strings.Contains(d.tmux("capture-pane", "-p", "-t", flint), "before") })

	x.open("ux-pass", d.view("ux-pass", ux), w, h)
	lines := x.r.lines(newStyles(true), w, h)
	plain := strings.Join(plainLines(lines), "\n")
	if strings.Contains(plain, "FLINT") || strings.Contains(plain, "before") {
		t.Fatalf("after the switch the region shows flint-auth:\n%s", plain)
	}
	opening := lineWith(lines, "opening ux-pass…")
	if opening == "" {
		t.Fatalf("after the switch the region does not say opening ux-pass…:\n%s", plain)
	}
	wantInks(t, opening, [][2]string{{"opening ux-pass…", "faint"}})
	if mode := x.r.mode(); mode != "LIST" {
		t.Fatalf("after the switch the keys are %s, want LIST", mode)
	}
	if x.typed("meanwhile\r") {
		t.Fatal("a key typed on the switch was sent")
	}
	x.until("ux-pass's screen", x.shows("ux-pass ready"))
	if plain := strings.Join(plainLines(x.r.lines(newStyles(true), w, h)), "\n"); strings.Contains(plain, "FLINT") {
		t.Fatalf("ux-pass's screen shows flint-auth:\n%s", plain)
	}
	if mode := x.r.mode(); mode != "LIST" {
		t.Fatalf("ux-pass's first frame gave it the keys: %s", mode)
	}
	x.step(300 * time.Millisecond)
	for _, p := range []string{flint, ux} {
		if got := d.tmux("capture-pane", "-p", "-t", p); strings.Contains(got, "meanwhile") {
			t.Fatalf("the key typed on the switch reached %s:\n%s", p, got)
		}
	}
}

// TestAFloodNeverSlowsADraw: while the agent prints as fast as it can, a
// draw still takes less than a frame at 60 fps.
func TestAFloodNeverSlowsADraw(t *testing.T) {
	if raceBuild {
		t.Skip("latency is measured without -race: the race detector's instrumentation dominates the time")
	}
	const w, h = 160, 48
	x := runRegion(t)
	x.open("flood", []string{"/bin/sh", "-c", `printf 'ready\n'; exec yes '` + "\x1b[31m" + strings.Repeat("x", 100) + `'`}, w, h)
	x.until("the flood", x.shows(strings.Repeat("x", 10)))
	for i := range 100 {
		start := time.Now()
		_ = x.r.lines(newStyles(true), w, h)
		if took := time.Since(start); took > 20*time.Millisecond {
			t.Fatalf("draw %d took %v during a flood, want under 20ms", i, took)
		}
	}
}

// TestTheRegionSaysWhyAViewFailed: a view that fails draws today's
// view-failure words (design review D6).
func TestTheRegionSaysWhyAViewFailed(t *testing.T) {
	d := newAgentDesk(t, 80, 20, map[string]string{})
	x := runRegion(t)
	x.open("flint-auth", d.view("flint-auth", "%999"), 80, 20)
	x.until("the view's failure", x.shows("flint-auth cannot be shown: can't find pane: %999."))
}

// TestTheAgentScreenClosesAClientThatIgnoresAHangUp: close kills what a
// hang-up leaves, and returns.
func TestTheAgentScreenClosesAClientThatIgnoresAHangUp(t *testing.T) {
	x := runRegion(t)
	x.open("stubborn", []string{"/bin/sh", "-c", "trap '' HUP; echo ready; while :; do sleep 0.1; done"}, 40, 5)
	x.until("the client", x.shows("ready"))
	pid := x.r.term.cmd.Process.Pid
	done := make(chan struct{})
	go func() { x.r.shut(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("close waits for a client that ignores the hang-up")
	}
	// The killed children go to init, which reaps them a moment later.
	for end := time.Now().Add(time.Second); !errors.Is(syscall.Kill(-pid, 0), syscall.ESRCH); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("the client's group is left")
		}
	}
}

func TestTheAgentScreenFollowsAResize(t *testing.T) {
	// The list takes 40 of 160 columns and 32 of 132, the edge 1 (D16).
	const wide, narrow, h = 119, 99, 30
	d := newAgentDesk(t, wide, h, map[string]string{
		"flint-auth": `show() { printf '\033[H\033[2Jcolumns %s\n' "$(tput cols)"; }; trap show WINCH; show; while :; do sleep 600 & wait; done`,
	})
	x := runRegion(t)
	x.open("flint-auth", d.view("flint-auth", d.pane("flint-auth")), wide, h)
	x.until("the agent at 119 columns", x.shows(fmt.Sprintf("columns %d", wide)))
	x.r.resize(narrow, h)
	x.until("the agent at 99 columns", x.shows(fmt.Sprintf("columns %d", narrow)))
	if got := d.tmux("display", "-p", "-t", d.pane("flint-auth"), "#{window_width}x#{window_height}"); got != fmt.Sprintf("%dx%d", narrow, h) {
		t.Fatalf("the agent's window is %s, want %dx%d", got, narrow, h)
	}
	for _, l := range x.r.lines(newStyles(true), narrow, h) {
		if lipgloss.Width(l) != narrow {
			t.Fatalf("a line of the region is %d wide, want %d: %q", lipgloss.Width(l), narrow, ansi.Strip(l))
		}
	}
}

func TestTheAgentScreenEndsWithWords(t *testing.T) {
	d := newAgentDesk(t, 80, 20, map[string]string{"flint-auth": "printf 'FLINT SCREEN\\n'; exec cat"})
	pane := d.pane("flint-auth")
	x := runRegion(t)
	x.open("flint-auth", d.view("flint-auth", pane), 80, 20)
	x.until("flint-auth's screen", x.shows("FLINT SCREEN"))
	d.tmux("send-keys", "-t", pane, "C-d")
	x.until("the end's words", x.shows("flint-auth ended. ctrl+q: back to the list"))
}

func TestAnIdleAgentCostsNoRedraw(t *testing.T) {
	d := newAgentDesk(t, 80, 20, map[string]string{"flint-auth": "printf 'FLINT SCREEN\\n'; exec sleep 600"})
	x := runRegion(t)
	x.open("flint-auth", d.view("flint-auth", d.pane("flint-auth")), 80, 20)
	x.until("flint-auth's screen", x.shows("FLINT SCREEN"))
	x.step(500 * time.Millisecond) // the client's last bytes
	before := x.frames
	x.step(2 * time.Second)
	if n := x.frames - before; n != 0 {
		t.Fatalf("2 s with no output drew the region %d times", n)
	}
	d.tmux("send-keys", "-t", d.pane("flint-auth"), "-l", "x") // sleep echoes nothing, but the pane shows the x
	x.until("the next frame", func() bool { return x.frames > before })
}
