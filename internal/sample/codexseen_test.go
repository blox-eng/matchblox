package sample

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestCodexSeenSavesConcurrently: two samplers (the service and a one-shot
// status read) may save at once; the file must always parse.
func TestCodexSeenSavesConcurrently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codex-seen.json")
	small := map[string]map[procKey]time.Time{"/a": {{1, 1}: time.Unix(1, 0)}}
	big := map[string]map[procKey]time.Time{}
	for i := range 50 {
		big["/work/repo-with-a-long-name-"+string(rune('a'+i%26))] = map[procKey]time.Time{{i, uint64(i)}: time.Unix(int64(i), 0)}
	}
	for range 300 {
		var wg sync.WaitGroup
		for _, m := range []map[string]map[procKey]time.Time{small, big} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = saveCodexSeen(path, m, time.Unix(9, 0))
			}()
		}
		wg.Wait()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !json.Valid(b) {
			t.Fatalf("corrupt file: %q", b)
		}
	}
}
