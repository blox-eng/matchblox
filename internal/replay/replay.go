// Package replay turns frames of the console into the files the site and
// the README play: JSON frames, an animated SVG, and one still frame.
package replay

import (
	"encoding/json"
	"fmt"
	"html"
	"image/color"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Span is a run of text in one style. FG and BG name colour tokens; empty is
// the default colour.
type Span struct {
	Text   string
	FG, BG string
	Bold   bool
	Under  bool
}

// Frame is the screen at a time in the script.
type Frame struct {
	At    time.Duration
	Lines [][]Span
}

func hex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}

// Parse reads a rendered view: SGR truecolour, bold, underline and reset.
// A colour that is not a token is an error, so the replay can follow the
// site's theme.
func Parse(view string, tokens map[string]color.Color) ([][]Span, error) {
	names := make(map[string]string, len(tokens))
	for n, c := range tokens {
		names[hex(c)] = n
	}
	var (
		lines [][]Span
		line  []Span
		cur   Span
		text  strings.Builder
	)
	flush := func() {
		if text.Len() == 0 {
			return
		}
		s := cur
		s.Text = text.String()
		text.Reset()
		if n := len(line); n > 0 && sameStyle(line[n-1], s) {
			line[n-1].Text += s.Text
			return
		}
		line = append(line, s)
	}
	for i := 0; i < len(view); i++ {
		switch c := view[i]; {
		case c == '\n':
			flush()
			lines, line = append(lines, line), nil
		case c == 0x1b && i+1 < len(view) && view[i+1] == '[':
			j := i + 2
			for j < len(view) && (view[j] < 0x40 || view[j] > 0x7e) {
				j++
			}
			if j >= len(view) {
				return nil, fmt.Errorf("unterminated escape at %d", i)
			}
			if view[j] == 'm' {
				flush()
				if err := sgr(&cur, view[i+2:j], names); err != nil {
					return nil, err
				}
			}
			i = j
		default:
			text.WriteByte(c)
		}
	}
	flush()
	return append(lines, line), nil
}

func sameStyle(a, b Span) bool {
	return a.FG == b.FG && a.BG == b.BG && a.Bold == b.Bold && a.Under == b.Under
}

func sgr(s *Span, params string, names map[string]string) error {
	if params == "" {
		*s = Span{}
		return nil
	}
	ps := strings.Split(params, ";")
	for k := 0; k < len(ps); k++ {
		switch ps[k] {
		case "0", "":
			*s = Span{}
		case "1":
			s.Bold = true
		case "4":
			s.Under = true
		case "22":
			s.Bold = false
		case "24":
			s.Under = false
		case "39":
			s.FG = ""
		case "49":
			s.BG = ""
		case "38", "48":
			if k+4 >= len(ps) || ps[k+1] != "2" {
				return fmt.Errorf("colour %q: only truecolour is recorded", params)
			}
			var rgb [3]int
			for n := range rgb {
				v, err := strconv.Atoi(ps[k+2+n])
				if err != nil {
					return fmt.Errorf("colour %q: %w", params, err)
				}
				rgb[n] = v
			}
			h := fmt.Sprintf("#%02X%02X%02X", rgb[0], rgb[1], rgb[2])
			name, ok := names[h]
			if !ok {
				return fmt.Errorf("colour %s is not a theme token", h)
			}
			if ps[k] == "38" {
				s.FG = name
			} else {
				s.BG = name
			}
			k += 4
		}
	}
	return nil
}

func flags(s Span) int {
	f := 0
	if s.Bold {
		f |= 1
	}
	if s.Under {
		f |= 2
	}
	return f
}

// JSON is the file the site plays. A line equal to the same line of the
// frame before is null.
func JSON(cols, rows int, frames []Frame) ([]byte, error) {
	type jframe struct {
		At    int64 `json:"at"`
		Lines []any `json:"lines"`
	}
	doc := struct {
		Cols   int      `json:"cols"`
		Rows   int      `json:"rows"`
		Frames []jframe `json:"frames"`
	}{Cols: cols, Rows: rows}
	var prev [][]Span
	for _, f := range frames {
		jf := jframe{At: f.At.Milliseconds(), Lines: make([]any, len(f.Lines))}
		for i, l := range f.Lines {
			if i < len(prev) && reflect.DeepEqual(prev[i], l) {
				continue
			}
			spans := make([][]any, len(l))
			for k, s := range l {
				spans[k] = []any{s.Text, s.FG, s.BG, flags(s)}
			}
			jf.Lines[i] = spans
		}
		doc.Frames = append(doc.Frames, jf)
		prev = f.Lines
	}
	return json.Marshal(doc)
}

func classes(s Span) string {
	var c []string
	if s.FG != "" {
		c = append(c, "fg-"+s.FG)
	}
	if s.BG != "" {
		c = append(c, "bg-"+s.BG)
	}
	if s.Bold {
		c = append(c, "b")
	}
	if s.Under {
		c = append(c, "u")
	}
	return strings.Join(c, " ")
}

// Still is one frame as HTML for the page without JavaScript. Colours are
// classes (the CSP allows no inline style).
func Still(f Frame) string {
	var b strings.Builder
	b.WriteString(`<pre class="replay" aria-hidden="true">`)
	for i, l := range f.Lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		for _, s := range l {
			t := html.EscapeString(s.Text)
			if c := classes(s); c != "" {
				fmt.Fprintf(&b, `<span class="%s">%s</span>`, c, t)
			} else {
				b.WriteString(t)
			}
		}
	}
	b.WriteString(`</pre>`)
	return b.String()
}
