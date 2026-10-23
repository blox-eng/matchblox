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

// seenDoc is CodexSeenFile: the sessions, and until when the sampler ran.
type seenDoc struct {
	Until time.Time              `json:"until"`
	Seen  map[string][]seenEntry `json:"seen"`
}

// loadCodexSeen reads the Codex sessions seen in each directory, and the
// last sample before matchblox stopped (zero: unknown). A file that is
// missing or unreadable gives none: a neighbour is then forgotten, as
// before the file.
func loadCodexSeen(path string) (seen map[string]map[procKey]time.Time, until time.Time) {
	seen = map[string]map[procKey]time.Time{}
	if path == "" {
		return seen, until
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return seen, until
	}
	var doc seenDoc
	if json.Unmarshal(b, &doc) != nil {
		return seen, until
	}
	for dir, entries := range doc.Seen {
		seen[dir] = map[procKey]time.Time{}
		for _, e := range entries {
			seen[dir][procKey{e.PID, e.Start}] = e.At
		}
	}
	return seen, doc.Until
}

// saveCodexSeen writes seen, and until (the last sample), to path through
// a temp file, so a crash leaves the old file whole.
func saveCodexSeen(path string, seen map[string]map[procKey]time.Time, until time.Time) error {
	doc := seenDoc{Until: until, Seen: map[string][]seenEntry{}}
	for dir, keys := range seen {
		for k, at := range keys {
			doc.Seen[dir] = append(doc.Seen[dir], seenEntry{k.pid, k.start, at})
		}
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// A temp file of its own: two samplers may save at once.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".codex-seen-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
