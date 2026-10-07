package replay

import (
	"fmt"
	"html"
	"image/color"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const (
	cellW    = 8.4
	cellH    = 17.0
	margin   = 12
	fontSize = 14
	hold     = 3.0 // seconds the last frame stays before the loop starts again
)

// SVG is the replay as an animated SVG for the README: CSS keyframes, no
// script, colours from the dark tokens with a light block. Half blocks are
// drawn as rectangles, so the mark is exact at every size.
func SVG(cols, rows int, frames []Frame, tokens map[bool]map[string]color.Color) []byte {
	w, h := float64(cols)*cellW, float64(rows)*cellH
	period := hold
	if n := len(frames); n > 0 {
		period += frames[n-1].At.Seconds()
	}
	var b strings.Builder
	// A margin round the screen, so a glyph in the first or last column is
	// never cut where the SVG is shown edge to edge (the README).
	vw, vh := w+2*margin, h+2*margin
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="-%d -%d %.0f %.0f" width="%.0f" height="%.0f" font-family="ui-monospace,SFMono-Regular,Menlo,Consolas,monospace" font-size="%d" role="img" aria-label="The matchblox console">`, margin, margin, vw, vh, vw, vh, fontSize)
	b.WriteString("<style>")
	b.WriteString(palette(tokens[true]))
	b.WriteString("@media (prefers-color-scheme: light){" + palette(tokens[false]) + "}")
	b.WriteString("text{white-space:pre}.b{font-weight:600}.u{text-decoration:underline}g{opacity:0}")
	for i, f := range frames {
		start := f.At.Seconds() / period * 100
		end := 100.0
		if i+1 < len(frames) {
			end = frames[i+1].At.Seconds() / period * 100
		}
		fmt.Fprintf(&b, ".f%d{animation:k%d %.2fs step-end infinite}@keyframes k%d{0%%{opacity:%d}%.3f%%{opacity:1}%.3f%%{opacity:%d}}",
			i, i, period, i, boolInt(start == 0), start, end, boolInt(end == 100))
	}
	fmt.Fprintf(&b, "@media (prefers-reduced-motion: reduce){g{animation:none!important}.f%d{opacity:1}}", len(frames)-1)
	b.WriteString("</style>")
	fmt.Fprintf(&b, `<rect class="c-bg" x="-%d" y="-%d" width="%.0f" height="%.0f"/>`, margin, margin, vw, vh)
	for i, f := range frames {
		fmt.Fprintf(&b, `<g class="f%d">`, i)
		for y, l := range f.Lines {
			col := 0
			for _, s := range l {
				col = span(&b, s, col, y)
			}
		}
		b.WriteString("</g>")
	}
	b.WriteString("</svg>")
	return []byte(b.String())
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func palette(t map[string]color.Color) string {
	names := make([]string, 0, len(t))
	for n := range t {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, ".c-%s{fill:%s}", n, hex(t[n]))
	}
	return b.String()
}

// span draws one span from column col on row y and returns the next column.
func span(b *strings.Builder, s Span, col, y int) int {
	x0, top := float64(col)*cellW, float64(y)*cellH
	width := ansi.StringWidth(s.Text)
	if strings.Trim(s.Text, "▀▄") == "" {
		for i, r := range []rune(s.Text) {
			x := float64(col+i) * cellW
			upper, lower := s.FG, s.BG
			if r == '▄' {
				upper, lower = s.BG, s.FG
			}
			if upper != "" {
				fmt.Fprintf(b, `<rect class="c-%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f"/>`, upper, x, top, cellW+0.2, cellH/2+0.2)
			}
			if lower != "" {
				fmt.Fprintf(b, `<rect class="c-%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f"/>`, lower, x, top+cellH/2, cellW+0.2, cellH/2+0.2)
			}
		}
		return col + width
	}
	if s.BG != "" {
		fmt.Fprintf(b, `<rect class="c-%s" x="%.1f" y="%.1f" width="%.1f" height="%.0f"/>`, s.BG, x0, top, float64(width)*cellW, cellH)
	}
	if strings.TrimSpace(s.Text) != "" {
		fg := s.FG
		if fg == "" {
			fg = "fg"
		}
		cls := "c-" + fg
		if s.Bold {
			cls += " b"
		}
		if s.Under {
			cls += " u"
		}
		fmt.Fprintf(b, `<text class="%s" x="%.1f" y="%.1f" textLength="%.1f" lengthAdjust="spacingAndGlyphs">%s</text>`,
			cls, x0, top+cellH*0.75, float64(width)*cellW, html.EscapeString(s.Text))
	}
	return col + width
}
