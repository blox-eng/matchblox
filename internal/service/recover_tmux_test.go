package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/recovery"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// TestRecoverAfterARealServerLoss is #215's proof. Agents run on the
// builder's tmux server (testhome.Run makes it a private one). The server
// dies. recover reopens each conversation in its folder, with its resume
// flag and no prompt. A second recover starts nothing.
func TestRecoverAfterARealServerLoss(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	sock := sample.SocketPath("default")            // the builder's server: TmuxOn("")
	must(t, os.MkdirAll(filepath.Dir(sock), 0o700)) // as tmux makes it
	tmux := func(args ...string) (string, error) {
		out, err := exec.Command("tmux", append([]string{"-S", sock}, args...)...).CombinedOutput() //nolint:gosec // test argv
		return string(out), err
	}
	t.Cleanup(func() { _, _ = tmux("kill-server") })

	// Fake agents: each writes its argv and folder, then waits.
	bin, logs := t.TempDir(), t.TempDir()
	for _, a := range []string{"claude", "codex", sample.CursorAgent} {
		script := "#!/bin/sh\necho \"$(basename \"$0\") $* @ $(pwd -P)\" > " + filepath.Join(logs, a) + "\nexec sleep 300\n"
		must(t, os.WriteFile(filepath.Join(bin, a), []byte(script), 0o700)) //nolint:gosec // a test program
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	s := newTest(t, time.Hour)
	s.Home = t.TempDir()
	rec, err := recovery.Open(t.TempDir())
	must(t, err)
	s.Recovery = rec
	s.Actions.Run = nil // real steps

	var sessions []sample.Session
	for _, a := range []struct{ agent, id, name string }{
		{"claude", "c1", "api"}, {"codex", "x1", "web"}, {sample.CursorAgent, "u1", "ui"},
	} {
		x := agentIn(t, s, a.agent, a.id, a.name, "")
		x.Cwd, err = filepath.EvalSymlinks(x.Cwd) // macOS: /var is /private/var
		must(t, err)
		if out, err := tmux("-f", os.DevNull, "new-session", "-d", "-s", a.name, "-c", x.Cwd, "sleep", "300"); err != nil {
			t.Fatalf("the builder's server does not start: %v %s", err, out)
		}
		sessions = append(sessions, x)
	}
	sample1 := func() []sample.Pane {
		out, _ := sample.TmuxPanesOn("")
		return sample.ParsePanes(out)
	}
	ps := sample1()
	for i := range sessions {
		for _, p := range ps {
			if strings.HasPrefix(p.Target, sessions[i].Name+":") {
				sessions[i].Pane, sessions[i].Target = p.ID, p.Target
			}
		}
	}
	s.publish(sample.Snapshot{At: time.Now(), Sessions: sessions, Panes: ps}, func(state.Doc) {})
	if n := len(rec.List()); n != 3 {
		t.Fatalf("the record holds %d entries, want 3: %+v", n, rec.List())
	}

	// The server dies.
	if out, err := tmux("kill-server"); err != nil {
		t.Fatalf("kill-server: %v %s", err, out)
	}
	s.publish(sample.Snapshot{At: time.Now(), Panes: sample1()}, func(state.Doc) {})

	res := recoverAct(t, s, proto.Recover{All: true}, "")
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	for _, r := range res.Recover {
		if r.State != proto.RecoverReopened {
			t.Errorf("row %+v", r)
		}
	}
	want := map[string]string{
		"claude":           "claude --resume c1 @ " + sessions[0].Cwd,
		"codex":            "codex resume x1 @ " + sessions[1].Cwd,
		sample.CursorAgent: "cursor-agent --resume u1 @ " + sessions[2].Cwd,
	}
	deadline := time.Now().Add(10 * time.Second)
	for a, line := range want {
		for {
			b, _ := os.ReadFile(filepath.Join(logs, a)) //nolint:gosec // the test's own log
			if strings.TrimSpace(string(b)) == line {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s ran %q, want %q", a, strings.TrimSpace(string(b)), line)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	names := func() string {
		out, _ := tmux("list-sessions", "-F", "#{session_name}")
		l := strings.Fields(out)
		sort.Strings(l)
		return strings.Join(l, " ")
	}
	if got := names(); got != "api ui web" {
		t.Fatalf("the reopened server has sessions %q, want api ui web", got)
	}

	// The next sample sees the reopened agents; a second recover starts nothing.
	ps = sample1()
	for i := range sessions {
		for _, p := range ps {
			if strings.HasPrefix(p.Target, sessions[i].Name+":") {
				sessions[i].Pane, sessions[i].Target = p.ID, p.Target
			}
		}
	}
	s.publish(sample.Snapshot{At: time.Now(), Sessions: sessions, Panes: ps}, func(state.Doc) {})
	res = recoverAct(t, s, proto.Recover{All: true}, "")
	for _, r := range res.Recover {
		if r.State != proto.RecoverLive {
			t.Errorf("second recover: row %+v", r)
		}
	}
	if got := names(); got != "api ui web" {
		t.Fatalf("after a second recover the sessions are %q", got)
	}
}

// TestRecoverOnARealServerSaysWhenTheAgentExits: the agent's CLI fails at
// start and its tmux session ends with it. recover says it could not
// reopen, with the command to run to see why.
func TestRecoverOnARealServerSaysWhenTheAgentExits(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	sock := sample.SocketPath("default")
	must(t, os.MkdirAll(filepath.Dir(sock), 0o700))
	t.Cleanup(func() { _ = exec.Command("tmux", "-S", sock, "kill-server").Run() }) //nolint:gosec // test argv
	bin := t.TempDir()
	must(t, os.WriteFile(filepath.Join(bin, "codex"), []byte("#!/bin/sh\necho 'no such session' >&2\nexit 1\n"), 0o700)) //nolint:gosec // a test program
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	s := newTest(t, time.Hour)
	s.Home = t.TempDir()
	rec, err := recovery.Open(t.TempDir())
	must(t, err)
	s.Recovery = rec
	s.Actions.Run = nil
	web := agentIn(t, s, "codex", "x1", "web", "%4")
	must(t, rec.Update([]recovery.Entry{{Agent: "codex", ConversationID: "x1", Session: "web", Pane: "%4", Cwd: web.Cwd, Socket: sock, ServerPID: "1"}}, nil, time.Now()))

	res := recoverAct(t, s, proto.Recover{All: true}, "")
	r := rows(res)["web"]
	if r.State != proto.RecoverCannot || !strings.Contains(r.Why, "codex exited at once") {
		t.Fatalf("row %+v", r)
	}
}
