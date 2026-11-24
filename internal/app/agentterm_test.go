//go:build linux

package app

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image/color"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"

	"github.com/blox-eng/matchblox/internal/proto"
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
		d.agent(name, script, w, h)
	}
	d.path = d.tmux("display", "-p", "#{socket_path}")
	return d
}

// agent starts a session that runs script at w by h.
func (d agentDesk) agent(name, script string, w, h int) {
	d.tmux("new-session", "-d", "-s", name, "-x", fmt.Sprint(w), "-y", fmt.Sprint(h), "/bin/sh", "-c", script)
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
	m      *Model // the console the region is in, if any: it takes the input and the frames
	msgs   chan tea.Msg
	frames int
}

func runRegion(t *testing.T) *regionRun {
	x := &regionRun{t: t, r: &agentRegion{}, msgs: make(chan tea.Msg, 64)}
	t.Cleanup(x.r.shut)
	return x
}

// runDesk is a region inside a console of st.
func runDesk(t *testing.T, st proto.State) *regionRun {
	m := boardModel(t, st)
	m.opt.Run = func([]string) error { return nil }
	x := &regionRun{t: t, m: &m, r: &m.agent, msgs: make(chan tea.Msg, 64)}
	t.Cleanup(x.r.shut)
	return x
}

func (x *regionRun) pump(cmd tea.Cmd) {
	if cmd != nil {
		go func() {
			msg := cmd()
			if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeFor[tea.Cmd]() { // a batch or a sequence
				for i := range v.Len() {
					x.pump(v.Index(i).Interface().(tea.Cmd))
				}
				return
			}
			x.msgs <- msg
		}()
	}
}

// update is msg through the console.
func (x *regionRun) update(msg tea.Msg) {
	next, cmd := x.m.Update(msg)
	*x.m = next.(Model)
	x.pump(cmd)
}

func (x *regionRun) press(k string) { x.update(keyPress(k)) }

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
				if x.m != nil {
					x.update(f)
				} else {
					next, _ := x.r.frame(f)
					x.pump(next)
				}
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
	got := screenCells(strings.Join(x.r.term.draw(nil, nil, w, h, nil), "\r\n"), w, h)
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

// hostile is what an agent, or a file it prints, sends to reach the
// owner's terminal through matchblox's.
const hostile = "\x1b]0;pwned\x07" + // the title
	"\x1b]52;c;cHduZWQ=\x07" + // the clipboard: "pwned"
	"\x1b[6n" + "\x1b[c" + // queries whose answers would type into the agent
	"\x1bPtmux;\x1b\x1b]0;pwned\x07\x1b\\" + // tmux's passthrough
	"\x1b]7;file://pwned/tmp\x07" + // the working directory
	"\x1b]1337;SetUserVar=pwned=cHduZWQ=\x07" + // iTerm2's own
	"\x07" + // the bell
	"\u009b2J" + "\x9b2J" + // C1's CSI, as UTF-8 and as a raw byte
	"\x0e" + "\u202e" + // shift out, and a right-to-left override
	"\x1b[21t\x1b[14t\x1b[8;5;5t" + // the window's title and size, asked and set
	"\x1b]10;?\x07\x1b]11;?\x07\x1b]4;1;?\x07" + // colour queries
	"\x1bP$qm\x1b\\" + "\x1bP+q544e\x1b\\" + // DECRQSS and XTGETTCAP
	"\x1b_Ga=T;AAAA\x1b\\" + // the kitty graphics protocol
	"\x1b]52;c;?\x07" + // a clipboard read
	"\x1b]8;;https://pwned.example/\x07a link\x1b]8;;\x07" // a hyperlink, after the clears

// sgr is the only sequence matchblox draws the agent's screen with.
var onlySGR = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

func TestAnAgentCannotTouchTheOwnersTerminal(t *testing.T) {
	const w, h = 80, 20
	fixture := filepath.Join(t.TempDir(), "hostile")
	if err := os.WriteFile(fixture, []byte(hostile), 0o600); err != nil {
		t.Fatal(err)
	}
	clean := func(t *testing.T, x *regionRun) {
		t.Helper()
		drawn := strings.Join(x.r.lines(newStyles(true), w, h), "\n")
		rest := onlySGR.ReplaceAllString(drawn, "")
		if strings.Contains(rest, "pwned") || strings.ContainsFunc(rest, func(r rune) bool { return !unicode.IsPrint(r) && r != ' ' && r != '\n' }) {
			t.Fatalf("matchblox writes the agent's sequences to its terminal: %q", rest)
		}
	}

	// The client itself is hostile, as an agent's tmux that passes all through.
	t.Run("from the client", func(t *testing.T) {
		x := runRegion(t)
		x.open("flint-auth", []string{"/bin/sh", "-c", `stty raw -echo; printf '\033[H'; cat "$1"; ` +
			`stty min 0 time 5; printf '\r\nanswers: '; dd bs=1 count=4096 2>/dev/null | tr '\033' '~'; printf '\r\nend'; exec sleep 600`, "sh", fixture}, w, h)
		x.until("the emulator's answers", x.shows("end"))
		if !x.shows("~[1;1R~[?62;1;6;22c")() {
			t.Fatalf("the queries' answers are not the emulator's:\n%s", strings.Join(plainLines(x.r.lines(newStyles(true), w, h)), "\n"))
		}
		clean(t, x)
		if x.r.copy != "pwned" {
			t.Fatalf("the agent asked to copy %q, the region keeps %q", "pwned", x.r.copy)
		}
	})

	t.Run("through the agent's tmux", func(t *testing.T) {
		d := newAgentDesk(t, w, h, map[string]string{"flint-auth": `cat "` + fixture + `"; printf '\nend\n'; exec sleep 600`})
		x := runRegion(t)
		x.open("flint-auth", d.view("flint-auth", d.pane("flint-auth")), w, h)
		x.until("the agent's output", x.shows("end"))
		x.step(300 * time.Millisecond) // the client's last bytes
		clean(t, x)
	})

	t.Run("the list foot says the copy once", func(t *testing.T) {
		const asked = "the agent asked to copy: c copies it"
		m := boardModel(t, boardState())
		term := &agentTerm{scr: newAgentScreen(w, h)}
		m.agent.term = term
		for i, want := range []string{asked, ""} {
			_, _ = term.scr.Write([]byte("\x1b]52;c;cHduZWQ=\x07"))
			m.noticeText, m.saying = "", nil
			m = upd(t, m, agentFrameMsg{t: term})
			if got := m.sideSays(); got != want {
				t.Fatalf("copy request %d: the list foot says %q, want %q", i+1, got, want)
			}
		}
		if m.agent.copy != "pwned" {
			t.Fatalf("the region keeps %q to copy, want pwned", m.agent.copy)
		}
	})
}

// emulatorAnswers are the only bytes the screen may send the agent back: a
// cursor's place, the device's attributes and status, and a colour.
var emulatorAnswers = regexp.MustCompile(`^(\x1b\[[0-9;?]*[Rcny]|\x1b\]1[0-2];rgb:[0-9a-f/]+\x07)*$`)

func TestTheAgentScreenAnswersOnlyWhatItMay(t *testing.T) {
	scr := newAgentScreen(80, 20)
	read := make(chan []byte)
	go func() {
		buf := make([]byte, 4<<10)
		for {
			n, err := scr.Read(buf)
			if err != nil {
				close(read)
				return
			}
			read <- slices.Clone(buf[:n])
		}
	}()
	go func() { _, _ = scr.Write([]byte(hostile)) }()
	var got []byte
	for quiet := time.After(500 * time.Millisecond); ; {
		select {
		case b := <-read:
			got = append(got, b...)
			continue
		case <-quiet:
		}
		break
	}
	_ = scr.Close()
	if len(got) == 0 || !emulatorAnswers.Match(got) {
		t.Fatalf("the screen sends the agent %q", got)
	}
}

// rawAgent keeps every byte it reads in file, after it prints say and asks
// its terminal for the modes in asks.
func rawAgent(file, say, asks string) string {
	return `printf '` + say + `'; stty raw -echo; printf '` + asks + `'; exec cat > ` + file
}

func readsOf(t *testing.T, file string) string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(b)
}

// reading waits until pane's agent reads its terminal.
func (d agentDesk) reading(pane string) {
	d.t.Helper()
	for end := time.Now().Add(5 * time.Second); d.tmux("display", "-p", "-t", pane, "#{pane_current_command}") != "cat"; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(end) {
			d.t.Fatalf("the agent in %s does not read", pane)
		}
	}
}

// agentKeys opens name's screen in a console and gives it the keys.
func agentKeys(t *testing.T, d agentDesk, st proto.State, name string, w, h int) *regionRun {
	t.Helper()
	x := runDesk(t, st)
	x.pump(x.r.open(name, d.view(name, d.pane(name)), w, h))
	x.until(name+"'s first frame", func() bool { return x.r.term != nil && x.r.term.framed })
	x.press(flipKey)
	if mode := x.r.mode(); mode != "AGENT" {
		t.Fatalf("ctrl+q gave the keys to %s, want AGENT", mode)
	}
	return x
}

// TestASwitchLeavesAStalledAgent: an agent that reads nothing, with a
// paste queued for it, never holds up a switch, and ctrl+q still flips
// the keys after it.
func TestASwitchLeavesAStalledAgent(t *testing.T) {
	const w, h = 80, 20
	x := runDesk(t, boardState())
	x.pump(x.r.open("stalled", []string{"/bin/sh", "-c", `stty raw -echo; printf 'ready\n'; exec sleep 600`}, w, h))
	x.until("the stalled agent", x.shows("ready"))
	if x.press(flipKey); x.r.mode() != "AGENT" {
		t.Fatalf("ctrl+q gave the keys to %s, want AGENT", x.r.mode())
	}
	x.update(tea.PasteMsg{Content: strings.Repeat("p", 64<<10)})
	x.step(200 * time.Millisecond) // the writer fills the pty and waits

	opened := make(chan tea.Cmd, 1)
	go func() {
		opened <- x.r.open("ready", []string{"/bin/sh", "-c", `printf 'other ready\n'; exec sleep 600`}, w, h)
	}()
	select {
	case cmd := <-opened:
		x.pump(cmd)
	case <-time.After(2 * time.Second):
		t.Fatal("a switch away from a stalled agent waits for it")
	}
	x.until("the other agent", x.shows("other ready"))
	if x.press(flipKey); x.r.mode() != "AGENT" {
		t.Fatalf("after the switch ctrl+q gave the keys to %s, want AGENT", x.r.mode())
	}
	if x.press(flipKey); x.r.mode() != "LIST" {
		t.Fatalf("after the switch ctrl+q left the keys with %s, want LIST", x.r.mode())
	}
}

func TestKeysReachTheAgent(t *testing.T) {
	const w, h = 80, 20
	dir := t.TempDir()
	d := newAgentDesk(t, w, h, nil)
	d.tmux("set-option", "-s", "extended-keys", "on")
	for _, name := range []string{"desk", "own"} { // each asks for the keys a plain terminal cannot tell apart, as Claude Code does
		d.agent(name, rawAgent(filepath.Join(dir, name), "", `\033[>4;1m`), w, h)
	}
	desk, own := d.pane("desk"), d.pane("own")
	d.reading(desk)
	d.reading(own)
	x := agentKeys(t, d, boardState(), "desk", w, h)
	keys := []struct {
		k    tea.KeyPressMsg
		tmux string
	}{
		{tea.KeyPressMsg{Code: 'l', Text: "l"}, "l"},
		{tea.KeyPressMsg{Code: 's', Text: "s"}, "s"},
		{tea.KeyPressMsg{Code: tea.KeyEnter}, "Enter"},
		{tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, "C-c"},
		{tea.KeyPressMsg{Code: tea.KeyEscape}, "Escape"},
		{tea.KeyPressMsg{Code: tea.KeyUp}, "Up"},
		{tea.KeyPressMsg{Code: tea.KeyDown}, "Down"},
		{tea.KeyPressMsg{Code: tea.KeyRight}, "Right"},
		{tea.KeyPressMsg{Code: tea.KeyLeft}, "Left"},
		{tea.KeyPressMsg{Code: 'b', Mod: tea.ModAlt}, "M-b"},
		// What the kitty protocol tells apart, as bubbletea decodes it.
		{tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}, "S-Enter"},
		{tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModCtrl}, "C-Left"},
	}
	// One at a time, as a person types: tmux reads ESC and a key after it
	// at once as alt and the key.
	grew := func(file string, n int) bool { return len(readsOf(t, filepath.Join(dir, file))) > n }
	for _, k := range keys {
		n := len(readsOf(t, filepath.Join(dir, "desk")))
		x.update(k.k)
		x.until(k.tmux+" in the agent", func() bool { return grew("desk", n) })
		n = len(readsOf(t, filepath.Join(dir, "own")))
		d.tmux("send-keys", "-t", own, k.tmux)
		x.until(k.tmux+" in its own pane", func() bool { return grew("own", n) })
	}
	x.step(100 * time.Millisecond)
	if got, want := readsOf(t, filepath.Join(dir, "desk")), readsOf(t, filepath.Join(dir, "own")); got != want {
		t.Fatalf("the agent read %q through matchblox, its own pane %q", got, want)
	}
}

func TestAPasteReachesTheAgentWhole(t *testing.T) {
	const w, h = 80, 20
	file := filepath.Join(t.TempDir(), "read")
	d := newAgentDesk(t, w, h, map[string]string{"flint-auth": rawAgent(file, "", `\033[?2004h`)})
	d.reading(d.pane("flint-auth"))
	x := agentKeys(t, d, boardState(), "flint-auth", w, h)
	var b strings.Builder
	for i := 0; b.Len() < 20<<10; i++ {
		fmt.Fprintf(&b, "line %d of a paste: func main() { fmt.Println(\"%d\") }\n", i, i)
	}
	pasted := b.String()
	x.update(tea.PasteMsg{Content: pasted})
	want := "\x1b[200~" + pasted + "\x1b[201~"
	x.until("the whole paste", func() bool { return len(readsOf(t, file)) >= len(want) })
	x.step(100 * time.Millisecond)
	if got := readsOf(t, file); got != want {
		t.Fatalf("the agent read %d bytes, %d starts and %d ends of a paste; want %d bytes in one bracketed paste:\n%q…",
			len(got), strings.Count(got, "\x1b[200~"), strings.Count(got, "\x1b[201~"), len(want), got[:min(len(got), 200)])
	}
}

func TestCtrlQFlips(t *testing.T) {
	const w, h = 80, 20
	file := filepath.Join(t.TempDir(), "read")
	d := newAgentDesk(t, w, h, map[string]string{"flint-auth": rawAgent(file, `FLINT SCREEN\n`, "")})
	x := runDesk(t, boardState(doneMatch("flint-auth", 10*time.Minute)))
	x.pump(x.r.open("flint-auth", d.view("flint-auth", d.pane("flint-auth")), w, h))
	x.until("flint-auth's screen", x.shows("FLINT SCREEN"))
	d.reading(d.pane("flint-auth"))
	bar := func(want string, away bool) {
		t.Helper()
		line, _ := x.m.frameBar(x.r.mode(), 160)
		plain := ansi.Strip(line)
		if !strings.HasPrefix(strings.TrimSpace(plain), want) || strings.Contains(plain, "ctrl+q list") != away {
			t.Fatalf("the bar reads %q, want %s first and ctrl+q list %v", plain, want, away)
		}
	}
	bar("LIST", false)
	x.press(flipKey)
	bar("AGENT", true)
	x.until("the session seen", func() bool {
		return slices.Equal(seenActs(boardFake(*x.m)), []string{proto.SeenPrefix + "%flint-auth"})
	})
	x.press(flipKey)
	bar("LIST", false)
	x.step(200 * time.Millisecond)
	if got := readsOf(t, file); got != "" {
		t.Fatalf("ctrl+q reached the agent: %q", got)
	}
}

// TestCtrlOStillGoesIn: with the agent's screen on the desk, ⌃o from the
// list still runs the jump into its own pane, and ctrl+a m (the home key)
// gives the keys back to the list; neither types into the agent.
func TestCtrlOStillGoesIn(t *testing.T) {
	const w, h = 80, 20
	file := filepath.Join(t.TempDir(), "read")
	d := newAgentDesk(t, w, h, map[string]string{"flint-auth": rawAgent(file, `FLINT SCREEN\n`, "")})
	x := runDesk(t, boardState(doneMatch("flint-auth", 10*time.Minute)))
	x.pump(x.r.open("flint-auth", d.view("flint-auth", d.pane("flint-auth")), w, h))
	x.until("flint-auth's screen", x.shows("FLINT SCREEN"))
	d.reading(d.pane("flint-auth"))

	next, cmd := x.m.Update(keyPress("ctrl+o"))
	*x.m = next.(Model)
	msgs := cmdMsgs(cmd)
	if !slices.ContainsFunc(msgs, func(msg tea.Msg) bool {
		r, ok := msg.(ranMsg)
		return ok && r.cmd == "tmux switch-client -t %flint-auth"
	}) {
		t.Fatalf("ctrl+o sent %#v, want the jump into %%flint-auth", msgs)
	}

	if x.press(flipKey); x.r.mode() != "AGENT" {
		t.Fatalf("ctrl+q gave the keys to %s, want AGENT", x.r.mode())
	}
	x.update(uv.UnknownApcEvent(homeKey))
	if mode := x.r.mode(); mode != "LIST" {
		t.Fatalf("after ctrl+a m the keys are %s, want LIST", mode)
	}
	x.step(200 * time.Millisecond)
	if got := readsOf(t, file); got != "" {
		t.Fatalf("ctrl+o or ctrl+a m reached the agent: %q", got)
	}
}

func TestAClickGivesTheKeys(t *testing.T) {
	const w, h = 80, 20
	file := filepath.Join(t.TempDir(), "read")
	d := newAgentDesk(t, w, h, map[string]string{"flint-auth": rawAgent(file, `CLICK ME\n`, `\033[?1000h\033[?1006h`)})
	x := runDesk(t, boardState(doneMatch("flint-auth", 10*time.Minute)))
	x.pump(x.r.open("flint-auth", d.view("flint-auth", d.pane("flint-auth")), w, h))
	x.r.x, x.r.y = 41, 1 // the region beside the list, under the head row
	x.until("flint-auth's screen", x.shows("CLICK ME"))
	d.reading(d.pane("flint-auth"))

	x.update(tea.MouseClickMsg{X: 40, Y: 4, Button: tea.MouseLeft})
	if mode := x.r.mode(); mode != "LIST" {
		t.Fatalf("a click left of the region gave the keys to %s", mode)
	}
	x.update(tea.MouseClickMsg{X: 41 + 10, Y: 1 + 3, Button: tea.MouseLeft})
	x.update(tea.MouseReleaseMsg{X: 41 + 10, Y: 1 + 3, Button: tea.MouseLeft})
	if mode := x.r.mode(); mode != "AGENT" {
		t.Fatalf("a click in the region gave the keys to %s, want AGENT", mode)
	}
	const want = "\x1b[<0;11;4M\x1b[<0;11;4m" // column 11, row 4 of the agent's screen
	x.until("the click in the agent", func() bool { return len(readsOf(t, file)) >= len(want) })
	if got := readsOf(t, file); got != want {
		t.Fatalf("the agent read %q, want the click %q", got, want)
	}
	x.until("the session seen", func() bool {
		return slices.Equal(seenActs(boardFake(*x.m)), []string{proto.SeenPrefix + "%flint-auth"})
	})
}

// TestTypingIsFast types 1000 keys into an agent that echoes them, one
// every 25 ms (twice a fast typist's pace), and times each from the
// console's input to the frame that draws its echo (the spec's bar).
func TestTypingIsFast(t *testing.T) {
	if raceBuild {
		t.Skip("latency is measured without -race: the race detector's instrumentation dominates the time")
	}
	const w, h, keys = 100, 20, 1000
	const typed = "abcdefghijklmnopqrstuvwxyz" // no space: a blank cell shows no echo
	d := newAgentDesk(t, w, h, map[string]string{"flint-auth": `printf '\033[H\033[2J'; exec cat`})
	d.reading(d.pane("flint-auth"))
	x := agentKeys(t, d, boardState(), "flint-auth", w, h)
	x.step(300 * time.Millisecond) // the client's last bytes

	sent := make([]time.Time, 0, keys)
	took := make([]time.Duration, 0, keys)
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	deadline := time.After(2 * time.Minute)
	for len(took) < keys {
		select {
		case <-tick.C:
			if len(sent) < keys {
				r := rune(typed[len(sent)%len(typed)])
				sent = append(sent, time.Now())
				x.update(tea.KeyPressMsg{Code: r, Text: string(r)})
			}
		case msg := <-x.msgs:
			f, ok := msg.(agentFrameMsg)
			if !ok {
				continue
			}
			x.update(f)
			lines := plainLines(x.r.lines(x.m.st, w, h))
			for i := len(took); i < len(sent); i++ {
				if row := lines[i/w]; len(row) <= i%w || row[i%w] != typed[i%len(typed)] {
					break
				}
				took = append(took, time.Since(sent[i]))
			}
		case <-deadline:
			t.Fatalf("%d of %d keys echoed in 2 minutes", len(took), keys)
		}
	}
	slices.Sort(took)
	p50, p95 := took[len(took)/2], took[len(took)*95/100]
	t.Logf("%d keys: echo drawn p50 %v, p95 %v, max %v", keys, p50, p95, took[len(took)-1])
	if p95 > 16*time.Millisecond {
		t.Fatalf("a key's echo takes %v at the 95th percentile, want 16ms or less", p95)
	}
}

func TestTheAgentsCursorShows(t *testing.T) {
	const w, h = 80, 20
	d := newAgentDesk(t, w, h, map[string]string{"flint-auth": `printf 'prompt> \033[6;13H'; exec cat`})
	pane := d.pane("flint-auth")
	d.reading(pane)
	x := runDesk(t, boardState())
	x.pump(x.r.open("flint-auth", d.view("flint-auth", pane), w, h))
	x.r.x, x.r.y = 41, 1
	x.until("flint-auth's screen", x.shows("prompt>"))
	x.step(300 * time.Millisecond) // the client's last bytes
	if c := x.m.View().Cursor; c != nil {
		t.Fatalf("with the list's keys the agent's cursor shows at %v", c.Position)
	}
	x.press(flipKey)
	var cx, cy int
	if _, err := fmt.Sscanf(d.tmux("display", "-p", "-t", pane, "#{cursor_x} #{cursor_y}"), "%d %d", &cx, &cy); err != nil {
		t.Fatal(err)
	}
	c := x.m.View().Cursor
	if c == nil || c.X != 41+cx || c.Y != 1+cy {
		t.Fatalf("with the agent's keys the cursor is %v, want the pane's %d,%d at the region's 41,1", c, cx, cy)
	}
}

// TestAPasteReachesAStalledAgentInOrder: a client that reads nothing for
// a while never holds up the console, and what is typed after a paste
// reaches the agent after it.
func TestAPasteReachesAStalledAgentInOrder(t *testing.T) {
	file := filepath.Join(t.TempDir(), "read")
	x := runDesk(t, boardState())
	x.pump(x.r.open("stalled", []string{"/bin/sh", "-c", `stty raw -echo; printf 'ready\n'; sleep 2; exec cat > "$1"`, "sh", file}, 80, 20))
	x.until("the agent", x.shows("ready"))
	if x.press(flipKey); x.r.mode() != "AGENT" {
		t.Fatalf("ctrl+q gave the keys to %s, want AGENT", x.r.mode())
	}
	pasted := strings.Repeat("0123456789abcdef", 1<<16) // 1 MB
	for _, msg := range []tea.Msg{tea.PasteMsg{Content: pasted}, keyPress("x"), keyPress("y"), keyPress("z")} {
		start := time.Now()
		x.update(msg)
		if took := time.Since(start); took > 50*time.Millisecond {
			t.Fatalf("%T took %v in the console while the agent reads nothing, want under 50ms", msg, took)
		}
	}
	want := pasted + "xyz"
	x.until("the paste and the keys", func() bool { return len(readsOf(t, file)) >= len(want) })
	if got := readsOf(t, file); got != want {
		t.Fatalf("the agent read %d bytes ending %q, want %d ending %q", len(got), got[max(len(got)-8, 0):], len(want), want[len(want)-8:])
	}
}

// TestKeysReachTheAgentButNotAQuestion: the scan question's y and a
// blocked host's ⏎ stay with the box that asks, though the agent has the keys.
func TestKeysReachTheAgentButNotAQuestion(t *testing.T) {
	const w, h = 80, 20
	file := filepath.Join(t.TempDir(), "read")
	d := newAgentDesk(t, w, h, map[string]string{"flint-auth": rawAgent(file, "", "")})
	d.reading(d.pane("flint-auth"))
	for _, c := range []struct {
		layer layerID
		key   string
		ask   func(*Model)
	}{
		{layerScan, "y", func(m *Model) { m.scanAsk = &scanAsk{} }},
		{layerBlocked, "enter", func(m *Model) { m.blocked = errors.New("unreachable") }},
	} {
		x := agentKeys(t, d, boardState(), "flint-auth", w, h)
		c.ask(x.m)
		if f := x.m.focus(keyPress(c.key)); f != c.layer {
			t.Fatalf("the question is not up: %s goes to layer %d, want %d", c.key, f, c.layer)
		}
		x.press(c.key)
		x.r.shut()
	}
	time.Sleep(200 * time.Millisecond)
	if got := readsOf(t, file); got != "" {
		t.Fatalf("a question's answer reached the agent: %q", got)
	}
}

// regionProbe is the console with its agent region alone on the screen,
// at the screen's top left: the desk's frame is not drawn yet. The
// screen is wider than the region, as on the desk.
type regionProbe struct {
	Model
	first tea.Cmd
}

func (p regionProbe) Init() tea.Cmd { return p.first }

func (p regionProbe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := p.Model.Update(msg)
	return regionProbe{Model: next.(Model)}, cmd
}

func (p regionProbe) View() tea.View {
	v := tea.NewView(strings.Join(p.agent.lines(p.st, p.agent.w, p.agent.h), "\n"))
	v.AltScreen = true
	v.Cursor = p.agent.cursor()
	return v
}

// screenWatch is the terminal the console's program writes to: a screen
// that calls drawn after each write. Its tty maps a newline to a return
// and a newline, as the program expects of output that is not a terminal.
type screenWatch struct {
	mu    sync.Mutex
	e     *vt.Emulator
	drawn func(*vt.Emulator)
}

func (s *screenWatch) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.e.Write(bytes.ReplaceAll(p, []byte("\n"), []byte("\r\n")))
	s.drawn(s.e)
	return len(p), nil
}

// TestTypingIsFastThroughTheRenderer is TestTypingIsFast through the
// console's own program and renderer, at its frame rate: each key from
// the program's input to its echo in the bytes it writes to the terminal.
// The keys come 20 to 30 ms apart, as a person's do, never in step with
// the renderer's frames.
func TestTypingIsFastThroughTheRenderer(t *testing.T) {
	if raceBuild {
		t.Skip("latency is measured without -race: the race detector's instrumentation dominates the time")
	}
	const w, h, keys = 100, 20, 1000
	const typed = "abcdefghijklmnopqrstuvwxyz"
	d := newAgentDesk(t, w, h, map[string]string{"flint-auth": `printf '\033[H\033[2J'; exec cat`})
	d.reading(d.pane("flint-auth"))
	m := boardModel(t, boardState())
	first := m.agent.open("flint-auth", d.view("flint-auth", d.pane("flint-auth")), w, h)
	t.Cleanup(m.agent.shut)

	var mu sync.Mutex
	sent := make([]time.Time, 0, keys)
	took := make([]time.Duration, 0, keys)
	screen := &screenWatch{e: vt.NewEmulator(w+20, h), drawn: func(e *vt.Emulator) {
		mu.Lock()
		defer mu.Unlock()
		for i := len(took); i < len(sent); i++ {
			if c := e.CellAt(i%w, i/w); c == nil || c.Content != typed[i%len(typed):i%len(typed)+1] {
				break
			}
			took = append(took, time.Since(sent[i]))
		}
	}}
	go func() { _, _ = io.Copy(io.Discard, screen.e) }() // the screen's answers to the program
	p := tea.NewProgram(regionProbe{Model: m, first: first}, tea.WithFPS(ConsoleFPS), tea.WithInput(nil),
		tea.WithOutput(screen), tea.WithWindowSize(w+20, h), tea.WithoutSignalHandler())
	ran := make(chan error, 1)
	go func() { _, err := p.Run(); ran <- err }()
	t.Cleanup(func() { p.Kill(); <-ran })
	time.Sleep(time.Second) // the agent's first frame
	p.Send(keyPress(flipKey))
	time.Sleep(100 * time.Millisecond)

	deadline := time.After(2 * time.Minute)
	for n := 0; ; {
		select {
		case <-time.After(20*time.Millisecond + rand.N(10*time.Millisecond)):
		case <-deadline:
			screen.mu.Lock()
			shown := screen.e.String()
			screen.mu.Unlock()
			t.Fatalf("%d of %d keys echoed on the screen in 2 minutes; the screen shows:\n%s", n, keys, shown)
		}
		mu.Lock()
		n = len(took)
		if len(sent) < keys {
			sent = append(sent, time.Now())
			r := rune(typed[(len(sent)-1)%len(typed)])
			mu.Unlock()
			p.Send(tea.KeyPressMsg{Code: r, Text: string(r)})
			continue
		}
		mu.Unlock()
		if n == keys {
			break
		}
	}
	slices.Sort(took)
	p50, p95 := took[len(took)/2], took[len(took)*95/100]
	t.Logf("%d keys: echo on the screen p50 %v, p95 %v, max %v", keys, p50, p95, took[len(took)-1])
	if p95 > 16*time.Millisecond {
		t.Fatalf("a key's echo reaches the screen in %v at the 95th percentile, want 16ms or less", p95)
	}
}

// screenDesk is a console whose agent region shows, at 41,1, a screen of
// w by h after text, and what the screen sends its agent.
type screenDesk struct {
	t    *testing.T
	m    Model
	mu   sync.Mutex
	sent []byte
}

func newScreenDesk(t *testing.T, text string, w, h int) *screenDesk {
	t.Helper()
	d := &screenDesk{t: t, m: boardModel(t, boardState(doneMatch("flint-auth", 10*time.Minute), doneMatch("ux-pass", time.Minute)))}
	d.m.opt.OutsideTmux = true // the console's clip writes OSC 52, as over ssh
	scr := newAgentScreen(w, h)
	_, _ = scr.Write([]byte(text))
	d.m.agent = agentRegion{term: &agentTerm{scr: scr, framed: true}, name: "flint-auth", x: 41, y: 1, w: w, h: h}
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 1<<10)
		for {
			n, err := scr.Read(buf)
			d.mu.Lock()
			d.sent = append(d.sent, buf[:n]...)
			d.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() { _ = scr.Close(); <-done })
	return d
}

// do is msg through the console; it gives what msg put on the clipboard.
func (d *screenDesk) do(msg tea.Msg) []string {
	d.t.Helper()
	next, cmd := d.m.Update(msg)
	d.m = next.(Model)
	var clipped []string
	for _, msg := range cmdMsgs(cmd) {
		if v := reflect.ValueOf(msg); v.IsValid() && v.Type() == reflect.TypeOf(tea.SetClipboard("")()) {
			clipped = append(clipped, v.String())
		}
	}
	return clipped
}

// drag presses at from, moves through to and releases there, each a cell
// of the agent's screen; to may be outside it.
func (d *screenDesk) drag(from, to point) {
	at := func(p point) tea.Mouse { return tea.Mouse{X: 41 + p.x, Y: 1 + p.y, Button: tea.MouseLeft} }
	d.do(tea.MouseClickMsg(at(from)))
	d.do(tea.MouseMotionMsg(at(point{from.x + 1, from.y})))
	d.do(tea.MouseMotionMsg(at(to)))
	d.do(tea.MouseReleaseMsg(at(to)))
}

// agentRead is what the agent read: the screen's bytes, after a moment.
func (d *screenDesk) agentRead() string {
	time.Sleep(50 * time.Millisecond)
	d.mu.Lock()
	defer d.mu.Unlock()
	return string(d.sent)
}

// reversed is, for each cell of the region as drawn, whether it draws in
// reverse video.
func (d *screenDesk) reversed() [][]bool {
	r := d.m.agent
	e := vt.NewEmulator(r.w, r.h)
	_, _ = e.WriteString(strings.Join(r.lines(newStyles(true), r.w, r.h), "\r\n"))
	out := make([][]bool, r.h)
	for y := range out {
		for x := range r.w {
			out[y] = append(out[y], e.CellAt(x, y).Style.Attrs&uv.AttrReverse != 0)
		}
	}
	return out
}

const dragScreen = "zero\r\nalpha beta\r\n\x1b[38;2;215;119;87mgamma\x1b[0m \x1b[7mdelta\x1b[0m\r\nepsilon zeta\r\neta"

func TestADragSelectsTheAgentsText(t *testing.T) {
	const w, h = 40, 8
	d := newScreenDesk(t, dragScreen, w, h)
	before := d.reversed()
	d.drag(point{6, 1}, point{6, 3})
	after := d.reversed()
	for y := range h {
		for x := range w {
			in := y == 1 && x >= 6 || y == 2 || y == 3 && x <= 6
			if flipped := before[y][x] != after[y][x]; flipped != in {
				t.Fatalf("cell %d,%d: reverse video flipped %v, want %v (in the selection: %v)", x, y, flipped, in, in)
			}
		}
	}
	if !before[2][6] || after[2][6] {
		t.Fatalf("the agent's own reverse text does not show selected: reverse %v, then %v", before[2][6], after[2][6])
	}
	if got, want := d.do(keyPress("c")), []string{"beta\ngamma delta\nepsilon"}; !slices.Equal(got, want) {
		t.Fatalf("c put %q on the clipboard, want %q", got, want)
	}
	if got := d.m.noticeText; !strings.HasPrefix(got, "copied: 24 characters") {
		t.Fatalf("c said %q", got)
	}

	// A drag that leaves the region stops at its edge: it never takes the list.
	d.drag(point{6, 1}, point{-30, 3})
	if got, want := d.do(keyPress("c")), []string{"beta\ngamma delta\ne"}; !slices.Equal(got, want) {
		t.Fatalf("a drag into the list copied %q, want %q", got, want)
	}
	d.drag(point{2, 3}, point{90, 40})
	if got, want := d.do(keyPress("c")), []string{"silon zeta\neta"}; !slices.Equal(got, want) {
		t.Fatalf("a drag past the region's end copied %q, want %q", got, want)
	}

	t.Run("a resize ends the selection", func(t *testing.T) {
		d := newScreenDesk(t, dragScreen, w, h)
		d.drag(point{0, 1}, point{4, 1})
		d.m.agent.resize(w-4, h)
		if d.m.agent.sel != nil || d.m.agent.press != nil {
			t.Fatalf("after a resize the selection is %v, its press %v", d.m.agent.sel, d.m.agent.press)
		}
	})
	t.Run("output that scrolls the selected text ends the selection", func(t *testing.T) {
		d := newScreenDesk(t, dragScreen, w, h)
		d.drag(point{0, 1}, point{4, 1})
		d.do(agentFrameMsg{t: d.m.agent.term})
		if d.m.agent.sel == nil {
			t.Fatal("a frame that changed nothing ended the selection")
		}
		_, _ = d.m.agent.term.scr.Write([]byte("\x1b[8;1H\r\nnew\r\nlines"))
		d.do(agentFrameMsg{t: d.m.agent.term})
		if d.m.agent.sel != nil {
			t.Fatalf("the screen scrolled under the selection, which stays at %v", *d.m.agent.sel)
		}
	})
	t.Run("a wide glyph at an end is selected whole", func(t *testing.T) {
		d := newScreenDesk(t, "ab日本cd", w, h)
		before := d.reversed()
		d.drag(point{3, 0}, point{4, 0}) // the second cell of 日 to the first of 本
		after := d.reversed()
		for _, x := range []int{0, 1, 2, 4, 6, 7} { // a wide glyph's second cell draws with its first
			if flipped, in := before[0][x] != after[0][x], x >= 2 && x <= 5; flipped != in {
				t.Fatalf("cell %d: reverse video flipped %v, want %v", x, flipped, in)
			}
		}
		if got := d.do(keyPress("c")); !slices.Equal(got, []string{"日本"}) {
			t.Fatalf("c copied %q, want both glyphs whole", got)
		}
	})
}

func TestAWrappedLineCopiesWhole(t *testing.T) {
	const w, h = 64, 6
	d := newScreenDesk(t, "  Open this link to sign in:\r\n"+
		"   https://accounts.example.com/auth?redirect_uri=https%3A%2F\r\n"+
		"     %2Fsdk.example.com%2Fcode&scope=openid\r\n"+
		"     &state=Bkxx\r\n"+
		"  \x1b]8;;https://x\x07then\x1b]8;;\x07 press \x1b[1menter\x1b[0m", w, h)
	d.drag(point{0, 1}, point{w - 1, 3})
	const url = "https://accounts.example.com/auth?redirect_uri=https%3A%2F%2Fsdk.example.com%2Fcode&scope=openid&state=Bkxx"
	if got := d.do(keyPress("c")); !slices.Equal(got, []string{url}) {
		t.Fatalf("c copied %q, want the URL on one line", got)
	}
	d.drag(point{0, 0}, point{w - 1, 4})
	if got, want := d.do(keyPress("c")), []string{"  Open this link to sign in:\n" +
		"   https://accounts.example.com/auth?redirect_uri=https%3A%2F\n" +
		"     %2Fsdk.example.com%2Fcode&scope=openid\n" +
		"     &state=Bkxx\n" +
		"  then press enter"}; !slices.Equal(got, want) {
		t.Fatalf("a copy of more than the URL is %q, want it as it shows %q", got, want)
	}
}

func TestTheMouseTable(t *testing.T) {
	const w, h = 40, 8
	mouseOn := "\x1b[?1002h\x1b[?1006h" // the agent's tmux asks for the mouse
	t.Run("a click in the region gives the agent the keys, then passes", func(t *testing.T) {
		d := newScreenDesk(t, mouseOn+dragScreen, w, h)
		d.do(tea.MouseClickMsg{X: 41 + 3, Y: 1 + 2, Button: tea.MouseLeft})
		d.do(tea.MouseReleaseMsg{X: 41 + 3, Y: 1 + 2, Button: tea.MouseLeft})
		if mode := d.m.agent.mode(); mode != "AGENT" {
			t.Fatalf("the keys are %s", mode)
		}
		if got := d.agentRead(); got != "\x1b[<0;4;3M\x1b[<0;4;3m" {
			t.Fatalf("the agent read %q, want the click", got)
		}
	})
	t.Run("a drag in the region is matchblox's selection", func(t *testing.T) {
		d := newScreenDesk(t, mouseOn+dragScreen, w, h)
		d.drag(point{6, 1}, point{6, 3})
		if d.m.agent.sel == nil {
			t.Fatal("a drag selected nothing")
		}
		if got := d.agentRead(); got != "\x1b[<0;7;2M" {
			t.Fatalf("the agent read %q, want only the press", got)
		}
	})
	t.Run("shift and a drag are the terminal's own selection", func(t *testing.T) {
		d := newScreenDesk(t, mouseOn+dragScreen, w, h)
		if mode := d.m.View().MouseMode; mode != tea.MouseModeCellMotion {
			t.Fatalf("the console's mouse mode is %v: a drag never reaches it", mode)
		}
		d.do(tea.MouseClickMsg{X: 41 + 6, Y: 1 + 1, Button: tea.MouseLeft})
		d.do(tea.MouseMotionMsg{X: 41 + 6, Y: 1 + 3, Button: tea.MouseLeft, Mod: tea.ModShift})
		if d.m.agent.sel != nil {
			t.Fatalf("a shift drag selected %v", *d.m.agent.sel)
		}
	})
	t.Run("the wheel in the region goes to the agent's tmux", func(t *testing.T) {
		d := newAgentDesk(t, w, h, map[string]string{"flint-auth": `seq 1 100; printf 'END\n'; exec sleep 600`})
		pane := d.pane("flint-auth")
		x := runDesk(t, boardState(doneMatch("flint-auth", 10*time.Minute), doneMatch("ux-pass", time.Minute)))
		x.pump(x.r.open("flint-auth", d.view("flint-auth", pane), w, h))
		x.r.x, x.r.y = 41, 1
		x.until("flint-auth's screen", x.shows("END"))
		x.step(300 * time.Millisecond) // the client asks for the mouse
		chosen := x.m.chosenSide()
		x.update(tea.MouseWheelMsg{X: 41 + 5, Y: 1 + 3, Button: tea.MouseWheelUp})
		x.until("the agent's pane in copy mode", func() bool { return d.tmux("display", "-p", "-t", pane, "#{pane_in_mode}") == "1" })
		if got := x.m.chosenSide(); got != chosen {
			t.Fatalf("the wheel over the agent moved the list's choice to %v", got)
		}
	})
	t.Run("the wheel over the list scrolls the list", func(t *testing.T) {
		d := newScreenDesk(t, mouseOn+dragScreen, w, h)
		chosen := d.m.chosenSide()
		d.do(tea.MouseWheelMsg{X: 5, Y: 4, Button: tea.MouseWheelDown})
		if got := d.m.chosenSide(); got == chosen {
			t.Fatalf("the wheel over the list left the choice on %v", got)
		}
		if got := d.agentRead(); got != "" {
			t.Fatalf("the wheel over the list reached the agent: %q", got)
		}
	})
	t.Run("with a selection, c copies it and any other key clears it and goes on", func(t *testing.T) {
		d := newScreenDesk(t, dragScreen, w, h)
		d.drag(point{0, 1}, point{4, 1})
		if got := d.do(keyPress("c")); !slices.Equal(got, []string{"alpha"}) {
			t.Fatalf("c copied %q, want alpha", got)
		}
		if d.m.agent.sel != nil {
			t.Fatal("the selection stays after c")
		}
		d.drag(point{0, 1}, point{4, 1})
		if got := d.do(keyPress("x")); len(got) > 0 || d.m.agent.sel != nil {
			t.Fatalf("x copied %q, and the selection is %v", got, d.m.agent.sel)
		}
		if got := d.agentRead(); got != "x" {
			t.Fatalf("the agent read %q, want x", got)
		}
	})
}

func TestCCopiesOnlyWithASelection(t *testing.T) {
	const w, h = 40, 8
	d := newScreenDesk(t, dragScreen, w, h)
	d.do(keyPress(flipKey))
	if got := d.do(keyPress("c")); len(got) > 0 {
		t.Fatalf("with no selection c copied %q", got)
	}
	if got := d.agentRead(); got != "c" {
		t.Fatalf("with no selection the agent read %q, want c", got)
	}
	d.drag(point{0, 3}, point{6, 3})
	if got := d.do(keyPress("c")); !slices.Equal(got, []string{"epsilon"}) {
		t.Fatalf("after a drag c copied %q, want epsilon", got)
	}
	if got := d.agentRead(); got != "c" {
		t.Fatalf("after a drag c typed into the agent: it read %q", got)
	}

	t.Run("with no selection, c in the list copies what the agent asked", func(t *testing.T) {
		d := newScreenDesk(t, dragScreen, w, h)
		d.m.agent.copy = "go test ./...\r\n\x1b[31mred\x1b[0m\x07 ‮evil"
		const want = "go test ./...\n[31mred[0m evil"
		d.do(keyPress(flipKey))
		if got := d.do(keyPress("c")); len(got) > 0 || d.m.agent.copy == "" {
			t.Fatalf("with the agent's keys c copied %q and the request is %q", got, d.m.agent.copy)
		}
		d.do(keyPress(flipKey))
		if got := d.do(keyPress("c")); !slices.Equal(got, []string{want}) {
			t.Fatalf("c in the list copied %q, want %q", got, want)
		}
		if d.m.agent.copy != "" {
			t.Fatalf("the request stays after c: %q", d.m.agent.copy)
		}
		if got := d.do(keyPress("c")); slices.Contains(got, want) {
			t.Fatal("c copied the agent's request twice")
		}
	})

	t.Run("a request over 64 KB is not kept", func(t *testing.T) {
		d := newScreenDesk(t, dragScreen, w, h)
		ask := func(n int) {
			_, _ = d.m.agent.term.scr.Write([]byte("\x1b]52;c;" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("a"), n)) + "\x07"))
			d.m.noticeText, d.m.saying = "", nil
			d.do(agentFrameMsg{t: d.m.agent.term})
		}
		ask(64 << 10)
		if len(d.m.agent.copy) != 64<<10 {
			t.Fatalf("a request of 64 KB keeps %d bytes", len(d.m.agent.copy))
		}
		ask(3_000_000)
		if d.m.agent.copy != "" {
			t.Fatalf("a request of 3 MB keeps %d bytes", len(d.m.agent.copy))
		}
		if got, want := d.m.sideSays(), "not kept: the agent asked to copy 3 MB, 64 KB at most"; got != want {
			t.Fatalf("the list foot says %q, want %q", got, want)
		}
	})
	t.Run("tabs and lines stay", func(t *testing.T) {
		d := newScreenDesk(t, "x\ty", w, h)
		d.m.agent.copy = "a\tb\nc"
		if got := d.do(keyPress("c")); !slices.Equal(got, []string{"a\tb\nc"}) {
			t.Fatalf("c in the list copied %q, want the tab and the line kept", got)
		}
		// The screen keeps no tab: it moves the cursor to column 8 over blank cells.
		d.drag(point{0, 0}, point{8, 0})
		if got := d.do(keyPress("c")); !slices.Equal(got, []string{"x       y"}) {
			t.Fatalf("a selection over a tab copied %q, want the blank cells as spaces", got)
		}
	})
}
