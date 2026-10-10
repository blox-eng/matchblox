package main

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/config"
)

func stopJSON(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../testdata/hooks/stop.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func spoolLines(t *testing.T, spool string) int {
	t.Helper()
	b, err := os.ReadFile(spool)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(b), "\n")
}

func TestHookWithoutServiceSpools(t *testing.T) {
	sock := isolate(t)
	spool := filepath.Join(t.TempDir(), "spool.jsonl")
	start := time.Now()
	if err := hook([]string{"Stop"}, bytes.NewReader(stopJSON(t)), io.Discard, sock, spool, false); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("hook took %v", d)
	}
	if n := spoolLines(t, spool); n != 1 {
		t.Fatalf("spool has %d lines", n)
	}
}

func TestHookStaleSocket(t *testing.T) {
	sock := isolate(t)
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	l.(*net.UnixListener).SetUnlinkOnClose(false)
	l.Close() // the file stays, nobody listens
	spool := filepath.Join(t.TempDir(), "spool.jsonl")
	start := time.Now()
	if err := hook([]string{"Stop"}, bytes.NewReader(stopJSON(t)), io.Discard, sock, spool, false); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("hook took %v", d)
	}
	if n := spoolLines(t, spool); n != 1 {
		t.Fatalf("spool has %d lines", n)
	}
}

// A listener that accepts and never says hello must not hold the agent.
func TestHookSilentListener(t *testing.T) {
	sock := isolate(t)
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			defer c.Close()
		}
	}()
	spool := filepath.Join(t.TempDir(), "spool.jsonl")
	start := time.Now()
	if err := hook([]string{"Stop"}, bytes.NewReader(stopJSON(t)), io.Discard, sock, spool, false); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("hook took %v", d)
	}
	if n := spoolLines(t, spool); n != 1 {
		t.Fatalf("spool has %d lines", n)
	}
}

func TestHookReachesService(t *testing.T) {
	sock := isolate(t)
	s := newService(config.Default(), fixtures(t))
	go serve(sock, s) //nolint:errcheck // ends with the test process
	deadline := time.Now().Add(5 * time.Second)
	for len(s.State().Sessions) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	ev := `{"session_id":"s9","cwd":"/work/app","hook_event_name":"Notification","message":"Claude needs your permission to use Bash","notification_type":"permission_prompt"}`
	t.Setenv("TMUX_PANE", "%1")
	spool := filepath.Join(t.TempDir(), "spool.jsonl")
	if err := hook([]string{"Notification"}, strings.NewReader(ev), io.Discard, sock, spool, false); err != nil {
		t.Fatal(err)
	}
	for time.Now().Before(deadline) {
		for _, it := range s.State().Queue {
			if it.SessionID == "s9" && it.Pane == "%1" && !it.Estimated {
				if _, err := os.Stat(spool); !os.IsNotExist(err) {
					t.Fatal("delivered and spooled")
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("queue %+v", s.State().Queue)
}

// The agent never sees a failing hook: bad input is dropped, exit 0.
func TestHookIgnoresBadInput(t *testing.T) {
	sock := isolate(t)
	if err := hook([]string{"Stop"}, strings.NewReader("not json"), io.Discard, sock, filepath.Join(t.TempDir(), "s"), false); err != nil {
		t.Fatal(err)
	}
}

// statusInput is what Claude Code gives its status line (shortened).
const statusInput = `{"session_id":"s-1","model":{"display_name":"Opus"},"rate_limits":{"five_hour":{"used_percentage":28,"resets_at":1791673800},"seven_day":{"used_percentage":12,"resets_at":1792267200}}}`

// TestTheStatusLineHookTapsTheLimits: the hook keeps the limits for its
// config directory, then prints the line of the status line it wraps, fed
// the same input; with nothing wrapped it prints the limits. A broken input
// or a failing wrapped command never breaks the status line.
func TestTheStatusLineHookTapsTheLimits(t *testing.T) {
	stateDir := t.TempDir()
	cfgDir := filepath.Join(t.TempDir(), "claude")
	env := func(k string) string {
		if k == "CLAUDE_CONFIG_DIR" {
			return cfgDir
		}
		return ""
	}
	now := time.Unix(1791667916, 0)
	var out bytes.Buffer
	if err := statusLine(nil, strings.NewReader(statusInput), &out, env, stateDir, config.Default(), now); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.HasPrefix(got, "4 matches · 9 sparks") {
		t.Fatalf("alone, the status line is %q", got)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "limits")); err != nil {
		t.Fatal("no reading was kept")
	}
	out.Reset()
	if err := statusLine([]string{"--", "sh", "-c", "grep -c rate_limits; echo mine"}, strings.NewReader(statusInput), &out, env, stateDir, config.Default(), now); err != nil {
		t.Fatal(err)
	}
	if out.String() != "1\nmine\n" {
		t.Fatalf("the wrapped status line printed %q", out.String())
	}
	out.Reset()
	if err := statusLine([]string{"--", "sh", "-c", "echo still; exit 3"}, strings.NewReader("not json"), &out, env, stateDir, config.Default(), now); err != nil {
		t.Fatal(err)
	}
	if out.String() != "still\n" {
		t.Fatalf("a broken input: %q", out.String())
	}
}
