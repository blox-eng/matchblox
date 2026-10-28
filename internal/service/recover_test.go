package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/recovery"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

const recSock = "/tmp/tmux-501/default"

// recoverTest is a service with a recovery record and a home where each
// agent is installed. Its record holds what the sessions of the first
// snapshot give it.
func recoverTest(t *testing.T, sessions ...sample.Session) (*Service, *[][]string) {
	t.Helper()
	s := newTest(t, time.Hour)
	s.Home = t.TempDir()
	rec, err := recovery.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rec.Git = func(dir string) (string, string) { return dir, "main" }
	s.Recovery = rec
	s.LookPath = func(bin string) (string, error) {
		switch bin {
		case "claude", "codex", sample.CursorAgent:
			return "/bin/" + bin, nil
		}
		return "", os.ErrNotExist
	}
	var ran [][]string
	s.Actions.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	publishOn(s, "100", sessions...)
	return s, &ran
}

// publishOn samples the sessions on the builder's server, whose pid is pid.
func publishOn(s *Service, pid string, sessions ...sample.Session) {
	var panes []sample.Pane
	for _, x := range sessions {
		panes = append(panes, sample.Pane{ID: x.Pane, Target: x.Target, Socket: recSock, Server: pid})
	}
	panes = append(panes, sample.Pane{ID: "%99", Target: "shell:1.0", Socket: recSock, Server: pid})
	s.publish(sample.Snapshot{At: time.Now(), Sessions: sessions, Panes: panes}, func(state.Doc) {})
}

// loseServer: the builder's server died, and a new one runs only a shell.
func loseServer(s *Service) { publishOn(s, "200") }

// agentIn is an agent with a saved history, in a folder that is there.
func agentIn(t *testing.T, s *Service, agent, id, name, pane string) sample.Session {
	t.Helper()
	dir := filepath.Join(s.Home, "src", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	var history string
	switch agent {
	case "claude":
		history = filepath.Join(s.Home, ".claude", "projects", "-src-"+name, id+".jsonl")
	case "codex":
		history = filepath.Join(s.Home, ".codex", "sessions", "2026", "10", "28", "rollout-2026-10-28T09-00-00-"+id+".jsonl")
	}
	if history != "" {
		if err := os.MkdirAll(filepath.Dir(history), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(history, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return sample.Session{PID: 10, Agent: agent, SessionID: id, Name: name, Pane: pane, Target: name + ":1.0", Cwd: dir, Status: "idle"}
}

func recoverAct(t *testing.T, s *Service, r proto.Recover, from string) proto.Result {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return s.act(t.Context(), proto.Act{RecID: RecoverID, Confirm: "y", Text: string(b), From: from})
}

func rows(res proto.Result) map[string]proto.Recovered {
	m := map[string]proto.Recovered{}
	for _, r := range res.Recover {
		m[r.Name] = r
	}
	return m
}

// TestTheServiceRecordsTheBuildersAgents: each sample writes the agents of
// the builder's server to the record, with their server. The stoker's
// server, an agent with no conversation yet and an agent recover cannot
// resume stay out.
func TestTheServiceRecordsTheBuildersAgents(t *testing.T) {
	s := newTest(t, time.Hour)
	rec, err := recovery.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rec.Git = func(dir string) (string, string) { return dir, "main" }
	s.Recovery = rec
	in := []sample.Session{
		{Agent: "claude", SessionID: "c1", Pane: "%3", Target: "api:1.0", Cwd: "/src/api"},
		{Agent: sample.CursorAgent, SessionID: "u1", Pane: "%4", Target: "web:1.0", Cwd: "/src/web"},
		{Agent: "codex", SessionID: "x1", Pane: "%1@" + sample.StokerServer, Target: "stoker:1.0", Cwd: "/src"},
		{Agent: "claude", Pane: "%5", Target: "new:1.0", Cwd: "/src"},
		{Agent: "opencode", SessionID: "o1", Pane: "%6", Target: "oc:1.0", Cwd: "/src"},
	}
	publishOn(s, "100", in...)
	got := rec.List()
	if len(got) != 2 || got[0].ConversationID != "c1" || got[1].ConversationID != "u1" {
		t.Fatalf("the record holds %+v, want c1 and u1", got)
	}
	if e := got[0]; e.Session != "api" || e.Socket != recSock || e.ServerPID != "100" || e.Repo != "/src/api" {
		t.Fatalf("the entry is %+v", e)
	}
}

// TestRecoverPreviewsWithoutChange: with no All and no Names, recover
// tells each entry's state and runs nothing.
func TestRecoverPreviewsWithoutChange(t *testing.T) {
	s, ran := recoverTest(t)
	api := agentIn(t, s, "claude", "c1", "api", "%3")
	web := agentIn(t, s, "codex", "x1", "web", "%4")
	publishOn(s, "100", api, web)
	web.Pane, web.Target = "%0", "web:1.0"
	publishOn(s, "200", web) // the server died; web was started again by hand
	res := recoverAct(t, s, proto.Recover{}, "")
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	r := rows(res)
	if r["api"].State != proto.RecoverReady || r["web"].State != proto.RecoverLive {
		t.Fatalf("rows %+v", res.Recover)
	}
	if len(*ran) != 0 {
		t.Fatalf("a preview ran %q", *ran)
	}
}

// TestRecoverReopensEachConversationOnce: after the server loss, recover
// reopens each conversation in its folder, in a tmux session of its name,
// with no prompt. A second recover finds them live and starts nothing.
func TestRecoverReopensEachConversationOnce(t *testing.T) {
	s, ran := recoverTest(t)
	api := agentIn(t, s, "claude", "c1", "api", "%3")
	web := agentIn(t, s, "codex", "x1", "web", "%4")
	ui := agentIn(t, s, sample.CursorAgent, "u1", "ui", "%5")
	publishOn(s, "100", api, web, ui)
	loseServer(s)
	res := recoverAct(t, s, proto.Recover{All: true}, "")
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	pre := "env -u GIT_TERMINAL_PROMPT -u GIT_SSH_COMMAND -u MATCHBLOX_HOME tmux new-session -d"
	want := []string{
		pre + " -s api -n api -c " + api.Cwd + " claude --resume c1",
		pre + " -s ui -n ui -c " + ui.Cwd + " cursor-agent --resume u1",
		pre + " -s web -n web -c " + web.Cwd + " codex resume x1",
	}
	if len(*ran) != len(want) {
		t.Fatalf("ran %q", *ran)
	}
	for i, argv := range *ran {
		if got := strings.Join(argv, " "); got != want[i] {
			t.Errorf("step %d:\n got %s\nwant %s", i, got, want[i])
		}
	}
	for _, r := range res.Recover {
		if r.State != proto.RecoverReopened || r.As != r.Name {
			t.Errorf("row %+v", r)
		}
	}
	if r := rows(res)["ui"]; !strings.Contains(r.Why, "history not checked") {
		t.Errorf("a Cursor row does not say its history is not checked: %+v", r)
	}

	// The reopened agents show in the next sample.
	api.Pane, web.Pane, ui.Pane = "%0", "%1", "%2"
	publishOn(s, "200", api, web, ui)
	*ran = nil
	res = recoverAct(t, s, proto.Recover{All: true}, "")
	if len(*ran) != 0 {
		t.Fatalf("a second recover ran %q", *ran)
	}
	for _, r := range res.Recover {
		if r.State != proto.RecoverLive {
			t.Errorf("after reopening, row %+v", r)
		}
	}
	if n := len(s.Recovery.List()); n != 3 {
		t.Fatalf("the record holds %d entries, want 3", n)
	}
}

// TestRecoverSaysWhyItCannot: an entry that cannot reopen says its cause
// and the fix, and recover reopens the others.
func TestRecoverSaysWhyItCannot(t *testing.T) {
	s, ran := recoverTest(t)
	gone := agentIn(t, s, "claude", "c1", "gone", "%1")
	nohist := agentIn(t, s, "claude", "c2", "nohist", "%2")
	moved := agentIn(t, s, "claude", "c3", "moved", "%3")
	open := agentIn(t, s, "claude", "c4", "open", "%4")
	ok := agentIn(t, s, "claude", "c5", "ok", "%5")
	publishOn(s, "100", gone, nohist, moved, open, ok)
	loseServer(s)

	must(t, os.RemoveAll(gone.Cwd))
	must(t, os.RemoveAll(filepath.Join(s.Home, ".claude", "projects", "-src-nohist")))
	s.Recovery.Git = func(dir string) (string, string) {
		if dir == moved.Cwd {
			return dir, "other"
		}
		return dir, "main"
	}
	reg := filepath.Join(s.Home, ".claude", "sessions", strconv.Itoa(os.Getpid())+".json")
	must(t, os.MkdirAll(filepath.Dir(reg), 0o700))
	must(t, os.WriteFile(reg, []byte(`{"sessionId":"c4"}`), 0o600))
	res := recoverAct(t, s, proto.Recover{All: true}, "")
	r := rows(res)
	for name, want := range map[string]string{
		"gone":   "matchblox recover gone --dir ",
		"nohist": "no saved history",
		"moved":  "is on branch other now, not main",
		"open":   "open in another process (pid " + strconv.Itoa(os.Getpid()) + ")",
	} {
		if r[name].State != proto.RecoverCannot || !strings.Contains(r[name].Why, want) {
			t.Errorf("%s: %+v, want a cannot that says %q", name, r[name], want)
		}
	}
	if r["ok"].State != proto.RecoverReopened || len(*ran) != 1 {
		t.Errorf("ok: %+v, ran %q", r["ok"], *ran)
	}
}

// TestRecoverNamesTheMissingCLI: an agent that is not installed now cannot
// reopen, and the row says to install it.
func TestRecoverNamesTheMissingCLI(t *testing.T) {
	s, ran := recoverTest(t)
	web := agentIn(t, s, "codex", "x1", "web", "%4")
	publishOn(s, "100", web)
	loseServer(s)
	s.LookPath = func(string) (string, error) { return "", os.ErrNotExist }
	res := recoverAct(t, s, proto.Recover{All: true}, "")
	if r := rows(res)["web"]; r.State != proto.RecoverCannot || !strings.Contains(r.Why, "codex is not installed") {
		t.Fatalf("row %+v", r)
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

// TestRecoverTakesAMovedFolder: --dir gives a session's folder its new
// place. The conversation reopens there when it is the same branch.
func TestRecoverTakesAMovedFolder(t *testing.T) {
	s, ran := recoverTest(t)
	api := agentIn(t, s, "claude", "c1", "api", "%3")
	publishOn(s, "100", api)
	loseServer(s)
	to := filepath.Join(s.Home, "elsewhere")
	must(t, os.Rename(api.Cwd, to))
	res := recoverAct(t, s, proto.Recover{Names: []string{"api"}, Dirs: map[string]string{"api": to}}, "")
	if r := rows(res)["api"]; r.State != proto.RecoverReopened || r.Dir != to {
		t.Fatalf("row %+v", r)
	}
	if len(*ran) != 1 || !strings.Contains(strings.Join((*ran)[0], " "), " -c "+to+" ") {
		t.Fatalf("ran %q", *ran)
	}
}

// TestRecoverTakesAFreeName: a tmux session of the entry's name is there
// already, with no agent of the conversation. The reopened one takes a
// free name.
func TestRecoverTakesAFreeName(t *testing.T) {
	s, ran := recoverTest(t)
	api := agentIn(t, s, "claude", "c1", "api", "%3")
	publishOn(s, "100", api)
	s.publish(sample.Snapshot{At: time.Now(), Panes: []sample.Pane{{ID: "%0", Target: "api:1.0", Socket: recSock, Server: "200"}}}, func(state.Doc) {})
	res := recoverAct(t, s, proto.Recover{All: true}, "")
	if r := rows(res)["api"]; r.State != proto.RecoverReopened || r.As != "api-2" {
		t.Fatalf("row %+v", r)
	}
	if len(*ran) != 1 || !strings.Contains(strings.Join((*ran)[0], " "), " -s api-2 -n api-2 ") {
		t.Fatalf("ran %q", *ran)
	}
}

// TestRecoverIsTheBuilders: an agent's pane may preview, but only the
// builder reopens: recover starts agents.
func TestRecoverIsTheBuilders(t *testing.T) {
	s, ran := recoverTest(t)
	api := agentIn(t, s, "claude", "c1", "api", "%3")
	web := agentIn(t, s, "claude", "c2", "web", "%4")
	publishOn(s, "100", api, web)
	web.Pane = "%0"
	publishOn(s, "200", web)
	if res := recoverAct(t, s, proto.Recover{}, "%0"); res.Err != "" || len(res.Recover) != 2 {
		t.Fatalf("a preview from an agent's pane: %+v", res)
	}
	res := recoverAct(t, s, proto.Recover{All: true}, "%0")
	if res.Err != notTheBuilders || len(*ran) != 0 {
		t.Fatalf("a reopen from an agent's pane: %+v, ran %q", res, *ran)
	}
}

// TestRecoverRefusesAnUnknownName: a name not in the record reopens
// nothing and says how to list the names.
func TestRecoverRefusesAnUnknownName(t *testing.T) {
	s, ran := recoverTest(t)
	api := agentIn(t, s, "claude", "c1", "api", "%3")
	publishOn(s, "100", api)
	loseServer(s)
	res := recoverAct(t, s, proto.Recover{Names: []string{"apx"}}, "")
	if !strings.Contains(res.Err, "apx") || !strings.Contains(res.Err, "matchblox recover") || len(*ran) != 0 {
		t.Fatalf("%+v, ran %q", res, *ran)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
