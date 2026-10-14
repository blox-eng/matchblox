package stoker

import (
	"bufio"
	"bytes"
	"strings"
)

// Recipes are the names of the `# Recipe: <name>` headings of the ways
// file, in file order.
func Recipes(md []byte) []string {
	var names []string
	sc := bufio.NewScanner(bytes.NewReader(md))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		head, ok := strings.CutPrefix(sc.Text(), "#")
		if !ok || strings.HasPrefix(head, "#") {
			continue
		}
		rest, ok := strings.CutPrefix(strings.TrimSpace(head), "Recipe:")
		if name := strings.TrimSpace(rest); ok && name != "" {
			names = append(names, name)
		}
	}
	return names
}
