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
	// LastLine and Asks are read from the newest reply with text; Said is
	// that reply when it is of the current turn (after the person's last
	// prompt), its Markdown plain and its end kept within maxSaid.
	LastLine string `json:"last_line,omitempty"`
	Asks     bool   `json:"asks,omitempty"`
	Said     string `json:"said,omitempty"`
	// Ask is the question tool's call that waits for its answer.
	Ask *said.Ask `json:"ask,omitempty"`
	// Out: the newest turn was refused by the plan's limit, which resets here.
	Out time.Time `json:"out,omitzero"`
	// Ended: the newest turn ended here and no line of a next one followed;
	// zero while a turn runs. Background is the work it left running.
	Ended      time.Time `json:"ended,omitzero"`
	Background int       `json:"background,omitempty"`
	// Failed: the newest turn ended on an API error or a failed tool call and
	// the model said nothing after it; the error's first line.
	Failed string `json:"failed,omitempty"`
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
	Type           string    `json:"type"`
	IsSidechain    bool      `json:"isSidechain"`
	Timestamp      time.Time `json:"timestamp"`
	APIError       string    `json:"apiError"`
	APIErrorParams struct {
		RateLimit struct {
			ResetsAt int64 `json:"resetsAt"`
		} `json:"rate_limit_info"`
	} `json:"apiErrorParams"`
	Message struct {
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
	newer := false // an assistant line newer than the one read was seen
	turnKnown := false
	prompted := false // the person's last prompt was passed: older words are another turn's
	for i := len(lines) - 1; i >= 0 && (!found || !decided || !turnKnown); i-- {
		l := lines[i]
		if !turnKnown {
			turnKnown = turnEnd(l, &u)
		}
		if !decided && !prompted && prompt(l) {
			prompted = true
		}
		if !found && compacted == nil && bytes.Contains(l, []byte(`"compact_boundary"`)) {
			// A reply that only mentions the word is not a boundary.
			var b boundaryLine
			if json.Unmarshal(l, &b) == nil && b.Subtype == "compact_boundary" {
				compacted, newer = &b, true // a compact ran: the model answered after any refusal
				continue
			}
		}
		// Decode only a line that can still tell something: a busy turn has
		// many large tool calls between two replies with text.
		needUsage := !found && bytes.Contains(l, []byte(`"usage"`))
		needText := !decided && bytes.Contains(l, []byte(`"type":"text"`))
		if !bytes.Contains(l, []byte(`"assistant"`)) || (!needUsage && !needText) {
			continue
		}
		var t transcriptLine
		if json.Unmarshal(l, &t) != nil || t.Type != "assistant" || t.IsSidechain {
			continue
		}
		// <synthetic> turns are local messages that report zero usage. The
		// newest one may be the limit refusing the turn: until a real turn
		// follows, the session is out until its reset.
		if t.Message.Model == "<synthetic>" {
			// Other local lines (a 529, "No response requested.") say
			// nothing about the limit: only a real turn or a compact ends it.
			if !newer && t.APIError == "usage_limit_reached" && t.APIErrorParams.RateLimit.ResetsAt > 0 {
				u.Out = time.Unix(t.APIErrorParams.RateLimit.ResetsAt, 0)
				newer = true
			}
			continue
		}
		newer = true
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
			if !prompted {
				u.Said = cut(strings.TrimSpace(said.Plain(said.Clean(text))), maxSaid)
			}
			decided = true
		}
	}
	u.Ask = pendingAsk(lines)
	u.Failed = failedTurn(lines)
	return u, found
}

// maxFailed bounds the error a failed turn shows: one row of the console.
const maxFailed = 40

// failedTurn is the error the newest line of the main thread carries, when
// that line is an API error (the limit's refusal is Out, not a failure) or a
// tool_result that failed. A reply of the model after it, or a prompt, ends
// the failure.
func failedTurn(lines [][]byte) string {
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if !bytes.Contains(l, []byte(`"user"`)) && !bytes.Contains(l, []byte(`"assistant"`)) {
			continue
		}
		var t struct {
			Type        string `json:"type"`
			IsSidechain bool   `json:"isSidechain"`
			APIError    string `json:"apiError"`
			IsAPIError  bool   `json:"isApiErrorMessage"`
			Message     struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(l, &t) != nil || t.IsSidechain || t.Type != "user" && t.Type != "assistant" {
			continue
		}
		if t.Type == "assistant" {
			if (t.IsAPIError || t.APIError != "") && t.APIError != "usage_limit_reached" {
				return firstLine(replyText(t.Message.Content))
			}
			return ""
		}
		var blocks []struct {
			Type    string          `json:"type"`
			IsError bool            `json:"is_error"`
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(t.Message.Content, &blocks) != nil {
			return "" // the person's prompt
		}
		for _, b := range blocks {
			if b.Type == "tool_result" && b.IsError {
				return firstLine(replyText(b.Content))
			}
		}
		return ""
	}
	return ""
}

// firstLine is the first line with text, cut to what the console shows.
func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			if r := []rune(l); len(r) > maxFailed {
				return string(r[:maxFailed])
			}
			return l
		}
	}
	return ""
}

// maxSaid bounds what a session said: the answer peek shows nine lines.
const maxSaid = 2000

// cut keeps the last n runes of s: a reply ends on what it asks.
func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return "…" + string(r[len(r)-n:])
	}
	return s
}

// prompt reports a line of the person's own prompt in the main thread: a
// turn starts there. A tool's result, a local command's output, a meta
// line and an interrupt notice are not prompts.
func prompt(l []byte) bool {
	if !bytes.Contains(l, []byte(`"user"`)) {
		return false
	}
	var t struct {
		Type        string `json:"type"`
		IsSidechain bool   `json:"isSidechain"`
		IsMeta      bool   `json:"isMeta"`
		Summary     bool   `json:"isCompactSummary"`
		Message     struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(l, &t) != nil || t.Type != "user" || t.IsSidechain || t.IsMeta || t.Summary {
		return false
	}
	var text string
	if json.Unmarshal(t.Message.Content, &text) == nil {
		return words(text)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(t.Message.Content, &blocks) != nil {
		return false
	}
	found := false
	for _, b := range blocks {
		switch {
		case b.Type == "tool_result":
			return false
		case b.Type == "text" && words(b.Text):
			found = true
		}
	}
	return found
}

// words: a prompt's text, not a notice Claude Code writes in the person's
// place.
func words(text string) bool {
	t := strings.TrimSpace(text)
	return t != "" && !strings.HasPrefix(t, "<local-command-") && !strings.HasPrefix(t, "[Request interrupted")
}

// pendingAsk is the question tool's call when it is the newest tool call
// of the main thread and no result answers it yet.
func pendingAsk(lines [][]byte) *said.Ask {
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if !bytes.Contains(l, []byte(`"tool_use"`)) || !bytes.Contains(l, []byte(`"assistant"`)) {
			continue
		}
		var t struct {
			Type        string `json:"type"`
			IsSidechain bool   `json:"isSidechain"`
			Message     struct {
				Content []struct {
					Type  string   `json:"type"`
					ID    string   `json:"id"`
					Name  string   `json:"name"`
					Input said.Ask `json:"input"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(l, &t) != nil || t.Type != "assistant" || t.IsSidechain {
			continue
		}
		for _, c := range t.Message.Content {
			if c.Type != "tool_use" || c.Name != said.AskTool || c.ID == "" || len(c.Input.Questions) == 0 {
				continue
			}
			answer := []byte(`"tool_use_id":"` + c.ID + `"`)
			for _, later := range lines[i+1:] {
				if bytes.Contains(later, answer) {
					return nil
				}
			}
			ask := c.Input.Clean()
			ask.ID = c.ID
			return &ask
		}
		return nil
	}
	return nil
}

// turnEnd reads one line, newest first, until it tells whether the turn
// ended: Claude Code closes a turn with a turn_duration line, and a prompt or
// a reply of the main thread is a turn at work. It reports whether the line
// decided.
func turnEnd(l []byte, u *Usage) bool {
	if !bytes.Contains(l, []byte(`"turn_duration"`)) && !bytes.Contains(l, []byte(`"user"`)) && !bytes.Contains(l, []byte(`"assistant"`)) {
		return false
	}
	var t struct {
		Type        string    `json:"type"`
		Subtype     string    `json:"subtype"`
		IsSidechain bool      `json:"isSidechain"`
		Timestamp   time.Time `json:"timestamp"`
		Background  int       `json:"pendingBackgroundAgentCount"`
		Message     struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(l, &t) != nil || t.IsSidechain {
		return false
	}
	switch {
	case t.Type == "system" && t.Subtype == "turn_duration":
		u.Ended, u.Background = t.Timestamp, t.Background
		return true
	case t.Type == "user" && localOutput(t.Message.Content):
		// A local command (/compact, /model) writes no turn_duration: its
		// output is its end.
		u.Ended = t.Timestamp
		return true
	case t.Type == "user" || t.Type == "assistant":
		return true
	}
	return false
}

// localOutput is the content of the line a local command writes when it is
// done.
func localOutput(content json.RawMessage) bool {
	var text string
	return json.Unmarshal(content, &text) == nil && strings.HasPrefix(text, "<local-command-stdout>")
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
