// Package said reads what a coding agent said last, from its screen or its
// reply: the line to show, and whether it asks the person something.
package said

import (
	"regexp"
	"strings"
	"unicode"
)

// Kind is what a screen asks of the person.
type Kind int

const (
	None Kind = iota
	// Asks is a question: the person can answer it with a line of text.
	Asks
	// Permits is an approval prompt: a menu where Enter takes the selected
	// choice. It is answered in its pane, never by typing a line into it.
	Permits
)

func (k Kind) String() string { return [...]string{"none", "asks", "permits"}[k] }

const (
	// screenWithin is how many lines with words, from the bottom of a pane,
	// a prompt may sit above: an approval prompt lists its choices and a key
	// hint under the question. A question further up was answered.
	screenWithin = 8
	// replyWithin is the end of a reply: a question further up in a reply
	// is one the agent answered itself.
	replyWithin = 3
)

var (
	// permitsRe is an approval prompt, in the agents' own words: OpenCode's
	// permission box and its choices, aider's (Y)es/(N)o, a [y/n], Codex's
	// confirm hint, a numbered Yes choice (Claude Code, Codex).
	permitsRe = regexp.MustCompile(`(?i)^permission required\b|^allow once\b|\(y\)es/\(n\)o|[\[(]y/n[\])]|press enter to confirm|^\d\.\s+yes\b`)
	// midRe is a question with a sentence after it.
	midRe = regexp.MustCompile(`\pL\?\s+\pL`)
	// hintRe is a line of key hints or a footer, never what the agent said.
	hintRe = regexp.MustCompile(`(?i)\b(ctrl|shift|alt|esc|enter)\b|^\? for shortcuts|^[~/]\S*`)
	wordRe = regexp.MustCompile(`\pL\pL`)
)

type line struct {
	text    string
	boxed   bool // inside a box: an input field, a prompt, a quote
	person  bool // the person's own turn or draft: › or > or ❯ in front
	heading bool // a Markdown heading
}

// Screen reads an agent's pane. When it asks or wants an approval, the line
// is the question; else it is the last line with words outside the input
// box and the key hints.
func Screen(screen string) (string, Kind) {
	ls := lines(screen)
	win := ls[max(len(ls)-screenWithin, 0):]
	for _, l := range win {
		if permitsRe.MatchString(l.text) {
			for i := len(win) - 1; i >= 0; i-- {
				if strings.HasSuffix(win[i].text, "?") && !strings.HasPrefix(win[i].text, "?") {
					return win[i].text, Permits
				}
			}
			return l.text, Permits
		}
	}
	if q, ok := question(win); ok {
		return q, Asks
	}
	for i := len(ls) - 1; i >= 0; i-- {
		if !ls[i].boxed && !ls[i].person && !hintRe.MatchString(ls[i].text) {
			return ls[i].text, None
		}
	}
	return "", None
}

// Draft is what the person typed into an agent's input field and did not
// send: the text after the prompt marker on the lowest prompt line of the
// screen. "" when the field is empty or shows its placeholder.
func Draft(screen string) string {
	raw := strings.Split(strings.TrimRight(Clean(screen), "\n"), "\n")
	for i := len(raw) - 1; i >= max(len(raw)-2*screenWithin, 0); i-- {
		in := strings.TrimLeft(raw[i], " │┃")
		for _, mark := range []string{">", "›", "❯"} {
			rest, ok := strings.CutPrefix(in, mark)
			if !ok {
				continue
			}
			t := strings.TrimFunc(rest, frame)
			if strings.HasPrefix(t, `Try "`) {
				return "" // Claude Code's placeholder in an empty field
			}
			return t
		}
	}
	return ""
}

// Reply reads an agent's last reply (Markdown). The progress line that
// ends each reply (hooks.ProgressPrompt) and headings are never what it
// said.
func Reply(text string) (string, bool) {
	var ls []line
	for _, l := range lines(text) {
		if !l.heading && !strings.HasPrefix(l.text, "Progress [") {
			ls = append(ls, l)
		}
	}
	if q, ok := question(ls[max(len(ls)-replyWithin, 0):]); ok {
		return q, true
	}
	if len(ls) == 0 {
		return "", false
	}
	return ls[len(ls)-1].text, false
}

// Clean makes untrusted text safe to show: a tab is a space, and every
// other control key, which would drive the terminal, is dropped. New lines
// stay.
func Clean(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case r != '\n' && unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
}

// lines keeps the lines with words, without the frame around them.
func lines(s string) []line {
	s = Clean(s)
	var out []line
	for _, raw := range strings.Split(s, "\n") {
		t := strings.TrimFunc(raw, frame)
		if len(wordRe.FindAllString(t, 2)) < 2 && !strings.HasSuffix(t, "?") && !permitsRe.MatchString(t) {
			continue
		}
		in := strings.TrimLeft(raw, " ")
		out = append(out, line{
			text:    t,
			boxed:   strings.HasPrefix(in, "│") || strings.HasPrefix(in, "┃"),
			person:  strings.HasPrefix(in, "›") || strings.HasPrefix(in, ">") || strings.HasPrefix(in, "❯"),
			heading: strings.HasPrefix(in, "#"),
		})
	}
	return out
}

// question is the last line, of the person's own lines and the input box
// left out, that asks.
func question(ls []line) (string, bool) {
	for i := len(ls) - 1; i >= 0; i-- {
		l := ls[i]
		if l.boxed || l.person {
			continue
		}
		if (strings.HasSuffix(l.text, "?") && !strings.HasPrefix(l.text, "?")) || midRe.MatchString(l.text) {
			return l.text, true
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
