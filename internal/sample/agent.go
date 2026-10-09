package sample

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/said"
)

// registryEntry is the per-process file Claude Code keeps at
// <config dir>/sessions/<pid>.json. Only the fields the console reads.
type registryEntry struct {
	SessionID       string `json:"sessionId"`
	Name            string `json:"name"`
	Cwd             string `json:"cwd"`
	Status          string `json:"status"`
	StartedAt       int64  `json:"startedAt"`
	StatusUpdatedAt int64  `json:"statusUpdatedAt"`
	Tmux            string `json:"tmux"` // session:@window.%pane
}

// Usage is the context state read from the end of a transcript.
type Usage struct {
	Model    string    `json:"model"`
	Tokens   int       `json:"tokens"`
	Window   int       `json:"window,omitempty"` // as the agent reports it; 0: not reported
	At       time.Time `json:"at"`
	Progress *Progress `json:"progress,omitempty"`
	// LastLine and Asks are read from the newest reply with text.
	LastLine string `json:"last_line,omitempty"`
	Asks     bool   `json:"asks,omitempty"`
}

type fileKey struct {
	size  int64
	mtime time.Time
}

// agentReader caches everything keyed on file size and mtime, so an idle
// session costs one stat per tick and no reads.
type agentReader struct {
	registry    map[string]cached[registryEntry]
	transcripts map[string]string    // sessionId -> path
	missing     map[string]time.Time // sessionId -> when the last search found nothing
	usage       map[string]cached[Usage]
	codex       map[string]cached[codexRead] // by rollout path
}

type cached[T any] struct {
	key fileKey
	val T
}

func newAgentReader() *agentReader {
	return &agentReader{
		registry:    map[string]cached[registryEntry]{},
		transcripts: map[string]string{},
		missing:     map[string]time.Time{},
		usage:       map[string]cached[Usage]{},
		codex:       map[string]cached[codexRead]{},
	}
}

func statKey(path string) (fileKey, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return fileKey{}, false
	}
	return fileKey{st.Size(), st.ModTime()}, true
}

func (r *agentReader) entry(dir string, pid int) (registryEntry, bool) {
	path := filepath.Join(dir, "sessions", strconv.Itoa(pid)+".json")
	key, ok := statKey(path)
	if !ok {
		return registryEntry{}, false
	}
	if c, ok := r.registry[path]; ok && c.key == key {
		return c.val, true
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return registryEntry{}, false
	}
	var e registryEntry
	if json.Unmarshal(b, &e) != nil {
		return registryEntry{}, false
	}
	r.registry[path] = cached[registryEntry]{key, e}
	return e, true
}

// transcript finds the session's .jsonl: first at the path derived from its
// working directory, then by searching every project. A session that has not
// written one yet is searched again at most every 30 s, because the search
// lists every transcript on the machine.
func (r *agentReader) transcript(dir, sessionID, cwd string) string {
	if p, ok := r.transcripts[sessionID]; ok {
		return p
	}
	direct := filepath.Join(dir, "projects", projectDir(cwd), sessionID+".jsonl")
	if _, err := os.Stat(direct); err == nil {
		r.transcripts[sessionID] = direct
		return direct
	}
	if at, ok := r.missing[sessionID]; ok && time.Since(at) < 30*time.Second {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "projects", "*", sessionID+".jsonl"))
	if len(matches) == 0 {
		r.missing[sessionID] = time.Now()
		return ""
	}
	delete(r.missing, sessionID)
	r.transcripts[sessionID] = matches[0]
	return matches[0]
}

// projectDir is how the agent names a project directory: every character
// outside [A-Za-z0-9] becomes '-'.
func projectDir(cwd string) string {
	b := []byte(cwd)
	for i, c := range b {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			b[i] = '-'
		}
	}
	return string(b)
}

func (r *agentReader) usageOf(dir, sessionID, cwd string) (Usage, bool) {
	path := r.transcript(dir, sessionID, cwd)
	if path == "" {
		return Usage{}, false
	}
	key, ok := statKey(path)
	if !ok {
		delete(r.transcripts, sessionID)
		return Usage{}, false
	}
	if c, ok := r.usage[path]; ok && c.key == key {
		return c.val, true
	}
	u, ok := lastUsage(path, key.size)
	if !ok {
		return Usage{}, false
	}
	r.usage[path] = cached[Usage]{key, u}
	return u, true
}

// tailBytes bounds the read: a transcript can be hundreds of megabytes and the
// last assistant turn is always near the end.
const tailBytes = 512 << 10

type transcriptLine struct {
	Type        string    `json:"type"`
	IsSidechain bool      `json:"isSidechain"`
	Timestamp   time.Time `json:"timestamp"`
	Message     struct {
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   *struct {
			Input         int `json:"input_tokens"`
			CacheRead     int `json:"cache_read_input_tokens"`
			CacheCreation int `json:"cache_creation_input_tokens"`
			Output        int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

func lastUsage(path string, size int64) (Usage, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Usage{}, false
	}
	defer f.Close()
	off := max(size-tailBytes, 0)
	buf := make([]byte, size-off)
	if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
		return Usage{}, false
	}
	lines := bytes.Split(buf, []byte{'\n'})
	var u Usage
	found, decided := false, false
	// A compact after the last turn leaves its own count: the next turn
	// has not run, and the turn before it holds the old, full context.
	var compacted *boundaryLine
	for i := len(lines) - 1; i >= 0 && (!found || !decided); i-- {
		l := lines[i]
		if !found && compacted == nil && bytes.Contains(l, []byte(`"compact_boundary"`)) {
			var b boundaryLine
			if json.Unmarshal(l, &b) == nil && b.Subtype == "compact_boundary" && b.Meta.Post > 0 {
				compacted = &b
			}
			continue
		}
		// Decode only a line that can still tell something: a busy turn has
		// many large tool calls between two replies with text.
		needUsage := !found && bytes.Contains(l, []byte(`"usage"`))
		needText := !decided && bytes.Contains(l, []byte(`"type":"text"`))
		if !bytes.Contains(l, []byte(`"assistant"`)) || (!needUsage && !needText) {
			continue
		}
		var t transcriptLine
		// <synthetic> turns are local messages that report zero usage.
		if json.Unmarshal(l, &t) != nil || t.Type != "assistant" || t.IsSidechain || t.Message.Model == "<synthetic>" {
			continue
		}
		if needUsage && t.Message.Usage != nil {
			n := t.Message.Usage
			u.Model, u.Tokens, u.At = t.Message.Model, n.Input+n.CacheRead+n.CacheCreation+n.Output, t.Timestamp
			if compacted != nil {
				u.Tokens, u.At = compacted.Meta.Post, compacted.Timestamp
			}
			found = true
		}
		// The newest reply with text tells the progress, bar or no bar.
		if text := replyText(t.Message.Content); needText && text != "" {
			if p, ok := parseProgress(text); ok {
				u.Progress = &p
			}
			u.LastLine, u.Asks = said.Reply(text)
			decided = true
		}
	}
	return u, found
}

// modelSetting is the model a Claude Code process was started with, in the
// order Claude Code resolves it: the --model flag, ANTHROPIC_MODEL, then the
// project's local and shared settings and the user's settings in its config
// directory. A change made with /model inside the session is not visible here.
func modelSetting(cmdline string, env func(string) (string, bool), cwd, dir string) string {
	args := strings.Fields(cmdline)
	for i, a := range args {
		if v, ok := strings.CutPrefix(a, "--model="); ok {
			return v
		}
		if a == "--model" && i+1 < len(args) {
			return args[i+1]
		}
	}
	if v, ok := env("ANTHROPIC_MODEL"); ok && v != "" {
		return v
	}
	for _, f := range []string{
		filepath.Join(cwd, ".claude", "settings.local.json"),
		filepath.Join(cwd, ".claude", "settings.json"),
		filepath.Join(dir, "settings.json"),
	} {
		var s struct {
			Model string `json:"model"`
		}
		if b, err := os.ReadFile(f); err == nil && json.Unmarshal(b, &s) == nil && s.Model != "" {
			return s.Model
		}
	}
	return ""
}

// longContext reports a model setting that asks for the 1M window, such as
// "opus[1m]". The transcript names the model without that suffix.
func longContext(model string) bool {
	return strings.HasSuffix(strings.ToLower(model), "[1m]")
}
