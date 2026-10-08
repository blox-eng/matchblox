package sample

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProgress(t *testing.T) {
	for _, c := range []struct {
		name, text string
		pct        int
		step       string
		ok         bool
	}{
		{"the asked line", "Done with the hook.\nProgress [████░░░░] wiring the console", 50, "wiring the console", true},
		{"a free-form block", strings.Join([]string{
			"   1-9            ██████ done",
			"   11-lite        ██░░░░  running (Fable): manual pass",
			"   review 9-11    ░░░░░░",
			"  Progress  [███████████████░░░░░]",
		}, "\n"), 75, "11-lite running (Fable): manual pass", true},
		{"hashes and dashes", "Progress [#####-----] 50%", 50, "", true},
		{"the last bar wins", "Progress [██░░] a\nProgress [████] b", 100, "b", true},
		{"no bar", "all done, nothing to report", 0, "", false},
		{"a bar of one kind only is not a bar", "[----]", 0, "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, ok := parseProgress(c.text)
			if ok != c.ok || p.Pct != c.pct || p.Step != c.step {
				t.Fatalf("got %+v %v, want %d %q %v", p, ok, c.pct, c.step, c.ok)
			}
		})
	}
}

// TestProgressFromTheLastReply: the last reply with text carries the
// progress; a tool call after it does not hide it.
func TestProgressFromTheLastReply(t *testing.T) {
	lines := []string{
		`{"type":"assistant","timestamp":"2026-10-08T10:00:00Z","message":{"model":"m","content":[{"type":"text","text":"Progress [██░░░░░░] old"}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
		`{"type":"user","message":{"content":"go on"}}`,
		`{"type":"assistant","timestamp":"2026-10-08T10:01:00Z","message":{"model":"m","content":[{"type":"text","text":"Progress [██████░░] wiring"}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
		`{"type":"assistant","timestamp":"2026-10-08T10:02:00Z","message":{"model":"m","content":[{"type":"tool_use","name":"Bash","input":{}}],"usage":{"input_tokens":5,"output_tokens":1}}}`,
	}
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	u, ok := lastUsage(path, st.Size())
	if !ok || u.Tokens != 6 {
		t.Fatalf("usage %+v %v", u, ok)
	}
	if u.Progress == nil || u.Progress.Pct != 75 || u.Progress.Step != "wiring" {
		t.Fatalf("progress %+v", u.Progress)
	}
}

// Review 2, #3 and #8: a bar needs the word Progress before it, and the
// step loses control keys: transcript text is not trusted.
func TestProgressIsAskedForAndClean(t *testing.T) {
	for _, text := range []string{"[#]", "pip [=====]", "regexp `[█▓▰■#=]`", "build [#####-----] 50%"} {
		if p, ok := parseProgress(text); ok {
			t.Errorf("%q read as %+v", text, p)
		}
	}
	p, ok := parseProgress("Progress [██░░] a\x1b]52;c;eHg=\x07b")
	if !ok || strings.ContainsAny(p.Step, "\x1b\x07") || p.Step != "a]52;c;eHg=b" {
		t.Fatalf("step %q", p.Step)
	}
}

// Review 2, #7: the newest reply with text decides, also when the lines
// carry no usage.
func TestTheNewestReplyDecides(t *testing.T) {
	lines := []string{
		`{"type":"assistant","message":{"model":"m","content":[{"type":"text","text":"Progress [██░░] old"}]}}`,
		`{"type":"assistant","message":{"model":"m","content":[{"type":"text","text":"no bar this time"}]}}`,
	}
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	if u, _ := lastUsage(path, st.Size()); u.Progress != nil {
		t.Fatalf("an older bar won: %+v", u.Progress)
	}
}

// TestTheNewestReplyTellsIfItAsks: without hooks, a Claude Code turn that
// ended on a question waits for the person.
func TestTheNewestReplyTellsIfItAsks(t *testing.T) {
	lines := []string{
		`{"type":"assistant","message":{"model":"m","content":[{"type":"text","text":"Fixed.\n\nShall I open the PR?\n\nProgress [██████░░] review"}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
	}
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	if u, _ := lastUsage(path, st.Size()); !u.Asks || u.LastLine != "Shall I open the PR?" {
		t.Fatalf("usage %+v", u)
	}
}
