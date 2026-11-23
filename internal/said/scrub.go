package said

import (
	"regexp"
	"slices"
	"strings"
)

// Scrub hides folders and names a screen must not carry out of the machine.
// Each dir becomes <dir>, longest first. The first name becomes <host>, and
// so does its part before the first dot, which a prompt prints; the second
// becomes <user>. A name is replaced only as a whole word, so a short name
// mangles no word that holds it. A name under 2 characters stays.
func Scrub(screen string, dirs, names []string) string {
	dirs = slices.Clone(dirs)
	slices.SortFunc(dirs, func(a, b string) int { return len(b) - len(a) })
	for _, d := range dirs {
		if d != "" {
			screen = strings.ReplaceAll(screen, d, "<dir>")
		}
	}
	var host, user string
	if len(names) > 0 {
		host = names[0]
	}
	if len(names) > 1 {
		user = names[1]
	}
	short, _, _ := strings.Cut(host, ".")
	for _, n := range []struct{ name, label string }{{host, "<host>"}, {short, "<host>"}, {user, "<user>"}} {
		if len(n.name) >= 2 {
			re := regexp.MustCompile(`\b` + regexp.QuoteMeta(n.name) + `\b`)
			screen = re.ReplaceAllLiteralString(screen, n.label)
		}
	}
	return screen
}
