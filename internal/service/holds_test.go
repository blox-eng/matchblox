package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/holds"
	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

func holdsTest(t *testing.T, sessions ...sample.Session) (*Service, *[][]string) {
	t.Helper()
	s := newTest(t, time.Hour)
	hs, err := holds.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.Holds = hs
	var ran [][]string
	s.Actions.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	s.publish(sample.Snapshot{At: time.Now(), Sessions: sessions}, func(state.Doc) {})
	return s, &ran
}

func flint() sample.Session {
	return sample.Session{PID: 9, Start: 3, Pane: "%3", Name: "flint", SessionID: "s1", Status: "busy", Busy: true}
}

func TestParkTogglesAndRefusesMessages(t *testing.T) {
	s, ran := holdsTest(t, flint())
	ctx := t.Context()
	if res := s.act(ctx, proto.Act{Console: true, RecID: "park:%3"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if h := s.State().Holds; len(h) != 1 || h[0].SessionID != "s1" || h[0].By != holds.ByYou || h[0].Pane != "%3" {
		t.Fatalf("holds %+v", h)
	}
	res := s.act(ctx, proto.Act{RecID: "message:%3", Which: "secondary", Confirm: "y", Text: "go"})
	if res.Err != "flint is parked: z lets it go" {
		t.Fatalf("err %q", res.Err)
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
	if res := s.act(ctx, proto.Act{Console: true, RecID: "park:%3"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if h := s.State().Holds; len(h) != 0 {
		t.Fatalf("still held %+v", h)
	}
}

func TestHoldFollowsTheSessionNotThePane(t *testing.T) {
	s, _ := holdsTest(t, flint())
	if res := s.act(t.Context(), proto.Act{Console: true, RecID: "park:%3"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	now := time.Now()
	s.Now = func() time.Time { return now }
	other := sample.Session{PID: 12, Pane: "%3", Name: "other", SessionID: "s2", Status: "idle"}
	s.publish(sample.Snapshot{At: now, Sessions: []sample.Session{other}}, func(state.Doc) {})
	if _, held := s.heldOf("s2"); held {
		t.Fatal("the new agent in the pane is held")
	}
	if err := s.messageable("%3", 12); err != nil {
		t.Fatalf("new agent refused: %v", err)
	}
	if h := s.State().Holds; len(h) != 1 || h[0].SessionID != "s1" {
		t.Fatalf("holds %+v", h)
	}
	now = now.Add(holds.GoneFor)
	s.publish(sample.Snapshot{At: now, Sessions: []sample.Session{other}}, func(state.Doc) {})
	if h := s.State().Holds; len(h) != 0 {
		t.Fatalf("gone for 24 h, still held: %+v", h)
	}
}

func TestStokerHoldQueuesOneMessage(t *testing.T) {
	s, ran := holdsTest(t, flint())
	res := s.act(t.Context(), proto.Act{RecID: "hold:%3", Confirm: "y", Text: "not priority"})
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	want := [][]string{
		{"tmux", "send-keys", "-t", "%3", "-l", "--", "finish this step, then stop and wait"},
		{"tmux", "send-keys", "-t", "%3", "Enter"},
	}
	if fmt.Sprint(*ran) != fmt.Sprint(want) {
		t.Fatalf("ran %q", *ran)
	}
	h, ok := s.Holds.Of("s1")
	if !ok || h.By != holds.ByStoker || h.Why != "not priority" {
		t.Fatalf("hold %+v %v", h, ok)
	}
	if err := s.messageable("%3", 9); err == nil || err.Error() != "flint is held by the stoker: z lets it go" {
		t.Fatalf("err %v", err)
	}
	*ran = nil
	if res := s.act(t.Context(), proto.Act{Console: true, RecID: "release:%3", Confirm: "y"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	want = [][]string{
		{"tmux", "send-keys", "-t", "%3", "-l", "--", "carry on"},
		{"tmux", "send-keys", "-t", "%3", "Enter"},
	}
	if fmt.Sprint(*ran) != fmt.Sprint(want) {
		t.Fatalf("release ran %q", *ran)
	}
	if _, ok := s.Holds.Of("s1"); ok {
		t.Fatal("still held")
	}
}

func TestReleaseOfAParkSendsNothing(t *testing.T) {
	s, ran := holdsTest(t, flint())
	s.act(t.Context(), proto.Act{Console: true, RecID: "park:%3"})
	if res := s.act(t.Context(), proto.Act{Console: true, RecID: "release:%3", Confirm: "y"}); res.Err != "" || len(*ran) != 0 {
		t.Fatalf("%+v %q", res, *ran)
	}
	if len(s.Holds.List()) != 0 {
		t.Fatal("still parked")
	}
}

// TestParkACursorSessionJoinedByItsHook: Cursor's sampler has no session
// file; its hooks name the chat. After a turn, z parks that chat.
func TestParkACursorSessionJoinedByItsHook(t *testing.T) {
	cur := sample.Session{PID: 9, Start: 3, Pane: "%3", Name: "cur", Agent: sample.CursorAgent, Status: "idle"}
	s, _ := holdsTest(t, cur)
	s.Hook(hooks.Event{Name: "Stop", Agent: hooks.CursorAgent, SessionID: "chat1", Pane: "%3", At: time.Now()})
	if x, ok := s.sessionAt("%3"); !ok || x.SessionID != "chat1" {
		t.Fatalf("session %+v, want chat1 from its hook", x)
	}
	if res := s.act(t.Context(), proto.Act{Console: true, RecID: "park:%3"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if h := s.State().Holds; len(h) != 1 || h[0].SessionID != "chat1" || h[0].By != holds.ByYou {
		t.Fatalf("holds %+v", h)
	}
}

// TestAParkedSessionThatLosesItsIDStaysParked: a Codex session's id comes
// from a rollout the sampler can lose sight of (two Codex in a directory,
// a /new). While the pane's session has no id, the builder's park on that
// pane still refuses messages, as the console still shows it parked.
func TestAParkedSessionThatLosesItsIDStaysParked(t *testing.T) {
	cx := flint()
	cx.Agent = "codex"
	s, ran := holdsTest(t, cx)
	ctx := t.Context()
	if res := s.act(ctx, proto.Act{Console: true, RecID: "park:%3"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	cx.SessionID = ""
	s.publish(sample.Snapshot{At: time.Now(), Sessions: []sample.Session{cx}}, func(state.Doc) {})
	res := s.act(ctx, proto.Act{RecID: "message:%3", Which: "secondary", Confirm: "y", Text: "go"})
	if res.Err != "flint is parked: z lets it go" || len(*ran) != 0 {
		t.Fatalf("err %q, ran %q: a parked pane took a message once its id was lost", res.Err, *ran)
	}
}

// TestParkSaysWhyACodexSessionHasNoID: a Codex session whose rollout
// cannot be told apart gets no id from more turns, so z must not say "after
// its first turn".
func TestParkSaysWhyACodexSessionHasNoID(t *testing.T) {
	cx := flint()
	cx.Agent, cx.SessionID, cx.Context = "codex", "", "unmeasured"
	s, _ := holdsTest(t, cx)
	res := s.act(t.Context(), proto.Act{Console: true, RecID: "park:%3"})
	if want := "flint has no session id: its Codex rollout cannot be told apart from another's"; res.Err != want {
		t.Fatalf("err %q, want %q", res.Err, want)
	}
}
