package panes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/guard"
	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

func TestHomeArgvOutsideTmux(t *testing.T) {
	got := HomeArgv(false, true, []string{"/usr/local/bin/matchblox", "--no-motion"})
	want := []string{"tmux", "-u", "new-session", "-A", "-s", "matchblox", "-e", "MATCHBLOX_HOME=1", "/usr/local/bin/matchblox", "--no-motion"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestHomeArgvInsideTmuxRunsInPlace(t *testing.T) {
	if got := HomeArgv(true, true, []string{"matchblox"}); got != nil {
		t.Fatalf("inside tmux the console runs in place, got %q", got)
	}
}

func TestHomeArgvWithoutTmuxRunsInPlace(t *testing.T) {
	if got := HomeArgv(false, false, []string{"matchblox"}); got != nil {
		t.Fatalf("without tmux the console runs in place, got %q", got)
	}
}

// tmux32 tells if tmux is installed at 3.2 or later: the way back uses
// status ranges, which 3.2 added.
func tmux32(t *testing.T) {
	t.Helper()
	out, err := exec.Command("tmux", "-V").Output()
	if err != nil {
		t.Skip("tmux is not installed")
	}
	var major, minor int
	if _, err := fmt.Sscanf(strings.TrimPrefix(strings.TrimSpace(string(out)), "tmux "), "%d.%d", &major, &minor); err != nil {
		t.Skipf("unknown tmux version %q", out)
	}
	if major < 3 || major == 3 && minor < 2 {
		t.Skipf("tmux %d.%d is older than 3.2", major, minor)
	}
}

// TestWayBackParses loads WayBack into a private tmux server. source-file
// prints a bad line on stderr and still exits 0, so the test reads stderr
// and the bindings the lines made.
func TestWayBackParses(t *testing.T) {
	tmux32(t)
	conf := filepath.Join(t.TempDir(), "wayback.conf")
	if err := os.WriteFile(conf, []byte(WayBack), 0o600); err != nil {
		t.Fatal(err)
	}
	sock := tmuxtest.Server(t)
	cmd := exec.Command("tmux", "-L", sock, "-f", os.DevNull, "new-session", "-d", "-s", "work", //nolint:gosec // test argv
		";", "source-file", conf,
		";", "source-file", conf, // people source their conf again after an edit
		";", "list-keys", "-T", "prefix", "m",
		";", "list-keys", "-T", "root", "MouseDown1Status",
		";", "show-options", "-g", "status-left",
		";", "show-options", "-g", "status-left-length",
		";", "kill-server")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil || stderr.Len() > 0 {
		t.Fatalf("tmux: %v\n%s", err, stderr.String())
	}
	// tmux 3.8 pads list-keys with a column of its own: "bind-key  -T".
	out = regexp.MustCompile(` {2,}`).ReplaceAll(out, []byte(" "))
	for _, want := range []string{
		`bind-key -T prefix m if-shell -F "#{==:#{pane_current_command},matchblox}"`,
		`bind-key -T root MouseDown1Status if-shell -F "#{==:#{mouse_status_range},matchblox}"`,
		"switch-client -t #{pane_id} ;",
		"#[range=user|matchblox] ◂ matchblox #[norange]",
		"status-left-length 40",
	} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("tmux lacks %q:\n%s", want, out)
		}
	}
	if n := strings.Count(string(out), "◂ matchblox"); n != 1 {
		t.Fatalf("sourced twice, the status line has %d labels:\n%s", n, out)
	}
}

// TestHomeKeyArgvBindsPrefixMAsTheKey: the home key and prefix m run one
// command: on a pane that runs a console it hands the key on, elsewhere it
// shows a console. A tap on ◂ matchblox goes there too.
func TestHomeKeyArgvBindsPrefixMAsTheKey(t *testing.T) {
	argv := HomeKeyArgv()
	var cmds [][]string
	for part := range strings.SplitSeq(strings.Join(argv[1:], "\x00"), "\x00;\x00") {
		cmds = append(cmds, strings.Split(part, "\x00"))
	}
	if len(cmds) != 4 {
		t.Fatalf("%d commands in %q", len(cmds), argv)
	}
	ifHome := []string{"if-shell", "-F", "#{==:#{pane_current_command},matchblox}",
		`if-shell -F "#{pane_in_mode}" "send-keys -X cancel" ; send-keys -H 1b 5f 6d 61 74 63 68 62 6c 6f 78 1b 5c`,
		`if-shell -F "#{S:#{?#{==:#{session_name},matchblox},1,}}" { switch-client -t =matchblox ; run-shell -C -t =matchblox: "#{W:#{P:#{?#{==:#{pane_current_command},matchblox},select-window -t #{window_id} ; select-pane -t #{pane_id} ; ,}}}" } { if-shell -F "#{S:#{W:#{P:#{?#{==:#{pane_current_command},matchblox},1,}}}}" { run-shell -C "#{S:#{W:#{P:#{?#{==:#{pane_current_command},matchblox},switch-client -t #{pane_id} ; ,}}}}" } { display-message "No matchblox console here: run matchblox in a shell to open one" } }`}
	if want := append([]string{"bind-key", "-n", "User900"}, ifHome...); !slices.Equal(cmds[1], want) {
		t.Errorf("the home key:\n got %q\nwant %q", cmds[1], want)
	}
	if want := append([]string{"bind-key", "m"}, ifHome...); !slices.Equal(cmds[2], want) {
		t.Errorf("prefix m:\n got %q\nwant %q", cmds[2], want)
	}
	if want := []string{"bind-key", "-n", "MouseDown1Status", "if-shell", "-F", "#{==:#{mouse_status_range},matchblox}", ifHome[4], "select-window -t ="}; !slices.Equal(cmds[3], want) {
		t.Errorf("the tap on ◂ matchblox:\n got %q\nwant %q", cmds[3], want)
	}
}

func TestSizeArgv(t *testing.T) {
	want := []string{"tmux", "set-option", "-w", "-t", "=matchblox:", "window-size", "latest"}
	if got := SizeArgv(); !slices.Equal(got, want) {
		t.Errorf("SizeArgv() = %q, want %q", got, want)
	}
}

// TestHomeKeyMatchesTheApp reads what the phone sends and taps from the
// fixture the Swift tests read too.
func TestHomeKeyMatchesTheApp(t *testing.T) {
	b, err := os.ReadFile("../../testdata/home/key.json")
	if err != nil {
		t.Fatal(err)
	}
	var fx struct{ Sequence, Label string }
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	if HomeKey != fx.Sequence {
		t.Errorf("HomeKey = %q, the app sends %q", HomeKey, fx.Sequence)
	}
	if !strings.Contains(WayBack, fx.Label) {
		t.Errorf("WayBack lacks the label %q the app taps", fx.Label)
	}
}

// TestHomeAndCommandKeysDoNotCollide: the home key and every command key
// have each their own user-keys slot and their own sequence, so one never
// takes another's binding.
func TestHomeAndCommandKeysDoNotCollide(t *testing.T) {
	slots := map[string]string{}
	for _, argv := range [][]string{HomeKeyArgv(), CommandKeysArgv(), MarkKeyArgv(), FlipKeysArgv()} {
		for i, a := range argv {
			if a != "set-option" {
				continue
			}
			slot, seq := argv[i+2], argv[i+3]
			if was, ok := slots[slot]; ok {
				t.Fatalf("%s holds %q and %q", slot, was, seq)
			}
			for s, other := range slots {
				if other == seq {
					t.Fatalf("%s and %s both hold %q", s, slot, seq)
				}
			}
			slots[slot] = seq
		}
	}
	if slots["user-keys["+strconv.Itoa(keycmd.MarkSlot)+"]"] != keycmd.MarkKey {
		t.Fatalf("the mark key is not held: %q", slots)
	}
	if len(slots) < 2 || slots["user-keys["+homeUserKey+"]"] != HomeKey {
		t.Fatalf("slots %q", slots)
	}
}

func TestNamedConsole(t *testing.T) {
	for arg0, want := range map[string]bool{
		"matchblox": true, "/usr/local/bin/matchblox": true, "./matchblox": true,
		"/tmp/matchblox-dev": false, "mb": false, "": false,
	} {
		if got := NamedConsole(arg0); got != want {
			t.Errorf("NamedConsole(%q) = %v, want %v", arg0, got, want)
		}
	}
}

func TestMarkKeyArgvRunsOnlyAtTheConsole(t *testing.T) {
	line := strings.Join(MarkKeyArgv(), " ")
	for _, want := range []string{"user-keys[950]", "bind-key -n User950 if-shell -F",
		"#{&&:#{==:#{session_name},matchblox},#{==:#{pane_current_command},matchblox}}", "#{client_tty}"} {
		if !strings.Contains(line, want) {
			t.Errorf("MarkKeyArgv lacks %q: %s", want, line)
		}
	}
}

func TestViewerHookArgvTellsOnlyTheConsole(t *testing.T) {
	argv := ViewerHookArgv()
	line := strings.Join(argv, " ")
	for _, want := range []string{"client-attached[950]", "client-session-changed[950]", "if-shell -F '" + atConsole + "'",
		"send-keys -t #{pane_id} -H " + hexKeys(keycmd.ViewerKey)} {
		if !strings.Contains(line, want) {
			t.Errorf("ViewerHookArgv lacks %q: %s", want, line)
		}
	}
	for i, a := range argv {
		if i == 0 || argv[i-1] == ";" {
			if a != "tmux" && a != "set-hook" {
				t.Errorf("ViewerHookArgv runs %q", a)
			}
		}
	}
}

// TestTheWayBackWithNoConsoleSaysHowToOpenOne: on a real tmux server with
// no console, the home key says how to open one instead of tmux's bare
// "can't find session" (#354). A console run in place in another session
// is found there. With the console's session there, it goes there.
func TestTheWayBackWithNoConsoleSaysHowToOpenOne(t *testing.T) {
	tmux32(t)
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script is not installed")
	}
	sock := tmuxtest.Server(t)
	env := append(slices.DeleteFunc(os.Environ(), func(e string) bool {
		return strings.HasPrefix(e, "TMUX=") || strings.HasPrefix(e, "TMUX_PANE=") || strings.HasPrefix(e, "TERM=")
	}), "TERM=xterm-256color")
	tmux := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("tmux", append([]string{"-L", sock, "-f", os.DevNull}, args...)...) //nolint:gosec // the test's own server
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %q: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	tmux("new-session", "-d", "-s", "work", "-x", "80", "-y", "24")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", sock, "kill-server").Run() }) //nolint:gosec // the test's own server
	tmux(HomeKeyArgv()[1:]...)
	// A key pressed while a message shows may only clear the message: the
	// test's presses come far quicker than a person's.
	tmux("set-option", "-g", "display-time", "50")
	path := tmux("display-message", "-p", "#{socket_path}")
	args := []string{"-qc", "tmux -S " + path + " attach -t work", os.DevNull}
	if runtime.GOOS == "darwin" {
		args = []string{"-q", os.DevNull, "tmux", "-S", path, "attach", "-t", "work"}
	}
	client := exec.Command("script", args...)
	client.Env = env
	if _, err := client.StdinPipe(); err != nil { // an EOF would end the client
		t.Fatal(err)
	}
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Process.Kill(); _ = client.Wait() })
	tty := ""
	for deadline := time.Now().Add(5 * time.Second); tty == "" && time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		tty = tmux("list-clients", "-F", "#{client_tty}")
	}
	if tty == "" {
		t.Fatal("no client attached")
	}
	until := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(50 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%s\nclient: %s\npanes:\n%s\nmessages:\n%s", what,
					tmux("display-message", "-p", "-c", tty, "#{session_name} #{pane_id} #{pane_current_command}"),
					tmux("list-panes", "-a", "-F", "#{session_name} #{pane_id} #{pane_current_command}"),
					tmux("show-messages"))
			}
		}
	}
	tmux("send-keys", "-K", "-c", tty, "User"+homeUserKey)
	until("the home key with no console did not say how to open one:\n"+tmux("show-messages"), func() bool {
		return strings.Contains(tmux("show-messages"), NoHome)
	})
	// A console run in place, in a pane of another session: tmux knows it
	// by its command's name, so a link named matchblox stands in for it. The
	// link is to bash: a multi-call coreutils sleep refuses an unknown name.
	bin := filepath.Join(t.TempDir(), Home)
	shell, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not installed")
	}
	if err := os.Symlink(shell, bin); err != nil {
		t.Fatal(err)
	}
	tmux("new-session", "-d", "-s", "side", "-x", "80", "-y", "24")
	tmux("new-window", "-t", "side", bin+" -c 'sleep 300; :'")
	inPlace := tmux("display-message", "-p", "-t", "side:1", "#{pane_id}")
	until("the console in side never ran", func() bool {
		return tmux("display-message", "-p", "-t", inPlace, "#{pane_current_command}") == Home
	})
	time.Sleep(200 * time.Millisecond) // past the message's display-time
	tmux("send-keys", "-K", "-c", tty, "User"+homeUserKey)
	until("the home key did not go to the console run in place", func() bool {
		return tmux("display-message", "-p", "-c", tty, "#{pane_id}") == inPlace
	})
	tmux("switch-client", "-c", tty, "-t", "=work")
	tmux("new-session", "-d", "-s", Home, "-x", "80", "-y", "24")
	tmux("send-keys", "-K", "-c", tty, "User"+homeUserKey)
	until("the home key did not go to the console's session", func() bool {
		return tmux("display-message", "-p", "-c", tty, "#{session_name}") == Home
	})
}

// TestChooseTreeMovesToPrefixCapitalS: prefix s flips, so tmux's own
// session tree moves to prefix S.
func TestChooseTreeMovesToPrefixCapitalS(t *testing.T) {
	argv := strings.Join(FlipKeysArgv(), "\x00")
	if !strings.Contains(argv, "bind-key\x00S\x00choose-tree\x00-Zs") {
		t.Fatalf("no prefix S choose-tree in %q", FlipKeysArgv())
	}
	if !strings.Contains(argv, "bind-key\x00s\x00if-shell") {
		t.Error("prefix s does not flip")
	}
	if strings.Contains(argv, "bind-key\x00n\x00") {
		t.Error("prefix n is bound: it stays tmux's next window; the walk is ⌥n")
	}
}

// TestAGuardedPaneIsNoConsole: matchblox starts Cursor behind its redraw
// guard (#399), and the guard is the pane's own program. It runs under
// another name than matchblox, so its pane is no console: the way back
// leaves it and types nothing into Cursor, and with no console it says how
// to open one.
func TestAGuardedPaneIsNoConsole(t *testing.T) {
	tmux32(t)
	sock := tmuxtest.Server(t)
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", sock, "-f", os.DevNull}, args...)...).CombinedOutput() //nolint:gosec // the test's own server
		if err != nil {
			t.Fatalf("tmux %q: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	shell, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not installed")
	}
	// tmux knows a program by its name: a link stands in for each.
	dir := t.TempDir()
	for session, name := range map[string]string{"guarded": guard.LinkName, "console": Home} {
		if err := os.Symlink(shell, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
		tmux("new-session", "-d", "-s", session, "-x", "80", "-y", "24", filepath.Join(dir, name)+" -c 'sleep 300; :'")
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", sock, "kill-server").Run() }) //nolint:gosec // the test's own server
	for session, want := range map[string][2]string{"guarded": {guard.LinkName, "0"}, "console": {Home, "1"}} {
		var got string
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			if tmux("display-message", "-p", "-t", session+":", "#{pane_current_command}") == want[0] { // the link has started
				got = tmux("display-message", "-p", "-t", session+":", runsConsole)
				break
			}
		}
		if got != want[1] {
			t.Errorf("pane %s: runs a console = %q, want %q", session, got, want[1])
		}
	}
	tmux("kill-session", "-t", "console")
	if got := tmux("display-message", "-p", "-t", "guarded:", consoleExists); got != "" {
		t.Errorf("with only a guarded Cursor on the server, a console exists = %q, want none", got)
	}
}
