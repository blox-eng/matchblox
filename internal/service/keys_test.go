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
	for text, name := range map[string]string{"C-x": "C-x", "lit:a\x1b[A": "control", "lit:": "empty", "lit:" + string(make([]byte, 201)): "long"} {
		s, ran := keysTest(t)
		if res := s.act(t.Context(), key(text)); res.Err == "" || len(*ran) != 0 {
			t.Fatalf("%s: res %+v ran %q", name, res, *ran)
		}
	}
	s, _ := keysTest(t)
	if res := s.act(t.Context(), key("C-x")); res.Err != `not sent: "C-x" is not a key` {
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
	if res := s.act(t.Context(), a); res.Err != "keys go only from a console" {
		t.Fatalf("err %q", res.Err)
	}
	a = key("Down")
	a.From = "%7@mbtest"
	if res := s.act(t.Context(), a); res.Err != "keys go only from a console" {
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
	a.PID = agent.PID
	if res := s.act(t.Context(), a); res.Err != "" || len(*ran) != 1 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
	agent.Start++ // the snapshot still names the pid, but the process there started at another time
	s.publish(sample.Snapshot{At: time.Now(), Sessions: []sample.Session{agent}}, func(state.Doc) {})
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

func TestWatchIsRefusedFromTheStokersServer(t *testing.T) {
	s, _ := keysTest(t)
	s.StokerServer = "mbtest"
	cl := console(s)
	res := s.watchAct(cl, proto.ClientConsole, proto.Act{From: "%7@mbtest"}, "%1")
	if res.Err != "keys go only from a console" {
		t.Fatalf("err %q", res.Err)
	}
}
