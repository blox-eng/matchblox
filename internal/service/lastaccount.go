package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/blox-eng/matchblox/internal/accounts"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// lastAccountsFile is where the last account of each agent is kept, in the
// state directory.
const lastAccountsFile = "last-accounts.json"

// lastAccountState is the account the builder last started, per agent, and
// the agent they last started, so the new-session form can open on them.
// It is read from the state directory when first needed: a missing or
// unreadable file means none.
type lastAccountState struct {
	mu     sync.Mutex
	loaded bool
	from   string // the path the labels were read from
	// dirty: the file is not in step with label and agent, for a write failed.
	dirty bool
	label map[string]string
	agent string
	// pending: a new account started, per agent, that is the last one once a
	// session on its folder is signed in. Kept in memory only.
	pending map[string]pendingAccount
}

// pendingAccount is a new account that is not signed in yet: its label and
// its folder.
type pendingAccount struct{ label, folder string }

// lastFile is the file's form. A file from before LastAgent is the bare map
// of labels.
type lastFile struct {
	Accounts  map[string]string `json:"accounts,omitempty"`
	LastAgent string            `json:"last_agent,omitempty"`
}

// lastAccountsPath is the file in the state directory of statePath. Empty: none kept.
func lastAccountsPath(statePath string) string {
	if statePath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(statePath), lastAccountsFile)
}

func (r *lastAccountState) loadLocked(path string) {
	if r.loaded && r.from == path {
		return
	}
	r.loaded, r.from, r.dirty = true, path, false
	r.label, r.agent = map[string]string{}, ""
	if path == "" {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var f lastFile
	if json.Unmarshal(b, &f) == nil && (f.Accounts != nil || f.LastAgent != "") {
		if f.Accounts != nil {
			r.label = f.Accounts
		}
		r.agent = f.LastAgent
		return
	}
	var m map[string]string
	if json.Unmarshal(b, &m) != nil || m == nil { // corrupt, or null: none
		return
	}
	r.label = m
}

// set remembers label as the account agent last started, and writes the
// file. A file that cannot be written is not an error: the form just opens
// on the default after a restart.
func (r *lastAccountState) set(path, agent, label string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loadLocked(path)
	delete(r.pending, agent) // a later start is the choice
	if r.label[agent] == label && !r.dirty {
		return
	}
	r.label[agent] = label
	r.saveLocked(path)
}

// await keeps label, the new account of agent on folder, until a session on
// folder is signed in. The label remembered before it stays until then.
func (r *lastAccountState) await(agent, label, folder string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending == nil {
		r.pending = map[string]pendingAccount{}
	}
	r.pending[agent] = pendingAccount{label, filepath.Clean(folder)}
}

// signedIn remembers each pending account that a session of its agent on
// its folder is signed in on: it names its account. The folders compare
// with their symlinks resolved.
func (r *lastAccountState) signedIn(path string, sessions []sample.Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for agent, p := range r.pending {
		for _, x := range sessions {
			if x.Agent != agent || x.AccountDir == "" || x.Account == "" || !accounts.SameDir(x.AccountDir, p.folder) {
				continue
			}
			r.loadLocked(path)
			delete(r.pending, agent)
			if r.label[agent] != p.label || r.dirty {
				r.label[agent] = p.label
				r.saveLocked(path)
			}
			break
		}
	}
}

// setAgent remembers agent as the one last started, and writes the file.
func (r *lastAccountState) setAgent(path, agent string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loadLocked(path)
	if r.agent == agent && !r.dirty {
		return
	}
	r.agent = agent
	r.saveLocked(path)
}

// of is the label agent last started on; empty: none.
func (r *lastAccountState) of(path, agent string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loadLocked(path)
	return r.label[agent]
}

// lastAgent is the agent last started; empty: none.
func (r *lastAccountState) lastAgent(path string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loadLocked(path)
	return r.agent
}

// saveLocked writes the file (0600, atomically).
func (r *lastAccountState) saveLocked(path string) {
	if path == "" {
		return
	}
	r.dirty = true // until the file is written
	b, err := json.Marshal(lastFile{Accounts: r.label, LastAgent: r.agent})
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0o700) != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), lastAccountsFile+".*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b) // CreateTemp makes the file 0600
	if cerr := tmp.Close(); werr != nil || cerr != nil || os.Rename(tmp.Name(), path) != nil {
		_ = os.Remove(tmp.Name())
		return
	}
	r.dirty = false
}

// listed are the remembered labels that are still in known, per agent. Nil:
// none.
func (r *lastAccountState) listed(path string, known map[string][]string) map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loadLocked(path)
	var out map[string]string
	for agent, label := range r.label {
		if !slices.Contains(known[agent], label) {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[agent] = label
	}
	return out
}

// rememberStart notes the agent a start that ran in full was on, and its
// account as the console lists it: default, a folder name, or a hand-made
// folder's label. A new account is noted once a session on it is signed in.
func (s *Service) rememberStart(n proto.NewSession, acct *onAccount) {
	agent := n.Agent
	s.lastAccounts.setAgent(lastAccountsPath(s.StatePath), agent)
	// A start that names no account does not change the choice.
	if n.Account == "" || !slices.Contains(accountAgents, agent) {
		return
	}
	label := "default"
	if acct != nil {
		label = acct.name
		if acct.hand {
			label = s.folderLabel(acct.folder)
		}
	}
	if acct != nil && acct.fresh {
		// Not signed in yet: the account before it stays the choice.
		s.lastAccounts.await(agent, label, acct.folder)
		return
	}
	s.lastAccounts.set(lastAccountsPath(s.StatePath), agent, label)
}
