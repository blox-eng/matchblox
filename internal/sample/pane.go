package sample

import (
	"regexp"
	"strings"
	"unicode"
)

// asksWithin is how many lines with words, from the bottom of the pane, a
// question may sit above: an approval prompt lists its choices and a key
// hint under the question. A question further up was answered.
const asksWithin = 8

var (
	// asksRe is a prompt that asks without a question mark: aider's
	// (Y)es/(N)o, a [y/n], OpenCode's permission box.
	asksRe = regexp.MustCompile(`(?i)\(y\)es/\(n\)o|[\[(]y/n[\])]|^permission required\b`)
	// hintRe is a line of key hints or a footer, never what the agent said.
	hintRe = regexp.MustCompile(`(?i)\b(ctrl|shift|alt|esc|enter)\b|^\? for shortcuts|^[~/]\S*`)
	wordRe = regexp.MustCompile(`\pL\pL`)
)

// paneLine reads an agent's screen: the last line it said, and whether it
// asks the person something. When it asks, the line is the question.
func paneLine(screen string) (line string, asks bool) {
	type text struct {
		line  string
		boxed bool // inside a box: an input field, a prompt, a quote
	}
	var said []text // lines with words, bottom last
	for _, raw := range strings.Split(screen, "\n") {
		l := strings.TrimFunc(raw, frame)
		if len(wordRe.FindAllString(l, 2)) < 2 && !strings.HasSuffix(l, "?") {
			continue
		}
		in := strings.TrimLeft(raw, " ")
		said = append(said, text{l, strings.HasPrefix(in, "│") || strings.HasPrefix(in, "┃")})
	}
	for i := len(said) - 1; i >= 0 && i >= len(said)-asksWithin; i-- {
		l := said[i].line
		if (strings.HasSuffix(l, "?") && !strings.HasPrefix(l, "?")) || asksRe.MatchString(l) {
			return l, true
		}
	}
	// What the agent said is never the input box (its placeholder reads
	// like words) nor a footer of key hints.
	for i := len(said) - 1; i >= 0; i-- {
		if !said[i].boxed && !hintRe.MatchString(said[i].line) {
			return said[i].line, false
		}
	}
	return "", false
}

// frame is what a TUI draws around words: space, box and block drawing,
// and the markers in front of a turn or a choice.
func frame(r rune) bool {
	switch {
	case unicode.IsSpace(r), r >= 0x2500 && r <= 0x259f: // box drawing, blocks
		return true
	}
	return strings.ContainsRune("❯›>•⏺·*|⎿", r)
}
