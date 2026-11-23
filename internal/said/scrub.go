package said

import (
	"regexp"
	"slices"
	"strings"
)

// Scrub hides folders and names a screen must not carry out of the machine.
// Each dir becomes <dir>, longest first. The first name becomes <host>, the
// second <user>, and only as a whole word, so a short name mangles no word
// that holds it. A name under 2 characters stays.
func Scrub(screen string, dirs, names []string) string {
	dirs = slices.Clone(dirs)
	slices.SortFunc(dirs, func(a, b string) int { return len(b) - len(a) })
	for _, d := range dirs {
		if d != "" {
			screen = strings.ReplaceAll(screen, d, "<dir>")
		}
	}
	for i, label := range []string{"<host>", "<user>"} {
		if i < len(names) && len(names[i]) >= 2 {
			re := regexp.MustCompile(`\b` + regexp.QuoteMeta(names[i]) + `\b`)
			screen = re.ReplaceAllLiteralString(screen, label)
		}
	}
	return screen
}
