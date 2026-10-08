package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/blox-eng/matchblox/internal/doors"
)

// Events are the Claude Code hooks the queue reads.
var Events = []string{"Notification", "Stop", "UserPromptSubmit", "SessionStart", "SessionEnd", "PostToolUse"}

// settingsPath is Claude Code's user settings: $CLAUDE_CONFIG_DIR, else
// ~/.claude.
func settingsPath(e Env) string {
	if dir := e.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "settings.json")
	}
	return filepath.Join(e.Home, ".claude", "settings.json")
}

func usesClaude(e Env) bool {
	if _, err := e.LookPath("claude"); err == nil {
		return true
	}
	_, err := os.Stat(filepath.Dir(settingsPath(e)))
	return err == nil
}

func hooksDoor(e Env) doors.Door {
	path := settingsPath(e)
	d := doors.Door{ID: doors.Hooks, Title: "Add the queue hooks", Path: path,
		Why: "Claude Code tells matchblox when an agent waits for you. Without the hooks the queue is estimated."}
	_, old, _, err := read(path)
	if err != nil {
		d.Problem = err.Error()
		return d
	}
	next, err := withHooks(old, e.Exe)
	if err != nil {
		d.Problem = tilde(e.Home, path) + " is not valid JSON (" + err.Error() + "): fix it, then this door opens"
		return d
	}
	if bytes.Equal(next, old) {
		d.Done = true
		return d
	}
	d.Sum, d.Preview = Sum(old), diff(string(old), string(next))
	return d
}

// AddHooks adds a `matchblox hook <event>` command for each of Events to
// Claude Code's settings. Other hooks and keys stay as they are, and an
// event that already runs matchblox gets no second one.
func AddHooks(settingsPath, exe, seen string) (backup string, err error) {
	return edit(settingsPath, seen, func(old []byte) ([]byte, error) { return withHooks(old, exe) })
}

// member is one key of a JSON object, its value kept byte for byte.
type member struct {
	key string
	val json.RawMessage
}

// withHooks is the settings with every hook of Events in place. Only the
// arrays it adds to are written again; every other value keeps its bytes,
// so the diff shows the new lines and nothing else.
func withHooks(old []byte, exe string) ([]byte, error) {
	top, err := members(old)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(top, func(m member) bool { return m.key == "hooks" })
	if i < 0 {
		top = append(top, member{key: "hooks", val: json.RawMessage("{}")})
		i = len(top) - 1
	}
	events, err := members(top[i].val)
	if err != nil {
		return nil, fmt.Errorf("hooks: %w", err)
	}
	changed := false
	for _, ev := range Events {
		j := slices.IndexFunc(events, func(m member) bool { return m.key == ev })
		var entries []json.RawMessage
		if j >= 0 {
			if err := json.Unmarshal(events[j].val, &entries); err != nil {
				return nil, fmt.Errorf("hooks.%s: %w", ev, err)
			}
			if runsMatchblox(entries, ev) {
				continue
			}
		}
		entries = append(entries, entry(exe, ev))
		val := array(entries, 2)
		if j >= 0 {
			events[j].val = val
		} else {
			events = append(events, member{key: ev, val: val})
		}
		changed = true
	}
	if !changed {
		return old, nil
	}
	top[i].val = object(events, 1)
	return append(object(top, 0), '\n'), nil
}

// members reads a JSON object; an empty file is an empty object.
func members(b []byte) ([]member, error) {
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, errors.New("not an object")
	}
	var out []member
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return nil, err
		}
		out = append(out, member{key: t.(string), val: val})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("more after the object")
	}
	return out, nil
}

// object writes an object at a depth of 2-space indents: the shape Claude
// Code writes its settings in.
func object(ms []member, depth int) json.RawMessage {
	if len(ms) == 0 {
		return json.RawMessage("{}")
	}
	in := strings.Repeat("  ", depth+1)
	var b bytes.Buffer
	b.WriteString("{\n")
	for i, m := range ms {
		b.WriteString(in + quote(m.key) + ": ")
		b.Write(m.val)
		if i < len(ms)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(strings.Repeat("  ", depth) + "}")
	return b.Bytes()
}

func array(vs []json.RawMessage, depth int) json.RawMessage {
	in := strings.Repeat("  ", depth+1)
	var b bytes.Buffer
	b.WriteString("[\n")
	for i, v := range vs {
		b.WriteString(in)
		b.Write(v)
		if i < len(vs)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(strings.Repeat("  ", depth) + "]")
	return b.Bytes()
}

// entry is one hook group on one line, so the diff stays short.
func entry(exe, ev string) json.RawMessage {
	return json.RawMessage(`{"hooks": [{"type": "command", "command": ` + quote(hookCommand(exe, ev)) + `}]}`)
}

func hookCommand(exe, ev string) string {
	if exe == "" {
		exe = "matchblox"
	}
	return shellQuote(exe) + " hook " + ev
}

// runsMatchblox: an entry of the event already runs `matchblox hook <ev>`,
// from any path.
func runsMatchblox(entries []json.RawMessage, ev string) bool {
	for _, raw := range entries {
		var g struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		}
		if json.Unmarshal(raw, &g) != nil {
			continue
		}
		for _, h := range g.Hooks {
			f := strings.Fields(h.Command)
			if len(f) >= 3 && filepath.Base(strings.Trim(f[0], `'"`)) == "matchblox" && f[1] == "hook" && f[2] == ev {
				return true
			}
		}
	}
	return false
}

func quote(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // a string always encodes
	return strings.TrimSuffix(b.String(), "\n")
}

// shellQuote leaves a plain path as it is and quotes any other.
func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/._-+") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func shellJoin(argv []string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = shellQuote(a)
	}
	return strings.Join(q, " ")
}

func tilde(home, path string) string {
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok && home != "" {
		return "~/" + rest
	}
	return path
}
