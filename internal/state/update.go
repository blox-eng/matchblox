package state

import (
	"path/filepath"
	"time"
)

// UpdateFile is what the service keeps of the releases across restarts:
// the latest one found and when this machine first saw it, the release
// whose offer a person closed, and the last install.
type UpdateFile struct {
	Latest    string        `json:"latest,omitempty"`
	FirstSeen time.Time     `json:"first_seen,omitzero"`
	Closed    string        `json:"closed,omitempty"`
	Last      *UpdateRecord `json:"last,omitempty"`
}

// UpdatePath holds the UpdateFile, next to state.json.
func UpdatePath() string { return filepath.Join(filepath.Dir(Path()), "update.json") }

// ReadUpdate is empty for a missing or broken file.
func ReadUpdate(path string) UpdateFile {
	var f UpdateFile
	if readJSON(path, &f) != nil {
		return UpdateFile{}
	}
	return f
}

func WriteUpdate(path string, f UpdateFile) error { return writeJSON(path, f) }
