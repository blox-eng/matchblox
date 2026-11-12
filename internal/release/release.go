package release

import (
	"regexp"
	"strconv"
	"strings"
)

var tag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

// IsRelease reports whether s is a release tag, as install.sh and
// publish.yml accept it.
func IsRelease(s string) bool { return tag.MatchString(s) }

// Compare orders two release tags by their numbers: -1, 0 or 1. A
// pre-release of a version comes before the version.
func Compare(a, b string) int {
	na, pa := parts(a)
	nb, pb := parts(b)
	for i := range na {
		if na[i] != nb[i] {
			if na[i] < nb[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case pa == pb:
		return 0
	case pa == "":
		return 1
	case pb == "":
		return -1
	}
	return strings.Compare(pa, pb)
}

func parts(tag string) (n [3]int, pre string) {
	core, pre, _ := strings.Cut(strings.TrimPrefix(tag, "v"), "-")
	for i, s := range strings.SplitN(core, ".", 3) {
		n[i], _ = strconv.Atoi(s)
	}
	return n, pre
}
