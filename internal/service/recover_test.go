package service

import (
	"encoding/json"
	"errors"
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

// mainBranch: every folder is its own repository, on main.
func mainBranch(dir string) recovery.Checkout {
	return recovery.Checkout{Root: dir, Branch: "main", Common: dir + "/.git"}
}

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
	rec.Git = mainBranch
	s.Recovery = rec
	s.settle = time.Millisecond
	s.TmuxOut = func([]string) (string, error) { return "", nil } // each started agent runs
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
		panes = append(panes, sample.Pane{ID: x.Pane, Target: x.Target, Socket: recSock, Server: pid, ServerStart: "1" + pid})
	}
	panes = append(panes, sample.Pane{ID: "%99", Target: "shell:1.0", Socket: recSock, Server: pid, ServerStart: "1" + pid})
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
	rec.Git = mainBranch
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
	if e := got[0]; e.Session != "api" || e.Socket != recSock || e.ServerPID != "100" || e.ServerStart != "1100" || e.Repo != "/src/api" || e.Common != "/src/api/.git" {
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
	s.Recovery.Git = func(dir string) recovery.Checkout {
		c := mainBranch(dir)
		if dir == moved.Cwd {
			c.Branch = "other"
		}
		return c
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
	s.Recovery.Git = func(dir string) recovery.Checkout { // the worktree moved: same repository
		return recovery.Checkout{Root: dir, Branch: "main", Common: api.Cwd + "/.git"}
	}
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

// TestRecoverRefusesAFolderOfAnotherRepository: --dir names a folder of
// another repository on the same branch. The conversation does not reopen
// there.
func TestRecoverRefusesAFolderOfAnotherRepository(t *testing.T) {
	s, ran := recoverTest(t)
	api := agentIn(t, s, "claude", "c1", "api", "%3")
	publishOn(s, "100", api)
	loseServer(s)
	other := filepath.Join(s.Home, "other")
	must(t, os.MkdirAll(other, 0o700))
	res := recoverAct(t, s, proto.Recover{Names: []string{"api"}, Dirs: map[string]string{"api": other}}, "")
	if r := rows(res)["api"]; r.State != proto.RecoverCannot || !strings.Contains(r.Why, "is another repository than "+api.Cwd) {
		t.Fatalf("row %+v", r)
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

// TestRecoverRefusesARelativeDir: the service does not know the builder's
// folder, so a --dir that is not a full path reopens nothing.
func TestRecoverRefusesARelativeDir(t *testing.T) {
	s, ran := recoverTest(t)
	api := agentIn(t, s, "claude", "c1", "api", "%3")
	publishOn(s, "100", api)
	loseServer(s)
	res := recoverAct(t, s, proto.Recover{Names: []string{"api"}, Dirs: map[string]string{"api": "."}}, "")
	if r := rows(res)["api"]; r.State != proto.RecoverCannot || !strings.Contains(r.Why, "not a full path") || len(*ran) != 0 {
		t.Fatalf("row %+v, ran %q", r, *ran)
	}
}

// TestRecoverRefusesASessionItCannotSignInAgain: a session that ran on
// Bedrock, Vertex or an API key got its sign-in from the shell. recover
// cannot give it again, so it does not reopen the conversation on the
// builder's default sign-in.
func TestRecoverRefusesASessionItCannotSignInAgain(t *testing.T) {
	s, ran := recoverTest(t)
	bed := agentIn(t, s, "claude", "c1", "bed", "%3")
	bed.Provider = "bedrock"
	key := agentIn(t, s, "codex", "x1", "key", "%4")
	key.Account = "API key"
	publishOn(s, "100", bed, key)
	loseServer(s)
	res := recoverAct(t, s, proto.Recover{All: true}, "")
	r := rows(res)
	if r["bed"].State != proto.RecoverCannot || !strings.Contains(r["bed"].Why, "bedrock") || !strings.Contains(r["bed"].Why, "claude --resume c1") {
		t.Errorf("bed: %+v", r["bed"])
	}
	if r["key"].State != proto.RecoverCannot || !strings.Contains(r["key"].Why, "API key") || !strings.Contains(r["key"].Why, "codex resume x1") {
		t.Errorf("key: %+v", r["key"])
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

// TestRecoverByNameAnswersForThoseNames: matchblox recover api answers for
// api only, so another entry that cannot reopen does not fail it.
func TestRecoverByNameAnswersForThoseNames(t *testing.T) {
	s, _ := recoverTest(t)
	api := agentIn(t, s, "claude", "c1", "api", "%3")
	old := agentIn(t, s, "claude", "c2", "old", "%4")
	publishOn(s, "100", api, old)
	loseServer(s)
	must(t, os.RemoveAll(old.Cwd))
	res := recoverAct(t, s, proto.Recover{Names: []string{"api"}}, "")
	if len(res.Recover) != 1 || res.Recover[0].Name != "api" || res.Recover[0].State != proto.RecoverReopened {
		t.Fatalf("rows %+v", res.Recover)
	}
}

// TestRecoverTwiceBeforeASampleStartsNothing: a second recover comes
// before a sample shows the reopened agents, or the sample shows the pane
// before the agent's id. It starts no second agent.
func TestRecoverTwiceBeforeASampleStartsNothing(t *testing.T) {
	s, ran := recoverTest(t)
	api := agentIn(t, s, "claude", "c1", "api", "%3")
	publishOn(s, "100", api)
	loseServer(s)
	recoverAct(t, s, proto.Recover{All: true}, "")
	if len(*ran) != 1 {
		t.Fatalf("ran %q", *ran)
	}
	api.Pane, api.SessionID = "%0", "" // the sample sees the pane, not yet the id
	publishOn(s, "200", api)
	res := recoverAct(t, s, proto.Recover{All: true}, "")
	if len(*ran) != 1 || rows(res)["api"].State != proto.RecoverLive {
		t.Fatalf("a second recover: %+v, ran %q", res.Recover, *ran)
	}
}

// TestRecoverSaysWhenTheAgentExitsAtOnce: tmux started the session, but
// the agent exited and took it along. The row says so, with the command
// to run to see why, and the conversation does not count as live.
func TestRecoverSaysWhenTheAgentExitsAtOnce(t *testing.T) {
	s, _ := recoverTest(t)
	web := agentIn(t, s, "codex", "x1", "web", "%4")
	publishOn(s, "100", web)
	loseServer(s)
	s.TmuxOut = func([]string) (string, error) { return "", errors.New("can't find session: web") }
	res := recoverAct(t, s, proto.Recover{All: true}, "")
	r := rows(res)["web"]
	if r.State != proto.RecoverCannot || !strings.Contains(r.Why, "codex exited at once: run cd "+web.Cwd+" && codex resume x1") {
		t.Fatalf("row %+v", r)
	}
	if live, _ := s.recoverLive(); live[conversation("codex", "x1")] {
		t.Fatal("an agent that exited counts as live")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
