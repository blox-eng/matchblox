package app

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// night: the stoker is in charge, and the whole console wears the night.
func (m Model) night() bool {
	return m.stoker != nil && m.stoker.On && m.stoker.Rests.IsZero()
}

// paint lays bg under every cell of a frame: at the start of each line,
// after each escape that clears the background, and in the cells the line
// leaves empty up to w. A terminal has no transparency; this is the
// night's ambient light, the same in tmux, over ssh and on a phone.
func paint(view string, bg color.Color, w int) string {
	r, g, b, _ := bg.RGBA()
	on := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
	lines := strings.Split(view, "\n")
	for i, l := range lines {
		var out strings.Builder
		out.WriteString(on)
		for j := 0; j < len(l); j++ {
			if l[j] != 0x1b || j+1 >= len(l) || l[j+1] != '[' {
				out.WriteByte(l[j])
				continue
			}
			k := j + 2
			for k < len(l) && (l[k] < 0x40 || l[k] > 0x7e) {
				k++
			}
			if k >= len(l) {
				out.WriteString(l[j:])
				break
			}
			out.WriteString(l[j : k+1])
			if l[k] == 'm' && clearsBG(l[j+2:k]) {
				out.WriteString(on)
			}
			j = k
		}
		out.WriteString(strings.Repeat(" ", max(w-lipgloss.Width(l), 0)))
		out.WriteString("\x1b[m")
		lines[i] = out.String()
	}
	return strings.Join(lines, "\n")
}

// clearsBG tells if SGR parameters reset the background: a reset (empty or
// 0) or 49. The parts of a 38/48 colour are skipped, so a 0 in an RGB value
// is not read as a reset.
func clearsBG(params string) bool {
	if params == "" {
		return true
	}
	ps := strings.Split(params, ";")
	for k := 0; k < len(ps); k++ {
		switch ps[k] {
		case "", "0", "49":
			return true
		case "38", "48", "58":
			if k+1 < len(ps) && ps[k+1] == "5" {
				k += 2
			} else {
				k += 4
			}
		}
	}
	return false
}
