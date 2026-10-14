package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/said"
)

// Events are the Claude Code hooks the queue reads.
var Events = []string{"Notification", "Stop", "UserPromptSubmit", "SessionStart", "SessionEnd", "PostToolUse", "PreToolUse"}

// matchers narrow an event to the tools it is for, one entry each.
// PreToolUse is for the question tool, the one sign of a waiting menu
// that Claude Code sends before the menu draws, and for Bash, where the
// merge guard stops a merge, release or deploy the stoker's order did not
// give.
var matchers = map[string][]string{"PreToolUse": {said.AskTool, hooks.BashTool}}

// HandRules are the permission rules that let the stoker's hands run
// without a prompt in Claude Code: the service refuses their acts from any
// pane but the stoker's and the panes it drives, under its own guards.
var HandRules = []string{"Bash(matchblox sessions:*)", "Bash(matchblox ships:*)", "Bash(matchblox usage:*)",
	"Bash(matchblox send:*)", "Bash(matchblox compact:*)", "Bash(matchblox hold:*)",
	"Bash(matchblox release:*)", "Bash(matchblox stoker:*)", "Bash(matchblox peek:*)"}

// tools are the tools an event's entries are for; "" is an entry with no
// matcher.
func tools(ev string) []string {
	if m, ok := matchers[ev]; ok {
		return m
	}
	return []string{""}
}

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
		Why: "Claude Code tells matchblox when an agent waits for you. Without the hooks the queue is estimated, and the stoker's matchblox commands ask before they run."}
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
// Claude Code's settings, and HandRules to its allowed permissions. Other
// hooks, rules and keys stay as they are, and nothing is added twice.
func AddHooks(settingsPath, exe, seen string) (backup string, err error) {
	return edit(settingsPath, seen, func(old []byte) ([]byte, error) { return withHooks(old, exe) })
}

// member is one key of a JSON object, its value kept byte for byte.
type member struct {
	key string
	val json.RawMessage
}

// withHooks is the settings with every hook of Events and every rule of
// HandRules in place. Only the arrays it adds to are written again; every
// other value keeps its bytes, so the diff shows the new lines and nothing
// else.
func withHooks(old []byte, exe string) ([]byte, error) {
	top, err := members(old)
	if err != nil {
		return nil, err
	}
	// Claude Code reads the last of two equal keys, as JSON.parse does.
	i := lastIndex(top, "hooks")
	if i >= 0 && string(bytes.TrimSpace(top[i].val)) == "null" {
		top[i].val = json.RawMessage("{}")
	}
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
		j := lastIndex(events, ev)
		var entries []json.RawMessage
		if j >= 0 {
			if err := json.Unmarshal(events[j].val, &entries); err != nil {
				return nil, fmt.Errorf("hooks.%s: %w", ev, err)
			}
		}
		added := false
		for _, tool := range tools(ev) {
			if !runsMatchblox(entries, ev, tool) {
				entries, added = append(entries, entry(exe, ev, tool)), true
			}
		}
		if !added {
			continue
		}
		val := array(entries, 2)
		if j >= 0 {
			events[j].val = val
		} else {
			events = append(events, member{key: ev, val: val})
		}
		changed = true
	}
	if changed {
		top[i].val = object(events, 1)
	}
	top, added, err := withRules(top, i)
	if err != nil {
		return nil, err
	}
	if !changed && !added {
		return old, nil
	}
	return append(object(top, 0), '\n'), nil
}

// withRules adds the HandRules permissions.allow lacks. A settings file
// without permissions gets them right after hooks (at).
func withRules(top []member, at int) ([]member, bool, error) {
	i := lastIndex(top, "permissions")
	if i >= 0 && string(bytes.TrimSpace(top[i].val)) == "null" {
		top[i].val = json.RawMessage("{}")
	}
	if i < 0 {
		top = slices.Insert(top, at+1, member{key: "permissions", val: json.RawMessage("{}")})
		i = at + 1
	}
	perms, err := members(top[i].val)
	if err != nil {
		return nil, false, fmt.Errorf("permissions: %w", err)
	}
	j := lastIndex(perms, "allow")
	var allow []json.RawMessage
	if j >= 0 && string(bytes.TrimSpace(perms[j].val)) != "null" {
		if err := json.Unmarshal(perms[j].val, &allow); err != nil {
			return nil, false, fmt.Errorf("permissions.allow: %w", err)
		}
	}
	have := map[string]bool{}
	for _, raw := range allow {
		var r string
		if json.Unmarshal(raw, &r) == nil {
			have[r] = true
		}
	}
	added := false
	for _, r := range HandRules {
		if !have[r] {
			allow, added = append(allow, json.RawMessage(quote(r))), true
		}
	}
	if !added {
		return top, false, nil
	}
	if j >= 0 {
		perms[j].val = array(allow, 2)
	} else {
		perms = append(perms, member{key: "allow", val: array(allow, 2)})
	}
	top[i].val = object(perms, 1)
	return top, true, nil
}

func lastIndex(ms []member, key string) int {
	for i := len(ms) - 1; i >= 0; i-- {
		if ms[i].key == key {
			return i
		}
	}
	return -1
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
func entry(exe, ev, tool string) json.RawMessage {
	match := ""
	if tool != "" {
		match = `"matcher": ` + quote(tool) + `, `
	}
	return json.RawMessage(`{` + match + `"hooks": [{"type": "command", "command": ` + quote(hookCommand(exe, ev)) + `}]}`)
}

func hookCommand(exe, ev string) string {
	if exe == "" {
		exe = "matchblox"
	}
	return shellQuote(exe) + " hook " + ev
}

// runsMatchblox: an entry of the event already runs `matchblox hook <ev>`,
// from any path, for the tool ("" for any).
func runsMatchblox(entries []json.RawMessage, ev, tool string) bool {
	for _, raw := range entries {
		var g struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		}
		if json.Unmarshal(raw, &g) != nil {
			continue
		}
		if tool != "" && !matches(g.Matcher, tool) {
			continue
		}
		for _, h := range g.Hooks {
			f := shellWords(h.Command)
			if len(f) >= 3 && filepath.Base(f[0]) == "matchblox" && f[1] == "hook" && f[2] == ev {
				return true
			}
		}
	}
	return false
}

// matches reads a hook matcher as Claude Code does: empty or "*" is every
// tool, else a pattern the whole tool name matches.
func matches(matcher, tool string) bool {
	if matcher == "" || matcher == "*" {
		return true
	}
	re, err := regexp.Compile(`^(?:` + matcher + `)$`)
	return err == nil && re.MatchString(tool)
}

// shellWords splits a command as sh does for plain words and quotes; it is
// enough to read back what hookCommand writes.
func shellWords(s string) []string {
	var words []string
	var w strings.Builder
	quote, any := byte(0), false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0 && c == quote:
			quote = 0
		case quote != 0:
			w.WriteByte(c)
		case c == '\'' || c == '"':
			quote, any = c, true
		case c == '\\' && i+1 < len(s):
			i++
			w.WriteByte(s[i])
			any = true
		case c == ' ' || c == '\t':
			if any || w.Len() > 0 {
				words = append(words, w.String())
			}
			w.Reset()
			any = false
		default:
			w.WriteByte(c)
		}
	}
	if any || w.Len() > 0 {
		words = append(words, w.String())
	}
	return words
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
