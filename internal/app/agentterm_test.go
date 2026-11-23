//go:build linux

package app

import (
	"errors"
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode"

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
