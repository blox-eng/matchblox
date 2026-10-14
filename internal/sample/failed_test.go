package sample

import (
	"path/filepath"
	"strings"
	"testing"
)

const (
	failReply  = `{"type":"assistant","isSidechain":false,"timestamp":"2026-10-13T10:00:00Z","message":{"model":"claude-opus-5-5","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{}}],"usage":{"input_tokens":10,"cache_read_input_tokens":1000,"cache_creation_input_tokens":0,"output_tokens":9}}}` + "\n"
	failTool   = `{"type":"user","isSidechain":false,"timestamp":"2026-10-13T10:00:05Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","is_error":true,"content":"Exit code 2\nmake: *** [test] Error 1 and a very long tail that goes past the bound"}]}}` + "\n"
	failAPI    = `{"type":"assistant","isSidechain":false,"timestamp":"2026-10-13T10:01:00Z","apiError":"overloaded","isApiErrorMessage":true,"message":{"model":"<synthetic>","content":[{"type":"text","text":"API Error: 529 overloaded"}],"usage":{"input_tokens":0,"output_tokens":0}}}` + "\n"
	failSaid   = `{"type":"assistant","isSidechain":false,"timestamp":"2026-10-13T10:02:00Z","message":{"model":"claude-opus-5-5","content":[{"type":"text","text":"Fixed it."}],"usage":{"input_tokens":10,"cache_read_input_tokens":1100,"cache_creation_input_tokens":0,"output_tokens":9}}}` + "\n"
	failPrompt = `{"type":"user","isSidechain":false,"timestamp":"2026-10-13T10:03:00Z","message":{"role":"user","content":"try again"}}` + "\n"
)

func failedOf(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	writeFile(t, path, body)
	u, ok := lastUsage(path, int64(len(body)))
	if !ok {
		t.Fatal("no usage")
	}
	return u.Failed
}

func TestTranscriptFailedTurn(t *testing.T) {
	if got := failedOf(t, failReply+failTool); got != "Exit code 2" {
		t.Errorf("a failed tool call: %q", got)
	}
	if got := failedOf(t, failReply+failAPI); got != "API Error: 529 overloaded" {
		t.Errorf("an API error: %q", got)
	}
	long := strings.Replace(failAPI, "API Error: 529 overloaded", strings.Repeat("é", 60), 1)
	if got := failedOf(t, failReply+long); len([]rune(got)) != 40 {
		t.Errorf("cut to 40 runes: %q", got)
	}
	if got := failedOf(t, claudeOut); got != "" {
		t.Errorf("the limit's refusal is Out, not a failure: %q", got)
	}
}

func TestTranscriptRecoveredTurnIsNotFailed(t *testing.T) {
	for name, body := range map[string]string{
		"the model spoke after a tool error": failReply + failTool + failSaid,
		"the model spoke after an API error": failReply + failAPI + failSaid,
		"a prompt followed the error":        failReply + failTool + failPrompt,
	} {
		if got := failedOf(t, body); got != "" {
			t.Errorf("%s: %q", name, got)
		}
	}
}
