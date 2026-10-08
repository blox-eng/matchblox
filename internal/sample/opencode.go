package sample

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// OpenCode 1.1 keeps each session and each message as a JSON file under
// <XDG_DATA_HOME>/opencode/storage. A process does not name its session, so
// the session is the newest root session in the process's directory that
// was updated after the process started.

type opencodeSession struct {
	ID        string `json:"id"`
	Directory string `json:"directory"`
	ParentID  string `json:"parentID"`
	Time      struct {
		Updated int64 `json:"updated"`
	} `json:"time"`
}

type opencodeMessage struct {
	Role     string `json:"role"`
	ModelID  string `json:"modelID"`
	Provider string `json:"providerID"`
	Tokens   *struct {
		Input     int `json:"input"`
		Output    int `json:"output"`
		Reasoning int `json:"reasoning"`
		Cache     struct {
			Read  int `json:"read"`
			Write int `json:"write"`
		} `json:"cache"`
	} `json:"tokens"`
}

type opencodeReader struct {
	sessions map[string]cached[opencodeSession]
	messages map[string]map[string]cached[opencodeMessage] // per session: only the newest ones read
	windows  map[string]cached[map[string]int]             // models.json: "provider/model" -> limit.context
}

func newOpencodeReader() *opencodeReader {
	return &opencodeReader{
		sessions: map[string]cached[opencodeSession]{},
		messages: map[string]map[string]cached[opencodeMessage]{},
		windows:  map[string]cached[map[string]int]{},
	}
}

// session returns the root session in cwd that was updated after the
// process started at startMs (Unix ms), and how many there are: more than
// one (a finished `opencode run`, a /new) cannot be told apart.
func (r *opencodeReader) session(data, cwd string, startMs int64) (best opencodeSession, n int) {
	files, _ := filepath.Glob(filepath.Join(data, "storage", "session", "*", "ses_*.json"))
	seen := make(map[string]bool, len(files))
	for _, f := range files {
		seen[f] = true
		s := readCached(r.sessions, f, func(b []byte) opencodeSession {
			var s opencodeSession
			_ = json.Unmarshal(b, &s)
			return s
		})
		if s.ID == "" || s.ParentID != "" || s.Directory != cwd || s.Time.Updated < startMs {
			continue
		}
		n++
		if s.Time.Updated > best.Time.Updated {
			best = s
		}
	}
	// Prune only this data directory: another process can use another one.
	prefix := filepath.Join(data, "storage", "session") + string(filepath.Separator)
	for f := range r.sessions {
		if !seen[f] && strings.HasPrefix(f, prefix) {
			delete(r.sessions, f)
		}
	}
	return best, n
}

// opencodeLook bounds how many of the newest messages are read to find the
// last reply with output: a turn writes a few messages at most.
const opencodeLook = 50

// usage counts the context as OpenCode's own sidebar does: the newest
// assistant message with output, input + output + reasoning + cache read +
// cache write, against the model's limit.context in its models cache.
func (r *opencodeReader) usage(data, cache, sessionID string) (u Usage, provider string, ok bool) {
	dir := filepath.Join(data, "storage", "message", filepath.Base(sessionID))
	ents, err := os.ReadDir(dir)
	if err != nil {
		return Usage{}, "", false
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "msg_") && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	// Message ids grow with time.
	slices.Sort(names)
	prev, kept := r.messages[dir], map[string]cached[opencodeMessage]{}
	r.messages[dir] = kept
	for i, n := len(names)-1, 0; i >= 0 && n < opencodeLook; i, n = i-1, n+1 {
		path := filepath.Join(dir, names[i])
		if c, ok := prev[path]; ok {
			kept[path] = c
		}
		m := readCached(kept, path, func(b []byte) opencodeMessage {
			var m opencodeMessage
			_ = json.Unmarshal(b, &m)
			return m
		})
		if m.Role != "assistant" || m.Tokens == nil || m.Tokens.Output <= 0 {
			continue
		}
		t := m.Tokens
		u = Usage{Model: m.ModelID, Tokens: t.Input + t.Output + t.Reasoning + t.Cache.Read + t.Cache.Write}
		u.Window = r.window(cache, m.Provider, m.ModelID)
		return u, m.Provider, true
	}
	return Usage{}, "", false
}

func (r *opencodeReader) window(cache, provider, model string) int {
	limits := readCached(r.windows, filepath.Join(cache, "opencode", "models.json"), func(b []byte) map[string]int {
		var f map[string]struct {
			Models map[string]struct {
				Limit struct {
					Context int `json:"context"`
				} `json:"limit"`
			} `json:"models"`
		}
		if json.Unmarshal(b, &f) != nil {
			return nil
		}
		m := map[string]int{}
		for p, v := range f {
			for id, md := range v.Models {
				if md.Limit.Context > 0 {
					m[p+"/"+id] = md.Limit.Context
				}
			}
		}
		return m
	})
	return limits[provider+"/"+model]
}
