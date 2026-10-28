// Package recovery keeps the record of the agents on the builder's tmux
// server while they run: which conversation each pane holds, and where. When
// the server dies, the record is what matchblox recover reopens (#215).
package recovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// gitWait bounds the git calls that read one folder's checkout: a folder on
// a dead network mount must not hold the service's sample.
const gitWait = 2 * time.Second

// Entry is one agent on the builder's tmux server. It holds no transcript,
// prompt or environment value.
type Entry struct {
	Agent          string `json:"agent"`           // claude | codex | cursor-agent
	ConversationID string `json:"conversation_id"` // what the agent resumes by
	Session        string `json:"session"`         // the tmux session's name
	Pane           string `json:"pane"`
	Cwd            string `json:"cwd"`
	// Repo, Branch and Ident are the checkout of Cwd when last read: the
	// root of its worktree, its branch, and what names the repository
	// wherever its folder moves (Checkout.Ident). Empty: not a git folder.
	// Unread: git never answered for Cwd in time, so none is known.
	Repo   string `json:"repo,omitempty"`
	Branch string `json:"branch,omitempty"`
	Ident  string `json:"ident,omitempty"`
	Unread bool   `json:"unread,omitempty"`
	// AccountDir is the config folder of an agent not on the default
	// account, as sample.Session has it.
	AccountDir string `json:"account_dir,omitempty"`
	// Provider is the model's provider as sample.Session has it (bedrock,
	// vertex, ...), and APIKey that the agent signed in with an API key.
	// The shell gave both, and recover cannot give them again.
	Provider string `json:"provider,omitempty"`
	APIKey   bool   `json:"api_key,omitempty"`
	// Socket, ServerPID and ServerStart name the tmux server: a new server
	// on the same socket is another server, whose pane ids start again
	// from %0, even when it got the lost one's pid after a reboot.
	Socket      string    `json:"socket"`
	ServerPID   string    `json:"server_pid"`
	ServerStart string    `json:"server_start,omitempty"`
	Seen        time.Time `json:"seen"`
}

// Server is the identity of a tmux server, as Update's alive has it.
func Server(pid, start string) string { return pid + "@" + start }

func (e Entry) server() string { return Server(e.ServerPID, e.ServerStart) }

func (e Entry) key() string { return e.Socket + "\x00" + e.server() + "\x00" + e.Pane }

func (e Entry) conversation() string { return e.Agent + "\x00" + e.ConversationID }

// Checkout is a folder's checkout: see Entry. Ident is a hash of the place
// its origin names, or of its root commits when it has no origin: both
// move with a clone, and an unrelated repository has others. "" when git
// tells neither (no origin, no commit yet). Unread:
// git did not answer within gitWait, and the rest says nothing.
type Checkout struct {
	Root, Branch, Ident string
	Unread              bool
}

// Store keeps recover.json in a directory. An empty dir keeps it in memory.
type Store struct {
	// Git reads the checkout of a folder (nil: ReadCheckout).
	Git func(dir string) Checkout
	// Moved is where Open moved a record that did not parse ("": none).
	Moved string
	dir   string
	mu    sync.Mutex
	by    map[string]Entry
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
		s.Moved = bad
		return s, nil
	}
	for _, e := range list {
		s.by[e.key()] = e
	}
	return s, nil
}

// Update takes the agents of one sample, and alive: the identity (Server)
// of each tmux server the sample read, by its socket. A live pane's entry
// takes its conversation now, as after a /clear or a /resume. An entry not
// live leaves the record when its server still runs (the builder quit the
// agent), when its conversation runs in another pane (it was reopened), or
// after ExpireAfter. A lost server's entries stay. The checkouts are read
// without the lock, so List never waits on git.
func (s *Store) Update(live []Entry, alive map[string]string, now time.Time) error {
	s.mu.Lock()
	var read []int
	for i, l := range live {
		old, had := s.by[l.key()]
		if had && same(old, l) && now.Sub(old.Seen) < refresh {
			live[i] = old
			continue
		}
		read = append(read, i)
	}
	s.mu.Unlock()
	// In parallel: each read has gitWait, so a dead mount under many agents
	// holds the sample for gitWait, not for each of them.
	reads := make([]Checkout, len(read))
	var wg sync.WaitGroup
	for j, i := range read {
		wg.Go(func() { reads[j] = s.git(live[i].Cwd) })
	}
	wg.Wait()
	s.mu.Lock()
	for j, i := range read {
		c := reads[j]
		l := &live[i]
		l.Seen = now
		if c.Unread {
			// Keep what was read before: a slow git must not erase the
			// checkout recover checks.
			if old, had := s.by[l.key()]; had && old.Cwd == l.Cwd {
				l.Repo, l.Branch, l.Ident, l.Unread = old.Repo, old.Branch, old.Ident, old.Unread
			} else {
				l.Repo, l.Branch, l.Ident, l.Unread = "", "", "", true
			}
			continue
		}
		l.Repo, l.Branch, l.Ident, l.Unread = c.Root, c.Branch, c.Ident, false
	}
	s.mu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := len(read) > 0
	here := map[string]bool{}
	running := map[string]bool{}
	for _, l := range live {
		here[l.key()], running[l.conversation()] = true, true
		s.by[l.key()] = l
	}
	for k, e := range s.by {
		if here[k] {
			continue
		}
		if running[e.conversation()] || alive[e.Socket] == e.server() || now.Sub(e.Seen) >= ExpireAfter {
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
		a.Cwd == b.Cwd && a.AccountDir == b.AccountDir && a.Provider == b.Provider && a.APIKey == b.APIKey
}

func (s *Store) git(dir string) Checkout {
	if s.Git != nil {
		return s.Git(dir)
	}
	return ReadCheckout(dir)
}

// CheckoutOf is the checkout of dir, as Update reads it.
func (s *Store) CheckoutOf(dir string) Checkout { return s.git(dir) }

// ReadCheckout reads the checkout of dir with git, within gitWait: Unread
// when git did not answer in time. It only reads.
func ReadCheckout(dir string) Checkout {
	ctx, cancel := context.WithTimeout(context.Background(), gitWait)
	defer cancel()
	git := func(args ...string) (string, error) {
		c := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) //nolint:gosec // a folder of the builder's
		c.WaitDelay = time.Second
		out, err := c.Output()
		return strings.TrimSpace(string(out)), err
	}
	root, err := git("rev-parse", "--show-toplevel")
	if ctx.Err() != nil {
		return Checkout{Unread: true}
	}
	if err != nil {
		return Checkout{} // not a git folder
	}
	branch, _ := git("symbolic-ref", "--short", "-q", "HEAD")
	ident := ""
	if url, err := git("config", "--get", "remote.origin.url"); err == nil && url != "" {
		ident = hashed("origin " + originPlace(url))
	} else if roots, err := git("rev-list", "--max-parents=0", "HEAD"); err == nil && roots != "" {
		list := strings.Fields(roots)
		sort.Strings(list)
		ident = hashed("roots " + strings.Join(list, " "))
	}
	if ctx.Err() != nil {
		return Checkout{Unread: true}
	}
	return Checkout{Root: root, Branch: branch, Ident: ident}
}

// hashed is a short hash of what names a repository: the record keeps no
// origin URL, which can hold a token.
func hashed(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// originPlace is the host and path an origin URL names, without its
// scheme, user, password, port or .git: a clone over ssh and one over
// https of one repository name the same place.
func originPlace(raw string) string {
	s := raw
	if _, rest, ok := strings.Cut(s, "://"); ok {
		s = rest
	} else if host, path, ok := strings.Cut(s, ":"); ok && !strings.Contains(host, "/") {
		s = host + "/" + path // scp form: git@host:o/r
	}
	if i := strings.Index(s, "/"); i >= 0 {
		host, path := s[:i], s[i:]
		if at := strings.LastIndex(host, "@"); at >= 0 {
			host = host[at+1:]
		}
		if h, _, ok := strings.Cut(host, ":"); ok {
			host = h
		}
		s = strings.ToLower(host) + path
	}
	return strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git")
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
	if err := tmp.Sync(); err != nil { // a power loss must not leave an empty record
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
