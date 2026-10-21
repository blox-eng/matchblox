package hooks

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const cursorChat = "19bece93-9e50-4441-b026-46d1cbf6e177"

// cursorFixture is a payload of testdata/hooks, as Cursor sent it on a live
// machine, its transcript_path pointing at transcript ("" keeps the
// fixture's own, made absolute).
func cursorFixture(t *testing.T, file, transcript string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "hooks", file))
	if err != nil {
		t.Fatal(err)
	}
	if transcript == "" {
		transcript, _ = filepath.Abs(filepath.Join("..", "..", "testdata", "hooks", "cursor-transcript.jsonl"))
	}
	return strings.Replace(string(b), "testdata/hooks/cursor-transcript.jsonl", transcript, 1)
}

func parseCursor(t *testing.T, payload string) Event {
	t.Helper()
	ev, err := ParseCursor(strings.NewReader(payload), env(map[string]string{"TMUX_PANE": "%4"}))
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

// TestCursorEventsAreTheFiveCursorAccepts: Cursor ignores its whole
// hooks.json, silently, when one event name in it is one it does not take
// (notification did that on a live machine). The door writes this list.
func TestCursorEventsAreTheFiveCursorAccepts(t *testing.T) {
	want := []string{"sessionStart", "sessionEnd", "beforeSubmitPrompt", "stop", "postToolUse"}
	if !slices.Equal(CursorEvents, want) {
		t.Errorf("CursorEvents = %q, want %q", CursorEvents, want)
	}
}

func TestParseCursorMapsEachEvent(t *testing.T) {
	for _, c := range []struct {
		file, name, message string
		asks                bool
	}{
		{"cursor-sessionStart.json", "SessionStart", "", false},
		{"cursor-beforeSubmitPrompt.json", "UserPromptSubmit", "", false},
		{"cursor-postToolUse.json", "PostToolUse", "", false},
		{"cursor-stop-completed.json", "Stop", "Shall I also push it?", true},
		// Esc mid-turn: the person stopped it, nothing waits.
		{"cursor-stop-aborted.json", "UserPromptSubmit", "", false},
		// Ctrl-C at rest, just before sessionEnd.
		{"cursor-stop-error.json", "UserPromptSubmit", "", false},
		{"cursor-sessionEnd.json", "SessionEnd", "", false},
	} {
		ev := parseCursor(t, cursorFixture(t, c.file, ""))
		if ev.Name != c.name || ev.Message != c.message || ev.Asks != c.asks {
			t.Errorf("%s: %q %q asks=%v, want %q %q asks=%v", c.file, ev.Name, ev.Message, ev.Asks, c.name, c.message, c.asks)
		}
		if ev.SessionID != cursorChat || ev.Agent != CursorAgent || ev.Pane != "%4" || ev.Cwd != "/work/app" {
			t.Errorf("%s: %+v, want chat %s of %s in %%4, /work/app", c.file, ev, cursorChat, CursorAgent)
		}
	}
	if ev := parseCursor(t, cursorFixture(t, "cursor-postToolUse.json", "")); ev.ToolUseID != "dd61bbbb-d9e0-40e9-9460-e42bf6eafaa0" {
		t.Errorf("postToolUse: tool_use_id %q", ev.ToolUseID)
	}
}

func TestParseCursorStopWithoutATranscriptStillStops(t *testing.T) {
	payload := strings.Replace(cursorFixture(t, "cursor-stop-completed.json", "x"), `"x"`, "null", 1)
	ev := parseCursor(t, payload)
	if ev.Name != "Stop" || ev.Message != "" {
		t.Errorf("%q %q, want a Stop with no message", ev.Name, ev.Message)
	}
	ev = parseCursor(t, cursorFixture(t, "cursor-stop-completed.json", filepath.Join(t.TempDir(), "gone.jsonl")))
	if ev.Name != "Stop" || ev.Message != "" {
		t.Errorf("missing transcript: %q %q, want a Stop with no message", ev.Name, ev.Message)
	}
}

func TestParseCursorReadsOnlyTheTranscriptsTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "long.jsonl")
	turn := `{"role":"assistant","message":{"content":[{"type":"text","text":"` + strings.Repeat("words ", 1000) + `"}]}}` + "\n"
	body := strings.Repeat(turn, 5<<20/len(turn)) +
		`{"role":"assistant","message":{"content":[{"type":"text","text":"done."}]}}` + "\n" +
		`{"type":"turn_ended","status":"success"}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if ev := parseCursor(t, cursorFixture(t, "cursor-stop-completed.json", path)); ev.Message != "done." {
		t.Errorf("message %q, want done.", ev.Message)
	}
}

func TestParseCursorDropsUnknownEvents(t *testing.T) {
	if _, err := ParseCursor(strings.NewReader(`{"hook_event_name":"afterAgentThought","session_id":"x"}`), env(nil)); err == nil {
		t.Error("afterAgentThought parsed, want an error")
	}
}
