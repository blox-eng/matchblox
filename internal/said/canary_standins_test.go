//go:build canary

package said

import (
	"bufio"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

type sseEvent struct {
	name string
	data string
}

// postSSE posts body to path on h and returns the server-sent events it
// answers with.
func postSSE(t *testing.T, h standIn, path, body string) []sseEvent {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", path, strings.NewReader(body)))
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type %q, want text/event-stream; body:\n%s", ct, rec.Body)
	}
	var events []sseEvent
	var e sseEvent
	sc := bufio.NewScanner(rec.Body)
	for sc.Scan() {
		switch line := sc.Text(); {
		case line == "":
			if e.data != "" {
				events = append(events, e)
			}
			e = sseEvent{}
		case strings.HasPrefix(line, "event: "):
			e.name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			e.data = strings.TrimPrefix(line, "data: ")
		default:
			t.Fatalf("line %q is not server-sent events", line)
		}
	}
	if e.data != "" {
		t.Fatalf("the stream ends without a blank line after %q", e.data)
	}
	return events
}

// responsesEvents checks that each event names its own type and returns the
// decoded events.
func responsesEvents(t *testing.T, events []sseEvent) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, e := range events {
		var m map[string]any
		if err := json.Unmarshal([]byte(e.data), &m); err != nil {
			t.Fatalf("event %s: %v", e.name, err)
		}
		if m["type"] != e.name {
			t.Fatalf("event %s carries type %v", e.name, m["type"])
		}
		out = append(out, m)
	}
	return out
}

func eventTypes(events []map[string]any) []string {
	var types []string
	for _, e := range events {
		types = append(types, e["type"].(string))
	}
	return types
}

const codexRequest = `{"model":"stand-in","instructions":"x","stream":true,
	"tools":[{"type":"function","name":"shell","parameters":{"type":"object"}}],
	"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"make the canary file"}]}%s]}`

func TestResponsesStandInCallsShell(t *testing.T) {
	s := &responsesStandIn{}
	events := responsesEvents(t, postSSE(t, s, "/v1/responses", strings.Replace(codexRequest, "%s", "", 1)))

	want := []string{
		"response.created",
		"response.output_item.added",
		"response.function_call_arguments.delta",
		"response.function_call_arguments.done",
		"response.output_item.done",
		"response.completed",
	}
	if got := eventTypes(events); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("events %v, want %v", got, want)
	}
	done := events[4]["item"].(map[string]any)
	if done["type"] != "function_call" || done["name"] != "shell" || done["call_id"] == "" {
		t.Fatalf("output item %v is not a shell function_call", done)
	}
	var args struct{ Command []string }
	if err := json.Unmarshal([]byte(done["arguments"].(string)), &args); err != nil {
		t.Fatal(err)
	}
	if cmd := args.Command; len(cmd) == 0 || cmd[len(cmd)-1] != "touch canary-ran" {
		t.Fatalf("shell command %q, want it to run touch canary-ran", cmd)
	}
	if events[3]["arguments"] != done["arguments"] {
		t.Fatalf("arguments.done %v differs from the item's %v", events[3]["arguments"], done["arguments"])
	}
	resp := events[5]["response"].(map[string]any)
	if resp["status"] != "completed" || resp["id"] == "" {
		t.Fatalf("response.completed carries %v", resp)
	}
	if n := s.toolRequests(); n != 1 {
		t.Fatalf("toolRequests %d, want 1", n)
	}
}

func TestResponsesStandInAnswersAShellResult(t *testing.T) {
	s := &responsesStandIn{}
	output := `,{"type":"function_call","call_id":"call_canary","name":"shell","arguments":"{}"},
		{"type":"function_call_output","call_id":"call_canary","output":"ok"}`
	events := responsesEvents(t, postSSE(t, s, "/v1/responses", strings.Replace(codexRequest, "%s", output, 1)))

	types := eventTypes(events)
	if !strings.Contains(strings.Join(types, " "), "response.output_text.delta") || types[len(types)-1] != "response.completed" {
		t.Fatalf("events %v, want a text answer that completes", types)
	}
	for _, e := range events {
		if item, ok := e["item"].(map[string]any); ok && item["type"] != "message" {
			t.Fatalf("output item %v, want a message", item)
		}
	}
	if n := s.toolRequests(); n != 0 {
		t.Fatalf("toolRequests %d, want 0", n)
	}
}
