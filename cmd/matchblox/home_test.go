package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestErrorInTheHomeSessionWaits: the console is the only command of the
// tmux session matchblox, so on an error its pane closes; the error must
// stay on the screen until the person reads it.
func TestErrorInTheHomeSessionWaits(t *testing.T) {
	env := func(k string) string {
		if k == "MATCHBLOX_HOME" {
			return "1"
		}
		return ""
	}
	in := strings.NewReader("\n")
	var out bytes.Buffer
	holdOnError(errors.New("the service does not answer"), env, in, &out)
	if !strings.Contains(out.String(), "press Enter") {
		t.Fatalf("printed %q", out.String())
	}
	if in.Len() != 0 {
		t.Fatal("it did not wait for Enter")
	}

	in, out = strings.NewReader("\n"), bytes.Buffer{}
	holdOnError(errors.New("x"), func(string) string { return "" }, in, &out)
	if in.Len() == 0 || out.Len() != 0 {
		t.Fatal("outside the home session it waited")
	}
}

// TestSessionStartAsksForProgress: the SessionStart hook prints one line
// that Claude Code adds to the session's context, so the agent reports its
// progress and the console reads it from the transcript.
func TestSessionStartAsksForProgress(t *testing.T) {
	ev := `{"session_id":"s1","hook_event_name":"SessionStart","cwd":"/w"}`
	spool := filepath.Join(t.TempDir(), "spool.jsonl")
	var out bytes.Buffer
	// Review 2, #4: outside tmux (a script, claude -p) nothing is asked:
	// the console watches tmux panes only.
	t.Setenv("TMUX_PANE", "")
	if err := hook([]string{"SessionStart"}, strings.NewReader(ev), &out, filepath.Join(t.TempDir(), "none.sock"), spool, true); err != nil || out.Len() != 0 {
		t.Fatalf("outside tmux it printed %q", out.String())
	}
	t.Setenv("TMUX_PANE", "%3")
	if err := hook([]string{"SessionStart"}, strings.NewReader(ev), &out, filepath.Join(t.TempDir(), "none.sock"), spool, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Progress [████░░░░]") {
		t.Fatalf("printed %q", out.String())
	}
	if b, _ := os.ReadFile(spool); !strings.Contains(string(b), "SessionStart") {
		t.Fatal("the event was not kept")
	}
	out.Reset()
	if err := hook([]string{"SessionStart"}, strings.NewReader(ev), &out, filepath.Join(t.TempDir(), "none.sock"), spool, false); err != nil || out.Len() != 0 {
		t.Fatalf("progress_prompt = false still printed %q", out.String())
	}
	out.Reset()
	if err := hook([]string{"Stop"}, strings.NewReader(`{"session_id":"s1","hook_event_name":"Stop"}`), &out, filepath.Join(t.TempDir(), "none.sock"), spool, true); err != nil || out.Len() != 0 {
		t.Fatalf("Stop printed %q", out.String())
	}
}

// TestServiceWatchesTheDefaultTmux: the service outlives the console that
// started it, so it must not keep that console's tmux server: a console in
// another server (a nested or private one) would bind every later console
// to panes it cannot see.
func TestServiceWatchesTheDefaultTmux(t *testing.T) {
	isolate(t)
	t.Setenv("TMUX", "/tmp/tmux-1000/private,1,0")
	t.Setenv("TMUX_PANE", "%9")
	c, closeLog, err := serviceCmd()
	if err != nil {
		t.Fatal(err)
	}
	defer closeLog()
	if c.Env == nil {
		t.Fatal("the service takes the console's whole environment")
	}
	for _, kv := range c.Env {
		if strings.HasPrefix(kv, "TMUX=") || strings.HasPrefix(kv, "TMUX_PANE=") {
			t.Fatalf("the service keeps %s", kv)
		}
	}
	if !slices.ContainsFunc(c.Env, func(kv string) bool { return strings.HasPrefix(kv, "HOME=") }) {
		t.Fatal("the service lost the rest of the environment")
	}
}
