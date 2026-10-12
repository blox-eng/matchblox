// Package panes builds the tmux steps that type into a pane. Each step is
// argv: no shell reads the text.
package panes

import (
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/blox-eng/matchblox/internal/said"
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

// Pause is a step between two keys that change the question tool's page:
// keys sent before the next page draws are lost.
var Pause = []string{"sleep", "0.3"}

// Pick answers Claude Code's question tool (said.AskTool) the way its
// menu takes keys, found on a live pane (Claude Code 2.1):
//
//   - one choice: the option's digit picks it and moves on; the digit of
//     "Type something." (after the options) opens its field, the text is
//     typed and Enter moves on;
//   - several choices: each digit ticks its option and leaves the cursor
//     on the first row; Down to the "Type something" row and typing ticks
//     the text too; Down to the "Next" (or "Submit") row and Enter moves on;
//   - several questions, or one of several choices, end on a review page
//     whose first row, "Submit answers", sends them all.
//
// Text is only typed while the menu's own field has the cursor. ans must
// have passed ask.Check. pages are the steps where each page's keys start:
// one per question, then the review page when there is one. Each must be
// checked on the screen before its keys go.
func Pick(target string, ask said.Ask, ans said.Answer) (steps [][]string, pages []int) {
	key := func(keys ...string) []string {
		return append([]string{"tmux", "send-keys", "-t", target}, keys...)
	}
	text := func(t string) []string { return key("-l", "--", t) }
	down := func(n int) []string {
		keys := make([]string, n)
		for i := range keys {
			keys[i] = "Down"
		}
		return key(keys...)
	}
	for i, q := range ask.Questions {
		pages = append(pages, len(steps))
		c, n := ans.Choices[i], len(q.Options)
		switch {
		case !q.Multi && c.Text == "":
			steps = append(steps, key(strconv.Itoa(c.Picks[0])))
		case !q.Multi:
			steps = append(steps, key(strconv.Itoa(n+1)), Pause, text(c.Text), key("Enter"))
		default:
			for _, p := range c.Picks {
				steps = append(steps, key(strconv.Itoa(p)))
			}
			if c.Text != "" {
				steps = append(steps, down(n), text(c.Text), down(1))
			} else {
				steps = append(steps, down(n+1))
			}
			steps = append(steps, key("Enter"))
		}
		steps = append(steps, Pause)
	}
	if len(ask.Questions) == 1 && !ask.Questions[0].Multi {
		return steps[:len(steps)-1], pages // the pick sends it: no review page
	}
	pages = append(pages, len(steps))
	return append(steps, key("1")), pages
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
