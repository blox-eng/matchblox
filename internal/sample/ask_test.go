package sample

import (
	"path/filepath"
	"strings"
	"testing"
)

// claudeAsks is a turn as Claude Code 2.1.296 writes it while its question
// tool waits: the text, then the tool call with its questions, no result.
const claudeAsks = `{"type":"assistant","isSidechain":false,"timestamp":"2026-10-12T13:05:00Z","message":{"model":"claude-haiku-5-5","content":[{"type":"text","text":"Task 10 is done. Two ways on from here."}],"usage":{"input_tokens":10,"output_tokens":20}}}
{"type":"assistant","isSidechain":false,"timestamp":"2026-10-12T13:05:01Z","message":{"model":"claude-haiku-5-5","content":[{"type":"tool_use","id":"toolu_012X","name":"AskUserQuestion","input":{"questions":[{"question":"Which next?","header":"Next","multiSelect":false,"options":[{"label":"Start task 11","description":"the plan's order"},{"label":"Fix the flake"}]}]}}],"usage":{"input_tokens":10,"output_tokens":30}}}
`

func TestAPendingQuestionToolIsRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	writeFile(t, path, claudeAsks)
	u, _ := lastUsage(path, int64(len(claudeAsks)))
	if u.Said != "Task 10 is done. Two ways on from here." {
		t.Fatalf("said %q", u.Said)
	}
	a := u.Ask
	if a == nil || a.ID != "toolu_012X" || len(a.Questions) != 1 || a.Questions[0].Question != "Which next?" ||
		len(a.Questions[0].Options) != 2 || a.Questions[0].Options[0].Label != "Start task 11" {
		t.Fatalf("ask %+v", a)
	}
	answered := claudeAsks + `{"type":"user","isSidechain":false,"timestamp":"2026-10-12T13:06:00Z","message":{"content":[{"type":"tool_result","content":"Your questions have been answered","tool_use_id":"toolu_012X"}]}}` + "\n"
	writeFile(t, path, answered)
	if u, _ := lastUsage(path, int64(len(answered))); u.Ask != nil {
		t.Fatalf("an answered question still waits: %+v", u.Ask)
	}
}

func TestWhatASessionSaidIsBounded(t *testing.T) {
	long := strings.Repeat("word ", maxSaid)
	body := `{"type":"assistant","isSidechain":false,"timestamp":"2026-10-12T13:05:00Z","message":{"model":"m","content":[{"type":"text","text":"` + long + `"}],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n"
	path := filepath.Join(t.TempDir(), "s.jsonl")
	writeFile(t, path, body)
	u, _ := lastUsage(path, int64(len(body)))
	if n := len([]rune(u.Said)); n != maxSaid+1 || !strings.HasPrefix(u.Said, "…") {
		t.Fatalf("said is %d runes", n)
	}
}

// A long reply keeps its end, where the question is; its Markdown reads
// plain (review of #68, finding 7).
func TestWhatASessionSaidKeepsItsEnd(t *testing.T) {
	long := strings.Repeat("word ", maxSaid) + "**Shall I** open the \u0060PR\u0060?"
	body := `{"type":"assistant","isSidechain":false,"timestamp":"2026-10-12T13:05:00Z","message":{"model":"m","content":[{"type":"text","text":"` + long + `"}],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n"
	path := filepath.Join(t.TempDir(), "s.jsonl")
	writeFile(t, path, body)
	u, _ := lastUsage(path, int64(len(body)))
	if !strings.HasPrefix(u.Said, "…") || !strings.HasSuffix(u.Said, "word Shall I open the PR?") {
		t.Fatalf("said %q", u.Said[max(len(u.Said)-60, 0):])
	}
}

// A question tool call with no words before it in its turn says nothing:
// the reply before the person's last prompt was another turn's.
func TestSaidIsOnlyThisTurns(t *testing.T) {
	turn := `{"type":"assistant","isSidechain":false,"timestamp":"2026-10-12T13:00:00Z","message":{"model":"m","content":[{"type":"text","text":"Yesterday's reply."}],"usage":{"input_tokens":1,"output_tokens":1}}}
{"type":"system","subtype":"turn_duration","timestamp":"2026-10-12T13:00:01Z"}
{"type":"user","isSidechain":false,"timestamp":"2026-10-12T13:04:00Z","message":{"role":"user","content":"pick the next task"}}
{"type":"assistant","isSidechain":false,"timestamp":"2026-10-12T13:05:01Z","message":{"model":"m","content":[{"type":"tool_use","id":"toolu_9","name":"AskUserQuestion","input":{"questions":[{"question":"Which next?","options":[{"label":"a"},{"label":"b"}]}]}}],"usage":{"input_tokens":10,"output_tokens":30}}}
`
	path := filepath.Join(t.TempDir(), "s.jsonl")
	writeFile(t, path, turn)
	u, _ := lastUsage(path, int64(len(turn)))
	if u.Said != "" || u.Ask == nil {
		t.Fatalf("said %q ask %+v", u.Said, u.Ask)
	}
	// A tool's result in the turn is not a prompt: the words before it stay.
	withTool := strings.Replace(turn, `{"type":"user","isSidechain":false,"timestamp":"2026-10-12T13:04:00Z","message":{"role":"user","content":"pick the next task"}}`,
		`{"type":"user","isSidechain":false,"timestamp":"2026-10-12T13:04:00Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`, 1)
	writeFile(t, path, withTool)
	if u, _ := lastUsage(path, int64(len(withTool))); u.Said != "Yesterday's reply." {
		t.Fatalf("said %q", u.Said)
	}
}
