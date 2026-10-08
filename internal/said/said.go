// Package said reads what a coding agent said last, from its screen or its
// reply: the line to show, and whether it asks the person something.
package said

import (
	"regexp"
	"strings"
	"unicode"
)

// within is how many lines with words, from the end, a question may sit
// above: an approval prompt lists its choices and a key hint under the
// question, and a reply its progress line. A question further up was
// answered.
const within = 8

var (
	// asksRe is a prompt that asks without ending in a question mark:
	// aider's (Y)es/(N)o, a [y/n], OpenCode's permission box, or a question
	// with a sentence after it.
	asksRe = regexp.MustCompile(`(?i)\(y\)es/\(n\)o|[\[(]y/n[\])]|^permission required\b|\pL\?\s+\pL`)
	// hintRe is a line of key hints or a footer, never what the agent said.
	hintRe = regexp.MustCompile(`(?i)\b(ctrl|shift|alt|esc|enter)\b|^\? for shortcuts|^[~/]\S*`)
	wordRe = regexp.MustCompile(`\pL\pL`)
)

type line struct {
	text  string
	boxed bool // inside a box: an input field, a prompt, a quote
}

// Screen reads an agent's pane. When it asks, the line is the question;
// else it is the last line with words outside the input box and the key
// hints.
func Screen(screen string) (string, bool) {
	ls := lines(screen)
	if q, ok := question(ls); ok {
		return q, true
	}
	for i := len(ls) - 1; i >= 0; i-- {
		if !ls[i].boxed && !hintRe.MatchString(ls[i].text) {
			return ls[i].text, false
		}
	}
	return "", false
}

// Reply reads an agent's last reply (Markdown). The progress line that
// ends each reply (hooks.ProgressPrompt) is never what it said.
func Reply(text string) (string, bool) {
	var ls []line
	for _, l := range lines(text) {
		if !strings.HasPrefix(l.text, "Progress [") {
			ls = append(ls, l)
		}
	}
	if q, ok := question(ls); ok {
		return q, true
	}
	if len(ls) == 0 {
		return "", false
	}
	return ls[len(ls)-1].text, false
}

// lines keeps the lines with words, without the frame around them.
func lines(s string) []line {
	var out []line
	// The text is not trusted: a control key would drive the terminal.
	s = strings.Map(func(r rune) rune {
		if r != '\n' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	for _, raw := range strings.Split(s, "\n") {
		t := strings.TrimFunc(raw, frame)
		if len(wordRe.FindAllString(t, 2)) < 2 && !strings.HasSuffix(t, "?") {
			continue
		}
		in := strings.TrimLeft(raw, " ")
		out = append(out, line{t, strings.HasPrefix(in, "│") || strings.HasPrefix(in, "┃")})
	}
	return out
}

func question(ls []line) (string, bool) {
	for i := len(ls) - 1; i >= 0 && i >= len(ls)-within; i-- {
		t := ls[i].text
		if (strings.HasSuffix(t, "?") && !strings.HasPrefix(t, "?")) || asksRe.MatchString(t) {
			return t, true
		}
	}
	return "", false
}

// frame is what a TUI or Markdown draws around words: space, box and block
// drawing, emphasis, and the markers in front of a turn or a choice.
func frame(r rune) bool {
	switch {
	case unicode.IsSpace(r), r >= 0x2500 && r <= 0x259f: // box drawing, blocks
		return true
	}
	return strings.ContainsRune("❯›>•⏺·*|⎿_`#", r)
}
