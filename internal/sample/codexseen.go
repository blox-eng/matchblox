package sample

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// seenEntry is one Codex session in CodexSeenFile: its process and when
// it started.
type seenEntry struct {
	PID   int       `json:"pid"`
	Start uint64    `json:"start"`
	At    time.Time `json:"at"`
}

// loadCodexSeen reads the Codex sessions seen in each directory. A file
// that is missing or unreadable gives none: a neighbour is then forgotten,
// as before the file.
func loadCodexSeen(path string) map[string]map[procKey]time.Time {
	seen := map[string]map[procKey]time.Time{}
	if path == "" {
		return seen
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return seen
	}
	var doc map[string][]seenEntry
	if json.Unmarshal(b, &doc) != nil {
		return seen
	}
	for dir, entries := range doc {
		seen[dir] = map[procKey]time.Time{}
		for _, e := range entries {
			seen[dir][procKey{e.PID, e.Start}] = e.At
		}
	}
	return seen
}

// saveCodexSeen writes seen to path through a temp file, so a crash
// leaves the old file whole.
func saveCodexSeen(path string, seen map[string]map[procKey]time.Time) error {
	doc := map[string][]seenEntry{}
	for dir, keys := range seen {
		for k, at := range keys {
			doc[dir] = append(doc[dir], seenEntry{k.pid, k.start, at})
		}
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
