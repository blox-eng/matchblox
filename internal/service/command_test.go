package service

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

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

// consoleCompact is the chat's compact act on the agent pid of commandTest,
// for a live screen the console showed a minute into the night.
func consoleCompact(pid int) proto.Act {
	return proto.Act{Console: true, RecID: "compact:%1", PID: pid, Seen: night0.Add(time.Minute), Confirm: "y"}
}

// TestACompactNeedsTheLiveScreenSeen: a compact ends in an Enter, so it goes,
// as an Enter does, only for a live screen the console showed, and only to the
// agent it showed, by pid and start.
func TestACompactNeedsTheLiveScreenSeen(t *testing.T) {
	s, ran := commandTest(t, "claude", idleScreen)
	unseen := consoleCompact(42)
	unseen.Seen = time.Time{}
	if res := s.act(t.Context(), unseen); !strings.HasSuffix(res.Err, "the console has not shown work's screen yet — look again") || len(*ran) != 0 {
		t.Fatalf("unseen: res %+v ran %q", res, *ran)
	}
	s, ran = commandTest(t, "claude", idleScreen)
	restarted := consoleCompact(42)
	restarted.Start = 9
	if res := s.act(t.Context(), restarted); res.Err != "another agent runs in %1 now" || len(*ran) != 0 {
		t.Fatalf("restarted: res %+v ran %q", res, *ran)
	}
}

// TestCompactFromTheChatChecksTheAgent: the chat names the agent it
// showed; a new one in the pane gets no compact, and a console's compact
// with no agent named is refused.
func TestCompactFromTheChatChecksTheAgent(t *testing.T) {
	for _, pid := range []int{41, 0} {
		s, ran := commandTest(t, "claude", idleScreen)
		res := s.act(t.Context(), consoleCompact(pid))
		if res.Err != "another agent runs in %1 now" || len(*ran) != 0 {
			t.Fatalf("pid %d: res %+v ran %q", pid, res, *ran)
		}
	}
	s, ran := commandTest(t, "claude", idleScreen)
	if res := s.act(t.Context(), consoleCompact(42)); res.Err != "" || len(*ran) != 2 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
}
