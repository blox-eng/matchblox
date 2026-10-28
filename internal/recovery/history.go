package recovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// History is the saved history of the agent's conversation id in its
// config folder: a Claude Code transcript or a Codex rollout that is not
// empty. checked is false for an agent whose history matchblox does not
// read (Cursor): then path says nothing.
func History(agent, configDir, id string) (path string, checked bool) {
	var pattern string
	switch agent {
	case "claude":
		pattern = filepath.Join(configDir, "projects", "*", id+".jsonl")
	case "codex":
		pattern = filepath.Join(configDir, "sessions", "*", "*", "*", "rollout-*-"+id+".jsonl")
	default:
		return "", false
	}
	found, _ := filepath.Glob(pattern)
	for _, p := range found {
		if st, err := os.Stat(p); err == nil && st.Size() > 0 {
			return p, true
		}
	}
	return "", true
}

// OpenIn is the pid of a running Claude Code that has the conversation id
// open, as its registry in the config folder says: 0 when none, and for
// other agents, whose lock matchblox does not read.
func OpenIn(agent, configDir, id string) int {
	if agent != "claude" {
		return 0
	}
	files, _ := filepath.Glob(filepath.Join(configDir, "sessions", "*.json"))
	for _, f := range files {
		pid, err := strconv.Atoi(strings.TrimSuffix(filepath.Base(f), ".json"))
		if err != nil || pid <= 0 {
			continue
		}
		b, err := os.ReadFile(f) //nolint:gosec // the agent's own registry
		if err != nil {
			continue
		}
		var e struct {
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(b, &e) != nil || e.SessionID != id {
			continue
		}
		if p, err := os.FindProcess(pid); err == nil && p.Signal(syscall.Signal(0)) == nil {
			return pid
		}
	}
	return 0
}

// CheckoutOf is the root and branch of dir's checkout, as Update reads them.
func (s *Store) CheckoutOf(dir string) (root, branch string) { return s.git(dir) }
