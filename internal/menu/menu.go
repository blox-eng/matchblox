// Package menu reads the numbered menu an agent draws at the foot of its
// screen: a permission prompt, a question, a choice. It reads text only,
// so the console and the service read a screen the same way.
package menu

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// optionLine is a line of a menu: its mark or blanks, the option's number,
// and its text.
var optionLine = regexp.MustCompile(`^\s*([›❯>])?\s*(\d+)\.\s+(.*?)\s*$`)

// hintLine is the keys line under a menu ("Enter to select · … · Esc to
// cancel").
var hintLine = regexp.MustCompile(`(?i)\bto (select|navigate|cancel)\b`)

// Read is the menu an agent draws at the foot of screen: its options by
// number, each text with the mark and spaces trimmed, and the lines of an
// option that wraps (indented, with no number) joined to it. The menu is
// the last run of options N down to 1. Below it are only blank lines, box
// lines and a keys hint. Nil when there is no menu: a numbered list in a
// reply has other text under it, or the agent's empty input, which a menu
// takes the place of.
func Read(screen string) map[int]string {
	menu, _, _ := read(screen)
	return menu
}

// Chosen is the number of the option the menu at the foot of screen
// marks; 0 when there is no menu or no option is marked.
func Chosen(screen string) int {
	_, _, chosen := read(screen)
	return chosen
}

// signLines is how many lines above a menu its signature holds: the
// question, and the command a permission prompt asks about.
const signLines = 12

// Sign is the signature of the menu at the foot of screen: a hash of its
// lines and the signLines lines above it, each with its box and blanks
// taken off. Two prompts with the same options for two commands sign
// apart. "" when there is no menu.
func Sign(screen string) string {
	menu, top, _ := read(screen)
	if menu == nil {
		return ""
	}
	lines := strings.Split(ansi.Strip(screen), "\n")
	h := sha256.New()
	for _, l := range lines[max(top-signLines, 0):] {
		if t := strings.TrimSpace(unboxed(l)); t != "" {
			h.Write([]byte(t + "\n"))
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// read is Read, the index of the menu's first line in screen, and Chosen.
func read(screen string) (map[int]string, int, int) {
	lines := strings.Split(ansi.Strip(screen), "\n")
	i := len(lines) - 1
	for ; i >= 0; i-- { // the foot
		if t := unboxed(lines[i]); t != "" && !hintLine.MatchString(t) {
			break
		}
	}
	menu := map[int]string{}
	var wrapped []string
	want := 0 // the number the next option up has; 0 before the first
	chosen := 0
	for ; i >= 0 && want != 1; i-- {
		l := unboxed(lines[i])
		g := optionLine.FindStringSubmatch(l)
		switch {
		case g != nil:
			n, err := strconv.Atoi(g[2])
			if err != nil || n < 1 || want != 0 && n != want-1 {
				return nil, 0, 0
			}
			if g[1] != "" {
				chosen = n
			}
			menu[n] = strings.Join(append([]string{g[3]}, wrapped...), " ")
			want, wrapped = n, nil
		case l == "":
			// a blank or a rule between options
		case indent(l) >= 3:
			wrapped = append([]string{strings.TrimSpace(l)}, wrapped...)
		default:
			return nil, 0, 0
		}
	}
	if want != 1 {
		return nil, 0, 0
	}
	return menu, i + 1, chosen
}

// unboxed is a line with a box's sides and its end blanks taken off; ""
// for a blank line or a line of a box's rule.
func unboxed(l string) string {
	l = strings.TrimRight(l, " ")
	if t, ok := strings.CutPrefix(strings.TrimLeft(l, " "), "│"); ok {
		l = strings.TrimRight(strings.TrimSuffix(strings.TrimRight(t, " "), "│"), " ")
	}
	if strings.Trim(l, " ─│╭╮╰╯") == "" {
		return ""
	}
	return l
}

// indent counts the blanks a line starts with.
func indent(l string) int { return len(l) - len(strings.TrimLeft(l, " ")) }
