package said

import (
	"regexp"
	"strings"
	"unicode"
)

// menuWithin is how many lines from the bottom of a pane the question
// tool's menu may reach: a question, its options with their descriptions,
// the typing row and the key hints.
const menuWithin = 40

// optionRe is a numbered row of a menu: a cursor mark, the digit and a dot,
// then a tick box when the question takes several choices.
var optionRe = regexp.MustCompile(`^[❯›>\s]*(\d+)\.\s+(?:\[[^\]]*\]\s*|[☐☑☒✔✓◯◉●○]\s*)?(.*)$`)

// ShowsQuestion reports whether a pane's screen shows the question tool's
// menu on q: its options numbered in order at the bottom, and q's text
// above the first of them. A pane that moved to another page, or that a
// person answered by hand, shows another menu or none.
func ShowsQuestion(screen string, q Question) bool {
	if len(q.Options) == 0 {
		return false
	}
	ls := menuLines(screen)
	first := -1
	for i := len(ls) - 1; i >= 0; i-- {
		if n, label, ok := option(ls[i]); ok && n == 1 && sameLabel(label, q.Options[0].Label) {
			first = i
			break
		}
	}
	if first < 0 {
		return false
	}
	next := 2
	for _, l := range ls[first+1:] {
		if next > len(q.Options) {
			break
		}
		if n, label, ok := option(l); ok && n == next && sameLabel(label, q.Options[next-1].Label) {
			next++
		}
	}
	if next <= len(q.Options) {
		return false
	}
	above := strings.Join(ls[max(first-12, 0):first], " ")
	return strings.Contains(squash(above), prefix(squash(q.Question), 60))
}

// ShowsReview reports whether a pane's screen shows the question tool's
// last page, whose first row sends every answer.
func ShowsReview(screen string) bool {
	for _, l := range menuLines(screen) {
		if n, label, ok := option(l); ok && n == 1 && strings.HasPrefix(label, "Submit answers") {
			return true
		}
	}
	return false
}

// menuLines are the bottom lines of a screen without the frame around
// them; the empty ones go.
func menuLines(screen string) []string {
	raw := strings.Split(Clean(screen), "\n")
	raw = raw[max(len(raw)-menuWithin, 0):]
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		t := strings.TrimFunc(r, func(c rune) bool { return unicode.IsSpace(c) || c == '│' || c == '┃' })
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func option(l string) (int, string, bool) {
	m := optionRe.FindStringSubmatch(l)
	if m == nil {
		return 0, "", false
	}
	n := 0
	for _, c := range m[1] {
		n = n*10 + int(c-'0')
	}
	return n, strings.TrimSpace(m[2]), true
}

// sameLabel: the row starts with the label, or with as much of it as a
// narrow pane leaves before it cuts the row.
func sameLabel(row, label string) bool {
	row, label = squash(row), squash(label)
	if label == "" {
		return row == ""
	}
	p := prefix(label, 24)
	return strings.HasPrefix(row, p) || len([]rune(row)) >= 8 && strings.HasPrefix(p, strings.TrimSuffix(row, "…"))
}

// squash makes text comparable across wrapping: one space between words.
func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

func prefix(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
