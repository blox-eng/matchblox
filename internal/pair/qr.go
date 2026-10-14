package pair

import (
	"strings"

	"rsc.io/qr"
)

const quiet = 2

// QR draws text as terminal lines, two modules to a cell. Light modules are
// the filled glyphs, so the code scans on a dark terminal.
func QR(text string) ([]string, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil, err
	}
	light := func(x, y int) bool {
		if x < 0 || y < 0 || x >= code.Size || y >= code.Size {
			return true
		}
		return !code.Black(x, y)
	}
	glyphs := [4]string{" ", "▄", "▀", "█"}
	total := code.Size + 2*quiet
	var lines []string
	for row := 0; row < total; row += 2 {
		var sb strings.Builder
		for col := 0; col < total; col++ {
			i := 0
			if light(col-quiet, row-quiet) {
				i += 2
			}
			if light(col-quiet, row+1-quiet) {
				i++
			}
			sb.WriteString(glyphs[i])
		}
		lines = append(lines, sb.String())
	}
	return lines, nil
}
