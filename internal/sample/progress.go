package sample

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Progress is what an agent last reported as its progress: a bar such as
// "Progress [████░░░░] wiring the console" in its last reply with text.
type Progress struct {
	Pct  int    `json:"pct"`
	Step string `json:"step,omitempty"`
}

var (
	progressBar = regexp.MustCompile(`\[([█▓▰■#=]*)([░▱□\-.· ]*)\]`)
	barGlyphs   = regexp.MustCompile(`[█▓▰■░▱□]+`)
	pctWord     = regexp.MustCompile(`^\d{1,3}\s?%\s*|\s*\d{1,3}\s?%$`)
	spaces      = regexp.MustCompile(`\s+`)
)

// parseProgress reads the last bar of a reply. The step is the text after
// the bar, else the first line that says what runs.
func parseProgress(text string) (Progress, bool) {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		ms := progressBar.FindAllStringSubmatchIndex(lines[i], -1)
		for j := len(ms) - 1; j >= 0; j-- {
			m := ms[j]
			full := utf8.RuneCountInString(lines[i][m[2]:m[3]])
			empty := lines[i][m[4]:m[5]]
			total := full + utf8.RuneCountInString(empty)
			// Only the asked line: "Progress [...]". A bracket of = or # alone
			// is too common in code and logs.
			if total == 0 || (full == 0 && !strings.ContainsAny(empty, "░▱□")) ||
				!strings.Contains(strings.ToLower(lines[i][:m[0]]), "progress") {
				continue
			}
			step := strings.TrimSpace(pctWord.ReplaceAllString(strings.TrimSpace(lines[i][m[1]:]), ""))
			if step == "" {
				step = runningLine(lines)
			}
			return Progress{Pct: (100*full + total/2) / total, Step: clean(step)}, true
		}
	}
	return Progress{}, false
}

// clean drops control keys: the step comes from an agent's reply, and an
// escape sequence there must not reach the terminal.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

func runningLine(lines []string) string {
	for _, l := range lines {
		low := strings.ToLower(l)
		if strings.Contains(low, "running") || strings.Contains(low, "in progress") {
			return strings.TrimSpace(spaces.ReplaceAllString(barGlyphs.ReplaceAllString(l, " "), " "))
		}
	}
	return ""
}

// replyText is the text of an assistant message: a string, or the text
// blocks of a list.
func replyText(content json.RawMessage) string {
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}
