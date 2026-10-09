// Package state is the snapshot file a running console leaves behind so an
// agent can read the same facts without sampling again.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// Doc is everything the console knows at one moment.
type Doc struct {
	sample.Snapshot
	Git             *gitscan.Report `json:"git,omitempty"`
	Recommendations []advice.Rec    `json:"recommendations"`
	Queue           []queue.Item    `json:"queue"`
	// Doors are the setup steps of the machine; the console shows the open ones.
	Doors []doors.Door `json:"doors,omitempty"`
	// Stoker is the stoker mode and the steps of its last run. Nil: the
	// service keeps no stoker.
	Stoker *stoker.View `json:"stoker,omitempty"`
}

// Path is $XDG_STATE_HOME/matchblox/state.json, defaulting to ~/.local/state.
func Path() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "matchblox", "state.json")
}

// Write replaces the file atomically so a reader never sees half a snapshot.
func Write(path string, doc Doc) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".state-*.json")
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

func Read(path string) (Doc, error) {
	var doc Doc
	b, err := os.ReadFile(path)
	if err != nil {
		return doc, err
	}
	return doc, json.Unmarshal(b, &doc)
}
