package service

import (
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// opencodeScreen is OpenCode at rest: its prompt has no mark to read.
const opencodeScreen = "  Done. All 42 tests pass.\n┃\n┃  Ask anything... \"Fix broken tests\"\n╹▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀\n  ctrl+t variants  tab agents  ctrl+p commands\n"

// commandTest is the service with one session of agent, pid 42, at rest
// in %1, whose pane shows screen, then the text a step typed after its
// prompt mark.
func commandTest(t *testing.T, agent, screen string) (*Service, *[][]string) {
	t.Helper()
	s, _, _ := stokerTest(t)
	s.Actions.Check = nil
	s.publish(sample.Snapshot{At: night0, Sessions: []sample.Session{
		{Agent: agent, PID: 42, Pane: "%1", SessionID: "s1", Name: "work", Status: "idle"},
	}}, func(state.Doc) {})
	var mu sync.Mutex
	var ran [][]string
	s.Capture = func([]string) map[string]string {
		mu.Lock()
		defer mu.Unlock()
		return map[string]string{"%1": screen}
	}
	s.Actions.Run = func(argv []string) error {
		mu.Lock()
		defer mu.Unlock()
		ran = append(ran, argv)
		if i := slices.Index(argv, "-l"); i >= 0 {
			screen = strings.Replace(screen, "> \n", "> "+argv[len(argv)-1]+"\n", 1)
		}
		return nil
	}
	return s, &ran
}

func command(id string) proto.Act {
	return proto.Act{Console: true, RecID: proto.CommandPrefix + "%1", Text: id, PID: 42, Confirm: "y"}
}

func TestCommandTypesItsTextThenEnterAtRest(t *testing.T) {
	for agent, id := range map[string]string{"claude": "review", "codex": "codex-review"} {
		s, ran := commandTest(t, agent, idleScreen)
		res := s.act(t.Context(), command(id))
		if want := panes.Send("%1", "/review"); res.Err != "" || !reflect.DeepEqual(*ran, want) {
			t.Fatalf("%s: res %+v ran %q, want %q", agent, res, *ran, want)
		}
	}
}

func TestCommandRefusesADraft(t *testing.T) {
	s, ran := commandTest(t, "claude", strings.Replace(idleScreen, "> \n", "> half a prompt\n", 1))
	res := s.act(t.Context(), command("review"))
	if res.Err != "/review not sent: the pane %1 has text you typed" || len(*ran) != 0 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
}

func TestCommandRefusesAMenuOrAQuestion(t *testing.T) {
	menu := " Bash command\n   rm -rf build\n Do you want to proceed?\n ❯ 1. Yes\n   2. No, and tell Claude what to do differently (esc)\n"
	s, ran := commandTest(t, "claude", menu)
	if res := s.act(t.Context(), command("model")); res.Err != "/model not sent: the pane %1 shows a permission prompt" || len(*ran) != 0 {
		t.Fatalf("menu: res %+v ran %q", res, *ran)
	}
	s, ran = commandTest(t, "codex", idleScreen)
	s.cur.Queue = []queue.Item{{Pane: "%1", State: queue.StateQuestion, Since: night0}}
	if res := s.act(t.Context(), command("codex-model")); res.Err != "/model not sent: work waits for you" || len(*ran) != 0 {
		t.Fatalf("question: res %+v ran %q", res, *ran)
	}
}

func TestCommandWaitsForTheEndOfTheTurn(t *testing.T) {
	s, ran := commandTest(t, "claude", idleScreen)
	s.cur.Sessions[0].Status, s.cur.Sessions[0].Busy = "busy", true
	if res := s.act(t.Context(), command("review")); res.Err != "/review not sent: work is not at rest: a /review waits for the end of its turn" || len(*ran) != 0 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
}

func TestCommandRefusedForANewAgentInThePane(t *testing.T) {
	for _, pid := range []int{41, 0} {
		s, ran := commandTest(t, "claude", idleScreen)
		a := command("review")
		a.PID = pid
		if res := s.act(t.Context(), a); res.Err != "/review not sent: %1 runs a different agent now" || len(*ran) != 0 {
			t.Fatalf("pid %d: res %+v ran %q", pid, res, *ran)
		}
	}
}

func TestCommandOfAnotherAgentIsRefused(t *testing.T) {
	s, ran := commandTest(t, "codex", idleScreen)
	if res := s.act(t.Context(), command("review")); res.Err != "/review is Claude Code's: work runs Codex" || len(*ran) != 0 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
}

// TestCommandWhoseAgentsPromptIsNotReadTypesAtRest: OpenCode's prompt has
// no mark, so its draft is not read. The rest, question and permission
// guards still hold.
func TestCommandWhoseAgentsPromptIsNotReadTypesAtRest(t *testing.T) {
	s, ran := commandTest(t, "opencode", opencodeScreen)
	if res := s.act(t.Context(), command("opencode-compact")); res.Err != "" || !reflect.DeepEqual(*ran, panes.Send("%1", "/compact")) {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
	s, ran = commandTest(t, "opencode", opencodeScreen)
	s.cur.Queue = []queue.Item{{Pane: "%1", State: queue.StatePermission, Since: night0}}
	if res := s.act(t.Context(), command("opencode-compact")); res.Err == "" || len(*ran) != 0 {
		t.Fatalf("permission: res %+v ran %q", res, *ran)
	}
}

func TestCommandOnlyFromAConsoleAndOnlyATypedOne(t *testing.T) {
	s, ran := commandTest(t, "claude", idleScreen)
	a := command("review")
	a.Console = false
	if res := s.act(t.Context(), a); res.Err != "keys go only from a console" {
		t.Fatalf("err %q", res.Err)
	}
	for _, id := range []string{"nope", "compact"} {
		if res := s.act(t.Context(), command(id)); res.Err != `not sent: "`+id+`" is no command this host types` {
			t.Fatalf("%s: err %q", id, res.Err)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

// TestCompactAndClearFromTheChatCheckTheAgent: the chat names the agent
// it showed; a new one in the pane gets no compact and no clear.
func TestCompactAndClearFromTheChatCheckTheAgent(t *testing.T) {
	for _, act := range []string{"compact:", "clear:"} {
		s, ran := commandTest(t, "claude", idleScreen)
		res := s.act(t.Context(), proto.Act{Console: true, RecID: act + "%1", PID: 41, Confirm: "y"})
		if res.Err != "another agent runs in %1 now" || len(*ran) != 0 {
			t.Fatalf("%s: res %+v ran %q", act, res, *ran)
		}
		if res := s.act(t.Context(), proto.Act{Console: true, RecID: act + "%1", PID: 42, Confirm: "y"}); res.Err != "" || len(*ran) != 2 {
			t.Fatalf("%s: res %+v ran %q", act, res, *ran)
		}
	}
}
