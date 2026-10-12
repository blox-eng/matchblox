package ships

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func appendFile(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func size(t *testing.T, path string) int64 {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return st.Size()
}

// TestReadIncrementally: each read takes only the complete lines written
// since the last, a call waits for its result across reads, and a
// rewritten file is read again.
func TestReadIncrementally(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	appendFile(t, path, assistantText("hello")+"\n"+toolUse("t1", "/w", "", "gh pr create --fill")+"\n")
	var tr transcript
	got, err := read(path, &tr)
	if err != nil || len(got) != 0 || tr.Offset != size(t, path) || len(tr.Pending) != 1 {
		t.Fatalf("first read: %v %v offset %d pending %v", got, err, tr.Offset, tr.Pending)
	}

	result := toolResult("t1", "/w", "https://github.com/o/r/pull/5", nil)
	appendFile(t, path, result[:40]) // the agent is still writing the line
	before := tr.Offset
	if got, _ := read(path, &tr); len(got) != 0 || tr.Offset != before {
		t.Fatalf("a partial line was read: %v offset %d", got, tr.Offset)
	}
	appendFile(t, path, result[40:]+"\n")
	got, _ = read(path, &tr)
	if len(got) != 1 || got[0].Kind != Created || got[0].Number != 5 || tr.Pending != nil {
		t.Fatalf("the result: %+v pending %v", got, tr.Pending)
	}
	if got, _ := read(path, &tr); got != nil {
		t.Fatalf("nothing new, read again: %v", got)
	}

	// /clear can rewrite the file shorter: it is read from the start.
	os.WriteFile(path, []byte(toolUse("t2", "/w", "", "gh pr view 9")+"\n"), 0o600)
	got, _ = read(path, &tr)
	if len(got) != 1 || got[0].Number != 9 || tr.Offset != size(t, path) {
		t.Fatalf("rewritten: %+v offset %d", got, tr.Offset)
	}
}

// TestReadSkipsHugeLines: a line over maxLine is a tool's output; it is
// skipped whole and the lines after it are read.
func TestReadSkipsHugeLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	huge := `{"type":"user","x":"gh pr view 1 ` + strings.Repeat("a", maxLine) + `"}`
	appendFile(t, path, huge+"\n"+toolUse("t1", "/w", "", "gh pr view 2")+"\n")
	var tr transcript
	got, err := read(path, &tr)
	if err != nil || len(got) != 1 || got[0].Number != 2 || tr.Offset != size(t, path) {
		t.Fatalf("got %+v %v offset %d", got, err, tr.Offset)
	}
}
