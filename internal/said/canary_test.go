//go:build canary

package said

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

type canaryResult struct {
	Agent   string `json:"agent"`
	Version string `json:"version"`
	OK      bool   `json:"ok"`
	Check   string `json:"check"`
	Screen  string `json:"screen"`
}

const couldNotRun = "could not run"

var versionRe = regexp.MustCompile(`^\d+(\.\d+){1,3}$`)

func TestCanary(t *testing.T) {
	out := os.Getenv("MATCHBLOX_CANARY_OUT")
	if out == "" {
		out = t.TempDir()
	}
	want := agents
	if names := os.Getenv("MATCHBLOX_CANARY_AGENTS"); names != "" {
		want = nil
		for _, key := range strings.Split(names, ",") {
			i := slices.IndexFunc(agents, func(a agent) bool { return a.key == key })
			if i < 0 {
				t.Errorf("MATCHBLOX_CANARY_AGENTS names %q, which is not an agent", key)
				continue
			}
			want = append(want, agents[i])
		}
	}

	tried := 0
	for _, a := range want {
		t.Run(a.key, func(t *testing.T) {
			tried++
			res := runCanary(t, a)
			b, err := json.Marshal(res)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, a.key+".json"), b, 0o600); err != nil {
				t.Fatal(err)
			}
			switch {
			case res.Check == couldNotRun:
				t.Errorf("could not run %s: %s", a.name, res.Screen)
			case !res.OK:
				t.Errorf("%s %s draws a screen matchblox cannot read (%s):\n%s", a.name, res.Version, res.Check, res.Screen)
			case !versionRe.MatchString(res.Version):
				t.Errorf("%s version %q is not a version", a.name, res.Version)
			default:
				t.Logf("%s %s: %s read well", a.name, res.Version, a.reads)
			}
		})
	}
	if tried == 0 {
		t.Fatal("the canary tried no agent")
	}
}

func runCanary(t *testing.T, a agent) canaryResult {
	res := canaryResult{Agent: a.name}
	host, _ := os.Hostname()
	var name string
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	var home, work string
	fail := func(reason string) canaryResult {
		res.Check, res.Screen = couldNotRun, scrub(reason, work, home, host, name)
		return res
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		return fail("tmux is not installed")
	}
	bin, err := exec.LookPath(a.bin)
	if err != nil {
		return fail(a.bin + " is not installed")
	}

	// A real break an attempt saw stays the verdict when a retry errors.
	var broke *canaryResult
	for attempt := 1; attempt <= 2; attempt++ {
		base := t.TempDir()
		home, work = filepath.Join(base, "home"), filepath.Join(base, "work")
		for _, d := range []string{home, work} {
			if err := os.Mkdir(d, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		stand := a.stand()
		srv := httptest.NewServer(stand)
		t.Cleanup(srv.Close)
		env := a.env(home, srv.URL, filepath.Dir(bin))

		if res.Version == "" {
			v, err := a.version(env)
			if err != nil {
				return fail(err.Error())
			}
			res.Version = v
		}
		if err := a.setup(home, work, srv.URL); err != nil {
			if broke != nil {
				return *broke
			}
			return fail(err.Error())
		}

		screen, check, err := drawMenu(t, fmt.Sprintf("mbc-%d-%s-%d", os.Getpid(), a.key, attempt), work, env, a.argv, a.check)
		shown := scrub(screen, work, home, host, name)
		switch {
		case err != nil:
			res.Check, res.Screen = couldNotRun, because(err.Error(), shown)
		case stand.toolRequests() == 0:
			res.Check, res.Screen = couldNotRun, because(a.name+" sent the stand-in model no request with a "+stand.toolName()+" tool", shown)
		case check != "" && !strings.Contains(screen, standInCommand):
			res.Check, res.Screen = couldNotRun, because(a.name+" never showed the command "+standInCommand+": the stand-in's reply was not taken", shown)
		default:
			res.Check, res.OK, res.Screen = check, check == "", shown
			if res.OK {
				if n := stand.toolRequests(); n != 1 {
					t.Errorf("the stand-in got %d requests with a %s tool before the menu, want 1", n, stand.toolName())
				}
				return res
			}
			b := res
			broke = &b
		}
	}
	if broke != nil && res.Check == couldNotRun {
		return *broke
	}
	return res
}

// because is a could-not-run reason, then the screen the agent drew, if any.
func because(reason, screen string) string {
	if strings.TrimSpace(screen) == "" {
		return reason
	}
	return reason + "\n\n" + screen
}

// menuTimeout is how long drawMenu waits for a screen that reads well.
var menuTimeout = 30 * time.Second

var errSessionEnded = errors.New("the agent's terminal closed before it drew a menu")

// drawMenu starts argv in a tmux server of its own and returns the last
// screen it drew, with verdict's, once the screen reads well or menuTimeout has
// passed.
func drawMenu(t *testing.T, sock, work string, env, argv []string, verdict func(string) (string, bool)) (screen, check string, err error) {
	tmux := func(args ...string) *exec.Cmd {
		return exec.Command("tmux", append([]string{"-L", sock}, args...)...) //nolint:gosec // the test's own socket and argv
	}
	// remain-on-exit keeps the last screen of an agent that quits.
	start := append([]string{"start-server", ";", "set-option", "-g", "remain-on-exit", "on", ";",
		"new-session", "-d", "-s", "canary", "-x", "120", "-y", "40", "-c", work, "env", "-i"}, env...)
	if b, err := tmux(append(start, argv...)...).CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("tmux new-session: %v: %s", err, b)
	}
	path, _ := tmux("display-message", "-p", "#{socket_path}").Output()
	kill := func() {
		_ = tmux("kill-server").Run()
		if p := strings.TrimSpace(string(path)); p != "" {
			_ = os.Remove(p)
		}
	}
	defer kill()
	check = "no waiting state"
	for deadline := time.Now().Add(menuTimeout); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
		dead, _ := tmux("display-message", "-p", "-t", "canary", "#{pane_dead}").Output()
		b, err := tmux("capture-pane", "-p", "-t", "canary").Output()
		if err != nil {
			return screen, check, errSessionEnded
		}
		screen = string(b)
		if strings.TrimSpace(string(dead)) == "1" {
			// A dead pane moves what it drew into its history; 40 lines of it
			// keep the issue short.
			if b, err := tmux("capture-pane", "-p", "-S", "-40", "-t", "canary").Output(); err == nil {
				screen = strings.TrimLeft(string(b), "\n")
			}
			return screen, check, errSessionEnded
		}
		var ok bool
		if check, ok = verdict(screen); ok {
			return screen, "", nil
		}
	}
	return screen, check, nil
}

// An agent whose version command never answers could not run; the canary
// does not wait for go test's own timeout to panic.
func TestHungVersionCouldNotRun(t *testing.T) {
	defer func(d time.Duration) { commandTimeout = d }(commandTimeout)
	commandTimeout = time.Second

	// The sleep is a child, not exec'd: it holds the output open after the
	// script is killed.
	bin := filepath.Join(t.TempDir(), "hang")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nsleep 600\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := agent{
		key: "hang", name: "Hang", bin: bin,
		version: func(env []string) (string, error) { return firstField(env, bin, "--version") },
		setup:   func(_, _, _ string) error { return nil },
		env:     func(home, _, _ string) []string { return []string{"HOME=" + home, "PATH=/usr/bin:/bin"} },
		argv:    []string{bin},
		stand:   func() standIn { return &anthropicStandIn{} },
		check:   canaryCheck,
	}
	start := time.Now()
	res := runCanary(t, a)
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("runCanary took %v, want about 1 s", took)
	}
	if want := bin + " --version did not answer in 1 s"; res.Check != couldNotRun || res.Screen != want {
		t.Errorf("check %q, screen %q, want %q, %q", res.Check, res.Screen, couldNotRun, want)
	}
}

// An agent that dies on start leaves what it printed in the result, so its
// issue says why.
func TestQuitKeepsTheLastScreen(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "quit")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho boom: a config this release does not know\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := agent{
		key: "quit", name: "Quit", bin: bin,
		version: func([]string) (string, error) { return "1.0.0", nil },
		setup:   func(_, _, _ string) error { return nil },
		env:     func(home, _, _ string) []string { return []string{"HOME=" + home, "PATH=/usr/bin:/bin"} },
		argv:    []string{bin},
		stand:   func() standIn { return &anthropicStandIn{} },
		check:   canaryCheck,
	}
	res := runCanary(t, a)
	if res.Check != couldNotRun || !strings.HasPrefix(res.Screen, errSessionEnded.Error()) ||
		!strings.Contains(res.Screen, "boom: a config this release does not know") {
		t.Errorf("check %q, screen:\n%s", res.Check, res.Screen)
	}
}

// fixedStandIn reports a fixed count of tool requests and answers nothing,
// for agents that are shell scripts.
type fixedStandIn struct{ n int }

func (s fixedStandIn) ServeHTTP(http.ResponseWriter, *http.Request) {}
func (s fixedStandIn) toolName() string                             { return "Bash" }
func (s fixedStandIn) toolRequests() int                            { return s.n }

// scriptAgent is an agent that runs script under sh against stand.
func scriptAgent(t *testing.T, key, script string, stand standIn) agent {
	t.Helper()
	bin := filepath.Join(t.TempDir(), key)
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	return agent{
		key: key, name: key, bin: bin,
		version: func([]string) (string, error) { return "1.0.0", nil },
		setup:   func(_, _, _ string) error { return nil },
		env:     func(home, _, _ string) []string { return []string{"HOME=" + home, "PATH=/usr/bin:/bin"} },
		argv:    []string{bin},
		stand:   func() standIn { return stand },
		check:   canaryCheck,
	}
}

func shortMenuWait(t *testing.T) {
	t.Helper()
	d := menuTimeout
	t.Cleanup(func() { menuTimeout = d })
	menuTimeout = 2 * time.Second
}

// Every npm agent's launcher is a `#!/usr/bin/env node` script, so node's
// folder must be on the agent's PATH, not only on the test's.
func TestNodeLauncherFindsNode(t *testing.T) {
	nodeDir, binDir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(nodeDir, "node"), []byte("#!/bin/sh\necho 9.8.7\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, a := range agents {
		if err := os.WriteFile(filepath.Join(binDir, a.bin), []byte("#!/usr/bin/env node\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+nodeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, a := range agents {
		v, err := a.version(a.env(t.TempDir(), "http://127.0.0.1:1", binDir))
		if err != nil || v != "9.8.7" {
			t.Errorf("%s: version %q, %v; want 9.8.7", a.name, v, err)
		}
	}
}

// A version command that fails says why, so its issue names the cause.
func TestVersionKeepsStderr(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "broken")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho \"env: 'node': No such file or directory\" >&2\nexit 127\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := firstField([]string{"PATH=/usr/bin:/bin"}, bin, "--version")
	if err == nil || !strings.Contains(err.Error(), "exit status 127") || !strings.Contains(err.Error(), "env: 'node': No such file or directory") {
		t.Errorf("error %v, want the exit status and what the command printed", err)
	}
}

// A version or setup error names the run's own folders; the issue must not.
func TestFailReasonsAreScrubbed(t *testing.T) {
	a := scriptAgent(t, "failv", "exit 0\n", fixedStandIn{})
	a.version = func(env []string) (string, error) {
		return "", fmt.Errorf("no config in %s/.codex", strings.TrimPrefix(env[0], "HOME="))
	}
	res := runCanary(t, a)
	if res.Check != couldNotRun || !strings.Contains(res.Screen, "<home>/.codex") || strings.Contains(res.Screen, os.TempDir()) {
		t.Errorf("version error: check %q, screen %q", res.Check, res.Screen)
	}

	a = scriptAgent(t, "fails", "exit 0\n", fixedStandIn{})
	a.setup = func(_, work, _ string) error { return fmt.Errorf("cannot write %s/opencode.json", work) }
	res = runCanary(t, a)
	if res.Check != couldNotRun || !strings.Contains(res.Screen, "<work>/opencode.json") || strings.Contains(res.Screen, os.TempDir()) {
		t.Errorf("setup error: check %q, screen %q", res.Check, res.Screen)
	}
}

// An agent that never asked the stand-in for a tool could not run, and its
// issue shows what it drew instead.
func TestNoToolRequestKeepsTheScreen(t *testing.T) {
	shortMenuWait(t)
	res := runCanary(t, scriptAgent(t, "notool", "echo 'Sign in to go on'\nsleep 600\n", fixedStandIn{}))
	if res.Check != couldNotRun || !strings.Contains(res.Screen, "sent the stand-in model no request") ||
		!strings.Contains(res.Screen, "Sign in to go on") {
		t.Errorf("check %q, screen:\n%s", res.Check, res.Screen)
	}
}

// An agent that got the tool call but never shows its command did not take
// the stand-in's reply: the canary could not run it, matchblox did not break.
func TestUntakenReplyCouldNotRun(t *testing.T) {
	shortMenuWait(t)
	res := runCanary(t, scriptAgent(t, "untaken", "echo 'Working'\nsleep 600\n", fixedStandIn{n: 1}))
	if res.Check != couldNotRun || !strings.Contains(res.Screen, "the stand-in's reply was not taken") ||
		!strings.Contains(res.Screen, "Working") {
		t.Errorf("check %q, screen:\n%s", res.Check, res.Screen)
	}
}

// A real break on the first attempt stays the verdict when the retry errors.
func TestRetryKeepsARealBreak(t *testing.T) {
	shortMenuWait(t)
	ran := filepath.Join(t.TempDir(), "ran")
	script := fmt.Sprintf("if [ -e %q ]; then exit 1; fi\n: >%q\necho '$ %s'\necho 'Thinking'\nsleep 600\n", ran, ran, standInCommand)
	res := runCanary(t, scriptAgent(t, "retry", script, fixedStandIn{n: 1}))
	if res.Check != "no waiting state" || res.OK || !strings.Contains(res.Screen, standInCommand) {
		t.Errorf("check %q, ok %v, screen:\n%s", res.Check, res.OK, res.Screen)
	}
}

// A setup that fails on the retry must not hide the break the first attempt saw.
func TestRetrySetupFailureKeepsARealBreak(t *testing.T) {
	shortMenuWait(t)
	a := scriptAgent(t, "setup", fmt.Sprintf("echo '$ %s'\necho 'Thinking'\nsleep 600\n", standInCommand), fixedStandIn{n: 1})
	setups := 0
	a.setup = func(_, _, _ string) error {
		if setups++; setups > 1 {
			return errors.New("setup failed on retry")
		}
		return nil
	}
	res := runCanary(t, a)
	if res.Check != "no waiting state" || !strings.Contains(res.Screen, standInCommand) {
		t.Errorf("check %q, screen:\n%s", res.Check, res.Screen)
	}
}
