package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/service"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/stoker"
	"github.com/blox-eng/matchblox/internal/transport"
)

// matchblox starts this test binary as `matchblox args...` in its own
// process: its own stdin, stdout and environment, through run as main
// calls it.
func matchblox(t *testing.T, stdin string, env []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(append(os.Environ(), "MATCHBLOX_TEST_ARGS="+strings.Join(args, "\x1f")), env...)
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// TestTheHookProcessDeniesAMerge drives `matchblox hook PreToolUse` as
// Claude Code does: the event on stdin, TMUX_PANE in the environment, and
// the run's driven.json in the state dir.
func TestTheHookProcessDeniesAMerge(t *testing.T) {
	isolate(t)
	dir := filepath.Dir(state.Path())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stoker.json"), []byte(`{"on":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// The builder's %9 has the merge; the stoker's %9, on its own
	// server, never runs one.
	d := stoker.Driven{Run: "r", Panes: map[string][]string{"%1": {"release"}, "%9": {"merge"}}, StokerSocket: stokersSocket}
	if err := stoker.WriteDriven(dir, d); err != nil {
		t.Fatal(err)
	}
	event := func(cmd string) string {
		b, err := json.Marshal(map[string]any{"session_id": "s1", "hook_event_name": "PreToolUse", "tool_name": "Bash",
			"tool_use_id": "toolu_1", "tool_input": map[string]string{"command": cmd}})
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	for _, tc := range []struct {
		pane, cmd string
		deny      bool
	}{
		{"%1", "cd api && gh pr merge 4556 --squash", true}, // driven, the order gave no merge
		{"%1", "gh release create v1", false},               // the order's release
		{"%9@matchblox", "gh pr merge 4556", true},          // the stoker's own pane, whatever the grants
		{"%9", "gh pr merge 4556", false},                   // the builder's %9 and its go
		{"%2", "gh pr merge 4556", false},                   // a pane the stoker never drove
		{"%1", "gh pr view 4556", false},                    // a read
		// N1: the guard reads the line before the scrub hides its values.
		{"%1", `echo "set A='1" && gh pr merge 5 && echo 'ok'`, true},
		{"%1", `echo " Z='" ; gh pr merge 5 ; echo "'"`, true},
	} {
		out, err := matchblox(t, event(tc.cmd), tmuxEnv(tc.pane), "hook", "PreToolUse")
		if err != nil {
			t.Fatalf("%s %q: %v\n%s", tc.pane, tc.cmd, err, out)
		}
		if got := denied(t, strings.TrimSpace(out)); got != tc.deny {
			t.Fatalf("%s %q: denied %v, want %v (%q)", tc.pane, tc.cmd, got, tc.deny, out)
		}
	}
}

// stokersSocket is the socket of the stoker's own tmux server in these
// tests, and buildersSocket the builder's.
const (
	stokersSocket  = "/tmp/tmux-test/matchblox"
	buildersSocket = "/tmp/tmux-test/default"
)

// tmuxEnv is the environment of a hook in pane: %n@matchblox is %n on the
// stoker's server, any other pane is on the builder's.
func tmuxEnv(pane string) []string {
	id, server, ok := strings.Cut(pane, "@")
	sock := buildersSocket
	if ok && server != "" {
		sock = stokersSocket
	}
	return []string{"TMUX=" + sock + ",4242,0", "TMUX_PANE=" + id}
}

// TestTheHookDeniesANewPaneOfTheStokersServer: a pane the stoker opened
// since the last sample is in no list of driven.json; the hook knows by
// its own $TMUX that it runs on the stoker's server, and asks no tmux.
func TestTheHookDeniesANewPaneOfTheStokersServer(t *testing.T) {
	isolate(t)
	dir := filepath.Dir(state.Path())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stoker.json"), []byte(`{"on":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	d := stoker.Driven{Run: "r", Panes: map[string][]string{}, StokerSocket: stokersSocket}
	if err := stoker.WriteDriven(dir, d); err != nil {
		t.Fatal(err)
	}
	// A tmux that says it ran: the hook must not need one.
	bin := t.TempDir()
	ranTmux := filepath.Join(bin, "ran")
	fake := "#!/bin/sh\ntouch " + ranTmux + "\n"
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte(fake), 0o700); err != nil { //nolint:gosec // a test's fake tmux
		t.Fatal(err)
	}
	path := "PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH")
	b, err := json.Marshal(map[string]any{"session_id": "s1", "hook_event_name": "PreToolUse", "tool_name": "Bash",
		"tool_use_id": "toolu_1", "tool_input": map[string]string{"command": "gh pr merge 4556"}})
	if err != nil {
		t.Fatal(err)
	}
	for pane, deny := range map[string]bool{"%12@matchblox": true, "%12": false, "%3": false} {
		out, err := matchblox(t, string(b), append(tmuxEnv(pane), path), "hook", "PreToolUse")
		if err != nil {
			t.Fatalf("%s: %v\n%s", pane, err, out)
		}
		if got := denied(t, strings.TrimSpace(out)); got != deny {
			t.Fatalf("%s: denied %v, want %v (%q)", pane, got, deny, out)
		}
	}
	if _, err := os.Stat(ranTmux); err == nil {
		t.Fatal("the hook ran tmux")
	}
}

// TestTheCLIsPaneComesFromItsProcess runs `matchblox stoker step` as its
// own process against a service on a real socket: the service reads the
// process's pid from the socket and finds its pane from it, whatever
// TMUX_PANE the process claims.
func TestTheCLIsPaneComesFromItsProcess(t *testing.T) {
	sock := isolate(t)
	l, err := transport.Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := newService(config.Default(), fixtures(t))
	st, err := stoker.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.Stoker, s.StokerServer = st, "mbtest"
	var seen atomic.Int64
	var pane atomic.Value
	s.PeerPane = func(pid int) string {
		seen.Store(int64(pid))
		return pane.Load().(string)
	}
	go s.Serve(ctx, l) //nolint:errcheck // ends with the test

	step := []string{"stoker", "step", "0", "--rule", "compact on PR", "--state", "done", "compacted work"}
	// The stoker's process, claiming another pane: it is the stoker.
	pane.Store("%9@mbtest")
	if out, err := matchblox(t, "", []string{"TMUX_PANE=%1"}, step...); err != nil {
		t.Fatalf("the stoker's step was refused: %v\n%s", err, out)
	}
	if pid := seen.Load(); pid <= 0 || pid == int64(os.Getpid()) {
		t.Fatalf("the service saw pid %d, not the CLI's own", pid)
	}
	if es := st.Entries(); len(es) != 1 || es[0].Text != "compacted work" {
		t.Fatalf("entries %+v", es)
	}
	// Another process, claiming the stoker's pane: refused.
	pane.Store("%1")
	out, err := matchblox(t, "", []string{"TMUX_PANE=%9"}, step...)
	if err == nil || !strings.Contains(out, "only the stoker logs its steps") {
		t.Fatalf("a claimed stoker pane was believed: %v\n%s", err, out)
	}
}

// TestTheHookProcessSpeaksForItsOwnPane: Ruling AN (C1). Claude Code runs
// `matchblox hook` through a shell, so the hook's process is a grandchild
// of the agent in the pane; this test process plays that agent. The
// service finds the hook's pane from its pid, by its parents, and takes an
// event only for that pane: the same hook from a process in another pane
// changes nothing.
func TestTheHookProcessSpeaksForItsOwnPane(t *testing.T) {
	sock := isolate(t)
	s := newService(config.Default(), fixtures(t))
	var agentIn atomic.Value // the pane this test process runs in
	agentIn.Store("%1")
	var asked atomic.Int64
	s.PeerPane = func(pid int) string {
		asked.Add(1)
		return service.PaneOfProcess(pid, []sample.Pane{{ID: agentIn.Load().(string), PID: os.Getpid()}})
	}
	go serve(sock, func() *service.Service { return s }) //nolint:errcheck // ends with the test process
	deadline := time.Now().Add(5 * time.Second)
	for len(s.State().Sessions) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	hookOfShell := func(name, event string) {
		t.Helper()
		cmd := exec.Command("sh", "-c", `"$0" hook "$1"; exit $?`, os.Args[0], name)
		cmd.Env = append(os.Environ(), "MATCHBLOX_TEST_ARGS=hook\x1f"+name, "TMUX_PANE=%1")
		cmd.Stdin = strings.NewReader(event)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("hook %s: %v\n%s", name, err, out)
		}
	}
	permission := func() bool {
		for _, it := range s.State().Queue {
			if it.SessionID == "s9" && it.Pane == "%1" && it.State == "permission" {
				return true
			}
		}
		return false
	}
	until := func(what string, cond func() bool) {
		t.Helper()
		for end := time.Now().Add(5 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(end) {
				t.Fatalf("%s in 5s: queue %+v", what, s.State().Queue)
			}
		}
	}
	hookOfShell("Notification", `{"session_id":"s9","cwd":"/work/app","hook_event_name":"Notification","message":"Claude needs your permission to use Bash","notification_type":"permission_prompt"}`)
	until("the agent's own hook reached the queue", permission)

	// The same UserPromptSubmit, from a process that runs in the stoker's
	// pane while it claims TMUX_PANE=%1.
	agentIn.Store("%9")
	before := asked.Load()
	submit := `{"session_id":"s9","cwd":"/work/app","hook_event_name":"UserPromptSubmit","prompt":"1"}`
	hookOfShell("UserPromptSubmit", submit)
	// The hook has sent its event when its process ends; give the service
	// a moment to read the pane and take it or drop it.
	for end := time.Now().Add(time.Second); asked.Load() == before && time.Now().Before(end); {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	if !permission() {
		t.Fatal("a hook from another pane cleared the permission prompt")
	}

	agentIn.Store("%1")
	hookOfShell("UserPromptSubmit", submit)
	until("the agent's own UserPromptSubmit cleared its prompt", func() bool { return !permission() })
}
