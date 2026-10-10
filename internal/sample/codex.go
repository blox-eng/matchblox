package sample

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/limits"
)

// codexRollout finds the rollout a Codex process writes: Codex opens
// <CODEX_HOME>/sessions/YYYY/MM/DD/rollout-*.jsonl for append at its first
// turn and holds it open. An npm install runs the native binary as a child,
// so the children are searched too. read is false when the open files of
// the process cannot be read; read with no path: the session is fresh.
func (s *Sampler) codexRollout(pid int, children map[int][]int) (path string, read bool) {
	stack := []int{pid}
	for n := 0; len(stack) > 0 && n < 64; n++ {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		files, ok := s.FS.OpenFiles(cur)
		read = read || ok
		for _, f := range files {
			base := filepath.Base(f)
			if !strings.HasPrefix(base, "rollout-") || !strings.HasSuffix(base, ".jsonl") || !strings.Contains(f, "/sessions/") {
				continue
			}
			// More than one open: the one written last is on screen.
			if path == "" || newer(f, path) {
				path = f
			}
		}
		stack = append(stack, children[cur]...)
	}
	return path, read
}

func newer(a, b string) bool {
	ka, okA := statKey(a)
	kb, okB := statKey(b)
	return okA && (!okB || ka.mtime.After(kb.mtime))
}

// codexFound says what the end of a rollout told.
type codexFound int

const (
	codexNone      codexFound = iota // the whole file has no usage yet: the first turn runs
	codexNotInTail                   // the last tailBytes hold no usage: not measured
	codexMeasured
)

type codexLine struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Payload   struct {
		Type       string `json:"type"`
		Model      string `json:"model"`
		RateLimits *struct {
			Primary   *codexWindow `json:"primary"`
			Secondary *codexWindow `json:"secondary"`
		} `json:"rate_limits"`
		Info *struct {
			Last struct {
				Total int `json:"total_tokens"`
			} `json:"last_token_usage"`
			Window int `json:"model_context_window"` // null for a model Codex has no window for
		} `json:"info"`
	} `json:"payload"`
}

type codexWindow struct {
	Used     float64 `json:"used_percent"`
	Minutes  int     `json:"window_minutes"`
	ResetsAt int64   `json:"resets_at"`
}

// codexUsage reads the end of a rollout: the newest token_count with info
// gives the tokens of the last turn (what Codex counts as in the context
// window) and the window; the newest turn_context gives the model; the
// newest rate_limits gives the plan's limits (nil: none in the tail).
func (r *agentReader) codexUsage(path string) (Usage, *limits.Reading, codexFound) {
	key, ok := statKey(path)
	if !ok {
		return Usage{}, nil, codexNone
	}
	if c, ok := r.codex[path]; ok && c.key == key {
		return c.val.u, c.val.lim, c.val.found
	}
	f, err := os.Open(path)
	if err != nil {
		return Usage{}, nil, codexNone
	}
	defer f.Close()
	off := max(key.size-tailBytes, 0)
	buf := make([]byte, key.size-off)
	if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
		return Usage{}, nil, codexNone
	}
	var u Usage
	// lim is the newest reading with a window; empty, the newest without
	// one (Codex also writes limits of other ids, all null).
	var lim, empty *limits.Reading
	found := false
	lines := bytes.Split(buf, []byte{'\n'})
	for i := len(lines) - 1; i >= 0 && (!found || u.Model == "" || lim == nil); i-- {
		l := lines[i]
		needUsage := !found && bytes.Contains(l, []byte(`"token_count"`)) && bytes.Contains(l, []byte(`"last_token_usage"`))
		needModel := u.Model == "" && bytes.Contains(l, []byte(`"turn_context"`))
		needLimits := lim == nil && bytes.Contains(l, []byte(`"rate_limits"`))
		if !needUsage && !needModel && !needLimits {
			continue
		}
		var c codexLine
		if json.Unmarshal(l, &c) != nil {
			continue
		}
		if needUsage && c.Payload.Type == "token_count" && c.Payload.Info != nil {
			u.Tokens, u.Window, found = c.Payload.Info.Last.Total, c.Payload.Info.Window, true
		}
		if needModel && c.Type == "turn_context" {
			u.Model = c.Payload.Model
		}
		if needLimits && c.Payload.RateLimits != nil {
			r := &limits.Reading{At: c.Timestamp}
			for _, w := range []*codexWindow{c.Payload.RateLimits.Primary, c.Payload.RateLimits.Secondary} {
				if w != nil && w.Minutes > 0 {
					r.Windows = append(r.Windows, limits.Window{UsedPct: w.Used, Minutes: w.Minutes, ResetsAt: limits.Unix(w.ResetsAt)})
				}
			}
			switch {
			case len(r.Windows) > 0:
				lim = r
			case empty == nil:
				empty = r
			}
		}
	}
	if lim == nil {
		lim = empty
	}
	res := codexMeasured
	switch {
	case !found && off > 0:
		res = codexNotInTail
	case !found:
		res = codexNone
	}
	r.codex[path] = cached[codexRead]{key, codexRead{u, lim, res}}
	return u, lim, res
}

type codexRead struct {
	u     Usage
	lim   *limits.Reading
	found codexFound
}
