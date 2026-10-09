package sample

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// claudeCompacted is a transcript as Claude Code 2.1 writes a manual
// compact: a turn with its usage, the boundary with the token counts, and
// the summary the model wrote.
const claudeCompacted = `{"type":"assistant","isSidechain":false,"timestamp":"2026-10-09T23:00:00Z","message":{"model":"claude-opus-5-5","content":[{"type":"text","text":"Done."}],"usage":{"input_tokens":10,"cache_read_input_tokens":170000,"cache_creation_input_tokens":0,"output_tokens":90}}}
{"type":"system","subtype":"compact_boundary","isSidechain":false,"timestamp":"2026-10-09T23:10:05Z","content":"Conversation compacted","compactMetadata":{"trigger":"manual","preTokens":170100,"postTokens":9400}}
{"type":"user","isSidechain":false,"isCompactSummary":true,"timestamp":"2026-10-09T23:10:05Z","message":{"role":"user","content":"This session is being continued from a previous conversation.\n\nSummary:\n1. The task: the migration.\n\n**RESUME:** run the migration test, then open the PR\n"}}
`

func TestResumeLine(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{"Summary\nRESUME: run the migration test, then open the PR\n", "run the migration test, then open the PR"},
		{"**RESUME:** run the test**", "run the test"},
		{"- RESUME: `go test ./...`", "`go test ./...`"},
		{"RESUME: first\nmore\nRESUME: second", "second"},
		{"The resume: is not a line that starts with it", ""},
		{"no line", ""},
	} {
		if got := ResumeLine(c.text); got != c.want {
			t.Errorf("ResumeLine(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

func TestCompactedReadsTheBoundaryAndTheResumeLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	writeFile(t, path, claudeCompacted)
	c, ok := Compacted(path, time.Date(2026, 10, 9, 23, 10, 0, 0, time.UTC))
	if !ok {
		t.Fatal("no compact found")
	}
	if c.Before != 170100 || c.After != 9400 || c.Resume != "run the migration test, then open the PR" {
		t.Fatalf("compact %+v", c)
	}
	if _, ok := Compacted(path, time.Date(2026, 10, 9, 23, 11, 0, 0, time.UTC)); ok {
		t.Fatal("a compact before the step counts as its result")
	}
}

// After a compact the next turn has not run yet: the context is what the
// boundary says is left, not the last turn before it. Else a compacted
// session still reads full and is compacted again.
func TestUsageAfterACompactIsWhatTheBoundaryLeaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	writeFile(t, path, claudeCompacted)
	u, ok := lastUsage(path, int64(len(claudeCompacted)))
	if !ok || u.Tokens != 9400 || u.Model != "claude-opus-5-5" {
		t.Fatalf("usage %+v ok=%v", u, ok)
	}
}

// A boundary that reports no count leaves the context unmeasured, never
// the full count of the turn before it.
func TestUsageAfterACompactWithoutACountIsNotTheOldOne(t *testing.T) {
	body := strings.Replace(claudeCompacted, `,"postTokens":9400`, ``, 1)
	path := filepath.Join(t.TempDir(), "s.jsonl")
	writeFile(t, path, body)
	if u, _ := lastUsage(path, int64(len(body))); u.Tokens != 0 {
		t.Fatalf("usage %+v", u)
	}
}

// The RESUME line is the model's text: no control key reaches the screen,
// and it stays one short line.
func TestResumeLineIsSafeToShow(t *testing.T) {
	got := ResumeLine("RESUME: go\x1b]52;c;bad\x07 on\x1b[2J " + strings.Repeat("x", 1000))
	if strings.ContainsFunc(got, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		t.Fatalf("control key in %q", got)
	}
	if n := len([]rune(got)); n > MaxResume {
		t.Fatalf("%d runes", n)
	}
}
