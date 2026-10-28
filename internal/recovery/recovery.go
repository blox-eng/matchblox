// Package recovery keeps the record of the agents on the builder's tmux
// server while they run: which conversation each pane holds, and where. When
// the server dies, the record is what matchblox recover reopens (#215).
package recovery

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// File is the record's name in matchblox's state folder.
const File = "recover.json"

// ExpireAfter is how long the entry of a lost server stays in the record.
const ExpireAfter = 14 * 24 * time.Hour

// refresh is how often a live entry's checkout and Seen are read again.
const refresh = time.Minute

// Entry is one agent on the builder's tmux server. It holds no transcript,
// prompt or environment value.
type Entry struct {
	Agent          string `json:"agent"`           // claude | codex | cursor-agent
	ConversationID string `json:"conversation_id"` // what the agent resumes by
	Session        string `json:"session"`         // the tmux session's name
	Pane           string `json:"pane"`
	Cwd            string `json:"cwd"`
	// Repo and Branch are the checkout of Cwd when last seen: the root of
	// its worktree and its branch. Empty: not a git folder.
	Repo   string `json:"repo,omitempty"`
	Branch string `json:"branch,omitempty"`
	// AccountDir is the config folder of an agent not on the default
	// account, as sample.Session has it.
	AccountDir string `json:"account_dir,omitempty"`
	// Socket and ServerPID name the tmux server: a new server on the same
	// socket is another server, whose pane ids start again from %0.
	Socket    string    `json:"socket"`
	ServerPID string    `json:"server_pid"`
	Seen      time.Time `json:"seen"`
}

func (e Entry) key() string { return e.Socket + "\x00" + e.ServerPID + "\x00" + e.Pane }

func (e Entry) conversation() string { return e.Agent + "\x00" + e.ConversationID }

// Store keeps recover.json in a directory. An empty dir keeps it in memory.
type Store struct {
	// Git is the root and branch of the checkout of a folder (nil: git).
	Git func(dir string) (root, branch string)
	dir string
	mu  sync.Mutex
	by  map[string]Entry
}

func (s *Store) path() string { return filepath.Join(s.dir, File) }

// Open reads the record. A missing file is an empty record; one that does
// not parse is moved aside, so a broken file never stops the service.
func Open(dir string) (*Store, error) {
	s := &Store{dir: dir, by: map[string]Entry{}}
	if dir == "" {
		return s, nil
	}
	b, err := os.ReadFile(s.path()) //nolint:gosec // our own state dir
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var list []Entry
	if err := json.Unmarshal(b, &list); err != nil {
		bad := s.path() + ".bad-" + time.Now().UTC().Format("20060102T150405")
		if err := os.Rename(s.path(), bad); err != nil {
			return nil, err
		}
		return s, nil
	}
	for _, e := range list {
		s.by[e.key()] = e
	}
	return s, nil
}

// Update takes the agents of one sample, and alive: the pid of each tmux
// server the sample read, by its socket. A live pane's entry takes its
// conversation now, as after a /clear or a /resume. An entry not live
// leaves the record when its server still runs (the builder quit the
// agent), when its conversation runs in another pane (it was reopened), or
// after ExpireAfter. A lost server's entries stay.
func (s *Store) Update(live []Entry, alive map[string]string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	here := map[string]bool{}
	running := map[string]bool{}
	for _, l := range live {
		k := l.key()
		here[k], running[l.conversation()] = true, true
		old, had := s.by[k]
		if had && same(old, l) && now.Sub(old.Seen) < refresh {
			continue
		}
		l.Repo, l.Branch = s.git(l.Cwd)
		l.Seen = now
		s.by[k], changed = l, true
	}
	for k, e := range s.by {
		if here[k] {
			continue
		}
		if running[e.conversation()] || alive[e.Socket] == e.ServerPID || now.Sub(e.Seen) >= ExpireAfter {
			delete(s.by, k)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.save()
}

// same: b is the agent of a, in the same place.
func same(a, b Entry) bool {
	return a.Agent == b.Agent && a.ConversationID == b.ConversationID && a.Session == b.Session &&
		a.Cwd == b.Cwd && a.AccountDir == b.AccountDir
}

func (s *Store) git(dir string) (string, string) {
	if s.Git != nil {
		return s.Git(dir)
	}
	return Checkout(dir)
}

// Checkout is the root of the worktree dir is in, and its branch: "" for
// each that git does not tell. It only reads.
func Checkout(dir string) (root, branch string) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output() //nolint:gosec // a folder of the builder's
	if err != nil {
		return "", ""
	}
	root = strings.TrimSpace(string(out))
	out, err = exec.Command("git", "-C", dir, "symbolic-ref", "--short", "-q", "HEAD").Output() //nolint:gosec // as above
	if err == nil {
		branch = strings.TrimSpace(string(out))
	}
	return root, branch
}

// List is every entry, by tmux session and conversation.
func (s *Store) List() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0, len(s.by))
	for _, e := range s.by {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Session != out[j].Session {
			return out[i].Session < out[j].Session
		}
		return out[i].ConversationID < out[j].ConversationID
	})
	return out
}

// save writes the record atomically, readable by its owner only. Callers
// hold mu.
func (s *Store) save() error {
	if s.dir == "" {
		return nil
	}
	list := make([]Entry, 0, len(s.by))
	for _, e := range s.by {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].key() < list[j].key() })
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, ".recover-*.json")
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
	return os.Rename(tmp.Name(), s.path())
}
