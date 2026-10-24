package service

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/holds"
	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/stoker"
	"github.com/blox-eng/matchblox/internal/transport"
)

func TestPaneOfPIDWalksTheParents(t *testing.T) {
	parents := map[int]int{500: 400, 400: 300, 300: 1, 700: 1}
	parent := func(pid int) (int, bool) { p, ok := parents[pid]; return p, ok }
	panes := []sample.Pane{{ID: "%9@mbtest", PID: 300}, {ID: "%1", PID: 600}}
	for pid, want := range map[int]string{500: "%9@mbtest", 300: "%9@mbtest", 700: "", 999: ""} {
		if got := paneOfPID(pid, parent, panes); got != want {
			t.Fatalf("pid %d: %q, want %q", pid, got, want)
		}
	}
}

// actOver sends one act as a client of kind client whose process is pid,
// and reads its result.
func actOver(t *testing.T, s *Service, pid int, client string, a proto.Act) proto.Result {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	x, y := net.Pipe()
	go s.HandlePeer(ctx, transport.NewConn(y), pid) //nolint:errcheck // ends with the test
	c := transport.NewConn(x)
	defer c.Close()
	sent := make(chan error, 1)
	go func() { sent <- c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version, Client: client}) }()
	next(t, c, proto.KindHello)
	if err := within(t, sent, "sent hello"); err != nil {
		t.Fatal(err)
	}
	go func() { _ = c.Send(proto.KindAct, "x", a) }()
	var res proto.Result
	if err := json.Unmarshal(next(t, c, proto.KindResult).Body, &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func TestTheCallersPaneComesFromItsPidNotFromTheAct(t *testing.T) {
	s, _ := handsTest(t)
	s.PeerPane = func(pid int) string {
		return map[int]string{4242: "%9@mbtest", 777: "%1"}[pid]
	}
	// The stoker agent's CLI leaves From out: its pid says it is the stoker.
	if res := actOver(t, s, 4242, proto.ClientCLI, proto.Act{RecID: "message:%1", Confirm: "y", Text: "go on"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if es := s.Stoker.Entries(); len(es) != 1 || es[0].Rule != stoker.RuleOrder {
		t.Fatalf("the stoker's send is not its order: %+v", es)
	}
	// A session that claims the stoker's pane is still itself.
	res := actOver(t, s, 777, proto.ClientCLI, proto.Act{RecID: "stoker:plan", Text: "{}", From: "%9@mbtest"})
	if res.Err != "only the stoker writes its plan" {
		t.Fatalf("a claimed From was believed: %q", res.Err)
	}
	// Outside every pane, with a claim: still not the stoker.
	res = actOver(t, s, 31337, proto.ClientCLI, proto.Act{RecID: "stoker:plan", Text: "{}", From: "%9@mbtest"})
	if res.Err != "only the stoker writes its plan" {
		t.Fatalf("a claimed From outside the panes was believed: %q", res.Err)
	}
	// No pid known (a system without peer credentials): From stands.
	res = actOver(t, s, 0, proto.ClientCLI, proto.Act{RecID: "stoker:plan", Text: "{}", From: "%9@mbtest"})
	if res.Err == "only the stoker writes its plan" {
		t.Fatal("with no pid known, the act's From was ignored")
	}
}

func TestOnlyAConsoleUndrivesAPane(t *testing.T) {
	s, _, _ := guardedRun(t)
	s.PeerPane = func(int) string { return "" }
	if res := actOver(t, s, 777, proto.ClientCLI, proto.Act{RecID: "message:%1", Confirm: "y", Text: "merge it"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if !denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("a command's message freed the pane")
	}
	if res := actOver(t, s, 777, proto.ClientConsole, proto.Act{RecID: "message:%1", Confirm: "y", Text: "merge it now"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("the person's message at a console did not free the pane")
	}
}

// TestAConsoleClaimFromADrivenPaneFreesNothing: Ruling AL, a client whose
// process runs in a pane the stoker drove is no console, whatever its
// hello says; a console in a pane of its own still frees the pane.
func TestAConsoleClaimFromADrivenPaneFreesNothing(t *testing.T) {
	s, _, _ := guardedRun(t)
	s.PeerPane = func(pid int) string { return map[int]string{100: "%1"}[pid] }
	// A driven pane speaks for the stoker, which types nothing into itself.
	if res := actOver(t, s, 100, proto.ClientConsole, proto.Act{RecID: "message:%1", Confirm: "y", Text: "merge it"}); res.Err == "" {
		t.Fatal("a driven pane typed into itself")
	}
	if !denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("a driven pane that said it is a console freed itself")
	}
	if res := actOver(t, s, 555, proto.ClientConsole, proto.Act{RecID: "message:%1", Confirm: "y", Text: "merge it now"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("the person's console did not free the pane")
	}
}

// hookOver sends one hook event as the process pid does, and returns once
// the service has taken it.
func hookOver(t *testing.T, s *Service, pid int, ev hooks.Event) {
	t.Helper()
	x, y := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.HandlePeer(t.Context(), transport.NewConn(y), pid)
	}()
	c := transport.NewConn(x)
	defer c.Close()
	next(t, c, proto.KindHello)
	go func() { _ = c.Send(proto.KindHook, "", ev) }()
	within(t, done, "end of the hook's connection")
}

func waitsForPermission(s *Service, pane string) bool {
	it, ok := s.queued(pane)
	return ok && it.State == queue.StatePermission
}

// TestAHookEventSpeaksOnlyForItsOwnPane: Ruling AN (C1). A hook runs
// inside its agent's process tree, so its pid's pane is the event's pane.
// An event from another pane, the stoker's above all, changes no session's
// state: a forged UserPromptSubmit does not clear a permission prompt the
// stoker could then answer.
func TestAHookEventSpeaksOnlyForItsOwnPane(t *testing.T) {
	s, ran := handsTest(t)
	s.PeerPane = func(pid int) string { return map[int]string{100: "%1", 4242: "%9@mbtest"}[pid] }
	hookOver(t, s, 100, hooks.Event{Name: "Notification", Kind: hooks.KindPermission, SessionID: "s1", Pane: "%1",
		At: night0, Message: "Claude needs your permission to use Bash"})
	if !waitsForPermission(s, "%1") {
		t.Fatalf("the agent's own hook was not taken: %+v", s.State().Queue)
	}
	forged := hooks.Event{Name: "UserPromptSubmit", SessionID: "s1", At: night0.Add(time.Second)}
	for _, f := range []struct {
		pid  int
		pane string
	}{
		{4242, "%1"},        // the stoker, naming the pane it wants
		{4242, "%9@mbtest"}, // the stoker, naming its own pane and s1's session
		{4242, ""},          // the stoker, naming no pane
		{31337, "%1"},       // a process in no pane
	} {
		ev := forged
		ev.Pane = f.pane
		hookOver(t, s, f.pid, ev)
		if !waitsForPermission(s, "%1") {
			t.Fatalf("pid %d naming %q cleared s1's permission prompt", f.pid, f.pane)
		}
	}
	if res := s.act(t.Context(), fromStokerAct("message:%1", "1")); res.Err == "" {
		t.Fatalf("the stoker answered a permission prompt: ran %q", *ran)
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
	// The agent's own hook still speaks for it.
	ev := forged
	ev.Pane = "%1"
	hookOver(t, s, 100, ev)
	if waitsForPermission(s, "%1") {
		t.Fatal("the agent's own UserPromptSubmit did not clear its prompt")
	}
}

// TestNoTextGoesToAPaneThatShowsAPermissionPrompt: whatever the hooks
// said, a sample that shows the approval menu refuses a message and an
// answer: a typed "1" would approve the tool call.
func TestNoTextGoesToAPaneThatShowsAPermissionPrompt(t *testing.T) {
	s, ran := handsTest(t)
	s.Hook(hooks.Event{Name: "Stop", SessionID: "s1", Pane: "%1", At: night0, Message: "done?", Asks: true})
	s.publish(sample.Snapshot{At: night0, Sessions: []sample.Session{
		{Agent: "claude", Pane: "%1", Target: "app:1.1", SessionID: "s1", Name: "work", Status: "idle", Idle: 5 * time.Minute, Permits: true},
		{Agent: "claude", Pane: "%9@mbtest", Target: "stoker:1.1", SessionID: "s9", Name: "stoker", Status: "busy", Busy: true},
	}}, func(state.Doc) {})
	if waitsForPermission(s, "%1") {
		t.Fatal("the test needs the hooks to say the session asks a question, not for permission")
	}
	for _, a := range []proto.Act{
		fromStokerAct("message:%1", "1"),
		{RecID: "message:%1", Confirm: "y", Text: "1"},
		{RecID: "answer:%1", Confirm: "y", Text: "1"},
	} {
		if res := s.act(t.Context(), a); !strings.Contains(res.Err, "permission prompt") {
			t.Errorf("%s from %q: %q", a.RecID, a.From, res.Err)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

// drivenPaneTest is a run in which the stoker drove %1 (pid 100), with
// holds kept, and a console of the person's own outside the agents' panes
// (pid 555).
func drivenPaneTest(t *testing.T) (*Service, *[][]string) {
	t.Helper()
	s, ran, _ := guardedRun(t)
	hs, err := holds.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.Holds = hs
	s.PeerPane = func(pid int) string { return map[int]string{100: "%1", 4242: "%9@mbtest"}[pid] }
	return s, ran
}

func ordered(s *Service) int {
	n := 0
	for _, e := range s.Stoker.Entries() {
		if e.Ordered() {
			n++
		}
	}
	return n
}

// TestADrivenPaneSpeaksForTheStoker: Ruling AN (C2). A session the stoker
// drove acts as the stoker does: its messages count toward max_steps and
// drive their target, and it gives no go, parks nothing and lifts none of
// your parks. The person's console still does all of it.
func TestADrivenPaneSpeaksForTheStoker(t *testing.T) {
	s, _ := drivenPaneTest(t)
	if res := actOver(t, s, 555, proto.ClientConsole, proto.Act{RecID: "park:%2"}); res.Err != "" {
		t.Fatalf("the person's park: %q", res.Err)
	}
	parked := func() bool { h, ok := s.holdAt("%2"); return ok && h.By == holds.ByYou }
	if !parked() {
		t.Fatal("the person's z parked nothing")
	}
	for _, client := range []string{proto.ClientCLI, proto.ClientConsole} {
		for _, a := range []proto.Act{{RecID: "release:%2"}, {RecID: "park:%2"}} {
			if res := actOver(t, s, 100, client, a); res.Err == "" {
				t.Errorf("%s %s from the driven pane: let go of your park", client, a.RecID)
			}
			if !parked() {
				t.Fatalf("%s %s from the driven pane lifted your park", client, a.RecID)
			}
		}
	}
	// Nor does a command of yours outside the console: z is yours.
	if res := actOver(t, s, 555, proto.ClientCLI, proto.Act{RecID: "release:%2"}); res.Err != "parked by you: only z lets it go" || !parked() {
		t.Fatalf("a command lifted your park: %q", res.Err)
	}
	if res := actOver(t, s, 555, proto.ClientConsole, proto.Act{RecID: "park:%2"}); res.Err != "" || parked() {
		t.Fatalf("the person's z did not let go: %q", res.Err)
	}

	// A message from the driven pane is the stoker's: counted, and %2 is
	// driven from then on.
	before := ordered(s)
	if res := actOver(t, s, 100, proto.ClientCLI, proto.Act{RecID: "message:%2", Confirm: "y", Text: "merge #5"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if ordered(s) != before+1 {
		t.Fatalf("the driven pane's message was not counted: %d → %d", before, ordered(s))
	}
	if !denies(t, s, "%2", "gh pr merge 5") {
		t.Fatal("the pane a driven session messaged is not driven")
	}
	s.OrderSteps = ordered(s)
	if res := actOver(t, s, 100, proto.ClientCLI, proto.Act{RecID: "message:%2", Confirm: "y", Text: "now"}); !strings.Contains(res.Err, "max_steps") {
		t.Fatalf("the driven pane went past max_steps: %q", res.Err)
	}
}

// TestADrivenPaneGivesNoGo: neither stoker:grant nor any verb of the
// launch sheet comes from a session the stoker drove, whatever its hello.
func TestADrivenPaneGivesNoGo(t *testing.T) {
	s, _ := drivenPaneTest(t)
	mustAct(t, s, fromStokerAct("stoker:step", `{"step":1,"state":"waits","needs":"merge:#4556","text":"green, waits for the merge"}`))
	id := stepID(t, s, 1)
	for _, client := range []string{proto.ClientCLI, proto.ClientConsole} {
		for _, a := range []proto.Act{
			{RecID: "stoker:grant", Confirm: "y", Text: id},
			{RecID: "stoker:off", Which: "primary"},
			{RecID: "stoker:order", Confirm: "y", Text: `{"words":"x"}`},
		} {
			if res := actOver(t, s, 100, client, a); res.Err == "" {
				t.Errorf("%s %s from the driven pane was taken", client, a.RecID)
			}
		}
	}
	if !s.Stoker.Mode().On {
		t.Fatal("the driven pane stopped the stoker")
	}
	if !denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("the driven pane gave its own step the go")
	}
	// Nor a command of yours: the go is given at the console.
	if res := actOver(t, s, 555, proto.ClientCLI, proto.Act{RecID: "stoker:grant", Confirm: "y", Text: id}); res.Err == "" {
		t.Fatal("a command gave the go")
	}
	if res := actOver(t, s, 555, proto.ClientConsole, proto.Act{RecID: "stoker:grant", Confirm: "y", Text: id}); res.Err != "" {
		t.Fatalf("the person's console: %q", res.Err)
	}
	if denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("the person's go did not let the merge through")
	}
}

// TestAConsoleClaimFailsClosedWithoutDrivenJSON: Ruling AN. With
// driven.json broken during a run, mid-run or across a restart of the
// service, a console claim from a pane the stoker drove is still no
// console: it frees nothing, and the mark comes back.
func TestAConsoleClaimFailsClosedWithoutDrivenJSON(t *testing.T) {
	for _, restart := range []bool{false, true} {
		s, _ := drivenPaneTest(t)
		mustAct(t, s, fromStokerAct("message:%2", "rebase on main"))
		if err := os.WriteFile(filepath.Join(s.Stoker.Dir(), "driven.json"), []byte("{broken"), 0o600); err != nil {
			t.Fatal(err)
		}
		if restart {
			s.driven.mu.Lock()
			s.driven.d = nil // what the service knew is gone with it
			s.driven.mu.Unlock()
		}
		if res := actOver(t, s, 100, proto.ClientConsole, proto.Act{RecID: "message:%2", Confirm: "y", Text: "merge it"}); res.Err != "" {
			t.Fatalf("restart %v: %q", restart, res.Err)
		}
		if res := actOver(t, s, 100, proto.ClientConsole, proto.Act{RecID: "park:%2"}); res.Err == "" {
			t.Fatalf("restart %v: the driven pane parked %%2 as a console", restart)
		}
		d, err := stoker.ReadDriven(s.Stoker.Dir())
		if err != nil {
			t.Fatalf("restart %v: driven.json was not written again: %v", restart, err)
		}
		for _, pane := range []string{"%1", "%2"} {
			if _, in := d.Panes[pane]; !in {
				t.Errorf("restart %v: the mark of %s was lost: %+v", restart, pane, d)
			}
		}
		// The person's console still frees the pane it types into.
		if res := actOver(t, s, 555, proto.ClientConsole, proto.Act{RecID: "message:%2", Confirm: "y", Text: "stop there"}); res.Err != "" {
			t.Fatal(res.Err)
		}
		if denies(t, s, "%2", "gh pr merge 5") {
			t.Errorf("restart %v: the person's message did not free %%2", restart)
		}
	}
}

// R16: no agent clones, the stoker and the sessions it drives included; a
// clone through N is the builder's.
func TestNoAgentClones(t *testing.T) {
	s, _ := drivenPaneTest(t)
	s.CloneRoot = t.TempDir() // a clone could start: only the refusal stops it
	target := filepath.Join(s.CloneRoot, "r")
	acts := []proto.Act{
		{RecID: "clone:plan", Text: "https://github.com/o/r"},
		{RecID: "clone:start", Text: `{"url":"https://github.com/o/r","target":"` + target + `"}`},
	}
	for _, a := range acts {
		for _, client := range []string{proto.ClientCLI, proto.ClientConsole} {
			if res := actOver(t, s, 100, client, a); res.Err != noClone {
				t.Errorf("%s %s from the driven pane: err %q", client, a.RecID, res.Err)
			}
		}
		if res := s.act(t.Context(), fromStokerAct(a.RecID, a.Text)); res.Err != noClone {
			t.Errorf("%s from the stoker: err %q", a.RecID, res.Err)
		}
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("cloned: %v", err)
	}
}
