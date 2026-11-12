// Package state is the snapshot file a running console leaves behind so an
// agent can read the same facts without sampling again.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/blox-eng/matchblox/internal/accounts"
	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/chains"
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
	// Stokers are the chain stokers, by id, an ended one too: its card is
	// read in the morning. Nil: the family has no chain stoker.
	Stokers []ChainStoker `json:"stokers,omitempty"`
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
	// AccountsSkipped are, per agent, the folders under matchblox's root that
	// Accounts does not list, each with why: a name that fails the rule, or
	// the root itself when it cannot be read. The new-session form says so.
	AccountsSkipped map[string][]accounts.Skip `json:"accounts_skipped,omitempty"`
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
	// Chains are the teams of this host and their members. Nil: none.
	Chains *ChainsView `json:"chains,omitempty"`
	// Update is what the service knows of a newer release. A console reads
	// its presence as "this host knows the update act". Nil: a build that
	// is not a release, or a service older than the update.
	Update *UpdateView `json:"update,omitempty"`
}

// UpdateView is the [update] mode, this build's release and the latest one
// found. Err is why the last look failed; Latest stays the last one found.
// Managed is the command of the package manager that installed matchblox.
type UpdateView struct {
	Mode      string        `json:"mode"`
	Current   string        `json:"current"`
	Latest    string        `json:"latest,omitempty"`
	CheckedAt time.Time     `json:"checked_at,omitzero"`
	FirstSeen time.Time     `json:"first_seen,omitzero"`
	Err       string        `json:"err,omitempty"`
	Managed   string        `json:"managed,omitempty"`
	Last      *UpdateRecord `json:"last,omitempty"`
	// Waiting is why mode auto waits to install a release that is due.
	Waiting string `json:"waiting,omitempty"`
	// Restart: the last install put the latest release on disk, but this
	// service started on the old one. It says so, with the fix.
	Restart string `json:"restart,omitempty"`
}

// UpdateRecord is the last install: from which release to which, what it
// checked, and why it failed.
type UpdateRecord struct {
	From       string    `json:"from,omitempty"`
	To         string    `json:"to,omitempty"`
	At         time.Time `json:"at,omitzero"`
	Checksum   bool      `json:"checksum,omitempty"`
	Provenance bool      `json:"provenance,omitempty"`
	Err        string    `json:"err,omitempty"`
}

// ChainsView is what consoles and the CLI see of the chains.
type ChainsView struct {
	Chains []chains.Chain `json:"chains,omitempty"`
	// Mates is each member of any team once: its inbox and when it was
	// first missed while its pane lives.
	Mates []MateView `json:"mates,omitempty"`
}

// MateView is what the room shows of one member: Inbox items wait, Full
// when some were dropped, and Unseen when its pane lives but shows no agent
// (zero: seen).
type MateView struct {
	Member chains.Member `json:"member"`
	Inbox  int           `json:"inbox,omitempty"`
	Full   bool          `json:"full,omitempty"`
	Unseen time.Time     `json:"unseen,omitzero"`
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

// ChainStoker is the view of one chain's stoker, with the chain's id and
// name.
type ChainStoker struct {
	ID    string      `json:"id"`
	Chain string      `json:"chain"`
	Name  string      `json:"name"`
	View  stoker.View `json:"view"`
}
