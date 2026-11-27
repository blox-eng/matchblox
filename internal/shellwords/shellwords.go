// Package shellwords reads a command line as a shell reads Pocket's
// quoting.
package shellwords

import (
	"errors"
	"strings"
)

// Split splits line into words: blanks between words, single quotes around
// a word's text, and a backslash before a character outside quotes, which
// is how a quoted word holds a quote.
func Split(line string) ([]string, error) {
	var out []string
	var w strings.Builder
	begun, quoted := false, false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quoted && c == '\'':
			quoted = false
		case quoted:
			w.WriteByte(c)
		case c == '\'':
			quoted, begun = true, true
		case c == '\\' && i+1 < len(line):
			i++
			w.WriteByte(line[i])
			begun = true
		case c == ' ':
			if begun {
				out = append(out, w.String())
				w.Reset()
				begun = false
			}
		default:
			w.WriteByte(c)
			begun = true
		}
	}
	if quoted {
		return nil, errors.New("a quote is not closed")
	}
	if begun {
		out = append(out, w.String())
	}
	return out, nil
}
