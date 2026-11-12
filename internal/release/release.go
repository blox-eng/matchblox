package release

import (
	"cmp"
	"regexp"
	"strconv"
	"strings"
)

var tag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

// IsRelease reports whether s is a release tag, as install.sh and
// publish.yml accept it.
func IsRelease(s string) bool { return tag.MatchString(s) }

// IsPre reports whether s is a release tag with a pre-release suffix
// (v0.43.0-rc.1, or a pseudo-version): one never offered or installed by
// itself.
func IsPre(s string) bool { return IsRelease(s) && strings.Contains(s, "-") }

var pseudo = regexp.MustCompile(`[-.][0-9]{14}-[0-9a-f]{12}$`)

// IsPseudo reports whether s is a version Go made up for a build from a
// commit (v0.42.1-0.20261112093000-f15b80c1abcd), not a release.
func IsPseudo(s string) bool { return IsRelease(s) && pseudo.MatchString(s) }

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
	return comparePre(pa, pb)
}

// comparePre orders two pre-release suffixes one identifier at a time, as
// semver does: numbers by value and before words, and the shorter of two
// that start the same first.
func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, xerr := strconv.Atoi(as[i])
		y, yerr := strconv.Atoi(bs[i])
		switch {
		case xerr == nil && yerr == nil:
			if x != y {
				return cmp.Compare(x, y)
			}
		case xerr == nil:
			return -1
		case yerr == nil:
			return 1
		case as[i] != bs[i]:
			return strings.Compare(as[i], bs[i])
		}
	}
	return cmp.Compare(len(as), len(bs))
}

func parts(tag string) (n [3]int, pre string) {
	core, pre, _ := strings.Cut(strings.TrimPrefix(tag, "v"), "-")
	for i, s := range strings.SplitN(core, ".", 3) {
		n[i], _ = strconv.Atoi(s)
	}
	return n, pre
}
