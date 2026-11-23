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
	var args struct {
		Command            []string
		SandboxPermissions string `json:"sandbox_permissions"`
	}
	if err := json.Unmarshal([]byte(done["arguments"].(string)), &args); err != nil {
		t.Fatal(err)
	}
	if cmd := args.Command; len(cmd) == 0 || cmd[len(cmd)-1] != "touch canary-ran" {
		t.Fatalf("shell command %q, want it to run touch canary-ran", cmd)
	}
	// Under approval_policy "on-request" Codex asks only for a command that
	// wants to leave its sandbox.
	if args.SandboxPermissions != "require_escalated" {
		t.Fatalf("sandbox_permissions %q, want require_escalated so Codex asks", args.SandboxPermissions)
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

// Codex 0.171 offers exec_command, whose command is one string in cmd.
func TestResponsesStandInCallsExecCommand(t *testing.T) {
	s := &responsesStandIn{}
	req := strings.Replace(strings.Replace(codexRequest, `"name":"shell"`, `"name":"exec_command"`, 1), "%s", "", 1)
	events := responsesEvents(t, postSSE(t, s, "/v1/responses", req))
	if len(events) < 5 {
		t.Fatalf("events %v, want a function_call", eventTypes(events))
	}
	done, _ := events[4]["item"].(map[string]any)
	if done["type"] != "function_call" || done["name"] != "exec_command" {
		t.Fatalf("output item %v is not an exec_command function_call", done)
	}
	var args struct {
		Cmd                string
		SandboxPermissions string `json:"sandbox_permissions"`
		Justification      string
	}
	if err := json.Unmarshal([]byte(done["arguments"].(string)), &args); err != nil {
		t.Fatal(err)
	}
	if args.Cmd != "touch canary-ran" || args.SandboxPermissions != "require_escalated" || args.Justification == "" {
		t.Fatalf("exec_command arguments %+v, want cmd touch canary-ran, require_escalated and a justification", args)
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

// chatChunks decodes the chunks of a chat completions stream, which ends with
// data: [DONE].
func chatChunks(t *testing.T, events []sseEvent) []map[string]any {
	t.Helper()
	if len(events) == 0 || events[len(events)-1].data != "[DONE]" {
		t.Fatalf("the stream %v does not end with [DONE]", events)
	}
	var out []map[string]any
	for _, e := range events[:len(events)-1] {
		var m map[string]any
		if err := json.Unmarshal([]byte(e.data), &m); err != nil {
			t.Fatalf("chunk %s: %v", e.data, err)
		}
		if m["object"] != "chat.completion.chunk" {
			t.Fatalf("chunk %v is not a chat.completion.chunk", m)
		}
		out = append(out, m)
	}
	return out
}

func choice(chunk map[string]any) map[string]any {
	choices, _ := chunk["choices"].([]any)
	if len(choices) == 0 {
		return map[string]any{}
	}
	return choices[0].(map[string]any)
}

const opencodeRequest = `{"model":"stand-in","stream":true,
	"tools":[{"type":"function","function":{"name":"bash","parameters":{"type":"object"}}}],
	"messages":[{"role":"system","content":"x"},{"role":"user","content":"make the canary file"}%s]}`

func TestChatStandInCallsBash(t *testing.T) {
	s := &chatStandIn{}
	chunks := chatChunks(t, postSSE(t, s, "/v1/chat/completions", strings.Replace(opencodeRequest, "%s", "", 1)))

	var calls []map[string]any
	args := ""
	finish := ""
	for _, c := range chunks {
		ch := choice(c)
		if delta, ok := ch["delta"].(map[string]any); ok {
			tcs, _ := delta["tool_calls"].([]any)
			for _, tc := range tcs {
				tc := tc.(map[string]any)
				if tc["index"] != 0.0 {
					t.Fatalf("tool call %v, want index 0", tc)
				}
				if tc["id"] != nil {
					calls = append(calls, tc)
				}
				args += tc["function"].(map[string]any)["arguments"].(string)
			}
		}
		if r, ok := ch["finish_reason"].(string); ok {
			finish = r
		}
	}
	if len(calls) != 1 || calls[0]["type"] != "function" || calls[0]["function"].(map[string]any)["name"] != "bash" {
		t.Fatalf("tool calls %v, want one bash call", calls)
	}
	var cmd struct{ Command string }
	if err := json.Unmarshal([]byte(args), &cmd); err != nil || cmd.Command != "touch canary-ran" {
		t.Fatalf("arguments %q, want command touch canary-ran (%v)", args, err)
	}
	if finish != "tool_calls" {
		t.Fatalf("finish_reason %q, want tool_calls", finish)
	}
	if n := s.toolRequests(); n != 1 {
		t.Fatalf("toolRequests %d, want 1", n)
	}
}

func TestChatStandInAnswersABashResult(t *testing.T) {
	s := &chatStandIn{}
	result := `,{"role":"assistant","content":null,"tool_calls":[{"id":"call_canary","type":"function","function":{"name":"bash","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"call_canary","content":"ok"}`
	chunks := chatChunks(t, postSSE(t, s, "/v1/chat/completions", strings.Replace(opencodeRequest, "%s", result, 1)))

	text, finish := "", ""
	for _, c := range chunks {
		ch := choice(c)
		if delta, ok := ch["delta"].(map[string]any); ok {
			if delta["tool_calls"] != nil {
				t.Fatalf("chunk %v calls a tool again", c)
			}
			part, _ := delta["content"].(string)
			text += part
		}
		if r, ok := ch["finish_reason"].(string); ok {
			finish = r
		}
	}
	if text != "Done." || finish != "stop" {
		t.Fatalf("answer %q with finish_reason %q, want Done. and stop", text, finish)
	}
	if n := s.toolRequests(); n != 0 {
		t.Fatalf("toolRequests %d, want 0", n)
	}
}
