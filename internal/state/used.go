package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Use is one place a session started, and when.
type Use struct {
	Path string    `json:"path"`
	At   time.Time `json:"at"`
}

// Used is where matchblox started sessions, the newest first.
type Used struct {
	Repos []Use `json:"repos,omitempty"`
}

// MaxUsed is how many places the record keeps.
const MaxUsed = 200

// Add is u with paths used at at: each cleaned, moved to the front, once.
func (u Used) Add(at time.Time, paths ...string) Used {
	front := make([]Use, 0, len(paths))
	seen := map[string]bool{}
	for _, p := range paths {
		p = filepath.Clean(p)
		if !seen[p] {
			seen[p] = true
			front = append(front, Use{Path: p, At: at})
		}
	}
	out := front
	for _, r := range u.Repos {
		if !seen[r.Path] {
			out = append(out, r)
		}
	}
	return Used{Repos: slices.Clip(out[:min(len(out), MaxUsed)])}
}

// UsedPath holds the Used, next to state.json.
func UsedPath() string { return filepath.Join(filepath.Dir(Path()), "used.json") }

// ReadUsed is empty for a missing or broken file.
func ReadUsed(path string) Used {
	var u Used
	if readJSON(path, &u) != nil {
		return Used{}
	}
	return u
}

func WriteUsed(path string, u Used) error { return writeJSON(path, u) }

// KeepBrokenUsed moves a Used file that is there but does not read to
// path.broken, over an older one, so the next write does not lose it.
func KeepBrokenUsed(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil // missing, or not to be read: a write says why
	}
	var u Used
	if json.Unmarshal(b, &u) == nil {
		return nil
	}
	return os.Rename(path, path+".broken")
}
