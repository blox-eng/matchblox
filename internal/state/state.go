// Package state is the snapshot file a running console leaves behind so an
// agent can read the same facts without sampling again.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/holds"
	"github.com/blox-eng/matchblox/internal/idle"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/ships"
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
	// Ships are the pull requests sessions drive. Nil: the service reads
	// none (no git, or a fixture).
	Ships *ships.Report `json:"ships,omitempty"`
	// Holds are the sessions nothing types into: parked, or held by the stoker.
	Holds []holds.Hold `json:"holds,omitempty"`
	// Reasons say why each quiet session is idle, by its pane.
	Reasons map[string]idle.Reason `json:"reasons,omitempty"`
	// StokerServer is the tmux server (-L) matchblox keeps the stoker
	// agent on: a session whose pane is on it is the stoker's. Empty: none.
	StokerServer string `json:"stoker_server,omitempty"`
	// Agents are the agents a new session can start on this host: the
	// installed ones of claude, codex, opencode and cursor-agent.
	Agents []string `json:"agents,omitempty"`
	// Accounts are the accounts each agent can start on: "default" first, then
	// the folders under matchblox's root, then the hand-made folders live
	// sessions run on (by path, "~/" for one under home). Agents without
	// accounts have no entry.
	Accounts map[string][]string `json:"accounts,omitempty"`
	// LastAccount is, per agent, the account the builder last started, as
	// listed in Accounts ("default" included): the new-session form opens on
	// it. An account no longer listed is not sent.
	LastAccount map[string]string `json:"last_account,omitempty"`
	// LastAgent is the agent the builder last started a session on, from a
	// console or the CLI: the new-session form opens on it. Sent only when it
	// is in Agents.
	LastAgent string `json:"last_agent,omitempty"`
	// Renames are the names sessions take at their next rest, by pane.
	Renames map[string]string `json:"renames,omitempty"`
	// Focus is the session a tap on the island or a buzz opened on a phone,
	// for a short while after: each console acts on it once, by its At.
	// Nil: none.
	Focus *Focus `json:"focus,omitempty"`
	// Push is the push sender's one line for the Machine tab: `push: <why>`
	// after a failed send, or the island's renewal. Empty: all is well, or
	// no sender ([push] off).
	Push string `json:"push,omitempty"`
	// SignIns say, by pane (or by tmux session name when its pane was not
	// found), how the sign-in of a new account goes: "by hand" when
	// matchblox gave up driving it.
	SignIns map[string]string `json:"sign_ins,omitempty"`
}

// Focus is the session to open: its pane, or Gone when its number's
// session ended.
type Focus struct {
	SessionID string    `json:"session_id,omitempty"`
	Pane      string    `json:"pane,omitempty"`
	Client    string    `json:"client,omitempty"`
	Gone      bool      `json:"gone,omitempty"`
	At        time.Time `json:"at"`
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
func Write(path string, doc Doc) error { return writeJSON(path, doc) }

// writeJSON replaces path atomically with v, readable by its owner only.
func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
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
