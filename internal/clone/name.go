// Package clone clones a repo onto the host and names its folder.
package clone

import "math/rand/v2"

// Words are the suffixes FreeName tries, in this order of preference.
var Words = []string{"ember", "flint", "spark", "cinder", "tinder", "wick", "flare", "char", "soot", "coal"}

const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// FreeName returns a folder name for repo that taken does not report: repo-word
// for a random free word, else repo-word-xxxx with four random characters. The
// caller checks the plain repo name first.
func FreeName(repo string, taken func(name string) bool, rnd *rand.Rand) string {
	for _, i := range rnd.Perm(len(Words)) {
		if n := repo + "-" + Words[i]; !taken(n) {
			return n
		}
	}
	for {
		b := make([]byte, 4)
		for i := range b {
			b[i] = alphabet[rnd.IntN(len(alphabet))]
		}
		n := repo + "-" + Words[rnd.IntN(len(Words))] + "-" + string(b)
		if !taken(n) {
			return n
		}
	}
}
