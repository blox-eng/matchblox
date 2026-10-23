package service

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/accounts"
	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/holds"
	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// lifeSessions are the builder's: work at rest in %1 (tmux session app,
// its only window), busy in %2 (session dev, one of two windows), a Codex
// one in %3. Each window carries its agent's name, as `new` and `r` name
// it.
func lifeSnap(busy bool) sample.Snapshot {
	return sample.Snapshot{At: night0, Sessions: []sample.Session{
		{Agent: "claude", PID: 101, Start: 7, Pane: "%1", Target: "app:1.1", Cwd: "/work/app", SessionID: "s1", Name: "work", Status: "idle", Idle: 5 * time.Minute, Busy: busy},
		{Agent: "claude", PID: 102, Start: 7, Pane: "%2", Target: "dev:1.1", Cwd: "/work/app", SessionID: "s2", Name: "busy", Status: "busy", Busy: true},
		{Agent: "codex", PID: 103, Start: 7, Pane: "%3", Target: "cx:1.1", SessionID: "s3", Name: "cx", Status: "idle", Idle: time.Hour},
	}, Panes: []sample.Pane{
		{ID: "%1", Target: "app:1.1", Window: "work", Command: "claude"},
		{ID: "%2", Target: "dev:1.1", Window: "busy", Command: "claude"}, {ID: "%4", Target: "dev:2.1"},
		{ID: "%3", Target: "cx:1.1", Window: "cx", Command: "codex"},
		{ID: "%9@mbtest", Target: "stoker:1.1"},
	}}
}

// lifeTest is a service with the builder's sessions above, the steps it
// runs recorded, and the pane of a session it starts named by tmux.
func lifeTest(t *testing.T) (*Service, *[][]string) {
	t.Helper()
	s := newTest(t, time.Hour)
	s.StokerServer = "mbtest"
	s.LookPath = func(cmd string) (string, error) {
		if cmd == "opencode" {
			return "", exec.ErrNotFound
		}
		return "/usr/bin/" + cmd, nil
	}
	var mu sync.Mutex
	var ran [][]string
	s.Actions.Run = func(argv []string) error {
		mu.Lock()
		defer mu.Unlock()
		ran = append(ran, argv)
		return nil
	}
	s.Capture = func([]string) map[string]string { return map[string]string{"%1": idleScreen, "%3": idleScreen} }
	s.TmuxOut = func(argv []string) (string, error) { return "%42\n", nil }
	s.publish(lifeSnap(false), func(state.Doc) {})
	return s, &ran
}

func consoleAct(id, text string) proto.Act {
	return proto.Act{RecID: id, Confirm: "y", Text: text, Console: true}
}

func TestKillEndsTheAgentAndItsPane(t *testing.T) {
	s, ran := lifeTest(t)
	res := s.act(t.Context(), consoleAct("kill:%1", ""))
	want := [][]string{{"tmux", "kill-pane", "-t", "%1"}, {"kill", "-TERM", "101"}}
	if res.Err != "" || !slices.EqualFunc(*ran, want, slices.Equal) {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
}

func TestKillNeedsTheTypedY(t *testing.T) {
	s, ran := lifeTest(t)
	a := consoleAct("kill:%1", "")
	a.Confirm = ""
	if res := s.act(t.Context(), a); res.Err != "needs confirm" || len(*ran) != 0 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
}

func TestKillAsksAgainWhileItWorks(t *testing.T) {
	s, ran := lifeTest(t)
	res := s.act(t.Context(), consoleAct("kill:%2", ""))
	if res.Err != "busy works: y again kills it mid-turn" || len(*ran) != 0 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	if res := s.act(t.Context(), consoleAct("kill:%2", KillAnyway)); res.Err != "" || len(*ran) != 2 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
}

func TestKillNeverTheStokersSession(t *testing.T) {
	s, ran := lifeTest(t)
	snap := lifeSnap(false)
	snap.Sessions = append(snap.Sessions, sample.Session{Agent: "claude", PID: 109, Pane: "%9@mbtest", Name: "stoker", Status: "idle"})
	s.publish(snap, func(state.Doc) {})
	res := s.act(t.Context(), consoleAct("kill:%9@mbtest", KillAnyway))
	if res.Err != "the stoker is not killed here: Q on the STOKER tab quits it" || len(*ran) != 0 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
}

func TestKillAGoneSession(t *testing.T) {
	s, _ := lifeTest(t)
	if res := s.act(t.Context(), consoleAct("kill:%77", "")); res.Err != "no agent session in %77 any more" {
		t.Fatal(res.Err)
	}
}

func TestRenameNamesTmuxAndTheAgentAlike(t *testing.T) {
	s, ran := lifeTest(t)
	res := s.act(t.Context(), consoleAct("rename:%1", "ships-flake"))
	// The agent's /rename goes first: a guard that stops it leaves tmux
	// as it was, so the two names never part.
	want := [][]string{
		{"tmux", "send-keys", "-t", "%1", "-l", "--", "/rename ships-flake"},
		{"tmux", "send-keys", "-t", "%1", "Enter"},
		{"tmux", "rename-window", "-t", "%1", "ships-flake"},
		{"tmux", "rename-session", "-t", "%1", "ships-flake"},
	}
	if res.Err != "" || !slices.EqualFunc(*ran, want, slices.Equal) {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
}

func TestRenameKeepsASessionOfManyWindows(t *testing.T) {
	s, ran := lifeTest(t)
	snap := lifeSnap(false)
	snap.Sessions[1].Busy, snap.Sessions[1].Status = false, "idle"
	s.publish(snap, func(state.Doc) {})
	s.Capture = func([]string) map[string]string { return map[string]string{"%2": idleScreen} }
	if res := s.act(t.Context(), consoleAct("rename:%2", "two")); res.Err != "" {
		t.Fatal(res.Err)
	}
	for _, step := range *ran {
		if slices.Contains(step, "rename-session") {
			t.Fatalf("renamed dev, which holds two windows: %q", *ran)
		}
	}
}

func TestRenameRefusesABadOrTakenName(t *testing.T) {
	s, ran := lifeTest(t)
	for name, why := range map[string]string{
		"a.b": "a name is letters, digits, - and _, and starts with a letter or a digit",
		"dev": "a tmux session is named dev already: choose another name",
	} {
		if res := s.act(t.Context(), consoleAct("rename:%1", name)); res.Err != why {
			t.Errorf("%s: %q", name, res.Err)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

func TestRenameOfABusySessionWaitsForItsRest(t *testing.T) {
	s, ran := lifeTest(t)
	s.publish(lifeSnap(true), func(state.Doc) {})
	res := s.act(t.Context(), consoleAct("rename:%1", "later"))
	if res.Err != "" || len(*ran) != 0 || !slices.Equal(res.Skipped, []string{"work renames when it rests"}) {
		t.Fatalf("err %q skipped %q ran %q", res.Err, res.Skipped, *ran)
	}
	s.renameAtRest(t.Context()) // still busy
	if len(*ran) != 0 {
		t.Fatalf("ran while busy: %q", *ran)
	}
	s.publish(lifeSnap(false), func(state.Doc) {})
	s.renameAtRest(t.Context())
	if len(*ran) != 4 || (*ran)[2][len((*ran)[2])-1] != "later" {
		t.Fatalf("ran %q", *ran)
	}
	s.renameAtRest(t.Context())
	if len(*ran) != 4 {
		t.Fatalf("renamed twice: %q", *ran)
	}
}

func TestRenameOfCodexIsTmuxOnly(t *testing.T) {
	s, ran := lifeTest(t)
	if res := s.act(t.Context(), consoleAct("rename:%3", "cx2")); res.Err != "" {
		t.Fatal(res.Err)
	}
	for _, step := range *ran {
		if slices.Contains(step, "send-keys") {
			t.Fatalf("typed into Codex: %q", *ran)
		}
	}
	if len(*ran) != 2 {
		t.Fatalf("ran %q", *ran)
	}
}

func newAct(t *testing.T, n proto.NewSession) proto.Act {
	t.Helper()
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	return consoleAct("new:session", string(b))
}

func TestNewStartsTheAgentInASessionOfItsName(t *testing.T) {
	s, ran := lifeTest(t)
	dir := t.TempDir()
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: dir, Agent: "claude", Name: "fresh", First: "fix the flaky test"}))
	if res.Err != "" || len(*ran) != 1 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	got := strings.Join((*ran)[0], " ")
	want := "tmux new-session -d -s fresh -n fresh -c " + dir + " claude --name fresh fix the flaky test"
	if !strings.HasSuffix(got, want) || !strings.HasPrefix(got, "env -u GIT_TERMINAL_PROMPT") || slices.Contains((*ran)[0], "-L") {
		t.Fatalf("ran %q", got)
	}
}

func TestNewNamesItAfterItsDirectory(t *testing.T) {
	s, ran := lifeTest(t)
	dir := filepath.Join(t.TempDir(), "blox-api")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if res := s.act(t.Context(), newAct(t, proto.NewSession{Where: dir, Agent: "codex"})); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := strings.Join((*ran)[0], " "); !strings.HasSuffix(got, "-s blox-api -n blox-api -c "+dir+" codex") {
		t.Fatalf("ran %q", got)
	}
}

func TestNewAddsAWorktreeForABranch(t *testing.T) {
	s, ran := lifeTest(t)
	// git names the repository by its real path: macOS's temp dir is a link.
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(tmp, "blox-api")
	for _, c := range [][]string{{"init", "-q", repo}, {"-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "x"}} {
		if out, err := exec.Command("git", c...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %s", err, out)
		}
	}
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: repo, Branch: "feat/88-x", Agent: "claude"}))
	if res.Err != "" || len(*ran) != 2 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	wt := filepath.Join(filepath.Dir(repo), "blox-api-feat-88-x")
	if got, want := strings.Join((*ran)[0], " "), "git -C "+repo+" worktree add --no-track -b feat/88-x "+wt+" HEAD"; got != want {
		t.Fatalf("ran %q, want %q", got, want)
	}
	if got := strings.Join((*ran)[1], " "); !strings.HasSuffix(got, "-s blox-api-feat-88-x -n blox-api-feat-88-x -c "+wt+" claude --name blox-api-feat-88-x") {
		t.Fatalf("ran %q", got)
	}
}

func TestNewRefuses(t *testing.T) {
	s, ran := lifeTest(t)
	dir := t.TempDir()
	for why, n := range map[string]proto.NewSession{
		"a tmux session is named dev already: choose another name":                        {Where: dir, Agent: "claude", Name: "dev"},
		"opencode is not installed here":                                                  {Where: dir, Agent: "opencode", Name: "x"},
		"aider is not an agent matchblox starts: claude, codex, opencode or cursor-agent": {Where: dir, Agent: "aider", Name: "x"},
		"no directory " + dir + "/nope":                                                   {Where: dir + "/nope", Agent: "claude", Name: "x"},
		"a new session starts in a full path":                                             {Where: "rel/dir", Agent: "claude", Name: "x"},
		"a new worktree needs a git repository: " + dir + " is none":                      {Where: dir, Branch: "b", Agent: "claude"},
		"a first message does not start with -":                                           {Where: dir, Agent: "claude", Name: "x", First: "--dangerously-skip-permissions"},
		"the first message must be one line, without control keys":                        {Where: dir, Agent: "claude", Name: "x", First: "a\nb"},
	} {
		if res := s.act(t.Context(), newAct(t, n)); res.Err != why {
			t.Errorf("want %q, got %q", why, res.Err)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

// The stoker's hands: kill waits for the builder's go unless the order
// gave kills, and is refused on a session that works, waits or is parked.

func handsLife(t *testing.T) (*Service, *[][]string) {
	t.Helper()
	s, ran := handsTest(t)
	s.TmuxOut = func([]string) (string, error) { return "%42\n", nil }
	hs, err := holds.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.Holds = hs
	return s, ran
}

func TestStokerKillWaitsForTheBuildersGo(t *testing.T) {
	s, ran := handsLife(t)
	res := s.act(t.Context(), fromStokerAct("kill:%1", ""))
	if res.Err != "the kill waits for the builder's go: it is on the card, y gives it" || len(*ran) != 0 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	run := s.Stoker.View().Run
	e := run[len(run)-1]
	if e.Rule != stoker.RuleGo || e.State != stoker.StepWaits || e.Needs != "kill:%1" || e.Text != "stoker wants to kill work" {
		t.Fatalf("card %+v", e)
	}
	// Asked twice, the card asks once.
	s.act(t.Context(), fromStokerAct("kill:%1", ""))
	if n := len(s.Stoker.View().Run); n != len(run) {
		t.Fatalf("the card asks twice: %d steps, was %d", n, len(run))
	}
	if res := s.act(t.Context(), consoleAct("stoker:grant", e.ID)); res.Err != "" {
		t.Fatal(res.Err)
	}
	if res := s.act(t.Context(), fromStokerAct("kill:%1", "")); res.Err != "" || len(*ran) != 1 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	// The go was good for one kill.
	s.publish(sample.Snapshot{At: night0, Sessions: []sample.Session{
		{Agent: "claude", Pane: "%1", SessionID: "s1", Name: "work", Status: "idle", Idle: 5 * time.Minute},
		{Agent: "claude", Pane: "%9@mbtest", SessionID: "s9", Name: "stoker", Status: "busy", Busy: true},
	}}, func(state.Doc) {})
	if res := s.act(t.Context(), fromStokerAct("kill:%1", "")); !strings.HasPrefix(res.Err, "the kill waits") {
		t.Fatalf("a second kill on one go: %q", res.Err)
	}
}

func TestStokerKillsWithTheKillsGo(t *testing.T) {
	s, ran := handsLife(t)
	if _, err := s.Stoker.Update(func(m stoker.Mode) (stoker.Mode, bool) {
		m.Order = &stoker.Order{ID: "run-1", State: stoker.OrderRunning, Grants: []string{"kill"}, At: night0, Went: night0}
		return m, true
	}); err != nil {
		t.Fatal(err)
	}
	if res := s.act(t.Context(), fromStokerAct("kill:%1", "")); res.Err != "" || len(*ran) != 1 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
}

func TestStokerNeverKillsWorkWaitsOrParked(t *testing.T) {
	s, ran := handsLife(t)
	if _, err := s.Stoker.Update(func(m stoker.Mode) (stoker.Mode, bool) {
		m.Order = &stoker.Order{ID: "run-1", State: stoker.OrderRunning, Grants: []string{"kill"}, At: night0, Went: night0}
		return m, true
	}); err != nil {
		t.Fatal(err)
	}
	snap := sample.Snapshot{At: night0, Sessions: []sample.Session{
		{Agent: "claude", Pane: "%1", SessionID: "s1", Name: "work", Status: "busy", Busy: true},
		{Agent: "claude", Pane: "%5", SessionID: "s5", Name: "parked", Status: "idle", Idle: time.Hour},
		{Agent: "claude", Pane: "%9@mbtest", SessionID: "s9", Name: "stoker", Status: "busy", Busy: true},
	}}
	s.publish(snap, func(state.Doc) {})
	if err := s.Holds.Set(holds.Hold{SessionID: "s5", Pane: "%5", Name: "parked", By: holds.ByYou, At: night0}); err != nil {
		t.Fatal(err)
	}
	for pane, why := range map[string]string{
		"%1": "work works: the stoker kills only a session at rest",
		"%5": "parked by you: the stoker leaves it alone",
	} {
		if res := s.act(t.Context(), fromStokerAct("kill:"+pane, KillAnyway)); res.Err != why {
			t.Errorf("%s: %q", pane, res.Err)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

func TestStokerStartsSessionsOnTheBuildersServerAndDrivesThem(t *testing.T) {
	s, ran := handsLife(t)
	b, _ := json.Marshal(proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "n88", First: "work on #88"})
	res := s.act(t.Context(), fromStokerAct("new:session", string(b)))
	if res.Err != "" || len(*ran) != 1 || slices.Contains((*ran)[0], "-L") {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	es := s.Stoker.Entries()
	if e := es[len(es)-1]; !e.Ordered() || e.Pane != "%42" || e.Name != "n88" {
		t.Fatalf("not counted: %+v", e)
	}
	if !s.agentsPane(proto.Act{From: "%42"}) {
		t.Fatal("the session the stoker started is not one it drives")
	}
}

func TestStokerStartsNothingWithNoRun(t *testing.T) {
	s, ran := handsLife(t)
	if err := s.Stoker.Set(stoker.Mode{}); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "n88"})
	for _, a := range []proto.Act{fromStokerAct("new:session", string(b)), fromStokerAct("kill:%1", ""), fromStokerAct("rename:%1", "x")} {
		if res := s.act(t.Context(), a); res.Err != "the stoker is off: no run" {
			t.Errorf("%s: %q", a.RecID, res.Err)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

func TestKillOfACodexAtRestTakesOneY(t *testing.T) {
	s, ran := lifeTest(t)
	if res := s.act(t.Context(), consoleAct("kill:%3", "")); res.Err != "" || len(*ran) != 2 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
}

// TestANewSessionIsSampledAtOnce: the service knows it made a pane, so it
// samples now instead of at the next tick.
func TestANewSessionIsSampledAtOnce(t *testing.T) {
	s, _ := lifeTest(t) // a tick of an hour
	var samples atomic.Int32
	s.sample = func() sample.Snapshot {
		samples.Add(1)
		snap := lifeSnap(false)
		snap.At = time.Now()
		return snap
	}
	ctx := run(t, s)
	c, _ := connect(t, ctx, s)
	snapshot(t, c)
	before := samples.Load()
	if r := act(t, c, "n", newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "fresh"})); r.Err != "" {
		t.Fatalf("%+v", r)
	}
	deadline := time.Now().Add(2 * time.Second)
	for samples.Load() == before {
		if time.Now().After(deadline) {
			t.Fatal("no sample after the new session: it waits for the tick")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestLaunchArgvOfEachAgent(t *testing.T) {
	for _, c := range []struct {
		bin, agent, first string
		want              []string
	}{
		{"claude", "claude", "go", []string{"claude", "--name", "work", "go"}},
		{"codex", "codex", "", []string{"codex"}},
		{"opencode", "opencode", "go", []string{"opencode", "--prompt", "go"}},
		{"agent", sample.CursorAgent, "go", []string{"agent", "--trust", "go"}},
		{"cursor-agent", sample.CursorAgent, "", []string{"cursor-agent", "--trust"}},
	} {
		if got := launchArgv(c.bin, c.agent, "work", c.first, nil); !slices.Equal(got, c.want) {
			t.Errorf("%s %q: got %q, want %q", c.agent, c.first, got, c.want)
		}
	}
}

func TestNewStartsCursor(t *testing.T) {
	for _, agent := range []string{"cursor-agent", "cursor"} {
		s, ran := lifeTest(t)
		dir := t.TempDir()
		res := s.act(t.Context(), newAct(t, proto.NewSession{Where: dir, Agent: agent, Name: "fresh", First: "fix the build"}))
		if res.Err != "" || len(*ran) != 1 {
			t.Fatalf("%s: err %q ran %q", agent, res.Err, *ran)
		}
		if got := strings.Join((*ran)[0], " "); !strings.HasSuffix(got, "-c "+dir+" cursor-agent --trust fix the build") {
			t.Errorf("%s: ran %q", agent, got)
		}
	}
}

// TestNewStartsCursorByAgentWhenOnlyThatIsInstalled: Cursor's installer
// adds both commands, a hand install may add only `agent`.
func TestNewStartsCursorByAgentWhenOnlyThatIsInstalled(t *testing.T) {
	s, ran := lifeTest(t)
	agent := cursorInstall(t)
	s.LookPath = func(cmd string) (string, error) {
		switch cmd {
		case "cursor-agent":
			return "", exec.ErrNotFound
		case "agent":
			return agent, nil
		}
		return "/usr/bin/" + cmd, nil
	}
	dir := t.TempDir()
	if res := s.act(t.Context(), newAct(t, proto.NewSession{Where: dir, Agent: "cursor-agent", Name: "fresh", First: "fix the build"})); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := strings.Join((*ran)[0], " "); !strings.HasSuffix(got, "-c "+dir+" agent --trust fix the build") {
		t.Errorf("ran %q", got)
	}
	if !slices.Contains(s.startable(), "cursor-agent") {
		t.Errorf("startable %q, want cursor-agent", s.startable())
	}
}

// cursorInstall is Cursor's installer's `agent`: a link into its versions.
func cursorInstall(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "share", "cursor-agent", "versions", "2026.10.19-6bbf2c4", "cursor-agent")
	link := filepath.Join(home, ".local", "bin", "agent")
	for _, d := range []string{filepath.Dir(bin), filepath.Dir(link)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(bin, link); err != nil {
		t.Fatal(err)
	}
	return link
}

// TestAnotherAgentIsNotCursor: "agent" is a common name. Only Cursor's
// own counts for Cursor.
func TestAnotherAgentIsNotCursor(t *testing.T) {
	s, ran := lifeTest(t)
	other := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(other, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	s.LookPath = func(cmd string) (string, error) {
		switch cmd {
		case "cursor-agent":
			return "", exec.ErrNotFound
		case "agent":
			return other, nil
		}
		return "/usr/bin/" + cmd, nil
	}
	if slices.Contains(s.startable(), "cursor-agent") {
		t.Errorf("startable %q offers cursor-agent for another agent", s.startable())
	}
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "cursor-agent", Name: "fresh"}))
	if res.Err != "cursor-agent is not installed here" || len(*ran) != 0 {
		t.Errorf("err %q ran %q, want it not installed", res.Err, *ran)
	}
}

// switchTest is lifeTest with holds kept and /work/app there.
func switchTest(t *testing.T) (*Service, *[][]string) {
	t.Helper()
	s, ran := lifeTest(t)
	s.IsDir = func(path string) bool { return path == "/work/app" }
	hs, err := holds.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.Holds = hs
	return s, ran
}

func switchAct(t *testing.T, pane string, n proto.SwitchAgent) proto.Act {
	t.Helper()
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	return consoleAct("switch:"+pane, string(b))
}

func TestSwitchRespawnsTheAgentInItsPane(t *testing.T) {
	s, ran := switchTest(t)
	res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex", Handoff: proto.DefaultHandoff}))
	want := [][]string{{"tmux", "respawn-pane", "-k", "-t", "%1", "-c", "/work/app",
		"env", "-u", "GIT_TERMINAL_PROMPT", "-u", "GIT_SSH_COMMAND", "-u", panes.HomeEnv, "codex", proto.DefaultHandoff}}
	if res.Err != "" || !slices.EqualFunc(*ran, want, slices.Equal) {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	for _, step := range *ran {
		if slices.Contains(step, "kill-pane") || slices.Contains(step, "new-session") {
			t.Fatalf("a switch closed or opened a pane: %q", *ran)
		}
	}
}

func TestSwitchToClaudeKeepsTheName(t *testing.T) {
	s, ran := switchTest(t)
	snap := lifeSnap(false)
	snap.Sessions[2].Cwd = "/work/app"
	s.publish(snap, func(state.Doc) {})
	if res := s.act(t.Context(), switchAct(t, "%3", proto.SwitchAgent{Agent: "claude"})); res.Err != "" || len(*ran) != 1 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	if got := strings.Join((*ran)[0], " "); !strings.HasSuffix(got, " claude --name cx") {
		t.Fatalf("ran %q", got)
	}
}

// TestTheSeatKeepsItsNameThroughSwitches: the sampler names an agent that
// keeps no name of its own as its tmux window (see
// TestAnAgentWithoutANameIsCalledAsItsTmuxWindow). A seat whose window tmux names after its command is named for the switch,
// so Codex there reads as the seat, and a switch back starts Claude with
// the seat's name.
func TestTheSeatKeepsItsNameThroughSwitches(t *testing.T) {
	for _, c := range []struct {
		window string
		rename bool
	}{
		{"work", false},  // new and r name the window as the seat
		{"claude", true}, // tmux names it after the command it runs
	} {
		s, ran := switchTest(t)
		snap := lifeSnap(false)
		snap.Panes[0].Window = c.window
		s.publish(snap, func(state.Doc) {})
		if res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex"})); res.Err != "" {
			t.Fatalf("%s: %+v", c.window, res)
		}
		steps := 1
		if c.rename {
			steps = 2
		}
		named := slices.Equal((*ran)[0], []string{"tmux", "rename-window", "-t", "%1", "work"})
		if named != c.rename || len(*ran) != steps {
			t.Fatalf("%s: ran %q", c.window, *ran)
		}
		// Codex runs in the seat, named as its window, which says work now.
		snap = lifeSnap(false)
		snap.Sessions[0] = sample.Session{Agent: "codex", PID: 201, Start: 9, Pane: "%1", Target: "app:1.1", Cwd: "/work/app", Name: "work", Status: "idle"}
		snap.Panes[0].Command = "codex"
		s.publish(snap, func(state.Doc) {})
		*ran = nil
		if res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "claude"})); res.Err != "" || len(*ran) != 1 {
			t.Fatalf("%s: back to claude: %+v ran %q", c.window, res, *ran)
		}
		if got := strings.Join((*ran)[0], " "); !strings.HasSuffix(got, " claude --name work") {
			t.Fatalf("%s: back to claude ran %q", c.window, got)
		}
	}
}

func TestSwitchToCursorTrusts(t *testing.T) {
	s, ran := switchTest(t)
	if res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "cursor", Handoff: "go on"})); res.Err != "" || len(*ran) != 1 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	if got := strings.Join((*ran)[0], " "); !strings.HasSuffix(got, " cursor-agent --trust go on") {
		t.Fatalf("ran %q", got)
	}
}

func TestSwitchAsksAgainWhileItWorks(t *testing.T) {
	s, ran := switchTest(t)
	res := s.act(t.Context(), switchAct(t, "%2", proto.SwitchAgent{Agent: "codex"}))
	if res.Err != "busy works: y again switches it mid-turn" || len(*ran) != 0 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	if res := s.act(t.Context(), switchAct(t, "%2", proto.SwitchAgent{Agent: "codex", Anyway: true})); res.Err != "" || len(*ran) != 1 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
}

func TestSwitchRefuses(t *testing.T) {
	s, ran := switchTest(t)
	snap := lifeSnap(false)
	snap.Sessions = append(snap.Sessions, sample.Session{Agent: "claude", PID: 109, Pane: "%9@mbtest", Name: "stoker", Status: "idle", Cwd: "/work/app"})
	s.publish(snap, func(state.Doc) {})
	unconfirmed := switchAct(t, "%1", proto.SwitchAgent{Agent: "codex"})
	unconfirmed.Confirm = ""
	for _, c := range []struct {
		a   proto.Act
		why string
	}{
		{unconfirmed, "needs confirm"},
		{switchAct(t, "%1", proto.SwitchAgent{Agent: "claude"}), "work runs claude already"},
		{switchAct(t, "%1", proto.SwitchAgent{Agent: "gpt"}), "gpt is not an agent matchblox starts: claude, codex, opencode or cursor-agent"},
		{switchAct(t, "%1", proto.SwitchAgent{Agent: "opencode"}), "opencode is not installed here"},
		{switchAct(t, "%77", proto.SwitchAgent{Agent: "codex"}), "no agent session in %77 any more"},
		{switchAct(t, "%9@mbtest", proto.SwitchAgent{Agent: "codex"}), "the stoker's session keeps its agent"},
		{switchAct(t, "%1", proto.SwitchAgent{Agent: "codex", Handoff: "--yolo"}), "the handoff does not start with -"},
		{switchAct(t, "%1", proto.SwitchAgent{Agent: "codex", Handoff: "a\x1bb"}), "the handoff must be one line, without control keys"},
	} {
		if res := s.act(t.Context(), c.a); res.Err != c.why {
			t.Errorf("%s %s: want %q, got %q", c.a.RecID, c.a.Text, c.why, res.Err)
		}
	}
	s.IsDir = func(string) bool { return false }
	if res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex"})); res.Err != "no directory /work/app" {
		t.Errorf("want no directory, got %q", res.Err)
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}

	// A session the stoker drives does not switch another's agent.
	h, hran := handsLife(t)
	h.drive("%4")
	b, _ := json.Marshal(proto.SwitchAgent{Agent: "codex"})
	res := h.act(t.Context(), proto.Act{RecID: "switch:%1", Confirm: "y", Text: string(b), From: "%4"})
	if res.Err != "the stoker does not switch agents: the builder does" || len(*hran) != 0 {
		t.Fatalf("err %q ran %q", res.Err, *hran)
	}
}

// lifeStoker turns a stoker run on in a lifeTest service.
func lifeStoker(t *testing.T, s *Service) {
	t.Helper()
	st, err := stoker.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.Stoker = st
	s.Activity = func() time.Time { return time.Time{} }
	if err := st.Set(stoker.Mode{}.TurnOn(night0)); err != nil {
		t.Fatal(err)
	}
}

func TestSwitchTakesTheSeatFromTheStoker(t *testing.T) {
	s, _ := switchTest(t)
	lifeStoker(t, s)
	s.drive("%1")
	if !s.agentsPane(proto.Act{From: "%1"}) {
		t.Fatal("the stoker does not drive %1")
	}
	if res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex"})); res.Err != "" || len(res.Ran) != 1 {
		t.Fatalf("%+v", res)
	}
	if s.agentsPane(proto.Act{From: "%1"}) {
		t.Fatal("the stoker still drives the switched seat")
	}
}

func TestSwitchKeepsTheParkOnTheSeat(t *testing.T) {
	s, _ := switchTest(t)
	if res := s.act(t.Context(), consoleAct("park:%1", "")); res.Err != "" {
		t.Fatal(res.Err)
	}
	if res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex"})); res.Err != "" || len(res.Ran) != 1 {
		t.Fatalf("%+v", res)
	}
	if h, ok := s.holdAt("%1"); !ok || h.By != holds.ByYou {
		t.Fatalf("the park left the seat: %+v %v", h, ok)
	}
	// The new agent has no session id yet: the seat is still parked.
	snap := lifeSnap(false)
	snap.Sessions[0] = sample.Session{Agent: "codex", PID: 201, Start: 9, Pane: "%1", Target: "app:1.1", Cwd: "/work/app", Name: "work", Status: "idle"}
	s.publish(snap, func(state.Doc) {})
	if err := s.messageable("%1", 201); err == nil || err.Error() != "work is parked: z lets it go" {
		t.Fatalf("messageable %v", err)
	}
	if h, ok := s.holdAt("%1"); !ok || h.By != holds.ByYou {
		t.Fatalf("no hold at the seat before its id: %+v %v", h, ok)
	}
	if !s.heldSessions()["s1"] {
		t.Fatalf("held %v", s.heldSessions())
	}
	// Its id shows: the park follows it.
	snap.Sessions[0].SessionID = "s9"
	s.publish(snap, func(state.Doc) {})
	if _, ok := s.Holds.Of("s9"); !ok {
		t.Fatalf("the park did not follow the new agent: %+v", s.Holds.List())
	}
	if _, ok := s.Holds.Of("s1"); ok {
		t.Fatalf("the old session is still parked: %+v", s.Holds.List())
	}
}

func TestASwitchTheGuardStopsLeavesTheHoldAsItWas(t *testing.T) {
	s, ran := switchTest(t)
	if res := s.act(t.Context(), consoleAct("park:%1", "")); res.Err != "" {
		t.Fatal(res.Err)
	}
	s.Actions.Check = func(advice.Guard) error { return errors.New("another process has pid 101 now") }
	res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex"}))
	if len(res.Ran) != 0 || len(*ran) != 0 {
		t.Fatalf("%+v ran %q", res, *ran)
	}
	if _, pending := s.Holds.Pending("%1"); pending {
		t.Fatal("a stopped switch left the hold waiting for a new agent")
	}
	if h, ok := s.Holds.Of("s1"); !ok || h.Rebind {
		t.Fatalf("the park is not as it was: %+v %v", h, ok)
	}
}

func TestASwitchForgetsTheOldTurn(t *testing.T) {
	s, _ := switchTest(t)
	s.Hook(hooks.Event{Name: "Stop", SessionID: "s1", Pane: "%1", At: night0})
	if _, ok := s.queued("%1"); !ok {
		t.Fatal("the turn does not wait")
	}
	if res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex"})); res.Err != "" || len(res.Ran) != 1 {
		t.Fatalf("%+v", res)
	}
	snap := lifeSnap(false)
	snap.Sessions[0] = sample.Session{Agent: "codex", PID: 201, Start: 9, Pane: "%1", Target: "app:1.1", Cwd: "/work/app", Name: "work", Status: "idle"}
	s.publish(snap, func(state.Doc) {})
	// The new agent at rest with no hook yet is guessed to wait, as any is;
	// the old agent's hooked turn is gone.
	if it, ok := s.queued("%1"); ok && !it.Estimated {
		t.Fatalf("the old turn still waits: %+v", it)
	}
}

// TestASwitchTheGuardStopsAfterTheRenameFails: the window was named, then
// the guard stopped the respawn. The old agent still runs, so the switch
// says it did not happen and the hold is as it was.
func TestASwitchTheGuardStopsAfterTheRenameFails(t *testing.T) {
	s, ran := switchTest(t)
	snap := lifeSnap(false)
	snap.Panes[0].Window = "claude"
	s.publish(snap, func(state.Doc) {})
	if res := s.act(t.Context(), consoleAct("park:%1", "")); res.Err != "" {
		t.Fatal(res.Err)
	}
	checks := 0
	s.Actions.Check = func(advice.Guard) error {
		if checks++; checks == 2 {
			return errors.New("another process has pid 101 now")
		}
		return nil
	}
	res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex"}))
	if res.Err != "another process has pid 101 now" || len(res.Skipped) != 0 || len(*ran) != 1 {
		t.Fatalf("%+v ran %q", res, *ran)
	}
	if _, pending := s.Holds.Pending("%1"); pending {
		t.Fatal("a stopped switch left the hold waiting for a new agent")
	}
	if h, ok := s.Holds.Of("s1"); !ok || h.Rebind {
		t.Fatalf("the park is not as it was: %+v %v", h, ok)
	}
}

// TestASharedWindowKeepsItsName: a window tmux named after its command
// that holds another pane too is not the seat's to rename. A pane of the
// same name on another server does not share it.
func TestASharedWindowKeepsItsName(t *testing.T) {
	for _, c := range []struct {
		other  string
		rename bool
	}{
		{"%5", false},
		{"%5@other", true},
	} {
		s, ran := switchTest(t)
		snap := lifeSnap(false)
		snap.Panes[0].Window = "claude"
		snap.Panes = append(snap.Panes, sample.Pane{ID: c.other, Target: "app:1.2", Window: "claude", Command: "zsh"})
		s.publish(snap, func(state.Doc) {})
		if res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex"})); res.Err != "" {
			t.Fatalf("%s: %+v", c.other, res)
		}
		named := slices.Contains((*ran)[0], "rename-window")
		if named != c.rename {
			t.Fatalf("%s: ran %q", c.other, *ran)
		}
	}
}

// TestASwitchHearsTheNewAgentThatHooksInAtOnce: the new agent's first hook
// can land before the switch returns. Only the old agent's ids are
// forgotten, so the new one is heard.
func TestASwitchHearsTheNewAgentThatHooksInAtOnce(t *testing.T) {
	s, _ := switchTest(t)
	s.Hook(hooks.Event{Name: "Stop", SessionID: "s1", Pane: "%1", At: night0})
	run := s.Actions.Run
	s.Actions.Run = func(argv []string) error {
		if err := run(argv); err != nil {
			return err
		}
		if slices.Contains(argv, "respawn-pane") {
			s.Hook(hooks.Event{Name: "Stop", SessionID: "s9", Pane: "%1", At: night0.Add(time.Second)})
		}
		return nil
	}
	if res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex"})); res.Err != "" {
		t.Fatalf("%+v", res)
	}
	snap := lifeSnap(false)
	snap.At = night0.Add(2 * time.Second)
	snap.Sessions[0] = sample.Session{Agent: "codex", PID: 201, Start: 9, Pane: "%1", Target: "app:1.1", Cwd: "/work/app", Name: "work", SessionID: "s9", Status: "idle"}
	s.publish(snap, func(state.Doc) {})
	if it, ok := s.queued("%1"); !ok || it.Estimated || it.SessionID != "s9" {
		t.Fatalf("the new agent's hook was forgotten: %+v %v", it, ok)
	}
}

// accountTest is lifeTest with the builder's home in a temp dir.
func accountTest(t *testing.T) (*Service, *[][]string, string) {
	t.Helper()
	s, ran := lifeTest(t)
	// Short: codex refuses a folder whose socket path would be too long.
	home, err := os.MkdirTemp("/tmp", "mb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	s.Home = home
	s.GOOS = "linux"
	return s, ran, home
}

func prepared(t *testing.T, home, agent, name string) string {
	t.Helper()
	folder := accounts.Folder(home, agent, name)
	if err := accounts.Prepare(home, agent, folder, accounts.Empty); err != nil {
		t.Fatal(err)
	}
	return folder
}

func TestStartOnAnAccount(t *testing.T) {
	s, ran, home := accountTest(t)
	folder := prepared(t, home, "claude", "work")
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "fresh", Account: "work"}))
	if res.Err != "" || len(*ran) != 1 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	got := strings.Join((*ran)[0], " ")
	if !strings.HasSuffix(got, envOn("claude")+" CLAUDE_CONFIG_DIR="+folder+" claude --name fresh") || strings.Contains(got, "BROWSER") {
		t.Fatalf("ran %q", got)
	}
}

func TestUnknownAccountIsRefused(t *testing.T) {
	s, ran, _ := accountTest(t)
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Account: "nope"}))
	if !strings.Contains(res.Err, "no claude account nope") || len(*ran) != 0 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
}

func TestNewAccountPreparesAndStartsWithoutFirst(t *testing.T) {
	s, ran, home := accountTest(t)
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "fresh", Account: "work2", NewAccount: true, Setup: "copy", First: "x"}))
	if res.Err != "" || len(*ran) != 1 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	folder := accounts.Folder(home, "claude", "work2")
	if fi, err := os.Stat(folder); err != nil || !fi.IsDir() {
		t.Fatalf("no folder: %v", err)
	}
	got := strings.Join((*ran)[0], " ")
	if !strings.Contains(got, "BROWSER=/usr/bin/true") || !strings.HasSuffix(got, " claude --name fresh") {
		t.Fatalf("ran %q", got)
	}
}

func TestNewAccountEmptySetupMeansLikeDefaultAndUnknownIsRefused(t *testing.T) {
	s, ran, home := accountTest(t)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Account: "w", NewAccount: true}))
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	if _, err := os.Lstat(filepath.Join(accounts.Folder(home, "claude", "w"), "settings.json")); err != nil {
		t.Fatalf("like-default links the default's settings: %v", err)
	}
	n := len(*ran)
	res = s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Account: "w3", NewAccount: true, Setup: "bogus"}))
	for _, m := range []string{"like-default", "copy", "empty"} {
		if !strings.Contains(res.Err, m) {
			t.Fatalf("err %q does not name %s", res.Err, m)
		}
	}
	if len(*ran) != n {
		t.Fatalf("ran %q", *ran)
	}
	if _, err := os.Stat(accounts.Folder(home, "claude", "w3")); err == nil {
		t.Fatal("a refused setup wrote a folder")
	}
}

func TestNewAccountAlreadyThereIsRefused(t *testing.T) {
	s, _, home := accountTest(t)
	prepared(t, home, "claude", "work")
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Account: "work", NewAccount: true}))
	if !strings.Contains(res.Err, "work") || !strings.Contains(res.Err, "already") {
		t.Fatalf("err %q", res.Err)
	}
}

func TestDryNewAccountWritesNothing(t *testing.T) {
	s, ran, home := accountTest(t)
	a := newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Account: "work", NewAccount: true})
	a.Confirm = ""
	res := s.act(t.Context(), a)
	if res.Err == "" || len(*ran) != 0 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	if _, err := os.Stat(accounts.Root(home)); err == nil {
		t.Fatal("a dry plan wrote the account folder")
	}
}

func TestNewAccountLabelNamesTheSetup(t *testing.T) {
	got := newAccountLabel("claude", "work", "copy")
	if want := `new claude account "work", copy: sign in in the session`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCodexAccountForcesFileStore(t *testing.T) {
	s, ran, home := accountTest(t)
	folder := prepared(t, home, "codex", "work")
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "codex", Name: "fresh", Account: "work"}))
	if res.Err != "" || len(*ran) != 1 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	got := strings.Join((*ran)[0], " ")
	if !strings.HasSuffix(got, envOn("codex")+" CODEX_HOME="+folder+" codex -c cli_auth_credentials_store=\"file\"") {
		t.Fatalf("ran %q", got)
	}
}

func TestNewAccountStepsNeverNameTheDefault(t *testing.T) {
	for _, agent := range []string{"claude", "codex", "cursor-agent"} {
		s, ran, home := accountTest(t)
		s.GOOS = "darwin" // Cursor accounts run on macOS only
		res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: agent, Name: "fresh", Account: "work", NewAccount: true}))
		if res.Err != "" {
			t.Fatalf("%s: %s", agent, res.Err)
		}
		for _, step := range *ran {
			got := strings.Join(step, " ")
			for _, bad := range []string{home + "/.claude", home + "/.codex", home + "/.cursor/auth.json", "security"} {
				if strings.Contains(got, bad) {
					t.Errorf("%s: step %q names %q", agent, got, bad)
				}
			}
		}
	}
}

func TestAccountEnvIsOnTheCommandNotTheSession(t *testing.T) {
	for _, agent := range []string{"claude", "codex", "cursor-agent"} {
		s, ran, _ := accountTest(t)
		s.GOOS = "darwin" // Cursor accounts run on macOS only
		res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: agent, Name: "fresh", Account: "work", NewAccount: true}))
		if res.Err != "" {
			t.Fatalf("%s: %s", agent, res.Err)
		}
		for _, step := range *ran {
			for _, bad := range []string{"-e", "set-environment", "setenv"} {
				if slices.Contains(step, bad) {
					t.Errorf("%s: step %q has %s", agent, step, bad)
				}
			}
		}
	}
}

func TestDefaultStartsAsToday(t *testing.T) {
	for _, account := range []string{"", "default"} {
		s, ran, home := accountTest(t)
		dir := t.TempDir()
		res := s.act(t.Context(), newAct(t, proto.NewSession{Where: dir, Agent: "claude", Name: "fresh", Account: account, First: "go"}))
		if res.Err != "" || len(*ran) != 1 {
			t.Fatalf("%q: err %q ran %q", account, res.Err, *ran)
		}
		got := strings.Join((*ran)[0], " ")
		want := "env -u GIT_TERMINAL_PROMPT -u GIT_SSH_COMMAND -u MATCHBLOX_HOME tmux new-session -d -s fresh -n fresh -c " + dir + " claude --name fresh go"
		if got != want {
			t.Errorf("%q: got %q, want %q", account, got, want)
		}
		if _, err := os.Stat(accounts.Root(home)); err == nil {
			t.Error("the default made an account folder")
		}
	}
}

func TestOpencodeHasNoAccounts(t *testing.T) {
	s, ran, _ := accountTest(t)
	s.LookPath = func(cmd string) (string, error) { return "/usr/bin/" + cmd, nil }
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "opencode", Account: "work"}))
	if !strings.Contains(res.Err, "opencode has no accounts") || len(*ran) != 0 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
}

func TestStartOnAHandMadeFolder(t *testing.T) {
	s, ran, home := accountTest(t)
	old := filepath.Join(home, ".claude-old")
	snap := lifeSnap(false)
	snap.Sessions[0].AccountDir = old
	s.publish(snap, func(state.Doc) {})
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "fresh", Account: "~/.claude-old"}))
	if res.Err != "" || len(*ran) != 1 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	got := strings.Join((*ran)[0], " ")
	if !strings.Contains(got, envOn("claude")+" CLAUDE_CONFIG_DIR="+old+" claude") || strings.Contains(got, "BROWSER=") {
		t.Fatalf("ran %q", got)
	}
}

// TestHandMadeCodexFolderKeepsItsStore: a hand-made Codex folder may keep
// its sign-in in the keyring, so its start does not force the file store.
func TestHandMadeCodexFolderKeepsItsStore(t *testing.T) {
	s, ran, home := accountTest(t)
	old := filepath.Join(home, ".codex-old")
	snap := lifeSnap(false)
	snap.Sessions[2].AccountDir = old
	s.publish(snap, func(state.Doc) {})
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "codex", Name: "fresh", Account: "~/.codex-old"}))
	if res.Err != "" || len(*ran) != 1 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	got := strings.Join((*ran)[0], " ")
	if !strings.HasSuffix(got, envOn("codex")+" CODEX_HOME="+old+" codex") || strings.Contains(got, "cli_auth_credentials_store") {
		t.Fatalf("ran %q", got)
	}
}

func TestUnknownFolderIsRefused(t *testing.T) {
	s, ran, home := accountTest(t)
	snap := lifeSnap(false)
	snap.Sessions[2].AccountDir = filepath.Join(home, ".codex-old") // another agent's
	s.publish(snap, func(state.Doc) {})
	for _, acct := range []string{"~/.claude-old", home + "/.claude-old", "/etc"} {
		res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "fresh", Account: acct}))
		if !strings.Contains(res.Err, "no claude account") {
			t.Fatalf("%s: err %q", acct, res.Err)
		}
	}
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "codex", Name: "fresh", Account: "~/.codex-old", NewAccount: true}))
	if res.Err == "" || len(*ran) != 0 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
}

func TestCursorAccountsRunOnMacOnly(t *testing.T) {
	s, ran, home := accountTest(t) // linux
	prepared(t, home, "cursor-agent", "work")
	for _, n := range []proto.NewSession{
		{Agent: "cursor-agent", Account: "work"},
		{Agent: "cursor-agent", Account: "new1", NewAccount: true},
		{Agent: "cursor", Account: "work"},
	} {
		n.Where, n.Name = t.TempDir(), "fresh"
		res := s.act(t.Context(), newAct(t, n))
		if res.Err != "Cursor accounts run on macOS only for now" {
			t.Fatalf("%+v: err %q", n, res.Err)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
	if _, err := os.Stat(accounts.Folder(home, "cursor-agent", "new1")); err == nil {
		t.Fatal("a refused Cursor account wrote a folder")
	}
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "cursor-agent", Name: "fresh", Account: "default"}))
	if res.Err != "" {
		t.Fatalf("the default: %s", res.Err)
	}
	s.GOOS = "darwin"
	res = s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "cursor-agent", Name: "mac", Account: "work"}))
	if res.Err != "" {
		t.Fatalf("on macOS: %s", res.Err)
	}
}

// writeDefault writes a file in the builder's default folder of an agent.
func writeDefault(t *testing.T, home, rel string) {
	t.Helper()
	path := filepath.Join(home, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLikeDefaultAccountGetsNewEntriesOnStart(t *testing.T) {
	s, _, home := accountTest(t)
	writeDefault(t, home, ".claude/settings.json")
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "one", Account: "work", NewAccount: true}))
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	writeDefault(t, home, ".claude/CLAUDE.md")
	res = s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "two", Account: "work"}))
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	got, err := os.Readlink(filepath.Join(accounts.Folder(home, "claude", "work"), "CLAUDE.md"))
	if err != nil || got != filepath.Join(home, ".claude", "CLAUDE.md") {
		t.Fatalf("CLAUDE.md: link %q, %v", got, err)
	}
}

func TestCopyAccountIsNotLinkedAgain(t *testing.T) {
	s, _, home := accountTest(t)
	writeDefault(t, home, ".codex/config.toml")
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "codex", Name: "one", Account: "work", NewAccount: true, Setup: "copy"}))
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	writeDefault(t, home, ".codex/AGENTS.md")
	res = s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "codex", Name: "two", Account: "work"}))
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	if _, err := os.Lstat(filepath.Join(accounts.Folder(home, "codex", "work"), "AGENTS.md")); err == nil {
		t.Fatal("a copy account got a new entry of the default")
	}
}

func TestAccountWithNoMarkerIsLikeDefault(t *testing.T) {
	s, _, home := accountTest(t)
	folder := accounts.Folder(home, "claude", "old")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	writeDefault(t, home, ".claude/settings.json")
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "two", Account: "old"}))
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	if _, err := os.Readlink(filepath.Join(folder, "settings.json")); err != nil {
		t.Fatalf("an account made before the marker was not linked: %v", err)
	}
}

func TestFailedNewAccountLeavesNoFolder(t *testing.T) {
	t.Run("the start fails", func(t *testing.T) {
		s, _, home := accountTest(t)
		s.Actions.Run = func([]string) error { return errors.New("no tmux") }
		res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "fresh", Account: "work", NewAccount: true}))
		if res.Err == "" {
			t.Fatal("the start did not fail")
		}
		if _, err := os.Lstat(accounts.Folder(home, "claude", "work")); err == nil {
			t.Fatal("the folder stayed")
		}
		s.Actions.Run = func([]string) error { return nil }
		if res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "fresh", Account: "work", NewAccount: true})); res.Err != "" {
			t.Fatalf("the retry: %s", res.Err)
		}
	})
	t.Run("the copy fails", func(t *testing.T) {
		s, ran, home := accountTest(t)
		writeDefault(t, home, ".claude/settings.json")
		if err := os.Chmod(filepath.Join(home, ".claude", "settings.json"), 0); err != nil {
			t.Fatal(err)
		}
		res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "fresh", Account: "work", NewAccount: true, Setup: "copy"}))
		if res.Err == "" || len(*ran) != 0 {
			t.Fatalf("err %q ran %q", res.Err, *ran)
		}
		if _, err := os.Lstat(accounts.Folder(home, "claude", "work")); err == nil {
			t.Fatal("the folder stayed")
		}
	})
}

func TestStartOnAHandMadeFolderByItsFullPath(t *testing.T) {
	s, ran, home := accountTest(t)
	old := filepath.Join(home, ".claude-old")
	snap := lifeSnap(false)
	snap.Sessions[0].AccountDir = old
	s.publish(snap, func(state.Doc) {})
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "fresh", Account: old + "/"}))
	if res.Err != "" || len(*ran) != 1 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	if got := strings.Join((*ran)[0], " "); !strings.Contains(got, envOn("claude")+" CLAUDE_CONFIG_DIR="+old+" claude") {
		t.Fatalf("ran %q", got)
	}
}

func TestNewAccountWithNoPaneSaysByHand(t *testing.T) {
	s, _, _ := accountTest(t)
	s.TmuxOut = func([]string) (string, error) { return "", errors.New("no server") }
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "fresh", Account: "work", NewAccount: true}))
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := s.State().SignIns["fresh"]; got != SignInByHand {
		t.Fatalf("notes %v", s.State().SignIns)
	}
	snap := lifeSnap(false)
	snap.Panes = append(snap.Panes, sample.Pane{ID: "%50", Target: "fresh:1.1"})
	s.publish(snap, func(state.Doc) {})
	if got := s.State().SignIns["fresh"]; got != SignInByHand {
		t.Fatalf("the note went while its session is here: %v", s.State().SignIns)
	}
	s.publish(lifeSnap(false), func(state.Doc) {})
	if n := s.State().SignIns; n != nil {
		t.Fatalf("the note stayed after its session went: %v", n)
	}
}

// TestTwoConfirmsOfOneNewAccountMakeItOnce: two confirms of one new account
// at once start it once. Both pass the first check before either makes the
// folder; the second is refused as there already, and the first's folder
// stays.
func TestTwoConfirmsOfOneNewAccountMakeItOnce(t *testing.T) {
	s, _, home := accountTest(t)
	// Each act looks up its agent after it checks the account: hold both
	// there until both are in flight.
	var arrived atomic.Int32
	var timedOut atomic.Bool
	both := make(chan struct{})
	s.LookPath = func(cmd string) (string, error) {
		if arrived.Add(1) == 2 {
			close(both)
		}
		select {
		case <-both:
		case <-time.After(2 * time.Second):
			timedOut.Store(true)
		}
		return "/usr/bin/" + cmd, nil
	}
	var starts atomic.Int32
	s.Actions.Run = func([]string) error {
		if starts.Add(1) > 1 {
			return errors.New("no tmux") // a failed second start must not take the folder
		}
		return nil
	}
	results := make(chan proto.Result, 2)
	for _, name := range []string{"one", "two"} {
		go func() {
			results <- s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: name, Account: "work", NewAccount: true}))
		}()
	}
	var ok, refused int
	for range 2 {
		switch res := <-results; {
		case res.Err == "":
			ok++
		case strings.Contains(res.Err, "the claude account work is there already"):
			refused++
		default:
			t.Errorf("err %q", res.Err)
		}
	}
	// An act waits at its first look-up, so the second arrival is the
	// other act's: a barrier that timed out proves nothing.
	if timedOut.Load() || arrived.Load() < 2 {
		t.Fatalf("the acts were not in flight together: %d look-ups, timed out %v", arrived.Load(), timedOut.Load())
	}
	if ok != 1 || refused != 1 || starts.Load() != 1 {
		t.Fatalf("%d started, %d refused, %d starts", ok, refused, starts.Load())
	}
	if _, err := os.Lstat(filepath.Join(accounts.Folder(home, "claude", "work"), accounts.SetupFile)); err != nil {
		t.Fatalf("the first's folder went: %v", err)
	}
}

// TestFailedRelinkStillStarts: when the entries of an existing like-default
// account cannot be linked again, it starts all the same, and the result
// names the first error.
func TestFailedRelinkStillStarts(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a folder of mode 0")
	}
	s, ran, home := accountTest(t)
	writeDefault(t, home, ".claude/settings.json")
	res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "one", Account: "work", NewAccount: true}))
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	def := filepath.Join(home, ".claude")
	if err := os.Chmod(def, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(def, 0o700) })
	*ran = nil
	res = s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "two", Account: "work"}))
	if res.Err != "" || len(*ran) != 1 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	if len(res.Skipped) != 1 || !strings.HasPrefix(res.Skipped[0], "some shared entries did not link: ") || !strings.Contains(res.Skipped[0], "permission denied") {
		t.Fatalf("skipped %q", res.Skipped)
	}
}

func TestStokerStartsOnDefaultOnly(t *testing.T) {
	for _, n := range []proto.NewSession{
		{Agent: "claude", Name: "n88", Account: "work"},
		{Agent: "claude", Name: "n88", Account: "fresh", NewAccount: true},
		{Agent: "claude", Name: "n88", Account: "default", NewAccount: true},
	} {
		s, ran := handsLife(t)
		n.Where = t.TempDir()
		b := newAct(t, n)
		if res := s.act(t.Context(), fromStokerAct("new:session", b.Text)); res.Err != "the stoker starts on default only" || len(*ran) != 0 {
			t.Fatalf("%+v: err %q ran %q", n, res.Err, *ran)
		}
	}
}

// switchAccountTest is accountTest with switchTest's holds and /work/app.
func switchAccountTest(t *testing.T) (*Service, *[][]string, string) {
	t.Helper()
	s, ran, home := accountTest(t)
	s.IsDir = func(path string) bool {
		fi, err := os.Stat(path)
		return path == "/work/app" || err == nil && fi.IsDir()
	}
	hs, err := holds.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.Holds = hs
	return s, ran, home
}

func TestSwitchUsesTheTargetsLastAccount(t *testing.T) {
	s, ran, home := switchAccountTest(t)
	folder := prepared(t, home, "codex", "team")
	if res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "codex", Name: "a1", Account: "team"})); res.Err != "" {
		t.Fatal(res.Err)
	}
	s.publish(lifeSnap(false), func(state.Doc) {})
	*ran = nil
	res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex", Handoff: proto.DefaultHandoff}))
	if res.Err != "" || len(*ran) != 1 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	got := strings.Join((*ran)[0], " ")
	want := " -u " + panes.HomeEnv + envOn("codex") + " CODEX_HOME=" + folder + ` codex -c cli_auth_credentials_store="file" ` + proto.DefaultHandoff
	if !strings.HasSuffix(got, want) {
		t.Fatalf("ran %q, want it to end %q", got, want)
	}
	if slices.Contains((*ran)[0], "new-session") || strings.Contains(got, " -e ") {
		t.Fatalf("the env is not on the command: %q", got)
	}
	if got := lastOf(s, "codex"); got != "team" {
		t.Fatalf("the switch changed the remembered account: %q", got)
	}
}

func TestSwitchOnAGoneAccountUsesDefault(t *testing.T) {
	s, ran, home := switchAccountTest(t)
	folder := prepared(t, home, "codex", "team")
	if res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "codex", Name: "a1", Account: "team"})); res.Err != "" {
		t.Fatal(res.Err)
	}
	if err := os.RemoveAll(folder); err != nil {
		t.Fatal(err)
	}
	s.publish(lifeSnap(false), func(state.Doc) {})
	*ran = nil
	res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex"}))
	if res.Err != "" || len(*ran) != 1 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	if got := strings.Join((*ran)[0], " "); strings.Contains(got, "CODEX_HOME") || strings.Contains(got, "cli_auth_credentials_store") || !strings.HasSuffix(got, " -u "+panes.HomeEnv+" codex") {
		t.Fatalf("ran %q, want the default", got)
	}
}

func TestSwitchNeverMakesAnAccount(t *testing.T) {
	s, ran, home := switchAccountTest(t)
	// The file names an account that is not there, and a hand-made folder
	// no live session runs on.
	for _, label := range []string{"ghost", "~/.codex-old"} {
		s.lastAccounts.set(lastAccountsPath(s.StatePath), "codex", label)
		*ran = nil
		res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex"}))
		if res.Err != "" || len(*ran) != 1 {
			t.Fatalf("%s: err %q ran %q", label, res.Err, *ran)
		}
		if got := strings.Join((*ran)[0], " "); strings.Contains(got, "CODEX_HOME") {
			t.Fatalf("%s: ran %q, want the default", label, got)
		}
		if _, err := os.Lstat(accounts.Root(home)); !os.IsNotExist(err) {
			t.Fatalf("%s: the switch made the accounts root: %v", label, err)
		}
	}
}

// TestSwitchOnACursorAccountOffMacOSUsesDefault: Cursor accounts run on
// macOS only, so a switch elsewhere starts Cursor on the default.
func TestSwitchOnACursorAccountOffMacOSUsesDefault(t *testing.T) {
	s, ran, home := switchAccountTest(t)
	prepared(t, home, sample.CursorAgent, "team")
	s.lastAccounts.set(lastAccountsPath(s.StatePath), sample.CursorAgent, "team")
	res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "cursor-agent"}))
	if res.Err != "" || len(*ran) != 1 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	if got := strings.Join((*ran)[0], " "); strings.Contains(got, "CURSOR_CONFIG_DIR") {
		t.Fatalf("ran %q, want the default", got)
	}
}

// TestSwitchWithAFailedRelinkStillSwitches: a like-default account whose
// entries do not link again switches all the same, and says so.
func TestSwitchWithAFailedRelinkStillSwitches(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a folder of mode 0")
	}
	s, ran, home := switchAccountTest(t)
	writeDefault(t, home, ".codex/config.toml")
	if res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "codex", Name: "one", Account: "team", NewAccount: true})); res.Err != "" {
		t.Fatal(res.Err)
	}
	def := filepath.Join(home, ".codex")
	if err := os.Chmod(def, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(def, 0o700) })
	// The new account is the last one once its session is signed in.
	snap := lifeSnap(false)
	snap.Sessions[2].AccountDir, snap.Sessions[2].Account = accounts.Folder(home, "codex", "team"), "you@example.com"
	s.publish(snap, func(state.Doc) {})
	*ran = nil
	res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex"}))
	if res.Err != "" || len(*ran) != 1 || !strings.Contains(strings.Join((*ran)[0], " "), "CODEX_HOME=") {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	if note, ok := proto.NotLinkedNote(res); !ok || !strings.Contains(note, "permission denied") {
		t.Fatalf("skipped %q", res.Skipped)
	}
}

// signInEnv is what an account start unsets, per agent: the sign-in a tmux
// server can pass on.
var signInEnv = map[string][]string{
	"claude":       {"CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"},
	"codex":        {"CODEX_API_KEY", "CODEX_ACCESS_TOKEN", "OPENAI_API_KEY"},
	"cursor-agent": {"CURSOR_API_KEY", "CURSOR_AUTH_TOKEN"},
}

// envOn is the start of the env command of an account start, joined by
// spaces: env and the unsets of agent's sign-in.
func envOn(agent string) string {
	out := " env"
	for _, n := range signInEnv[agent] {
		out += " -u " + n
	}
	return out
}

// unsetsBefore says whether argv has `env -u <each name> ...` before want.
func unsetsBefore(argv []string, names []string, want string) bool {
	at := slices.Index(argv, want)
	if at < 0 {
		return false
	}
	for _, n := range names {
		i := slices.Index(argv, n)
		if i < 1 || i > at || argv[i-1] != "-u" {
			return false
		}
	}
	return true
}

// TestAccountStartDropsTheServersSignIn: a start on an account unsets the
// sign-in env of its agent, so the account's own sign-in is used. A start
// on default keeps it.
func TestAccountStartDropsTheServersSignIn(t *testing.T) {
	for agent, names := range signInEnv {
		s, ran, home := accountTest(t)
		s.GOOS = "darwin" // Cursor accounts run on macOS only
		folder := prepared(t, home, agent, "work")
		res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: agent, Name: "a1", Account: "work"}))
		if res.Err != "" || len(*ran) != 1 {
			t.Fatalf("%s: err %q ran %q", agent, res.Err, *ran)
		}
		first := accounts.Env(agent, folder, false)[0]
		if !unsetsBefore((*ran)[0], names, first) {
			t.Errorf("%s: ran %q, want %v unset before %s", agent, (*ran)[0], names, first)
		}
		*ran = nil
		if res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: agent, Name: "a2", Account: "default"})); res.Err != "" || len(*ran) != 1 {
			t.Fatalf("%s default: err %q ran %q", agent, res.Err, *ran)
		}
		for _, n := range names {
			if slices.Contains((*ran)[0], n) {
				t.Errorf("%s: a default start unsets %s: %q", agent, n, (*ran)[0])
			}
		}
	}
}

// TestAccountSwitchDropsTheServersSignIn: a switch onto an account unsets
// the sign-in env of the new agent.
func TestAccountSwitchDropsTheServersSignIn(t *testing.T) {
	s, ran, home := switchAccountTest(t)
	folder := prepared(t, home, "codex", "team")
	if res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "codex", Name: "a1", Account: "team"})); res.Err != "" {
		t.Fatal(res.Err)
	}
	s.publish(lifeSnap(false), func(state.Doc) {})
	*ran = nil
	res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex"}))
	if res.Err != "" || len(*ran) != 1 {
		t.Fatalf("err %q ran %q", res.Err, *ran)
	}
	if !unsetsBefore((*ran)[0], signInEnv["codex"], "CODEX_HOME="+folder) {
		t.Errorf("ran %q, want %v unset", (*ran)[0], signInEnv["codex"])
	}
}
