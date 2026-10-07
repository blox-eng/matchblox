// Package panes builds the tmux steps that type into a pane. Each step is
// argv: no shell reads the text.
package panes

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxAnswer bounds an answer: a short reply, not a document.
const MaxAnswer = 500

// Send types text into the pane as literal keys, then presses Enter as a
// key of its own. "--" ends the flags, so a text that starts with a dash
// stays text.
func Send(target, text string) [][]string {
	return [][]string{
		{"tmux", "send-keys", "-t", target, "-l", "--", text},
		{"tmux", "send-keys", "-t", target, "Enter"},
	}
}

// CheckAnswer refuses a text that is not one short line: a new line or a
// control key would do more in the session than the person read.
func CheckAnswer(text string) error {
	switch {
	case strings.TrimSpace(text) == "":
		return errors.New("the answer is empty")
	case utf8.RuneCountInString(text) > MaxAnswer || !utf8.ValidString(text):
		return errors.New("the answer is longer than 500 characters")
	case strings.IndexFunc(text, unicode.IsControl) >= 0:
		return errors.New("the answer must be one line, without control keys")
	}
	return nil
}
