package sample

import "strings"

// knownWindows is the context window of each Claude model as Claude Code
// starts it, by model id prefix. The longest prefix wins. Models with 1M as
// an option only ([1m] in the model setting) have their default here.
var knownWindows = map[string]int{
	"claude-fable-5":    1_000_000,
	"claude-mythos-5":   1_000_000,
	"claude-opus-5":     1_000_000,
	"claude-opus-4-8":   1_000_000,
	"claude-opus-4-7":   1_000_000,
	"claude-opus-4-6":   1_000_000,
	"claude-sonnet-5":   1_000_000,
	"claude-sonnet-4-6": 1_000_000,
	"claude-haiku-5":    1_000_000,
	"claude-opus-4-5":   200_000,
	"claude-opus-4-1":   200_000,
	"claude-opus-4-2":   200_000, // claude-opus-4-20250514
	"claude-sonnet-4-5": 200_000,
	"claude-sonnet-4-2": 200_000, // claude-sonnet-4-20250514
	"claude-haiku-4-5":  200_000,
	"claude-3":          200_000,
}

// window is the context window a session is measured against, first match
// wins: the window the agent reports for the session, then the config's
// [sessions.windows], then knownWindows. A session past its table window
// shows that the table is wrong for it. Zero: not measured.
func (s *Sampler) window(u Usage) int {
	if u.Window > 0 {
		return u.Window
	}
	if w := longestPrefix(s.Rules.Windows, u.Model); w > 0 {
		return w
	}
	model := u.Model
	if i := strings.Index(model, "claude-"); i > 0 {
		model = model[i:] // a cloud id, such as anthropic.claude-opus-5-5
	}
	if w := longestPrefix(knownWindows, model); w >= u.Tokens {
		return w
	}
	return 0
}

func longestPrefix(windows map[string]int, model string) int {
	w, best := 0, -1
	for prefix, n := range windows {
		if strings.HasPrefix(model, prefix) && len(prefix) > best {
			w, best = n, len(prefix)
		}
	}
	return w
}
