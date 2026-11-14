package service

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/actions"
	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

func keysTest(t *testing.T) (*Service, *[][]string) {
	t.Helper()
	s, ran := holdsTest(t, sample.Session{PID: 42, Pane: "%1", Name: "flint", SessionID: "s1", Status: "idle"})
	return s, ran
}

func key(text string) proto.Act {
	return proto.Act{Console: true, RecID: proto.KeysPrefix + "%1", Text: text, PID: 42}
}

func TestKeysAreRefusedOnTheRealService(t *testing.T) {
	s, ran := keysTest(t)
	s.KeysToWorld = false
	res := s.act(t.Context(), key("Enter"))
	if res.Err != keysOnlyInDemo || len(*ran) != 0 {
		t.Errorf("err %q ran %q", res.Err, *ran)
	}
}

func TestKeysSendsANamedKey(t *testing.T) {
	s, _ := keysTest(t)
	res := s.act(t.Context(), key("Down"))
	if want := [][]string{panes.Keys("%1", "Down")}; res.Err != "" || !reflect.DeepEqual(res.Ran, want) {
		t.Fatalf("res %+v, want ran %q", res, want)
	}
}

func TestKeysSendsLiteralText(t *testing.T) {
	s, _ := keysTest(t)
	res := s.act(t.Context(), key("lit:yes please"))
	if want := [][]string{panes.Keys("%1", "-l", "--", "yes please")}; res.Err != "" || !reflect.DeepEqual(res.Ran, want) {
		t.Fatalf("res %+v, want ran %q", res, want)
	}
}

func TestKeysTwiceIsTwice(t *testing.T) {
	s, ran := keysTest(t)
	for range 2 {
		if res := s.act(t.Context(), key("Down")); res.Err != "" || len(res.Skipped) != 0 {
			t.Fatalf("res %+v", res)
		}
	}
	if len(*ran) != 2 {
		t.Fatalf("ran %q", *ran)
	}
}

func TestKeysRefusesWhatIsNotAKey(t *testing.T) {
	for text, name := range map[string]string{"C-z": "C-z", "lit:a\x1b[A": "control", "lit:": "empty", "lit:" + string(make([]byte, 201)): "long"} {
		s, ran := keysTest(t)
		if res := s.act(t.Context(), key(text)); res.Err == "" || len(*ran) != 0 {
			t.Fatalf("%s: res %+v ran %q", name, res, *ran)
		}
	}
	s, _ := keysTest(t)
	if res := s.act(t.Context(), key("C-z")); res.Err != `not sent: "C-z" is not a key` {
		t.Fatalf("err %q", res.Err)
	}
}

func TestKeysRefusedForANewAgentInThePane(t *testing.T) {
	for _, pid := range []int{41, 0} {
		s, ran := keysTest(t)
		a := key("Down")
		a.PID = pid
		if res := s.act(t.Context(), a); res.Err != "not sent: %1 runs a different agent now" || len(*ran) != 0 {
			t.Fatalf("pid %d: res %+v ran %q", pid, res, *ran)
		}
	}
}

func TestEnterRefusedForAQuestionNotSeen(t *testing.T) {
	s, ran := keysTest(t)
	since := time.Now()
	s.cur.Queue = []queue.Item{{Pane: "%1", State: queue.StateQuestion, Since: since}}
	a := key("Enter")
	a.Seen = since.Add(-time.Second)
	if res := s.act(t.Context(), a); res.Err != "not sent: flint asks something new — look again" || len(*ran) != 0 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
	a.Seen = since.Add(time.Second)
	if res := s.act(t.Context(), a); res.Err != "" || len(*ran) != 1 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
}

// TestTheServiceRefusesAnAnswerToAQuestionNewerThanTheSample: the board
// sends a digit with the time of the sample its card drew as Seen, and
// relies on this guard for a question that began after that sample.
func TestTheServiceRefusesAnAnswerToAQuestionNewerThanTheSample(t *testing.T) {
	sampled := time.Now()
	refused := "not sent: flint asks something new — look again"
	for _, c := range []struct {
		name  string
		since time.Time
		err   string
		ran   int
	}{
		{"a question newer than the sample", sampled.Add(time.Second), refused, 0},
		{"a question that began just before the sample", sampled.Add(-waitSettle / 2), refused, 0},
		{"a question the sample showed", sampled.Add(-time.Second), "", 1},
	} {
		s, ran := keysTest(t)
		s.cur.Queue = []queue.Item{{Pane: "%1", State: queue.StateQuestion, Since: c.since}}
		a := key(proto.LitPrefix + "1")
		a.Seen = sampled
		res := s.act(t.Context(), a)
		if res.Err != c.err || len(*ran) != c.ran {
			t.Errorf("%s: res %+v ran %q", c.name, res, *ran)
		}
	}
}

func TestEnterAtAPermissionPromptRuns(t *testing.T) {
	s, ran := keysTest(t)
	since := time.Now()
	s.cur.Queue = []queue.Item{{Pane: "%1", State: queue.StatePermission, Since: since}}
	a := key("Enter")
	a.Seen = since.Add(time.Second)
	if res := s.act(t.Context(), a); res.Err != "" || len(*ran) != 1 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
}

func TestKeysOnlyFromAConsole(t *testing.T) {
	s, ran := keysTest(t)
	s.StokerServer = "mbtest"
	a := key("Down")
	a.Console = false
	if res := s.act(t.Context(), a); res.Err != keysOnlyInDemo {
		t.Fatalf("err %q", res.Err)
	}
	a = key("Down")
	a.From = "%7@mbtest"
	if res := s.act(t.Context(), a); res.Err != keysOnlyInDemo {
		t.Fatalf("stoker: err %q", res.Err)
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

func TestKeysWaitForThePaneAndThenCheckTheAgent(t *testing.T) {
	s, ran := keysTest(t)
	release := s.holdPane("%1")
	done := make(chan proto.Result, 1)
	go func() { done <- s.act(t.Context(), key("Down")) }()
	select {
	case res := <-done:
		t.Fatalf("a key went past the pane's lock: %+v", res)
	case <-time.After(50 * time.Millisecond):
	}
	s.publish(sample.Snapshot{At: time.Now(), Sessions: []sample.Session{{PID: 43, Pane: "%1", Name: "new", SessionID: "s2", Status: "idle"}}}, func(state.Doc) {})
	release()
	if res := <-done; res.Err != "not sent: %1 runs a different agent now" || len(*ran) != 0 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
}

// The key's step carries the agent's pid and start, and the runner checks
// them on the machine: a pid that another process took after the snapshot
// gets no key.
func TestAKeyCarriesTheAgentsGuard(t *testing.T) {
	self, ok := actions.NewHost().Proc(os.Getpid())
	if !ok {
		t.Skip("this machine does not tell the test's own start")
	}
	agent := sample.Session{PID: os.Getpid(), Start: self.StartTime, Pane: "%1", Name: "flint", SessionID: "s1", Status: "idle"}
	s, ran := holdsTest(t, agent)
	var guards []advice.Guard
	s.Actions.Check = func(g advice.Guard) error {
		guards = append(guards, g)
		return actions.CheckGuard(g)
	}
	a := key("Down")
	a.PID, a.Start = agent.PID, agent.Start
	if res := s.act(t.Context(), a); res.Err != "" || len(*ran) != 1 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
	agent.Start++ // the snapshot still names the pid, but the process there started at another time
	s.publish(sample.Snapshot{At: time.Now(), Sessions: []sample.Session{agent}}, func(state.Doc) {})
	a.Start = agent.Start // as the console saw it: the runner still checks the machine
	if res := s.act(t.Context(), a); len(res.Skipped) != 1 || len(*ran) != 1 {
		t.Fatalf("a new process in the pid: res %+v ran %q", res, *ran)
	}
	if len(guards) != 2 || guards[0] != (advice.Guard{PID: agent.PID, StartTicks: self.StartTime}) || guards[1].StartTicks != agent.Start {
		t.Fatalf("guards %+v", guards)
	}
}

func TestKeysKeepTheStokerResting(t *testing.T) {
	s, _, _ := stokerTest(t)
	s.Now = func() time.Time { return night0.Add(time.Minute) }
	s.publish(sample.Snapshot{At: night0, Sessions: []sample.Session{{PID: 42, Pane: "%1", Name: "work", SessionID: "s1", Status: "idle"}}}, func(state.Doc) {})
	if r := s.act(t.Context(), proto.Act{Console: true, RecID: "stoker:rest"}); r.Err != "" {
		t.Fatal(r.Err)
	}
	s.Now = func() time.Time { return night0.Add(2 * time.Minute) }
	if res := s.act(t.Context(), key("Down")); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := s.Stoker.Mode().Touched; !got.Equal(night0.Add(2 * time.Minute)) {
		t.Fatalf("touched %v", got)
	}
}

func TestAKeyFromTheConsoleFreesADrivenPane(t *testing.T) {
	s, _, _ := guardedRun(t)
	s.cur.Sessions[0].PID = 42
	if !denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("the pane is not driven")
	}
	if res := s.act(t.Context(), key("Down")); res.Err != "" {
		t.Fatal(res.Err)
	}
	if denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("the person's key at a console did not free the pane")
	}
}

// Scrolling only looks: it leaves a driven pane's merge guard on. A person who
// reads the agent's history has not taken the pane from the stoker.
func TestAScrollLeavesTheMergeGuardOn(t *testing.T) {
	for _, text := range []string{proto.WheelUp, proto.WheelDown, "PPage", "NPage"} {
		s, _, _ := guardedRun(t)
		s.cur.Sessions[0].PID = 42
		s.PaneMouse = func(string) (int, int, bool) { return 80, 24, true }
		if res := s.act(t.Context(), key(text)); res.Err != "" {
			t.Fatalf("%s: %s", text, res.Err)
		}
		if !denies(t, s, "%1", "gh pr merge 4556") {
			t.Fatalf("%s freed the driven pane", text)
		}
	}
}

func TestAKeyKicksTheWatchingConsole(t *testing.T) {
	s, _ := keysTest(t)
	cl := console(s)
	cl.screenKick = make(chan struct{}, 1)
	pane := "%1"
	cl.watch.Store(&watched{pane: pane})
	if res := s.act(t.Context(), key("Down")); res.Err != "" {
		t.Fatal(res.Err)
	}
	select {
	case <-cl.screenKick:
	default:
		t.Fatal("the watching console was not kicked")
	}
}

// The agent draws after send-keys returns, so the read a key kicks at once
// misses the key, and the tick shows it up to watchEvery late (#286).
func TestAKeyReadsTheScreenAgainBeforeTheTick(t *testing.T) {
	s, _ := keysTest(t)
	cl := console(s)
	cl.screenKick = make(chan struct{}, 1)
	cl.watch.Store(&watched{pane: "%1"})
	if res := s.act(t.Context(), key("Down")); res.Err != "" {
		t.Fatal(res.Err)
	}
	<-cl.screenKick // the read at once
	select {
	case <-cl.screenKick:
	case <-time.After(watchEvery):
		t.Fatalf("no read again within %v of the key", watchEvery)
	}
}

func TestARefusedKeyReadsNoScreenAgain(t *testing.T) {
	s, _ := keysTest(t)
	cl := console(s)
	cl.screenKick = make(chan struct{}, 1)
	cl.watch.Store(&watched{pane: "%1"})
	if res := s.act(t.Context(), key("not-a-key")); res.Err == "" {
		t.Fatal("the key was not refused")
	}
	select {
	case <-cl.screenKick:
		t.Fatal("a refused key kicked a read")
	case <-time.After(watchEvery):
	}
}

// The chat is a window on the pane: the wheel goes to the agent, in the middle
// of its window, and the agent scrolls itself (#288).
func TestTheWheelGoesToTheAgent(t *testing.T) {
	s, ran := keysTest(t)
	s.PaneMouse = func(string) (int, int, bool) { return 80, 24, true }
	if res := s.act(t.Context(), key(proto.WheelUp)); res.Err != "" {
		t.Fatal(res.Err)
	}
	if res := s.act(t.Context(), key(proto.WheelDown)); res.Err != "" {
		t.Fatal(res.Err)
	}
	want := [][]string{panes.Wheel("%1", true, 40, 12), panes.Wheel("%1", false, 40, 12)}
	if !reflect.DeepEqual(*ran, want) {
		t.Fatalf("ran %v, want %v", *ran, want)
	}
}

func TestTheWheelSaysWhatToDoWhenTheAgentTakesNoMouse(t *testing.T) {
	s, ran := keysTest(t)
	s.PaneMouse = func(string) (int, int, bool) { return 0, 0, false }
	res := s.act(t.Context(), key(proto.WheelUp))
	if res.Err != "not scrolled: flint does not read the mouse — open its pane to scroll it" || len(*ran) != 0 {
		t.Fatalf("result %+v, ran %v", res, *ran)
	}
}

func TestPageKeysGoToTheAgent(t *testing.T) {
	s, ran := keysTest(t)
	if res := s.act(t.Context(), key("PPage")); res.Err != "" {
		t.Fatal(res.Err)
	}
	if want := [][]string{panes.Keys("%1", "PPage")}; !reflect.DeepEqual(*ran, want) {
		t.Fatalf("ran %v, want %v", *ran, want)
	}
}

// The wheel answers no menu, so it needs no screen the console has shown.
func TestTheWheelAtAMenuNeedsNoSeenScreen(t *testing.T) {
	s, ran := holdsTest(t, sample.Session{PID: 42, Pane: "%1", Name: "flint", SessionID: "s1", Status: "idle", Permits: true})
	s.PaneMouse = func(string) (int, int, bool) { return 80, 24, true }
	if res := s.act(t.Context(), key(proto.WheelDown)); res.Err != "" || len(*ran) != 1 {
		t.Fatalf("result %+v, ran %v", res, *ran)
	}
}

func TestWatchIsRefusedFromTheStokersServer(t *testing.T) {
	s, _ := keysTest(t)
	s.StokerServer = "mbtest"
	cl := console(s)
	res := s.watchAct(cl, proto.ClientConsole, proto.Act{From: "%7@mbtest"}, "%1")
	if res.Err != watchOnlyFromConsole {
		t.Fatalf("err %q", res.Err)
	}
}

// TestACompactCarriesTheAgentsGuard: every keystroke of a console's compact
// names the agent's pid and start, so a pid another process took after the
// snapshot gets no key.
func TestACompactCarriesTheAgentsGuard(t *testing.T) {
	for _, a := range []proto.Act{consoleCompact(42)} {
		a.Start = 7
		s, ran := commandTest(t, "claude", idleScreen)
		s.cur.Sessions[0].Start = 7
		var guards []advice.Guard
		s.Actions.Check = func(g advice.Guard) error {
			guards = append(guards, g)
			return nil
		}
		if res := s.act(t.Context(), a); res.Err != "" || len(*ran) != 2 {
			t.Fatalf("%s: res %+v ran %q", a.RecID, res, *ran)
		}
		if len(guards) != 2 {
			t.Fatalf("%s: guards %+v", a.RecID, guards)
		}
		for _, g := range guards {
			if g.PID != 42 || g.StartTicks != 7 {
				t.Fatalf("%s: guards %+v", a.RecID, guards)
			}
		}
	}
}

// TestATypedSemicolonGoes: a "lit:" key that ends in ";" still types it.
func TestATypedSemicolonGoes(t *testing.T) {
	for lit, want := range map[string]string{";": `\;`, "ls;": `ls\;`} {
		step, why := keyStep("%1", "lit:"+lit)
		if why != "" || step[len(step)-1] != want {
			t.Errorf("lit:%s: %q %q", lit, step, why)
		}
	}
}

// A pid that a new agent took in the same pane is told apart by its start:
// the act names the start the console saw.
func TestKeysRefusedForANewProcessOnTheSamePID(t *testing.T) {
	s, ran := holdsTest(t, sample.Session{PID: 7, Start: 100, Pane: "%1", Name: "flint", SessionID: "s1", Status: "idle"})
	a := key("Down")
	a.PID, a.Start = 7, 99
	if res := s.act(t.Context(), a); res.Err != "not sent: %1 runs a different agent now" || len(*ran) != 0 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
	a.Start = 100
	if res := s.act(t.Context(), a); res.Err != "" || len(*ran) != 1 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
}

// The phone's Ctrl key reaches the agent, and no ctrl key counts as an
// answer to a menu (#300).
func TestKeysSendsTheCtrlKeysOfTheBelt(t *testing.T) {
	for _, l := range proto.CtrlKeys {
		s, ran := keysTest(t)
		if res := s.act(t.Context(), key("C-"+string(l))); res.Err != "" || len(*ran) != 1 {
			t.Fatalf("C-%c: res %+v ran %q", l, res, *ran)
		}
		if answers("C-" + string(l)) {
			t.Fatalf("C-%c answers a menu", l)
		}
	}
}
