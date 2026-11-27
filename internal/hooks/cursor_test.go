package hooks

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

const cursorChat = "19bece93-9e50-4441-b026-46d1cbf6e177"

func parseCursor(t *testing.T, file string) Event {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "testdata", "hooks", file))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ev, err := ParseCursor(f, env(map[string]string{"TMUX_PANE": "%4"}))
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

// TestCursorEventsAreTheSixCursorAccepts: Cursor ignores its whole
// hooks.json, silently, when one event name in it is one it does not take
// (notification did that on a live machine). The door writes this list.
func TestCursorEventsAreTheSixCursorAccepts(t *testing.T) {
	want := []string{"sessionStart", "sessionEnd", "beforeSubmitPrompt", "stop", "afterAgentResponse", "postToolUse"}
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
		// Cursor writes its transcript after the stop hook: the reply comes
		// with afterAgentResponse, right after.
		{"cursor-stop-completed.json", "Stop", "", false},
		{"cursor-afterAgentResponse.json", Reply, "Shall I also push it?", true},
		// Esc mid-turn: the person stopped it, nothing waits.
		{"cursor-stop-aborted.json", "UserPromptSubmit", "", false},
		// Ctrl-C, mid-turn or at rest.
		{"cursor-stop-error.json", "UserPromptSubmit", "", false},
		{"cursor-sessionEnd.json", "SessionEnd", "", false},
	} {
		ev := parseCursor(t, c.file)
		if ev.Name != c.name || ev.Message != c.message || ev.Asks != c.asks {
			t.Errorf("%s: %q %q asks=%v, want %q %q asks=%v", c.file, ev.Name, ev.Message, ev.Asks, c.name, c.message, c.asks)
		}
		if ev.SessionID != cursorChat || ev.Agent != CursorAgent || ev.Pane != "%4" || ev.Cwd != "/work/app" {
			t.Errorf("%s: %+v, want chat %s of %s in %%4, /work/app", c.file, ev, cursorChat, CursorAgent)
		}
	}
	for _, f := range []string{"cursor-stop-completed.json", "cursor-afterAgentResponse.json"} {
		if ev := parseCursor(t, f); ev.Turn != "3600b2ee-f3ad-4db6-95b9-0e4b521b3f74" {
			t.Errorf("%s: turn %q, want its generation_id", f, ev.Turn)
		}
	}
	if ev := parseCursor(t, "cursor-postToolUse.json"); ev.ToolUseID != "dd61bbbb-d9e0-40e9-9460-e42bf6eafaa0" {
		t.Errorf("postToolUse: tool_use_id %q", ev.ToolUseID)
	}
}

func TestParseCursorDropsUnknownEvents(t *testing.T) {
	if _, err := ParseCursor(strings.NewReader(`{"hook_event_name":"afterAgentThought","session_id":"x"}`), env(nil)); err == nil {
		t.Error("afterAgentThought parsed, want an error")
	}
}

func cursorPayload(t *testing.T, event, field, text string) Event {
	t.Helper()
	b, err := json.Marshal(map[string]string{"session_id": cursorChat, "hook_event_name": event, field: text})
	if err != nil {
		t.Fatal(err)
	}
	ev, err := ParseCursor(bytes.NewReader(b), env(nil))
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

// TestCursorKeepsTheTurnsText: a capture of a Cursor match holds its recent
// conversation, and Cursor writes it nowhere matchblox can read but its
// hooks. The text is untrusted: a prompt can carry any byte.
func TestCursorKeepsTheTurnsText(t *testing.T) {
	ev := cursorPayload(t, "beforeSubmitPrompt", "prompt", "fix \x1b[31mthe\x1b[0m\x00 build\x1b]52;c;cGF3bmVk\x07")
	if ev.Text != "fix the build" {
		t.Errorf("hostile prompt: Text %q, want %q", ev.Text, "fix the build")
	}
	if ev := parseCursor(t, "cursor-beforeSubmitPrompt.json"); ev.Name != "UserPromptSubmit" || ev.Text != "Commit the fix" {
		t.Errorf("beforeSubmitPrompt: %q Text %q, want UserPromptSubmit %q", ev.Name, ev.Text, "Commit the fix")
	}
	ev = cursorPayload(t, "afterAgentResponse", "text", "Fixed the null check.\nThe tests pass.\nShall I also push it?")
	if ev.Name != Reply || ev.Message != "Shall I also push it?" || ev.Text != "Fixed the null check.\nThe tests pass.\nShall I also push it?" {
		t.Errorf("afterAgentResponse: %q Message %q Text %q, want the last line and all three", ev.Name, ev.Message, ev.Text)
	}
	long := "LOG START " + strings.Repeat("журнал ", 40000/len("журнал ")) + " NOW FIX THE NULL CHECK"
	ev = cursorPayload(t, "beforeSubmitPrompt", "prompt", long)
	if len(ev.Text) > TextLimit || !utf8.ValidString(ev.Text) || !strings.HasPrefix(ev.Text, "LOG START") || !strings.HasSuffix(ev.Text, "NOW FIX THE NULL CHECK") || !strings.Contains(ev.Text, "[middle of message omitted]") {
		t.Errorf("a %d-byte prompt: Text of %d bytes, valid %v, starts %q, ends %q", len(long), len(ev.Text), utf8.ValidString(ev.Text), ev.Text[:min(20, len(ev.Text))], ev.Text[max(0, len(ev.Text)-30):])
	}
	for _, f := range []string{"cursor-sessionStart.json", "cursor-stop-completed.json", "cursor-postToolUse.json"} {
		if ev := parseCursor(t, f); ev.Text != "" {
			t.Errorf("%s: Text %q, want none", f, ev.Text)
		}
	}
}
