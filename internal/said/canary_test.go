//go:build canary

package said

import (
	"encoding/json"
	"errors"
	"fmt"
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
				t.Logf("%s %s: waiting state, menu and options read well", a.name, res.Version)
			}
		})
	}
	if tried == 0 {
		t.Fatal("the canary tried no agent")
	}
}

func runCanary(t *testing.T, a agent) canaryResult {
	res := canaryResult{Agent: a.name}
	fail := func(reason string) canaryResult {
		res.Check, res.Screen = couldNotRun, reason
		return res
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		return fail("tmux is not installed")
	}
	bin, err := exec.LookPath(a.bin)
	if err != nil {
		return fail(a.bin + " is not installed")
	}

	host, _ := os.Hostname()
	var name string
	if u, err := user.Current(); err == nil {
		name = u.Username
	}

	for attempt := 1; attempt <= 2; attempt++ {
		base := t.TempDir()
		home, work := filepath.Join(base, "home"), filepath.Join(base, "work")
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
			return fail(err.Error())
		}

		screen, check, err := drawMenu(t, fmt.Sprintf("mbc-%d-%s-%d", os.Getpid(), a.key, attempt), work, env, a.argv())
		res.Screen = scrub(screen, work, home, host, name)
		if err != nil {
			res.Check = couldNotRun
			res.Screen = err.Error()
			continue
		}
		if stand.toolRequests() == 0 {
			res.Check = couldNotRun
			res.Screen = a.name + " sent the stand-in model no request with a " + stand.toolName() + " tool"
			continue
		}
		res.Check, res.OK = check, check == ""
		if res.OK {
			if n := stand.toolRequests(); n != 1 {
				t.Errorf("the stand-in got %d requests with a %s tool before the menu, want 1", n, stand.toolName())
			}
			return res
		}
	}
	return res
}

var errSessionEnded = errors.New("the agent's terminal closed before it drew a menu")

// drawMenu starts argv in a tmux server of its own and returns the last
// screen it drew, with canaryCheck's verdict, once the menu reads well or
// 30 s have passed.
func drawMenu(t *testing.T, sock, work string, env, argv []string) (screen, check string, err error) {
	tmux := func(args ...string) *exec.Cmd {
		return exec.Command("tmux", append([]string{"-L", sock}, args...)...) //nolint:gosec // the test's own socket and argv
	}
	start := append([]string{"new-session", "-d", "-s", "canary", "-x", "120", "-y", "40", "-c", work, "env", "-i"}, env...)
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
	t.Cleanup(kill)
	check = "no waiting state"
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
		b, err := tmux("capture-pane", "-p", "-t", "canary").Output()
		if err != nil {
			return screen, check, errSessionEnded
		}
		screen = string(b)
		var ok bool
		if check, ok = canaryCheck(screen); ok {
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
		argv:    func() []string { return []string{bin} },
		stand:   func() standIn { return &anthropicStandIn{} },
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
