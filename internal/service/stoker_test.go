package service

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// night0 is in the past: an ack runs on the real clock.
var night0 = time.Now().Add(-time.Hour).Truncate(time.Second)

// stokerTest is a service with the stoker on and one Claude Code session
// at 88 %, idle in pane %1, whose transcript is in a temp dir.
func stokerTest(t *testing.T) (*Service, *[][]string, string) {
	t.Helper()
	s := newTest(t, time.Hour)
	dir := t.TempDir()
	st, err := stoker.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Stoker = st
	if err := st.Set(stoker.Mode{}.TurnOn(night0)); err != nil {
		t.Fatal(err)
	}
	tr := filepath.Join(dir, "s1.jsonl")
	s.publish(sample.Snapshot{At: night0, Sessions: []sample.Session{{
		Agent: "claude", Pane: "%1", SessionID: "s1", Name: "work", Status: "idle",
		Idle: 5 * time.Minute, Tokens: 176000, ContextPct: 88, Transcript: tr,
	}}}, func(state.Doc) {})
	// The pane shows the empty prompt, then the text the step typed.
	var mu sync.Mutex
	var ran [][]string
	screen := idleScreen
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
			screen = strings.Replace(idleScreen, "> \n", "> "+argv[len(argv)-1]+"\n", 1)
		}
		return nil
	}
	return s, &ran, tr
}

func TestStokerCompactsAndReadsTheResumeBack(t *testing.T) {
	s, ran, tr := stokerTest(t)
	s.stoke(context.Background(), night0)
	if len(*ran) != 2 || !strings.HasPrefix((*ran)[0][len((*ran)[0])-1], "/compact Keep the task") || (*ran)[1][4] != "Enter" {
		t.Fatalf("ran %q", *ran)
	}
	es := s.Stoker.Entries()
	if len(es) != 1 || es[0].Result != stoker.Sent || es[0].Guard != "idle" || es[0].Before != 176000 || len(es[0].Argv) != 2 {
		t.Fatalf("entries %+v", es)
	}
	writeTranscript(t, tr, night0.Add(90*time.Second), "RESUME: run the migration test, then open the PR")
	s.stoke(context.Background(), night0.Add(2*time.Minute))
	if len(*ran) != 2 {
		t.Fatalf("compacted again within 30 min: %q", *ran)
	}
	got := s.State().Stoker
	if got == nil || len(got.Run) != 1 || got.Run[0].After != 9400 || got.Run[0].Resume != "run the migration test, then open the PR" || !got.Unseen {
		t.Fatalf("view %+v", got)
	}
}

func TestStokerSkipsBusySession(t *testing.T) {
	s, ran, _ := stokerTest(t)
	// The sample said idle; the host says busy right before the send.
	s.Actions.Idle = func(string) bool { return false }
	s.stoke(context.Background(), night0)
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
	es := s.Stoker.Entries()
	if len(es) != 1 || !strings.HasPrefix(es[0].Result, stoker.Skipped+": ") || !strings.Contains(es[0].Guard, "no longer idle") {
		t.Fatalf("entries %+v", es)
	}
}

func TestStokerNeverAnswers(t *testing.T) {
	s, ran, _ := stokerTest(t)
	s.mu.Lock()
	s.cur.Queue = []queue.Item{{Pane: "%1", State: queue.StatePermission}}
	s.mu.Unlock()
	s.stoke(context.Background(), night0)
	if len(*ran) != 0 || len(s.Stoker.Entries()) != 0 {
		t.Fatalf("stepped on a waiting session: %q", *ran)
	}
}

func TestStokerOffSendsNothing(t *testing.T) {
	s, ran, _ := stokerTest(t)
	if r := s.act(context.Background(), proto.Act{RecID: "stoker:off"}); r.Err != "" {
		t.Fatal(r.Err)
	}
	s.stoke(context.Background(), night0)
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

func TestNightSurvivesRestartAndEndsInTheMorning(t *testing.T) {
	s, _, _ := stokerTest(t)
	_ = s.Stoker.Set(stoker.Mode{})
	s.NightEnds = "07:00"
	if r := s.act(context.Background(), proto.Act{RecID: "stoker:night"}); r.Err != "" {
		t.Fatal(r.Err)
	}
	if st := s.State().Stoker; st == nil || !st.On || st.Until.IsZero() {
		t.Fatalf("view after n %+v", st)
	}
	until := s.Stoker.Mode().Until
	again, err := stoker.Open(storeDir(t, s))
	if err != nil || !again.Mode().On || !again.Mode().Until.Equal(until) {
		t.Fatalf("after restart %+v %v", again.Mode(), err)
	}
	s.stoke(context.Background(), until)
	if s.Stoker.Mode().On || s.State().Stoker.On {
		t.Fatal("the night did not end in the morning")
	}
}

func TestStokerAcksAndRefusesUnknown(t *testing.T) {
	s, _, _ := stokerTest(t)
	s.stoke(context.Background(), night0)
	if r := s.act(context.Background(), proto.Act{RecID: "stoker:ack"}); r.Err != "" || s.State().Stoker.Unseen {
		t.Fatalf("ack %+v %+v", r, s.State().Stoker)
	}
	if r := s.act(context.Background(), proto.Act{RecID: "stoker:fire"}); r.Err == "" {
		t.Fatal("an unknown stoker act ran")
	}
	s.Stoker = nil
	if r := s.act(context.Background(), proto.Act{RecID: "stoker:on"}); r.Err == "" {
		t.Fatal("no stoker, no error")
	}
}

func TestNightEndsComesFromTheConfig(t *testing.T) {
	cfg := config.Default()
	if cfg.Stoker.NightEnds != "07:00" {
		t.Fatalf("default night_ends %q", cfg.Stoker.NightEnds)
	}
	cfg.Stoker.NightEnds = "06:30"
	if s := New(cfg, nil, nil); s.NightEnds != "06:30" {
		t.Fatalf("service night ends %q", s.NightEnds)
	}
}

func writeTranscript(t *testing.T, path string, at time.Time, resume string) {
	t.Helper()
	ts := at.UTC().Format(time.RFC3339)
	body := `{"type":"system","subtype":"compact_boundary","timestamp":"` + ts + `","compactMetadata":{"trigger":"manual","preTokens":176100,"postTokens":9400}}
{"type":"user","isCompactSummary":true,"timestamp":"` + ts + `","message":{"role":"user","content":"Summary.\n` + resume + `\n"}}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// storeDir is where the test's store keeps its files.
func storeDir(t *testing.T, s *Service) string {
	t.Helper()
	return s.Stoker.Dir()
}

// idleScreen is Claude Code waiting at an empty prompt.
const idleScreen = "⏺ Done. The migration is written.\n\n────────────────────\n> \n────────────────────\n  ? for shortcuts\n"

// Claude Code sends idle_prompt about a minute after any turn ends: it is
// a reminder, not a question, and must not keep a full session from its
// compact all night.
func TestStokerCompactsAfterTheIdleReminder(t *testing.T) {
	s, ran, _ := stokerTest(t)
	s.Hook(hooks.Event{Name: "Stop", SessionID: "s1", Pane: "%1", At: night0.Add(-2 * time.Minute)})
	s.Hook(hooks.Event{Name: "Notification", Kind: hooks.KindIdlePrompt, SessionID: "s1", Pane: "%1", Message: "Claude is waiting for your input", At: night0.Add(-time.Minute)})
	s.stoke(context.Background(), night0)
	if len(*ran) != 2 {
		t.Fatalf("a finished turn with its idle reminder got %q", *ran)
	}
}

func TestStokerLeavesARealQuestion(t *testing.T) {
	s, ran, _ := stokerTest(t)
	s.Hook(hooks.Event{Name: "Stop", SessionID: "s1", Pane: "%1", Asks: true, Message: "Drop the old table?", At: night0.Add(-2 * time.Minute)})
	s.Hook(hooks.Event{Name: "Notification", Kind: hooks.KindIdlePrompt, SessionID: "s1", Pane: "%1", At: night0.Add(-time.Minute)})
	s.stoke(context.Background(), night0)
	if len(*ran) != 0 {
		t.Fatalf("a question got %q", *ran)
	}
}

// The guard reads the pane on the host right before each step: a menu, a
// question or a draft there stops the step, whatever the sample said.
func TestStokerReadsThePaneBeforeTheSend(t *testing.T) {
	for name, screen := range map[string]string{
		"permission": "Bash command\n  rm -rf build\nDo you want to proceed?\n❯ 1. Yes\n  2. No, and tell Claude what to do differently\n",
		"question":   "⏺ Should I drop the old table?\n\n> \n",
		"draft":      "⏺ Done.\n────────\n> half a prompt the builder left\n────────\n",
		"unreadable": "",
	} {
		s, ran, _ := stokerTest(t)
		if screen == "" {
			s.Capture = func([]string) map[string]string { return map[string]string{} }
		} else {
			s.Capture = func([]string) map[string]string { return map[string]string{"%1": screen} }
		}
		s.stoke(context.Background(), night0)
		es := s.Stoker.Entries()
		if len(*ran) != 0 || len(es) != 1 || !strings.HasPrefix(es[0].Result, stoker.Skipped+": ") {
			t.Fatalf("%s: ran %q, entries %+v", name, *ran, es)
		}
	}
}

// A wait the hooks report between the text and its Enter stops the Enter.
func TestStokerChecksAgainBeforeTheEnter(t *testing.T) {
	s, ran, _ := stokerTest(t)
	run := s.Actions.Run
	s.Actions.Run = func(argv []string) error {
		err := run(argv)
		s.Hook(hooks.Event{Name: "Notification", Kind: hooks.KindPermission, SessionID: "s1", Pane: "%1", At: night0})
		return err
	}
	s.stoke(context.Background(), night0)
	if len(*ran) != 1 {
		t.Fatalf("ran %q", *ran)
	}
	if es := s.Stoker.Entries(); len(es) != 1 || es[0].Result == stoker.Sent {
		t.Fatalf("entries %+v", es)
	}
}
