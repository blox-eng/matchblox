package setup

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/blox-eng/matchblox/internal/doors"
)

// pctOverride is the variable that sets the percentage of the context window
// at which Claude Code compacts a session by itself.
const pctOverride = "CLAUDE_AUTOCOMPACT_PCT_OVERRIDE"

func safetyNetDoor(e Env) doors.Door {
	path := settingsPath(e)
	line := strconv.Itoa(int(e.compactAt() + 0.5))
	d := doors.Door{ID: doors.SafetyNet, Title: "Keep a net under every session", Path: path,
		Why: "Claude Code compacts a session by itself at " + line + "% of its context, even mid-turn. The stoker picks a boundary before that; this is the net when it cannot."}
	_, old, _, err := read(path)
	if err != nil {
		d.Problem = err.Error()
		return d
	}
	next, err := withSafetyNet(old, line)
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

// compactAt is the stoker's line, compact_at; 85 when it is not set.
func (e Env) compactAt() float64 {
	if e.CompactAt > 0 {
		return e.CompactAt
	}
	return 85
}

// TurnOnSafetyNet writes autoCompactEnabled and the percentage override.
func TurnOnSafetyNet(settingsPath string, line float64, seen string) (backup string, err error) {
	pct := strconv.Itoa(int(line + 0.5))
	return edit(settingsPath, seen, func(old []byte) ([]byte, error) { return withSafetyNet(old, pct) })
}

// withSafetyNet is the settings with auto-compact on and a percentage in
// env; an override that is there already is the builder's and stays. The
// other keys keep their bytes.
func withSafetyNet(old []byte, pct string) ([]byte, error) {
	top, err := members(old)
	if err != nil {
		return nil, err
	}
	changed := false
	if i := lastIndex(top, "autoCompactEnabled"); i >= 0 && string(bytes.TrimSpace(top[i].val)) == "false" {
		top[i].val, changed = json.RawMessage("true"), true
	}
	i := lastIndex(top, "env")
	var env []member
	if i >= 0 && string(bytes.TrimSpace(top[i].val)) != "null" {
		if env, err = members(top[i].val); err != nil {
			return nil, err
		}
	}
	if lastIndex(env, pctOverride) < 0 {
		env = append(env, member{key: pctOverride, val: json.RawMessage(quote(pct))})
		parts := make([]string, len(env))
		for j, m := range env {
			parts[j] = quote(m.key) + ": " + string(compact(m.val))
		}
		val := json.RawMessage("{" + strings.Join(parts, ", ") + "}")
		if i >= 0 {
			top[i].val = val
		} else {
			top = append(top, member{key: "env", val: val})
		}
		changed = true
	}
	if !changed {
		return old, nil
	}
	return append(object(top, 0), '\n'), nil
}
