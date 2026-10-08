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
		{"hashes and dashes", "build [#####-----] 50%", 50, "", true},
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
